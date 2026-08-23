package main

import (
	"path/filepath"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"okashi/internal/textarea"
)

// corkKey returns the stable, collision-free identity of a card's chapterRef for the corkboard's
// per-card maps (m.synopses, m.corkFirstLines, and corkChapterSet's prune set). An ordinary
// chapter (or a Part's chapter) keys off its folder, exactly as before this helper existed — that
// key is ALSO the sidecar's on-disk key (synopsis.go's saveSynopses/loadSynopses), so it must stay
// bare, unprefixed, for backward-compatible persistence. A standalone scene has folder == "" BY
// CONSTRUCTION (the whole point of the feature — it lives at the manuscript root, no folder of its
// own), so two-or-more standalone scenes in the same manuscript would all collide on the same ""
// key. A scene is instead keyed by its birth-stable filename (ch.texts[0].file — the same identity
// findSceneByFile uses), namespaced with a "scene:" prefix so it can never collide with a real
// chapter folder (folders are slugs; "scene:" is not a valid slug prefix in practice, and even a
// pathological folder literally named "scene:foo" would only collide with a same-named scene file,
// not with an ordinary chapter). A scene with no texts (should not happen — the invariant is
// Folder=="" && len(Texts)==1 — but defend anyway) falls back to "scene:"+ch.title so it still gets
// a key distinct from "" instead of silently colliding.
func corkKey(ch chapterRef) string {
	if !ch.scene {
		return ch.folder
	}
	if len(ch.texts) > 0 {
		return "scene:" + ch.texts[0].file
	}
	return "scene:" + ch.title
}

// enterCorkboard opens the corkboard for the current manuscript: it loads the same staged buffer
// structure mode uses (so reorder + commit are shared) plus the synopsis sidecar.
//
// Staging is reduced to BARE chapters only for this plan (Parts are not yet editable from the
// corkboard/structure mode — full 3-level structure mode is the next plan's job): the resolved
// view's synthetic untitled part (v.parts[0] when its title == "") is what gets staged into
// m.structureItems []chapterRef. Any real Parts in the manifest are left untouched by
// commitStructure (see its comment) — this mode simply doesn't see them.
func (m *model) enterCorkboard() {
	m.save() // flush the current buffer first: opening it from the board hits loadFile's
	// currentFile==path branch, which reloads from disk and would clobber unsaved edits (mirrors ctrl+l)
	dir := m.files.dir
	_, present, err := readManifest(dir)
	if err != nil {
		m.status = "impossible d'ouvrir le tableau — le manifest.json de ce manuscrit est illisible (corrompu ou version plus récente)"
		return
	}
	if !present {
		m.status = "le tableau fonctionne uniquement dans un manuscrit (un projet avec chapitres ordonnés)"
		return
	}
	v := resolveManuscript(dir, readEntries(dir))
	var bare []chapterRef
	if len(v.parts) > 0 && v.parts[0].title == "" {
		bare = v.parts[0].chapters
	}
	m.structureDir = dir
	m.structureItems = append([]chapterRef{}, bare...)
	m.structureSel = 0
	m.structurePendingNew = map[string]bool{}
	m.structureDirty = false
	m.structureConfirm = false
	m.structureAdding = false   // defensive: never inherit a structure-mode sub-mode
	m.structureRenaming = false // (the two modes share the structure* fields)
	m.synopses = loadSynopses(dir)
	// Preload the first-line fallbacks ONCE (disk I/O) so corkboardView never reads files on the
	// render path — View() fires per keystroke and must stay I/O-free (and iCloud-safe).
	m.corkFirstLines = map[string]string{}
	for _, ch := range bare {
		if len(ch.texts) == 0 {
			continue
		}
		key := corkKey(ch)
		if m.synopses[key] == "" {
			m.corkFirstLines[key] = firstProseLine(filepath.Join(dir, ch.folder, ch.texts[0].file))
		}
	}
	m.synEditing = false
	m.screen = screenCorkboard
}

