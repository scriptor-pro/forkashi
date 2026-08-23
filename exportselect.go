package main

import (
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// exportSelectEntry is one flattened row on the "all texts" screen: a chapter header, an
// indented scene, a standalone scene, or a Resource.
type exportSelectEntry struct {
	file     string // path relative to the manuscript dir — the sidecar's exclusion key
	title    string
	preview  string // first 111 chars, whitespace-normalized, ellipsis if truncated
	words    int
	chars    int // runes, spaces included
	excluded bool
	indent   bool // true for a scene inside a chapter (rendered indented)
	isHeader bool // true for a chapter header row (its own words/chars don't count — its child scenes do)
}

type exportSelectModel struct {
	entries []exportSelectEntry
	sel     int
}

// previewOf returns the first 111 runes of data's content, internal whitespace collapsed to
// single spaces, with a trailing "…" if the source was longer than 111 runes.
func previewOf(data []byte) string {
	fields := strings.Fields(string(data)) // splits on any whitespace, drops runs
	joined := strings.Join(fields, " ")
	runes := []rune(joined)
	if len(runes) <= 111 {
		return joined
	}
	return string(runes[:111]) + "…"
}

// buildExportSelectEntries resolves dir's manuscript into the flat display list: chapter
// headers + indented scenes, then standalone scenes, then Resources (alphabetical, matching
// v.loose's existing order). excluded is the loaded sidecar map, keyed by the same relative
// path used as each entry's file field.
func buildExportSelectEntries(dir string, v manuscriptView, excluded map[string]bool) []exportSelectEntry {
	var out []exportSelectEntry
	for _, part := range v.parts {
		for _, ch := range part.chapters {
			if ch.scene {
				continue // standalone scenes are collected in a second pass below, after chapters
			}
			out = append(out, exportSelectEntry{title: ch.title, isHeader: true})
			for _, t := range ch.texts {
				rel := filepath.Join(ch.folder, t.file)
				out = append(out, buildEntry(dir, rel, t.title, true, excluded))
			}
		}
	}
	for _, part := range v.parts {
		for _, ch := range part.chapters {
			if !ch.scene || len(ch.texts) == 0 {
				continue
			}
			rel := ch.texts[0].file
			out = append(out, buildEntry(dir, rel, ch.title, false, excluded))
		}
	}
	for _, e := range v.loose {
		out = append(out, buildEntry(dir, e.name, sectionTitle(e.name), false, excluded))
	}
	return out
}

// buildEntry reads rel's content once to compute words/chars/preview and reports its excluded
// state from the sidecar map.
func buildEntry(dir, rel, title string, indent bool, excluded map[string]bool) exportSelectEntry {
	data, _ := os.ReadFile(filepath.Join(dir, rel)) // best-effort: an unreadable file gets zero counts, not a crash
	text := string(data)
	return exportSelectEntry{
		file:     rel,
		title:    title,
		preview:  previewOf(data),
		words:    wordCount(text),
		chars:    charCount(text),
		excluded: excluded[rel],
		indent:   indent,
	}
}

// enterExportSelect resolves the current manuscript into the flat display list and shows the
// screen. All I/O happens here, once, before m.screen flips — exportSelectView stays I/O-free.
func (m *model) enterExportSelect() {
	dir := m.files.dir
	v := resolveManuscript(dir, readEntries(dir))
	excluded := loadExportSelection(dir)
	m.exportSelect = exportSelectModel{entries: buildExportSelectEntries(dir, v, excluded)}
	m.screen = screenExportSelect
}

func (m model) updateExportSelect(msg tea.Msg) (tea.Model, tea.Cmd) {
	if sz, ok := msg.(tea.WindowSizeMsg); ok {
		m.width, m.height = sz.Width, sz.Height
		return m, nil
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.screen = screenWriting
		m.focus = focusSidebar
	case "up", "k":
		if m.exportSelect.sel > 0 {
			m.exportSelect.sel--
		}
	case "down", "j":
		if m.exportSelect.sel < len(m.exportSelect.entries)-1 {
			m.exportSelect.sel++
		}
	case " ":
		m.toggleExportSelectAtCursor()
		m.saveExportSelectState()
	case "shift+up":
		m.moveExportSelectEntry(-1)
	case "shift+down":
		m.moveExportSelectEntry(1)
	}
	return m, nil
}

// toggleExportSelectAtCursor flips the selected entry's excluded state. Toggling a chapter
// header cascades to all of its child scenes (the rows immediately following it whose indent
// is true, up to the next header or the end of the list). Toggling a scene, standalone scene,
// or Resource only affects that one row; afterward every header's OWN excluded state is
// recomputed as derived (a header reads excluded only when ALL its children are excluded).
func (m *model) toggleExportSelectAtCursor() {
	entries := m.exportSelect.entries
	i := m.exportSelect.sel
	if i >= len(entries) {
		return
	}
	if entries[i].isHeader {
		newState := !entries[i].excluded
		for j := i + 1; j < len(entries) && entries[j].indent; j++ {
			entries[j].excluded = newState
		}
	} else {
		entries[i].excluded = !entries[i].excluded
	}
	recomputeExportSelectHeaders(entries)
}

// recomputeExportSelectHeaders sets each header's excluded field to true only when every one
// of its child (indented) rows is excluded — false (included) as soon as at least one child is
// included, including a header with zero children (nothing to exclude).
func recomputeExportSelectHeaders(entries []exportSelectEntry) {
	for i := range entries {
		if !entries[i].isHeader {
			continue
		}
		anyIncluded := false
		anyChild := false
		for j := i + 1; j < len(entries) && entries[j].indent; j++ {
			anyChild = true
			if !entries[j].excluded {
				anyIncluded = true
			}
		}
		entries[i].excluded = anyChild && !anyIncluded
	}
}

// saveExportSelectState persists every non-header entry's excluded state to the sidecar.
func (m *model) saveExportSelectState() {
	excluded := map[string]bool{}
	known := map[string]bool{}
	for _, e := range m.exportSelect.entries {
		if e.isHeader {
			continue
		}
		known[e.file] = true
		if e.excluded {
			excluded[e.file] = true
		}
	}
	if err := saveExportSelection(m.files.dir, excluded, known); err != nil {
		m.status = "échec de l'enregistrement de la sélection d'export : " + err.Error()
	}
}

// moveExportSelectEntry moves the selected row by dir (-1 up, +1 down) within its current
// container: a chapter header moves as a block (itself + all its scenes); a scene moves within
// its own chapter's Texts[]; a standalone scene or Resource moves among the other top-level
// rows. Persists the manifest (and re-resolves the entry list) immediately. Task 8 extends this
// to handle a scene/standalone-scene/Resource crossing into a DIFFERENT container.
func (m *model) moveExportSelectEntry(dir int) {
	entries := m.exportSelect.entries
	i := m.exportSelect.sel
	if i < 0 || i >= len(entries) {
		return
	}
	if entries[i].isHeader {
		m.moveExportSelectChapterBlock(i, dir)
		return
	}
	if entries[i].indent {
		m.moveExportSelectSceneWithinChapter(i, dir)
		return
	}
	// Standalone scene / Resource — Task 8 will replace this branch with full cross-boundary
	// logic. For this task, only reorder among entries of the SAME kind (a real chapter
	// boundary crossing is out of scope here).
}

// moveExportSelectChapterBlock moves the chapter header at index i (plus all its indented
// child rows) up or down past the ADJACENT chapter block, by swapping the two blocks' order in
// the on-disk manifest's bare-chapter items, then re-resolving the entry list from scratch.
func (m *model) moveExportSelectChapterBlock(i, dir int) {
	mani, present, err := readManifest(m.files.dir)
	if err != nil || !present {
		m.status = "impossible de déplacer : le manifeste est introuvable ou illisible"
		return
	}
	// Locate bare-chapter indices in mani.Items (Chapter != nil, Scene == false) in order.
	var bareIdx []int
	for idx, it := range mani.Items {
		if it.Chapter != nil && !it.Chapter.Scene {
			bareIdx = append(bareIdx, idx)
		}
	}
	// Map this header's position among headers in m.exportSelect.entries to a position in
	// bareIdx by counting headers seen up to i.
	headerPos := -1
	seen := 0
	for idx, e := range m.exportSelect.entries {
		if e.isHeader {
			if idx == i {
				headerPos = seen
				break
			}
			seen++
		}
	}
	if headerPos < 0 {
		return
	}
	target := headerPos + dir
	if target < 0 || target >= len(bareIdx) {
		return // already at an edge
	}
	mani.Items[bareIdx[headerPos]], mani.Items[bareIdx[target]] = mani.Items[bareIdx[target]], mani.Items[bareIdx[headerPos]]
	if err := writeManifest(m.files.dir, mani); err != nil {
		m.status = "échec du déplacement : " + err.Error()
		return
	}
	m.reloadExportSelectAfterMove()
}

// moveExportSelectSceneWithinChapter moves the scene at index i up/down within its own
// chapter's Texts[], refusing to cross the chapter's own header/footer boundary (that's Task
// 8's job). i's owning chapter is found by walking backward to the nearest header.
func (m *model) moveExportSelectSceneWithinChapter(i, dir int) {
	entries := m.exportSelect.entries
	headerIdx := -1
	for j := i - 1; j >= 0; j-- {
		if entries[j].isHeader {
			headerIdx = j
			break
		}
	}
	if headerIdx < 0 {
		return
	}
	// This chapter's scene rows span (headerIdx, chapterEnd).
	chapterEnd := len(entries)
	for j := headerIdx + 1; j < len(entries); j++ {
		if entries[j].isHeader || !entries[j].indent {
			chapterEnd = j
			break
		}
	}
	target := i + dir
	if target <= headerIdx || target >= chapterEnd {
		return // would leave this chapter — Task 8's job, not this task's
	}
	folder := m.chapterFolderForHeader(headerIdx)
	if folder == "" {
		return
	}
	mani, present, err := readManifest(m.files.dir)
	if err != nil || !present {
		m.status = "impossible de déplacer : le manifeste est introuvable ou illisible"
		return
	}
	ch := findChapterByFolder(&mani, folder)
	if ch == nil {
		return
	}
	sceneIdx := i - headerIdx - 1
	targetIdx := target - headerIdx - 1
	if sceneIdx < 0 || sceneIdx >= len(ch.Texts) || targetIdx < 0 || targetIdx >= len(ch.Texts) {
		return
	}
	ch.Texts[sceneIdx], ch.Texts[targetIdx] = ch.Texts[targetIdx], ch.Texts[sceneIdx]
	if err := writeManifest(m.files.dir, mani); err != nil {
		m.status = "échec du déplacement : " + err.Error()
		return
	}
	m.reloadExportSelectAfterMove()
}

// chapterFolderForHeader re-resolves the manuscript to find the folder of the chapter whose
// header is at entries[headerIdx] — counting headers up to headerIdx and matching against the
// resolved view's bare chapters in the same order buildExportSelectEntries used.
func (m model) chapterFolderForHeader(headerIdx int) string {
	v := resolveManuscript(m.files.dir, readEntries(m.files.dir))
	seen := 0
	for _, part := range v.parts {
		for _, ch := range part.chapters {
			if ch.scene {
				continue
			}
			if seen == countHeadersBefore(m.exportSelect.entries, headerIdx) {
				return ch.folder
			}
			seen++
		}
	}
	return ""
}

// countHeadersBefore counts header rows at indices strictly less than upTo. If entries[upTo]
// is itself a header, it is NOT counted — its own 0-indexed position among headers equals this
// count, which is exactly the alignment chapterFolderForHeader's "seen == countHeadersBefore(...)"
// comparison relies on (the Nth header, 0-indexed, is preceded by exactly N earlier headers).
func countHeadersBefore(entries []exportSelectEntry, upTo int) int {
	n := 0
	for i := 0; i < upTo && i < len(entries); i++ {
		if entries[i].isHeader {
			n++
		}
	}
	return n
}

// reloadExportSelectAfterMove re-resolves the manuscript and rebuilds the entry list — the
// simplest way to keep the display consistent with a just-written manifest, at the cost of a
// full re-read per move (acceptable: moves are an interactive, human-paced action, not a hot
// path — same tradeoff enterExportSelect already makes on screen entry).
func (m *model) reloadExportSelectAfterMove() {
	dir := m.files.dir
	v := resolveManuscript(dir, readEntries(dir))
	excluded := loadExportSelection(dir)
	sel := m.exportSelect.sel
	m.exportSelect = exportSelectModel{entries: buildExportSelectEntries(dir, v, excluded), sel: sel}
	if m.exportSelect.sel >= len(m.exportSelect.entries) {
		m.exportSelect.sel = len(m.exportSelect.entries) - 1
	}
}

// exportSelectTotals sums words/chars across every non-header, non-excluded entry.
func exportSelectTotals(entries []exportSelectEntry) (words, chars int) {
	for _, e := range entries {
		if e.isHeader || e.excluded {
			continue
		}
		words += e.words
		chars += e.chars
	}
	return words, chars
}

func exportSelectView(m model) string {
	a := m.exportSelect
	header := lipgloss.NewStyle().Foreground(accent).Bold(true).Render("── tous les textes ")

	var rows []string
	if len(a.entries) == 0 {
		rows = append(rows, lipgloss.NewStyle().Foreground(subtle).Render("  (aucun texte dans ce manuscrit)"))
	} else {
		width := max(10, min(m.width-8, 72))
		for i, e := range a.entries {
			box := "[x]"
			if e.excluded {
				box = "[ ]"
			}
			indent := ""
			if e.indent {
				indent = "  "
			}
			line := indent + box + " " + ansi.Truncate(e.title, width, "…")
			if i == a.sel {
				line = selectedStyle.Render("▸ " + line)
			} else {
				line = "  " + line
			}
			rows = append(rows, line)
			if !e.isHeader {
				preview := lipgloss.NewStyle().Foreground(subtle).Render(indent + "    " + ansi.Truncate(e.preview, width, "…"))
				rows = append(rows, preview)
			}
		}
	}
	body := header + "\n\n" + strings.Join(rows, "\n")

	words, chars := exportSelectTotals(a.entries)
	footer := commafy(words) + " mots · " + commafy(chars) + " caractères (espaces comprises) inclus dans l'export"

	var b strings.Builder
	b.WriteString(lipgloss.Place(m.width, m.height-2, lipgloss.Center, lipgloss.Center, body))
	b.WriteString("\n" + lipgloss.PlaceHorizontal(m.width, lipgloss.Center, lipgloss.NewStyle().Foreground(accent).Render(footer)))
	foot := lipgloss.NewStyle().Foreground(subtle).Render("↑↓ sélectionner · espace inclure/exclure · shift+↑↓ déplacer · esc retour · F1 aide")
	b.WriteString("\n" + lipgloss.PlaceHorizontal(m.width, lipgloss.Center, foot))
	return b.String()
}
