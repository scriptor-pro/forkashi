package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
	"okashi/internal/textarea"
)

// version is overridden at build time via -ldflags "-X main.version=…".
// The Homebrew formula injects the release tag here.
var version = "dev"

const usage = `okashi — a minimal terminal writing app

Usage:
  okashi              open the writing app
  okashi <dir>        open in a specific folder (overrides $OKASHI_DIR)
  okashi --version    print the version
  okashi --help       show this help

With no path, files open in $OKASHI_DIR, else iCloud Drive's okashi folder,
else ~/Documents/okashi. Inside the app, ctrl+p toggles a Markdown preview.`

// defaultColumnWidth is the target writing measure (the readable "ideal
// measure" is ~66). Override with OKASHI_WIDTH.
const defaultColumnWidth = 72

const helpText = `NAVIGUER
  ctrl+o accueil   ctrl+b panneau   ctrl+y inspecteur
  ctrl+k tableau   ctrl+l plan   esc retour/focus

FICHIERS  (focus panneau)
  ctrl+n nouveau · chapitre|ressource|scène   r renommer (F2)
  d dupliquer   M déplacer   suppr supprimer

MANUSCRIT  (tableau : ctrl+k ou c)
  J/K réordonner (maj+↑↓)   e synopsis   a ajouter   x retirer   r retitrer
  m lecture   b sauvegardes/diff   n notes

PLAN  (ctrl+l)
  maj+↑/↓ déplacer le beat   ctrl+p promouvoir → chapitre   (alt marche aussi)

ÉCRIRE
  ctrl+s enregistrer   ctrl+z annuler   ⇥/⇧⇥ indenter
  ctrl+t machine à écrire   ctrl+d focus atténué   ctrl+x sélection
  ctrl+r orthographe   ⌥/⇧+glisser sélectionner · ⌘C copier

RELECTURE & SORTIE
  ctrl+f rechercher   ctrl+p aperçu   ctrl+e exporter

OBJECTIFS & TEMPS
  ctrl+g objectifs   ctrl+u sprint   g historique (panneau)

APPLICATION
  i propriétés (accueil)   F1/? aide   ctrl+c quitter`

// resolveColumnWidthEnv reads OKASHI_WIDTH (a column count in [20,200]); the bool reports whether a
// valid value was present (so a higher tier can decide whether to override it).
func resolveColumnWidthEnv() (int, bool) {
	if v := os.Getenv("OKASHI_WIDTH"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 20 && n <= 200 {
			return n, true
		}
	}
	return defaultColumnWidth, false
}

// resolveColumnWidth is the env-or-default column width (thin wrapper over the Env variant).
func resolveColumnWidth() int { n, _ := resolveColumnWidthEnv(); return n }

// resolveSmartQuotesEnv reads OKASHI_SMARTQUOTES; off/false/0 disable, any other non-empty value
// enables. The bool reports whether the env var was set (present) at all.
func resolveSmartQuotesEnv() (bool, bool) {
	v := strings.ToLower(os.Getenv("OKASHI_SMARTQUOTES"))
	if v == "" {
		return true, false
	}
	switch v {
	case "off", "false", "0":
		return false, true
	}
	return true, true
}

// resolveSmartQuotes is the env-or-default smart-quote setting (thin wrapper).
func resolveSmartQuotes() bool { b, _ := resolveSmartQuotesEnv(); return b }

// applyProjectSettings resolves per-project + personal settings for the current file-pane dir and
// applies width/smartquotes to the live editor. Called on each hub → writing transition (opening a
// project can change the measure, since width is now per-project).
func (m *model) applyProjectSettings() {
	eff := resolveSettings(m.files.dir)
	m.colWidth = eff.Width
	m.smartQuotes = eff.Smartquotes
	m.layout() // re-apply the measure immediately (guards on unset window size)
}

// enterWriting switches to the writing screen after applying the opened project's settings.
// A v1 manifest diverts to the migration confirm screen instead — no disk writes happen
// until the author accepts via confirmMigration.
func (m *model) enterWriting() {
	if needsMigration(m.files.dir) {
		v1, _, err := readManifestV1(m.files.dir)
		if err == nil {
			if plan, planErr := migratePlan(m.files.dir, v1); planErr == nil {
				m.migrationPending = &plan
				m.migrationDir = m.files.dir
				return
			}
		}
		// A v1 manifest that fails to plan (malformed title data, etc.) falls
		// through to the normal refuse-to-guess path: applyProjectSettings +
		// screenWriting proceed, and resolveManuscript's existing "unsupported
		// schemaVersion" warning path (readManifest still gates on v2) surfaces
		// the problem to the author exactly like any other unreadable manifest.
	}
	m.applyProjectSettings()
	m.screen = screenWriting
}

// confirmMigration executes the pending v1→v2 migration and enters the writing
// screen. Called when the author accepts the migration confirm screen. On
// failure (e.g. the v1 manifest references a missing or typo'd filename), the
// error surfaces in m.status instead of being silently discarded — otherwise
// the manuscript stays on v1 and needsMigration re-triggers the same failing
// migration on every subsequent launch with no indication why (the bug this
// fixes). The manifest itself is untouched on failure (migrateV1ToV2 validates
// every source file before moving anything), so the author can fix the
// offending file or manifest entry and simply relaunch okashi to retry.
func (m *model) confirmMigration() {
	if m.migrationPending == nil {
		return
	}
	v1, _, err := readManifestV1(m.migrationDir)
	if err == nil {
		if migErr := migrateV1ToV2(m.migrationDir, v1); migErr != nil {
			m.status = "migration échouée : " + migErr.Error()
		}
	}
	m.migrationPending = nil
	m.files.SetDir(m.files.dir) // re-reads entries against the migrated v2 manifest
	m.applyProjectSettings()
	m.screen = screenWriting
}

// cancelMigration dismisses the pending confirm screen without touching disk;
// the manuscript is left exactly as it was (still v1, still readable next time
// via needsMigration).
func (m *model) cancelMigration() {
	m.migrationPending = nil
	m.applyProjectSettings()
	m.screen = screenWriting
}

// enterTextPicker opens the text-picker screen for the sidebar's currently selected
// chapter. Called only when filelist.activate() returned activateTextPicker — the
// caller (Update's sidebar key handling) already confirmed the selection is a
// chapter folder with zero or 2+ texts.
func (m *model) enterTextPicker() {
	folder, ok := m.files.selectedEntryName()
	if !ok {
		return
	}
	ch := chapterByFolder(m.files.view, folder)
	m.textPickerChapter = &ch
	m.textPickerSel = 0
	m.textPickerDir = m.files.dir
	m.screen = screenTextPicker
}

// updateTextPicker handles input while the text-picker screen is showing: up/down
// move the selection, enter opens the chosen text and enters the writing screen,
// esc dismisses without opening anything.
func (m model) updateTextPicker(msg tea.Msg) (tea.Model, tea.Cmd) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	texts := m.textPickerChapter.texts
	switch km.Type {
	case tea.KeyUp:
		if m.textPickerSel > 0 {
			m.textPickerSel--
		}
	case tea.KeyDown:
		if m.textPickerSel < len(texts)-1 {
			m.textPickerSel++
		}
	case tea.KeyEnter:
		if len(texts) == 0 {
			return m, nil // empty state: nothing to open
		}
		t := texts[m.textPickerSel]
		path := filepath.Join(m.textPickerDir, m.textPickerChapter.folder, t.file)
		m.textPickerChapter = nil
		m.loadFile(path)
		m.focus = focusEditor
		m.editor.Focus()
		m.screen = screenWriting
	case tea.KeyEsc:
		m.textPickerChapter = nil
		m.screen = screenWriting
	case tea.KeyCtrlN:
		folder := m.textPickerChapter.folder
		m.textPickerChapter = nil
		m.createChapterFolder = folder
		m.createKind = 3
		m.screen = screenWriting
		m.startInPaneCreate()
		return m, textinput.Blink
	}
	return m, nil
}

// textPickerView renders the list of a chapter's texts, one per line, each with
// its own word count — or an empty-state message if the chapter has none. Placed
// centered over the full width/height (AltScreen only diffs changed lines, so an
// un-filled panel would otherwise leave the previous screen's content showing
// through around it, exactly like the exportChooser overlay in View).
func textPickerView(ch *chapterRef, sel int, dir string, wc *wordCountCache, width, height int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "── %s ──\n\n", ch.title)
	if len(ch.texts) == 0 {
		b.WriteString("  (aucune scène dans ce chapitre)")
		return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, b.String())
	}
	for i, t := range ch.texts {
		marker := "  "
		if i == sel {
			marker = selectedStyle.Render("▸ ")
		}
		words := wc.count(filepath.Join(dir, ch.folder, t.file))
		fmt.Fprintf(&b, "%s%s  %s m\n", marker, t.title, commafy(words))
	}
	b.WriteString("\n↑↓ sélectionner · Entrée ouvrir · Échap annuler")
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, b.String())
}

// smartQuote returns the curly form of a straight quote, as a string (the French
// double-quote form is multi-character: chevron + non-breaking space). It's an opening
// quote at the start of a line or after whitespace / an opening bracket; otherwise
// closing (which also yields the right apostrophe in contractions).
func smartQuote(prev rune, hasPrev bool, q rune) string {
	opening := !hasPrev || prev == ' ' || prev == '\t' || prev == '\n' ||
		prev == '(' || prev == '[' || prev == '{'
	nbsp := string(rune(0x00A0)) // espace insécable normale
	switch q {
	case '\'':
		if opening {
			return string(rune(0x2018)) // U+2018 left single quote
		}
		return string(rune(0x2019)) // U+2019 right single quote
	case '"':
		if opening {
			return "«" + nbsp
		}
		return nbsp + "»"
	}
	return string(q)
}

var fineInsecableSigns = map[rune]bool{'!': true, '?': true, ';': true}

// punctuationSpacing reports whether the space immediately before the cursor should become
// non-breaking before inserting sign, and which non-breaking space to use. ok=false means no
// change (the preceding character is not an ordinary space — nothing to convert).
func punctuationSpacing(prev rune, hasPrev bool, sign rune) (insecable rune, ok bool) {
	if !hasPrev || prev != ' ' {
		return 0, false
	}
	if fineInsecableSigns[sign] {
		return rune(0x202F), true // espace fine insécable
	}
	if sign == ':' {
		return rune(0x00A0), true // espace insécable normale
	}
	return 0, false
}

// nupleInsert reports how many copies of sign to actually insert at the cursor, given count —
// the number of sign already present immediately before the cursor.
//   - count <= 0 (first of a new run): insert 1.
//   - count == 1: insert 2 (jump straight to 3 total — 2 is never a valid resting state in a
//     normal typing flow, since this same function already skipped it going from 0 to 1 to 3).
//   - count == 2: reachable only if the buffer already holds a non-conforming run (e.g. pasted
//     text, not produced by this function itself) — top up to 3 by inserting 1.
//   - count >= 3: insert 0 (the keystroke is swallowed, the sequence is already at its ceiling).
func nupleInsert(count int) int {
	switch {
	case count <= 0:
		return 1
	case count == 1:
		return 2
	case count == 2:
		return 1
	default: // count >= 3
		return 0
	}
}

var listItemRe = regexp.MustCompile(`^(\s*)([-*+]|\d+\.)\s+(.*)$`)

// listContinuation inspects a list line for Enter handling. ok=false means it's
// not a list item (normal Enter). clear=true means the item is empty → end the
// list. Otherwise prefix is inserted after a newline to continue the list.
func listContinuation(line string) (prefix string, clear bool, ok bool) {
	mtch := listItemRe.FindStringSubmatch(line)
	if mtch == nil {
		return "", false, false
	}
	indent, marker, content := mtch[1], mtch[2], mtch[3]
	if strings.TrimSpace(content) == "" {
		return "", true, true
	}
	next := marker
	if n, err := strconv.Atoi(strings.TrimSuffix(marker, ".")); err == nil {
		next = strconv.Itoa(n+1) + "."
	}
	return indent + next + " ", false, true
}

type focus int

const (
	focusSidebar focus = iota
	focusEditor
)

type screen int

const (
	screenHome screen = iota
	screenWriting
	screenManuscript
	screenSearch
	screenMover
	screenProperties
	screenSnapshots
	screenDiff
	screenHeatmap
	screenCorkboard
	screenNotes
	screenAllNotes
	screenOutline
	screenTextPicker
)

const (
	scopeProject = iota
	scopeDocument
	scopeAll
)

const searchLimit = 200

// renameTarget is the item a pending rename prompt will rename.
type renameTarget struct {
	dir             string // directory containing the item
	name            string // current base name
	isDir           bool
	section         bool // a numbered section -> title-only rename (legacy file rename)
	manifestChapter bool // a manifest chapter -> edit items[].title, filename birth-stable
	standaloneScene bool // a manifest v3 standalone scene -> edit items[].chapter.title, filename birth-stable
}

const inspectorWidth = 34
const minEditorMeasure = 50