// corkboardCardMeta decides a card's open-marker and body source: an authored synopsis (not dim),
// a dimmed first-line fallback, or "" to signal the (no synopsis) placeholder.
func corkboardCardMeta(isCurrent bool, syn, firstLine string) (openMark, rawBody string, dim bool) {
	if isCurrent {
		openMark = lipgloss.NewStyle().Foreground(accent).Render("● ")
	}
	switch {
	case syn != "":
		return openMark, syn, false
	case firstLine != "":
		return openMark, firstLine, true
	default:
		return openMark, "", false
	}
}

// corkChapterSet is the ON-DISK chapter folder set — the safe prune target for an immediate
// synopsis write. It must NOT come from the staged m.structureItems: a staged x/a change
// (uncommitted, and possibly discarded) would otherwise prune a still-live chapter's synopsis
// off disk. Synopsis writes are committed independently of the structure commit, so they prune
// against committed reality. Keyed by corkKey (birth-stable chapter/scene identity), across all
// parts — a synopsis for a chapter inside a real Part must not be pruned either.
func (m model) corkChapterSet() map[string]bool {
	s := map[string]bool{}
	v := resolveManuscript(m.structureDir, readEntries(m.structureDir))
	for _, p := range v.parts {
		for _, ch := range p.chapters {
			s[corkKey(ch)] = true
		}
	}
	return s
}

func newSynopsisArea(val string) textarea.Model {
	ta := textarea.New()
	ta.Prompt = ""
	ta.ShowLineNumbers = false
	ta.CharLimit = 0
	ta.MaxHeight = 0
	ta.SetWidth(60)
	ta.SetHeight(3)
	ta.SetValue(val)
	return ta
}

func (m *model) startSynopsisEdit() {
	if m.structureSel < 0 || m.structureSel >= len(m.structureItems) {
		return
	}
	key := corkKey(m.structureItems[m.structureSel])
	m.synArea = newSynopsisArea(m.synopses[key])
	m.synArea.Focus()
	m.synEditing = true
}

// commitSynopsis stores the edited synopsis and writes the sidecar immediately (pruned), keeping
// synopsis persistence independent of the manifest reorder commit.
func (m *model) commitSynopsis() {
	m.synEditing = false
	m.synArea.Blur()
	if m.structureSel < 0 || m.structureSel >= len(m.structureItems) {
		return
	}
	ch := m.structureItems[m.structureSel]
	key := corkKey(ch)
	text := strings.TrimRight(m.synArea.Value(), "\n")
	if m.synopses == nil {
		m.synopses = map[string]string{}
	}
	if text == "" {
		delete(m.synopses, key)
		// Clearing a synopsis reveals the first-line fallback — populate it now (once, off the
		// render path) so the card updates in-session, not only on corkboard re-entry.
		if m.corkFirstLines == nil {
			m.corkFirstLines = map[string]string{}
		}
		if _, ok := m.corkFirstLines[key]; !ok && len(ch.texts) > 0 {
			m.corkFirstLines[key] = firstProseLine(filepath.Join(m.structureDir, ch.folder, ch.texts[0].file))
		}
	} else {
		m.synopses[key] = text
	}
	if err := saveSynopses(m.structureDir, m.synopses, m.corkChapterSet()); err != nil {
		m.status = "échec de l'enregistrement du synopsis : " + err.Error()
	} else {
		m.status = "synopsis enregistré"
	}
}

