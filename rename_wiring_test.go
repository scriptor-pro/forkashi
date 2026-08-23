package main

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// typeInto sends each rune of s to the model as a key message.
func typeInto(t *testing.T, m model, s string) model {
	t.Helper()
	for _, r := range s {
		nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = nm.(model)
	}
	return m
}

func sidebarModel(t *testing.T, dir string) model {
	t.Helper()
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = nm.(model)
	m.screen = screenWriting
	m.files.SetDir(dir)
	m.focus = focusSidebar
	return m
}

func TestSidebarRenameLooseFileKeepsExt(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	os.WriteFile(filepath.Join(root, "draft.md"), []byte("hi"), 0o644)
	m := sidebarModel(t, root)
	m.files.selectName("draft.md")

	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = nm.(model)
	if !m.renaming {
		t.Fatal("r should start a rename")
	}
	m.nameInput.SetValue("")
	m = typeInto(t, m, "notes")
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if _, err := os.Stat(filepath.Join(root, "notes.md")); err != nil {
		t.Fatalf("expected renamed notes.md (ext kept): %v", err)
	}
}

// TestRenameManifestChapterRetitles was previously TestRenameRefusedForManifestChapter.
// Task 3 changed behavior: pressing r on a manifest chapter now opens a retitle
// prompt that edits items[].title, leaving the filename birth-stable.
func TestRenameManifestChapterRetitles(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	proj := filepath.Join(root, "novel")
	os.MkdirAll(filepath.Join(proj, "the-letter"), 0o755)
	os.WriteFile(filepath.Join(proj, "the-letter", "the-letter.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(proj, manifestName), []byte(
		`{"schemaVersion":3,"title":"N","items":[{"chapter":{"folder":"the-letter","title":"The Letter","texts":[{"file":"the-letter.md","title":"The Letter"}]}}]}`), 0o644)
	m := sidebarModel(t, proj)
	m.files.selectName("the-letter")

	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = nm.(model)
	if !m.renaming || !m.renameTarget.manifestChapter {
		t.Fatalf("r on a manifest chapter must start a manifestChapter retitle; renaming=%v target=%+v", m.renaming, m.renameTarget)
	}
	if got := m.nameInput.Value(); got != "The Letter" {
		t.Fatalf("prefill should be the current chapter title 'The Letter', got %q", got)
	}

	// Type a new title and confirm.
	m.nameInput.SetValue("")
	m = typeInto(t, m, "A New Title")
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)

	// File + folder must remain on disk, untouched.
	if _, err := os.Stat(filepath.Join(proj, "the-letter", "the-letter.md")); err != nil {
		t.Fatalf("chapter file must not be renamed on disk: %v", err)
	}
	// Manifest title must be updated.
	mf, _, _ := readManifest(proj)
	if mf.Items[0].Chapter.Title != "A New Title" {
		t.Fatalf("manifest title = %q, want 'A New Title'", mf.Items[0].Chapter.Title)
	}
	if mf.Items[0].Chapter.Folder != "the-letter" {
		t.Fatalf("manifest folder changed to %q — must be birth-stable", mf.Items[0].Chapter.Folder)
	}
}

