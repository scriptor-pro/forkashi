package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestFilelistReadsFiltersSorts(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "b.md"), "x")
	mustWrite(t, filepath.Join(dir, "a.txt"), "x")
	mustWrite(t, filepath.Join(dir, "skip.png"), "x") // wrong extension
	if err := os.Mkdir(filepath.Join(dir, "chapters"), 0o755); err != nil {
		t.Fatal(err)
	}

	f := newFilelist()
	f.SetDir(dir)

	var names []string
	for _, e := range f.entries {
		names = append(names, e.name)
	}
	// ".." first (temp dir has a parent), then dir, then files alpha; .png skipped.
	want := []string{"..", "chapters", "a.txt", "b.md"}
	if len(names) != len(want) {
		t.Fatalf("entries = %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("entries = %v, want %v", names, want)
		}
	}
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFilelistNavigationAndScroll(t *testing.T) {
	f := newFilelist()
	f.height = 3
	f.entries = []fileEntry{
		{name: ".."}, {name: "a"}, {name: "b"}, {name: "c"}, {name: "d"},
	}

	f.moveBy(-1) // clamp at 0
	if f.selected != 0 {
		t.Fatalf("selected = %d, want 0", f.selected)
	}
	f.moveBy(4) // to last; window should scroll
	if f.selected != 4 {
		t.Fatalf("selected = %d, want 4", f.selected)
	}
	if f.offset != 2 { // height 3 → window [2,3,4]
		t.Fatalf("offset = %d, want 2", f.offset)
	}
	f.moveBy(10) // clamp at last
	if f.selected != 4 {
		t.Fatalf("selected = %d, want 4 (clamped)", f.selected)
	}
}

func TestFilelistSelectRow(t *testing.T) {
	f := newFilelist()
	f.height = 3
	f.offset = 2
	f.entries = []fileEntry{
		{name: ".."}, {name: "a"}, {name: "b"}, {name: "c"}, {name: "d"},
	}
	f.selectRow(1) // offset 2 + row 1 = index 3
	if f.selected != 3 {
		t.Fatalf("selected = %d, want 3", f.selected)
	}
	f.selectRow(99) // out of range: ignored
	if f.selected != 3 {
		t.Fatalf("selected = %d, want 3 (unchanged)", f.selected)
	}
}

func TestFilelistActivate(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "note.md"), "x")
	f := newFilelist()
	f.SetDir(dir)

	// Select the file (entries: "..", "note.md") and activate it.
	f.selected = 1
	path, result := f.activate()
	if result != activateFile || path != filepath.Join(dir, "note.md") {
		t.Fatalf("activate file = (%q, %v), want (%q, activateFile)", path, result, filepath.Join(dir, "note.md"))
	}

	// Activating ".." navigates up and opens nothing.
	f.SetDir(dir)
	f.selected = 0
	if _, result := f.activate(); result != activateNone {
		t.Fatal("activating .. should not open a file")
	}
	if f.dir != filepath.Dir(dir) {
		t.Fatalf("after .. dir = %q, want %q", f.dir, filepath.Dir(dir))
	}
}

func TestFilelistHasAndSelectName(t *testing.T) {
	f := newFilelist()
	f.entries = []fileEntry{{name: ".."}, {name: "a.md"}, {name: "b.md"}}

	if !f.has("a.md") || f.has("nope.md") {
		t.Fatal("has() should report membership correctly")
	}
	f.selectName("b.md")
	if f.selected != 2 {
		t.Fatalf("selectName: selected = %d, want 2", f.selected)
	}
	f.selectName("missing") // no-op
	if f.selected != 2 {
		t.Fatalf("selectName(missing) should be a no-op, selected = %d", f.selected)
	}
}

func TestFilelistViewShowsIconsNoSlash(t *testing.T) {
	t.Setenv("OKASHI_ICONS", "plain")
	f := newFilelist()
	f.width = 20
	f.height = 5
	f.entries = []fileEntry{{name: "proj", isDir: true}, {name: "a.md"}}

	view := f.View(-1, "")
	if strings.Contains(view, "proj/") {
		t.Fatal("dir should not get a trailing slash (the icon conveys it)")
	}
	if !strings.Contains(view, "▸ proj") {
		t.Fatalf("plain folder icon missing; view=%q", view)
	}
}

func TestBreadcrumb(t *testing.T) {
	root := "/home/me/okashi"
	cases := []struct {
		dir  string
		want string
	}{
		{"/home/me/okashi", "okashi"},
		{"/home/me/okashi/Book Name", "okashi / Book Name"},
		{"/home/me/okashi/Essays/Drafts", "okashi / Essays / Drafts"},
	}
	for _, c := range cases {
		f := filelist{root: root, dir: c.dir}
		if got := f.breadcrumb(); got != c.want {
			t.Fatalf("breadcrumb(%q) = %q, want %q", c.dir, got, c.want)
		}
	}
}

func TestFilelistConfinedToRoot(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "novel")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	f := newFilelist()
	f.root = root

	// At root: no ".." entry.
	f.SetDir(root)
	if f.has("..") {
		t.Fatal("root should not show a .. entry")
	}
	// In a subdir: ".." present.
	f.SetDir(sub)
	if !f.has("..") {
		t.Fatal("subdir should show a .. entry")
	}
	// Trying to go above root clamps back to root.
	f.SetDir(filepath.Dir(root))
	if f.dir != root {
		t.Fatalf("navigating above root should clamp to root, got %q", f.dir)
	}
}

func TestFilelistGutterAndDimExtension(t *testing.T) {
	f := newFilelist()
	f.width = 29
	f.height = 5
	f.selected = -1 // nothing selected → file uses the dim-extension path
	f.entries = []fileEntry{{name: "chapter.md"}}

	view := f.View(-1, "")
	if !strings.HasPrefix(view, " ") {
		t.Fatal("rows should start with a one-column gutter")
	}
	wantExt := lipgloss.NewStyle().Foreground(subtle).Render(".md")
	if !strings.Contains(view, wantExt) {
		t.Fatal("a file extension should be dimmed with the subtle style")
	}
}

