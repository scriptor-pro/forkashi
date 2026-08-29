package main

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestNotesRoundTripAndTolerantLoad(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "01-a.md")
	if err := os.WriteFile(file, []byte("body"), 0o644); err != nil {
		t.Fatal(err)
	}
	notes := []note{
		{ID: "n1", Scope: "chapter", Text: "Cut the flashback.\nToo slow."},
		{ID: "n2", Scope: "chapter", Text: "Check the date < 1911 & > 1900."},
	}
	if err := saveNotes(file, notes); err != nil {
		t.Fatal(err)
	}
	got := loadNotes(file)
	if len(got) != 2 || got[0].Text != "Cut the flashback.\nToo slow." || got[1].ID != "n2" {
		t.Fatalf("round-trip: %+v", got)
	}
	// Ampersand/angle brackets not HTML-escaped.
	data, _ := os.ReadFile(notesPath(file))
	if !contains(string(data), "< 1911 & > 1900") {
		t.Fatalf("notes should not HTML-escape:\n%s", data)
	}
	// Tolerant: missing → nil; corrupt → nil.
	if loadNotes(filepath.Join(dir, "nope.md")) != nil {
		t.Fatal("missing notes → nil")
	}
	if err := os.WriteFile(notesPath(file), []byte("{bad"), 0o644); err != nil {
		t.Fatal(err)
	}
	if loadNotes(file) != nil {
		t.Fatal("corrupt notes → nil, no error")
	}
}

func TestSaveNotesEmptyRemovesSidecar(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.md")
	os.WriteFile(file, []byte("x"), 0o644)
	saveNotes(file, []note{{ID: "n1", Text: "keep"}})
	if _, err := os.Stat(notesPath(file)); err != nil {
		t.Fatal("sidecar should exist")
	}
	if err := saveNotes(file, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(notesPath(file)); !os.IsNotExist(err) {
		t.Fatal("emptying notes should remove the sidecar")
	}
}

func TestMoveAndDeleteNotesCoupling(t *testing.T) {
	dir := t.TempDir()
	oldFile := filepath.Join(dir, "old.md")
	newFile := filepath.Join(dir, "new.md")
	os.WriteFile(oldFile, []byte("x"), 0o644)
	saveNotes(oldFile, []note{{ID: "n1", Text: "note"}})

	moveNotes(oldFile, newFile)
	if _, err := os.Stat(notesPath(oldFile)); !os.IsNotExist(err) {
		t.Fatal("old sidecar should be gone after moveNotes")
	}
	if loadNotes(newFile)[0].Text != "note" {
		t.Fatal("notes should follow the rename")
	}
	deleteNotes(newFile)
	if _, err := os.Stat(notesPath(newFile)); !os.IsNotExist(err) {
		t.Fatal("deleteNotes should drop the sidecar")
	}
}

func TestNotesScreenAddEditDelete(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.md")
	os.WriteFile(file, []byte("x"), 0o644)

	m := model{width: 80, height: 24, notes: notesModel{file: file}}
	m.screen = screenNotes

	// a → add; type; esc → commit + persist.
	mm, _ := m.updateNotes(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m = mm.(model)
	if !m.notes.adding {
		t.Fatal("a should start adding")
	}
	m.notes.area.SetValue("This chapter drags.")
	mm, _ = m.updateNotes(tea.KeyMsg{Type: tea.KeyEsc})
	m = mm.(model)
	if len(m.notes.notes) != 1 || loadNotes(file)[0].Text != "This chapter drags." {
		t.Fatalf("note not added/persisted: %+v", m.notes.notes)
	}

	// e → edit; change; esc → persist.
	mm, _ = m.updateNotes(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	m = mm.(model)
	m.notes.area.SetValue("Cut the flashback.")
	mm, _ = m.updateNotes(tea.KeyMsg{Type: tea.KeyEsc})
	m = mm.(model)
	if loadNotes(file)[0].Text != "Cut the flashback." {
		t.Fatalf("edit not persisted: %+v", loadNotes(file))
	}

	// d → confirm → y deletes + persists (sidecar removed).
	mm, _ = m.updateNotes(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m = mm.(model)
	if !m.notes.confirmDelete {
		t.Fatal("d should raise the delete confirm")
	}
	mm, _ = m.updateNotes(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	m = mm.(model)
	if len(m.notes.notes) != 0 {
		t.Fatal("note should be deleted")
	}
	if _, err := os.Stat(notesPath(file)); !os.IsNotExist(err) {
		t.Fatal("deleting the last note should remove the sidecar")
	}
}

func TestEnterAllNotesFlattensAcrossChapters(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "un"), 0o755)
	os.MkdirAll(filepath.Join(dir, "deux"), 0o755)
	os.WriteFile(filepath.Join(dir, "un", "un.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, "deux", "deux.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, manifestName), []byte(
		`{"schemaVersion":3,"title":"N","items":[`+
			`{"chapter":{"folder":"un","title":"Un","texts":[{"file":"un.md","title":"Un"}]}},`+
			`{"chapter":{"folder":"deux","title":"Deux","texts":[{"file":"deux.md","title":"Deux"}]}}]}`), 0o644)

	saveNotes(filepath.Join(dir, "un", "un.md"), []note{{ID: "n1", Text: "Première note."}})
	saveNotes(filepath.Join(dir, "deux", "deux.md"), []note{
		{ID: "n2", Text: "Deuxième note."},
		{ID: "n3", Text: "Troisième note."},
	})

	m := model{files: filelist{dir: dir}}
	m.enterAllNotes()

	if m.screen != screenAllNotes {
		t.Fatal("enterAllNotes should switch to screenAllNotes")
	}
	if len(m.allNotes.entries) != 3 {
		t.Fatalf("expected 3 flattened entries, got %d: %+v", len(m.allNotes.entries), m.allNotes.entries)
	}
	if m.allNotes.entries[0].chapterTitle != "Un" || m.allNotes.entries[0].n.Text != "Première note." {
		t.Fatalf("unexpected first entry: %+v", m.allNotes.entries[0])
	}
	if m.allNotes.entries[1].chapterTitle != "Deux" || m.allNotes.entries[1].n.Text != "Deuxième note." {
		t.Fatalf("unexpected second entry: %+v", m.allNotes.entries[1])
	}
	if m.allNotes.entries[2].n.Text != "Troisième note." {
		t.Fatalf("unexpected third entry: %+v", m.allNotes.entries[2])
	}
}

func TestEnterAllNotesIncludesResourceNotes(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "un"), 0o755)
	os.MkdirAll(filepath.Join(dir, "Ressources"), 0o755)
	os.WriteFile(filepath.Join(dir, "un", "un.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, "racine.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, "Ressources", "premisse.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, manifestName), []byte(
		`{"schemaVersion":3,"title":"N","items":[`+
			`{"chapter":{"folder":"un","title":"Un","texts":[{"file":"un.md","title":"Un"}]}}]}`), 0o644)

	saveNotes(filepath.Join(dir, "un", "un.md"), []note{{ID: "n1", Text: "Note de chapitre."}})
	saveNotes(filepath.Join(dir, "racine.md"), []note{{ID: "n2", Text: "Note racine."}})
	saveNotes(filepath.Join(dir, "Ressources", "premisse.md"), []note{{ID: "n3", Text: "Note sous-dossier."}})

	m := model{files: filelist{dir: dir}}
	m.enterAllNotes()

	if len(m.allNotes.entries) != 3 {
		t.Fatalf("expected 3 entries (chapter + 2 resources), got %d: %+v", len(m.allNotes.entries), m.allNotes.entries)
	}
	var texts []string
	for _, e := range m.allNotes.entries {
		texts = append(texts, e.n.Text)
	}
	for _, want := range []string{"Note de chapitre.", "Note racine.", "Note sous-dossier."} {
		found := false
		for _, got := range texts {
			if got == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("expected entry with text %q, got %+v", want, texts)
		}
	}
}

func TestEnterAllNotesSkipsChaptersWithoutNotes(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "un"), 0o755)
	os.WriteFile(filepath.Join(dir, "un", "un.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, manifestName), []byte(
		`{"schemaVersion":3,"title":"N","items":[`+
			`{"chapter":{"folder":"un","title":"Un","texts":[{"file":"un.md","title":"Un"}]}}]}`), 0o644)

	m := model{files: filelist{dir: dir}}
	m.enterAllNotes()

	if m.screen != screenAllNotes {
		t.Fatal("enterAllNotes should still switch to screenAllNotes with zero notes")
	}
	if len(m.allNotes.entries) != 0 {
		t.Fatalf("expected 0 entries, got %d", len(m.allNotes.entries))
	}
}

