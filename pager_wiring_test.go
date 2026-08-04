package main

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestPagerManifestTitleAndOrder(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "opening"), 0o755)
	os.MkdirAll(filepath.Join(dir, "the-letter"), 0o755)
	os.WriteFile(filepath.Join(dir, "opening", "opening.md"), []byte("one two"), 0o644)
	os.WriteFile(filepath.Join(dir, "the-letter", "the-letter.md"), []byte("a b"), 0o644)
	os.WriteFile(filepath.Join(dir, manifestName), []byte(
		`{"schemaVersion":2,"title":"Windermere","items":[`+
			`{"chapter":{"folder":"the-letter","title":"The Letter","texts":[{"file":"the-letter.md","title":"The Letter"}]}},`+
			`{"chapter":{"folder":"opening","title":"Chapter One","texts":[{"file":"opening.md","title":"Chapter One"}]}}]}`), 0o644)
	var p pagerModel
	p.load(dir, 60)
	// First header should use manifest order and manifest title.
	if len(p.lines) == 0 || p.lines[0].text != "── The Letter ──" {
		first := ""
		if len(p.lines) > 0 {
			first = p.lines[0].text
		}
		t.Fatalf("first pager line should be manifest header '── The Letter ──', got %q", first)
	}
	// Second header should be "Chapter One" (after header + 1 body line from first section).
	if len(p.lines) < 3 || p.lines[2].text != "── Chapter One ──" {
		second := ""
		if len(p.lines) > 2 {
			second = p.lines[2].text
		}
		t.Fatalf("pager should walk manifest order; expected '── Chapter One ──' at index 2, got %q", second)
	}
}

func manuscriptModel(t *testing.T) (model, string) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	proj := filepath.Join(root, "novel")
	os.MkdirAll(proj, 0o755)
	os.WriteFile(filepath.Join(proj, "01-a.md"), []byte("alpha beta\ngamma"), 0o644)
	os.WriteFile(filepath.Join(proj, "02-b.md"), []byte("delta"), 0o644)
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = nm.(model)
	m.screen = screenWriting
	m.files.SetDir(proj)
	return m, proj
}

func TestSidebarMEntersPager(t *testing.T) {
	m, _ := manuscriptModel(t)
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}}) // m (sidebar) → pager
	m = nm.(model)
	if m.screen != screenManuscript {
		t.Fatalf("m in the outline should enter the pager, got screen %v", m.screen)
	}
	if len(m.pager.lines) == 0 {
		t.Fatal("the pager should be built on entry")
	}
}

func TestPagerEnterJumpsToEditAtLine(t *testing.T) {
	m, proj := manuscriptModel(t)
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}}) // sidebar m → pager
	m = nm.(model)
	// Move the cursor to the "gamma" line (header, alpha, gamma -> index 2).
	m.pager.cursor = 2
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if m.screen != screenWriting {
		t.Fatalf("Enter should jump into the editor, got screen %v", m.screen)
	}
	if m.currentFile != filepath.Join(proj, "01-a.md") {
		t.Fatalf("Enter should open the mapped section, currentFile = %q", m.currentFile)
	}
	if m.editor.Line() != 1 {
		t.Fatalf("Enter should place the editor cursor on source line 1 (gamma), got %d", m.editor.Line())
	}
}

func TestPagerClampsWidthToTerminal(t *testing.T) {
	m, _ := manuscriptModel(t)
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 30, Height: 20}) // narrow terminal
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}}) // sidebar m → pager
	m = nm.(model)
	if m.pager.width > 30 {
		t.Fatalf("pager width must clamp to the terminal (<=30), got %d", m.pager.width)
	}
}

func TestPagerResizeReclampsWidth(t *testing.T) {
	m, _ := manuscriptModel(t)                                            // 100-col terminal
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}}) // sidebar m → pager
	m = nm.(model)
	before := m.pager.width
	nm, _ = m.Update(tea.WindowSizeMsg{Width: 20, Height: 15}) // shrink
	m = nm.(model)
	if m.pager.width >= before || m.pager.width > 20 {
		t.Fatalf("resize should re-clamp the pager width to <=20 (was %d, now %d)", before, m.pager.width)
	}
	if len(m.pager.lines) == 0 {
		t.Fatal("resize should re-wrap the lines, not clear them")
	}
}

func TestPagerClickThenDoubleClickJumps(t *testing.T) {
	m, proj := manuscriptModel(t)
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}}) // sidebar m → pager
	m = nm.(model)
	// Click the body line at offset 0 + (clickY - pagerHeaderHeight) = line 2 (gamma).
	clickY := pagerHeaderHeight + 2
	nm, _ = m.Update(tea.MouseMsg{X: 2, Y: clickY, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = nm.(model)
	if m.pager.cursor != 2 {
		t.Fatalf("click should move the cursor to line 2, got %d", m.pager.cursor)
	}
	nm, _ = m.Update(tea.MouseMsg{X: 2, Y: clickY, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = nm.(model)
	if m.screen != screenWriting || m.currentFile != filepath.Join(proj, "01-a.md") {
		t.Fatalf("double-click should jump into the editor, screen=%v file=%q", m.screen, m.currentFile)
	}
	if m.editor.Line() != 1 {
		t.Fatalf("double-click jump should land on source line 1 (gamma), got %d", m.editor.Line())
	}
}