type model struct {
	width, height int

	files     filelist
	editor    textarea.Model
	nameInput textinput.Model
	preview   viewport.Model

	screen       screen
	homeItems    []homeItem
	homeRegion   homeRegion     // launch screen: which column/group
	homeIndex    int            // index within the region
	homeLastCol  homeRegion     // last column visited (for up-out-of-Actions)
	homeFiles    []homeFileItem // FILES column: the current dir's folders + documents
	homeFilesDir string         // the dir FILES currently shows (drill-down within the selection)

	structureDir   string       // the manuscript being restructured
	structureItems []chapterRef // staged BARE chapter order/membership (committed on exit);
	// real Parts are left untouched by this plan's structure mode
	structureSel        int             // cursor row
	structurePendingNew map[string]bool // new-blank files to create on commit
	structureDirty      bool            // any staged edit?
	structureAdding     bool            // the add-pick sub-mode is open
	structureAddSel     int             // cursor in the add-pick
	structureRenaming   bool            // the retitle field is open (reuses nameInput)
	structureConfirm    bool            // the commit confirm bar is open
	migrationPending    *migrationStep  // non-nil while the v1→v2 confirm screen is showing
	migrationDir        string          // dir being migrated (for confirmMigration/cancelMigration)
	textPickerChapter   *chapterRef     // non-nil while the text-picker screen is showing
	textPickerSel       int             // selected index into textPickerChapter.texts
	textPickerDir       string          // manuscript root, for resolving the chapter's folder path
	librarySelected     int             // index into projects+folders driving FILES
	sources             []source        // library sources; [0] is always the primary (writingDir())
	activeSource        int             // index into sources driving the home library
	pinned              []string        // pinned project/folder paths (persisted via pins.go)
	snippets            *snippetCache

	searchInput     textinput.Model
	searchScope     int // scopeProject | scopeDocument
	searchHits      []searchHit
	searchSel       int
	searchOffset    int
	searchHighlight string // transient: highlight this query on the editor's visible lines
	searchReturn    screen // where ctrl+f was invoked from (esc returns here)
	replaceInput    textinput.Model
	replaceMode     bool // in the search screen: entering a replacement for the current query

	sidebarVisible      bool
	inspector           inspectorModel
	focus               focus
	creatingFile        bool
	creatingFolder      bool
	createPicker        bool   // manuscript ctrl+n: choosing chapter vs resource vs scene
	createKind          int    // 0 normal, 1 chapter, 2 resource, 3 scene (set by the picker)
	createChapterFolder string // target chapter's folder when createKind == 3 (set by the picker or the text-picker screen)
	addingSource        bool   // home screen: typing a folder path into nameInput to add a source
	confirmRemoveSource bool   // home screen: y-confirm before detaching the active library source
	previewing          bool
	previewTufte        bool
	previewAvail        int
	typewriter          bool
	dimEnabled          bool

	mdStyle           string // glamour theme, detected once at startup
	colWidth          int
	smartQuotes       bool
	sessionBaseline   int // word count when the current file was opened/created
	now               time.Time
	activeSecs        int
	activeSaveCtr     int
	sprintActive      bool
	sprintEnd         time.Time
	sprintOnBreak     bool
	currentFile       string
	outlineReturnFile string // chapter to return to after editing outline.md (ctrl+l)
	status            string
	icons             iconSet
	todayQuote        quote // resolved once at startup; never recomputed in View()

	pager pagerModel

	renaming       bool
	renamingInPane bool
	creatingInPane bool
	renameTarget   renameTarget

	deleting     bool
	deleteTarget string

	moverSource    string
	moverIsDir     bool
	moverFromDir   string
	moverDestDir   string
	moverEntries   []moverEntry
	moverSel       int
	moverConfirm   bool
	moverAsChapter bool
	moverReturn    screen
	moverError     string

	moverPhase      int          // moverPickSource | moverPickDest
	moverSrcDir     string       // left-pane browse dir (pick-source phase)
	moverSrcEntries []moverEntry // left-pane rows
	moverSrcSel     int

	exportChooser *exportChooserModel // nil when the screen is not showing

	goalsAll        map[string]projectGoals
	goalPromptField int // 0 off, 1 daily words, 2 project words, 3 daily minutes, 4 deadline

	analysis analysisState

	grammarChecker   grammarChecker
	appleFindings    map[string][]grammarFinding
	checkingGrammar  bool
	autoRecheck      bool      // re-run the Apple pass after edits settle (opt-in)
	lastGrammarCheck time.Time // when the last Apple pass was dispatched
	// grammalecteProc is non-nil only when THIS okashi process launched the Grammalecte
	// server itself (OKASHI_GRAMMALECTE_CMD configured, no server was already reachable).
	// It is the ownership marker main() uses to decide whether to kill the subprocess on
	// exit — a server that was already running before okashi started is never touched.
	grammalecteProc *exec.Cmd

	lastClickRow  int
	lastClickTime time.Time

	backedUp    map[string]bool      // files snapshotted this session
	loadedMtime map[string]time.Time // file → mtime at load, for the external-change guard
	undoStack   []string             // coarse per-file buffer checkpoints for ctrl+z undo

	dirty      bool
	lastEditAt time.Time

	suggesting               bool
	suggestions              []string
	suggestIndex             int
	suggestStart, suggestEnd int
	suggestWord              string

	showHelp bool

	selectMode bool // native drag-select mode: okashi's mouse capture is released (ctrl+x toggles)

	properties propertiesModel
	snapshots  snapshotsModel
	diff       diffModel
	heatmap    heatmapModel

	// Corkboard (shares the structure* staged buffer + commitStructure).
	synopses       map[string]string // filename → synopsis, loaded on corkboard entry
	corkFirstLines map[string]string // filename → first prose line, preloaded on entry (keeps View() I/O-free)
	synEditing     bool
	synArea        textarea.Model

	notes    notesModel
	allNotes allNotesModel
}

func initialModel() model {
	maybeSeedSample(writingDir(), seedMarkerPath())
	fl := newFilelist()
	fl.root = writingDir()
	fl.SetDir(writingDir())
	startupSettings := resolveSettings(writingDir())

	quotes := loadQuotes()
	var todayQuote quote
	if len(quotes) > 0 {
		cfgPath := userConfigPath()
		uc := loadUserConfig(cfgPath)
		idx, needsSave := quoteIndexForToday(uc, time.Now(), cfgPath != "", len(quotes))
		todayQuote = quotes[idx]
		if needsSave {
			uc.FirstLaunch = time.Now().Format(quoteDateLayout)
			_ = saveUserConfig(cfgPath, uc) // best-effort: a write failure just re-seeds day 1 next launch
		}
	}

	ta := textarea.New()
	ta.Placeholder = "Commencez à écrire…"
	ta.Prompt = "" // no gutter pipe — read like paper, not code
	ta.ShowLineNumbers = false
	ta.CharLimit = 0 // unlimited
	ta.MaxHeight = 0 // unlimited — chapters routinely exceed the fork's default 99-line cap
	ta.Focus()

	// Strip the textarea's built-in chrome so the prose stands alone.
	ta.FocusedStyle.Base = lipgloss.NewStyle()
	ta.BlurredStyle.Base = lipgloss.NewStyle()
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.Typewriter = true // typewriter scrolling on by default; ctrl+t toggles
	ta.Dim = true
	ta.DimStyle = lipgloss.NewStyle().Foreground(subtle)

	ti := textinput.New()
	ti.Prompt = ""
	ti.Placeholder = "chapter-01.md"
	ti.CharLimit = 255

	vp := viewport.New(defaultColumnWidth, 1) // real size set in layout()

	// Probe the grammar backend's availability once, here, at startup — not on every
	// render. If unavailable (no Grammalecte server running), fall back to nil so
	// m.grammarChecker != nil keeps meaning "backend is actually usable" everywhere it's
	// checked (the analysis action row, the click handlers, the inspector label).
	gcForProbe := newGrammarChecker()
	available := gcForProbe != nil && gcForProbe.Available()
	gc := gcForProbe
	if !available {
		gc = nil
	}

	// Auto-launch: only when nothing answered above AND the user opted in via
	// OKASHI_GRAMMALECTE_CMD. A server that was already reachable is never a candidate
	// for launching — it isn't okashi's to own or later kill (see main()'s cleanup).
	var grammalecteProc *exec.Cmd
	var startupStatus string
	if !available {
		if cmdline, ok := grammalecteAutoLaunchCmd(); ok {
			if proc, err := launchGrammalecte(cmdline); err == nil {
				grammalecteProc = proc
			}
			// err != nil: silent, best-effort — grammarChecker and grammalecteProc both
			// stay nil, identical to today's "no server available" behavior.
		} else {
			startupStatus = "Grammalecte indisponible — configurez OKASHI_GRAMMALECTE_CMD pour un lancement automatique"
		}
	}

	m := model{
		files:           fl,
		editor:          ta,
		nameInput:       ti,
		preview:         vp,
		mdStyle:         previewStyle(),
		colWidth:        startupSettings.Width,
		smartQuotes:     startupSettings.Smartquotes,
		screen:          screenHome,
		homeItems:       buildHomeItems(loadRecents(recentPath()), writingDir(), loadPins(pinsPath())), // writingDir() == activeSourceRoot() at init (activeSource==0 is the primary)
		sources:         loadSources(sourcesPath()),
		pinned:          loadPins(pinsPath()),
		activeSource:    0,
		sidebarVisible:  true,
		focus:           focusSidebar,
		typewriter:      true,
		dimEnabled:      true,
		status:          startupStatus,
		icons:           resolveIcons(),
		todayQuote:      todayQuote,
		goalsAll:        loadGoals(goalsPath()),
		grammarChecker:  gc,
		grammalecteProc: grammalecteProc,
		appleFindings:   map[string][]grammarFinding{},
		snippets:        newSnippetCache(),
		searchInput:     newSearchInput(),
		replaceInput:    newSearchInput(),
		now:             time.Now(),
	}
	m.resetHomeSelection()
	return m
}

// previewStyle picks the glamour theme for the markdown preview. It's resolved
// once, here, because initialModel() runs *before* Bubble Tea takes over the
// terminal — so the one terminal-background query happens on the normal screen
// and can't race Bubble Tea for stdin. $OKASHI_THEME forces a choice.
func previewStyle() string {
	switch strings.ToLower(os.Getenv("OKASHI_THEME")) {
	case "light":
		return "light"
	case "dark":
		return "dark"
	}
	if termenv.HasDarkBackground() {
		return "dark"
	}
	return "light"
}

// writingDir returns the folder okashi opens in on launch. Priority:
//  1. $OKASHI_DIR, if set — lets a user point okashi anywhere.
//  2. iCloud Drive's okashi folder, when iCloud Drive exists on this Mac.
//  3. ~/Documents/okashi as a cross-platform fallback (e.g. iCloud off, Linux).
//
// The chosen folder is created lazily on first run — nothing is written at
// install time, so a Homebrew formula only needs to drop the binary.
func writingDir() string {
	if dir := os.Getenv("OKASHI_DIR"); dir != "" {
		_ = os.MkdirAll(dir, 0o755)
		return dir
	}

	home, err := os.UserHomeDir()
	if err != nil {
		wd, _ := os.Getwd()
		return wd
	}

	// "com~apple~CloudDocs" is Apple's fixed iCloud Drive container id — it's
	// identical for every macOS user, so this path is safe to hardcode.
	icloud := filepath.Join(home, "Library", "Mobile Documents", "com~apple~CloudDocs")
	dir := filepath.Join(home, "Documents", "okashi")
	if fi, err := os.Stat(icloud); err == nil && fi.IsDir() {
		dir = filepath.Join(icloud, "okashi")
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		wd, _ := os.Getwd()
		return wd
	}
	return dir
}

// readOutlineDoc returns the project's outline.md content ("" if none) for the
// inspector's read-only Outline tab.
func readOutlineDoc(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, "outline.md"))
	if err != nil {
		return ""
	}
	return string(data)
}

type autosaveTickMsg time.Time

// autosaveTick schedules the next autosave check. One loop runs for the app's
// lifetime, started in Init.
func autosaveTick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return autosaveTickMsg(t) })
}

// bellCmd rings the terminal bell once. Fired at a pomodoro transition.
func bellCmd() tea.Cmd {
	return func() tea.Msg {
		os.Stdout.Write([]byte{0x07})
		return nil
	}
}

// analysisActionRowY is the inspector body row (content-relative, 0 = tab bar)
// for the "Check grammar" action button — rendered below the Grammar row (5),
// after a blank row (6), at row 7. Does NOT shift the 2 checkbox rows.
// The backend name renders at 8 and the Auto-recheck toggle at 9.
const (
	analysisActionRowY = 7
	analysisAutoRowY   = 9
)

// grammarResultMsg is returned by checkGrammarCmd when the backend finishes.
type grammarResultMsg struct {
	file     string
	findings []grammarFinding
	err      error
}

// checkGrammarCmd runs c.Check asynchronously and returns the result as a
// grammarResultMsg. Called by the action row click handler.
func checkGrammarCmd(c grammarChecker, file, text string) tea.Cmd {
	return func() tea.Msg {
		f, err := c.Check(text)
		return grammarResultMsg{file, f, err}
	}
}

// grammalecteReadyMsg signals that an auto-launched Grammalecte server has become reachable.
// It carries the checker so Update() can activate it without needing outside context.
type grammalecteReadyMsg struct{ checker grammarChecker }

// grammalectePollInterval is the delay between successive availability checks while waiting
// for an auto-launched Grammalecte server to finish starting up (typically ~5-6s in practice).
const grammalectePollInterval = 500 * time.Millisecond

// grammalectePollMaxAttempts caps how long pollGrammalecteCmd keeps retrying (60 × 500ms = 30s)
// before giving up silently — the launched process may have crashed or never bind its port;
// best-effort means okashi keeps running without French grammar checking rather than polling
// forever.
const grammalectePollMaxAttempts = 60

// pollGrammalecteCmd checks whether an auto-launched Grammalecte server has become reachable
// yet. On success it returns grammalecteReadyMsg carrying gc. On failure it sleeps
// grammalectePollInterval and retries, up to grammalectePollMaxAttempts total attempts, after
// which it gives up silently (returns nil — Update() treats a nil tea.Msg as a no-op, so this
// simply stops the polling loop without any user-visible error).
func pollGrammalecteCmd(gc grammarChecker, attempt int) tea.Cmd {
	return func() tea.Msg {
		for {
			if gc.Available() {
				return grammalecteReadyMsg{checker: gc}
			}
			if attempt >= grammalectePollMaxAttempts {
				return nil
			}
			time.Sleep(grammalectePollInterval)
			attempt++
		}
	}
}

// activeIdle is the inactivity window after which the active-time stopwatch pauses.
const activeIdle = 2 * time.Minute

// isWritingActive reports whether the writer edited within the idle window.
func isWritingActive(now, lastEdit time.Time, idle time.Duration) bool {
	return now.Sub(lastEdit) < idle
}

// autosaveDue reports whether the buffer should be flushed: there are unsaved
// edits to a real file and the writer has paused for at least 2s.
func (m model) autosaveDue(now time.Time) bool {
	return m.dirty && m.currentFile != "" && now.Sub(m.lastEditAt) >= 2*time.Second
}

