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
	path, ok := f.activate()
	if !ok || path != filepath.Join(dir, "note.md") {
		t.Fatalf("activate file = (%q, %v), want (%q, true)", path, ok, filepath.Join(dir, "note.md"))
	}

	// Activating ".." navigates up and opens nothing.
	f.SetDir(dir)
	f.selected = 0
	if _, ok := f.activate(); ok {
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
	if !strings.Contains(view, "3w") {
		t.Fatalf("manuscript pane should show the section word count '3w':\n%s", view)
	}
}

func TestSidebarRendersManifestTitleAndOrder(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "opening.md"), []byte("one two three"), 0o644)
	os.WriteFile(filepath.Join(dir, "the-letter.md"), []byte("a b"), 0o644)
	os.WriteFile(filepath.Join(dir, manifestName), []byte(
		`{"schemaVersion":1,"title":"Windermere","items":[`+
			`{"file":"the-letter.md","title":"The Letter"},`+
			`{"file":"opening.md","title":"Chapter One"}]}`), 0o644)
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
