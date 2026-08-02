package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// moverModelAt builds a model whose file pane is at root with `sel` selected.
func moverModelAt(t *testing.T, root string, selName string) model {
	t.Helper()
	t.Setenv("OKASHI_DIR", root)
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	m = nm.(model)
	m.files.root = root
	m.files.SetDir(root)
	m.files.selectName(selName)
	return m
}

func TestMoverEnterFromFilePane(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "stray.md"), []byte("x"), 0o644)
	os.MkdirAll(filepath.Join(root, "research"), 0o755)
	m := moverModelAt(t, root, "stray.md")
	m.enterMover()
	if m.screen != screenMover {
		t.Fatalf("M should enter the mover, screen=%v", m.screen)
	}
	if m.moverSource != filepath.Join(root, "stray.md") || m.moverIsDir {
		t.Fatalf("source should be the selected file, got %q isDir=%v", m.moverSource, m.moverIsDir)
	}
	// The destination browser lists a "déplacer dans" row + the subfolder(s).
	out := ansi.Strip(m.moverView())
	if !strings.Contains(out, "déplacer dans") || !strings.Contains(out, "research") {
		t.Fatalf("mover view should show the destination browser (move-into + subfolders):\n%s", out)
	}
}

func TestMoverMoveFileIntoManuscriptAsChapter(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "scene.md"), []byte("x"), 0o644)
	proj := filepath.Join(root, "novel")
	createManuscript(proj, "Novel", "Opening")
	m := moverModelAt(t, root, "scene.md")
	m.enterMover()
	// drill into novel, then "move here" as a chapter.
	for i, e := range m.moverEntries {
		if e.kind == moverFolder && e.name == "novel" {
			m.moverSel = i
		}
	}
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // drill into novel
	m = nm.(model)
	m.moverSel = 0                                   // the "move here" row
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // → confirm
	m = nm.(model)
	if !m.moverConfirm {
		t.Fatal("moving a file into a manuscript should open the confirm")
	}
	if !m.moverAsChapter {
		t.Fatal("the confirm should default to 'chapter'")
	}
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}}) // apply
	m = nm.(model)
	// scene.md moved into novel and was appended as a chapter.
	if _, err := os.Stat(filepath.Join(proj, "scene.md")); err != nil {
		t.Fatalf("file should have moved into the manuscript: %v", err)
	}
	mf, _, _ := readManifest(proj)
	last := mf.Items[len(mf.Items)-1]
	if last.File != "scene.md" {
		t.Fatalf("file should be appended as a chapter, items=%+v", mf.Items)
	}
	if m.screen != screenWriting {
		t.Fatalf("after a move the mover should return, screen=%v", m.screen)
	}
}

func TestMoverMoveFolder(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "worldbuild"), 0o755)
	os.MkdirAll(filepath.Join(root, "trilogy"), 0o755)
	m := moverModelAt(t, root, "worldbuild")
	m.enterMover()
	if !m.moverIsDir {
		t.Fatal("source should be a folder")
	}
	for i, e := range m.moverEntries {
		if e.kind == moverFolder && e.name == "trilogy" {
			m.moverSel = i
		}
	}
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // drill into trilogy
	m = nm.(model)
	m.moverSel = 0                                   // move here
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // folder → plain confirm (no radio)
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	m = nm.(model)
	if _, err := os.Stat(filepath.Join(root, "trilogy", "worldbuild")); err != nil {
		t.Fatalf("folder should have moved under trilogy: %v", err)
	}
}

func TestMoverDrillIntoSubfolderAndBack(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "stray.md"), []byte("x"), 0o644)
	os.MkdirAll(filepath.Join(root, "research", "deep"), 0o755)
	m := moverModelAt(t, root, "stray.md")
	m.enterMover()
	// entries: [move-here, ▸ research]  (no ".." at the source root)
	// select "research" and drill in.
	researchIdx := -1
	for i, e := range m.moverEntries {
		if e.kind == moverFolder && e.name == "research" {
			researchIdx = i
		}
	}
	if researchIdx < 0 {
		t.Fatalf("research folder should be listed: %+v", m.moverEntries)
	}
	m.moverSel = researchIdx
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // drill into research
	m = nm.(model)
	if m.moverDestDir != filepath.Join(root, "research") {
		t.Fatalf("drilling should move destDir into research, got %q", m.moverDestDir)
	}
	// Now there is a ".." row and a "deep" subfolder.
	hasUp, hasDeep := false, false
	for _, e := range m.moverEntries {
		if e.kind == moverUp {
			hasUp = true
		}
		if e.kind == moverFolder && e.name == "deep" {
			hasDeep = true
		}
	}
	if !hasUp || !hasDeep {
		t.Fatalf("drilled browser should show '..' + 'deep', got %+v", m.moverEntries)
	}
	// Select ".." and go back to root; ".." must not escape the source root.
	for i, e := range m.moverEntries {
		if e.kind == moverUp {
			m.moverSel = i
		}
	}
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if m.moverDestDir != root {
		t.Fatalf("'..' should return to root, got %q", m.moverDestDir)
	}
}

