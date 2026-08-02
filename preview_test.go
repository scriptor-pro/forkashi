package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestFootnotesToEndnotes(t *testing.T) {
	out := footnotesToEndnotes("cat[^1] and[^2] cat again[^1].\n\n[^1]: first.\n[^2]: second.\n")
	if strings.Contains(out, "[^1]") || strings.Contains(out, "[^2]:") {
		t.Fatalf("syntax not stripped: %q", out)
	}
	if !strings.Contains(out, "### Notes") || !strings.Contains(out, "1. first") || !strings.Contains(out, "2. second") {
		t.Fatalf("notes section missing: %q", out)
	}
	if !strings.Contains(out, "¹") || !strings.Contains(out, "²") {
		t.Fatalf("superscript markers missing: %q", out)
	}
	if got := footnotesToEndnotes("plain text\n"); got != "plain text\n" {
		t.Fatalf("no-footnote input should be unchanged, got %q", got)
	}
	if !strings.Contains(footnotesToEndnotes("see[^x] here\n"), "[^x]") {
		t.Fatal("orphan reference (no definition) should be kept literal")
	}
}

func TestFootnotesSkipCode(t *testing.T) {
	out := footnotesToEndnotes("```\narr[^1] = x\n```\n\nreal[^1] body.\n\n[^1]: note.\n")
	if !strings.Contains(out, "arr[^1] = x") {
		t.Fatal("footnote-like text inside a code block must survive verbatim")
	}
	if strings.Contains(out, "real[^1]") || !strings.Contains(out, "### Notes") {
		t.Fatal("a real footnote reference outside code should still convert")
	}
	if !strings.Contains(footnotesToEndnotes("use `x[^1]` and real[^1].\n\n[^1]: n.\n"), "`x[^1]`") {
		t.Fatal("inline code span [^1] must survive")
	}
}

func TestFootnotesToSidenotesSplitsNotes(t *testing.T) {
	src := "Alpha[^a] and beta[^b].\n\n[^a]: first note\n[^b]: second note\n"
	body, notes := footnotesToSidenotes(src)
	if len(notes) != 2 {
		t.Fatalf("want 2 notes, got %d: %v", len(notes), notes)
	}
	if notes[0] != "first note" || notes[1] != "second note" {
		t.Fatalf("notes out of order: %v", notes)
	}
	if strings.Contains(body, "Notes") || strings.Contains(body, "[^a]") {
		t.Fatalf("body should have no Notes section and no raw refs: %q", body)
	}
	if !strings.Contains(body, superscript(1)) || !strings.Contains(body, superscript(2)) {
		t.Fatalf("body missing superscript refs: %q", body)
	}
}

func TestFootnotesToSidenotesNoFootnotes(t *testing.T) {
	body, notes := footnotesToSidenotes("Just prose, no notes.\n")
	if len(notes) != 0 {
		t.Fatalf("want 0 notes, got %v", notes)
	}
	if !strings.Contains(body, "Just prose") {
		t.Fatalf("body mangled: %q", body)
	}
}

func TestFootnotesToSidenotesIgnoresCodeAndOrphans(t *testing.T) {
	src := "See `arr[^1]` and real[^r].\n\n[^r]: real note\n"
	body, notes := footnotesToSidenotes(src)
	if len(notes) != 1 || notes[0] != "real note" {
		t.Fatalf("want 1 real note, got %v", notes)
	}
	if !strings.Contains(body, "arr[^1]") {
		t.Fatalf("code span footnote must stay literal: %q", body)
	}
}

func TestPreviewTufteToggle(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-a.md"), []byte("# Title\n\nThe cat[^1] sat.\n\n[^1]: a note.\n"), 0o644)
	t.Setenv("OKASHI_DIR", dir)
	m := initialModel()
	m.screen = screenWriting
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	m = nm.(model)
	m.loadFile(filepath.Join(dir, "01-a.md"))
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	m = nm.(model)
	if !m.previewing {
		t.Fatal("ctrl+p should enter preview")
	}
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "Notes") || !strings.Contains(v, "a note") || strings.Contains(v, "[^1]") {
		t.Fatal("preview should fold footnotes to endnotes")
	}
	if !strings.Contains(v, "Standard") {
		t.Fatal("preview header should show the Standard style")
	}
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	m = nm.(model)
	if !m.previewTufte || !strings.Contains(ansi.Strip(m.View()), "Tufte") {
		t.Fatal("t should toggle to the Tufte style")
	}
}