func TestBreadcrumbSegments(t *testing.T) {
	f := filelist{root: "/home/me/okashi", dir: "/home/me/okashi/Book/Drafts"}
	segs := f.breadcrumbSegments()
	want := []breadcrumbSeg{
		{"okashi", "/home/me/okashi"},
		{"Book", "/home/me/okashi/Book"},
		{"Drafts", "/home/me/okashi/Book/Drafts"},
	}
	if len(segs) != len(want) {
		t.Fatalf("got %d segments, want %d: %+v", len(segs), len(want), segs)
	}
	for i := range want {
		if segs[i] != want[i] {
			t.Fatalf("segment %d = %+v, want %+v", i, segs[i], want[i])
		}
	}
}

func TestPaneIconColoredWhenNotSelected(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(2) // force ANSI256 so styles emit codes in tests
	defer lipgloss.SetColorProfile(old)

	g := glyph{ch: "X", color: iconPdfColor}
	if renderIcon(g, false) == "X" {
		t.Fatal("non-selected icon should be color-wrapped (ANSI), got the bare glyph")
	}
	// Selected → plain glyph so the selection bar's white foreground applies.
	if renderIcon(g, true) != "X" {
		t.Fatalf("selected icon should be the bare glyph, got %q", renderIcon(g, true))
	}
	// No color → bare glyph (plain icon set).
	if renderIcon(glyph{ch: "Y"}, false) != "Y" {
		t.Fatalf("uncolored glyph should render bare, got %q", renderIcon(glyph{ch: "Y"}, false))
	}
}

func TestBreadcrumbBarFitsWithHits(t *testing.T) {
	f := filelist{root: "/r/okashi", dir: "/r/okashi/Book", height: 5}
	f.entries = []fileEntry{{name: "a"}, {name: "b"}} // 2 < height → no indicator
	row, hits := f.breadcrumbBar(40)
	if !strings.Contains(row, "okashi / Book") {
		t.Fatalf("row = %q", row)
	}
	if len(hits) != 2 {
		t.Fatalf("want 2 clickable segments, got %d", len(hits))
	}
	// The "Book" hit's column range should contain its rune.
	if hits[1].path != "/r/okashi/Book" || hits[1].start >= hits[1].end {
		t.Fatalf("bad hit %+v", hits[1])
	}
}

func TestBreadcrumbBarIndicator(t *testing.T) {
	f := filelist{root: "/r/okashi", dir: "/r/okashi", height: 2}
	f.selected = 2
	f.entries = make([]fileEntry, 10) // 10 > height 2 → indicator
	row, _ := f.breadcrumbBar(40)
	if !strings.Contains(row, "3/10") {
		t.Fatalf("expected scroll indicator 3/10, row = %q", row)
	}
}

func TestBreadcrumbBarNeverOverflows(t *testing.T) {
	root := "/x/this-is-a-very-long-workspace-folder-name"
	f := filelist{root: root, dir: filepath.Join(root, "Sub", "Deeper"), height: 5}
	row, _ := f.breadcrumbBar(29)
	if lipgloss.Width(row) > 29 {
		t.Fatalf("breadcrumb row width %d exceeds budget 29: %q", lipgloss.Width(row), row)
	}
}

func TestSidebarShowsTitlesAndCounts(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-opening.md"), []byte("one two three"), 0o644)
	os.WriteFile(filepath.Join(dir, "02-the-letter.md"), []byte("a b"), 0o644)
	f := newFilelist()
	f.root = ""
	f.width = 29
	f.height = 10
	f.SetDir(dir)

	view := f.View(-1, "")
	if !strings.Contains(view, "opening") || strings.Contains(view, "01-opening") {
		t.Fatalf("manuscript pane should show stripped title 'opening', not raw filename:\n%s", view)
	}
	if !strings.Contains(view, "3 m") {
		t.Fatalf("manuscript pane should show the section word count '3 m':\n%s", view)
	}
}

func TestSidebarRendersManifestTitleAndOrder(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "opening"), 0o755)
	os.MkdirAll(filepath.Join(dir, "the-letter"), 0o755)
	os.WriteFile(filepath.Join(dir, "opening", "opening.md"), []byte("one two three"), 0o644)
	os.WriteFile(filepath.Join(dir, "the-letter", "the-letter.md"), []byte("a b"), 0o644)
	os.WriteFile(filepath.Join(dir, manifestName), []byte(
		`{"schemaVersion":3,"title":"Windermere","items":[`+
			`{"chapter":{"folder":"the-letter","title":"The Letter","texts":[{"file":"the-letter.md","title":"The Letter"}]}},`+
			`{"chapter":{"folder":"opening","title":"Chapter One","texts":[{"file":"opening.md","title":"Chapter One"}]}}]}`), 0o644)
	f := newFilelist()
	f.root = ""
	f.width, f.height = 60, 12
	f.SetDir(dir)
	view := f.View(-1, "")
	if !strings.Contains(view, "The Letter") || !strings.Contains(view, "Chapter One") {
		t.Fatalf("sidebar should show manifest titles:\n%s", view)
	}
	// Manifest order: "The Letter" precedes "Chapter One" despite filename alpha.
	if strings.Index(view, "The Letter") > strings.Index(view, "Chapter One") {
		t.Fatalf("sidebar must honor manifest order, not filename order:\n%s", view)
	}
}

func TestSidebarOrdersSectionsNumerically(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "10-ten.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, "2-two.md"), []byte("x"), 0o644)
	f := newFilelist()
	f.root = ""
	f.width = 29
	f.height = 10
	f.SetDir(dir)

	var names []string
	for _, e := range f.entries {
		if !e.isDir {
			names = append(names, e.name)
		}
	}
	if strings.Join(names, ",") != "2-two.md,10-ten.md" {
		t.Fatalf("sections should sort numerically (2 before 10): %v", names)
	}
}

func TestFileListInlineEditRow(t *testing.T) {
	t.Setenv("OKASHI_ICONS", "plain")
	f := filelist{width: 20, height: 10, icons: resolveIcons()}
	f.entries = []fileEntry{{name: "alpha.md"}, {name: "bravo.md"}}
	f.selected = 1
	out := ansi.Strip(f.View(1, "EDITING_HERE"))
	if !strings.Contains(out, "EDITING_HERE") {
		t.Fatalf("editRow should render the field:\n%s", out)
	}
	if strings.Contains(out, "bravo.md") {
		t.Fatalf("the edited row should show the field, not the filename:\n%s", out)
	}
	// Normal render (editRow -1) is unchanged.
	if !strings.Contains(ansi.Strip(f.View(-1, "")), "bravo.md") {
		t.Fatal("normal render should show filenames")
	}
}

