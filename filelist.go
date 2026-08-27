package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// renderIcon colors a glyph by its type, except on the selected row (where the
// selection style's foreground must win) or when the glyph carries no color
// (the plain/ascii set).
func renderIcon(g glyph, selected bool) string {
	if selected || g.color == "" {
		return g.ch
	}
	return lipgloss.NewStyle().Foreground(g.color).Render(g.ch)
}

type fileEntry struct {
	name         string
	isDir        bool
	isPartHeader bool   // a non-selectable Part title row; cursor movement skips it
	isScene      bool   // a standalone scene (manifest v3, chapterRef.scene) — distinct glyph
	isChildScene bool   // a scene row nested under an expanded multi-text chapter (fold state)
	parentFolder string // isChildScene only: the owning chapter's folder, for activate()
}

// filelist is a minimal, mouse-friendly file browser we fully own.
type filelist struct {
	dir        string
	root       string
	entries    []fileEntry
	view       manuscriptView // resolved structure of dir (chapters, loose, source)
	selected   int
	offset     int // index of the top visible row
	width      int
	height     int
	allowed    map[string]bool
	folded     map[string]bool // corkKey(chapterRef) → true if EXPANDED (absence = collapsed)
	foldedRoot string          // manuscript dir folded was loaded for; "" when not a manuscript
	icons      iconSet
	wc         *wordCountCache
}

// allowedDocExts is the single source of truth for which files count as editable
// documents — used by both the sidebar listing and the outline's readEntries so
// the two views of a folder never diverge. Never mutated.
var allowedDocExts = map[string]bool{
	".md": true, ".txt": true, ".wg": true, ".markdown": true,
}

func newFilelist() filelist {
	return filelist{
		width:   sidebarWidth - 2,
		height:  1,
		allowed: allowedDocExts,
		icons:   resolveIcons(),
		wc:      newWordCountCache(),
	}
}

// withinRoot reports whether dir is root or a descendant of it.
func withinRoot(dir, root string) bool {
	rel, err := filepath.Rel(root, dir)
	if err != nil {
		return false
	}
	return rel == "." || !strings.HasPrefix(rel, "..")
}

// partWordTotal sums the word count of every text in every chapter of p, using wc
// (same cache filelist already threads through chapterWords) — the number shown on
// a Part's header row.
func partWordTotal(dir string, p partRef, wc *wordCountCache) int {
	total := 0
	for _, ch := range p.chapters {
		for _, t := range ch.texts {
			total += wc.count(filepath.Join(dir, ch.folder, t.file))
		}
	}
	return total
}

// SetDir loads dir's entries (filtered, sorted dirs-first) and resets the cursor.
func (f *filelist) SetDir(dir string) {
	if f.root != "" && !withinRoot(dir, f.root) {
		dir = f.root
	}
	f.dir = dir
	f.entries = nil
	f.selected = 0
	f.offset = 0

	showParent := filepath.Dir(dir) != dir // not at filesystem root
	if f.root != "" {
		showParent = dir != f.root // confined: only below the workspace root
	}
	if showParent {
		f.entries = append(f.entries, fileEntry{name: "..", isDir: true})
	}

	items, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	var dirs, files []fileEntry
	for _, it := range items {
		name := it.Name()
		if strings.HasPrefix(name, ".") {
			continue // hidden
		}
		if it.IsDir() {
			dirs = append(dirs, fileEntry{name: name, isDir: true})
			continue
		}
		if f.allowed[strings.ToLower(filepath.Ext(name))] {
			files = append(files, fileEntry{name: name})
		}
	}
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].name < dirs[j].name })

	// Resolve the manuscript view once per SetDir; it becomes the single source
	// of chapter order/titles/membership for View(), sectionRow(), and callers.
	f.view = resolveManuscript(dir, files)

	if f.view.ordered() {
		if dir != f.foldedRoot {
			f.folded = loadFolded(dir)
			f.foldedRoot = dir
		}
	} else {
		f.folded = nil
		f.foldedRoot = ""
	}

	// Build the ordered entry list: dirs first, then chapters in view order
	// (flattened across parts — Part headers are the next plan's job), then loose.
	// A manifest (v2) chapter is represented in f.entries by its folder name, since
	// it is now a directory on disk, not a direct file. A LEGACY chapter (no
	// manifest, numeric-prefix fallback) has folder == "" — resolveManuscript
	// keeps those as single-text chapterRefs with the flat filename in
	// texts[0].file, so it's represented as a plain (non-dir) file entry instead,
	// exactly as it was on disk before v2.
	// A chapter's folder is a real directory on disk, so it would otherwise ALSO
	// show up in the raw dirs listing above — exclude it there; the chapter loop
	// below is its single source of truth for where it appears (in manifest order).
	var plainDirs []fileEntry
	for _, d := range dirs {
		if !isChapterOf(f.view, d.name) {
			plainDirs = append(plainDirs, d)
		}
	}
	f.entries = append(f.entries, plainDirs...)
	for _, p := range f.view.parts {
		if p.title != "" {
			label := p.title + "  " + commafy(partWordTotal(dir, p, f.wc)) + " m"
			f.entries = append(f.entries, fileEntry{name: label, isPartHeader: true})
		}
		for _, ch := range p.chapters {
			if ch.folder == "" {
				if len(ch.texts) > 0 {
					f.entries = append(f.entries, fileEntry{name: ch.texts[0].file, isScene: ch.scene})
				}
				continue
			}
			f.entries = append(f.entries, fileEntry{name: ch.folder, isDir: true})
			if len(ch.texts) >= 2 && f.folded[corkKey(ch)] {
				for _, t := range ch.texts {
					f.entries = append(f.entries, fileEntry{
						name:         t.file,
						isChildScene: true,
						parentFolder: ch.folder,
					})
				}
			}
		}
	}
	f.entries = append(f.entries, f.view.loose...)
}

