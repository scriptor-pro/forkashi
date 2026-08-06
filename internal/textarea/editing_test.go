package textarea

import "testing"

func TestIndentOutdent(t *testing.T) {
	m := New()
	m.SetValue("hello")
	m.SetCursor(5) // end of "hello"

	m.Indent()
	if got := m.Value(); got != "  hello" {
		t.Fatalf("after Indent: %q, want %q", got, "  hello")
	}
	if m.col != 7 {
		t.Fatalf("cursor col = %d, want 7", m.col)
	}

	m.Outdent()
	if got := m.Value(); got != "hello" {
		t.Fatalf("after Outdent: %q, want %q", got, "hello")
	}
	if m.col != 5 {
		t.Fatalf("cursor col = %d, want 5", m.col)
	}

	m.Outdent() // no leading spaces → no-op
	if got := m.Value(); got != "hello" {
		t.Fatalf("Outdent on unindented line changed it: %q", got)
	}
}

func TestLineHelpers(t *testing.T) {
	m := New()
	m.SetValue("- item")
	m.SetCursor(6)
	if m.CurrentLine() != "- item" {
		t.Fatalf("CurrentLine = %q", m.CurrentLine())
	}
	if !m.AtLineEnd() {
		t.Fatal("AtLineEnd should be true at col 6")
	}
	if r, ok := m.CharBeforeCursor(); !ok || r != 'm' {
		t.Fatalf("CharBeforeCursor = %q,%v want 'm',true", r, ok)
	}
	m.SetCursor(0)
	if _, ok := m.CharBeforeCursor(); ok {
		t.Fatal("CharBeforeCursor at col 0 should be ok=false")
	}
	if m.AtLineEnd() {
		t.Fatal("AtLineEnd should be false at col 0 of a non-empty line")
	}

	m.ClearLine()
	if m.Value() != "" || m.col != 0 {
		t.Fatalf("after ClearLine: %q col=%d", m.Value(), m.col)
	}
}

func TestReplaceCharBeforeCursor(t *testing.T) {
	m := New()
	m.SetValue("a!")
	m.SetCursor(2)
	m.ReplaceCharBeforeCursor(' ')
	if got := m.Value(); got != "a " {
		t.Fatalf("after ReplaceCharBeforeCursor: %q, want %q", got, "a ")
	}
	if m.col != 2 {
		t.Fatalf("cursor col = %d, want 2 (unchanged)", m.col)
	}

	m.SetCursor(0)
	m.ReplaceCharBeforeCursor('x') // no-op at start of line
	if got := m.Value(); got != "a " {
		t.Fatalf("ReplaceCharBeforeCursor at col 0 should be no-op, got %q", got)
	}
}

func TestRunCountBeforeCursor(t *testing.T) {
	m := New()
	m.SetValue("a!!!b")
	m.SetCursor(4) // just before 'b', after "a!!!"
	if n := m.RunCountBeforeCursor('!'); n != 3 {
		t.Fatalf("RunCountBeforeCursor('!') = %d, want 3", n)
	}
	if n := m.RunCountBeforeCursor('?'); n != 0 {
		t.Fatalf("RunCountBeforeCursor('?') = %d, want 0 (wrong rune)", n)
	}

	m.SetCursor(0)
	if n := m.RunCountBeforeCursor('!'); n != 0 {
		t.Fatalf("RunCountBeforeCursor at col 0 = %d, want 0", n)
	}

	m.SetCursor(1) // just after "a"
	if n := m.RunCountBeforeCursor('!'); n != 0 {
		t.Fatalf("RunCountBeforeCursor('!') at col 1 = %d, want 0", n)
	}
}
