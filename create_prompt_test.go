package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"okashi/internal/textarea"
)

func createFlowModel(t *testing.T) (m model, dir string) {
	t.Helper()
	dir = t.TempDir()
	mkChapterDir(t, dir, "a", map[string]string{"a.md": "x"})
	writeManifest(dir, manifest{SchemaVersion: manifestSchemaVersion, Title: "W",
		Items: []manifestItem{{Chapter: &manifestChapter{Folder: "a", Title: "One",
			Texts: []manifestText{{File: "a.md", Title: "One"}}}}}})
	fl := newFilelist()
	fl.root, fl.width, fl.height = dir, 30, 30
	fl.SetDir(dir)
	m = model{width: 100, height: 30, files: fl, screen: screenWriting, focus: focusSidebar,
		sidebarVisible: true, editor: textarea.New(), nameInput: textinput.New()}
	return m, dir
}

func typeName(m model, s string) model {
	m.nameInput.SetValue(s)
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	return nm.(model)
}

func TestCtrlNChapterAppendsToManifest(t *testing.T) {
	m, dir := createFlowModel(t)
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlN})
	m = nm.(model)
	if !m.createPicker {
		t.Fatal("ctrl+n in a manuscript should open the chapter|resource picker")
	}
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}}) // chapter
	m = nm.(model)
	m = typeName(m, "the-fog")

	if _, err := os.Stat(filepath.Join(dir, "the-fog", "the-fog.md")); err != nil {
		t.Fatal("chapter folder + text file should be created")
	}
	mani, _, _ := readManifest(dir)
	if len(mani.Items) != 2 || mani.Items[1].Chapter == nil || mani.Items[1].Chapter.Folder != "the-fog" {
		t.Fatalf("chapter should be appended to the manifest: %+v", mani.Items)
	}
}

func TestCtrlNResourceLooseAndFolder(t *testing.T) {
	m, dir := createFlowModel(t)
	// Loose resource.
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlN})
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}}) // resource
	m = nm.(model)
	m = typeName(m, "notes")
	if _, err := os.Stat(filepath.Join(dir, "notes.md")); err != nil {
		t.Fatal("loose resource should be created at the root")
	}
	if mani, _, _ := readManifest(dir); len(mani.Items) != 1 {
		t.Fatal("a resource must NOT be added to the manifest")
	}

	// Resource in a folder.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlN})
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = nm.(model)
	m = typeName(m, "Characters/Aldous")
	if _, err := os.Stat(filepath.Join(dir, "Characters", "Aldous.md")); err != nil {
		t.Fatal("resource should be filed into the (created) Characters folder")
	}
}

func TestCtrlNOutsideManuscriptUnchanged(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "loose.md"), []byte("x"), 0o644) // no manifest → category
	fl := newFilelist()
	fl.root, fl.width, fl.height = dir, 30, 30
	fl.SetDir(dir)
	m := model{width: 100, height: 30, files: fl, screen: screenWriting, focus: focusSidebar,
		sidebarVisible: true, editor: textarea.New(), nameInput: textinput.New()}
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlN})
	m = nm.(model)
	if m.createPicker {
		t.Fatal("ctrl+n outside a manuscript should not show the picker")
	}
	if !m.creatingFile {
		t.Fatal("ctrl+n outside a manuscript should start the normal create flow")
	}
}

func TestCreateSceneAppendsToExistingChapter(t *testing.T) {
	m, dir := createFlowModel(t)
	m.createScene("a", "deuxième scène")

	if _, err := os.Stat(filepath.Join(dir, "a", "deuxieme-scene.md")); err != nil {
		t.Fatal("scene file should be created inside the chapter folder")
	}
	mani, _, _ := readManifest(dir)
	if len(mani.Items) != 1 {
		t.Fatalf("scene creation must not add a new manifest item, got %d items", len(mani.Items))
	}
	texts := mani.Items[0].Chapter.Texts
	if len(texts) != 2 || texts[1].File != "deuxieme-scene.md" || texts[1].Title != "deuxième scène" {
		t.Fatalf("scene should be appended to the chapter's Texts, got %+v", texts)
	}
}

func TestCreateSceneOpensInEditor(t *testing.T) {
	m, dir := createFlowModel(t)
	m.createScene("a", "deuxième scène")

	want := filepath.Join(dir, "a", "deuxieme-scene.md")
	if m.currentFile != want {
		t.Fatalf("currentFile = %q, want %q", m.currentFile, want)
	}
	if m.focus != focusEditor {
		t.Fatalf("focus should move to the editor, got %v", m.focus)
	}
}

func TestCreateSceneCollisionRefusesCreation(t *testing.T) {
	m, dir := createFlowModel(t)
	// "a.md" already exists in chapter "a" from createFlowModel's mkChapterDir.
	m.createScene("a", "a")

	mani, _, _ := readManifest(dir)
	if len(mani.Items[0].Chapter.Texts) != 1 {
		t.Fatalf("colliding scene name must not be added, got %+v", mani.Items[0].Chapter.Texts)
	}
	if !strings.Contains(m.status, "existe déjà") {
		t.Fatalf("status should report the collision, got %q", m.status)
	}
}

func TestCreateSceneUnknownChapterFolder(t *testing.T) {
	m, dir := createFlowModel(t)
	m.createScene("inexistant", "scène perdue")

	if _, err := os.Stat(filepath.Join(dir, "inexistant")); err == nil {
		t.Fatal("no folder should be created for an unknown chapter target")
	}
	if !strings.Contains(m.status, "introuvable") {
		t.Fatalf("status should report the missing chapter, got %q", m.status)
	}
}
