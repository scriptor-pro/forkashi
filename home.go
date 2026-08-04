package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type homeKind int

const (
	homePinned homeKind = iota
	homeRecentFile
	homeProject
	homeFolder
	homeLoose
	homeNewDocument
	homeNewProject
	homeMoveFiles
	homeOpenOther
)

// homeFileItem is one entry in the FILES column: a document (name, path, word count, one-line
// snippet) or, when isDir is set, a drillable subfolder or the ".." parent entry.
type homeFileItem struct {
	name, path, snippet string
	words               int
	isDir               bool
}

// classifyLibrary splits the workspace's top-level subdirs into manuscripts (projects) and
// plain category folders, alpha-sorted, excluding hidden dirs.
func classifyLibrary(workspace string) (projects, folders []homeItem) {
	entries, err := os.ReadDir(workspace)
	if err != nil {
		return
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, n := range names {
		dir := filepath.Join(workspace, n)
		if dirIsManuscript(dir) {
			projects = append(projects, homeItem{kind: homeProject, label: n, path: dir})
		} else {
			folders = append(folders, homeItem{kind: homeFolder, label: n, path: dir})
		}
	}
	return
}

// dirIsManuscript reports whether dir resolves as an ordered manuscript (manifest or legacy
// numbered files) vs. a plain category folder.
func dirIsManuscript(dir string) bool {
	sub, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	var fes []fileEntry
	for _, e := range sub {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if allowedDocExts[strings.ToLower(filepath.Ext(e.Name()))] { // match homeFilesFor
			fes = append(fes, fileEntry{name: e.Name()})
		}
	}
	return resolveManuscript(dir, fes).ordered()
}

// homeFilesFor resolves a dir into FILES entries: when showFolders is set, drillable subfolders
// (alpha-sorted, hidden excluded) come first, then the documents (chapters in manifest order,
// then loose), each with word count + snippet. showFolders is false for the flat Notes bucket.
func (m *model) homeFilesFor(dir string, showFolders bool) []homeFileItem {
	sub, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var dirNames []string
	var fes []fileEntry
	for _, e := range sub {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if e.IsDir() {
			dirNames = append(dirNames, e.Name())
			continue
		}
		if m.files.allowed[strings.ToLower(filepath.Ext(e.Name()))] {
			fes = append(fes, fileEntry{name: e.Name()})
		}
	}
	view := resolveManuscript(dir, fes)
	// A v2 chapter's folder is a real directory on disk — exclude it from the plain
	// folders list (below) so it isn't listed twice: once as a drillable folder, once
	// as its chapter entry (mirrors the same fix in filelist.go's SetDir).
	var folders []homeFileItem
	if showFolders {
		for _, name := range dirNames {
			if isChapterOf(view, name) {
				continue
			}
			folders = append(folders, homeFileItem{name: name, path: filepath.Join(dir, name), isDir: true})
		}
	}
	sort.Slice(folders, func(i, j int) bool { return folders[i].name < folders[j].name })
	mk := func(name, file string) homeFileItem {
		p := filepath.Join(dir, file)
		return homeFileItem{name: name, path: p, words: m.files.wc.count(p), snippet: m.snippets.get(p)}
	}
	out := folders
	for _, p := range view.parts {
		for _, ch := range p.chapters {
			if len(ch.texts) == 0 {
				continue
			}
			out = append(out, mk(ch.title, filepath.Join(ch.folder, ch.texts[0].file)))
		}
	}
	for _, l := range view.loose {
		out = append(out, mk(l.name, l.name))
	}
	return out
}

// homeRegion identifies a navigable group on the launch screen: the Projects
// column, the Recent column, or the centered Actions below them.
type homeRegion int

const (
	regionPinned homeRegion = iota
	regionRecent
	regionLibrary
	regionFiles
	regionActions
)

// Box widths (total, incl. the rounded border; framedPanel content = width-4). LIBRARY/FILES grow
// responsively with the terminal between their min and max (see homeColumns), so a wide terminal
// isn't a tiny island in empty space and long titles stop truncating.
const (
	homeRecentBox     = 20
	homeLibraryBoxMin = 22
	homeLibraryBoxMax = 34
	homeFilesBoxMin   = 38
	homeFilesBoxMax   = 68
	homeColGap        = 2
)

// homeItem is one selectable entry on the launch screen.
type homeItem struct {
	kind  homeKind
	label string
	path  string
}

// homeCell is a clickable item's position within the pre-Place content block.
type homeCell struct {
	region homeRegion
	index  int
	row    int
	x0, x1 int
}

// innerCell is a clickable item relative to a column's content (row 0 = first content
// line, x 0 = first content column); translated to a homeCell when the box is placed.
type innerCell struct {
	region homeRegion
	index  int
	row    int
	x0, x1 int
}

// buildHomeItems composes the flat launch list: pinned items, then recent files, then projects
// (manuscripts), then folders (categories), then the actions. The three-column launcher groups by kind.
func buildHomeItems(recents []string, workspace string, pinned []string) []homeItem {
	var items []homeItem
	for _, p := range pinned {
		if _, err := os.Stat(p); err == nil { // skip dead pins
			items = append(items, homeItem{kind: homePinned, label: "★ " + filepath.Base(p), path: p})
		}
	}
	for _, p := range recents {
		items = append(items, homeItem{kind: homeRecentFile, label: filepath.Base(p), path: p})
	}
	projects, folders := classifyLibrary(workspace)
	items = append(items, projects...)
	items = append(items, folders...)
	items = append(items, homeItem{kind: homeLoose, label: "◦ Notes", path: workspace})
	// New document / New project now live as the inline `+` on the FILES / LIBRARY panels
	// (design §4); the action row is Move files + Browse.
	items = append(items,
		homeItem{kind: homeMoveFiles, label: "Move files"},
		homeItem{kind: homeOpenOther, label: "Browse all files"},
	)
	return items
}

// homePlusIndex is the sentinel cell index for a panel's inline "+" affordance (LIBRARY/FILES),
// distinguishing a click on the "+" from a click on a list row.
const homePlusIndex = -1

// homeCreate opens a create prompt for a panel's inline "+": LIBRARY + makes a project or folder
// in the active source root (trailing-slash → folder, plain name → manuscript); FILES + makes a
// new document in the selected library item's directory. Both reuse the writing-screen create flow.
func (m *model) homeCreate(region homeRegion) tea.Cmd {
	switch region {
	case regionLibrary:
		m.files.SetDir(m.activeSourceRoot())
		m.enterWriting()
		return m.startCreate(true)
	case regionFiles:
		if m.homeFilesDir == "" {
			return nil
		}
		m.files.SetDir(m.homeFilesDir) // create in the currently-viewed (possibly drilled) dir
		m.enterWriting()
		return m.startCreate(false)
	}
	return nil
}

// homeGroups splits the flat list by kind. `other` holds the loose/Notes entry, which renders
// in its own OTHER section at the foot of the library.
func homeGroups(items []homeItem) (pinned, recents, projects, folders, other, actions []homeItem) {
	for _, it := range items {
		switch it.kind {
		case homePinned:
			pinned = append(pinned, it)
		case homeRecentFile:
			recents = append(recents, it)
		case homeProject:
			projects = append(projects, it)
		case homeFolder:
			folders = append(folders, it)
		case homeLoose:
			other = append(other, it)
		default:
			actions = append(actions, it)
		}
	}
	return
}

func (m model) recents() []homeItem     { _, r, _, _, _, _ := homeGroups(m.homeItems); return r }
func (m model) actions() []homeItem     { _, _, _, _, _, a := homeGroups(m.homeItems); return a }
func (m model) pinnedItems() []homeItem { p, _, _, _, _, _ := homeGroups(m.homeItems); return p }
func (m model) library() []homeItem {
	_, _, projects, folders, other, _ := homeGroups(m.homeItems)
	lib := append([]homeItem{}, projects...)
	lib = append(lib, folders...)
	lib = append(lib, other...)
	return lib
}

func (m model) regionCount(r homeRegion) int {
	switch r {
	case regionPinned:
		return len(m.pinnedItems())
	case regionRecent:
		return len(m.recents())
	case regionLibrary:
		return len(m.library())
	case regionFiles:
		return len(m.homeFiles)
	default:
		return len(m.actions())
	}
}

// visibleRegions returns the regions actually rendered at the current width (per
// homeColumns) plus Actions — the single source nav and reset both consult so they can
// never focus a column the render dropped. Width 0 (unsized) treats all as visible.
func (m model) visibleRegions() []homeRegion {
	if m.width <= 0 {
		regs := []homeRegion{}
		if m.regionCount(regionPinned) > 0 { // only when there are pins (match the sized path)
			regs = append(regs, regionPinned)
		}
		return append(regs, regionRecent, regionLibrary, regionFiles, regionActions)
	}
	cols, _, _ := m.homeColumns()
	var regs []homeRegion
	if m.regionCount(regionPinned) > 0 {
		regs = append(regs, regionPinned)
	}
	regs = append(regs, regionRecent)
	regs = append(regs, cols...)
	return append(regs, regionActions)
}

// visibleCols is the rendered browse columns — visibleRegions without the PINNED strip, the
// RECENT strip, or the Actions row (i.e. just LIBRARY/FILES that fit the width).
func (m model) visibleCols() []homeRegion {
	var out []homeRegion
	for _, r := range m.visibleRegions() {
		if r != regionActions && r != regionRecent && r != regionPinned {
			out = append(out, r)
		}
	}
	return out
}

func (m model) regionVisible(r homeRegion) bool {
	for _, v := range m.visibleRegions() {
		if v == r {
			return true
		}
	}
	return false
}

// activeSourceRoot is the filesystem root of the active library source. Falls back to
// writingDir() if the index is somehow out of range (defensive; activeSource is clamped).
func (m model) activeSourceRoot() string {
	if m.activeSource < 0 || m.activeSource >= len(m.sources) {
		return writingDir()
	}
	return m.sources[m.activeSource].root()
}

// rebuildHome rebuilds the launch list from recents + the active source's library, then
// refreshes the FILES column. Call after anything that changes the active source.
func (m *model) rebuildHome() {
	m.homeItems = buildHomeItems(loadRecents(recentPath()), m.activeSourceRoot(), m.pinned)
	m.recomputeHomeFiles()
}

// recomputeHomeFiles fills the FILES column from the selected library item, honoring any
// drill-down (m.homeFilesDir). The Notes bucket is flat (loose docs only, no folders/drill);
// projects and categories cascade — subfolders are drillable and a ".." entry leads back up,
// bounded to the selected item's root. A changed library selection resets the drill.
func (m *model) recomputeHomeFiles() {
	lib := m.library()
	if m.librarySelected < 0 || m.librarySelected >= len(lib) {
		m.homeFiles = nil
		m.homeFilesDir = ""
		return
	}
	item := lib[m.librarySelected]
	if item.kind == homeLoose { // Notes: flat loose bucket, no cascade
		m.homeFilesDir = item.path
		m.homeFiles = m.homeFilesFor(item.path, false)
		return
	}
	// Reset the drill when the selection moved out of the current subtree.
	if m.homeFilesDir == "" || !withinRoot(m.homeFilesDir, item.path) {
		m.homeFilesDir = item.path
	}
	items := m.homeFilesFor(m.homeFilesDir, true)
	if m.homeFilesDir != item.path { // drilled below the item root → offer a way back up
		items = append([]homeFileItem{{name: "..", path: filepath.Dir(m.homeFilesDir), isDir: true}}, items...)
	}
	m.homeFiles = items
}

// resetHomeSelection focuses the first non-empty column (Recent first) and points the
// library at its first item so FILES is populated.
func (m *model) resetHomeSelection() {
	if m.librarySelected < 0 || m.librarySelected >= len(m.library()) {
		m.librarySelected = 0
	}
	m.recomputeHomeFiles()
	m.homeRegion = regionActions
	for _, r := range m.visibleRegions() {
		if m.regionCount(r) > 0 {
			m.homeRegion = r
			break
		}
	}
	m.homeLastCol = regionLibrary
	if vc := m.visibleCols(); len(vc) > 0 {
		m.homeLastCol = vc[0]
	}
	if m.homeRegion != regionActions {
		m.homeLastCol = m.homeRegion
	}
	if m.homeRegion == regionLibrary {
		m.homeIndex = m.librarySelected
	} else {
		m.homeIndex = 0
	}
}

// ensureVisibleFocus refocuses to the first visible non-empty region if the current focus
// became hidden (e.g. after a resize dropped its column).
func (m *model) ensureVisibleFocus() {
	if m.regionVisible(m.homeRegion) {
		return
	}
	for _, r := range m.visibleRegions() {
		if m.regionCount(r) > 0 {
			m.focusAt(r, m.indexIn(r))
			return
		}
	}
}

// updateHome handles input on the launch screen.
func (m model) updateHome(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.ensureVisibleFocus() // a narrower width may have dropped the focused column
		return m, nil
	case tea.KeyMsg:
		// While the add-source prompt is active, capture all input for nameInput.
		if m.addingSource {
			switch msg.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "esc":
				m.addingSource = false
				m.nameInput.Blur()
				m.status = "add source cancelled"
				return m, nil
			case "enter":
				m.addingSource = false
				m.nameInput.Blur()
				m.confirmAddSource(strings.TrimSpace(m.nameInput.Value()))
				return m, nil
			}
			var cmd tea.Cmd
			m.nameInput, cmd = m.nameInput.Update(msg)
			return m, cmd
		}
		// While confirming a source removal, only y/enter removes; anything else cancels.
		if m.confirmRemoveSource {
			m.confirmRemoveSource = false
			switch msg.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "y", "enter":
				m.removeActiveSource()
			default:
				m.status = "removal cancelled"
			}
			return m, nil
		}
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			m.homeMove(0, -1)
		case "down", "j":
			m.homeMove(0, 1)
		case "left", "h":
			m.homeMove(-1, 0)
		case "right", "l":
			m.homeMove(1, 0)
		case "tab":
			m.homeCycleRegion(1)
		case "shift+tab":
			m.homeCycleRegion(-1)
		case "s":
			m.cycleSource(1)
		case "a":
			m.addingSource = true
			m.nameInput.SetValue("")
			m.nameInput.Placeholder = "/path/to/folder"
			m.nameInput.Focus()
			return m, textinput.Blink
		case "d":
			if m.activeSource >= 0 && m.activeSource < len(m.sources) && m.sources[m.activeSource].Kind != sourceKindPrimary {
				m.confirmRemoveSource = true
				m.status = "remove source \"" + m.sources[m.activeSource].Name + "\"? y = remove · re-add with a"
			} else {
				m.status = "the primary source can't be removed"
			}
		case "p":
			if m.homeRegion == regionLibrary {
				lib := m.library()
				if m.librarySelected >= 0 && m.librarySelected < len(lib) {
					it := lib[m.librarySelected]
					if it.kind == homeProject || it.kind == homeFolder {
						m.pinned = togglePin(pinsPath(), it.path)
						m.rebuildHome()
					}
				}
			}
		case "i":
			if m.homeRegion == regionLibrary {
				lib := m.library()
				if m.librarySelected >= 0 && m.librarySelected < len(lib) {
					it := lib[m.librarySelected]
					if it.kind == homeProject || it.kind == homeFolder {
						m.properties = newPropertiesModel(it.path)
						m.screen = screenProperties
						m.status = "properties"
					}
				}
			}
		case "+", "n":
			if m.homeRegion == regionLibrary || m.homeRegion == regionFiles {
				return m, m.homeCreate(m.homeRegion)
			}
		case "enter":
			return m, m.openHomeSelection()
		}
	case tea.MouseMsg:
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			m.homeMove(0, -1)
		case tea.MouseButtonWheelDown:
			m.homeMove(0, 1)
		case tea.MouseButtonLeft:
			if msg.Action != tea.MouseActionPress {
				return m, nil
			}
			r, idx, ok := m.homeItemAt(msg.X, msg.Y)
			if !ok {
				return m, nil
			}
			if idx == homePlusIndex { // clicked a panel's inline "+"
				return m, m.homeCreate(r)
			}
			doubled := r == m.homeRegion && idx == m.indexIn(r) &&
				time.Since(m.lastClickTime) < 400*time.Millisecond
			m.focusAt(r, idx)
			if doubled {
				m.lastClickTime = time.Time{}
				return m, m.openHomeSelection()
			}
			m.lastClickTime = time.Now()
		}
		return m, nil
	}
	return m, nil
}