func TestFileListInlineCreateRow(t *testing.T) {
	t.Setenv("OKASHI_ICONS", "plain")
	f := filelist{width: 20, height: 10, icons: resolveIcons()}
	f.entries = []fileEntry{{name: "alpha.md"}}
	out := ansi.Strip(f.View(createRowSentinel, "NEWFILE"))
	if !strings.Contains(out, "NEWFILE") || !strings.Contains(out, "alpha.md") {
		t.Fatalf("create row should add the field AND keep existing entries:\n%s", out)
	}
}

func TestMoveByDownSkipsPartHeader(t *testing.T) {
	f := newFilelist()
	f.entries = []fileEntry{
		{name: "chap-un", isDir: true},
		{name: "Partie Deux", isPartHeader: true},
		{name: "chap-deux", isDir: true},
	}
	f.selected = 0
	f.moveBy(1)
	if f.selected != 2 {
		t.Fatalf("moveBy(1) from index 0 must skip the header at index 1 and land on 2, got %d", f.selected)
	}
}

func TestMoveByUpSkipsPartHeader(t *testing.T) {
	f := newFilelist()
	f.entries = []fileEntry{
		{name: "chap-un", isDir: true},
		{name: "Partie Deux", isPartHeader: true},
		{name: "chap-deux", isDir: true},
	}
	f.selected = 2
	f.moveBy(-1)
	if f.selected != 0 {
		t.Fatalf("moveBy(-1) from index 2 must skip the header at index 1 and land on 0, got %d", f.selected)
	}
}

func TestMoveByStopsAtEndWithoutInfiniteLoopWhenTrailingHeaders(t *testing.T) {
	f := newFilelist()
	f.entries = []fileEntry{
		{name: "chap-un", isDir: true},
		{name: "Partie Deux", isPartHeader: true},
	}
	f.selected = 0
	f.moveBy(1)
	// No selectable entry below index 0 other than the header — selection must clamp,
	// never land on a header, never loop forever.
	if f.selected != 0 {
		t.Fatalf("moveBy(1) with only a trailing header must clamp back to the last selectable index, got %d", f.selected)
	}
}

// TestMoveByDegenerateAllHeadersFallsBackToDocumentedSafeIndex covers the fully
// degenerate case the review flagged: every entry in f.entries is a Part header,
// so there is no selectable index anywhere in the list for moveBy to land on.
// This should never occur in practice (a Part is never rendered with zero
// chapters/loose files under it), but moveBy must stay safe by construction: it
// must land on the documented fallback (index 0), not on whatever index the
// walk/clamp logic happens to leave pos at.
//
// This is distinct from — and does not contradict — the invariant "never select
// a header when a selectable entry exists" exercised by the other TestMoveBy*
// tests above: here no selectable entry exists anywhere, so the only thing left
// to verify is that the fallback is the deterministic, documented one.
func TestMoveByDegenerateAllHeadersFallsBackToDocumentedSafeIndex(t *testing.T) {
	f := newFilelist()
	f.entries = []fileEntry{
		{name: "Partie Un", isPartHeader: true},
		{name: "Partie Deux", isPartHeader: true},
		{name: "Partie Trois", isPartHeader: true},
	}

	f.selected = 0
	f.moveBy(1)
	if f.selected != 0 {
		t.Fatalf("moveBy(1) on an all-header list must fall back to the documented safe index 0, got %d", f.selected)
	}

	f.selected = 0
	f.moveBy(-1)
	if f.selected != 0 {
		t.Fatalf("moveBy(-1) on an all-header list must fall back to the documented safe index 0, got %d", f.selected)
	}

	f.selected = 2
	f.moveBy(1)
	if f.selected != 0 {
		t.Fatalf("moveBy(1) from the last index on an all-header list must fall back to the documented safe index 0, got %d", f.selected)
	}

	f.selected = 2
	f.moveBy(-1)
	if f.selected != 0 {
		t.Fatalf("moveBy(-1) from the last index on an all-header list must fall back to the documented safe index 0, got %d", f.selected)
	}
}

// TestMoveByFallsBackToSelectableEntryElsewhereWhenHeadersSurroundSelection
// covers the intermediate degenerate case: f.selected itself sits on a header
// (which moveBy's normal walk never produces on its own, but structure-mode
// edits or a future caller could), and no selectable entry exists in the walk
// direction — but one does exist elsewhere in the list. moveBy must fold to it
// rather than settling for the header at f.selected, which the old "last-resort
// guard: pos = f.selected" fallback used to do.
func TestMoveByFallsBackToSelectableEntryElsewhereWhenHeadersSurroundSelection(t *testing.T) {
	f := newFilelist()
	f.entries = []fileEntry{
		{name: "chap-un", isDir: true},
		{name: "Partie Un", isPartHeader: true},
		{name: "Partie Deux", isPartHeader: true},
	}
	f.selected = 1 // starts on a header — abnormal, but must still resolve safely
	f.moveBy(1)    // no selectable entry forward (index 2 is also a header)
	if f.entries[f.selected].isPartHeader {
		t.Fatalf("moveBy(1) must never leave f.selected on a header when a selectable entry exists elsewhere in the list, got index %d", f.selected)
	}
	if f.selected != 0 {
		t.Fatalf("moveBy(1) must fold back to the only selectable entry (index 0), got %d", f.selected)
	}
}

func TestSelectRowSkipsPartHeaderForward(t *testing.T) {
	f := newFilelist()
	f.entries = []fileEntry{
		{name: "chap-un", isDir: true},
		{name: "Partie Deux", isPartHeader: true},
		{name: "chap-deux", isDir: true},
	}
	f.height = 10
	f.selectRow(1) // clicking directly on the header row
	if f.selected != 2 {
		t.Fatalf("clicking a header row must select the next selectable entry (forward), got %d", f.selected)
	}
}

