package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestCapturingText(t *testing.T) {
	if !(model{screen: screenWriting, focus: focusEditor}).capturingText() {
		t.Fatal("editor focus should capture text (a literal ? is typed)")
	}
	if (model{screen: screenWriting, focus: focusSidebar}).capturingText() {
		t.Fatal("sidebar focus should NOT capture text")
	}
	if (model{screen: screenHome}).capturingText() {
		t.Fatal("home should NOT capture text")
	}
	if !(model{screen: screenSearch}).capturingText() {
		t.Fatal("search should capture text (typing the query)")
	}
	if !(model{renaming: true}).capturingText() {
		t.Fatal("a rename prompt should capture text")
	}
	if !(model{screen: screenWriting, focus: focusEditor, previewing: false}).capturingText() {
		t.Fatal("editor focus (not previewing) captures")
	}
	if (model{screen: screenWriting, focus: focusEditor, previewing: true}).capturingText() {
		t.Fatal("preview mode is not typing → does not capture")
	}
}

func TestHelpOpensGloballyAndRenders(t *testing.T) {
	// F1 opens help even on the home screen (the handler is global, before the screen dispatch).
	m := model{screen: screenHome, width: 80, height: 24}
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyF1})
	if !nm.(model).showHelp {
		t.Fatal("F1 should open help on the home screen")
	}
	// `?` opens help where the user isn't typing.
	nm2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	if !nm2.(model).showHelp {
		t.Fatal("? should open help on the home screen")
	}
	// The overlay renders over any screen (here: home), showing the categorized keys.
	out := ansi.Strip(model{screen: screenHome, width: 80, height: 24, showHelp: true}.View())
	for _, want := range []string{"NAVIGUER", "MANUSCRIT", "ctrl+k tableau", "J/K réordonner"} {
		if !strings.Contains(out, want) {
			t.Fatalf("help overlay should render %q over the home screen, got:\n%s", want, out)
		}
	}
}
