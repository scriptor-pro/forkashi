package main

import (
	"os"
	"strconv"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// A failed save from the unsaved-changes prompt must NOT navigate home (which would drop the edits).
func TestPropertiesSaveFailureFromConfirmStaysOnScreen(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses file permissions")
	}
	t.Setenv("OKASHI_WIDTH", "")
	dir := t.TempDir()
	m := model{properties: newPropertiesModel(dir)}
	m.files.dir = dir
	m.screen = screenProperties
	m.properties.width.SetValue("80") // dirty (per-project store)
	m.properties.confirmExit = true

	// Make the project dir unwritable so saveProjectSettings fails.
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(dir, 0o755)

	mm, _ := m.updateProperties(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	got := mm.(model)
	if got.screen == screenHome {
		t.Fatal("a failed save must not navigate home — edits would be lost")
	}
	if got.properties.width.Value() != "80" {
		t.Fatal("edits must be preserved after a failed save")
	}
}

func TestPropertiesViewRendersFields(t *testing.T) {
	dir := t.TempDir()
	m := model{width: 80, height: 24, properties: newPropertiesModel(dir)}
	out := m.propertiesView()
	for _, want := range []string{"propriétés", "Auteur", "Contact", "Largeur", "Guillemets typo."} {
		if !strings.Contains(out, want) {
			t.Errorf("propertiesView missing %q", want)
		}
	}
	// Navigating must not panic and must still render.
	mm, _ := m.updateProperties(tea.KeyMsg{Type: tea.KeyDown})
	if got := mm.(model).propertiesView(); got == "" {
		t.Fatal("view empty after nav")
	}
}

func TestPropertiesToggleSmartquotes(t *testing.T) {
	dir := t.TempDir()
	m := model{width: 80, height: 24, properties: newPropertiesModel(dir)}
	for i, k := range m.properties.fields {
		if k == propSmartquotes {
			m.properties.focus = i
		}
	}
	before := m.properties.smartquotes
	mm, _ := m.updateProperties(tea.KeyMsg{Type: tea.KeySpace})
	if mm.(model).properties.smartquotes == before {
		t.Fatal("space on the smartquotes field should toggle it")
	}
}

func TestPropertiesSaveOnlyChangedStores(t *testing.T) {
	t.Setenv("OKASHI_WIDTH", "")
	t.Setenv("OKASHI_SMARTQUOTES", "")
	dir := t.TempDir()

	p := newPropertiesModel(dir)
	p.width.SetValue("80") // change width only

	changed, err := p.save()
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("a width change should report projectChanged=true")
	}
	ps := loadProjectSettings(dir)
	if ps.Width == nil || *ps.Width != 80 {
		t.Fatalf("width not saved: %+v", ps)
	}
	// Smartquotes was untouched → must stay unwritten (per-field save, not force-materialized).
	if ps.Smartquotes != nil {
		t.Fatal("unchanged smartquotes must not be written to .okashi.json")
	}
	// A second save with nothing changed writes nothing new.
	if changed, _ := p.save(); changed {
		t.Fatal("no-op save should report projectChanged=false")
	}
}

func TestPropertiesNonManuscriptTitleReadOnly(t *testing.T) {
	dir := t.TempDir() // no manifest
	p := newPropertiesModel(dir)
	if p.isManuscript {
		t.Fatal("a bare temp dir is not a manuscript")
	}
	for _, k := range p.fields {
		if k == propTitle {
			t.Fatal("Title must be skipped in the tab order for a non-manifest dir")
		}
	}
}

