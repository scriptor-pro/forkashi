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
	m.moveTopLevelEntry(i, dir)
}

// moveTopLevelEntry handles a standalone scene or a Resource crossing into the OTHER kind
// (scene→Resource drops the manifest item, file stays at root; Resource→scene adds a new
// Scene:true item, file stays at root — neither moves a file, since both live at the
// manuscript root already) or reordering among entries of the same kind (Resources sort
// alphabetically today and are not manifest-ordered, so "moving" a Resource past another
// Resource has no manifest effect; moving a standalone scene past another standalone scene
// reorders items[]).
func (m *model) moveTopLevelEntry(i, dir int) {
	entries := m.exportSelect.entries
	target := i + dir
	if target < 0 || target >= len(entries) || entries[target].isHeader || entries[target].indent {
		return
	}
	movingIsScene := !entries[i].isHeader && !entries[i].indent && isStandaloneSceneFile(m.files.dir, entries[i].file)
	targetIsScene := !entries[target].isHeader && !entries[target].indent && isStandaloneSceneFile(m.files.dir, entries[target].file)

	if movingIsScene && targetIsScene {
		m.reorderStandaloneScenes(entries[i].file, entries[target].file)
		return
	}
	if movingIsScene && !targetIsScene {
		m.convertStandaloneSceneToResource(entries[i].file)
		return
	}
	if !movingIsScene && targetIsScene {
		m.convertResourceToStandaloneScene(entries[i].file)
		return
	}
	// Both Resources: no manifest to touch (Resources aren't ordered by items[]) — nothing to persist.
}

// isStandaloneSceneFile reports whether file (relative to dir) is currently listed as a
// Scene:true item's Texts[0].File in dir's manifest.
func isStandaloneSceneFile(dir, file string) bool {
	mani, present, err := readManifest(dir)
	if err != nil || !present {
		return false
	}
	return findSceneByFile(&mani, file) != nil
}

// reorderStandaloneScenes swaps the items[] positions of the two standalone scenes referencing
// fileA and fileB — no file moves, both already live at the manuscript root.
func (m *model) reorderStandaloneScenes(fileA, fileB string) {
	mani, present, err := readManifest(m.files.dir)
	if err != nil || !present {
		m.status = "impossible de déplacer : le manifeste est introuvable ou illisible"
		return
	}
	idxA, idxB := -1, -1
	for idx, it := range mani.Items {
		if it.Chapter == nil || !it.Chapter.Scene || len(it.Chapter.Texts) == 0 {
			continue
		}
		switch it.Chapter.Texts[0].File {
		case fileA:
			idxA = idx
		case fileB:
			idxB = idx
		}
	}
	if idxA < 0 || idxB < 0 {
		return
	}
	mani.Items[idxA], mani.Items[idxB] = mani.Items[idxB], mani.Items[idxA]
	if err := writeManifest(m.files.dir, mani); err != nil {
		m.status = "échec du déplacement : " + err.Error()
		return
	}
	m.reloadExportSelectAfterMove(followFile(fileA))
}

// convertStandaloneSceneToResource drops file's Scene:true item from items[] — the file stays
// on disk at the manuscript root, unmoved, and becomes a Resource by the existing definition
// (an unlisted root .md file).
func (m *model) convertStandaloneSceneToResource(file string) {
	mani, present, err := readManifest(m.files.dir)
	if err != nil || !present {
		m.status = "impossible de déplacer : le manifeste est introuvable ou illisible"
		return
	}
	var kept []manifestItem
	for _, it := range mani.Items {
		if it.Chapter != nil && it.Chapter.Scene && len(it.Chapter.Texts) > 0 && it.Chapter.Texts[0].File == file {
			continue
		}
		kept = append(kept, it)
	}
	mani.Items = kept
	if err := writeManifest(m.files.dir, mani); err != nil {
		m.status = "échec du déplacement : " + err.Error()
		return
	}
	m.reloadExportSelectAfterMove(followFile(file))
}

// convertResourceToStandaloneScene adds file as a new Scene:true item at the end of items[] —
// the file stays on disk at the manuscript root, unmoved.
func (m *model) convertResourceToStandaloneScene(file string) {
	mani, present, err := readManifest(m.files.dir)
	if err != nil || !present {
		m.status = "impossible de déplacer : le manifeste est introuvable ou illisible"
		return
	}
	title := sectionTitle(file)
	mani.Items = append(mani.Items, manifestItem{Chapter: &manifestChapter{
		Title: title,
		Scene: true,
		Texts: []manifestText{{File: file, Title: title}},
	}})
	if err := writeManifest(m.files.dir, mani); err != nil {
		m.status = "échec du déplacement : " + err.Error()
		return
	}
	m.reloadExportSelectAfterMove(followFile(file))
}