// autoRecheckDue reports whether an automatic Apple grammar pass should fire: opt-in,
// grammar on, a backend present, not already checking, real content, the buffer has been
// idle ≥1.5s, and there's been an edit since the last check (so it fires once per edit
// burst, never on clean/unchanged text).
func (m model) autoRecheckDue(now time.Time) bool {
	return m.autoRecheck && m.analysis.grammar && m.grammarChecker != nil &&
		!m.checkingGrammar && m.currentFile != "" && m.editor.Value() != "" &&
		now.Sub(m.lastEditAt) >= 1500*time.Millisecond &&
		m.lastEditAt.After(m.lastGrammarCheck)
}

// sidebarRow maps an absolute mouse Y to a row index within the file list, or
// -1 if the click is outside the list. The list starts just below the banner.
func sidebarRow(mouseY, bannerH, listHeight int) int {
	row := mouseY - bannerH
	if row < 0 || row >= listHeight {
		return -1
	}
	return row
}

// syncDim keeps the editor's dim state in step with focus mode: dim only when
// typewriter AND dimEnabled.
func (m *model) syncDim() {
	m.editor.Dim = m.typewriter && m.dimEnabled
}

// applyDecorator sets the editor's Decorator from the current analysis toggles.
// Compose order: spell first, then grammar — so spell underline wins.
func (m *model) applyDecorator() {
	a := m.analysis
	grammarOn := a.grammar
	apple := m.appleFindings[m.currentFile] // Tier 2 (Apple) findings, gated by grammar
	build := func(line string, idx int) []textarea.Decoration {
		var d []textarea.Decoration
		if a.spell {
			d = append(d, spellDecorator(line)...)
		}
		if grammarOn {
			// Findings from the active grammarChecker backend (Grammalecte in this
			// fork); keyed by line index, clamped against stale offsets (cleared on
			// edit, but guard anyway).
			nr := len([]rune(line))
			for _, f := range apple {
				if f.Line == idx && f.Start >= 0 && f.Start < f.End && f.End <= nr {
					d = append(d, textarea.Decoration{Start: f.Start, End: f.End, Style: grammarStyle})
				}
			}
		}
		if m.searchHighlight != "" {
			d = append(d, searchDecorator(line, m.searchHighlight)...)
		}
		return d
	}
	if a.spell || grammarOn || m.searchHighlight != "" {
		m.editor.Decorator = build
	} else {
		m.editor.Decorator = nil
	}
}

// invalidateAppleFindings drops the cached Apple (Tier 2) findings for the current file;
// their rune offsets no longer match once the text changes. Re-run "Check grammar" to refresh.
func (m *model) invalidateAppleFindings() {
	// The on-edit hook: drop stale Apple findings AND the transient search highlight, then
	// refresh the decorator if either was active.
	had := len(m.appleFindings[m.currentFile]) > 0 || m.searchHighlight != ""
	delete(m.appleFindings, m.currentFile)
	m.searchHighlight = ""
	if had {
		m.applyDecorator()
	}
}

// maybeOpenAppleSuggestion opens the suggestion bar when an Apple (Tier 2) finding with
// replacements covers the cursor (set by a preceding click). Returns true if it opened one.
func (m *model) maybeOpenGrammarSuggestion() bool {
	f, ok := m.grammarFindingUnderCursor()
	if !ok || len(f.Replacements) == 0 {
		return false
	}
	runes := []rune(m.editor.CurrentLine())
	m.suggesting = true
	m.suggestions = f.Replacements
	m.suggestIndex = 0
	m.suggestWord = string(runes[f.Start:f.End])
	m.suggestStart, m.suggestEnd = f.Start, f.End
	m.status = f.Message
	return true
}

// syncGoal rolls the current project's daily baseline over to today on the first
// writing activity each day, persisting only on change.
func (m *model) syncGoal() {
	pg := m.goalsAll[m.files.dir].applyEnvDefaults()
	total := computeProjStats(m.files.dir, m.files.view, m.files.wc).words
	pg, changed := rolloverIfNeeded(pg, total, today())
	if m.goalsAll == nil {
		m.goalsAll = map[string]projectGoals{}
	}
	m.goalsAll[m.files.dir] = pg
	if changed {
		saveGoals(goalsPath(), m.goalsAll)
	}
}

// wordUnderCursor returns the token spanning the editor cursor on its line (O(line)).
func (m *model) wordUnderCursor() (word string, start, end int, ok bool) {
	line := m.editor.CurrentLine()
	col := m.editor.CursorColumn()
	runes := []rune(line)
	for _, s := range wordSpans(line) {
		if col >= s[0] && col <= s[1] {
			return string(runes[s[0]:s[1]]), s[0], s[1], true
		}
	}
	return "", 0, 0, false
}

// capturingText reports whether a text field currently has focus, so a literal `?`
// should be typed rather than opening the help overlay. (F1 always opens help;
// `?` only opens it where the user isn't typing.)
func (m model) capturingText() bool {
	if m.renaming || m.goalPromptField != 0 || m.suggesting || m.exportChooser != nil || m.creatingFile || m.addingSource {
		return true
	}
	switch m.screen {
	case screenSearch:
		return true
	case screenCorkboard:
		return m.structureRenaming || m.synEditing
	case screenWriting:
		return m.focus == focusEditor && !m.previewing
	}
	return false
}

// cursorSpellHint returns the misspelled word under the cursor and its suggestions
// for passive display, or ok=false. Active only when spellcheck is on, on the
// writing screen, and no modal/prompt is up.
func (m *model) cursorSpellHint() (word string, suggestions []string, ok bool) {
	if !m.analysis.spell || m.screen != screenWriting {
		return "", nil, false
	}
	if m.renaming || m.goalPromptField != 0 || m.suggesting || m.previewing || m.exportChooser != nil || m.creatingFile {
		return "", nil, false
	}
	w, _, _, found := m.wordUnderCursor()
	if !found || spellOK(w) {
		return "", nil, false
	}
	sugg := append(spellSuggest(w, 4), dictItem) // + add-to-dictionary slot
	return w, sugg, true
}

// grammarFindingUnderCursor returns the grammar finding spanning the cursor,
// reported by the active grammarChecker backend (Grammalecte in this fork).
func (m *model) grammarFindingUnderCursor() (grammarFinding, bool) {
	if !m.analysis.grammar {
		return grammarFinding{}, false
	}
	line := m.editor.Line()
	col := m.editor.CursorColumn()
	cur := m.editor.CurrentLine()
	nr := len([]rune(cur))
	for _, f := range m.appleFindings[m.currentFile] {
		if f.Line == line && col >= f.Start && col <= f.End && f.Start < f.End && f.End <= nr {
			return f, true
		}
	}
	return grammarFinding{}, false
}

// cursorGrammarHint returns the wrong text, its fix(es), and the reason for the grammar
// finding under the cursor — the passive bottom-bar hint, mirroring the spell one.
func (m *model) cursorGrammarHint() (word string, suggestions []string, reason string, ok bool) {
	if !m.analysis.grammar || m.screen != screenWriting {
		return "", nil, "", false
	}
	if m.renaming || m.goalPromptField != 0 || m.suggesting || m.previewing || m.exportChooser != nil || m.creatingFile {
		return "", nil, "", false
	}
	f, found := m.grammarFindingUnderCursor()
	if !found || len(f.Replacements) == 0 {
		return "", nil, "", false
	}
	runes := []rune(m.editor.CurrentLine())
	return string(runes[f.Start:f.End]), f.Replacements, f.Message, true
}

// applyGrammarHint applies the i-th replacement for the grammar finding under the cursor.
func (m *model) applyGrammarHint(i int) {
	f, ok := m.grammarFindingUnderCursor()
	if !ok || len(f.Replacements) == 0 {
		return
	}
	runes := []rune(m.editor.CurrentLine())
	m.suggestions = f.Replacements
	m.suggestWord = string(runes[f.Start:f.End])
	m.suggestStart, m.suggestEnd = f.Start, f.End
	m.applySuggestion(i)
}

// matchCase applies orig's capitalization pattern to sugg.
func matchCase(orig, sugg string) string {
	if orig == "" || sugg == "" {
		return sugg
	}
	if isAllCaps(orig) {
		return strings.ToUpper(sugg)
	}
	or := []rune(orig)
	if unicode.IsUpper(or[0]) {
		sr := []rune(sugg)
		sr[0] = unicode.ToUpper(sr[0])
		return string(sr)
	}
	return sugg
}

// applySuggestion applies the i-th suggestion to the editor, replacing the word.
func (m *model) applySuggestion(i int) {
	if i < 0 || i >= len(m.suggestions) {
		m.suggesting = false
		return
	}
	if m.suggestions[i] == dictItem { // add the word to the personal dictionary instead
		m.suggesting = false
		if addToDictionary(m.suggestWord) {
			m.status = "« " + m.suggestWord + " » ajouté au dictionnaire"
		} else {
			m.status = "« " + m.suggestWord + " » est déjà connu"
		}
		m.applyDecorator() // the word is now known — clear its underline
		return
	}
	chosen := matchCase(m.suggestWord, m.suggestions[i])
	m.checkpointUndo() // capture pre-apply state so ctrl+z can undo the fix
	m.editor.ReplaceRange(m.suggestStart, m.suggestEnd, chosen)
	m.status = "« " + m.suggestWord + " » → « " + chosen + " »"
	m.suggesting = false
	m.invalidateAppleFindings() // the edit shifts offsets; refresh via Check grammar
}

// spellHintSuggestionAtX maps a content column on the spell-hint status row to a
// suggestion index. The hint is "✗ " + word + " → " + sugg joined by " · " + "  ·  ^R".
func spellHintSuggestionAtX(word string, sugg []string, localX int) (int, bool) {
	col := lipgloss.Width("✗ " + word + " → ")
	for i, s := range sugg {
		w := lipgloss.Width(s)
		if localX >= col && localX < col+w {
			return i, true
		}
		col += w + lipgloss.Width(" · ")
	}
	return 0, false
}

// openSpellMenuAndApply applies suggestion i for the misspelled word under the cursor.
func (m *model) openSpellMenuAndApply(i int, sugg []string) {
	w, s, e, ok := m.wordUnderCursor()
	if !ok {
		return
	}
	m.suggestions = sugg
	m.suggestWord = w
	m.suggestStart, m.suggestEnd = s, e
	m.applySuggestion(i)
}

