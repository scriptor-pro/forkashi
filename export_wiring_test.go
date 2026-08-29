package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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
	// Write chapter folders in reverse alpha order; manifest orders them the-letter first.
	os.MkdirAll(filepath.Join(dir, "opening"), 0o755)
	os.MkdirAll(filepath.Join(dir, "the-letter"), 0o755)
	os.WriteFile(filepath.Join(dir, "opening", "opening.md"), []byte("chapter one text"), 0o644)
	os.WriteFile(filepath.Join(dir, "the-letter", "the-letter.md"), []byte("chapter two text"), 0o644)
	os.WriteFile(filepath.Join(dir, manifestName), []byte(
		`{"schemaVersion":3,"title":"Windermere","items":[`+
			`{"chapter":{"folder":"the-letter","title":"The Letter","texts":[{"file":"the-letter.md","title":"The Letter"}]}},`+
			`{"chapter":{"folder":"opening","title":"Chapter One","texts":[{"file":"opening.md","title":"Chapter One"}]}}]}`), 0o644)
	entries := readEntries(dir)
	v := resolveManuscript(dir, entries)
	doc := manuscriptDocFromChapters(dir, v.parts, nil)
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

func TestRunExportWholeManuscriptExcludesSelectedText(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	proj := filepath.Join(root, "novel")
	os.MkdirAll(filepath.Join(proj, "ch1"), 0o755)
	os.WriteFile(filepath.Join(proj, "ch1", "a.md"), []byte("Kept prose."), 0o644)
	os.WriteFile(filepath.Join(proj, "ch1", "b.md"), []byte("Excluded prose."), 0o644)
	if err := writeManifest(proj, manifest{
		Title: "Novel",
		Items: []manifestItem{
			{Chapter: &manifestChapter{Folder: "ch1", Title: "C1", Texts: []manifestText{
				{File: "a.md", Title: "Kept"},
				{File: "b.md", Title: "Excluded"},
			}}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := saveExportSelection(proj, map[string]bool{filepath.Join("ch1", "b.md"): true},
		map[string]bool{filepath.Join("ch1", "a.md"): true, filepath.Join("ch1", "b.md"): true}); err != nil {
		t.Fatal(err)
	}

	m := model{}
	m.files.dir = proj
	m.screen = screenCorkboard // exportWholeManuscript() gates on this
	m.currentFile = filepath.Join(proj, "ch1", "a.md")
	m.exportChooser = &exportChooserModel{checked: map[exportFormat]bool{formatRTF: true}, style: StyleManuscript}
	m.runExport()

	out, err := os.ReadFile(filepath.Join(proj, "export", "novel.rtf"))
	if err != nil {
		t.Fatalf("export file not written: %v", err)
	}
	body := string(out)
	if !strings.Contains(body, "Kept") {
		t.Fatal("exported RTF must contain the non-excluded scene's content")
	}
	if strings.Contains(body, "Excluded prose") {
		t.Fatal("exported RTF must NOT contain the excluded scene's content")
	}
}

func TestRunExportWholeManuscriptIncludesCheckedResource(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	proj := filepath.Join(root, "novel")
	os.MkdirAll(filepath.Join(proj, "ch1"), 0o755)
	os.WriteFile(filepath.Join(proj, "ch1", "a.md"), []byte("Chapter prose."), 0o644)
	os.WriteFile(filepath.Join(proj, "sidenote.md"), []byte("A resource, opted into export."), 0o644)
	if err := writeManifest(proj, manifest{
		Title: "Novel",
		Items: []manifestItem{
			{Chapter: &manifestChapter{Folder: "ch1", Title: "C1", Texts: []manifestText{{File: "a.md", Title: "A"}}}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	// sidenote.md is explicitly included (false in the sidecar) — a Resource now defaults to
	// EXCLUDED, so an explicit opt-in is required for it to appear (Task 3).
	if err := saveExportSelectionRaw(t, proj, map[string]bool{"sidenote.md": false}); err != nil {
		t.Fatal(err)
	}

	m := model{}
	m.files.dir = proj
	m.screen = screenCorkboard
	m.exportChooser = &exportChooserModel{checked: map[exportFormat]bool{formatRTF: true}, style: StyleManuscript}
	m.runExport()

	out, err := os.ReadFile(filepath.Join(proj, "export", "novel.rtf"))
	if err != nil {
		t.Fatalf("export file not written: %v", err)
	}
	if !strings.Contains(string(out), "resource") {
		t.Fatal("exported RTF must include the Resource's content when explicitly opted in")
	}
}

func TestRunExportCurrentDocumentUnaffectedBySelection(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	proj := filepath.Join(root, "novel")
	os.MkdirAll(filepath.Join(proj, "ch1"), 0o755)
	os.WriteFile(filepath.Join(proj, "ch1", "a.md"), []byte("Solo document text."), 0o644)
	if err := writeManifest(proj, manifest{
		Title: "Novel",
		Items: []manifestItem{
			{Chapter: &manifestChapter{Folder: "ch1", Title: "C1", Texts: []manifestText{{File: "a.md", Title: "A"}}}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	// Exclude a.md in the sidecar — must have NO effect on a single-document export.
	if err := saveExportSelection(proj, map[string]bool{filepath.Join("ch1", "a.md"): true},
		map[string]bool{filepath.Join("ch1", "a.md"): true}); err != nil {
		t.Fatal(err)
	}

	m := model{}
	m.files.dir = proj
	m.screen = screenWriting // NOT corkboard — single-document export path
	m.currentFile = filepath.Join(proj, "ch1", "a.md")
	m.exportChooser = &exportChooserModel{checked: map[exportFormat]bool{formatRTF: true}, style: StyleManuscript}
	m.runExport()

	out, err := os.ReadFile(filepath.Join(proj, "ch1", "export", "a.rtf"))
	if err != nil {
		t.Fatalf("export file not written: %v", err)
	}
	if !strings.Contains(string(out), "Solo document") {
		t.Fatal("single-document export must be unaffected by the manuscript-wide exclusion sidecar")
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

// saveExportSelectionRaw writes the sidecar with the exact map given, bypassing
// saveExportSelection's pruning of false values — needed only to test buildEntry/runExport's
// read-side "explicit key always wins" contract for a value production code never writes.
func saveExportSelectionRaw(t *testing.T, dir string, excluded map[string]bool) error {
	t.Helper()
	data, err := json.Marshal(exportSelectionFile{SchemaVersion: exportSelectionSchemaVersion, Excluded: excluded})
	if err != nil {
		return err
	}
	return os.WriteFile(exportSelectionPath(dir), data, 0o644)
}

func TestRunExportResourceExcludedByDefaultWithoutExplicitSidecarEntry(t *testing.T) {
	dir := t.TempDir()
	mkChapterDir(t, dir, "ch1", map[string]string{"scene-1.md": "Chapter text."})
	os.WriteFile(filepath.Join(dir, "notes.md"), []byte("A Resource, never touched."), 0o644)
	if err := writeManifest(dir, manifest{
		Title: "N",
		Items: []manifestItem{
			{Chapter: &manifestChapter{Folder: "ch1", Title: "Chapter One", Texts: []manifestText{
				{File: "scene-1.md", Title: "Opening"},
			}}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	m := model{}
	m.files.dir = dir
	m.screen = screenCorkboard // exportWholeManuscript() == true
	c := newExportChooser()
	c.checked[formatRTF] = true
	m.exportChooser = &c

	m.runExport()

	data, err := os.ReadFile(filepath.Join(dir, "export", "n.rtf"))
	if err != nil {
		t.Fatalf("expected export/n.rtf to be written: %v, status=%q", err, m.status)
	}
	if strings.Contains(string(data), "A Resource, never touched.") {
		t.Fatal("a Resource with no explicit sidecar entry must be excluded from the export by default")
	}
}

func TestRunExportResourceExplicitlyIncludedStillAppears(t *testing.T) {
	dir := t.TempDir()
	mkChapterDir(t, dir, "ch1", map[string]string{"scene-1.md": "Chapter text."})
	os.WriteFile(filepath.Join(dir, "notes.md"), []byte("An explicitly included Resource."), 0o644)
	if err := writeManifest(dir, manifest{
		Title: "N",
		Items: []manifestItem{
			{Chapter: &manifestChapter{Folder: "ch1", Title: "Chapter One", Texts: []manifestText{
				{File: "scene-1.md", Title: "Opening"},
			}}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	// Simulate an explicit "included" state the way the export-selection screen produces it:
	// toggling a Resource on twice (excluded → included) leaves no key in the sidecar at all,
	// which already defaults to included for listed items but NOT for Resources — so to keep a
	// Resource included, the UI never writes true. The only way "explicit inclusion" of a
	// Resource actually persists is by the sidecar not pruning a false-equivalent absence — this
	// test instead verifies the direct runExport contract using an explicit `false` sidecar
	// entry, which is what the schema is capable of storing.
	if err := saveExportSelectionRaw(t, dir, map[string]bool{"notes.md": false}); err != nil {
		t.Fatal(err)
	}
	m := model{}
	m.files.dir = dir
	m.screen = screenCorkboard
	c := newExportChooser()
	c.checked[formatRTF] = true
	m.exportChooser = &c

	m.runExport()

	data, err := os.ReadFile(filepath.Join(dir, "export", "n.rtf"))
	if err != nil {
		t.Fatalf("expected export/n.rtf to be written: %v, status=%q", err, m.status)
	}
	if !strings.Contains(string(data), "An explicitly included Resource.") {
		t.Fatal("a Resource with an explicit false sidecar entry must still be included")
	}
}