func TestLayoutSidenotesAnchorsOnRefRow(t *testing.T) {
	body := "line zero\nalpha " + superscript(1) + " here\nline two\n"
	out := layoutSidenotes(body, []string{"the note"}, 20, 12, false)
	lines := strings.Split(out, "\n")
	// The note must appear on the same row as the ¹ marker (row index 1), in the gutter.
	if !strings.Contains(lines[1], "┆") || !strings.Contains(lines[1], "the note") {
		t.Fatalf("note not on ref row:\n%s", out)
	}
	// Row 0 has a gutter divider but no note text.
	if !strings.Contains(lines[0], "┆") || strings.Contains(lines[0], "the note") {
		t.Fatalf("row 0 should be divider-only:\n%s", out)
	}
}

func TestLayoutSidenotesCascadeNoOverlap(t *testing.T) {
	// Two markers on adjacent rows; notes must not land on the same gutter row.
	body := "a " + superscript(1) + "\nb " + superscript(2) + "\n"
	out := layoutSidenotes(body, []string{"note one", "note two"}, 10, 12, false)
	lines := strings.Split(out, "\n")
	row1 := -1
	row2 := -1
	for i, ln := range lines {
		if strings.Contains(ln, "note one") {
			row1 = i
		}
		if strings.Contains(ln, "note two") {
			row2 = i
		}
	}
	if row1 == -1 || row2 == -1 || row1 == row2 {
		t.Fatalf("notes overlap or missing (row1=%d row2=%d):\n%s", row1, row2, out)
	}
}

func TestLayoutSidenotesMarkerIsWholeRun(t *testing.T) {
	// ¹ must not anchor inside ¹² (note 12's marker).
	body := "x " + superscript(12) + "\ny " + superscript(1) + "\n"
	notes := make([]string, 12)
	for i := range notes {
		notes[i] = "n" + superscript(i+1)
	}
	out := layoutSidenotes(body, notes, 10, 14, false)
	lines := strings.Split(out, "\n")
	// note 1 (n¹) should anchor on row 1 (the y-line), not row 0 (the ¹² line).
	for i, ln := range lines {
		if strings.Contains(ln, "n"+superscript(1)) && !strings.Contains(ln, "n"+superscript(12)) {
			if i == 0 {
				t.Fatalf("note 1 mis-anchored onto the ¹² row:\n%s", out)
			}
			break
		}
	}
}

func TestSidenoteGeometryGate(t *testing.T) {
	// avail 80, measure 72 → gutter 5 < 18 → no sidenotes.
	if _, ok := sidenoteGeometry(80, 72); ok {
		t.Fatalf("avail 80 / measure 72 should not fit a sidenote margin")
	}
	// avail 96, measure 72 → gutter 21 → ok, within [18,30].
	gutter, ok := sidenoteGeometry(96, 72)
	if !ok {
		t.Fatalf("avail 96 / measure 72 should fit sidenotes")
	}
	if gutter < 18 || gutter > 30 {
		t.Fatalf("gutter %d out of [18,30]", gutter)
	}
	// avail 200 → gutter clamped to 30.
	if g, _ := sidenoteGeometry(200, 72); g != 30 {
		t.Fatalf("gutter should clamp to 30, got %d", g)
	}
}

func TestSidenotePlanEngagesOnlyWithRoomAndNotes(t *testing.T) {
	doc := "Alpha[^a] and beta[^b].\n\n[^a]: first\n[^b]: second\n"
	// Wide pane + footnotes → engaged, body measure stays 72.
	measure, gutter, _, notes, ok := sidenotePlan(120, 72, doc)
	if !ok || len(notes) != 2 || measure != 72 || gutter < 18 || gutter > 30 {
		t.Fatalf("wide+footnotes should engage: measure=%d gutter=%d notes=%d ok=%v", measure, gutter, len(notes), ok)
	}
	// Narrow pane → not engaged even with footnotes.
	if _, _, _, _, ok := sidenotePlan(80, 72, doc); ok {
		t.Fatalf("narrow pane should not engage sidenotes")
	}
	// Wide pane, no footnotes → not engaged.
	if _, _, _, _, ok := sidenotePlan(120, 72, "just prose\n"); ok {
		t.Fatalf("no footnotes should not engage sidenotes")
	}
}
