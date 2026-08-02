package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// checkFormat drives the export chooser: ctrl+e already pressed, m.exportChooser != nil.
// Moves the cursor to fmt's row and presses space, leaving the chooser open (caller presses
// enter separately) so a test can check multiple formats before submitting.
func checkFormat(m model, f exportFormat) model {
	row := 0
	for i, ff := range exportFormatOrder {
		if ff == f {
			row = i
		}
	}
	for m.exportChooser.cursor != row {
		nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = nm.(model)
	}
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	return nm.(model)
}

func submitExport(m model) model {
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	return nm.(model)
}

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
	if m.exportChooser == nil {
		t.Fatal("ctrl+e should raise the export chooser")
	}
	m = checkFormat(m, formatRTF)
	m = checkFormat(m, formatPDF)
	m = submitExport(m)

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
	os.MkdirAll(proj, 0o755)
	os.WriteFile(filepath.Join(proj, "01-a.md"), []byte("alpha"), 0o644)
	os.WriteFile(filepath.Join(proj, "02-b.md"), []byte("beta"), 0o644)
	writeManifest(proj, manifest{SchemaVersion: manifestSchemaVersion, Title: "my novel",
		Items: []manifestItem{{File: "01-a.md", Title: "One"}, {File: "02-b.md", Title: "Two"}}})
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = nm.(model)
	m.screen = screenWriting
	m.files.SetDir(proj)
	m.enterCorkboard()
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlE})
	m = nm.(model)
	m = checkFormat(m, formatPDF)
	// Switch to Tufte: move cursor to the style row (index len(exportFormatOrder)) and toggle.
	for m.exportChooser.cursor != len(exportFormatOrder) {
		nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = nm.(model)
	}
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	m = nm.(model)
	m = submitExport(m)

	if _, err := os.Stat(filepath.Join(proj, "export", "my-novel.pdf")); err != nil {
		t.Fatalf("expected the whole-manuscript PDF: %v", err)
	}
}

func TestExportManifestManuscriptUsesManifestOrder(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "opening.md"), []byte("chapter one text"), 0o644)
	os.WriteFile(filepath.Join(dir, "the-letter.md"), []byte("chapter two text"), 0o644)
	os.WriteFile(filepath.Join(dir, manifestName), []byte(
		`{"schemaVersion":1,"title":"Windermere","items":[`+
			`{"file":"the-letter.md","title":"The Letter"},`+
			`{"file":"opening.md","title":"Chapter One"}]}`), 0o644)
	entries := readEntries(dir)
	v := resolveManuscript(dir, entries)
	doc := manuscriptDocFromChapters(dir, v.chapters)
	if len(doc) != 2 {
		t.Fatalf("expected 2 sections, got %d", len(doc))
	}
	if doc[0].Title != "The Letter" {
		t.Fatalf("first section title = %q, want 'The Letter'", doc[0].Title)
	}
	if doc[1].Title != "Chapter One" {
		t.Fatalf("second section title = %q, want 'Chapter One'", doc[1].Title)
	}
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
	if m.exportChooser == nil {
		t.Fatal("ctrl+e should raise the export chooser")
	}
	m = checkFormat(m, formatDOCX)
	m = submitExport(m)

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
	if m.exportChooser == nil {
		t.Fatal("ctrl+e should raise the export chooser")
	}
	m = checkFormat(m, formatODT)
	m = submitExport(m)

	odtPath := filepath.Join(proj, "export", "the-letter.odt")
	b, err := os.ReadFile(odtPath)
	if err != nil {
		t.Fatalf("expected an ODT at %s: %v", odtPath, err)
	}
	if !hasZipEntry(b, "content.xml") {
		t.Fatalf("ODT at %s is not a valid zip or missing content.xml", odtPath)
	}
}

func TestExportEmitsEPUB(t *testing.T) {
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
	if m.exportChooser == nil {
		t.Fatal("ctrl+e should raise the export chooser")
	}
	m = checkFormat(m, formatEPUB)
	m = submitExport(m)

	epubPath := filepath.Join(proj, "export", "the-letter.epub")
	b, err := os.ReadFile(epubPath)
	if err != nil {
		t.Fatalf("expected an EPUB at %s: %v", epubPath, err)
	}
	if !hasZipEntry(b, "mimetype") {
		t.Fatalf("EPUB at %s is not a valid zip or missing mimetype", epubPath)
	}
}

func TestExportOnlyWritesCheckedFormats(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	proj := filepath.Join(root, "novel")
	os.MkdirAll(proj, 0o755)
	os.WriteFile(filepath.Join(proj, "02-the-letter.md"), []byte("Text."), 0o644)
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = nm.(model)
	m.screen = screenWriting
	m.files.SetDir(proj)
	m.currentFile = filepath.Join(proj, "02-the-letter.md")

	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlE})
	m = nm.(model)
	m = checkFormat(m, formatEPUB) // ONLY epub checked
	m = submitExport(m)

	if _, err := os.Stat(filepath.Join(proj, "export", "the-letter.epub")); err != nil {
		t.Fatalf("expected the-letter.epub to exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(proj, "export", "the-letter.pdf")); !os.IsNotExist(err) {
		t.Fatal("the-letter.pdf should NOT exist — only EPUB was checked")
	}
	if _, err := os.Stat(filepath.Join(proj, "export", "the-letter.rtf")); !os.IsNotExist(err) {
		t.Fatal("the-letter.rtf should NOT exist — only EPUB was checked")
	}
}

func TestExportEnterWithNothingCheckedShowsError(t *testing.T) {
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
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // nothing checked yet
	m = nm.(model)
	if m.exportChooser == nil {
		t.Fatal("enter with nothing checked must NOT close the chooser")
	}
	if m.status != "choisissez au moins un format" {
		t.Fatalf("status = %q, want the no-format-checked warning", m.status)
	}
	if _, err := os.Stat(filepath.Join(root, "export")); !os.IsNotExist(err) {
		t.Fatal("nothing should have been exported")
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
	if m.exportChooser != nil {
		t.Fatal("esc should dismiss the export chooser")
	}
	if _, err := os.Stat(filepath.Join(root, "export")); !os.IsNotExist(err) {
		t.Fatal("cancel should write nothing")
	}
}