func TestPaneLabel(t *testing.T) {
	root := t.TempDir()
	// A manuscript whose title differs from its folder name.
	proj := filepath.Join(root, "novel-dir")
	if _, err := createManuscript(proj, "The Real Title", "Untitled"); err != nil {
		t.Fatal(err)
	}
	// A plain category folder.
	cat := filepath.Join(root, "research")
	os.MkdirAll(cat, 0o755)

	f := newFilelist()
	f.root = root

	f.SetDir(root)
	if got := f.paneLabel(); got != "Fichiers" {
		t.Fatalf("root paneLabel = %q, want Fichiers", got)
	}
	f.SetDir(proj)
	if got := f.paneLabel(); got != "The Real Title" {
		t.Fatalf("manuscript paneLabel = %q, want the manifest title", got)
	}
	f.SetDir(cat)
	if got := f.paneLabel(); got != "research" {
		t.Fatalf("category paneLabel = %q, want the folder name", got)
	}
}

func TestSidebarShowsPartHeaderWithWordTotal(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "the-letter"), 0o755)
	os.WriteFile(filepath.Join(dir, "the-letter", "the-letter.md"), []byte("one two three four five"), 0o644)
	os.WriteFile(filepath.Join(dir, manifestName), []byte(
		`{"schemaVersion":3,"title":"Windermere","items":[`+
			`{"part":"Part One","chapters":[`+
			`{"folder":"the-letter","title":"The Letter","texts":[{"file":"the-letter.md","title":"The Letter"}]}]}]}`), 0o644)
	f := newFilelist()
	f.root = ""
	f.width, f.height = 60, 12
	f.SetDir(dir)
	view := f.View(-1, "")
	if !strings.Contains(view, "Part One") {
		t.Fatalf("sidebar must show the Part title, got:\n%s", view)
	}
	if !strings.Contains(view, "5 m") {
		t.Fatalf("sidebar's Part header must show the total word count of its chapters, got:\n%s", view)
	}
}

func TestSidebarOmitsHeaderForSyntheticUntitledPart(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "opening"), 0o755)
	os.WriteFile(filepath.Join(dir, "opening", "opening.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, manifestName), []byte(
		`{"schemaVersion":3,"title":"N","items":[`+
			`{"chapter":{"folder":"opening","title":"Opening","texts":[{"file":"opening.md","title":"Opening"}]}}]}`), 0o644)
	f := newFilelist()
	f.root = ""
	f.width, f.height = 60, 12
	f.SetDir(dir)
	for _, e := range f.entries {
		if e.isPartHeader {
			t.Fatalf("a manuscript with no real Part must not render any header row, got entries: %+v", f.entries)
		}
	}
}

func TestSidebarShowsMixedBareChaptersAndPart(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "prologue"), 0o755)
	os.MkdirAll(filepath.Join(dir, "the-letter"), 0o755)
	os.WriteFile(filepath.Join(dir, "prologue", "prologue.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, "the-letter", "the-letter.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, manifestName), []byte(
		`{"schemaVersion":3,"title":"N","items":[`+
			`{"chapter":{"folder":"prologue","title":"Prologue","texts":[{"file":"prologue.md","title":"Prologue"}]}},`+
			`{"part":"Part One","chapters":[`+
			`{"folder":"the-letter","title":"The Letter","texts":[{"file":"the-letter.md","title":"The Letter"}]}]}]}`), 0o644)
	f := newFilelist()
	f.root = ""
	f.width, f.height = 60, 12
	f.SetDir(dir)
	view := f.View(-1, "")
	iPrologue := strings.Index(view, "Prologue")
	iPartOne := strings.Index(view, "Part One")
	iLetter := strings.Index(view, "The Letter")
	if iPrologue == -1 || iPartOne == -1 || iLetter == -1 {
		t.Fatalf("all three must appear, got:\n%s", view)
	}
	if !(iPrologue < iPartOne && iPartOne < iLetter) {
		t.Fatalf("order must be Prologue (bare), then Part One header, then The Letter, got:\n%s", view)
	}
}

// TestSidebarEmptyPartHeaderIsSkippedByRealCursorMovement exercises Task 1's
// header-skipping moveBy/selectRow against f.entries as SetDir actually builds
// them (not a hand-built f.entries slice, which is all the Task 1 tests used) —
// specifically for a Part declared in the manifest with zero resolvable
// chapters under it (its folder listed in "chapters" doesn't exist on disk), so
// its header row exists but is immediately followed by the next entry with no
// selectable row in between. This is the real-world shape of the "list ends in
// a header" / "header immediately followed by another entry" cases Task 1's
// moveBy fallback was built for, now wired through the real manifest → SetDir
// → f.entries path instead of a synthetic f.entries literal.
func TestSidebarEmptyPartHeaderIsSkippedByRealCursorMovement(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "the-letter"), 0o755)
	os.WriteFile(filepath.Join(dir, "the-letter", "the-letter.md"), []byte("one two"), 0o644)
	// "Empty Part" lists a chapter folder that does not exist on disk, so it
	// resolves to zero chapters (manifestView's resolve() skips it) — the Part
	// header still renders (its title is non-empty), but nothing follows it
	// until the next Part's header.
	os.WriteFile(filepath.Join(dir, manifestName), []byte(
		`{"schemaVersion":3,"title":"N","items":[`+
			`{"part":"Empty Part","chapters":[`+
			`{"folder":"missing","title":"Ghost","texts":[{"file":"missing.md","title":"Ghost"}]}]},`+
			`{"part":"Part Two","chapters":[`+
			`{"folder":"the-letter","title":"The Letter","texts":[{"file":"the-letter.md","title":"The Letter"}]}]}]}`), 0o644)
	f := newFilelist()
	f.root = ""
	f.width, f.height = 60, 12
	f.SetDir(dir)

	// Confirm the shape we're testing actually occurred: two header rows back
	// to back (or header then non-header), never two adjacent selectable rows
	// mistaken for the empty-part case — i.e. "Empty Part" really has nothing
	// selectable between it and "Part Two".
	var headerIdx, secondHeaderIdx = -1, -1
	for i, e := range f.entries {
		if e.isPartHeader && strings.Contains(e.name, "Empty Part") {
			headerIdx = i
		}
		if e.isPartHeader && strings.Contains(e.name, "Part Two") {
			secondHeaderIdx = i
		}
	}
	if headerIdx == -1 || secondHeaderIdx == -1 {
		t.Fatalf("expected both Part headers in f.entries, got: %+v", f.entries)
	}
	if secondHeaderIdx != headerIdx+1 {
		t.Fatalf("Empty Part's header must be immediately followed by Part Two's header (no chapters in between), got entries: %+v", f.entries)
	}

	// Land the cursor right before the run of two headers and move down: it
	// must skip both and land on "the-letter", never rest on a header.
	f.selected = headerIdx - 1
	if f.selected < 0 || f.entries[f.selected].isPartHeader {
		t.Fatalf("test setup invalid: index before the header run must be a real selectable entry, got %+v at %d", f.entries, headerIdx-1)
	}
	f.moveBy(1)
	if f.entries[f.selected].isPartHeader {
		t.Fatalf("moveBy(1) must never land on a header, even across two adjacent real headers, got selected=%d entries=%+v", f.selected, f.entries)
	}
	if f.entries[f.selected].name != "the-letter" {
		t.Fatalf("moveBy(1) must skip both headers and land on the-letter, got %q", f.entries[f.selected].name)
	}

	// Clicking directly on the empty Part's header row must also resolve
	// forward, through the second header, to the same selectable chapter.
	f.selected = 0
	f.offset = 0
	f.selectRow(headerIdx)
	if f.entries[f.selected].isPartHeader {
		t.Fatalf("selectRow on a header must never leave selection on a header, got selected=%d entries=%+v", f.selected, f.entries)
	}
	if f.entries[f.selected].name != "the-letter" {
		t.Fatalf("selectRow on Empty Part's header must resolve forward past both headers to the-letter, got %q", f.entries[f.selected].name)
	}
}