// indexIn returns the active index within region r (library uses librarySelected).
func (m model) indexIn(r homeRegion) int {
	if r == regionLibrary {
		return m.librarySelected
	}
	if r == m.homeRegion {
		return m.homeIndex
	}
	return 0
}

// focusAt focuses region r at index idx (clamped; library drives FILES).
func (m *model) focusAt(r homeRegion, idx int) {
	n := m.regionCount(r)
	if n == 0 {
		return
	}
	if idx < 0 {
		idx = 0
	}
	if idx >= n {
		idx = n - 1
	}
	m.homeRegion = r
	m.homeIndex = idx
	if r != regionActions {
		m.homeLastCol = r
	}
	if r == regionLibrary {
		m.librarySelected = idx
		m.recomputeHomeFiles()
	}
}

// homeMove navigates the launcher: up/down within a column (flowing into Actions),
// left/right across Recent ↔ Library ↔ Files.
func clampIdx(i, n int) int {
	if i < 0 {
		return 0
	}
	if i >= n {
		return n - 1
	}
	return i
}

// homeMove navigates the launcher: the PINNED + RECENT strips on top (horizontal), the LIBRARY/FILES
// columns in the middle, and the Actions row at the bottom (horizontal). Down flows strip →
// columns → actions; up flows actions → columns → strip; left/right move within a strip/row or
// across the columns.
func (m *model) homeMove(dx, dy int) {
	switch m.homeRegion {
	case regionPinned:
		if dy > 0 { // down → into the RECENT strip, then a column, then actions
			if m.regionCount(regionRecent) > 0 {
				m.focusAt(regionRecent, 0)
				return
			}
			for _, r := range m.visibleCols() {
				if m.regionCount(r) > 0 {
					m.focusAt(r, m.indexIn(r))
					return
				}
			}
			if m.regionCount(regionActions) > 0 {
				m.homeLastCol = regionPinned
				m.focusAt(regionActions, 0)
			}
			return
		}
		if dy < 0 {
			return // nothing above the PINNED strip
		}
		m.focusAt(regionPinned, clampIdx(m.homeIndex+dx, m.regionCount(regionPinned)))
		return

	case regionRecent:
		if dy > 0 { // down → into the first non-empty column, else the actions row
			for _, r := range m.visibleCols() {
				if m.regionCount(r) > 0 {
					m.focusAt(r, m.indexIn(r))
					return
				}
			}
			if m.regionCount(regionActions) > 0 {
				m.homeLastCol = regionRecent
				m.focusAt(regionActions, 0)
			}
			return
		}
		if dy < 0 {
			if m.regionCount(regionPinned) > 0 { // up → into the PINNED strip when pins exist
				m.focusAt(regionPinned, 0)
				return
			}
			return // nothing above the RECENT strip
		}
		m.focusAt(regionRecent, clampIdx(m.homeIndex+dx, m.regionCount(regionRecent)))
		return

	case regionActions:
		if dy < 0 { // up → the column we came from (or the last non-empty column / the strip)
			target := m.homeLastCol
			if !m.regionVisible(target) || m.regionCount(target) == 0 {
				target = regionRecent
				for _, r := range m.visibleCols() {
					if m.regionCount(r) > 0 {
						target = r
					}
				}
			}
			if m.regionVisible(target) && m.regionCount(target) > 0 {
				m.focusAt(target, m.regionCount(target)-1)
			}
			return
		}
		if dy > 0 {
			return // nothing below the actions row
		}
		m.focusAt(regionActions, clampIdx(m.homeIndex+dx, m.regionCount(regionActions)))
		return

	default: // a LIBRARY/FILES column
		if dy != 0 {
			n := m.regionCount(m.homeRegion)
			if dy > 0 {
				if m.homeIndex >= n-1 { // bottom of the column → the actions row
					if m.regionCount(regionActions) > 0 {
						m.homeLastCol = m.homeRegion
						m.focusAt(regionActions, 0)
					}
					return
				}
				m.focusAt(m.homeRegion, m.homeIndex+1)
				return
			}
			if m.homeIndex > 0 { // up within the column
				m.focusAt(m.homeRegion, m.homeIndex-1)
				return
			}
			if m.regionCount(regionRecent) > 0 { // top of the column → up into the strip
				m.homeLastCol = m.homeRegion
				m.focusAt(regionRecent, 0)
			}
			return
		}
		// left/right across the visible columns
		cols := m.visibleCols()
		cur := indexOfRegion(cols, m.homeRegion)
		if cur < 0 {
			cur = 0
		}
		for nxt := cur + dx; nxt >= 0 && nxt < len(cols); nxt += dx {
			if m.regionCount(cols[nxt]) > 0 {
				m.focusAt(cols[nxt], m.indexIn(cols[nxt]))
				return
			}
		}
		return
	}
}

