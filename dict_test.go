package main

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestPersonalDictionaryEngine(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	personalLoaded = false
	personalWords = map[string]bool{}
	if spellOK("Xanadu") {
		t.Skip("base dictionary already knows the test word")
	}
	if !addToDictionary("Xanadu") {
		t.Fatal("add failed")
	}
	if !spellOK("Xanadu") || !spellOK("xanadu") {
		t.Fatal("added word must pass spellOK (both cases)")
	}
	// persisted + reloaded
	personalLoaded = false
	personalWords = map[string]bool{}
	loadPersonalDictionary()
	if !inPersonalDict("xanadu") {
		t.Fatal("word not persisted/reloaded")
	}
	// skips numeric / all-caps / duplicate
	if addToDictionary("ABC") || addToDictionary("123") || addToDictionary("Xanadu") {
		t.Fatal("should skip all-caps/numeric/duplicate")
	}
}

func TestAddToDictionaryViaSuggestion(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	personalLoaded = false
	personalWords = map[string]bool{}
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-a.md"), []byte("La Xanadu marche."), 0o644)
	t.Setenv("OKASHI_DIR", dir)
	m := initialModel()
	m.screen = screenWriting
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 90, Height: 14})
	m = nm.(model)
	m.loadFile(filepath.Join(dir, "01-a.md"))
	m.analysis.spell = true
	if spellOK("Xanadu") {
		t.Skip("base dictionary already knows the test word")
	}
	m.editor.MoveToLine(0)
	m.editor.SetCursor(5)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlR})
	m = nm.(model)
	if !m.suggesting || m.suggestions[len(m.suggestions)-1] != dictItem {
		t.Fatalf("suggestion bar should end with the +dict slot: %v", m.suggestions)
	}
	m.applySuggestion(len(m.suggestions) - 1)
	if !spellOK("Xanadu") {
		t.Fatal("the word should be known after add")
	}
}