// convertChapterSceneToStandalone drops file from its parent chapter's Texts[] (identified by
// folder) and adds it as a new Scene:true item at the end of items[] — the scene's own title is
// preserved. Unlike the standalone-scene↔Resource conversions (which never move a file, since
// both statuses already live at the manuscript root), this DOES move the file on disk, chapter
// folder → manuscript root, using the same safeMove already used by moveSceneBetweenChapters.
func (m *model) convertChapterSceneToStandalone(folder, file string) {
	mani, present, err := readManifest(m.files.dir)
	if err != nil || !present {
		m.status = "impossible de déplacer : le manifeste est introuvable ou illisible"
		return
	}
	ch := findChapterByFolder(&mani, folder)
	if ch == nil {
		return
	}
	base := filepath.Base(file)
	var moved manifestText
	found := false
	kept := ch.Texts[:0]
	for _, t := range ch.Texts {
		if t.File == base {
			moved = t
			found = true
			continue
		}
		kept = append(kept, t)
	}
	if !found {
		return
	}

	src := filepath.Join(m.files.dir, folder, base)
	dst := filepath.Join(m.files.dir, base)
	if _, err := os.Stat(dst); err == nil {
		m.status = "un fichier nommé " + base + " existe déjà à la racine du manuscrit"
		return
	}
	if err := safeMove(src, dst); err != nil {
		m.status = "échec du déplacement : " + err.Error()
		return
	}
	ch.Texts = kept

	mani.Items = append(mani.Items, manifestItem{Chapter: &manifestChapter{
		Title: moved.Title,
		Scene: true,
		Texts: []manifestText{{File: base, Title: moved.Title}},
	}})
	if err := writeManifest(m.files.dir, mani); err != nil {
		m.status = "échec du déplacement : " + err.Error()
		return
	}

	oldKey := filepath.Join(folder, base)
	excluded := loadExportSelection(m.files.dir)
	if excluded[oldKey] {
		delete(excluded, oldKey)
		excluded[base] = true
		known := map[string]bool{base: true}
		for k := range excluded {
			known[k] = true
		}
		if err := saveExportSelection(m.files.dir, excluded, known); err != nil {
			m.status = "scène convertie mais échec de la migration de la sélection d'export : " + err.Error()
			return
		}
	}
	m.reloadExportSelectAfterMove(followFile(base))
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
	movedTitle := mani.Items[bareIdx[headerPos]].Chapter.Title
	mani.Items[bareIdx[headerPos]], mani.Items[bareIdx[target]] = mani.Items[bareIdx[target]], mani.Items[bareIdx[headerPos]]
	if err := writeManifest(m.files.dir, mani); err != nil {
		m.status = "échec du déplacement : " + err.Error()
		return
	}
	m.reloadExportSelectAfterMove(followHeader(movedTitle))
}

// moveExportSelectSceneWithinChapter moves the scene at index i up/down. If the target stays
// inside the same chapter's scene span, it's a simple Texts[] reorder (no disk change). If the
// target crosses into an ADJACENT chapter, the scene's file is moved on disk (safeMove) and the
// manifest is rewritten to drop it from the source chapter's Texts[] and append it to the
// target chapter's Texts[] at the crossed position; the sidecar's exclusion key (if any)
// migrates to the new path.
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
	chapterEnd := len(entries)
	for j := headerIdx + 1; j < len(entries); j++ {
		if entries[j].isHeader || !entries[j].indent {
			chapterEnd = j
			break
		}
	}
	target := i + dir

	if target > headerIdx && target < chapterEnd {
		// Same-chapter reorder — Task 7's logic, unchanged.
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
		m.reloadExportSelectAfterMove(followFile(entries[i].file))
		return
	}

	// Crosses this chapter's boundary — find which adjacent chapter header we crossed into.
	var dstHeaderIdx int
	if dir < 0 {
		dstHeaderIdx = headerIdx - 1
		for dstHeaderIdx >= 0 && !entries[dstHeaderIdx].isHeader {
			dstHeaderIdx--
		}
	} else {
		dstHeaderIdx = chapterEnd
	}
	if dstHeaderIdx < 0 || dstHeaderIdx >= len(entries) || !entries[dstHeaderIdx].isHeader {
		return // no adjacent chapter in that direction — already at an edge
	}
	srcFolder := m.chapterFolderForHeader(headerIdx)
	dstFolder := m.chapterFolderForHeader(dstHeaderIdx)
	if srcFolder == "" || dstFolder == "" {
		return
	}
	m.moveSceneBetweenChapters(srcFolder, entries[i].file, dstFolder)
}

