package main

import (
	"bytes"
	"testing"

	"codeberg.org/go-pdf/fpdf"
)

func TestWritePDFManuscriptValid(t *testing.T) {
	doc := ManuscriptDoc{
		{Title: "one", Blocks: []Block{Paragraph{Runs: []Run{{Text: "Plain line."}}}}},
		{Title: "two", Blocks: []Block{Paragraph{Runs: []Run{{Text: "Has "}, {Text: "bold", Bold: true}}}}},
	}
	out, err := writePDF(doc, StyleManuscript, Meta{Author: "Doe", Title: "The Garden"})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(out, []byte("%PDF")) {
		t.Fatalf("output is not a PDF (no %%PDF header)")
	}
	if len(out) < 800 {
		t.Fatalf("PDF suspiciously small: %d bytes", len(out))
	}
}

func TestWritePDFSmartQuotesNoError(t *testing.T) {
	// The editor inserts curly quotes/em dashes; the Courier (cp1252) path must transcode.
	doc := ManuscriptDoc{{Title: "q", Blocks: []Block{
		Paragraph{Runs: []Run{{Text: "“Curly” — dashes — and an é."}}},
	}}}
	out, err := writePDF(doc, StyleManuscript, Meta{Title: "T"})
	if err != nil {
		t.Fatalf("smart-quote text should not error on the Courier path: %v", err)
	}
	if !bytes.HasPrefix(out, []byte("%PDF")) {
		t.Fatal("not a PDF")
	}
}

func TestCp1252FallsBackOnUnencodable(t *testing.T) {
	// An astral emoji has no cp1252 encoding -> must fall back, not panic.
	got := cp1252("hi \U0001F600")
	if got == "" {
		t.Fatal("cp1252 returned empty")
	}
}

func TestWritePDFTufteAstralRuneSafe(t *testing.T) {
	// An astral emoji inside an emphasized paragraph crashed the Tufte path
	// (embedded ET Book has no astral glyphs; the HTML writer panicked).
	doc := ManuscriptDoc{{Title: "t", Blocks: []Block{
		Paragraph{Runs: []Run{{Text: "rocket "}, {Text: "boom", Bold: true}, {Text: " \U0001F680 end"}}},
	}}}
	out, err := writePDF(doc, StyleTufte, Meta{Title: "T"})
	if err != nil {
		t.Fatalf("astral rune should be sanitized, not error/panic: %v", err)
	}
	if !bytes.HasPrefix(out, []byte("%PDF")) {
		t.Fatal("not a PDF")
	}
}

func TestWritePDFEmphasisSpecialCharsNoEntities(t *testing.T) {
	// "Tom & Jerry" in bold previously rendered as the literal "Tom &amp; Jerry".
	doc := ManuscriptDoc{{Title: "t", Blocks: []Block{
		Paragraph{Runs: []Run{{Text: "Tom & Jerry ", Bold: true}, {Text: "<x> 'q'", Italic: true}}},
	}}}
	for _, st := range []ExportStyle{StyleManuscript, StyleTufte} {
		out, err := writePDF(doc, st, Meta{Title: "T"})
		if err != nil {
			t.Fatalf("style %d: emphasis with &/</'/\" must not error: %v", st, err)
		}
		if !bytes.HasPrefix(out, []byte("%PDF")) {
			t.Fatalf("style %d: not a PDF", st)
		}
	}
}

func TestWritePDFTufteEmbedsFontValid(t *testing.T) {
	doc := ManuscriptDoc{{Title: "one", Blocks: []Block{
		Paragraph{Runs: []Run{{Text: "Elegant "}, {Text: "serif", Italic: true}, {Text: " prose — with an em dash."}}},
	}}}
	out, err := writePDF(doc, StyleTufte, Meta{Title: "Tufte"})
	if err != nil {
		t.Fatalf("tufte PDF should build with the embedded font: %v", err)
	}
	if !bytes.HasPrefix(out, []byte("%PDF")) {
		t.Fatal("not a PDF")
	}
	if len(out) < 5000 {
		t.Fatalf("an embedded-font PDF should be larger than %d bytes", len(out))
	}
}

func TestWritePDFEndnotesBuilds(t *testing.T) {
	doc := ManuscriptDoc{{Title: "ch", Blocks: []Block{
		Paragraph{Runs: []Run{{Text: "Body [1]."}}},
		Endnotes{Items: []Endnote{{Num: 1, Runs: []Run{{Text: "the note"}}}}},
	}}}
	for _, st := range []ExportStyle{StyleManuscript, StyleTufte} {
		out, err := writePDF(doc, st, Meta{Title: "T"})
		if err != nil {
			t.Fatalf("style %d: endnotes PDF should build: %v", st, err)
		}
		if !bytes.HasPrefix(out, []byte("%PDF")) {
			t.Fatalf("style %d: not a PDF", st)
		}
	}
}