func (m model) Init() tea.Cmd {
	if m.grammalecteProc != nil && m.grammarChecker == nil {
		return tea.Batch(autosaveTick(), pollGrammalecteCmd(newGrammarChecker(), 0))
	}
	return autosaveTick()
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// The v1→v2 migration confirm screen intercepts all keys before anything
	// else — m.screen stays screenHome while it's pending, so without this
	// guard the keys would fall through to the home-screen key handling.
	if km, ok := msg.(tea.KeyMsg); ok && m.migrationPending != nil {
		switch km.String() {
		case "enter":
			m.confirmMigration()
		case "esc":
			m.cancelMigration()
		}
		return m, nil
	}

	var cmds []tea.Cmd

	if t, ok := msg.(autosaveTickMsg); ok {
		now := time.Time(t)
		m.now = now
		if isWritingActive(m.now, m.lastEditAt, activeIdle) {
			m.activeSecs++
			if m.goalsAll == nil {
				m.goalsAll = map[string]projectGoals{}
			}
			pg := m.goalsAll[m.files.dir]
			pg, _ = rolloverActive(pg, today())
			pg.ActiveSecsToday++
			// Record today's net words for the pace history (persisted by the periodic save
			// below + on the daily rollover in syncGoal). DayBaseline is kept current by syncGoal
			// on each keystroke, so this delta is accurate while writing is active.
			total := computeProjStats(m.files.dir, m.files.view, m.files.wc).words
			pg, _ = recordDay(pg, total, today()) // rolls DayBaseline first — never a stale delta
			m.goalsAll[m.files.dir] = pg
			m.activeSaveCtr++
			if m.activeSaveCtr >= 60 {
				saveGoals(goalsPath(), m.goalsAll)
				m.activeSaveCtr = 0
			}
		}
		m.checkpointUndo() // coarse per-tick undo checkpoint (no-op if unchanged)
		cmds := []tea.Cmd{autosaveTick()}
		if m.autoRecheckDue(now) {
			m.checkingGrammar = true
			m.lastGrammarCheck = now
			cmds = append(cmds, checkGrammarCmd(m.grammarChecker, m.currentFile, m.editor.Value()))
		}
		if m.sprintActive && !m.now.Before(m.sprintEnd) {
			end, onBreak, active, msg := advanceSprint(m.now, m.sprintEnd, m.sprintOnBreak, 5*time.Minute)
			m.sprintEnd, m.sprintOnBreak, m.sprintActive = end, onBreak, active
			m.status = msg
			cmds = append(cmds, bellCmd())
		}
		if m.autosaveDue(now) {
			m.save()
		}
		return m, tea.Batch(cmds...)
	}

	if msg, ok := msg.(grammarResultMsg); ok {
		m.appleFindings[msg.file] = msg.findings
		m.checkingGrammar = false
		if msg.err == nil {
			m.applyDecorator()
			n := len(msg.findings)
			if n == 1 {
				m.status = "1 remarque grammaticale"
			} else {
				m.status = fmt.Sprintf("%d remarques grammaticales", n)
			}
		} else {
			m.status = "échec de la vérification grammaticale"
		}
		return m, nil
	}

	if msg, ok := msg.(grammalecteReadyMsg); ok {
		m.grammarChecker = msg.checker
		return m, nil
	}

	// Global quit: flush unsaved work first, from any screen.
	if key, ok := msg.(tea.KeyMsg); ok && key.String() == "ctrl+c" {
		m.saveIfDirty()
		return m, tea.Quit
	}

	// Global help overlay — F1 from anywhere, or `?` where the user isn't typing; any key closes it.
	if m.showHelp {
		if key, ok := msg.(tea.KeyMsg); ok {
			if key.String() == "ctrl+c" {
				return m, tea.Quit
			}
			m.showHelp = false
			return m, nil
		}
		return m, nil
	}
	if key, ok := msg.(tea.KeyMsg); ok {
		if key.Type == tea.KeyF1 || (key.String() == "?" && !m.capturingText()) {
			m.showHelp = true
			return m, nil
		}
	}

	if m.screen == screenHome {
		return m.updateHome(msg)
	}

	if m.screen == screenManuscript {
		return m.updateManuscript(msg)
	}

	if m.screen == screenSearch {
		return m.updateSearch(msg)
	}

	if m.screen == screenMover {
		return m.updateMover(msg)
	}

	if m.screen == screenProperties {
		return m.updateProperties(msg)
	}

	if m.screen == screenSnapshots {
		return m.updateSnapshots(msg)
	}

	if m.screen == screenDiff {
		return m.updateDiff(msg)
	}

	if m.screen == screenHeatmap {
		return m.updateHeatmap(msg)
	}

	if m.screen == screenCorkboard {
		return m.updateCorkboard(msg)
	}

	if m.screen == screenNotes {
		return m.updateNotes(msg)
	}

	if m.screen == screenAllNotes {
		return m.updateAllNotes(msg)
	}

	if m.screen == screenOutline {
		return m.updateOutline(msg)
	}

	if m.screen == screenTextPicker {
		return m.updateTextPicker(msg)
	}

	// Manuscript ctrl+n: pick chapter or resource before naming.
	if m.createPicker {
		if key, ok := msg.(tea.KeyMsg); ok {
			switch key.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "c":
				m.createPicker, m.createKind = false, 1
				m.startInPaneCreate()
				return m, textinput.Blink
			case "r":
				m.createPicker, m.createKind = false, 2
				m.startInPaneCreate()
				return m, textinput.Blink
			case "s":
				m.createPicker, m.createKind = false, 3
				m.startInPaneCreate()
				return m, textinput.Blink
			case "esc":
				m.createPicker = false
				m.status = "création annulée"
			}
		}
		return m, nil
	}

	// While naming a new file, the prompt captures all input.
	if m.creatingFile {
		if key, ok := msg.(tea.KeyMsg); ok {
			switch key.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "esc":
				m.creatingFile = false
				m.creatingInPane = false
				m.creatingFolder = false
				m.createKind = 0 // don't let a cancelled chapter|resource pick bleed into the next create
				m.createChapterFolder = ""
				m.nameInput.Blur()
				m.status = "création annulée"
				return m, nil
			case "enter":
				m.confirmCreate()
				return m, nil
			}
		}
		var cmd tea.Cmd
		m.nameInput, cmd = m.nameInput.Update(msg)
		return m, cmd
	}

	if m.renaming {
		if key, ok := msg.(tea.KeyMsg); ok {
			switch key.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "esc":
				m.renaming = false
				m.renamingInPane = false
				m.nameInput.Blur()
				m.status = "renommage annulé"
				m.refreshAfterRename()
				return m, nil
			case "enter":
				m.confirmRename()
				return m, nil
			}
		}
		var cmd tea.Cmd
		m.nameInput, cmd = m.nameInput.Update(msg)
		return m, cmd
	}

	if m.deleting {
		if key, ok := msg.(tea.KeyMsg); ok {
			switch key.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "y":
				m.confirmDelete()
				return m, nil
			default:
				m.deleting = false
				m.status = "suppression annulée"
				return m, nil
			}
		}
		return m, nil
	}

	if m.exportChooser != nil {
		if key, ok := msg.(tea.KeyMsg); ok {
			switch key.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "up", "k":
				m.exportChooser.cursor = (m.exportChooser.cursor - 1 + exportChooserRowCount) % exportChooserRowCount
			case "down", "j":
				m.exportChooser.cursor = (m.exportChooser.cursor + 1) % exportChooserRowCount
			case "left", "right":
				if m.exportChooser.cursor == exportChooserRowCount-1 {
					if m.exportChooser.style == StyleManuscript {
						m.exportChooser.setStyle(StyleTufte)
					} else {
						m.exportChooser.setStyle(StyleManuscript)
					}
				}
			case " ":
				m.exportChooser.toggleAtCursor()
			case "enter":
				if !m.exportChooser.anyChecked() {
					m.status = "choisissez au moins un format"
					return m, nil
				}
				m.runExport()
				m.exportChooser = nil
			case "esc":
				m.exportChooser = nil
				m.status = "export annulé"
			}
		}
		return m, nil
	}

	if m.goalPromptField != 0 {
		if key, ok := msg.(tea.KeyMsg); ok {
			switch key.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "esc":
				m.goalPromptField = 0
				m.nameInput.Blur()
				return m, nil
			case "enter":
				n, _ := strconv.Atoi(strings.TrimSpace(m.nameInput.Value()))
				if m.goalsAll == nil {
					m.goalsAll = map[string]projectGoals{}
				}
				pg := m.goalsAll[m.files.dir]
				if m.goalPromptField == 1 {
					if n >= 0 {
						pg.DailyGoal = n
					}
					m.goalsAll[m.files.dir] = pg
					m.goalPromptField = 2
					m.nameInput.SetValue(strconv.Itoa(pg.applyEnvDefaults().ProjectGoal))
					return m, nil
				}
				if m.goalPromptField == 2 {
					if n >= 0 {
						pg.ProjectGoal = n
					}
					m.goalsAll[m.files.dir] = pg
					m.goalPromptField = 3
					m.nameInput.SetValue(strconv.Itoa(pg.SessionGoalMin))
					return m, nil
				}
				if m.goalPromptField == 3 {
					if n >= 0 {
						pg.SessionGoalMin = n
					}
					m.goalsAll[m.files.dir] = pg
					m.goalPromptField = 4
					m.nameInput.SetValue(pg.Deadline)
					return m, nil
				}
				// Field 4: deadline date (blank clears; validated before saving).
				raw := strings.TrimSpace(m.nameInput.Value())
				if raw == "" {
					pg.Deadline = ""
				} else if _, err := time.Parse("2006-01-02", raw); err == nil {
					pg.Deadline = raw
				} else {
					m.status = "échéance au format AAAA-MM-JJ (ou vide) — réessayez"
					return m, nil
				}
				m.goalsAll[m.files.dir] = pg
				saveGoals(goalsPath(), m.goalsAll)
				m.goalPromptField = 0
				m.nameInput.Blur()
				m.status = "objectifs enregistrés"
				return m, nil
			}
		}
		var cmd tea.Cmd
		m.nameInput, cmd = m.nameInput.Update(msg)
		return m, cmd
	}

	if m.suggesting {
		if key, ok := msg.(tea.KeyMsg); ok {
			switch key.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "esc":
				m.suggesting = false
				m.status = "suggestion annulée"
				return m, nil
			case "left":
				if m.suggestIndex > 0 {
					m.suggestIndex--
				}
				return m, nil
			case "right":
				if m.suggestIndex < len(m.suggestions)-1 {
					m.suggestIndex++
				}
				return m, nil
			case "enter":
				m.applySuggestion(m.suggestIndex)
				return m, nil
			default:
				if len(key.String()) == 1 && key.String() >= "1" && key.String() <= "9" {
					i := int(key.String()[0] - '1')
					if i < len(m.suggestions) {
						m.applySuggestion(i)
					}
				}
				return m, nil
			}
		}
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.layout()
		if m.previewing {
			m.renderPreview()
		}
		return m, nil

	case tea.MouseMsg:
		showSidebar, _, _ := m.effectivePanels()
		inSidebar := showSidebar && msg.X < sidebarWidth

		_, showInspector, _ := m.effectivePanels()
		if showInspector && msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress && msg.Y == 1 {
			localX := msg.X - (m.width - inspectorWidth) - 2 // panel-left + border + padding
			if localX >= 0 {
				if tb, ok := inspectorTabAtX(localX); ok {
					m.inspector.tab = tb
					return m, nil
				}
			}
		}

		if showInspector && msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress && m.inspector.tab == tabAnalysis {
			localX := msg.X - (m.width - inspectorWidth) - 2
			if localX >= 0 {
				if row, ok := inspectorAnalysisRowAtY(msg.Y - 1); ok {
					switch row {
					case 0:
						m.analysis.spell = !m.analysis.spell
					case 1:
						m.analysis.grammar = !m.analysis.grammar
					}
					m.applyDecorator()
					return m, nil
				}
			}
		}

		// Action row: "Check grammar" — only when grammar is on and a backend is available.
		if showInspector && msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress && m.inspector.tab == tabAnalysis {
			localX := msg.X - (m.width - inspectorWidth) - 2
			if m.analysis.grammar && m.grammarChecker != nil && !m.checkingGrammar && localX >= 0 && msg.Y-1 == analysisActionRowY {
				m.checkingGrammar = true
				m.lastGrammarCheck = time.Now()
				m.status = "vérification grammaticale…"
				return m, checkGrammarCmd(m.grammarChecker, m.currentFile, m.editor.Value())
			}
			// Auto-recheck toggle row.
			if m.analysis.grammar && m.grammarChecker != nil && localX >= 0 && msg.Y-1 == analysisAutoRowY {
				m.autoRecheck = !m.autoRecheck
				return m, nil
			}
		}

		// Wheel scrolls whichever pane is under the pointer.
		if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown {
			up := msg.Button == tea.MouseButtonWheelUp
			switch {
			case inSidebar:
				m.focus = focusSidebar
				m.editor.Blur()
				if up {
					m.files.moveBy(-1)
				} else {
					m.files.moveBy(1)
				}
				return m, nil
			case m.previewing:
				// Read-only preview: let the viewport scroll itself.
				var cmd tea.Cmd
				m.preview, cmd = m.preview.Update(msg)
				return m, cmd
			default:
				// Editor: with typewriter the view is caret-locked, so scrolling
				// means moving the caret. Step by the viewport's wheel delta.
				m.focus = focusEditor
				m.editor.Focus()
				const wheelStep = 3
				for i := 0; i < wheelStep; i++ {
					if up {
						m.editor.CursorUp()
					} else {
						m.editor.CursorDown()
					}
				}
				return m, nil
			}
		}

		// Click on the status bar applies a spell suggestion if one is active.
		// The status renders inside the editor column, so its content starts at
		// editorStart + 1 (column left + the style's left padding).
		if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress && msg.Y == m.height-1 {
			if w, sugg, ok := m.cursorSpellHint(); ok {
				showSidebar, _, _ := m.effectivePanels()
				editorStart := 0
				if showSidebar {
					editorStart = sidebarWidth
				}
				if i, hit := spellHintSuggestionAtX(w, sugg, msg.X-editorStart-1); hit {
					m.openSpellMenuAndApply(i, sugg)
					return m, nil
				}
			}
			// Same row for a grammar finding's fix (the reason after it isn't clickable).
			if w, sugg, _, ok := m.cursorGrammarHint(); ok {
				showSidebar, _, _ := m.effectivePanels()
				editorStart := 0
				if showSidebar {
					editorStart = sidebarWidth
				}
				if i, hit := spellHintSuggestionAtX(w, sugg, msg.X-editorStart-1); hit {
					m.applyGrammarHint(i)
					return m, nil
				}
			}
		}

		// Click in the editor area positions the cursor (enables click-to-suggest).
		if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress {
			showSidebar, showInspector, editorArea := m.effectivePanels()
			editorStart := 0
			if showSidebar {
				editorStart = sidebarWidth
			}
			inEditor := msg.X >= editorStart && (!showInspector || msg.X < m.width-inspectorWidth) && msg.Y < m.height-2
			if inEditor && !m.previewing {
				cw := min(m.colWidth, editorArea-2)
				textLeft := editorStart + (editorArea-cw)/2
				m.editor.ClickTo(msg.Y, msg.X-textLeft)
				m.focus = focusEditor
				m.editor.Focus()
				if !m.maybeOpenGrammarSuggestion() { // click a flagged span → suggestion bar;
					m.suggesting = false // otherwise dismiss any open bar
				}
				return m, nil
			}
		}

		// Right-click in the sidebar starts an in-place rename.
		if inSidebar && msg.Button == tea.MouseButtonRight && msg.Action == tea.MouseActionPress {
			if row := sidebarRow(msg.Y, 1, m.files.height); row >= 0 && row < len(m.files.entries) {
				m.focus = focusSidebar
				m.editor.Blur()
				m.files.selectRow(row)
				m.startRename()
				return m, textinput.Blink
			}
		}

		// Click the '+' in the sidebar title bar starts an in-pane create.
		// The '+' is rendered at column sidebarWidth-2 (right side of top border, before ╮).
		if inSidebar && msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress &&
			msg.Y == 0 && msg.X == sidebarWidth-2 {
			m.startInPaneCreate()
			return m, textinput.Blink
		}

		// Click selection / open is file-pane only.
		if !inSidebar || msg.Button != tea.MouseButtonLeft || msg.Action != tea.MouseActionPress {
			return m, nil
		}
		// Framed sidebar: top border occupies row 0; the file list starts at row 1.
		row := sidebarRow(msg.Y, 1, m.files.height)
		if row < 0 || row >= len(m.files.entries) { // ignore clicks on blank rows below the list
			return m, nil
		}
		m.focus = focusSidebar
		m.editor.Blur()
		m.files.selectRow(row)
		now := time.Now()
		if row == m.lastClickRow && now.Sub(m.lastClickTime) < 400*time.Millisecond {
			if path, result := m.files.activate(); result == activateFile {
				m.loadFile(path)
				m.focus = focusEditor
				m.editor.Focus()
			} else if result == activateTextPicker {
				m.enterTextPicker()
			}
			m.lastClickTime = time.Time{} // consume the double-click
		} else {
			m.lastClickRow = row
			m.lastClickTime = now
		}
		return m, nil

	case tea.KeyMsg:
		m.syncGoal()
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "ctrl+o":
			m.previewing = false
			m.screen = screenHome
			m.homeItems = buildHomeItems(loadRecents(recentPath()), m.activeSourceRoot(), m.pinned)
			m.resetHomeSelection()
			if m.selectMode {
				// Select mode disabled mouse capture; the hub needs clicks, and ctrl+x can't be
				// reached there — so restore the mouse on the way out.
				m.selectMode = false
				return m, tea.EnableMouseCellMotion
			}
			return m, nil
		case "ctrl+f":
			m.previewing = false // leave preview if active
			m.searchReturn = m.screen
			m.screen = screenSearch
			m.searchScope = scopeProject
			m.searchInput.SetValue(m.wordUnderCursorOrEmpty())
			m.searchInput.CursorEnd()
			m.searchInput.Focus()
			m.recomputeSearch()
			return m, textinput.Blink
		case "ctrl+n":
			if hasManifest(m.files.dir) {
				// In a manuscript, ask: chapter, resource, or scene — scene targets the
				// selected chapter's Texts when one is selected, else it's a standalone scene.
				m.createPicker = true
				m.createChapterFolder = ""
				if name, ok := m.files.selectedEntryName(); ok && isChapterOf(m.files.view, name) {
					m.createChapterFolder = name
				}
				m.status = "nouveau : c chapitre (ordonné) · r ressource (doc libre) · s scène · esc annuler"
				return m, nil
			}
			m.createKind = 0
			m.startInPaneCreate()
			return m, textinput.Blink
		case "f2":
			m.focus = focusSidebar
			m.editor.Blur()
			m.startRename()
			return m, textinput.Blink
		case "ctrl+p":
			m.togglePreview()
			return m, nil
		case "ctrl+x":
			// Toggle native selection: release okashi's mouse capture so a plain drag selects
			// text (and the terminal's own copy works), then restore capture so clicks work again.
			m.selectMode = !m.selectMode
			if m.selectMode {
				m.status = "-- SÉLECTION -- · glissez pour sélectionner, copiez avec votre terminal · ctrl+x quitte"
				return m, tea.DisableMouse
			}
			m.status = "mode sélection désactivé"
			return m, tea.EnableMouseCellMotion
		case "ctrl+t":
			m.typewriter = !m.typewriter
			m.editor.Typewriter = m.typewriter
			m.syncDim()
			if m.typewriter {
				m.status = "machine à écrire activée"
			} else {
				m.status = "machine à écrire désactivée"
			}
			return m, nil
		case "ctrl+l":
			// ctrl+l opens the full-screen outline mode (ctrl+k opens the corkboard).
			m.enterOutline()
			return m, nil
		case "ctrl+k":
			// The corkboard is full-screen: ctrl+k (and `c` from the sidebar) open it.
			m.enterCorkboard()
			return m, nil
		case "ctrl+e":
			c := newExportChooser()
			m.exportChooser = &c
			return m, nil
		case "ctrl+d":
			m.dimEnabled = !m.dimEnabled
			m.syncDim()
			if m.editor.Dim {
				m.status = "atténuation activée"
			} else {
				m.status = "atténuation désactivée"
			}
			return m, nil
		case "ctrl+b":
			m.sidebarVisible = !m.sidebarVisible
			if !m.sidebarVisible {
				m.focus = focusEditor
				m.editor.Focus()
			}
			m.layout()
			return m, nil
		case "ctrl+y":
			m.inspector.cycle()
			m.layout()
			return m, nil
		case "ctrl+g":
			pg := m.goalsAll[m.files.dir].applyEnvDefaults()
			m.goalPromptField = 1
			m.nameInput.SetValue(strconv.Itoa(pg.DailyGoal))
			m.nameInput.Focus()
			return m, nil
		case "ctrl+u":
			if m.sprintActive {
				m.sprintActive = false
				m.status = "sprint arrêté"
			} else {
				mins := m.goalsAll[m.files.dir].applyEnvDefaults().SprintMin
				m.sprintActive = true
				m.sprintOnBreak = false
				m.sprintEnd = m.now.Add(time.Duration(mins) * time.Minute)
				m.status = fmt.Sprintf("sprint démarré — %d min", mins)
			}
			return m, nil
		case "esc":
			if m.previewing {
				m.togglePreview() // exit preview
				return m, nil
			}
			if m.sidebarVisible {
				if m.focus == focusSidebar {
					m.focus = focusEditor
					m.editor.Focus()
				} else {
					m.focus = focusSidebar
					m.editor.Blur()
				}
			}
			return m, nil
		case "tab":
			if m.focus == focusEditor && !m.previewing {
				m.editor.Indent()
				m.dirty = true
				m.lastEditAt = time.Now()
			}
			return m, nil
		case "shift+tab":
			if m.focus == focusEditor && !m.previewing {
				m.editor.Outdent()
				m.dirty = true
				m.lastEditAt = time.Now()
			}
			return m, nil
		case "ctrl+s":
			m.save()
			return m, nil
		case "ctrl+z":
			m.undo()
			return m, nil
		case "ctrl+r":
			if !m.renaming && m.goalPromptField == 0 && !m.suggesting && !m.previewing {
				w, s, e, ok := m.wordUnderCursor()
				if !ok {
					m.status = "aucun mot sous le curseur"
					return m, nil
				}
				if spellOK(w) {
					m.status = "« " + w + " » semble correct"
					return m, nil
				}
				sugg := append(spellSuggest(w, 7), dictItem) // + add-to-dictionary slot
				m.suggesting = true
				m.suggestions = sugg
				m.suggestIndex = 0
				m.suggestWord = w
				m.suggestStart, m.suggestEnd = s, e
				return m, nil
			}
		}
	}

	// Route everything else to whichever pane has focus. Preview is a modal
	// read-only state, so it wins over pane focus — keys/wheel scroll it and
	// nothing reaches the editor or file list underneath.
	var cmd tea.Cmd
	if m.previewing {
		if key, ok := msg.(tea.KeyMsg); ok && key.String() == "t" {
			m.previewTufte = !m.previewTufte // toggle Default ⇄ Tufte
			m.renderPreview()
			return m, tea.Batch(cmds...)
		}
		m.preview, cmd = m.preview.Update(msg)
		cmds = append(cmds, cmd)
	} else if m.focus == focusSidebar && m.sidebarVisible {
		if key, ok := msg.(tea.KeyMsg); ok {
			if key.Type == tea.KeyDelete {
				m.startDelete()
			} else {
				switch key.String() {
				case "up", "k":
					m.files.moveBy(-1)
				case "down", "j":
					m.files.moveBy(1)
				case "enter", "right", "l":
					if path, result := m.files.activate(); result == activateFile {
						m.loadFile(path)
						m.focus = focusEditor
						m.editor.Focus()
					} else if result == activateTextPicker {
						m.enterTextPicker()
					}
				case "left", "h", "backspace":
					m.files.SetDir(filepath.Dir(m.files.dir))
				case "r":
					m.startRename()
				case "d":
					m.duplicateSelected()
				case "M":
					m.enterMover()
				case "b":
					m.enterSnapshots()
				case "g":
					m.enterHeatmap()
				case "n":
					m.enterNotes()
				case "N":
					m.enterAllNotes()
				case "c":
					m.enterCorkboard() // full-screen corkboard (the spread)
				case "m":
					m.enterManuscript() // read-through pager
				}
			}
		}
	} else {
		if km, ok := msg.(tea.KeyMsg); ok && km.Type == tea.KeyEnter && m.editor.AtLineEnd() {
			if prefix, clear, ok := listContinuation(m.editor.CurrentLine()); ok {
				if clear {
					m.editor.ClearLine()
				} else {
					m.editor.InsertString("\n" + prefix)
				}
				m.dirty = true
				m.lastEditAt = time.Now()
				m.invalidateAppleFindings() // buffer mutated — offsets/line indices shift
				return m, nil
			}
		}
		if km, ok := msg.(tea.KeyMsg); ok && m.smartQuotes &&
			km.Type == tea.KeyRunes && len(km.Runes) == 1 &&
			(km.Runes[0] == '\'' || km.Runes[0] == '"') {
			prev, hasPrev := m.editor.CharBeforeCursor()
			m.editor.InsertString(smartQuote(prev, hasPrev, km.Runes[0]))
			m.dirty = true
			m.lastEditAt = time.Now()
			m.invalidateAppleFindings()
			return m, nil
		}
		if km, ok := msg.(tea.KeyMsg); ok && m.smartQuotes &&
			km.Type == tea.KeyRunes && len(km.Runes) == 1 &&
			(km.Runes[0] == '!' || km.Runes[0] == '?' || km.Runes[0] == ';' || km.Runes[0] == ':') {
			sign := km.Runes[0]
			prev, hasPrev := m.editor.CharBeforeCursor()
			if insecable, ok := punctuationSpacing(prev, hasPrev, sign); ok {
				m.editor.ReplaceCharBeforeCursor(insecable)
			}
			if sign == '!' || sign == '?' {
				count := m.editor.RunCountBeforeCursor(sign)
				for i := 0; i < nupleInsert(count); i++ {
					m.editor.InsertRune(sign)
				}
			} else {
				m.editor.InsertRune(sign)
			}
			m.dirty = true
			m.lastEditAt = time.Now()
			m.invalidateAppleFindings()
			return m, nil
		}
		before := m.editor.Value()
		m.editor, cmd = m.editor.Update(msg)
		cmds = append(cmds, cmd)
		if m.editor.Value() != before {
			m.dirty = true
			m.lastEditAt = time.Now()
			m.invalidateAppleFindings() // offsets are stale once the text changes
		}
	}

	return m, tea.Batch(cmds...)
}