func TestMoverStandalonePicksFileThenDest(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	os.WriteFile(filepath.Join(root, "stray.md"), []byte("x"), 0o644)
	os.MkdirAll(filepath.Join(root, "research"), 0o755)
	os.WriteFile(filepath.Join(root, "research", "deep.md"), []byte("y"), 0o644)

	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	m = nm.(model)
	m.enterMoverStandalone()
	if m.screen != screenMover || m.moverPhase != moverPickSource {
		t.Fatalf("standalone entry should open the mover in pick-source phase, screen=%v phase=%d", m.screen, m.moverPhase)
	}
	// Left pane lists the root's folders + files.
	names := map[string]moverEntryKind{}
	for _, e := range m.moverSrcEntries {
		names[e.name] = e.kind
	}
	if names["research"] != moverFolder || names["stray.md"] != moverFile {
		t.Fatalf("source picker should list research/ (folder) + stray.md (file), got %+v", m.moverSrcEntries)
	}
	// Select stray.md as the source → advances to pick-dest with the source set.
	for i, e := range m.moverSrcEntries {
		if e.kind == moverFile && e.name == "stray.md" {
			m.moverSrcSel = i
		}
	}
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if m.moverPhase != moverPickDest {
		t.Fatalf("picking a file should advance to pick-dest, phase=%d", m.moverPhase)
	}
	if m.moverSource != filepath.Join(root, "stray.md") || m.moverIsDir {
		t.Fatalf("source should be stray.md, got %q isDir=%v", m.moverSource, m.moverIsDir)
	}
	if m.moverFromDir != root {
		t.Fatalf("moverFromDir should be the file's container, got %q", m.moverFromDir)
	}
}

func TestMoverStandaloneDrillAndPickFolder(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	os.MkdirAll(filepath.Join(root, "worldbuild", "characters"), 0o755)
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	m = nm.(model)
	m.enterMoverStandalone()
	// drill into worldbuild
	for i, e := range m.moverSrcEntries {
		if e.kind == moverFolder && e.name == "worldbuild" {
			m.moverSrcSel = i
		}
	}
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // drill
	m = nm.(model)
	if m.moverSrcDir != filepath.Join(root, "worldbuild") {
		t.Fatalf("enter on a folder should drill in, srcDir=%q", m.moverSrcDir)
	}
	// A "→ move this folder" row now exists (we're below the source root); pick it.
	moveThis := -1
	for i, e := range m.moverSrcEntries {
		if e.kind == moverMoveThis {
			moveThis = i
		}
	}
	if moveThis < 0 {
		t.Fatalf("a 'move this folder' row should exist below root, got %+v", m.moverSrcEntries)
	}
	m.moverSrcSel = moveThis
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if m.moverPhase != moverPickDest || !m.moverIsDir || m.moverSource != filepath.Join(root, "worldbuild") {
		t.Fatalf("'move this folder' should pick worldbuild as a folder source; phase=%d isDir=%v src=%q", m.moverPhase, m.moverIsDir, m.moverSource)
	}
}

// twoSourceModel builds a model with two FOLDER sources. Folder sources' root() == Path, so a
// test controls the dirs; a PRIMARY source's root() is writingDir(), which a test can't set.
func twoSourceModel(t *testing.T, a, b string) model {
	t.Helper()
	return model{sources: []source{
		{ID: "a", Name: "Writing", Kind: sourceKindFolder, Path: a},
		{ID: "b", Name: "Notes", Kind: sourceKindFolder, Path: b},
	}}
}