func (m model) updateCorkboard(msg tea.Msg) (tea.Model, tea.Cmd) {
	if sz, ok := msg.(tea.WindowSizeMsg); ok {
		m.width, m.height = sz.Width, sz.Height
		return m, nil
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	// Export prompt (ctrl+e from the corkboard → whole-manuscript export).
	if m.exportChooser != nil {
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
		return m, nil
	}

	if m.synEditing {
		if key.String() == "esc" { // esc commits (⏎ inserts the 1–3 line breaks)
			m.commitSynopsis()
			return m, nil
		}
		var cmd tea.Cmd
		m.synArea, cmd = m.synArea.Update(key)
		return m, cmd
	}

	// Retitle sub-mode.
	if m.structureRenaming {
		switch key.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			m.structureRenaming = false
			m.nameInput.Blur()
		case "enter":
			m.structureRenaming = false
			m.nameInput.Blur()
			if t := strings.TrimSpace(m.nameInput.Value()); t != "" && m.structureSel < len(m.structureItems) {
				m.structureItems[m.structureSel].title = t
				m.structureDirty = true
			}
		default:
			var cmd tea.Cmd
			m.nameInput, cmd = m.nameInput.Update(key)
			return m, cmd
		}
		return m, nil
	}

	// Add sub-mode (new blank chapter / promote a Resource).
	if m.structureAdding {
		choices := m.structureAddChoices()
		switch key.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			m.structureAdding = false
		case "up", "k":
			if m.structureAddSel > 0 {
				m.structureAddSel--
			}
		case "down", "j":
			if m.structureAddSel < len(choices)-1 {
				m.structureAddSel++
			}
		case "enter":
			if m.structureAddSel < len(choices) {
				m.applyAdd(choices[m.structureAddSel])
			}
			m.structureAdding = false
		}
		return m, nil
	}

	// Reorder/structure commit confirm.
	if m.structureConfirm {
		switch key.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "y":
			if err := m.commitStructure(); err != nil {
				m.status = "échec de l'enregistrement : " + err.Error()
				m.structureConfirm = false
				return m, nil
			}
			m.structureConfirm = false
			m.exitCorkboard()
			m.status = "modifications enregistrées"
		case "esc", "n":
			m.structureConfirm = false
			m.exitCorkboard()
			m.status = "modifications annulées"
		}
		return m, nil
	}

	switch key.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		if m.structureSel > 0 {
			m.structureSel--
		}
	case "down", "j":
		if m.structureSel < len(m.structureItems)-1 {
			m.structureSel++
		}
	case "J", "shift+down", "alt+down": // alt+↓ mirrors the outline's move-beat chord
		if m.structureSel < len(m.structureItems)-1 {
			i := m.structureSel
			m.structureItems[i], m.structureItems[i+1] = m.structureItems[i+1], m.structureItems[i]
			m.structureSel++
			m.structureDirty = true
		}
	case "K", "shift+up", "alt+up":
		if m.structureSel > 0 {
			i := m.structureSel
			m.structureItems[i], m.structureItems[i-1] = m.structureItems[i-1], m.structureItems[i]
			m.structureSel--
			m.structureDirty = true
		}
	case "e":
		m.startSynopsisEdit()
	case "ctrl+e":
		c := newExportChooser()
		m.exportChooser = &c
	case "a":
		m.structureAdding = true
		m.structureAddSel = 0
	case "x":
		if m.structureSel < len(m.structureItems) {
			f := m.structureItems[m.structureSel].folder
			delete(m.structurePendingNew, f)
			m.structureItems = append(m.structureItems[:m.structureSel], m.structureItems[m.structureSel+1:]...)
			if m.structureSel >= len(m.structureItems) && m.structureSel > 0 {
				m.structureSel--
			}
			m.structureDirty = true
		}
	case "r":
		if m.structureSel < len(m.structureItems) {
			m.structureRenaming = true
			m.nameInput.SetValue(m.structureItems[m.structureSel].title)
			m.nameInput.CursorEnd()
			m.nameInput.Focus()
			return m, textinput.Blink
		}
	case "enter":
		// Open the selected chapter; staged changes must be resolved first.
		if m.structureDirty {
			m.structureConfirm = true
			m.status = "appliquer les modifications d'abord — y appliquer · esc annuler"
		} else if m.structureSel < len(m.structureItems) {
			ch := m.structureItems[m.structureSel]
			if len(ch.texts) == 0 {
				m.status = "ce chapitre n'a pas encore de scène"
				return m, nil
			}
			if len(ch.texts) >= 2 {
				// Multi-scene chapter: let the reader pick which scene, instead of always
				// silently opening the first one.
				chCopy := ch
				m.textPickerChapter = &chCopy
				m.textPickerSel = 0
				m.textPickerDir = m.structureDir
				m.exitCorkboard()
				m.screen = screenTextPicker
				return m, nil
			}
			file := filepath.Join(m.structureDir, ch.folder, ch.texts[0].file)
			m.exitCorkboard()
			m.loadFile(file)
			m.focus = focusEditor
			m.editor.Focus()
		}
	case "esc":
		if m.structureDirty {
			m.structureConfirm = true
		} else {
			m.exitCorkboard()
		}
	}
	return m, nil
}