func TestActivateSingleTextChapterTogglesFold(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "opening"), 0o755)
	os.WriteFile(filepath.Join(dir, "opening", "opening.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, manifestName), []byte(
		`{"schemaVersion":3,"title":"N","items":[`+
			`{"chapter":{"folder":"opening","title":"Opening","texts":[{"file":"opening.md","title":"Opening"}]}}]}`), 0o644)
	f := newFilelist()
	f.root = ""
	f.width, f.height = 60, 12
	f.SetDir(dir)
	f.selectName("opening")
	path, result := f.activate()
	if result != activateNone || path != "" {
		t.Fatalf("a single-text chapter must toggle child scene rows, got path=%q result=%v", path, result)
	}
	found := false
	for _, e := range f.entries {
		if e.isChildScene && e.parentFolder == "opening" && e.name == "opening.md" {
			found = true
		}
	}
	if !found {
		t.Fatalf("single-scene chapter should expand to its scene row, entries=%+v", f.entries)
	}
}

func TestActivateMultiTextChapterSignalsPicker(t *testing.T) {
	// Superseded by the sidebar scene tree (2026-08-27): activating a multi-text chapter now
	// toggles its fold state inline (see TestActivateMultiTextChapterTogglesFoldInsteadOfPicker)
	// instead of routing to screenTextPicker. This test keeps the original scenario/name as a
	// regression guard on the NEW contract, so a future reader searching for this test name
	// still finds the current expected behavior.
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "chapitre-un"), 0o755)
	os.WriteFile(filepath.Join(dir, "chapitre-un", "scene-un.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, "chapitre-un", "scene-deux.md"), []byte("y"), 0o644)
	os.WriteFile(filepath.Join(dir, manifestName), []byte(
		`{"schemaVersion":3,"title":"N","items":[`+
			`{"chapter":{"folder":"chapitre-un","title":"Chapitre Un","texts":[`+
			`{"file":"scene-un.md","title":"Scène Un"},{"file":"scene-deux.md","title":"Scène Deux"}]}}]}`), 0o644)
	f := newFilelist()
	f.root = ""
	f.width, f.height = 60, 12
	f.SetDir(dir)
	f.selectName("chapitre-un")
	_, result := f.activate()
	if result != activateNone {
		t.Fatalf("a multi-text chapter must now toggle its fold state (activateNone), got %v", result)
	}
	if !f.folded["chapitre-un"] {
		t.Fatal("activate must have expanded the chapter")
	}
}

func TestActivatePlainFolderStillNavigates(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "notes"), 0o755)
	f := newFilelist()
	f.root = ""
	f.width, f.height = 60, 12
	f.SetDir(dir)
	f.selectName("notes")
	_, result := f.activate()
	if result != activateNone {
		t.Fatalf("a plain (non-chapter) folder must still navigate (activateNone), got %v", result)
	}
	if f.dir != filepath.Join(dir, "notes") {
		t.Fatalf("navigating into a plain folder must update f.dir, got %q", f.dir)
	}
}

func TestActivateEmptyChapterSignalsPickerWithNoTexts(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "vide"), 0o755)
	os.WriteFile(filepath.Join(dir, manifestName), []byte(
		`{"schemaVersion":3,"title":"N","items":[`+
			`{"chapter":{"folder":"vide","title":"Vide","texts":[]}}]}`), 0o644)
	f := newFilelist()
	f.root = ""
	f.width, f.height = 60, 12
	f.SetDir(dir)
	f.selectName("vide")
	_, result := f.activate()
	if result != activateTextPicker {
		t.Fatalf("an empty chapter must also route to the picker (which shows an empty state), not crash or navigate, got %v", result)
	}
}

func TestSetDirMarksStandaloneSceneEntry(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "aparte.md"), []byte("x"), 0o644)
	writeManifestRaw(t, dir, `{"schemaVersion":3,"title":"N","items":[
		{"chapter":{"title":"Aparté","scene":true,"texts":[{"file":"aparte.md","title":"Aparté"}]}}
	]}`)
	f := newFilelist()
	f.SetDir(dir)
	var found bool
	for _, e := range f.entries {
		if e.name == "aparte.md" {
			found = true
			if !e.isScene {
				t.Fatalf("standalone scene entry must have isScene=true, got %+v", e)
			}
		}
	}
	if !found {
		t.Fatal("aparte.md must appear in f.entries")
	}
}

func TestSetDirLegacyFlatChapterIsNotMarkedScene(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-a.md"), []byte("x"), 0o644)
	f := newFilelist()
	f.SetDir(dir)
	for _, e := range f.entries {
		if e.name == "01-a.md" && e.isScene {
			t.Fatalf("a legacy flat-file chapter must NOT be marked isScene, got %+v", e)
		}
	}
}