func indexOfRegion(cols []homeRegion, r homeRegion) int {
	for i, c := range cols {
		if c == r {
			return i
		}
	}
	return -1
}

// homeCycleRegion moves to the next/previous non-empty region.
func (m *model) homeCycleRegion(dir int) {
	order := m.visibleRegions()
	start := indexOfRegion(order, m.homeRegion)
	if start < 0 {
		start = 0
	}
	for i := 1; i <= len(order); i++ {
		r := order[((start+dir*i)%len(order)+len(order))%len(order)]
		if m.regionCount(r) > 0 {
			m.focusAt(r, m.indexIn(r))
			return
		}
	}
}

// --- rendering ---

func homeDim(s string) string { return lipgloss.NewStyle().Foreground(subtle).Render(s) }

func homeLabel(s string, sel bool) string {
	if sel {
		return selectedStyle.Render(s)
	}
	return s
}

// homeWindowOffset returns a scroll offset so the active row stays visible in h rows.
func homeWindowOffset(total, active, h int) int {
	if total <= h || h <= 0 {
		return 0
	}
	off := active - h/2
	if off < 0 {
		off = 0
	}
	if off > total-h {
		off = total - h
	}
	return off
}

// libraryColumn builds the LIBRARY box (PROJECTS + FOLDERS sections) + cells (≤ h rows).
func (m model) libraryColumn(h int) ([]string, []innerCell) {
	_, _, projects, folders, other, _ := homeGroups(m.homeItems)
	type lrow struct {
		header bool
		text   string
		libIdx int
	}
	var rows []lrow
	idx := 0
	if len(projects) > 0 {
		rows = append(rows, lrow{header: true, text: "PROJECTS"})
		for _, p := range projects {
			rows = append(rows, lrow{text: "› " + p.label, libIdx: idx})
			idx++
		}
	}
	if len(folders) > 0 {
		rows = append(rows, lrow{header: true, text: "FOLDERS"})
		for _, f := range folders {
			rows = append(rows, lrow{text: "› " + f.label, libIdx: idx})
			idx++
		}
	}
	// The loose/Notes entry renders last, under its own OTHER header. Its label already
	// carries the "◦ " marker.
	if len(other) > 0 {
		rows = append(rows, lrow{header: true, text: "OTHER"})
		for _, o := range other {
			rows = append(rows, lrow{text: o.label, libIdx: idx})
			idx++
		}
	}
	if len(rows) == 0 {
		return []string{homeDim("no projects — + to create")}, nil
	}
	// Find the active row (the selected library item) and window around it.
	activeRow := 0
	for i, r := range rows {
		if !r.header && r.libIdx == m.librarySelected {
			activeRow = i
		}
	}
	off := homeWindowOffset(len(rows), activeRow, h)
	var lines []string
	var cells []innerCell
	for i := off; i < len(rows) && len(lines) < h; i++ {
		r := rows[i]
		if r.header {
			lines = append(lines, homeDim(r.text))
			continue
		}
		sel := m.homeRegion == regionLibrary && m.librarySelected == r.libIdx
		cells = append(cells, innerCell{regionLibrary, r.libIdx, len(lines), 0, lipgloss.Width(r.text)})
		lines = append(lines, homeLabel(r.text, sel))
	}
	return lines, cells
}