// exitCorkboard leaves the corkboard for the writing screen, clearing the staged buffer and
// reloading the pane so it reflects any committed structural changes.
func (m *model) exitCorkboard() {
	m.structureDirty = false
	m.structureConfirm = false
	m.structureAdding = false
	m.structureRenaming = false
	m.structureItems = nil
	m.files.SetDir(m.files.dir)
	m.screen = screenWriting
	m.focus = focusSidebar
}

// wrapClamp word-wraps s to width columns, clamped to maxLines (overflow → an ellipsis).
func wrapClamp(s string, width, maxLines int) string {
	if width < 1 {
		width = 1
	}
	var lines []string
	for _, para := range strings.Split(s, "\n") {
		words := strings.Fields(para)
		if len(words) == 0 {
			continue
		}
		cur := ""
		for _, w := range words {
			switch {
			case cur == "":
				cur = w
			case lipgloss.Width(cur)+1+lipgloss.Width(w) <= width:
				cur += " " + w
			default:
				lines = append(lines, cur)
				cur = w
			}
		}
		if cur != "" {
			lines = append(lines, cur)
		}
	}
	if len(lines) > maxLines {
		lines = lines[:maxLines]
		lines[maxLines-1] = ansi.Truncate(lines[maxLines-1], width, "…")
	}
	return strings.Join(lines, "\n")
}

// corkboardStatusLine summarizes the manuscript above the cards: chapter count, total words,
// and — when a project goal is set — progress toward it with an optional deadline.
func corkboardStatusLine(items []chapterRef, dir string, wc *wordCountCache, pg projectGoals) string {
	total := 0
	if wc != nil {
		for _, ch := range items {
			if len(ch.texts) == 0 {
				continue
			}
			total += wc.count(filepath.Join(dir, ch.folder, ch.texts[0].file))
		}
	}
	unit := "chapitres"
	if len(items) == 1 {
		unit = "chapitre"
	}
	line := strconv.Itoa(len(items)) + " " + unit + " · " + commafy(total) + " mots"
	if pg.ProjectGoal > 0 {
		line += " · " + commafy(total) + " / " + commafy(pg.ProjectGoal)
		if pg.Deadline != "" {
			line += " avant " + pg.Deadline
		}
	}
	return line
}

