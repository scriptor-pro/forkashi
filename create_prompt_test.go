package main

import (
	"os"
	"path/filepath"
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