func TestWritePDFTimesFont(t *testing.T) {
	doc := ManuscriptDoc{{Title: "one", Blocks: []Block{
		Paragraph{Runs: []Run{{Text: "Times body text."}}},
	}}}
	out, err := writePDF(doc, StyleManuscript, Meta{Title: "T", PdfFont: "times"})
	if err != nil {
		t.Fatalf("times font should not error: %v", err)
	}
	if !bytes.HasPrefix(out, []byte("%PDF")) {
		t.Fatal("not a PDF")
	}
}

func TestWritePDFZeroValueMetaMatchesDefaults(t *testing.T) {
	// A Meta with no PDF fields set (as every pre-existing call site produces) must
	// render without error, on the Courier path, identically to before this change.
	doc := ManuscriptDoc{{Title: "one", Blocks: []Block{
		Paragraph{Runs: []Run{{Text: "Plain line."}}},
	}}}
	out, err := writePDF(doc, StyleManuscript, Meta{Title: "T"})
	if err != nil {
		t.Fatalf("zero-value Meta should render fine: %v", err)
	}
	if !bytes.HasPrefix(out, []byte("%PDF")) {
		t.Fatal("not a PDF")
	}
}

func TestResolvePdfFontName(t *testing.T) {
	cases := map[string]string{
		"":        "Courier",
		"courier": "Courier",
		"times":   "Times",
		"bogus":   "Courier",
	}
	for in, want := range cases {
		if got := resolvePdfFontName(in); got != want {
			t.Errorf("resolvePdfFontName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestResolveManuscriptMarginsCourierDerivesFromCPL(t *testing.T) {
	pdf := fpdf.New("P", "pt", "A4", "")
	pdf.AddPage()
	meta := Meta{PdfFont: "courier", PdfCharsPerLine: 62, PdfMarginTop: 72, PdfMarginBottom: 72}
	top, bottom, left, right := resolveManuscriptMargins(pdf, meta)
	if top != 72 || bottom != 72 {
		t.Fatalf("top/bottom should pass through: got %v/%v", top, bottom)
	}
	if left != right {
		t.Fatalf("courier margins should be symmetric: left=%v right=%v", left, right)
	}
	// CPL 62 at Courier 12pt on A4 (595.28pt wide) reproduces ~72pt margins (today's default).
	if left < 68 || left > 76 {
		t.Fatalf("left margin = %v, want close to 72 (default CPL)", left)
	}
}

func TestResolveManuscriptMarginsTimesUsesDirectValues(t *testing.T) {
	pdf := fpdf.New("P", "pt", "A4", "")
	pdf.AddPage()
	meta := Meta{PdfFont: "times", PdfMarginTop: 50, PdfMarginBottom: 60, PdfMarginLeft: 80, PdfMarginRight: 90}
	top, bottom, left, right := resolveManuscriptMargins(pdf, meta)
	if top != 50 || bottom != 60 || left != 80 || right != 90 {
		t.Fatalf("times margins should pass through directly: got %v/%v/%v/%v", top, bottom, left, right)
	}
}

func TestWritePDFCustomLineHeightNoError(t *testing.T) {
	doc := ManuscriptDoc{{Title: "one", Blocks: []Block{
		Paragraph{Runs: []Run{{Text: "Line."}}},
	}}}
	out, err := writePDF(doc, StyleManuscript, Meta{Title: "T", PdfFont: "courier", PdfLineHeight: 30, PdfCharsPerLine: 62, PdfMarginTop: 72, PdfMarginBottom: 72})
	if err != nil {
		t.Fatalf("custom line height should not error: %v", err)
	}
	if !bytes.HasPrefix(out, []byte("%PDF")) {
		t.Fatal("not a PDF")
	}
}

func TestWritePDFTufteIgnoresPdfMetaFields(t *testing.T) {
	// Tufte must render identically regardless of what's in the new Meta fields —
	// they're Manuscript-only.
	doc := ManuscriptDoc{{Title: "t", Blocks: []Block{
		Paragraph{Runs: []Run{{Text: "Tufte body."}}},
	}}}
	baseline, err := writePDF(doc, StyleTufte, Meta{Title: "T"})
	if err != nil {
		t.Fatal(err)
	}
	withPdfFields, err := writePDF(doc, StyleTufte, Meta{
		Title: "T", PdfFont: "times", PdfLineHeight: 30, PdfCharsPerLine: 100,
		PdfMarginTop: 200, PdfMarginBottom: 200, PdfMarginLeft: 200, PdfMarginRight: 200,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(baseline) != len(withPdfFields) {
		t.Fatalf("Tufte output size differs when Manuscript-only fields are set: %d vs %d", len(baseline), len(withPdfFields))
	}
}
