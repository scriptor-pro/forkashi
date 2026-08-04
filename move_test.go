package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMoveDocumentChapterBetweenManuscripts moves a chapter's sole text file out of its
// folder (the mover only ever offers loose filesystem files — reachable by drilling into the
// chapter folder and picking its text directly) into another manuscript as a new chapter.
func TestMoveDocumentChapterBetweenManuscripts(t *testing.T) {
	root := t.TempDir()
	a := filepath.Join(root, "a")
	first, _ := createManuscript(a, "A", "Alpha") // "alpha/alpha.md"
	b := filepath.Join(root, "b")
	createManuscript(b, "B", "Beta") // "beta/beta.md" (distinct name → no collision)

	srcDir := filepath.Join(a, filepath.Dir(first)) // a/alpha
	file := filepath.Base(first)                    // alpha.md

	if err := moveDocument(srcDir, file, b, true); err != nil {
		t.Fatal(err)
	}
	// Removed from A's manifest.
	am, _, _ := readManifest(a)
	for _, it := range am.Items {
		if it.Chapter != nil && it.Chapter.Folder == "alpha" {
			t.Fatalf("alpha should have been removed from A's manifest, items=%+v", am.Items)
		}
	}
	// Appended to B's manifest as a new chapter (wrapped into a same-slug folder).
	bm, _, _ := readManifest(b)
	found := false
	for _, it := range bm.Items {
		if it.Chapter != nil && len(it.Chapter.Texts) > 0 && it.Chapter.Texts[0].File == file {
			found = true
		}
	}
	if !found {
		t.Fatalf("%s should have been inserted into B's manifest as a chapter, items=%+v", file, bm.Items)
	}
	if _, err := os.Stat(filepath.Join(b, "alpha", file)); err != nil {
		t.Fatalf("file should have moved into B under a new alpha/ chapter folder: %v", err)
	}
}

func TestSafeMoveSameVolume(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.md")
	dst := filepath.Join(dir, "sub", "a.md")
	os.MkdirAll(filepath.Join(dir, "sub"), 0o755)
	if err := os.WriteFile(src, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := safeMove(src, dst); err != nil {
		t.Fatalf("safeMove: %v", err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatal("source should be gone after a move")
	}
	b, err := os.ReadFile(dst)
	if err != nil || string(b) != "hello" {
		t.Fatalf("dest content = %q err=%v", b, err)
	}
}

func TestMoveDocumentLooseToCategory(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "note.md"), []byte("x"), 0o644)
	cat := filepath.Join(root, "cat")
	os.MkdirAll(cat, 0o755)
	if err := moveDocument(root, "note.md", cat, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(cat, "note.md")); err != nil {
		t.Fatalf("file should have moved: %v", err)
	}
}

func TestMoveDocumentLooseIntoManuscriptAsChapter(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "deleted-scene.md"), []byte("x"), 0o644)
	proj := filepath.Join(root, "novel")
	createManuscript(proj, "Novel", "Untitled") // one chapter: untitled/untitled.md
	if err := moveDocument(root, "deleted-scene.md", proj, true); err != nil {
		t.Fatal(err)
	}
	m, _, _ := readManifest(proj)
	last := m.Items[len(m.Items)-1]
	if last.Chapter == nil || len(last.Chapter.Texts) == 0 || last.Chapter.Texts[0].File != "deleted-scene.md" {
		t.Fatalf("moved file should be appended as a chapter, items=%+v", m.Items)
	}
	if last.Chapter.Title != "deleted scene" { // sectionTitle de-slugs the filename
		t.Fatalf("chapter title = %q, want de-slugged 'deleted scene'", last.Chapter.Title)
	}
	if last.Chapter.Folder != "deleted-scene" {
		t.Fatalf("chapter folder = %q, want slugified 'deleted-scene'", last.Chapter.Folder)
	}
	if _, err := os.Stat(filepath.Join(proj, "deleted-scene", "deleted-scene.md")); err != nil {
		t.Fatalf("file should be wrapped into a new chapter folder: %v", err)
	}
}

// TestMoveDocumentLooseIntoManuscriptAsChapterDedupesFolderCollision covers the
// reviewer's finding: moveDocument only checked os.Stat(dst) at the
// destination manuscript ROOT, never at the derived chapter-folder path. A
// loose file whose title-derived slug happens to match a chapter folder
// ALREADY in the destination manifest used to make MkdirAll silently no-op
// into the existing folder, appending a second manifest item that pointed at
// the same on-disk folder as the first. After the fix, the new chapter must
// be deduped ("deleted-scene-2") rather than collide.
func TestMoveDocumentLooseIntoManuscriptAsChapterDedupesFolderCollision(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "novel")
	// createManuscript's first chapter is slugified from "Deleted scene", i.e.
	// folder "deleted-scene" — matching the slug the moved file will derive.
	createManuscript(proj, "Novel", "Deleted scene")
	os.WriteFile(filepath.Join(root, "deleted-scene.md"), []byte("x"), 0o644)

	if err := moveDocument(root, "deleted-scene.md", proj, true); err != nil {
		t.Fatal(err)
	}

	m, _, err := readManifest(proj)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Items) != 2 {
		t.Fatalf("items = %+v, want 2 (original chapter + moved-in chapter)", m.Items)
	}
	folders := map[string]bool{}
	for _, it := range m.Items {
		if it.Chapter == nil {
			t.Fatalf("item missing Chapter: %+v", it)
		}
		folders[it.Chapter.Folder] = true
	}
	if len(folders) != 2 || !folders["deleted-scene"] || !folders["deleted-scene-2"] {
		t.Fatalf("folders = %+v, want distinct {deleted-scene, deleted-scene-2}", folders)
	}
	// Both folders must actually exist on disk with their own file — no overwrite.
	if _, err := os.Stat(filepath.Join(proj, "deleted-scene", "deleted-scene.md")); err != nil {
		t.Fatalf("original chapter file missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(proj, "deleted-scene-2", "deleted-scene.md")); err != nil {
		t.Fatalf("moved-in chapter file missing from deduped folder: %v", err)
	}
}