// filesColumn builds the FILES box: subfolders / ".." as one line each (drillable), documents as
// two lines (name+count, dim snippet) + cells.
func (m model) filesColumn(h, contentW int) ([]string, []innerCell) {
	if len(m.homeFiles) == 0 {
		return []string{homeDim("no files — ctrl+n for a doc")}, nil
	}
	active := 0
	if m.homeRegion == regionFiles {
		active = m.homeIndex
	}
	perView := h / 2
	if perView < 1 {
		perView = 1
	}
	off := homeWindowOffset(len(m.homeFiles), active, perView)
	var lines []string
	var cells []innerCell
	for i := off; i < len(m.homeFiles); i++ {
		f := m.homeFiles[i]
		sel := m.homeRegion == regionFiles && m.homeIndex == i
		if f.isDir {
			if len(lines)+1 > h {
				break
			}
			text := "▸ " + f.name + "/"
			if f.name == ".." {
				text = "‹ .."
			}
			text = ansi.Truncate(text, contentW, "…")
			cells = append(cells, innerCell{regionFiles, i, len(lines), 0, lipgloss.Width(text)})
			lines = append(lines, homeLabel(text, sel))
			continue
		}
		if len(lines)+2 > h {
			break
		}
		count := commafy(f.words)
		name := f.name
		maxName := contentW - lipgloss.Width(count) - 1
		if maxName < 1 {
			maxName = 1
		}
		name = ansi.Truncate(name, maxName, "…")
		gap := contentW - lipgloss.Width(name) - lipgloss.Width(count)
		if gap < 1 {
			gap = 1
		}
		nameLine := homeLabel(name, sel) + strings.Repeat(" ", gap) + homeDim(count)
		snip := homeDim(ansi.Truncate(f.snippet, contentW, "…"))
		row := len(lines)
		cells = append(cells,
			innerCell{regionFiles, i, row, 0, contentW},
			innerCell{regionFiles, i, row + 1, 0, contentW})
		lines = append(lines, nameLine, snip)
	}
	return lines, cells
}

