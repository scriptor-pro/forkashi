package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestExportSingleDocFromEditor(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	proj := filepath.Join(root, "novel")
	os.MkdirAll(proj, 0o755)
	os.WriteFile(filepath.Join(proj, "02-the-letter.md"), []byte("She wrote **back**."), 0o644)
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = nm.(model)
	m.screen = screenWriting
	m.files.SetDir(proj)
	m.currentFile = filepath.Join(proj, "02-the-letter.md")

	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlE})
	m = nm.(model)
	if !m.exportPrompt {
		t.Fatal("ctrl+e should raise the export chooser")
	}
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	m = nm.(model)
	// single-doc title = sectionTitle -> "the letter" -> slug "the-letter"
	rtf := filepath.Join(proj, "export", "the-letter.rtf")
	pdf := filepath.Join(proj, "export", "the-letter.pdf")
	if b, err := os.ReadFile(rtf); err != nil || !bytes.Contains(b, []byte(`\rtf1`)) {
		t.Fatalf("expected an RTF at %s: %v", rtf, err)
	}
	if b, err := os.ReadFile(pdf); err != nil || !bytes.HasPrefix(b, []byte("%PDF")) {
		t.Fatalf("expected a PDF at %s: %v", pdf, err)
	}
}

func TestExportWholeManuscriptFromCorkboard(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	proj := filepath.Join(root, "my-novel")
	os.MkdirAll(filepath.Join(proj, "a"), 0o755)
	os.MkdirAll(filepath.Join(proj, "b"), 0o755)
	os.WriteFile(filepath.Join(proj, "a", "a.md"), []byte("alpha"), 0o644)
	os.WriteFile(filepath.Join(proj, "b", "b.md"), []byte("beta"), 0o644)
	writeManifest(proj, manifest{SchemaVersion: manifestSchemaVersion, Title: "my novel",
		Items: []manifestItem{
			{Chapter: &manifestChapter{Folder: "a", Title: "One", Texts: []manifestText{{File: "a.md", Title: "One"}}}},
			{Chapter: &manifestChapter{Folder: "b", Title: "Two", Texts: []manifestText{{File: "b.md", Title: "Two"}}}},
		}})
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = nm.(model)
	m.screen = screenWriting
	m.files.SetDir(proj)
	m.enterCorkboard() // full-screen corkboard → whole-manuscript export scope
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlE})
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}}) // tufte
	m = nm.(model)
	// whole-manuscript title = manifest title "my novel" -> slug "my-novel"
	if _, err := os.Stat(filepath.Join(proj, "export", "my-novel.pdf")); err != nil {
		t.Fatalf("expected the whole-manuscript PDF: %v", err)
	}
}

func TestExportManifestManuscriptUsesManifestOrder(t *testing.T) {
	dir := t.TempDir()
	// Write chapter folders in reverse alpha order; manifest orders them the-letter first.
	os.MkdirAll(filepath.Join(dir, "opening"), 0o755)
	os.MkdirAll(filepath.Join(dir, "the-letter"), 0o755)
	os.WriteFile(filepath.Join(dir, "opening", "opening.md"), []byte("chapter one text"), 0o644)
	os.WriteFile(filepath.Join(dir, "the-letter", "the-letter.md"), []byte("chapter two text"), 0o644)
	os.WriteFile(filepath.Join(dir, manifestName), []byte(
		`{"schemaVersion":2,"title":"Windermere","items":[`+
			`{"chapter":{"folder":"the-letter","title":"The Letter","texts":[{"file":"the-letter.md","title":"The Letter"}]}},`+
			`{"chapter":{"folder":"opening","title":"Chapter One","texts":[{"file":"opening.md","title":"Chapter One"}]}}]}`), 0o644)
	entries := readEntries(dir)
	v := resolveManuscript(dir, entries)
	doc := manuscriptDocFromChapters(dir, v.parts)
	if len(doc) != 2 {
		t.Fatalf("expected 2 sections, got %d", len(doc))
	}
	// Manifest order: The Letter first, Chapter One second.
	if doc[0].Title != "The Letter" {
		t.Fatalf("first section title = %q, want 'The Letter'", doc[0].Title)
	}
	if doc[1].Title != "Chapter One" {
		t.Fatalf("second section title = %q, want 'Chapter One'", doc[1].Title)
	}
	// Titles come from the manifest, not from filename slugs.
	if doc[0].Title == "the letter" || doc[1].Title == "opening" {
		t.Fatal("export must use manifest titles, not de-slugged filenames")
	}
}

func TestExportEmitsDOCX(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	proj := filepath.Join(root, "novel")
	os.MkdirAll(proj, 0o755)
	os.WriteFile(filepath.Join(proj, "02-the-letter.md"), []byte("She wrote **back**."), 0o644)
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = nm.(model)
	m.screen = screenWriting
	m.files.SetDir(proj)
	m.currentFile = filepath.Join(proj, "02-the-letter.md")

	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlE})
	m = nm.(model)
	if !m.exportPrompt {
		t.Fatal("ctrl+e should raise the export chooser")
	}
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	m = nm.(model)
	// single-doc title = sectionTitle -> "the letter" -> slug "the-letter"
	docxPath := filepath.Join(proj, "export", "the-letter.docx")
	b, err := os.ReadFile(docxPath)
	if err != nil {
		t.Fatalf("expected a DOCX at %s: %v", docxPath, err)
	}
	if !hasZipEntry(b, "word/document.xml") {
		t.Fatalf("DOCX at %s is not a valid zip or missing word/document.xml", docxPath)
	}
}

func TestExportEmitsODT(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	proj := filepath.Join(root, "novel")
	os.MkdirAll(proj, 0o755)
	os.WriteFile(filepath.Join(proj, "02-the-letter.md"), []byte("She wrote **back**."), 0o644)
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = nm.(model)
	m.screen = screenWriting
	m.files.SetDir(proj)
	m.currentFile = filepath.Join(proj, "02-the-letter.md")

	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlE})
	m = nm.(model)
	if !m.exportPrompt {
		t.Fatal("ctrl+e should raise the export chooser")
	}
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	m = nm.(model)
	// single-doc title = sectionTitle -> "the letter" -> slug "the-letter"
	odtPath := filepath.Join(proj, "export", "the-letter.odt")
	b, err := os.ReadFile(odtPath)
	if err != nil {
		t.Fatalf("expected an ODT at %s: %v", odtPath, err)
	}
	if !hasZipEntry(b, "content.xml") {
		t.Fatalf("ODT at %s is not a valid zip or missing content.xml", odtPath)
	}
}

func TestExportCancel(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	os.WriteFile(filepath.Join(root, "x.md"), []byte("x"), 0o644)
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = nm.(model)
	m.screen = screenWriting
	m.files.SetDir(root)
	m.currentFile = filepath.Join(root, "x.md")
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlE})
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = nm.(model)
	if m.exportPrompt {
		t.Fatal("esc should dismiss the export chooser")
	}
	if _, err := os.Stat(filepath.Join(root, "export")); !os.IsNotExist(err) {
		t.Fatal("cancel should write nothing")
	}
}