func (m model) View() string {
	if m.width == 0 {
		return "chargement…"
	}

	if m.migrationPending != nil {
		return migrationConfirmView(m.migrationPending, m.width)
	}

	// Help overlay renders over any screen (F1 / ? open it from anywhere).
	if m.showHelp {
		hH := m.height - 1
		if hH < 1 {
			hH = 1
		}
		card := framedPanel("Touches", helpText, 58, min(hH, lipgloss.Height(helpText)+2), "")
		body := lipgloss.Place(m.width, hH, lipgloss.Center, lipgloss.Center, card)
		return lipgloss.JoinVertical(lipgloss.Left, body, statusStyle.Width(m.width).Render("F1 · ? · esc  fermer"))
	}

	if m.screen == screenHome {
		return m.homeView()
	}

	if m.screen == screenManuscript {
		return m.pagerView()
	}

	if m.screen == screenSearch {
		return m.searchView()
	}

	if m.screen == screenMover {
		return m.moverView()
	}

	if m.screen == screenProperties {
		return m.propertiesView()
	}

	if m.screen == screenSnapshots {
		return m.snapshotsView()
	}

	if m.screen == screenDiff {
		return m.diffView()
	}

	if m.screen == screenHeatmap {
		return m.heatmapView()
	}

	if m.screen == screenCorkboard {
		return m.corkboardView()
	}

	if m.screen == screenNotes {
		return m.notesView()
	}

	if m.screen == screenAllNotes {
		return allNotesView(m)
	}

	if m.screen == screenOutline {
		return m.outlineView()
	}

	if m.screen == screenTextPicker {
		return textPickerView(m.textPickerChapter, m.textPickerSel, m.textPickerDir, m.files.wc, m.width, m.height)
	}

	bodyH := m.height - 1 // status only; no banner in the writing zone
	if bodyH < 1 {
		bodyH = 1
	}

	// Refresh the decorator so the grammar "don't nag the line you're typing" rule
	// tracks the LIVE cursor line. applyDecorator captures the cursor line by value
	// and only re-runs on toggle/load, so without this the suppression would freeze
	// to whatever line was current when Grammar was switched on. Cheap (a closure
	// rebuild); only when grammar is active.
	if m.analysis.grammar {
		m.applyDecorator()
	}

	// The writing pane shows either the live editor or the rendered preview.
	pane := m.editor.View()
	if m.previewing {
		name := filepath.Base(m.currentFile)
		if m.currentFile == "" {
			name = "sans titre"
		}
		style := "Standard"
		if m.previewTufte {
			style = "Tufte"
		}
		header := breadcrumbStyle.Render("▌ APERÇU · "+name) + lipgloss.NewStyle().Foreground(subtle).Render("  · "+style+" (t)")
		pane = lipgloss.JoinVertical(lipgloss.Left, header, m.preview.View())
	}

	showSidebar, showInspector, editorArea := m.effectivePanels()

	cols := []string{}
	if showSidebar {
		title := m.files.paneLabel()
		editRow, editField := -1, ""
		if m.renaming && m.renamingInPane {
			editRow, editField = m.files.selected, m.nameInput.View()
		} else if m.creatingFile && m.creatingInPane {
			editRow, editField = createRowSentinel, m.nameInput.View()
		}
		cols = append(cols, framedPanel(title, m.files.View(editRow, editField), sidebarWidth, m.height, "+"))
	}
	// Editor column: the editor pane, a blank line break, then the status bar —
	// all at the editor width, so the side panels render truly full height and the
	// status/spelling hint stay within the editor column.
	editorH := bodyH - 1 // leave one row for the blank line above the status
	if editorH < 1 {
		editorH = 1
	}
	editorPane := lipgloss.Place(editorArea, editorH, lipgloss.Center, lipgloss.Top, pane)
	statusRow := statusStyle.Width(editorArea).Render(m.statusBar())
	editorCol := lipgloss.JoinVertical(lipgloss.Left, editorPane, strings.Repeat(" ", editorArea), statusRow)
	cols = append(cols, editorCol)
	if showInspector {
		doc := computeDocStats(m.editor.Value())
		proj := computeProjStats(m.files.dir, m.files.view, m.files.wc)
		pg := m.goalsAll[m.files.dir].applyEnvDefaults()
		paceStr, _ := paceLine(pg, proj.words, today())
		gs := goalStats{today: todayWords(pg, proj.words), dailyGoal: pg.DailyGoal, project: proj.words, projectGoal: pg.ProjectGoal,
			sessionSecs: m.activeSecs, sessionGoalMin: pg.SessionGoalMin,
			todayActiveSecs: pg.ActiveSecsToday, idle: !isWritingActive(m.now, m.lastEditAt, activeIdle),
			pace: paceStr, streakDays: streak(pg.History, today()), spark: recentHistory(pg.History, today(), 14)}
		m.inspector.grammarBackend = ""
		if m.grammarChecker != nil {
			m.inspector.grammarBackend = m.grammarChecker.Name()
		}
		m.inspector.grammarChecking = m.checkingGrammar
		m.inspector.grammarAutoRecheck = m.autoRecheck
		notes := loadNotes(m.currentFile)
		insInner := m.inspector.View(inspectorInnerWidth(), doc, proj, readOutlineDoc(m.files.dir), gs, m.analysis, notes)
		title := inspectorTabLabels()[m.inspector.tab]
		cols = append(cols, framedPanel(title, insInner, inspectorWidth, m.height, ""))
	}
	body := lipgloss.JoinHorizontal(lipgloss.Top, cols...)
	if m.exportChooser != nil {
		panel := exportChooserView(*m.exportChooser, m.width)
		overlay := lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, panel)
		return overlay
	}
	return body
}