func TestPropertiesManuscriptTitleSavedPreservingItems(t *testing.T) {
	dir := t.TempDir()
	if err := writeManifest(dir, manifest{
		SchemaVersion: manifestSchemaVersion,
		Title:         "Old Title",
		Items: []manifestItem{
			{Chapter: &manifestChapter{Folder: "one", Title: "One", Texts: []manifestText{{File: "one.md", Title: "One"}}}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	p := newPropertiesModel(dir)
	if !p.isManuscript || len(p.fields) == 0 || p.fields[0] != propTitle {
		t.Fatalf("manuscript should have Title as the first field: %+v", p.fields)
	}
	if p.origTitle != "Old Title" {
		t.Fatalf("title seeded from manifest, got %q", p.origTitle)
	}
	p.title.SetValue("New Title")
	if _, err := p.save(); err != nil {
		t.Fatal(err)
	}
	mani, _, _ := readManifest(dir)
	if mani.Title != "New Title" {
		t.Fatalf("manifest title = %q, want New Title", mani.Title)
	}
	if len(mani.Items) != 1 || mani.Items[0].Chapter == nil || mani.Items[0].Chapter.Folder != "one" {
		t.Fatalf("items must be preserved by the read-modify-write: %+v", mani.Items)
	}
}

func TestPropertiesWidthRevertsOnInvalid(t *testing.T) {
	t.Setenv("OKASHI_WIDTH", "")
	dir := t.TempDir()
	m := model{properties: newPropertiesModel(dir)}
	p := &m.properties
	for i, k := range p.fields {
		if k == propWidth {
			p.focus = i
		}
	}
	p.editing = true
	p.width.SetValue("999") // out of [20,200]

	mm, _ := m.updatePropertiesEditing(tea.KeyMsg{Type: tea.KeyEsc})
	got := mm.(model)
	want := strconv.Itoa(got.properties.origWidth)
	if got.properties.width.Value() != want {
		t.Fatalf("invalid width should revert to %q, got %q", want, got.properties.width.Value())
	}
	if got.properties.editing {
		t.Fatal("commit should leave editing mode")
	}
}

func TestPropertiesDirtyTracking(t *testing.T) {
	t.Setenv("OKASHI_WIDTH", "")
	dir := t.TempDir()
	p := newPropertiesModel(dir)
	if p.dirty() {
		t.Fatal("a freshly loaded model is not dirty")
	}
	p.author.SetValue(p.origAuthor + "X")
	if !p.dirty() {
		t.Fatal("a changed author should be dirty")
	}
	p.author.SetValue(p.origAuthor)
	if p.dirty() {
		t.Fatal("reverting a field to its original should clear dirty")
	}
}

func TestPropertiesCoverFieldLoadsAndSaves(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(dir+"/manifest.json", []byte(`{"schemaVersion":1,"title":"T","items":[]}`), 0o644)
	p := newPropertiesModel(dir)
	found := false
	for _, k := range p.fields {
		if k == propCover {
			found = true
		}
	}
	if !found {
		t.Fatal("propCover should be in the field order")
	}
	p.cover.SetValue("cover.jpg")
	if !p.dirty() {
		t.Fatal("changing Cover should mark the form dirty")
	}
	if _, err := p.save(); err != nil {
		t.Fatal(err)
	}
	ps := loadProjectSettings(dir)
	if ps.Cover == nil || *ps.Cover != "cover.jpg" {
		t.Fatalf("Cover not persisted, got %v", ps.Cover)
	}
}

func TestPropertiesCoverFieldDefaultsEmpty(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(dir+"/manifest.json", []byte(`{"schemaVersion":1,"title":"T","items":[]}`), 0o644)
	p := newPropertiesModel(dir)
	if p.cover.Value() != "" {
		t.Errorf("Cover default = %q, want empty", p.cover.Value())
	}
}

func TestNewPropertiesModelPdfDefaults(t *testing.T) {
	dir := t.TempDir()
	p := newPropertiesModel(dir)
	if p.pdfFont != "courier" {
		t.Errorf("pdfFont default = %q, want %q", p.pdfFont, "courier")
	}
	if p.pdfLineHeight.Value() != "24" {
		t.Errorf("pdfLineHeight default = %q, want %q", p.pdfLineHeight.Value(), "24")
	}
	if p.pdfCharsPerLine.Value() != "62" {
		t.Errorf("pdfCharsPerLine default = %q, want %q", p.pdfCharsPerLine.Value(), "62")
	}
}

func TestPropertiesFieldsShowCplForCourierMarginsForTimes(t *testing.T) {
	dir := t.TempDir()
	p := newPropertiesModel(dir)

	hasField := func(k propKind) bool {
		for _, f := range p.fields {
			if f == k {
				return true
			}
		}
		return false
	}

	if !hasField(propPdfCharsPerLine) {
		t.Error("courier (default) should show propPdfCharsPerLine")
	}
	if hasField(propPdfMarginLeft) || hasField(propPdfMarginRight) {
		t.Error("courier (default) should not show margin fields")
	}

	p.pdfFont = "times"
	p.rebuildFields()

	if hasField(propPdfCharsPerLine) {
		t.Error("times should not show propPdfCharsPerLine")
	}
	if !hasField(propPdfMarginLeft) || !hasField(propPdfMarginRight) || !hasField(propPdfMarginTop) || !hasField(propPdfMarginBottom) {
		t.Error("times should show all 4 margin fields")
	}
}

func TestPropertiesPdfDirtyTracking(t *testing.T) {
	dir := t.TempDir()
	p := newPropertiesModel(dir)
	if p.dirty() {
		t.Fatal("freshly loaded model should not be dirty")
	}
	p.pdfFont = "times"
	if !p.dirty() {
		t.Fatal("changing pdfFont should mark dirty")
	}
}

func TestPropertiesSavePdfFields(t *testing.T) {
	dir := t.TempDir()
	p := newPropertiesModel(dir)
	p.pdfFont = "times"
	p.rebuildFields()
	p.pdfLineHeight.SetValue("20")
	p.pdfMarginTop.SetValue("50")
	p.pdfMarginBottom.SetValue("50")
	p.pdfMarginLeft.SetValue("85")
	p.pdfMarginRight.SetValue("85")

	if _, err := p.save(); err != nil {
		t.Fatal(err)
	}

	ps := loadProjectSettings(dir)
	if ps.PdfFont == nil || *ps.PdfFont != "times" {
		t.Fatalf("saved PdfFont: %+v", ps.PdfFont)
	}
	if ps.PdfLineHeight == nil || *ps.PdfLineHeight != 20 {
		t.Fatalf("saved PdfLineHeight: %+v", ps.PdfLineHeight)
	}
	if ps.PdfMarginLeft == nil || *ps.PdfMarginLeft != 85 {
		t.Fatalf("saved PdfMarginLeft: %+v", ps.PdfMarginLeft)
	}
}

func TestPropertiesSaveInvalidPdfFieldReverts(t *testing.T) {
	dir := t.TempDir()
	p := newPropertiesModel(dir)
	p.pdfLineHeight.SetValue("not-a-number")
	// Simulate the commit path used by updatePropertiesEditing for an out-of-range/invalid value.
	p.commitPdfLineHeight()
	if p.pdfLineHeight.Value() != "24" {
		t.Fatalf("invalid line height should revert to original 24, got %q", p.pdfLineHeight.Value())
	}
}