// createRowSentinel is passed as editRow to View() to prepend a new-file input row.
const createRowSentinel = -2

// View renders the visible window of entries, highlighting the selection.
// editRow >= 0: render editField at that row index instead of the filename.
// editRow == createRowSentinel: prepend editField as a new row before the list.
// editRow == -1 (or any other negative): normal render.
func (f filelist) View(editRow int, editField string) string {
	if len(f.entries) == 0 && editRow != createRowSentinel {
		return lipgloss.NewStyle().Foreground(subtle).Render("(empty)")
	}
	end := f.offset + f.height
	if end > len(f.entries) {
		end = len(f.entries)
	}
	editRowStyle := lipgloss.NewStyle().Foreground(accent).Width(f.width)
	var b strings.Builder
	if editRow == createRowSentinel {
		b.WriteString(editRowStyle.Render(ansi.Truncate(" "+editField, f.width, "")))
		if end > f.offset {
			b.WriteByte('\n')
		}
	}
	for i := f.offset; i < end; i++ {
		e := f.entries[i]
		g := f.icons.iconFor(e)
		section := f.isChapterEntry(e)
		switch {
		case e.isPartHeader:
			row := lipgloss.NewStyle().Bold(true).Foreground(accent).Render(ansi.Truncate(e.name, f.width, "…"))
			b.WriteString(row)
		case editRow >= 0 && i == editRow:
			b.WriteString(editRowStyle.Render(ansi.Truncate(" "+editField, f.width, "")))
		case i == f.selected:
			var content string
			if section {
				content = f.sectionRow(e, false) // selected: count + icon plain
			} else {
				content = " " + renderIcon(g, true) + e.name
			}
			b.WriteString(selectedStyle.Width(f.width).Render(ansi.Truncate(content, f.width, "…")))
		case section:
			b.WriteString(f.sectionRow(e, true))
		case e.isDir:
			row := " " + renderIcon(g, false) + lipgloss.NewStyle().Foreground(accent).Render(e.name)
			b.WriteString(ansi.Truncate(row, f.width, "…"))
		default:
			ext := filepath.Ext(e.name)
			icon := " " + renderIcon(g, false)
			if ext != "" && lipgloss.Width(icon+e.name) <= f.width {
				stem := icon + strings.TrimSuffix(e.name, ext)
				b.WriteString(stem + lipgloss.NewStyle().Foreground(subtle).Render(ext))
			} else {
				b.WriteString(ansi.Truncate(icon+e.name, f.width, "…"))
			}
		}
		if i < end-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// isChapterEntry reports whether e (an entry from f.entries, as built by SetDir) represents a
// manuscript chapter — either a v2 chapter folder (matched by isChapterOf on e.name as a
// folder) or a legacy single-file chapter (matched by its flat filename against texts[0].file,
// since a legacy chapterRef always has folder == ""; isChapterOf can't see those by name). A
// standalone scene (e.isScene) shares the same folder=="" shape as a legacy flat-file chapter
// but is NOT a chapter — excluded explicitly so it gets its own render path.
func (f filelist) isChapterEntry(e fileEntry) bool {
	if e.isScene {
		return false
	}
	if e.isDir {
		return isChapterOf(f.view, e.name)
	}
	for _, p := range f.view.parts {
		for _, ch := range p.chapters {
			if ch.folder == "" && !ch.scene && len(ch.texts) > 0 && ch.texts[0].file == e.name {
				return true
			}
		}
	}
	return false
}

// sectionRow builds a width-f.width row for a manuscript section: gutter+icon+
// title on the left, the word count right-aligned. dimCount styles the count
// subtle (used for non-selected rows; the selected bar keeps it plain).
func (f filelist) sectionRow(e fileEntry, dimCount bool) string {
	n := f.chapterWords(e.name)
	count := commafy(n) + " m"
	g := f.icons.iconFor(e)
	left := " " + renderIcon(g, !dimCount) + f.chapterTitle(e.name)
	maxLeft := f.width - lipgloss.Width(count) - 1
	if maxLeft < 1 {
		maxLeft = 1
	}
	left = ansi.Truncate(left, maxLeft, "…")
	gap := f.width - lipgloss.Width(left) - lipgloss.Width(count)
	if gap < 1 {
		gap = 1
	}
	rendered := count
	if dimCount {
		rendered = lipgloss.NewStyle().Foreground(subtle).Render(count)
	}
	return left + strings.Repeat(" ", gap) + rendered
}

// chapterWords sums the word counts of every text in the chapter whose folder is
// name, looking it up from the resolved view. Falls back to a direct file count
// (via f.wc) for legacy/zero-view entries where name is actually a flat filename
// (chapterRef.folder == "" for a legacy chapter — see resolveManuscript).
func (f filelist) chapterWords(name string) int {
	for _, p := range f.view.parts {
		for _, ch := range p.chapters {
			if ch.folder != name {
				continue
			}
			total := 0
			for _, t := range ch.texts {
				total += f.wc.count(filepath.Join(f.dir, ch.folder, t.file))
			}
			return total
		}
	}
	if f.wc != nil {
		return f.wc.count(filepath.Join(f.dir, name))
	}
	return 0
}

// chapterTitle returns the display title for a chapter folder, looking it up
// from the resolved view. Also matches a standalone scene by filename (ch.scene entries
// have folder == "", so the folder comparison alone can't reach them — mirrored from
// isChapterEntry/chapterWords' own name-based fallback for folder-less entries). Falls
// back to sectionTitle (for zero-view or legacy entries where name is actually a flat
// filename).
func (f filelist) chapterTitle(name string) string {
	for _, p := range f.view.parts {
		for _, ch := range p.chapters {
			if ch.folder == name {
				return ch.title
			}
			if ch.scene && len(ch.texts) > 0 && ch.texts[0].file == name {
				return ch.title
			}
		}
	}
	return sectionTitle(name)
}

func (f *filelist) moveBy(n int) {
	if len(f.entries) == 0 {
		return
	}
	step := 1
	if n < 0 {
		step = -1
	}
	remaining := n
	if remaining < 0 {
		remaining = -remaining
	}
	pos := f.selected
	for remaining > 0 {
		next := pos + step
		if next < 0 || next >= len(f.entries) {
			break // hit an edge — stop, don't wrap, don't land past the list
		}
		pos = next
		if !f.entries[pos].isPartHeader {
			remaining--
		}
	}
	// pos may have landed on a header if every remaining entry in this direction
	// is a header (or the list ends in one) — walk further in the same direction
	// until a selectable entry is found, or give up and keep the prior selection.
	for pos >= 0 && pos < len(f.entries) && f.entries[pos].isPartHeader {
		next := pos + step
		if next < 0 || next >= len(f.entries) {
			pos = f.selected // no selectable entry this way — clamp back
			break
		}
		pos = next
	}
	if pos < 0 {
		pos = 0
	}
	if pos >= len(f.entries) {
		pos = len(f.entries) - 1
	}
	if f.entries[pos].isPartHeader {
		// Nothing selectable was reachable in the walk direction. f.selected
		// alone is not a safe fallback here — it can itself already be sitting
		// on a header (e.g. every entry in f.entries is a header), so fall back
		// to scanning the whole list for any selectable entry, independent of
		// direction or prior selection.
		pos = f.selected
		if f.entries[pos].isPartHeader {
			pos = -1
			for i := range f.entries {
				if !f.entries[i].isPartHeader {
					pos = i
					break
				}
			}
			if pos == -1 {
				// Fully degenerate case: every entry in f.entries is a header,
				// so no selectable index exists anywhere. Should never occur in
				// practice (a Part is never rendered with zero chapters/loose
				// files under it), but stay safe-by-construction: land on 0,
				// the documented fallback, rather than leaving pos undefined.
				pos = 0
			}
		}
	}
	f.selected = pos
	f.scrollIntoView()
}

func (f *filelist) scrollIntoView() {
	if f.selected < f.offset {
		f.offset = f.selected
	} else if f.height > 0 && f.selected >= f.offset+f.height {
		f.offset = f.selected - f.height + 1
	}
	if f.offset < 0 {
		f.offset = 0
	}
}

// selectRow sets the selection from a row index within the visible window. A click
// landing on a Part-header row selects the next selectable entry below it instead
// (headers are never selectable) — forward, matching moveBy's own default direction
// for an explicit position jump.
func (f *filelist) selectRow(visibleRow int) {
	if visibleRow < 0 {
		return
	}
	idx := f.offset + visibleRow
	if idx >= len(f.entries) {
		return
	}
	for idx < len(f.entries) && f.entries[idx].isPartHeader {
		idx++
	}
	if idx >= len(f.entries) {
		return // nothing selectable below the clicked header — leave selection as-is
	}
	f.selected = idx
}

// activateResult is what activate() found at the cursor: a plain-dir navigation, a
// fold-state toggle on a multi-text chapter, or nothing selectable, needs no further
// action from the caller (activateNone); a single-text chapter, a child-scene row, or
// an ordinary file is ready to open at path (activateFile); an empty chapter needs the
// caller to open the text picker to create its first scene (activateTextPicker).
type activateResult int

const (
	activateNone activateResult = iota
	activateFile
	activateTextPicker
)

// activate acts on the selected entry. A "..", a plain directory, or a Part-header
// row (guarded above by moveBy/selectRow, but defensively checked here too) navigates
// and returns activateNone. A v2 chapter folder with exactly one text returns its
// path with activateFile — unchanged behavior from before multi-text chapters
// existed. A v2 chapter folder with zero texts returns activateTextPicker (caller opens
// the picker to create the first scene). A v2 chapter folder with 2+ texts TOGGLES its
// sidebar fold state (persisted via saveFolded) and returns activateNone — the caller
// does nothing further; f.entries already reflects the new state. A child-scene row
// (isChildScene, only present when its chapter is expanded) returns its own file path
// with activateFile. Anything else (an ordinary file, or a legacy single-file chapter)
// returns its path with activateFile.
func (f *filelist) activate() (string, activateResult) {
	if len(f.entries) == 0 || f.selected >= len(f.entries) {
		return "", activateNone
	}
	e := f.entries[f.selected]
	if e.isPartHeader {
		return "", activateNone // defensive: moveBy/selectRow never select a header
	}
	if e.isDir {
		if isChapterOf(f.view, e.name) {
			ch := chapterByFolder(f.view, e.name)
			if len(ch.texts) == 1 {
				return filepath.Join(f.dir, ch.folder, ch.texts[0].file), activateFile
			}
			if len(ch.texts) == 0 {
				return "", activateTextPicker
			}
			// 2+ texts: toggle this chapter's fold state instead of opening a picker.
			key := corkKey(ch)
			if f.folded[key] {
				delete(f.folded, key)
			} else {
				if f.folded == nil {
					f.folded = map[string]bool{}
				}
				f.folded[key] = true
			}
			if err := saveFolded(f.foldedRoot, f.folded, foldChapterSet(f.foldedRoot)); err == nil {
				f.SetDir(f.dir) // rebuild f.entries with/without the child rows; also reloads
				// f.folded from what was just saved — harmless (same content) and keeps SetDir
				// as the single source of truth for entry construction.
			}
			return "", activateNone
		}
		if e.name == ".." {
			f.SetDir(filepath.Dir(f.dir))
		} else {
			f.SetDir(filepath.Join(f.dir, e.name))
		}
		return "", activateNone
	}
	if e.isChildScene {
		return filepath.Join(f.dir, e.parentFolder, e.name), activateFile
	}
	return filepath.Join(f.dir, e.name), activateFile
}

// chapterByFolder returns the chapterRef whose folder matches, across all parts. The
// caller (activate) only calls this after isChapterOf already confirmed a match, so
// the zero-value fallback is unreachable in practice.
func chapterByFolder(v manuscriptView, folder string) chapterRef {
	for _, p := range v.parts {
		for _, ch := range p.chapters {
			if ch.folder == folder {
				return ch
			}
		}
	}
	return chapterRef{}
}

// selectedEntryName returns the raw name of the selected entry (folder or file),
// or ok=false if nothing is selected. Used by enterTextPicker to recover which
// chapter folder the cursor was on.
func (f filelist) selectedEntryName() (string, bool) {
	if f.selected < 0 || f.selected >= len(f.entries) {
		return "", false
	}
	return f.entries[f.selected].name, true
}

// selectedFile returns the selected entry's path if it's a regular file (not a dir or "..").
func (f filelist) selectedFile() (string, bool) {
	if f.selected < 0 || f.selected >= len(f.entries) {
		return "", false
	}
	e := f.entries[f.selected]
	if e.isDir {
		return "", false
	}
	return filepath.Join(f.dir, e.name), true
}

// paneLabel is the file-pane header: "Fichiers" at the source root, the manuscript title
// for a manuscript (manifest or legacy), else the folder name for a category.
func (f filelist) paneLabel() string {
	if f.dir == "" || f.dir == f.root {
		return "Fichiers"
	}
	if f.view.ordered() {
		return f.view.title
	}
	return filepath.Base(f.dir)
}

// breadcrumb is the current path relative to the workspace root, e.g.
// "okashi" at the root or "okashi / Book Name" inside a project.
func (f filelist) breadcrumb() string {
	base := filepath.Base(f.root)
	rel, err := filepath.Rel(f.root, f.dir)
	if err != nil || rel == "." || rel == "" {
		return base
	}
	parts := strings.Split(rel, string(filepath.Separator))
	return base + " / " + strings.Join(parts, " / ")
}

// has reports whether an entry with the given name is currently listed.
func (f filelist) has(name string) bool {
	for _, e := range f.entries {
		if e.name == name {
			return true
		}
	}
	return false
}

// selectName moves the selection to the entry with the given name, if present.
func (f *filelist) selectName(name string) {
	for i, e := range f.entries {
		if e.name == name {
			f.selected = i
			f.scrollIntoView()
			return
		}
	}
}

type breadcrumbSeg struct {
	label string
	path  string
}

// segHit is a clickable column range [start,end) in the breadcrumb row.
type segHit struct {
	start, end int
	path       string
}

// breadcrumbSegments returns the segments from the workspace root (base name
// first) down to the current dir, each with its target path.
func (f filelist) breadcrumbSegments() []breadcrumbSeg {
	segs := []breadcrumbSeg{{label: filepath.Base(f.root), path: f.root}}
	rel, err := filepath.Rel(f.root, f.dir)
	if err == nil && rel != "." && rel != "" {
		cur := f.root
		for _, part := range strings.Split(rel, string(filepath.Separator)) {
			cur = filepath.Join(cur, part)
			segs = append(segs, breadcrumbSeg{label: part, path: cur})
		}
	}
	return segs
}

// breadcrumbBar renders the breadcrumb head-truncated to width, with a
// right-aligned "sel/total" indicator when the list overflows, and returns the
// clickable column ranges of the visible segments (the "…" is not clickable).
func (f filelist) breadcrumbBar(width int) (string, []segHit) {
	segs := f.breadcrumbSegments()

	ind := ""
	if f.height > 0 && len(f.entries) > f.height {
		ind = fmt.Sprintf("%d/%d", f.selected+1, len(f.entries))
	}
	avail := width
	if ind != "" {
		avail -= lipgloss.Width(ind) + 1
	}
	if avail < 1 {
		avail = 1
	}

	const sep = " / "
	labels := make([]string, len(segs))
	for i, s := range segs {
		labels[i] = s.label
	}

	var visible []breadcrumbSeg
	if lipgloss.Width(strings.Join(labels, sep)) <= avail {
		visible = segs
	} else {
		root := segs[0]
		used := lipgloss.Width(root.label) + lipgloss.Width(sep) + lipgloss.Width("…")
		var tail []breadcrumbSeg
		for i := len(segs) - 1; i >= 1; i-- {
			w := lipgloss.Width(sep) + lipgloss.Width(segs[i].label)
			if used+w > avail {
				break
			}
			used += w
			tail = append([]breadcrumbSeg{segs[i]}, tail...)
		}
		visible = append([]breadcrumbSeg{root, {label: "…", path: ""}}, tail...)
	}

	var b strings.Builder
	var hits []segHit
	col := 0
	for i, s := range visible {
		if i > 0 {
			b.WriteString(sep)
			col += lipgloss.Width(sep)
		}
		start := col
		b.WriteString(s.label)
		col += lipgloss.Width(s.label)
		if s.path != "" {
			hits = append(hits, segHit{start: start, end: col, path: s.path})
		}
	}
	left := b.String()

	if lipgloss.Width(left) > avail {
		left = ansi.Truncate(left, avail, "…")
		visibleW := lipgloss.Width(left)
		kept := hits[:0]
		for _, h := range hits {
			if h.end <= visibleW {
				kept = append(kept, h)
			}
		}
		hits = kept
	}

	if ind == "" {
		return left, hits
	}
	gap := width - lipgloss.Width(left) - lipgloss.Width(ind)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + ind, hits
}