// cycleSource advances the active source by dir (wrapping), skipping unreachable sources, then
// rebuilds the library. Stays put if no other reachable source exists.
func (m *model) cycleSource(dir int) {
	n := len(m.sources)
	if n <= 1 {
		return
	}
	for i := 1; i <= n; i++ {
		nxt := ((m.activeSource+dir*i)%n + n) % n
		if nxt == m.activeSource {
			break
		}
		if m.sources[nxt].reachable() {
			m.activeSource = nxt
			m.rebuildHome()
			m.librarySelected = 0
			m.recomputeHomeFiles()
			m.resetHomeSelection()
			m.status = "source: " + m.sources[nxt].Name
			return
		}
	}
}

// homeColumns returns the browse boxes to show (responsive) with their widths + titles. RECENT is
// no longer a column — it renders as a full-width strip above these (see recentStrip/homeContent).
func (m model) homeColumns() (regions []homeRegion, titles []string, widths []int) {
	libTitle := "LIBRARY"
	if len(m.sources) > 1 && m.activeSource >= 0 && m.activeSource < len(m.sources) {
		libTitle = "LIBRARY · " + m.sources[m.activeSource].Name + " ▾"
	}
	// Responsive widths: grow both columns with the terminal, capped, keeping a margin. The browse
	// block is centered by the caller, so we target a share of the width rather than filling it.
	const margin = 6
	avail := m.width - 2*margin - homeColGap
	filesW := min(max(avail*3/5, homeFilesBoxMin), homeFilesBoxMax) // FILES ~60% of the pair
	libW := min(max(avail-filesW, homeLibraryBoxMin), homeLibraryBoxMax)
	if libW+homeColGap+filesW <= m.width { // both fit
		return []homeRegion{regionLibrary, regionFiles}, []string{libTitle, "FILES"}, []int{libW, filesW}
	}
	// Too narrow for two columns → FILES only, as wide as fits.
	only := m.width - 2*margin
	if only < homeFilesBoxMin {
		only = m.width // ultra-narrow: fill the width
	}
	if only > homeFilesBoxMax {
		only = homeFilesBoxMax
	}
	return []homeRegion{regionFiles}, []string{"FILES"}, []int{only}
}

