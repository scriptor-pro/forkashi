// okashi:selection
package textarea

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func shiftRight(m *Model) { *m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftRight}) }
func shiftLeft(m *Model)  { *m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftLeft}) }
func shiftDown(m *Model)  { *m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftDown}) }

func TestSelectionExtendAndText(t *testing.T) {
	m := New()
	m.Focus()
	m.SetValue("hello world")
	m.SetCursor(0)

	if m.HasSelection() {
		t.Fatal("no selection should be active before any shift movement")
	}

	for i := 0; i < 5; i++ {
		shiftRight(&m)
	}
	if !m.HasSelection() {
		t.Fatal("shift+right should start a selection")
	}
	if got := m.selectedText(); got != "hello" {
		t.Fatalf("selectedText = %q, want %q", got, "hello")
	}
}

func TestSelectionAcrossLines(t *testing.T) {
	m := New()
	m.Focus()
	m.SetValue("hello\nworld")
	m.row = 0
	m.SetCursor(3) // "hel|lo"

	shiftDown(&m) // extend onto the second line, same column
	if got := m.selectedText(); got != "lo\nwor" {
		t.Fatalf("selectedText across lines = %q, want %q", got, "lo\nwor")
	}
}

func TestSelectionClearedByPlainMovement(t *testing.T) {
	m := New()
	m.Focus()
	m.SetValue("hello world")
	m.SetCursor(0)
	shiftRight(&m)
	shiftRight(&m)
	if !m.HasSelection() {
		t.Fatal("expected an active selection")
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight}) // plain movement, no shift
	if m.HasSelection() {
		t.Fatal("a plain (non-shift) movement should clear the selection")
	}
}

func TestSelectionClearedByEscape(t *testing.T) {
	m := New()
	m.Focus()
	m.SetValue("hello world")
	m.SetCursor(0)
	shiftRight(&m)
	shiftRight(&m)

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.HasSelection() {
		t.Fatal("esc should clear an active selection")
	}
}

func TestCopyWritesSelectionAndKeepsText(t *testing.T) {
	m := New()
	m.Focus()
	m.SetValue("hello world")
	m.SetCursor(0)
	for i := 0; i < 5; i++ {
		shiftRight(&m)
	}

	cmd := m.Copy()
	if cmd == nil {
		t.Fatal("Copy should return a command when there is a selection")
	}
	if got := m.Value(); got != "hello world" {
		t.Fatalf("Copy must not modify the buffer, got %q", got)
	}
	if m.HasSelection() {
		t.Fatal("Copy should clear the selection once done")
	}
}

func TestCopyNoSelectionIsNoop(t *testing.T) {
	m := New()
	m.Focus()
	m.SetValue("hello")
	if cmd := m.Copy(); cmd != nil {
		t.Fatal("Copy with no active selection should return a nil command")
	}
}

func TestCutRemovesSelectionAndPlacesCursor(t *testing.T) {
	m := New()
	m.Focus()
	m.SetValue("hello world")
	m.SetCursor(0)
	for i := 0; i < 6; i++ { // select "hello "
		shiftRight(&m)
	}

	cmd := m.Cut()
	if cmd == nil {
		t.Fatal("Cut should return a command when there is a selection")
	}
	if got := m.Value(); got != "world" {
		t.Fatalf("Cut should remove the selection from the buffer, got %q", got)
	}
	if m.row != 0 || m.col != 0 {
		t.Fatalf("Cut should place the cursor at the selection start, got row=%d col=%d", m.row, m.col)
	}
	if m.HasSelection() {
		t.Fatal("Cut should clear the selection")
	}
}

func TestCutNoSelectionIsNoop(t *testing.T) {
	m := New()
	m.Focus()
	m.SetValue("hello")
	if cmd := m.Cut(); cmd != nil {
		t.Fatal("Cut with no active selection should return a nil command")
	}
	if got := m.Value(); got != "hello" {
		t.Fatalf("Cut with no selection must not modify the buffer, got %q", got)
	}
}

func TestCutAcrossLines(t *testing.T) {
	m := New()
	m.Focus()
	m.SetValue("hello\nworld")
	m.row = 0
	m.SetCursor(3) // "hel|lo"
	shiftDown(&m)  // select "lo\nwor"

	if cmd := m.Cut(); cmd == nil {
		t.Fatal("Cut should return a command")
	}
	// "hello\nworld" minus the selected "lo\nwor" (the newline between them is
	// part of the selection too) leaves "hel" and "ld" joined on one line.
	if got := m.Value(); got != "helld" {
		t.Fatalf("Cut across lines = %q, want %q", got, "helld")
	}
}