func TestIsChapterEntryFalseForStandaloneScene(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "aparte.md"), []byte("x"), 0o644)
	writeManifestRaw(t, dir, `{"schemaVersion":3,"title":"N","items":[
		{"chapter":{"title":"Aparté","scene":true,"texts":[{"file":"aparte.md","title":"Aparté"}]}}
	]}`)
	f := newFilelist()
	f.SetDir(dir)
	var entry fileEntry
	for _, e := range f.entries {
		if e.name == "aparte.md" {
			entry = e
		}
	}
	if f.isChapterEntry(entry) {
		t.Fatal("a standalone scene must not be reported as a chapter entry (it has its own icon/behavior, not the legacy-chapter path)")
	}
}

func TestSetDirCollapsedChapterHidesChildScenes(t *testing.T) {
	dir := t.TempDir()
	mkChapterDir(t, dir, "opening", map[string]string{
		"opening.md":  "hello",
		"opening2.md": "world",
	})
	writeManifestRaw(t, dir, `{"schemaVersion":3,"title":"N","items":[
		{"chapter":{"folder":"opening","title":"Opening","texts":[
			{"file":"opening.md","title":"Part One"},
			{"file":"opening2.md","title":"Part Two"}
		]}}
	]}`)
	f := newFilelist()
	f.SetDir(dir)
	for _, e := range f.entries {
		if e.isChildScene {
			t.Fatalf("collapsed chapter must not have child-scene rows, got entries=%+v", f.entries)
		}
	}
}

func TestSetDirExpandedChapterShowsChildScenesInOrder(t *testing.T) {
	dir := t.TempDir()
	mkChapterDir(t, dir, "opening", map[string]string{
		"opening.md":  "hello",
		"opening2.md": "world",
	})
	writeManifestRaw(t, dir, `{"schemaVersion":3,"title":"N","items":[
		{"chapter":{"folder":"opening","title":"Opening","texts":[
			{"file":"opening.md","title":"Part One"},
			{"file":"opening2.md","title":"Part Two"}
		]}}
	]}`)
	if err := saveFolded(dir, map[string]bool{"opening": true}, map[string]bool{"opening": true}); err != nil {
		t.Fatal(err)
	}
	f := newFilelist()
	f.SetDir(dir)

	var got []fileEntry
	for _, e := range f.entries {
		if e.isChildScene {
			got = append(got, e)
		}
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 child-scene rows, got %+v", got)
	}
	if got[0].name != "opening.md" || got[0].parentFolder != "opening" {
		t.Fatalf("first child scene wrong: %+v", got[0])
	}
	if got[1].name != "opening2.md" || got[1].parentFolder != "opening" {
		t.Fatalf("second child scene wrong: %+v", got[1])
	}
	// Child rows must be positioned immediately after their chapter's own row.
	chIdx, child1Idx := -1, -1
	for i, e := range f.entries {
		if e.name == "opening" && e.isDir {
			chIdx = i
		}
		if e.name == "opening.md" && e.isChildScene {
			child1Idx = i
		}
	}
	if chIdx == -1 || child1Idx != chIdx+1 {
		t.Fatalf("child scene must immediately follow its chapter row: chIdx=%d child1Idx=%d", chIdx, child1Idx)
	}
}

func TestSelectedFileOnChildSceneIncludesParentFolder(t *testing.T) {
	dir := t.TempDir()
	mkChapterDir(t, dir, "opening", map[string]string{
		"opening.md":  "hello",
		"opening2.md": "world",
	})
	writeManifestRaw(t, dir, `{"schemaVersion":3,"title":"N","items":[
		{"chapter":{"folder":"opening","title":"Opening","texts":[
			{"file":"opening.md","title":"Part One"},
			{"file":"opening2.md","title":"Part Two"}
		]}}
	]}`)
	if err := saveFolded(dir, map[string]bool{"opening": true}, map[string]bool{"opening": true}); err != nil {
		t.Fatal(err)
	}
	f := newFilelist()
	f.SetDir(dir)

	childIdx := -1
	for i, e := range f.entries {
		if e.isChildScene && e.name == "opening2.md" {
			childIdx = i
		}
	}
	if childIdx == -1 {
		t.Fatalf("expected a child-scene row for opening2.md, got entries=%+v", f.entries)
	}
	f.selected = childIdx

	got, ok := f.selectedFile()
	if !ok {
		t.Fatal("selectedFile() ok = false for a child-scene row, want true")
	}
	want := filepath.Join(dir, "opening", "opening2.md")
	if got != want {
		t.Fatalf("selectedFile() = %q, want %q (must include the chapter's parentFolder)", got, want)
	}
}

func TestSelectedFileResolvesSingleTextChapterFolder(t *testing.T) {
	dir := t.TempDir()
	// A single-text chapter (one text), plus a multi-text chapter (two texts).
	mkChapterDir(t, dir, "sensations", map[string]string{"sensations.md": "hi"})
	mkChapterDir(t, dir, "opening", map[string]string{
		"opening.md":  "hello",
		"opening2.md": "world",
	})
	writeManifestRaw(t, dir, `{"schemaVersion":3,"title":"N","items":[
		{"chapter":{"folder":"sensations","title":"Sensations","texts":[
			{"file":"sensations.md","title":"Sensations"}
		]}},
		{"chapter":{"folder":"opening","title":"Opening","texts":[
			{"file":"opening.md","title":"Part One"},
			{"file":"opening2.md","title":"Part Two"}
		]}}
	]}`)
	f := newFilelist()
	f.SetDir(dir)

	sel := func(name string) {
		for i, e := range f.entries {
			if e.isDir && e.name == name {
				f.selected = i
				return
			}
		}
		t.Fatalf("no chapter-folder row named %q in entries=%+v", name, f.entries)
	}

	// Single-text chapter folder is still a container; callers must pick its scene row.
	sel("sensations")
	if _, ok := f.selectedFile(); ok {
		t.Fatal("selectedFile() ok = true for a single-text chapter folder, want false")
	}

	// Multi-text chapter folder is also a container.
	sel("opening")
	if _, ok := f.selectedFile(); ok {
		t.Fatal("selectedFile() ok = true for a multi-text chapter folder, want false")
	}
}