// migrationConfirmView renders the v1→v2 migration confirm screen: the list of
// file→folder moves about to happen, and the two available actions.
func migrationConfirmView(plan *migrationStep, width int) string {
	var b strings.Builder
	b.WriteString("Ce manuscrit utilise l'ancien format de chapitres.\n\n")
	fmt.Fprintf(&b, "  %d chapitres seront convertis en dossiers :\n", len(plan.moves))
	for _, mv := range plan.moves {
		fmt.Fprintf(&b, "    %s → %s/%s\n", mv.fromFile, mv.toFolder, mv.toFile)
	}
	b.WriteString("\n  Chaque chapitre gagnera un titre par défaut si nécessaire\n")
	b.WriteString("  (« Chapitre un », « Chapitre deux », …) — les titres déjà\n")
	b.WriteString("  personnalisés dans le manifest sont conservés tels quels.\n\n")
	b.WriteString("  [Entrée] convertir maintenant   [Échap] annuler et fermer")
	return b.String()
}

// effectivePanels resolves which side panels are shown this render and the
// width left for the editor. When the inspector is open and showing both panels
// would squeeze the editor below minEditorMeasure, the sidebar is suppressed for
// this render (m.sidebarVisible is not mutated).
func (m model) effectivePanels() (showSidebar, showInspector bool, editorArea int) {
	showSidebar = m.sidebarVisible
	showInspector = m.inspector.visible
	if showInspector && showSidebar && m.width-sidebarWidth-inspectorWidth < minEditorMeasure {
		showSidebar = false
	}
	editorArea = m.width
	if showSidebar {
		editorArea -= sidebarWidth
	}
	if showInspector {
		editorArea -= inspectorWidth
	}
	return
}

// layout recomputes pane sizes whenever the window resizes or the sidebar
// toggles. The editor is always clamped to colWidth; the centering happens
// in View via lipgloss.Place.
func (m *model) layout() {
	if m.width == 0 || m.height == 0 {
		return
	}
	bodyH := m.height - 1 // no banner in the writing zone
	if bodyH < 1 {
		bodyH = 1
	}

	showSidebar, _, editorArea := m.effectivePanels()
	cw := min(m.colWidth, editorArea-2)
	if showSidebar {
		m.files.height = m.height - 2 // full-height panel content (m.height minus top+bottom border)
		m.files.width = sidebarWidth - 4
	}
	editorH := bodyH - 1 // editor column reserves a blank line + the status row
	if editorH < 1 {
		editorH = 1
	}
	m.previewAvail = editorArea - 2
	if m.previewAvail < 0 {
		m.previewAvail = 0
	}
	m.editor.SetWidth(cw)
	m.editor.SetHeight(editorH)
	m.preview.Width = cw
	m.preview.Height = editorH - 1 // reserve one row for the PREVIEW header
}

// updateManuscript handles input on the pager: scroll, jump-to-edit, and exits.
func (m model) updateManuscript(msg tea.Msg) (tea.Model, tea.Cmd) {
	if sz, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = sz.Width
		m.height = sz.Height
		w := pagerWidth(m.colWidth, sz.Width)
		m.pager.width = w
		m.pager.height = sz.Height - 1 - pagerHeaderHeight
		if m.pager.height < 1 {
			m.pager.height = 1
		}
		m.pager.load(m.pager.dir, w) // re-wrap the line→source map to the new width
		m.layout()
		return m, nil
	}
	if mouse, ok := msg.(tea.MouseMsg); ok {
		if mouse.Button != tea.MouseButtonLeft || mouse.Action != tea.MouseActionPress {
			return m, nil
		}
		row := sidebarRow(mouse.Y, pagerHeaderHeight, m.pager.height)
		if row < 0 {
			return m, nil
		}
		line := m.pager.offset + row
		if line >= len(m.pager.lines) {
			return m, nil
		}
		m.pager.cursor = line
		now := time.Now()
		if line == m.lastClickRow && now.Sub(m.lastClickTime) < 400*time.Millisecond {
			if file, src, ok := m.pager.jumpTarget(); ok {
				m.loadFile(filepath.Join(m.pager.dir, file))
				m.editor.MoveToLine(src)
				m.screen = screenWriting
				m.focus = focusEditor
				m.editor.Focus()
			}
			m.lastClickTime = time.Time{}
		} else {
			m.lastClickRow = line
			m.lastClickTime = now
		}
		return m, nil
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		m.pager.moveCursor(-1)
	case "down", "j":
		m.pager.moveCursor(1)
	case "pgup":
		m.pager.page(-1)
	case "pgdown":
		m.pager.page(1)
	case "enter":
		if file, src, ok := m.pager.jumpTarget(); ok {
			m.loadFile(filepath.Join(m.pager.dir, file))
			m.editor.MoveToLine(src)
			m.screen = screenWriting
			m.focus = focusEditor
			m.editor.Focus()
		}
	case "esc":
		m.screen = screenWriting
		m.focus = focusEditor
		m.editor.Focus()
	}
	return m, nil
}

// pagerView renders the pager screen with the status bar.
func (m model) pagerView() string {
	body := m.pager.View()
	status := statusStyle.Width(m.width).Render(m.statusBar())
	return lipgloss.JoinVertical(lipgloss.Left, body, status)
}

// enterManuscript builds the read-through pager for the current outline's
// manuscript and shows it. Reached from the outline's `m`.
func (m *model) enterManuscript() {
	w := pagerWidth(m.colWidth, m.width)
	m.pager.width = w
	m.pager.height = m.height - 1 - pagerHeaderHeight // status row + header
	if m.pager.height < 1 {
		m.pager.height = 1
	}
	m.pager.load(m.files.dir, w)
	m.lastClickTime = time.Time{} // don't carry a stale double-click in from another screen
	m.screen = screenManuscript
	m.status = "manuscrit · ↑↓ défiler · entrée éditer ici · esc éditeur · F1 aide"
}

// pagerWidth is the pager's measure: the configured column width, never wider than
// the terminal (mirrors the editor's clamp), floored at 1.
func pagerWidth(colWidth, termWidth int) int {
	w := min(colWidth, termWidth-2)
	if w < 1 {
		w = 1
	}
	return w
}

func (m *model) loadFile(path string) {
	if m.dirty && m.currentFile != "" && m.currentFile != path {
		m.save() // flush the outgoing buffer before clobbering it
		if m.dirty {
			// The flush failed (I/O error) — save() left dirty=true. Don't clobber the
			// unsaved buffer; stay on the current file and surface the error.
			m.status = "échec de l'enregistrement — reste sur " + filepath.Base(m.currentFile)
			return
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		m.status = "impossible d'ouvrir : " + filepath.Base(path)
		return
	}
	m.editor.SetValue(string(data))
	m.undoStack = []string{string(data)} // reset the undo ring per opened file, seeded with its content
	if ln := recentLine(recentPath(), path); ln > 0 {
		m.editor.MoveToLine(ln) // resume where you left off
	}
	m.currentFile = path
	if m.loadedMtime == nil {
		m.loadedMtime = map[string]time.Time{}
	}
	if fi, err := os.Stat(path); err == nil {
		m.loadedMtime[path] = fi.ModTime()
	}
	m.sessionBaseline = wordCount(string(data))
	m.previewing = false
	if utf8.Valid(data) {
		m.status = "ouvert " + filepath.Base(path)
	} else {
		// A non-UTF-8 file (e.g. Latin-1) decodes with replacement runes; saving would re-encode
		// it as UTF-8 and lose the original bytes. Warn, and leave the buffer clean so nothing is
		// rewritten until the writer makes an intentional edit.
		m.status = "⚠ " + filepath.Base(path) + " n'est pas un UTF-8 valide — modifier puis enregistrer le ré-encodera"
	}
	addRecent(recentPath(), path, m.editor.Line())
	m.dirty = false
	m.applyDecorator()
}

// confirmCreate turns the typed name into a new file or folder in the current
// pane dir. A trailing "/" (or an explicit New-project) makes a folder; an
// explicit New-project then enters it, while the sidebar "name/" convention
// creates-and-stays. Files default to .md and open a blank buffer.
// createChapter makes a new blank chapter — a folder holding one text file — at the
// manuscript root and appends it to the manifest as a bare chapter (read-modify-write,
// atomic), then opens it. name is the chapter's display title (also slugified into its
// birth-stable folder name).
func (m *model) createChapter(name string) {
	if strings.Contains(name, "/") {
		m.status = "un nom de chapitre ne peut pas contenir de séparateur de chemin"
		return
	}
	title := name
	folder := slugify(title)
	file := folder + ".md"
	chDir := filepath.Join(m.files.dir, folder)
	if _, err := os.Stat(chDir); err == nil {
		m.status = "un chapitre nommé " + folder + " existe déjà"
		return
	}
	if err := os.MkdirAll(chDir, 0o755); err != nil {
		m.status = "impossible de créer le chapitre : " + err.Error()
		return
	}
	dst := filepath.Join(chDir, file)
	if err := atomicWrite(dst, []byte(""), 0o644); err != nil {
		m.status = "impossible de créer le chapitre : " + err.Error()
		return
	}
	if mani, present, err := readManifest(m.files.dir); err == nil && present {
		mani.Items = append(mani.Items, manifestItem{Chapter: &manifestChapter{
			Folder: folder,
			Title:  title,
			Texts:  []manifestText{{File: file, Title: title}},
		}})
		if werr := writeManifest(m.files.dir, mani); werr != nil {
			m.status = "chapitre créé mais échec de la mise à jour du manifeste : " + werr.Error()
		}
	}
	m.files.SetDir(m.files.dir)
	m.loadFile(dst)
	m.focus = focusEditor
	m.editor.Focus()
	m.status = "nouveau chapitre " + title
}

// createResource makes an unlisted resource doc — loose at the manuscript root, or into a subfolder
// via a single "Folder/name" level (the folder is created if new). Never added to the manifest.
func (m *model) createResource(name string) {
	sub := ""
	if i := strings.IndexByte(name, '/'); i >= 0 {
		sub, name = name[:i], name[i+1:]
	}
	if strings.Contains(name, "/") || name == "" {
		m.status = "un nom de ressource ne peut pas être vide ni contenir '/' (utilisez Dossier/nom pour le classer dans un sous-dossier)"
		return
	}
	if filepath.Ext(name) == "" {
		name += ".md"
	}
	dir := m.files.dir
	if sub != "" {
		dir = filepath.Join(dir, sub)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			m.status = "impossible de créer le dossier : " + err.Error()
			return
		}
	}
	dst := filepath.Join(dir, name)
	if _, err := os.Stat(dst); err == nil {
		m.status = "un fichier nommé " + name + " existe déjà"
		return
	}
	if err := atomicWrite(dst, []byte(""), 0o644); err != nil {
		m.status = "impossible de créer la ressource : " + err.Error()
		return
	}
	m.files.SetDir(m.files.dir)
	m.files.selectName(name)
	m.focus = focusSidebar
	m.editor.Blur()
	m.status = "nouvelle ressource " + name
}

// createScene adds a new blank scene to an existing chapter — appends a manifestText to its
// Texts and opens the new file. folder identifies the target chapter (birth-stable, resolved
// by the caller at the ctrl+n picker or from the open text-picker screen); name is the
// scene's display title, slugified into its birth-stable filename.
func (m *model) createScene(folder, name string) {
	if strings.Contains(name, "/") {
		m.status = "un nom de scène ne peut pas contenir de séparateur de chemin"
		return
	}
	mani, present, err := readManifest(m.files.dir)
	if err != nil || !present {
		m.status = "le manifeste est introuvable ou illisible"
		return
	}
	ch := findChapterByFolder(&mani, folder)
	if ch == nil {
		m.status = "le chapitre est introuvable dans le manifeste"
		return
	}
	title := name
	file := slugify(title) + ".md"
	chDir := filepath.Join(m.files.dir, folder)
	dst := filepath.Join(chDir, file)
	if _, err := os.Stat(dst); err == nil {
		m.status = "une scène nommée " + file + " existe déjà dans ce chapitre"
		return
	}
	if err := atomicWrite(dst, []byte(""), 0o644); err != nil {
		m.status = "impossible de créer la scène : " + err.Error()
		return
	}
	ch.Texts = append(ch.Texts, manifestText{File: file, Title: title})
	if werr := writeManifest(m.files.dir, mani); werr != nil {
		m.status = "scène créée mais échec de la mise à jour du manifeste : " + werr.Error()
		return
	}
	m.files.SetDir(m.files.dir)
	m.loadFile(dst)
	m.focus = focusEditor
	m.editor.Focus()
	m.status = "nouvelle scène " + title
}

// createStandaloneScene creates a new blank scene at the manuscript root — not inside any
// chapter — and appends it as a new manifestItem (Scene: true) at the end of items[]. name is
// the scene's display title, slugified into its birth-stable root-level filename.
func (m *model) createStandaloneScene(name string) {
	if strings.Contains(name, "/") {
		m.status = "un nom de scène ne peut pas contenir de séparateur de chemin"
		return
	}
	title := name
	file := slugify(title) + ".md"
	dst := filepath.Join(m.files.dir, file)
	if _, err := os.Stat(dst); err == nil {
		m.status = "un fichier nommé " + file + " existe déjà dans ce manuscrit"
		return
	}
	if err := atomicWrite(dst, []byte(""), 0o644); err != nil {
		m.status = "impossible de créer la scène : " + err.Error()
		return
	}
	mani, present, err := readManifest(m.files.dir)
	if err != nil || !present {
		m.status = "scène créée mais le manifeste est introuvable ou illisible"
		return
	}
	mani.Items = append(mani.Items, manifestItem{Chapter: &manifestChapter{
		Title: title,
		Scene: true,
		Texts: []manifestText{{File: file, Title: title}},
	}})
	if werr := writeManifest(m.files.dir, mani); werr != nil {
		m.status = "scène créée mais échec de la mise à jour du manifeste : " + werr.Error()
		return
	}
	m.files.SetDir(m.files.dir)
	m.loadFile(dst)
	m.focus = focusEditor
	m.editor.Focus()
	m.status = "nouvelle scène " + title
}