func (m model) corkboardView() string {
	const bodyRows = 3
	cardRows := bodyRows + 2        // + top/bottom border
	perCard := cardRows + 1         // + one blank line between cards
	vis := (m.height - 5) / perCard // -5 leaves room for the status header + footer rows
	if vis < 1 {
		vis = 1
	}
	off := homeWindowOffset(len(m.structureItems), m.structureSel, vis)
	cardW := max(40, min(m.width-8, 76))

	var cards []string
	for i := off; i < len(m.structureItems) && len(cards) < vis; i++ {
		it := m.structureItems[i]
		wc := ""
		var chPath string
		if len(it.texts) > 0 {
			chPath = filepath.Join(m.structureDir, it.folder, it.texts[0].file)
			if m.files.wc != nil {
				wc = commafy(m.files.wc.count(chPath)) + " m"
			}
		}
		isCurrent := m.currentFile != "" && chPath != "" && chPath == m.currentFile
		itKey := corkKey(it)
		openMark, rawBody, dim := corkboardCardMeta(isCurrent, m.synopses[itKey], m.corkFirstLines[itKey])
		var body string
		if rawBody == "" {
			body = lipgloss.NewStyle().Foreground(subtle).Render("(pas de synopsis — e pour éditer)")
		} else {
			body = wrapClamp(rawBody, cardW-4, bodyRows)
			if dim {
				body = lipgloss.NewStyle().Foreground(subtle).Render(body)
			}
		}
		marker := "  "
		if i == m.structureSel {
			marker = selectedStyle.Render("▸ ")
		}
		hdr := marker + fmtNum(i+1) + " · " + openMark + it.title
		cards = append(cards, framedPanel(hdr, body, cardW, cardRows, wc))
	}

	// Real Parts (not staged in m.structureItems — structure mode doesn't edit
	// Parts yet) render read-only, after the editable bare-chapter cards.
	rv := resolveManuscript(m.structureDir, readEntries(m.structureDir))
	for _, p := range rv.parts {
		if p.title == "" {
			continue // synthetic part — its chapters are already the editable cards above
		}
		header := lipgloss.NewStyle().Bold(true).Foreground(accent).Render(
			p.title + "  " + commafy(partWordTotal(m.structureDir, p, m.files.wc)) + " m")
		cards = append(cards, header)
		for _, ch := range p.chapters {
			wc := ""
			if len(ch.texts) > 0 {
				wc = commafy(m.files.wc.count(filepath.Join(m.structureDir, ch.folder, ch.texts[0].file))) + " m"
			}
			_, rawBody, dim := corkboardCardMeta(false, m.synopses[corkKey(ch)], m.corkFirstLines[corkKey(ch)])
			var body string
			if rawBody == "" {
				body = lipgloss.NewStyle().Foreground(subtle).Render("(pas de synopsis)")
			} else {
				body = wrapClamp(rawBody, cardW-4, bodyRows)
				if dim {
					body = lipgloss.NewStyle().Foreground(subtle).Render(body)
				}
			}
			cards = append(cards, framedPanel("  "+ch.title, body, cardW, cardRows, wc))
		}
	}

	if len(cards) == 0 {
		cards = append(cards, lipgloss.NewStyle().Foreground(subtle).Render("(aucun chapitre)"))
	}

	var b strings.Builder
	board := strings.Join(cards, "\n")
	hdr := lipgloss.NewStyle().Foreground(subtle).Render(
		corkboardStatusLine(m.structureItems, m.structureDir, m.files.wc, m.goalsAll[m.structureDir].applyEnvDefaults()))
	b.WriteString(lipgloss.PlaceHorizontal(m.width, lipgloss.Center, hdr) + "\n")
	b.WriteString(lipgloss.Place(m.width, m.height-2, lipgloss.Center, lipgloss.Center, board))

	if m.synEditing {
		edit := framedPanel("synopsis · "+m.structureItems[m.structureSel].title, m.synArea.View(), cardW, 5, "esc enregistrer")
		b.WriteString("\n" + lipgloss.PlaceHorizontal(m.width, lipgloss.Center, edit))
		return b.String()
	}
	if m.structureRenaming {
		field := "renommer ▸ " + m.nameInput.View()
		b.WriteString("\n" + lipgloss.PlaceHorizontal(m.width, lipgloss.Center, field))
		return b.String()
	}
	if m.structureAdding {
		var picks []string
		for i, c := range m.structureAddChoices() {
			label := c.label
			if i == m.structureAddSel {
				label = selectedStyle.Render(label)
			}
			picks = append(picks, label)
		}
		if len(picks) == 0 {
			picks = append(picks, lipgloss.NewStyle().Foreground(subtle).Render("(aucune ressource à promouvoir)"))
		}
		pick := framedPanel("ajouter", strings.Join(picks, "\n"), max(30, min(m.width-8, 44)), len(picks)+2, "")
		b.WriteString("\n" + lipgloss.PlaceHorizontal(m.width, lipgloss.Center, pick))
		return b.String()
	}
	if m.structureConfirm {
		bar := lipgloss.NewStyle().Foreground(accent).Render("appliquer les modifications ? y appliquer · esc annuler")
		b.WriteString("\n" + lipgloss.PlaceHorizontal(m.width, lipgloss.Center, bar))
		return b.String()
	}
	if m.exportChooser != nil {
		panel := exportChooserView(*m.exportChooser, m.width)
		b.WriteString("\n" + lipgloss.PlaceHorizontal(m.width, lipgloss.Center, panel))
		return b.String()
	}
	foot := lipgloss.NewStyle().Foreground(subtle).Render("J/K/alt réordonner · e synopsis · a ajouter · x retirer · r renommer · ⏎ ouvrir · esc · F1 aide")
	b.WriteString("\n" + lipgloss.PlaceHorizontal(m.width, lipgloss.Center, foot))
	return b.String()
}
