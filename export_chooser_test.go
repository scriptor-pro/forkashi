package main

import "testing"

func TestNewExportChooserNothingCheckedByDefault(t *testing.T) {
	c := newExportChooser()
	if c.anyChecked() {
		t.Fatal("a fresh chooser should have nothing checked")
	}
}

func TestExportChooserToggle(t *testing.T) {
	c := newExportChooser()
	c.cursor = 0 // first row = formatRTF
	c.toggleAtCursor()
	if !c.checked[formatRTF] {
		t.Fatal("toggling the RTF row should check it")
	}
	c.toggleAtCursor()
	if c.checked[formatRTF] {
		t.Fatal("toggling again should uncheck it")
	}
}

func TestExportChooserStyleDefaultsManuscript(t *testing.T) {
	c := newExportChooser()
	if c.style != StyleManuscript {
		t.Fatalf("default style = %v, want StyleManuscript", c.style)
	}
}

func TestExportChooserAnyCheckedAfterFormatToggle(t *testing.T) {
	c := newExportChooser()
	c.cursor = 4 // formatEPUB row
	c.toggleAtCursor()
	if !c.anyChecked() {
		t.Fatal("checking EPUB should make anyChecked true")
	}
}