func (m *model) confirmCreate() {
	name := strings.TrimSpace(m.nameInput.Value())
	explicitFolder := m.creatingFolder
	kind := m.createKind
	chapterFolder := m.createChapterFolder
	m.createKind = 0
	m.createChapterFolder = ""
	m.creatingFile = false
	m.creatingInPane = false
	m.creatingFolder = false
	m.nameInput.Blur()
	if name == "" {
		m.status = "création annulée (aucun nom)"
		return
	}

	// Manuscript chapter/resource creation (from the ctrl+n picker).
	if kind == 1 {
		m.createChapter(name)
		return
	}
	if kind == 2 {
		m.createResource(name)
		return
	}
	if kind == 3 {
		if chapterFolder != "" {
			m.createScene(chapterFolder, name) // scene added to an existing chapter's Texts
		} else {
			m.createStandaloneScene(name) // standalone scene: new top-level item
		}
		return
	}

	folder := explicitFolder || strings.HasSuffix(name, "/")
	name = strings.TrimSuffix(name, "/")

	if strings.Contains(name, "/") || name == "." || name == ".." {
		m.status = "le nom ne peut pas contenir de séparateur de chemin"
		return
	}

	if name == manifestName {
		m.status = "manifest.json ne peut être ni renommé ni supprimé — c'est ainsi qu'okashi suit l'ordre des chapitres"
		return
	}

	if folder {
		dir := filepath.Join(m.files.dir, name)
		if explicitFolder {
			// New Project → a real manuscript (folder + manifest + first chapter you land in).
			first, err := createManuscript(dir, name, "Pas encore de titre")
			if err != nil {
				m.status = "impossible de créer le projet : " + err.Error()
				return
			}
			m.files.SetDir(dir)
			m.loadFile(filepath.Join(dir, first))
			m.focus = focusEditor
			m.editor.Focus()
			m.status = "nouveau projet " + name + " — commencez à écrire"
			return
		}
		// "name/" convention → a plain category folder; refresh and stay.
		if err := os.MkdirAll(dir, 0o755); err != nil {
			m.status = "impossible de créer le dossier : " + err.Error()
			return
		}
		m.files.SetDir(m.files.dir)
		m.files.selectName(name)
		m.status = "dossier créé " + name
		m.focus = focusSidebar
		m.editor.Blur()
		return
	}

	if filepath.Ext(name) == "" {
		name += ".md"
	}
	m.currentFile = filepath.Join(m.files.dir, name)
	m.editor.SetValue("")
	m.sessionBaseline = 0
	m.dirty = false
	m.focus = focusEditor
	m.editor.Focus()
	m.status = "nouveau fichier : " + name + " — ctrl+s pour enregistrer"
}

// beginRename opens the rename prompt for t, pre-filled with prefill.
func (m *model) beginRename(t renameTarget, prefill string) {
	m.renameTarget = t
	m.renaming = true
	m.creatingFile = false
	m.nameInput.SetValue(prefill)
	m.nameInput.CursorEnd()
	m.nameInput.Focus()
	m.editor.Blur()
	m.status = ""
}

// startInPaneCreate begins an in-place new-document prompt in the sidebar.
func (m *model) startInPaneCreate() {
	m.previewing = false
	m.sidebarVisible = true // the field renders in the file pane — make sure it shows
	m.creatingFile = true
	m.creatingInPane = true
	m.creatingFolder = false
	m.nameInput.SetValue("")
	m.nameInput.Width = m.files.width
	m.nameInput.Focus()
	m.editor.Blur()
	m.focus = focusSidebar
	m.status = ""
}

// startRename begins renaming the selected sidebar entry (skips the ".." row).
func (m *model) startRename() {
	if len(m.files.entries) == 0 {
		return
	}
	m.sidebarVisible = true // the field renders in the file pane — make sure it shows
	e := m.files.entries[m.files.selected]
	if e.name == ".." {
		return
	}
	v := m.files.view
	if v.source == sourceManifest && v.warning != "" {
		m.status = "manifeste illisible — structure en lecture seule (manifeste externe)"
		return
	}
	if e.isScene {
		// standalone scene (manifest v3): retitle the manifest entry; filename is
		// birth-stable, same principle as a chapter but keyed by filename (no folder).
		m.renamingInPane = true
		m.nameInput.Width = m.files.width
		m.beginRename(renameTarget{dir: m.files.dir, name: e.name, standaloneScene: true},
			m.files.chapterTitle(e.name))
		return
	}
	if m.files.isChapterEntry(e) {
		if v.source == sourceManifest {
			// manifest manuscript: retitle the manifest entry; filename is birth-stable (§5.7).
			m.renamingInPane = true
			m.nameInput.Width = m.files.width
			m.beginRename(renameTarget{dir: m.files.dir, name: e.name, manifestChapter: true},
				m.files.chapterTitle(e.name))
			return
		}
		// legacy (manifest-less) folder: retain pre-manifest prefix-preserving retitle (O1).
		m.renamingInPane = true
		m.nameInput.Width = m.files.width
		m.beginRename(renameTarget{dir: m.files.dir, name: e.name, isDir: e.isDir, section: true},
			sectionTitle(e.name))
		return
	}
	m.renamingInPane = true
	m.nameInput.Width = m.files.width
	m.beginRename(renameTarget{dir: m.files.dir, name: e.name, isDir: e.isDir}, e.name)
}

// startDelete begins a delete confirmation for the selected sidebar entry.
// Silently skips ".." and manifest.json; blocks manifest chapters.
func (m *model) startDelete() {
	if len(m.files.entries) == 0 {
		return
	}
	e := m.files.entries[m.files.selected]
	if e.name == ".." {
		return
	}
	if e.name == manifestName {
		m.status = "manifest.json ne peut être ni renommé ni supprimé — c'est ainsi qu'okashi suit l'ordre des chapitres"
		return
	}
	v := m.files.view
	if m.files.isChapterEntry(e) && v.source == sourceManifest {
		m.status = "les fichiers de chapitre sont en lecture seule (manifeste externe)"
		return
	}
	m.deleting = true
	m.deleteTarget = e.name
	m.status = "supprimer « " + e.name + " » ? [y]es · esc annuler"
}

// confirmDelete removes the deleteTarget file or folder and refreshes the sidebar.
func (m *model) confirmDelete() {
	idx := m.files.selected // keep the position so the selection lands on the adjacent row
	path := filepath.Join(m.files.dir, m.deleteTarget)
	info, err := os.Lstat(path)
	if err == nil {
		if info.IsDir() {
			err = os.RemoveAll(path)
		} else {
			err = os.Remove(path)
		}
	}
	m.deleting = false
	m.deleteTarget = ""
	if err != nil {
		m.status = "impossible de supprimer : " + err.Error()
		return
	}
	if info != nil && !info.IsDir() {
		deleteNotes(path) // drop the deleted file's notes sidecar
	}
	m.files.SetDir(m.files.dir) // re-reads entries and resets selection to 0
	if n := len(m.files.entries); n > 0 {
		if idx >= n {
			idx = n - 1
		}
		m.files.selectRow(idx) // the row the deleted item occupied (now its neighbor)
	}
	m.status = "supprimé"
}

// duplicateSelected copies the selected file to a free "name copy.ext" (then
// "name copy 2.ext", …) and selects the new file. Dirs and ".." are skipped.
func (m *model) duplicateSelected() {
	if len(m.files.entries) == 0 {
		return
	}
	e := m.files.entries[m.files.selected]
	if e.name == ".." || e.isDir {
		m.status = "duplication : fichiers uniquement"
		return
	}
	ext := filepath.Ext(e.name)
	stem := strings.TrimSuffix(e.name, ext)
	target := copyFreeName(m.files.dir, stem, ext)
	data, err := os.ReadFile(filepath.Join(m.files.dir, e.name))
	if err != nil {
		m.status = "échec de la duplication : " + err.Error()
		return
	}
	if err := atomicWrite(filepath.Join(m.files.dir, target), data, 0o644); err != nil {
		m.status = "échec de la duplication : " + err.Error()
		return
	}
	m.files.SetDir(m.files.dir)
	m.files.selectName(target)
	m.status = "dupliqué → " + target
}

// copyFreeName returns "stem copy.ext", then "stem copy 2.ext", … that doesn't exist in dir.
func copyFreeName(dir, stem, ext string) string {
	name := stem + " copy" + ext
	if _, err := os.Stat(filepath.Join(dir, name)); os.IsNotExist(err) {
		return name
	}
	for i := 2; ; i++ {
		name = fmt.Sprintf("%s copy %d%s", stem, i, ext)
		if _, err := os.Stat(filepath.Join(dir, name)); os.IsNotExist(err) {
			return name
		}
	}
}

// confirmRename applies the pending rename: builds the new name by target kind,
// refuses a collision, renames on disk, follows the open file, and refreshes.
func (m *model) confirmRename() {
	m.renaming = false
	m.renamingInPane = false
	m.nameInput.Blur()
	typed := strings.TrimSpace(m.nameInput.Value())
	t := m.renameTarget
	if typed == "" {
		m.status = "renommage annulé (vide)"
		m.refreshAfterRename()
		return
	}

	if t.manifestChapter {
		if err := renameChapterTitle(t.dir, t.name, typed); err != nil {
			m.status = "échec du retitrage : " + err.Error()
		} else {
			m.status = "retitré en " + typed
		}
		m.refreshAfterRename()
		return
	}

	if t.standaloneScene {
		if err := renameSceneTitle(t.dir, t.name, typed); err != nil {
			m.status = "échec du retitrage : " + err.Error()
		} else {
			m.status = "retitré en " + typed
		}
		m.refreshAfterRename()
		return
	}

	var newName string
	if t.section {
		newName = sectionRetitle(t.name, typed)
	} else {
		if strings.Contains(typed, "/") || typed == "." || typed == ".." {
			m.status = "le nom ne peut pas contenir de séparateur de chemin"
			m.refreshAfterRename()
			return
		}
		if t.isDir {
			newName = typed
		} else {
			newName = looseRename(t.name, typed)
		}
	}
	if newName == manifestName {
		m.status = "manifest.json ne peut être ni renommé ni supprimé — c'est ainsi qu'okashi suit l'ordre des chapitres"
		m.refreshAfterRename()
		return
	}

	if newName == t.name {
		m.status = "inchangé"
		m.refreshAfterRename()
		return
	}

	oldPath := filepath.Join(t.dir, t.name)
	newPath := filepath.Join(t.dir, newName)
	if _, err := os.Stat(newPath); err == nil {
		m.status = "un fichier nommé " + newName + " existe déjà"
		m.refreshAfterRename()
		return
	}
	if err := os.Rename(oldPath, newPath); err != nil {
		m.status = "échec du renommage : " + err.Error()
		m.refreshAfterRename()
		return
	}
	if !t.isDir {
		// The notes sidecar follows a FILE rename. Not for a dir: a dir's children's notes travel
		// inside it via os.Rename, and firing moveNotes on a dir could steal a same-stem loose
		// file's notes (folder `interlude/` vs file `interlude.md`).
		moveNotes(oldPath, newPath)
	}
	if m.currentFile == oldPath {
		m.currentFile = newPath
	}
	m.refreshAfterRename()
	m.status = "renommé en " + newName
}

// refreshAfterRename re-reads the sidebar and restores focus to the file pane.
func (m *model) refreshAfterRename() {
	m.files.SetDir(m.files.dir)
	m.focus = focusSidebar
	m.editor.Blur()
}

// togglePreview flips between editing and a read-only glamour render of the
// current buffer. The render is a snapshot taken on entry — you can't edit a
// rendered document, so it can't drift while preview is up.
func (m *model) togglePreview() {
	if m.previewing {
		m.previewing = false
		if m.focus == focusEditor || !m.sidebarVisible {
			m.editor.Focus()
		}
		m.status = "édition"
		return
	}

	m.renderPreview()
	m.preview.GotoTop()
	m.previewing = true
	m.editor.Blur()
	m.status = "aperçu (lecture seule) · ctrl+p éditer · t style · ↑/↓ défiler"
}

// NOTE: the sidenote helpers below (sidenoteGeometry, sidenotePlan, and in preview.go
// footnotesToSidenotes / layoutSidenotes) are DORMANT — the Tufte preview now renders footnotes
// as bottom endnotes. They are retained (with tests) to seed the planned margin/revision-notes
// feature, which will reuse the right-margin gutter for author annotations. Not currently wired.
const (
	sidenoteMinGutter = 18
	sidenoteMaxGutter = 30
)

// sidenoteGeometry reports the gutter width for a preview pane of `avail` columns holding a body
// of `measure` columns plus the 3-col " ┆ " gap, and whether a margin (>= sidenoteMinGutter) fits.
func sidenoteGeometry(avail, measure int) (gutter int, ok bool) {
	gutter = avail - measure - 3
	if gutter < sidenoteMinGutter {
		return 0, false
	}
	if gutter > sidenoteMaxGutter {
		gutter = sidenoteMaxGutter
	}
	return gutter, true
}

// sidenotePlan decides whether the Tufte preview should render margin sidenotes: the body measure
// stays the writing measure (colWidth), the gutter uses the pane's spare width, and it engages only
// when the pane is wide enough AND the doc has referenced footnotes. body/notes come from
// footnotesToSidenotes (body has superscript refs, no endnote section).
func sidenotePlan(avail, colWidth int, buffer string) (measure, gutter int, body string, notes []string, ok bool) {
	measure = colWidth
	if avail > 0 && avail < measure {
		measure = avail // tiny panes: never exceed the available width
	}
	g, gok := sidenoteGeometry(avail, measure)
	if !gok {
		return 0, 0, "", nil, false
	}
	b, ns := footnotesToSidenotes(buffer)
	if len(ns) == 0 {
		return 0, 0, "", nil, false
	}
	return measure, g, b, ns, true
}

// renderPreview rebuilds the preview viewport from the buffer: footnotes folded to endnotes
// (glamour can't render them), styled Default (the detected theme) or Tufte.
func (m *model) renderPreview() {
	wrap := min(m.colWidth, m.previewAvail)
	if wrap <= 0 {
		wrap = m.colWidth // pre-layout (previewAvail not set yet)
	}
	m.preview.Width = wrap
	styleOpt := glamour.WithStandardStyle(m.mdStyle)
	if m.previewTufte {
		styleOpt = glamour.WithStyles(tufteGlamourStyle(m.mdStyle == "dark"))
	}
	r, err := glamour.NewTermRenderer(styleOpt, glamour.WithWordWrap(wrap))
	if err != nil {
		m.status = "aperçu indisponible : " + err.Error()
		return
	}
	out, err := r.Render(footnotesToEndnotes(m.editor.Value()))
	if err != nil {
		m.status = "échec de l'aperçu : " + err.Error()
		return
	}
	m.preview.SetContent(out)
}

