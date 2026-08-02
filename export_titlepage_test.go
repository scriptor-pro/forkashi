package main

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestManuscriptWordCount(t *testing.T) {
	doc := ManuscriptDoc{
		{Title: "One", Blocks: []Block{
			Paragraph{Runs: []Run{{Text: "one two three"}}},
			Heading{Level: 1, Runs: []Run{{Text: "a heading"}}}, // 2
			List{Items: []Paragraph{{Runs: []Run{{Text: "four five"}}}}},
		}},
		{Title: "Two", Blocks: []Block{
			Blockquote{Children: []Block{Paragraph{Runs: []Run{{Text: "six seven eight nine"}}}}},
		}},
	}
	if got, want := manuscriptWordCount(doc), 3+2+2+4; got != want {
		t.Fatalf("manuscriptWordCount = %d, want %d", got, want)
	}
}

func TestApproxWords(t *testing.T) {
	cases := map[int]string{
		82437: "~82,500 mots",
		120:   "~100 mots",
		950:   "~1,000 mots",
		1234:  "~1,000 mots",
		1250:  "~1,500 mots",
		0:     "~0 mots",
	}
	for n, want := range cases {
		if got := approxWords(n); got != want {
			t.Errorf("approxWords(%d) = %q, want %q", n, got, want)
		}
	}
}

func titlePageMeta() Meta {
	return Meta{
		Author:    "Jane Ledoux",
		Title:     "The Lighthouse",
		Contact:   "123 Rue Ordinaire\nMontréal · jane@example.com",
		TitlePage: true,
	}
}

func sampleDoc() ManuscriptDoc {
	return ManuscriptDoc{{Title: "Chapter One", Blocks: []Block{
		Paragraph{Runs: []Run{{Text: "The lamp was lit."}}},
	}}}
}

func TestRTFTitlePagePresentAndSuppressesHeader(t *testing.T) {
	out := string(writeRTF(sampleDoc(), StyleManuscript, titlePageMeta()))
	for _, want := range []string{"Jane Ledoux", "123 Rue Ordinaire", "by Jane Ledoux", `\titlepg`, `\headerf`} {
		if !strings.Contains(out, want) {
			t.Errorf("RTF title page missing %q", want)
		}
	}
	// Word count string (RTF escapes the ~ as-is; digits/commas survive).
	if !strings.Contains(out, "mots") {
		t.Error("RTF title page missing word count")
	}
	// The title page must precede the first chapter's page break.
	if i, j := strings.Index(out, "Jane Ledoux"), strings.Index(out, `\page`); i < 0 || j < 0 || i > j {
		t.Errorf("title page (%d) should precede first \\page (%d)", i, j)
	}
}

func TestRTFNoTitlePageForSingleChapter(t *testing.T) {
	out := string(writeRTF(sampleDoc(), StyleManuscript, Meta{Author: "Jane Ledoux", Title: "Chapter One"}))
	if strings.Contains(out, "by Jane Ledoux") || strings.Contains(out, `\titlepg`) {
		t.Error("single-chapter export should not get a title page")
	}
}

func docxDocumentXML(t *testing.T, b []byte) string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			rc, err := f.Open()
			if err != nil {
				t.Fatal(err)
			}
			defer rc.Close()
			data, err := io.ReadAll(rc)
			if err != nil {
				t.Fatal(err)
			}
			return string(data)
		}
	}
	t.Fatal("word/document.xml not found")
	return ""
}

func TestDOCXTitlePagePresentWithPageBreak(t *testing.T) {
	b, err := writeDOCX(sampleDoc(), StyleManuscript, titlePageMeta())
	if err != nil {
		t.Fatal(err)
	}
	xml := docxDocumentXML(t, b)
	for _, want := range []string{"Jane Ledoux", "123 Rue Ordinaire", "by Jane Ledoux", "mots"} {
		if !strings.Contains(xml, want) {
			t.Errorf("DOCX title page missing %q", want)
		}
	}
	// Chapter One follows the title page and must start on a new page.
	ti, ci := strings.Index(xml, "by Jane Ledoux"), strings.Index(xml, "Chapter One")
	if ti < 0 || ci < 0 || ti > ci {
		t.Errorf("title page (%d) should precede chapter (%d)", ti, ci)
	}
	if !strings.Contains(xml, "<w:pageBreakBefore/>") {
		t.Error("first chapter after a title page must have a page break")
	}
}

func TestDOCXNoTitlePageForSingleChapter(t *testing.T) {
	b, err := writeDOCX(sampleDoc(), StyleManuscript, Meta{Author: "Jane Ledoux", Title: "Chapter One"})
	if err != nil {
		t.Fatal(err)
	}
	xml := docxDocumentXML(t, b)
	if strings.Contains(xml, "by Jane Ledoux") {
		t.Error("single-chapter export should not get a title page")
	}
	// Without a title page, the sole chapter should not force a page break.
	if strings.Contains(xml, "<w:pageBreakBefore/>") {
		t.Error("single chapter without a title page should not page-break")
	}
}

func TestPDFTitlePageRenders(t *testing.T) {
	// Can't introspect rendered PDF text here; assert it renders without error and is non-trivial.
	b, err := writePDF(sampleDoc(), StyleManuscript, titlePageMeta())
	if err != nil {
		t.Fatal(err)
	}
	if len(b) < 500 || !bytes.HasPrefix(b, []byte("%PDF")) {
		t.Fatalf("PDF with title page looks wrong (%d bytes)", len(b))
	}
}