func TestSetDirSingleTextChapterExpandsWhenFolded(t *testing.T) {
	dir := t.TempDir()
	mkChapterDir(t, dir, "opening", map[string]string{"opening.md": "hello"})
	writeManifestRaw(t, dir, `{"schemaVersion":3,"title":"N","items":[
		{"chapter":{"folder":"opening","title":"Opening","texts":[{"file":"opening.md","title":"Opening"}]}}
	]}`)
	// Single-text chapters are containers too; a persisted folded=true entry exposes
	// their one scene row.
	if err := saveFolded(dir, map[string]bool{"opening": true}, map[string]bool{"opening": true}); err != nil {
		t.Fatal(err)
	}
	f := newFilelist()
	f.SetDir(dir)
	found := false
	for _, e := range f.entries {
		if e.isChildScene && e.parentFolder == "opening" && e.name == "opening.md" {
			found = true
		}
	}
	if !found {
		t.Fatalf("single-text chapter should show its scene row when expanded, got entries=%+v", f.entries)
	}
}

func TestSetDirLoadsFoldedOnceForSameManuscriptRoot(t *testing.T) {
	dir := t.TempDir()
	mkChapterDir(t, dir, "opening", map[string]string{"opening.md": "a", "opening2.md": "b"})
	writeManifestRaw(t, dir, `{"schemaVersion":3,"title":"N","items":[
		{"chapter":{"folder":"opening","title":"Opening","texts":[
			{"file":"opening.md","title":"P1"},{"file":"opening2.md","title":"P2"}
		]}}
	]}`)
	f := newFilelist()
	f.SetDir(dir)
	if f.foldedRoot != dir {
		t.Fatalf("foldedRoot = %q, want %q", f.foldedRoot, dir)
	}
	// Mutate the sidecar on disk directly, then call SetDir again on the SAME dir — the
	// in-memory f.folded must NOT pick up the change (loaded once per manuscript root, by
	// design — see spec's "Chargement du sidecar" section).
	if err := saveFolded(dir, map[string]bool{"opening": true}, map[string]bool{"opening": true}); err != nil {
		t.Fatal(err)
	}
	f.SetDir(dir)
	for _, e := range f.entries {
		if e.isChildScene {
			t.Fatal("second SetDir on the same manuscript root must not reload the sidecar")
		}
	}
}

func TestSetDirNonManuscriptFolderNeverLoadsFolded(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "note.md"), "x") // plain category, no manifest
	f := newFilelist()
	f.SetDir(dir)
	if f.foldedRoot != "" {
		t.Fatalf("foldedRoot should stay empty for a non-manuscript folder, got %q", f.foldedRoot)
	}
	if len(f.folded) != 0 {
		t.Fatalf("folded should stay empty for a non-manuscript folder, got %+v", f.folded)
	}
}

func TestActivateMultiTextChapterTogglesFoldInsteadOfPicker(t *testing.T) {
	dir := t.TempDir()
	mkChapterDir(t, dir, "opening", map[string]string{"opening.md": "a", "opening2.md": "b"})
	writeManifestRaw(t, dir, `{"schemaVersion":3,"title":"N","items":[
		{"chapter":{"folder":"opening","title":"Opening","texts":[
			{"file":"opening.md","title":"P1"},{"file":"opening2.md","title":"P2"}
		]}}
	]}`)
	f := newFilelist()
	f.SetDir(dir)
	f.selectName("opening") // chapter row, collapsed

	path, result := f.activate()
	if result != activateNone || path != "" {
		t.Fatalf("expanding a chapter must return activateNone/\"\", got %q/%v", path, result)
	}
	if !f.folded["opening"] {
		t.Fatal("activate on a collapsed multi-text chapter must expand it")
	}
	// f.entries must have been rebuilt with the child rows now visible.
	found := false
	for _, e := range f.entries {
		if e.isChildScene && e.name == "opening.md" {
			found = true
		}
	}
	if !found {
		t.Fatal("after expanding, child-scene rows must be present in f.entries")
	}

	// Activating again on the (still selected) chapter row collapses it back.
	f.selectName("opening")
	path, result = f.activate()
	if result != activateNone || path != "" {
		t.Fatalf("collapsing a chapter must return activateNone/\"\", got %q/%v", path, result)
	}
	if f.folded["opening"] {
		t.Fatal("activate on an expanded multi-text chapter must collapse it")
	}
}

func TestActivateFoldTogglePersistsToSidecar(t *testing.T) {
	dir := t.TempDir()
	mkChapterDir(t, dir, "opening", map[string]string{"opening.md": "a", "opening2.md": "b"})
	writeManifestRaw(t, dir, `{"schemaVersion":3,"title":"N","items":[
		{"chapter":{"folder":"opening","title":"Opening","texts":[
			{"file":"opening.md","title":"P1"},{"file":"opening2.md","title":"P2"}
		]}}
	]}`)
	f := newFilelist()
	f.SetDir(dir)
	f.selectName("opening")
	f.activate() // expand

	// Fresh filelist re-reading the same manuscript root must see the persisted state.
	f2 := newFilelist()
	f2.SetDir(dir)
	found := false
	for _, e := range f2.entries {
		if e.isChildScene {
			found = true
		}
	}
	if !found {
		t.Fatal("fold toggle must be persisted to .okashi-folded.json, not just in-memory")
	}
}

func TestActivateChildSceneOpensItsFile(t *testing.T) {
	dir := t.TempDir()
	mkChapterDir(t, dir, "opening", map[string]string{"opening.md": "a", "opening2.md": "b"})
	writeManifestRaw(t, dir, `{"schemaVersion":3,"title":"N","items":[
		{"chapter":{"folder":"opening","title":"Opening","texts":[
			{"file":"opening.md","title":"P1"},{"file":"opening2.md","title":"P2"}
		]}}
	]}`)
	if err := saveFolded(dir, map[string]bool{"opening": true}, map[string]bool{"opening": true}); err != nil {
		t.Fatal(err)
	}
	f := newFilelist()
	f.SetDir(dir)
	f.selectName("opening2.md") // second child-scene row

	path, result := f.activate()
	if result != activateFile {
		t.Fatalf("activating a child-scene row must return activateFile, got %v", result)
	}
	want := filepath.Join(dir, "opening", "opening2.md")
	if path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
}

