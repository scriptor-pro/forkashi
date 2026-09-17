package main

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"okashi/internal/textarea"
)

func TestSelectModeToggle(t *testing.T) {
	m := model{screen: screenWriting, editor: textarea.New()}

	mm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}, Alt: true})
	m = mm.(model)
	if !m.selectMode {
		t.Fatal("alt+s should enable select mode")
	}
	if cmd == nil {
		t.Fatal("enabling select mode should return a mouse-disable command")
	}

	mm, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}, Alt: true})
	m = mm.(model)
	if m.selectMode {
		t.Fatal("a second alt+s should disable select mode")
	}
	if cmd == nil {
		t.Fatal("disabling select mode should return a mouse-enable command")
	}
}

// Leaving the writing screen for the hub must restore the mouse — the hub needs clicks and
// alt+s is unreachable there, so a lingering select mode would trap the user with a dead mouse.
func TestSelectModeRestoredOnGoHome(t *testing.T) {
	m := model{screen: screenWriting, selectMode: true, editor: textarea.New()}
	mm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	m = mm.(model)
	if m.selectMode {
		t.Fatal("going to the hub must clear select mode")
	}
	if cmd == nil {
		t.Fatal("going to the hub from select mode must return a mouse-enable command")
	}
}

func TestSelectModeStatusIndicator(t *testing.T) {
	m := model{width: 100, colWidth: 72, selectMode: true, editor: textarea.New()}
	if !containsSubstr(m.statusBar(), "SELECT") {
		t.Fatal("the status bar should show the -- SELECT -- indicator when select mode is on")
	}
}

// containsSubstr is a tiny helper so the test file needs no extra import.
func containsSubstr(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