func TestMoverBoundingSource(t *testing.T) {
	p, f := t.TempDir(), t.TempDir()
	m := twoSourceModel(t, p, f)
	if s, ok := m.moverBoundingSource(filepath.Join(p, "book")); !ok || s.ID != "a" {
		t.Fatalf("dir under source a should bind to a: %v %v", s, ok)
	}
	if s, ok := m.moverBoundingSource(f); !ok || s.ID != "b" {
		t.Fatalf("folder root should bind to source b: %v %v", s, ok)
	}
	if _, ok := m.moverBoundingSource("/nowhere/else"); ok {
		t.Fatalf("unrelated path should not bind")
	}
}

func TestMoverReloadSourcesList(t *testing.T) {
	p, f := t.TempDir(), t.TempDir()
	m := twoSourceModel(t, p, f)
	m.moverDestDir = "" // the sources list
	m.moverReload()
	if len(m.moverEntries) != 2 {
		t.Fatalf("want 2 source rows, got %d", len(m.moverEntries))
	}
	for _, e := range m.moverEntries {
		if e.kind != moverSource {
			t.Fatalf("sources list should only hold moverSource rows, got kind %d", e.kind)
		}
	}
}

func TestMoverReloadUpFromSourceRootGoesToSourcesList(t *testing.T) {
	p, f := t.TempDir(), t.TempDir()
	m := twoSourceModel(t, p, f)
	m.moverDestDir = p // a source root
	m.moverReload()
	if m.moverEntries[0].kind != moverMoveHere {
		t.Fatalf("first row should be move-here")
	}
	if m.moverEntries[1].kind != moverUp || m.moverEntries[1].path != "" {
		t.Fatalf("`..` at a source root must target the sources list (path \"\"), got %q", m.moverEntries[1].path)
	}
}

func TestMoverReloadUpBelowSourceRootGoesToParent(t *testing.T) {
	p, f := t.TempDir(), t.TempDir()
	sub := filepath.Join(p, "book")
	os.MkdirAll(sub, 0o755)
	m := twoSourceModel(t, p, f)
	m.moverDestDir = sub
	m.moverReload()
	if m.moverEntries[1].kind != moverUp || m.moverEntries[1].path != p {
		t.Fatalf("`..` below a source root must go to the parent %q, got %q", p, m.moverEntries[1].path)
	}
}

func TestMoverFailedMoveStaysOpenWithError(t *testing.T) {
	root := t.TempDir()
	// Source file + a colliding file at the destination so applyMove fails.
	srcDir := filepath.Join(root, "src")
	os.MkdirAll(srcDir, 0o755)
	os.WriteFile(filepath.Join(srcDir, "ch.md"), []byte("x"), 0o644)
	dstDir := filepath.Join(root, "dst")
	os.MkdirAll(dstDir, 0o755)
	os.WriteFile(filepath.Join(dstDir, "ch.md"), []byte("y"), 0o644) // collision

	m := twoSourceModel(t, root, root)
	m.screen = screenMover
	m.moverPhase = moverPickDest
	m.moverSource = filepath.Join(srcDir, "ch.md")
	m.moverFromDir = srcDir
	m.moverIsDir = false
	m.moverDestDir = dstDir
	m.moverConfirm = true
	m.moverReturn = screenWriting

	nm, _ := m.updateMover(tea.KeyMsg{Type: tea.KeyEnter})
	got := nm.(model)
	if got.moverError == "" {
		t.Fatalf("a failed move should set moverError")
	}
	if got.screen != screenMover {
		t.Fatalf("the mover must stay open on failure, screen=%v", got.screen)
	}
	if got.moverConfirm {
		t.Fatalf("confirm should be dismissed after the failed attempt")
	}
}

func TestMoverBoundingSourceNestedPrefersDeepest(t *testing.T) {
	outer := t.TempDir()
	inner := filepath.Join(outer, "drafts")
	if err := os.MkdirAll(inner, 0o755); err != nil {
		t.Fatal(err)
	}
	// outer is listed FIRST — the old first-match code returned it; the fix returns the deepest.
	m := model{sources: []source{
		{ID: "outer", Name: "Writing", Kind: sourceKindFolder, Path: outer},
		{ID: "inner", Name: "Drafts", Kind: sourceKindFolder, Path: inner},
	}}
	if s, ok := m.moverBoundingSource(filepath.Join(inner, "ch1")); !ok || s.ID != "inner" {
		t.Fatalf("a dir under the nested source should bind to it, got %q ok=%v", s.ID, ok)
	}
	if s, ok := m.moverBoundingSource(filepath.Join(outer, "notes")); !ok || s.ID != "outer" {
		t.Fatalf("a dir only under outer should bind to outer, got %q", s.ID)
	}
}