// pinnedStrip lays the pinned items horizontally across one full-width row (contentW columns),
// returning the inner line + click cells (relative to the strip content). Returns nil when
// there are no pinned items so the caller omits the strip entirely.
func (m model) pinnedStrip(contentW int) ([]string, []innerCell) {
	pins := m.pinnedItems()
	if len(pins) == 0 {
		return nil, nil
	}
	const sep = "   "
	label := func(i int) string { return pins[i].label } // already "★ name"

	// Window horizontally so the active pin is always rendered (and thus clickable).
	active := 0
	if m.homeRegion == regionPinned {
		active = clampIdx(m.homeIndex, len(pins))
	}
	start := active
	used := lipgloss.Width(label(active))
	for start > 0 {
		w := lipgloss.Width(sep) + lipgloss.Width(label(start-1))
		if used+w > contentW {
			break
		}
		used += w
		start--
	}

	var b strings.Builder
	var cells []innerCell
	col := 0
	for i := start; i < len(pins); i++ {
		lbl := label(i)
		if i == start {
			lbl = ansi.Truncate(lbl, contentW, "…") // guarantee the anchor renders even if over-wide
		} else {
			if col+lipgloss.Width(sep)+lipgloss.Width(lbl) > contentW {
				break // no room for this one → stop
			}
			b.WriteString(sep)
			col += lipgloss.Width(sep)
		}
		sel := m.homeRegion == regionPinned && m.homeIndex == i
		b.WriteString(homeLabel(lbl, sel))
		cells = append(cells, innerCell{regionPinned, i, 0, col, col + lipgloss.Width(lbl)})
		col += lipgloss.Width(lbl)
	}
	return []string{b.String()}, cells
}

// recentStrip lays the recent files horizontally across one full-width row (contentW columns),
// returning the inner line + click cells (relative to the strip content). Recents that don't fit
// are dropped from the right.
func (m model) recentStrip(contentW int) ([]string, []innerCell) {
	rec := m.recents()
	if len(rec) == 0 {
		return []string{homeDim("(no recent files)")}, nil
	}
	const sep = "   "
	label := func(i int) string { return "› " + rec[i].label }

	// Window horizontally so the active recent is always rendered (and thus clickable): pick the
	// smallest start index such that rec[start..active] fits in contentW, then fill rightward.
	active := 0
	if m.homeRegion == regionRecent {
		active = clampIdx(m.homeIndex, len(rec))
	}
	start := active
	used := lipgloss.Width(label(active))
	for start > 0 {
		w := lipgloss.Width(sep) + lipgloss.Width(label(start-1))
		if used+w > contentW {
			break
		}
		used += w
		start--
	}

	var b strings.Builder
	var cells []innerCell
	col := 0
	for i := start; i < len(rec); i++ {
		lbl := label(i)
		if i == start {
			lbl = ansi.Truncate(lbl, contentW, "…") // guarantee the anchor renders even if over-wide
		} else {
			if col+lipgloss.Width(sep)+lipgloss.Width(lbl) > contentW {
				break // no room for this one → stop
			}
			b.WriteString(sep)
			col += lipgloss.Width(sep)
		}
		sel := m.homeRegion == regionRecent && m.homeIndex == i
		b.WriteString(homeLabel(lbl, sel))
		cells = append(cells, innerCell{regionRecent, i, 0, col, col + lipgloss.Width(lbl)})
		col += lipgloss.Width(lbl)
	}
	return []string{b.String()}, cells
}