// wordCount counts whitespace-separated words.
func wordCount(s string) int {
	return len(strings.Fields(s))
}

// commafy formats an integer with thousands separators: 1240 -> "1,240".
func commafy(n int) string {
	s := strconv.Itoa(n)
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteByte(s[i])
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}

// signedComma formats a signed delta with an explicit sign: 142 -> "+142".
func signedComma(n int) string {
	if n < 0 {
		return "-" + commafy(-n)
	}
	return "+" + commafy(n)
}

// fmtDuration renders a duration as M:SS under an hour, H:MM:SS at or above. Negative clamps to 0.
func fmtDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	s := int(d.Seconds())
	h, m, sec := s/3600, (s%3600)/60, s%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, sec)
	}
	return fmt.Sprintf("%d:%02d", m, sec)
}

// sprintDisplay renders a pomodoro countdown: 🍅 for work, ☕ for a break.
func sprintDisplay(remaining time.Duration, onBreak bool) string {
	icon := "🍅"
	if onBreak {
		icon = "☕"
	}
	return icon + " " + fmtDuration(remaining)
}

// advanceSprint transitions a sprint that has reached its end: work → a break of breakDur, then
// break → complete (inactive). Returns the new end, the new on-break flag, whether it's still
// active, and a status message for the transition.
func advanceSprint(now, end time.Time, onBreak bool, breakDur time.Duration) (time.Time, bool, bool, string) {
	if onBreak {
		return end, false, false, "✓ sprint complete"
	}
	return now.Add(breakDur), true, true, "🍅 done — break time"
}

// statsText is the live readout shown on the right of the status bar:
// total words in the buffer, plus net words added since this file was opened.
func (m model) statsText() string {
	words := wordCount(m.editor.Value())
	delta := words - m.sessionBaseline
	timeSeg := "⏱ " + fmtDuration(time.Duration(m.activeSecs)*time.Second)
	if !isWritingActive(m.now, m.lastEditAt, activeIdle) {
		timeSeg += " ⏸"
	}
	if m.sprintActive {
		remaining := m.sprintEnd.Sub(m.now)
		disp := sprintDisplay(remaining, m.sprintOnBreak)
		if remaining <= time.Minute {
			disp = lipgloss.NewStyle().Foreground(accent).Render(disp)
		}
		timeSeg = disp
	}
	return fmt.Sprintf("%s mots · %s session · %s", commafy(words), signedComma(delta), timeSeg)
}

// statusBar composes the bottom line: the status message on the left, the live
// word-count readout right-aligned. While naming a new file the prompt owns the
// whole bar; on a terminal too narrow for both, the stats drop out rather than
// truncate. Width is the bar minus statusStyle's 1-col padding each side.
func (m model) statusBar() string {
	// An in-pane rename/create renders in the file row — but only if the sidebar
	// is actually drawn this frame; otherwise fall back to the bottom-bar prompt
	// so the field is never invisible.
	showSidebar, _, _ := m.effectivePanels()
	if m.creatingFile && (!m.creatingInPane || !showSidebar) {
		folderMode := m.creatingFolder || strings.HasSuffix(m.nameInput.Value(), "/")
		label := "nouveau fichier ▸ "
		if folderMode {
			label = "nouveau dossier ▸ "
		}
		bar := label + m.nameInput.View()
		if folderMode {
			return bar
		}
		hint := lipgloss.NewStyle().Foreground(subtle).Render("terminez par / pour un dossier")
		gap := (m.width - 2) - lipgloss.Width(bar) - lipgloss.Width(hint)
		if gap < 1 {
			return bar
		}
		return bar + strings.Repeat(" ", gap) + hint
	}
	if m.suggesting {
		parts := make([]string, len(m.suggestions))
		for i, s := range m.suggestions {
			if i == m.suggestIndex {
				parts[i] = selectedStyle.Render(s)
			} else {
				parts[i] = s
			}
		}
		return "suggestion ▸ " + strings.Join(parts, " · ")
	}
	if w, sugg, ok := m.cursorSpellHint(); ok {
		return "✗ " + w + " → " + strings.Join(sugg, " · ") + "  ·  ^R"
	}
	if w, sugg, reason, ok := m.cursorGrammarHint(); ok {
		_, _, editorArea := m.effectivePanels()
		hint := "✗ " + w + " → " + strings.Join(sugg, " · ")
		if reason != "" {
			hint += " · " + reason
		}
		return ansi.Truncate(hint, max(10, editorArea-2), "…") // keep the fix visible; trim the reason
	}
	if m.renaming && (!m.renamingInPane || !showSidebar) {
		return "renommer ▸ " + m.nameInput.View()
	}
	if m.goalPromptField == 1 {
		return "objectif quotidien ▸ " + m.nameInput.View()
	}
	if m.goalPromptField == 2 {
		return "objectif du projet ▸ " + m.nameInput.View()
	}
	if m.goalPromptField == 3 {
		return "minutes quotidiennes ▸ " + m.nameInput.View()
	}
	if m.goalPromptField == 4 {
		return "échéance AAAA-MM-JJ (vide efface) ▸ " + m.nameInput.View()
	}
	if m.createPicker {
		return "nouveau : c chapitre · r ressource · s scène · esc annuler"
	}
	mark := "✓"
	if m.dirty {
		mark = "●"
	}
	stats := mark + " " + m.statsText()
	if m.selectMode {
		stats = lipgloss.NewStyle().Foreground(accent).Bold(true).Render("-- SELECT --") + "  " + stats
	}
	// Browsing the sidebar (not writing)? Surface its otherwise-invisible actions in place of an empty
	// status — these single-key features are only in F1 otherwise.
	status := m.status
	if status == "" && m.focus == focusSidebar {
		status = "c tableau · m lecture · b sauvegardes · F1 touches"
	}
	return m.composeStatus(status, stats)
}

// composeStatus lays the stats at the editor text's left edge and the status
// (last save/open) right-aligned to the text's right edge, within the editor
// text column. Stats win if both don't fit.
func (m model) composeStatus(status, stats string) string {
	_, _, editorArea := m.effectivePanels()
	cw := min(m.colWidth, editorArea-2)
	totalW := editorArea - 2 // status renders at editorArea width; statusStyle pads one col each side
	sw := lipgloss.Width(stats)
	if cw < sw+1 || totalW < sw {
		return stats // too narrow for the two-element layout
	}
	left := (editorArea-cw)/2 - 1 // content col of the text's left edge (within the editor column)
	if left < 0 {
		left = 0
	}
	if left+sw > totalW {
		left = totalW - sw
	}
	// status right-aligned to the text right edge (content col left+cw), truncated to fit.
	avail := cw - sw - 1
	st := status
	if lipgloss.Width(st) > avail {
		if avail <= 0 {
			return strings.Repeat(" ", left) + stats
		}
		st = ansi.Truncate(st, avail, "…")
	}
	stW := lipgloss.Width(st)
	statusStart := left + cw - stW
	used := left + sw
	if statusStart < used+1 {
		statusStart = used + 1
	}
	if statusStart+stW > totalW {
		statusStart = totalW - stW
	}
	if statusStart < used {
		return strings.Repeat(" ", left) + stats
	}
	return strings.Repeat(" ", left) + stats + strings.Repeat(" ", statusStart-used) + st
}

const backupKeep = 10 // timestamped snapshots retained per file

// snapshotBackup copies the current on-disk file into <dir>/.okashi-bak/<base>.<YYYYMMDD-HHMMSS>,
// then keeps only the newest backupKeep snapshots for that base. Best-effort — never blocks a save.
func snapshotBackup(path string) {
	data, err := os.ReadFile(path)
	if err != nil { // new/unreadable file → nothing to back up
		return
	}
	bakDir := filepath.Join(filepath.Dir(path), ".okashi-bak")
	if os.MkdirAll(bakDir, 0o755) != nil {
		return
	}
	base := filepath.Base(path)
	_ = atomicWrite(filepath.Join(bakDir, base+"."+time.Now().Format("20060102-150405")), data, 0o644)
	pruneBackups(bakDir, base, backupKeep)
}

// pruneBackups removes all but the newest `keep` snapshots named "<base>.*" in dir.
func pruneBackups(dir, base string, keep int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	// Match only "<base>.<15-char timestamp>" so a shorter base (a.md) can't thin a longer
	// base's ring (a.md.old) whose backups live in the same .okashi-bak/ dir.
	const stampLen = len("20060102-150405")
	var snaps []string
	for _, e := range entries {
		name := e.Name()
		if !e.IsDir() && strings.HasPrefix(name, base+".") && len(name) == len(base)+1+stampLen {
			snaps = append(snaps, name)
		}
	}
	if len(snaps) <= keep {
		return
	}
	sort.Strings(snaps) // timestamp suffix sorts chronologically; oldest first
	for _, name := range snaps[:len(snaps)-keep] {
		_ = os.Remove(filepath.Join(dir, name))
	}
}

func (m *model) save() {
	if m.currentFile == "" {
		m.status = "aucun fichier ouvert — choisissez-en un dans le panneau d'abord"
		return
	}
	// External-change guard: never overwrite a file that changed on disk since we loaded it.
	if fi, err := os.Stat(m.currentFile); err == nil {
		if loaded, ok := m.loadedMtime[m.currentFile]; ok && fi.ModTime().After(loaded) {
			ext := filepath.Ext(m.currentFile)
			confl := strings.TrimSuffix(m.currentFile, ext) + ".conflict-" + time.Now().Format("20060102-150405") + ext
			if werr := atomicWrite(confl, []byte(m.editor.Value()), 0o644); werr != nil {
				m.status = "échec de l'enregistrement (conflit) : " + werr.Error()
				return
			}
			m.status = "⚠ " + filepath.Base(m.currentFile) + " a changé sur le disque — vos modifications ont été enregistrées dans " + filepath.Base(confl)
			m.currentFile = confl
			if m.loadedMtime == nil {
				m.loadedMtime = map[string]time.Time{}
			}
			if cfi, err := os.Stat(confl); err == nil {
				m.loadedMtime[confl] = cfi.ModTime()
			}
			m.dirty = false
			addRecent(recentPath(), confl, m.editor.Line())
			return
		}
	}
	if m.backedUp == nil {
		m.backedUp = map[string]bool{}
	}
	if !m.backedUp[m.currentFile] {
		snapshotBackup(m.currentFile)
		m.backedUp[m.currentFile] = true
	}
	if err := atomicWrite(m.currentFile, []byte(m.editor.Value()), 0o644); err != nil {
		m.status = "échec de l'enregistrement : " + err.Error()
		return // dirty stays true → retried next tick
	}
	m.dirty = false
	addRecent(recentPath(), m.currentFile, m.editor.Line())
	m.status = "enregistré " + filepath.Base(m.currentFile)
	if m.loadedMtime == nil {
		m.loadedMtime = map[string]time.Time{}
	}
	if fi, err := os.Stat(m.currentFile); err == nil {
		m.loadedMtime[m.currentFile] = fi.ModTime()
	}

	// Surface a newly-created file in the sidebar if we're browsing its folder.
	// Re-saving an already-listed file leaves the selection undisturbed.
	if base := filepath.Base(m.currentFile); filepath.Dir(m.currentFile) == m.files.dir && !m.files.has(base) {
		m.files.SetDir(m.files.dir)
		m.files.selectName(base)
	}
}

// saveIfDirty flushes unsaved work before the app exits.
func (m *model) saveIfDirty() {
	if m.dirty && m.currentFile != "" {
		m.save()
	}
	if m.goalsAll != nil {
		saveGoals(goalsPath(), m.goalsAll)
	}
}

// resolveDirArg interprets a positional path argument (`okashi <dir>`). It returns (absDir, true, nil)
// for a valid existing directory (which should override OKASHI_DIR), ("", true, err) for a bad path
// (missing, or a file — single-file open is deferred), and ("", false, nil) when there is no
// positional path arg (a flag, or none). Flags/subcommands are handled by main's switch first.
func resolveDirArg(args []string) (string, bool, error) {
	if len(args) < 2 || strings.HasPrefix(args[1], "-") {
		return "", false, nil
	}
	a := args[1]
	info, err := os.Stat(a)
	if err != nil {
		return "", true, fmt.Errorf("no such file or directory: %s", a)
	}
	if !info.IsDir() {
		return "", true, fmt.Errorf("%s is a file — pass a folder (opening a single file isn't supported yet)", a)
	}
	abs, err := filepath.Abs(a)
	if err != nil {
		return "", true, err
	}
	return abs, true, nil
}

// killGrammalecteProc terminates a Grammalecte server subprocess that okashi itself launched.
// A nil proc (no auto-launch happened, or a pre-existing server was reused instead) is a no-op —
// okashi never touches a server it didn't start. SIGTERM is tried first; if the process hasn't
// exited within a second, Kill() (SIGKILL) forces it, so okashi never hangs on exit waiting for
// a misbehaving child.
func killGrammalecteProc(proc *exec.Cmd) {
	if proc == nil || proc.Process == nil {
		return
	}
	proc.Process.Signal(syscall.SIGTERM)
	done := make(chan struct{})
	go func() {
		proc.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		proc.Process.Kill()
	}
}

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--version", "-v", "version":
			fmt.Println("okashi " + version)
			return
		case "--help", "-h", "help":
			fmt.Println(usage)
			return
		}
		// A positional path opens okashi in that folder, overriding $OKASHI_DIR.
		if dir, isDirArg, err := resolveDirArg(os.Args); isDirArg {
			if err != nil {
				fmt.Fprintln(os.Stderr, "okashi: "+err.Error())
				os.Exit(1)
			}
			os.Setenv("OKASHI_DIR", dir)
		}
	}

	p := tea.NewProgram(initialModel(), tea.WithAltScreen(), tea.WithMouseCellMotion())
	finalModel, err := p.Run()
	if fm, ok := finalModel.(model); ok {
		killGrammalecteProc(fm.grammalecteProc)
	}
	if err != nil {
		os.Exit(1)
	}
}