func TestMoveDocumentLooseIntoManuscriptAsResource(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "res.md"), []byte("x"), 0o644)
	proj := filepath.Join(root, "novel")
	createManuscript(proj, "Novel", "Untitled")
	before, _, _ := readManifest(proj)
	if err := moveDocument(root, "res.md", proj, false); err != nil {
		t.Fatal(err)
	}
	after, _, _ := readManifest(proj)
	if len(after.Items) != len(before.Items) {
		t.Fatalf("resource move must NOT change items: before=%d after=%d", len(before.Items), len(after.Items))
	}
	if _, err := os.Stat(filepath.Join(proj, "res.md")); err != nil {
		t.Fatalf("file should be in the folder as a resource: %v", err)
	}
}

func TestMoveDocumentChapterOutRemovesFromManifest(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "novel")
	first, _ := createManuscript(proj, "Novel", "Untitled") // first == "untitled/untitled.md"
	cat := filepath.Join(root, "cat")
	os.MkdirAll(cat, 0o755)

	srcDir := filepath.Join(proj, filepath.Dir(first)) // proj/untitled
	file := filepath.Base(first)                       // untitled.md
	if err := moveDocument(srcDir, file, cat, false); err != nil {
		t.Fatal(err)
	}
	m, _, _ := readManifest(proj)
	for _, it := range m.Items {
		if it.Chapter != nil && it.Chapter.Folder == "untitled" {
			t.Fatal("chapter should have been removed from the source manifest")
		}
	}
	if _, err := os.Stat(filepath.Join(cat, file)); err != nil {
		t.Fatalf("file should have moved to the category: %v", err)
	}
}

func TestMoveDocumentRefusesCollisionAndNoop(t *testing.T) {
	root := t.TempDir()
	a := filepath.Join(root, "a")
	b := filepath.Join(root, "b")
	os.MkdirAll(a, 0o755)
	os.MkdirAll(b, 0o755)
	os.WriteFile(filepath.Join(a, "x.md"), []byte("1"), 0o644)
	os.WriteFile(filepath.Join(b, "x.md"), []byte("2"), 0o644)
	if err := moveDocument(a, "x.md", b, false); err == nil {
		t.Fatal("a name collision in the destination must be refused")
	}
	if err := moveDocument(a, "x.md", a, false); err == nil {
		t.Fatal("moving into the same folder must be refused")
	}
}

func TestMoveFolderManuscriptRidesAlong(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "novel")
	createManuscript(proj, "Novel", "Untitled")
	trilogy := filepath.Join(root, "trilogy")
	os.MkdirAll(trilogy, 0o755)

	if err := moveFolder(proj, trilogy); err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(trilogy, "novel")
	if !hasManifest(moved) {
		t.Fatal("the manuscript's manifest should ride along into the new location")
	}
	m, _, err := readManifest(moved)
	if err != nil || m.Title != "Novel" {
		t.Fatalf("manifest should read back intact: title=%q err=%v", m.Title, err)
	}
	if _, err := os.Stat(proj); !os.IsNotExist(err) {
		t.Fatal("the old folder location should be gone")
	}
}

func TestMoveFolderRefusesCollisionAndSelf(t *testing.T) {
	root := t.TempDir()
	a := filepath.Join(root, "a")
	os.MkdirAll(filepath.Join(a, "sub"), 0o755)
	dst := filepath.Join(root, "dst")
	os.MkdirAll(filepath.Join(dst, "a"), 0o755) // collision: dst/a already exists

	if err := moveFolder(a, dst); err == nil {
		t.Fatal("a name collision must be refused")
	}
	if err := moveFolder(a, filepath.Join(a, "sub")); err == nil {
		t.Fatal("moving a folder into its own descendant must be refused")
	}
	if err := moveFolder(a, filepath.Dir(a)); err == nil {
		t.Fatal("moving a folder into the parent it already lives in must be refused")
	}
}

func TestMoveFolderSameVolumeSucceeds(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "chapters")
	if err := os.MkdirAll(filepath.Join(src, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	dstParent := filepath.Join(root, "book")
	if err := os.MkdirAll(dstParent, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := moveFolder(src, dstParent); err != nil {
		t.Fatalf("same-volume move should succeed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dstParent, "chapters", "sub")); err != nil {
		t.Fatalf("folder not moved: %v", err)
	}
}

func TestMoveFolderCollisionRefused(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "chapters")
	os.MkdirAll(src, 0o755)
	dstParent := filepath.Join(root, "book")
	os.MkdirAll(filepath.Join(dstParent, "chapters"), 0o755) // collision
	if err := moveFolder(src, dstParent); err == nil {
		t.Fatalf("expected a collision error")
	}
}