// homeContent assembles the centered logo, the framed columns, and the actions, plus the
// clickable cells in block-relative coords (render == hit-test).
func (m model) homeContent() (lines []string, cells []homeCell, blockW int) {
	regions, titles, widths := m.homeColumns()
	availH := m.height - 8
	if availH < 4 {
		availH = 4
	}

	type built struct {
		inner []string
		cells []innerCell
		w     int
	}
	cols := make([]built, len(regions))
	maxH := 1
	for i, r := range regions {
		contentW := widths[i] - 4
		var inner []string
		var ics []innerCell
		switch r {
		case regionLibrary:
			inner, ics = m.libraryColumn(availH - 2)
		default:
			inner, ics = m.filesColumn(availH-2, contentW)
		}
		cols[i] = built{inner, ics, widths[i]}
		if len(inner) > maxH {
			maxH = len(inner)
		}
	}
	colH := maxH + 2 // framed

	// blockW + each box's x origin.
	blockW = 0
	xorg := make([]int, len(cols))
	for i := range cols {
		xorg[i] = blockW
		blockW += cols[i].w
		if i < len(cols)-1 {
			blockW += homeColGap
		}
	}

	// Logo, centered over blockW.
	pad := func(s string) string {
		left := (blockW - lipgloss.Width(s)) / 2
		if left < 0 {
			left = 0
		}
		return strings.Repeat(" ", left) + s
	}
	for _, l := range strings.Split(bannerArt, "\n") {
		lines = append(lines, pad(bannerStyle.Render(l)))
	}
	lines = append(lines, "")

	// PINNED strip — rendered only when there are live pins, above the RECENT strip.
	if m.regionCount(regionPinned) > 0 {
		pinnedInner, pinnedCells := m.pinnedStrip(blockW - 4)
		pinnedTop := len(lines)
		pinnedBox := framedPanel("PINNED", strings.Join(pinnedInner, "\n"), blockW, 3, "")
		lines = append(lines, strings.Split(pinnedBox, "\n")...)
		for _, c := range pinnedCells {
			cells = append(cells, homeCell{
				region: c.region, index: c.index,
				row: pinnedTop + 1 + c.row, // +1 for the strip's top border
				x0:  2 + c.x0,              // +2 for "│ "
				x1:  2 + c.x1,
			})
		}
		lines = append(lines, "")
	}

	// RECENT strip — a full-width row above the LIBRARY/FILES columns.
	stripInner, stripCells := m.recentStrip(blockW - 4)
	stripTop := len(lines)
	strip := framedPanel("RECENT", strings.Join(stripInner, "\n"), blockW, 3, "")
	lines = append(lines, strings.Split(strip, "\n")...)
	for _, c := range stripCells {
		cells = append(cells, homeCell{
			region: c.region, index: c.index,
			row: stripTop + 1 + c.row, // +1 for the strip's top border
			x0:  2 + c.x0,             // +2 for "│ "
			x1:  2 + c.x1,
		})
	}
	lines = append(lines, "")

	boxTop := len(lines) // block row where the framed columns begin

	// Frame each column and join horizontally. LIBRARY/FILES carry an inline "+" (create).
	framed := make([]string, len(cols))
	for i := range cols {
		plus := ""
		if regions[i] == regionLibrary || regions[i] == regionFiles {
			plus = "+"
		}
		framed[i] = framedPanel(titles[i], strings.Join(cols[i].inner, "\n"), cols[i].w, colH, plus)
	}
	gap := strings.Repeat(" ", homeColGap)
	parts := make([]string, 0, len(framed)*2)
	for i, f := range framed {
		if i > 0 {
			parts = append(parts, gap)
		}
		parts = append(parts, f)
	}
	row := lipgloss.JoinHorizontal(lipgloss.Top, parts...)
	lines = append(lines, strings.Split(row, "\n")...)

	// Translate inner cells → block cells (box col +2 for "│ ", box row +1 for top border).
	for i := range cols {
		for _, c := range cols[i].cells {
			cells = append(cells, homeCell{
				region: c.region, index: c.index,
				row: boxTop + 1 + c.row,
				x0:  xorg[i] + 2 + c.x0,
				x1:  xorg[i] + 2 + c.x1,
			})
		}
		// The inline "+" sits on the panel's top border at column w-2 (framedPanel renders it
		// just before the ╮). Record it as a clickable cell for LIBRARY/FILES.
		if regions[i] == regionLibrary || regions[i] == regionFiles {
			cells = append(cells, homeCell{
				region: regions[i], index: homePlusIndex,
				row: boxTop, x0: xorg[i] + cols[i].w - 2, x1: xorg[i] + cols[i].w - 1,
			})
		}
	}

	// Actions, centered below.
	lines = append(lines, "")
	acts := m.actions()
	if len(acts) > 0 {
		labels := make([]string, len(acts))
		for i, a := range acts {
			labels[i] = a.label
		}
		// Lay the actions out on one centered row with gaps; record each cell.
		const asep = "    "
		full := strings.Join(labels, asep)
		left := (blockW - lipgloss.Width(full)) / 2
		if left < 0 {
			left = 0
		}
		arow := len(lines)
		col := left
		var b strings.Builder
		b.WriteString(strings.Repeat(" ", left))
		for i, a := range acts {
			if i > 0 {
				b.WriteString(asep)
				col += lipgloss.Width(asep)
			}
			sel := m.homeRegion == regionActions && m.homeIndex == i
			b.WriteString(homeLabel(a.label, sel))
			cells = append(cells, homeCell{regionActions, i, arow, col, col + lipgloss.Width(a.label)})
			col += lipgloss.Width(a.label)
		}
		lines = append(lines, b.String())
	}
	return lines, cells, blockW
}

func (m model) homeView() string {
	lines, _, blockW := m.homeContent()
	block := lipgloss.NewStyle().Width(blockW).Render(strings.Join(lines, "\n"))
	if m.addingSource {
		prompt := "add source ▸ " + m.nameInput.View()
		bottom := statusStyle.Width(m.width).Render(prompt)
		return lipgloss.JoinVertical(lipgloss.Left,
			lipgloss.Place(m.width, m.height-1, lipgloss.Center, lipgloss.Center, block),
			bottom,
		)
	}
	if m.confirmRemoveSource {
		bottom := statusStyle.Width(m.width).Align(lipgloss.Center).Render(m.status)
		return lipgloss.JoinVertical(lipgloss.Left,
			lipgloss.Place(m.width, m.height-1, lipgloss.Center, lipgloss.Center, block),
			bottom,
		)
	}
	hint := statusStyle.Width(m.width).Align(lipgloss.Center).Render("F1 · ?  keybindings")
	center := m.height - 1
	if m.freshWorkspace() {
		center -= 4 // reserve rows for the tagline + primer
	}
	view := lipgloss.JoinVertical(lipgloss.Left,
		lipgloss.Place(m.width, center, lipgloss.Center, lipgloss.Center, block),
	)
	if m.freshWorkspace() {
		tagline := lipgloss.NewStyle().Foreground(accent).Align(lipgloss.Center).Width(m.width).Render(
			"write a whole book in plain Markdown — chapters, outline, export")
		primer := lipgloss.NewStyle().Foreground(subtle).Align(lipgloss.Center).Width(m.width).Render(
			"manuscript  ordered chapters (a book)   ·   category  a plain folder of notes\n" +
				"+  new manuscript or folder   ·   ctrl+n  new doc   ·   open Demo/ to explore")
		view = lipgloss.JoinVertical(lipgloss.Left, view, tagline, primer)
	}
	return lipgloss.JoinVertical(lipgloss.Left, view, hint)
}