// moveSceneBetweenChapters moves file (currently in srcFolder's Texts[]) into dstFolder: the
// on-disk file first (safeMove; refused-collision leaves everything untouched), then a single
// manifest read-modify-write that drops it from srcFolder and appends it to dstFolder, then
// migrates the sidecar's exclusion key if the file had one.
func (m *model) moveSceneBetweenChapters(srcFolder, file, dstFolder string) {
	base := filepath.Base(file)
	src := filepath.Join(m.files.dir, srcFolder, base)
	dst := filepath.Join(m.files.dir, dstFolder, base)
	if _, err := os.Stat(dst); err == nil {
		m.status = "un fichier nommé " + base + " existe déjà dans le chapitre de destination"
		return
	}
	if err := safeMove(src, dst); err != nil {
		m.status = "échec du déplacement : " + err.Error()
		return
	}
	mani, present, err := readManifest(m.files.dir)
	if err != nil || !present {
		m.status = "scène déplacée mais le manifeste est introuvable ou illisible"
		return
	}
	srcCh := findChapterByFolder(&mani, srcFolder)
	dstCh := findChapterByFolder(&mani, dstFolder)
	if srcCh == nil || dstCh == nil {
		m.status = "scène déplacée mais un chapitre est introuvable dans le manifeste"
		return
	}
	var moved manifestText
	kept := srcCh.Texts[:0]
	for _, t := range srcCh.Texts {
		if t.File == base {
			moved = t
			continue
		}
		kept = append(kept, t)
	}
	srcCh.Texts = kept
	dstCh.Texts = append(dstCh.Texts, moved)
	if err := writeManifest(m.files.dir, mani); err != nil {
		m.status = "échec de la mise à jour du manifeste après déplacement : " + err.Error()
		return
	}
	oldKey := filepath.Join(srcFolder, base)
	newKey := filepath.Join(dstFolder, base)
	excluded := loadExportSelection(m.files.dir)
	if excluded[oldKey] {
		delete(excluded, oldKey)
		excluded[newKey] = true
		known := map[string]bool{newKey: true}
		for k := range excluded {
			known[k] = true
		}
		if err := saveExportSelection(m.files.dir, excluded, known); err != nil {
			m.status = "scène déplacée mais échec de la migration de la sélection d'export : " + err.Error()
			return
		}
	}
	m.reloadExportSelectAfterMove(followFile(newKey))
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
//
// follow identifies the entry the cursor must land on in the rebuilt list: a non-header row is
// identified by its file (relative path, unique across entries); a chapter-header row has no
// file, so it is identified by isHeader==true and its title instead. This is the ONE mechanism
// every caller uses — same-container reorders (Task 7, where the numeric index happens to stay
// correct by construction) and cross-container moves (Task 8, where entry count on either side
// of the cursor can change non-trivially) both resolve sel by re-finding this same entry in the
// rebuilt list, rather than one of them clamping a stale numeric index. If no entry matches
// (should not happen for a successful move), sel falls back to a clamp so it stays in range.
func (m *model) reloadExportSelectAfterMove(follow exportSelectFollow) {
	dir := m.files.dir
	v := resolveManuscript(dir, readEntries(dir))
	excluded := loadExportSelection(dir)
	prevSel := m.exportSelect.sel
	entries := buildExportSelectEntries(dir, v, excluded)
	m.exportSelect = exportSelectModel{entries: entries, sel: prevSel}
	if idx := follow.locate(entries); idx >= 0 {
		m.exportSelect.sel = idx
		return
	}
	if m.exportSelect.sel >= len(entries) {
		m.exportSelect.sel = len(entries) - 1
	}
}

// exportSelectFollow identifies which rebuilt entry reloadExportSelectAfterMove should move the
// cursor to. Exactly one of file (non-header rows) or headerTitle (chapter-header rows, which
// carry no file) is set.
type exportSelectFollow struct {
	file        string
	headerTitle string
	isHeader    bool
}

// followFile identifies a non-header entry (scene, standalone scene, or Resource) by its
// relative file path.
func followFile(file string) exportSelectFollow { return exportSelectFollow{file: file} }

// followHeader identifies a chapter-header entry by its title.
func followHeader(title string) exportSelectFollow {
	return exportSelectFollow{headerTitle: title, isHeader: true}
}

// locate finds follow's entry in the rebuilt list, returning its index or -1 if not found.
func (f exportSelectFollow) locate(entries []exportSelectEntry) int {
	if f.isHeader {
		for i, e := range entries {
			if e.isHeader && e.title == f.headerTitle {
				return i
			}
		}
		return -1
	}
	if f.file == "" {
		return -1
	}
	for i, e := range entries {
		if !e.isHeader && e.file == f.file {
			return i
		}
	}
	return -1
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