func TestUpdateAllNotesNavigationAndEsc(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "un"), 0o755)
	os.WriteFile(filepath.Join(dir, "un", "un.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, manifestName), []byte(
		`{"schemaVersion":3,"title":"N","items":[`+
			`{"chapter":{"folder":"un","title":"Un","texts":[{"file":"un.md","title":"Un"}]}}]}`), 0o644)
	saveNotes(filepath.Join(dir, "un", "un.md"), []note{
		{ID: "n1", Text: "Une."},
		{ID: "n2", Text: "Deux."},
	})

	m := model{width: 80, height: 24, files: filelist{dir: dir}}
	m.enterAllNotes()

	mm, _ := m.updateAllNotes(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = mm.(model)
	if m.allNotes.sel != 1 {
		t.Fatalf("j should move selection down, got sel=%d", m.allNotes.sel)
	}
	mm, _ = m.updateAllNotes(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	m = mm.(model)
	if m.allNotes.sel != 0 {
		t.Fatalf("k should move selection up, got sel=%d", m.allNotes.sel)
	}
	mm, _ = m.updateAllNotes(tea.KeyMsg{Type: tea.KeyEsc})
	m = mm.(model)
	if m.screen != screenWriting || m.focus != focusSidebar {
		t.Fatal("esc should return to screenWriting with sidebar focus")
	}
}

// A loose file and a same-stem folder resolve to ONE sidecar path — so a directory rename that ran
// moveNotes would steal the file's notes. This pins the collision that makes the rename handler's
// `if !t.isDir { moveNotes(...) }` gate load-bearing; if this ever stops colliding, revisit the gate.
func TestNotesSidecarStemCollisionJustifiesDirGate(t *testing.T) {
	dir := t.TempDir()
	fileSidecar := notesPath(filepath.Join(dir, "interlude.md"))
	dirSidecar := notesPath(filepath.Join(dir, "interlude"))
	if fileSidecar != dirSidecar {
		t.Fatalf("expected file and same-stem dir to share a sidecar path (%q vs %q)", fileSidecar, dirSidecar)
	}
}