func TestActivateSingleTextChapterIsContainer(t *testing.T) {
	// Regression guard: single-text chapters are still chapter containers, never direct files.
	dir := t.TempDir()
	mkChapterDir(t, dir, "opening", map[string]string{"opening.md": "hello"})
	writeManifestRaw(t, dir, `{"schemaVersion":3,"title":"N","items":[
		{"chapter":{"folder":"opening","title":"Opening","texts":[{"file":"opening.md","title":"Opening"}]}}
	]}`)
	f := newFilelist()
	f.SetDir(dir)
	f.selectName("opening")
	path, result := f.activate()
	if result != activateNone || path != "" {
		t.Fatalf("single-text chapter activate: path=%q result=%v", path, result)
	}
}

func TestSectionRowShowsCollapsedTriangleForMultiTextChapter(t *testing.T) {
	dir := t.TempDir()
	mkChapterDir(t, dir, "opening", map[string]string{"opening.md": "a", "opening2.md": "b"})
	writeManifestRaw(t, dir, `{"schemaVersion":3,"title":"N","items":[
		{"chapter":{"folder":"opening","title":"Opening","texts":[
			{"file":"opening.md","title":"P1"},{"file":"opening2.md","title":"P2"}
		]}}
	]}`)
	f := newFilelist()
	f.width = 40
	f.height = 10
	f.SetDir(dir)
	out := f.View(-1, "")
	if !strings.Contains(out, "▸") {
		t.Fatalf("collapsed multi-text chapter row must show a ▸ triangle, got:\n%s", out)
	}
	if strings.Contains(out, "▾") {
		t.Fatalf("collapsed chapter must not show the expanded ▾ triangle, got:\n%s", out)
	}
}

func TestSectionRowShowsExpandedTriangleAndIndentedChildren(t *testing.T) {
	dir := t.TempDir()
	mkChapterDir(t, dir, "opening", map[string]string{"opening.md": "a", "opening2.md": "b"})
	writeManifestRaw(t, dir, `{"schemaVersion":3,"title":"N","items":[
		{"chapter":{"folder":"opening","title":"Opening","texts":[
			{"file":"opening.md","title":"Part One"},{"file":"opening2.md","title":"Part Two"}
		]}}
	]}`)
	if err := saveFolded(dir, map[string]bool{"opening": true}, map[string]bool{"opening": true}); err != nil {
		t.Fatal(err)
	}
	f := newFilelist()
	f.width = 40
	f.height = 10
	f.SetDir(dir)
	out := f.View(-1, "")
	if !strings.Contains(out, "▾") {
		t.Fatalf("expanded multi-text chapter row must show a ▾ triangle, got:\n%s", out)
	}
	if !strings.Contains(out, "Part One") || !strings.Contains(out, "Part Two") {
		t.Fatalf("expanded chapter must render its scene titles, got:\n%s", out)
	}
}

func TestSectionRowNoTriangleForSingleTextChapter(t *testing.T) {
	dir := t.TempDir()
	mkChapterDir(t, dir, "opening", map[string]string{"opening.md": "hello"})
	writeManifestRaw(t, dir, `{"schemaVersion":3,"title":"N","items":[
		{"chapter":{"folder":"opening","title":"Opening","texts":[{"file":"opening.md","title":"Opening"}]}}
	]}`)
	f := newFilelist()
	f.width = 40
	f.height = 10
	f.SetDir(dir)
	out := f.View(-1, "")
	if strings.Contains(out, "▸") || strings.Contains(out, "▾") {
		t.Fatalf("single-text chapter must never show a fold triangle, got:\n%s", out)
	}
}

func TestChildSceneRowFallsBackToFilenameWhenTitleEmpty(t *testing.T) {
	dir := t.TempDir()
	mkChapterDir(t, dir, "opening", map[string]string{"opening.md": "a", "opening2.md": "b"})
	writeManifestRaw(t, dir, `{"schemaVersion":3,"title":"N","items":[
		{"chapter":{"folder":"opening","title":"Opening","texts":[
			{"file":"opening.md","title":""},{"file":"opening2.md","title":"Part Two"}
		]}}
	]}`)
	if err := saveFolded(dir, map[string]bool{"opening": true}, map[string]bool{"opening": true}); err != nil {
		t.Fatal(err)
	}
	f := newFilelist()
	f.width = 40
	f.height = 10
	f.SetDir(dir)
	out := f.View(-1, "")
	// sectionTitle("opening.md") de-slugs to "Opening" — the fallback must not render an
	// empty label for the first child row.
	if !strings.Contains(out, sectionTitle("opening.md")) {
		t.Fatalf("empty scene title must fall back to a de-slugged filename, got:\n%s", out)
	}
}

func TestSelectedEntryReturnsFullEntryUnderCursor(t *testing.T) {
	dir := t.TempDir()
	mkChapterDir(t, dir, "opening", map[string]string{
		"opening.md":  "hello",
		"opening2.md": "world",
	})
	writeManifestRaw(t, dir, `{"schemaVersion":3,"title":"N","items":[
		{"chapter":{"folder":"opening","title":"Opening","texts":[
			{"file":"opening.md","title":"Part One"},
			{"file":"opening2.md","title":"Part Two"}
		]}}
	]}`)
	if err := saveFolded(dir, map[string]bool{"opening": true}, map[string]bool{"opening": true}); err != nil {
		t.Fatal(err)
	}
	f := newFilelist()
	f.SetDir(dir)

	childIdx := -1
	for i, e := range f.entries {
		if e.isChildScene && e.name == "opening2.md" {
			childIdx = i
		}
	}
	if childIdx == -1 {
		t.Fatalf("expected a child-scene row for opening2.md, got entries=%+v", f.entries)
	}
	f.selected = childIdx

	got, ok := f.selectedEntry()
	if !ok {
		t.Fatal("selectedEntry() ok = false, want true")
	}
	if !got.isChildScene || got.name != "opening2.md" || got.parentFolder != "opening" {
		t.Fatalf("selectedEntry() = %+v, want isChildScene=true name=opening2.md parentFolder=opening", got)
	}
}

func TestSelectedEntryOutOfBoundsReturnsFalse(t *testing.T) {
	f := newFilelist()
	f.selected = 5
	_, ok := f.selectedEntry()
	if ok {
		t.Fatal("selectedEntry() ok = true for an out-of-bounds selection, want false")
	}
}