func TestRenameAllowedForResourceInManuscript(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	proj := filepath.Join(root, "novel")
	os.MkdirAll(filepath.Join(proj, "a"), 0o755)
	os.WriteFile(filepath.Join(proj, "a", "a.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(proj, "notes.md"), []byte("y"), 0o644) // unlisted = Resource
	os.WriteFile(filepath.Join(proj, manifestName), []byte(
		`{"schemaVersion":3,"title":"N","items":[{"chapter":{"folder":"a","title":"One","texts":[{"file":"a.md","title":"One"}]}}]}`), 0o644)
	m := sidebarModel(t, proj)
	m.files.selectName("notes.md")

	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = nm.(model)
	if !m.renaming {
		t.Fatal("r on a Resource (unlisted file) should start a plain rename")
	}
	m.nameInput.SetValue("")
	m = typeInto(t, m, "scratch")
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if _, err := os.Stat(filepath.Join(proj, "scratch.md")); err != nil {
		t.Fatalf("Resource rename should work like a loose-file rename: %v", err)
	}
}

func TestRenameAllowedForLegacySection(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	proj := filepath.Join(root, "legacy")
	os.MkdirAll(proj, 0o755)
	os.WriteFile(filepath.Join(proj, "01-opening.md"), []byte("x"), 0o644) // numbered, no manifest
	m := sidebarModel(t, proj)
	m.files.selectName("01-opening.md")

	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = nm.(model)
	if !m.renaming {
		t.Fatal("r on a legacy numbered section should start a retitle (O1: legacy ergonomics kept)")
	}
	m.nameInput.SetValue("")
	m = typeInto(t, m, "the dawn")
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if _, err := os.Stat(filepath.Join(proj, "01-the-dawn.md")); err != nil {
		t.Fatalf("legacy retitle must preserve the numeric prefix: %v", err)
	}
}

func TestSidebarRenameFolder(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	os.MkdirAll(filepath.Join(root, "oldname"), 0o755)
	m := sidebarModel(t, root)
	m.files.selectName("oldname")

	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = nm.(model)
	m.nameInput.SetValue("")
	m = typeInto(t, m, "newname")
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if _, err := os.Stat(filepath.Join(root, "newname")); err != nil {
		t.Fatalf("folder rename should rename the directory: %v", err)
	}
}

func TestSidebarRenameRefusesCollision(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	os.WriteFile(filepath.Join(root, "a.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(root, "b.md"), []byte("y"), 0o644)
	m := sidebarModel(t, root)
	m.files.selectName("a.md")

	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = nm.(model)
	m.nameInput.SetValue("")
	m = typeInto(t, m, "b")
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	// Both originals must still exist — no overwrite.
	if b, _ := os.ReadFile(filepath.Join(root, "b.md")); string(b) != "y" {
		t.Fatal("rename onto an existing name must not overwrite it")
	}
	if _, err := os.Stat(filepath.Join(root, "a.md")); err != nil {
		t.Fatal("the source must be left intact when the rename is refused")
	}
}

func TestSidebarRenameTracksOpenFile(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	os.WriteFile(filepath.Join(root, "draft.md"), []byte("x"), 0o644)
	m := sidebarModel(t, root)
	m.currentFile = filepath.Join(root, "draft.md")
	m.files.selectName("draft.md")

	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = nm.(model)
	m.nameInput.SetValue("")
	m = typeInto(t, m, "final.md")
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if m.currentFile != filepath.Join(root, "final.md") {
		t.Fatalf("open file path should follow the rename, got %q", m.currentFile)
	}
}

// --- C1 / I1 corpus-safety guards (see fix-wave spec) ---

// TestCreateRejectsReservedManifestName: typing "manifest.json" as a new-file
// name must be rejected — currentFile must not change, status must be set.
func TestCreateRejectsReservedManifestName(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	m := sidebarModel(t, root)

	// Sentinel: detect any unwanted currentFile change.
	sentinel := filepath.Join(root, "sentinel.md")
	m.currentFile = sentinel

	// Enter file-creation mode and type the reserved name.
	m.creatingFile = true
	m.nameInput.SetValue("")
	m.nameInput.Focus()
	m = typeInto(t, m, "manifest.json")

	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)

	if m.currentFile != sentinel {
		t.Fatalf("currentFile must remain unchanged (got %q)", m.currentFile)
	}
	if m.status == "" {
		t.Fatal("status must be set to explain the rejection")
	}
}

// TestRenameRejectsReservedManifestName: renaming a loose file to "manifest.json"
// must be refused — original untouched, no manifest.json created.
func TestRenameRejectsReservedManifestName(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	os.WriteFile(filepath.Join(root, "draft.md"), []byte("hello"), 0o644)
	m := sidebarModel(t, root)
	m.files.selectName("draft.md")

	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = nm.(model)
	if !m.renaming {
		t.Fatal("r should start a rename")
	}
	m.nameInput.SetValue("")
	m = typeInto(t, m, "manifest.json")

	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)

	if _, err := os.Stat(filepath.Join(root, "draft.md")); err != nil {
		t.Fatalf("original file must be untouched: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, manifestName)); err == nil {
		t.Fatal("manifest.json must not be created by a rename")
	}
	if m.status == "" {
		t.Fatal("status must be set after rejecting manifest.json rename")
	}
}

// TestRenameRefusedInRefuseModeManifest: a folder with an unreadable manifest
// (schemaVersion 2 = unsupported) must block rename entirely — pressing 'r' on
// a file in that folder must NOT start a rename.
func TestRenameRefusedInRefuseModeManifest(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	proj := filepath.Join(root, "novel")
	os.MkdirAll(proj, 0o755)
	os.WriteFile(filepath.Join(proj, "01-opening.md"), []byte("x"), 0o644)
	// schemaVersion 4 triggers refuse mode (unsupported future version).
	os.WriteFile(filepath.Join(proj, manifestName), []byte(
		`{"schemaVersion":4,"title":"N","items":[{"file":"01-opening.md","title":"Opening"}]}`), 0o644)
	m := sidebarModel(t, proj)
	m.files.selectName("01-opening.md")

	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = nm.(model)

	if m.renaming {
		t.Fatal("r must NOT start a rename in a refuse-mode manifest folder")
	}
	if _, err := os.Stat(filepath.Join(proj, "01-opening.md")); err != nil {
		t.Fatalf("file must be untouched: %v", err)
	}
	if m.status == "" {
		t.Fatal("status must be set when rename is refused in refuse-mode")
	}
}

func TestCtrlKOnNonManuscriptStaysPut(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	plain := filepath.Join(root, "plain")
	os.MkdirAll(plain, 0o755)
	os.WriteFile(filepath.Join(plain, "a.md"), []byte("x"), 0o644) // unnumbered, no manifest
	m := sidebarModel(t, plain)

	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlK})
	m = nm.(model)
	// ctrl+k opens the corkboard (manifest-only) — on a plain folder it's a no-op.
	if m.screen != screenWriting {
		t.Fatalf("ctrl+k on a non-manuscript must not open the corkboard (screen=%v)", m.screen)
	}
}