// freshWorkspace reports whether the library holds no user-created projects/folders — only the
// seeded Demo/ (or nothing). Drives the first-run primer that teaches okashi's model.
func (m model) freshWorkspace() bool {
	for _, it := range m.library() {
		if it.kind != homeProject && it.kind != homeFolder {
			continue
		}
		if filepath.Base(it.path) == "Demo" {
			continue
		}
		return false
	}
	return true
}

// homeItemAt maps an absolute screen (x,y) to a launch (region, index).
func (m model) homeItemAt(x, y int) (homeRegion, int, bool) {
	lines, cells, blockW := m.homeContent()
	xoff := (m.width - blockW) / 2
	yoff := (m.height - len(lines)) / 2
	if xoff < 0 {
		xoff = 0
	}
	if yoff < 0 {
		yoff = 0
	}
	cx, cy := x-xoff, y-yoff
	for _, c := range cells {
		if c.row == cy && cx >= c.x0 && cx < c.x1 {
			return c.region, c.index, true
		}
	}
	return 0, 0, false
}

// openHomeSelection acts on the focused launch item and enters writing mode.
func (m *model) openHomeSelection() tea.Cmd {
	switch m.homeRegion {
	case regionPinned:
		pins := m.pinnedItems()
		if m.homeIndex < len(pins) {
			m.files.SetDir(pins[m.homeIndex].path)
			m.focus = focusSidebar
			m.editor.Blur()
			m.enterWriting()
			m.layout()
		}
		return nil
	case regionRecent:
		rec := m.recents()
		if m.homeIndex < len(rec) {
			p := rec[m.homeIndex].path
			m.files.SetDir(filepath.Dir(p))
			m.loadFile(p)
			m.focus = focusEditor
			m.editor.Focus()
			m.enterWriting()
			m.layout()
		}
		return nil
	case regionFiles:
		if m.homeIndex < 0 || m.homeIndex >= len(m.homeFiles) {
			return nil
		}
		f := m.homeFiles[m.homeIndex]
		if f.isDir { // drill into the subfolder (or up via "..") — stay on the home
			m.homeFilesDir = f.path
			m.recomputeHomeFiles()
			m.homeIndex = 0
			return nil
		}
		m.files.SetDir(filepath.Dir(f.path))
		m.loadFile(f.path)
		m.focus = focusEditor
		m.editor.Focus()
		m.enterWriting()
		m.layout()
		return nil
	case regionLibrary:
		lib := m.library()
		if m.librarySelected < len(lib) {
			m.files.SetDir(lib[m.librarySelected].path)
			m.focus = focusSidebar
			m.editor.Blur()
			m.enterWriting()
			m.layout()
		}
		return nil
	default: // actions
		acts := m.actions()
		if m.homeIndex >= len(acts) {
			return nil
		}
		// Actions: Move files opens the standalone mover; Browse opens the sidebar.
		if acts[m.homeIndex].kind == homeMoveFiles {
			m.enterMoverStandalone()
			return nil
		}
		if acts[m.homeIndex].kind == homeOpenOther {
			m.files.SetDir(m.activeSourceRoot())
			m.focus = focusSidebar
			m.editor.Blur()
			m.enterWriting()
			m.layout()
		}
		return nil
	}
}

// confirmAddSource adds path as a folder source (if reachable), persists, switches to it, and
// rebuilds the library. Ignores an unreachable/duplicate path (addSource dedups by ID).
func (m *model) confirmAddSource(path string) {
	s := newFolderSource(path)
	if !s.reachable() {
		m.status = "not a folder: " + path
		return
	}
	before := len(m.sources)
	m.sources = addSource(m.sources, s)
	if len(m.sources) == before { // dedup: already present → just switch to it
		for i, e := range m.sources {
			if e.ID == s.ID {
				m.activeSource = i
			}
		}
	} else {
		m.activeSource = len(m.sources) - 1
	}
	_ = saveSources(sourcesPath(), m.sources)
	m.rebuildHome()
	m.librarySelected = 0
	m.recomputeHomeFiles()
	m.resetHomeSelection()
	m.status = "source added: " + s.Name
}

// removeActiveSource removes the active source if it is a folder source, persists, and returns
// to the primary. A no-op (with a status) when the active source is the primary.
func (m *model) removeActiveSource() {
	if m.activeSource < 0 || m.activeSource >= len(m.sources) {
		return
	}
	s := m.sources[m.activeSource]
	if s.Kind == sourceKindPrimary {
		m.status = "the primary source can't be removed"
		return
	}
	m.sources = removeSource(m.sources, s.ID)
	m.activeSource = 0
	_ = saveSources(sourcesPath(), m.sources)
	m.rebuildHome()
	m.librarySelected = 0
	m.recomputeHomeFiles()
	m.resetHomeSelection()
	m.status = "source removed: " + s.Name
}

// startCreate opens the name prompt in file or folder mode.
func (m *model) startCreate(folder bool) tea.Cmd {
	m.creatingFile = true
	m.creatingFolder = folder
	m.nameInput.SetValue("")
	m.nameInput.Focus()
	m.editor.Blur()
	m.layout()
	return textinput.Blink
}
