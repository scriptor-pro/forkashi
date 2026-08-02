package main

import (
	"archive/zip"
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestWriteEPUBManuscriptValid(t *testing.T) {
	doc := ManuscriptDoc{
		{Title: "one", Blocks: []Block{Paragraph{Runs: []Run{{Text: "Plain line."}}}}},
		{Title: "two", Blocks: []Block{Paragraph{Runs: []Run{{Text: "Has "}, {Text: "bold", Bold: true}}}}},
	}
	out, err := writeEPUB(doc, StyleManuscript, Meta{Author: "Doe", Title: "The Garden"}, "")
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(out), int64(len(out)))
	if err != nil {
		t.Fatalf("output is not a valid zip: %v", err)
	}
	var names []string
	for _, f := range zr.File {
		names = append(names, f.Name)
	}
	for _, want := range []string{"mimetype", "META-INF/container.xml", "OEBPS/content.opf"} {
		found := false
		for _, n := range names {
			if n == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("EPUB missing entry %q, got entries: %v", want, names)
		}
	}
}

func TestWriteEPUBMimetypeStoredNotDeflated(t *testing.T) {
	doc := ManuscriptDoc{{Title: "one", Blocks: []Block{Paragraph{Runs: []Run{{Text: "x"}}}}}}
	out, err := writeEPUB(doc, StyleManuscript, Meta{Title: "T"}, "")
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(out), int64(len(out)))
	if err != nil {
		t.Fatal(err)
	}
	if len(zr.File) == 0 || zr.File[0].Name != "mimetype" {
		t.Fatalf("mimetype must be the first entry in the zip, got first entry: %v", zr.File[0].Name)
	}
	if zr.File[0].Method != zip.Store {
		t.Errorf("mimetype must be stored (uncompressed), got method %d", zr.File[0].Method)
	}
	rc, err := zr.File[0].Open()
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	var buf bytes.Buffer
	buf.ReadFrom(rc)
	if buf.String() != "application/epub+zip" {
		t.Errorf("mimetype content = %q, want %q", buf.String(), "application/epub+zip")
	}
}

func TestWriteEPUBContentOPFHasMetadata(t *testing.T) {
	doc := ManuscriptDoc{{Title: "one", Blocks: []Block{Paragraph{Runs: []Run{{Text: "x"}}}}}}
	out, err := writeEPUB(doc, StyleManuscript, Meta{Author: "Jane Doe", Title: "My Book"}, "")
	if err != nil {
		t.Fatal(err)
	}
	zr, _ := zip.NewReader(bytes.NewReader(out), int64(len(out)))
	for _, f := range zr.File {
		if f.Name != "OEBPS/content.opf" {
			continue
		}
		rc, _ := f.Open()
		defer rc.Close()
		var buf bytes.Buffer
		buf.ReadFrom(rc)
		s := buf.String()
		if !strings.Contains(s, "Jane Doe") {
			t.Error("content.opf missing author")
		}
		if !strings.Contains(s, "My Book") {
			t.Error("content.opf missing title")
		}
		return
	}
	t.Fatal("OEBPS/content.opf not found in archive")
}

func TestWriteEPUBTableOfContentsOrder(t *testing.T) {
	doc := ManuscriptDoc{
		{Title: "Premier", Blocks: []Block{Paragraph{Runs: []Run{{Text: "a"}}}}},
		{Title: "Second", Blocks: []Block{Paragraph{Runs: []Run{{Text: "b"}}}}},
		{Title: "Troisième", Blocks: []Block{Paragraph{Runs: []Run{{Text: "c"}}}}},
	}
	out, err := writeEPUB(doc, StyleManuscript, Meta{Title: "T"}, "")
	if err != nil {
		t.Fatal(err)
	}
	zr, _ := zip.NewReader(bytes.NewReader(out), int64(len(out)))
	for _, f := range zr.File {
		if f.Name != "OEBPS/toc.ncx" {
			continue
		}
		rc, _ := f.Open()
		defer rc.Close()
		var buf bytes.Buffer
		buf.ReadFrom(rc)
		s := buf.String()
		iP := strings.Index(s, "Premier")
		iS := strings.Index(s, "Second")
		iT := strings.Index(s, "Troisième")
		if iP < 0 || iS < 0 || iT < 0 || !(iP < iS && iS < iT) {
			t.Fatalf("toc.ncx entries not in document order: Premier=%d Second=%d Troisième=%d", iP, iS, iT)
		}
		return
	}
	t.Fatal("OEBPS/toc.ncx not found")
}

func TestWriteEPUBCoverImageValid(t *testing.T) {
	dir := t.TempDir()
	coverPath := dir + "/cover.png"
	// Minimal valid PNG signature + IHDR-less body is fine here: writeEPUB only reads and
	// embeds the bytes, it never decodes the image.
	pngBytes := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}
	if err := os.WriteFile(coverPath, pngBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	doc := ManuscriptDoc{{Title: "one", Blocks: []Block{Paragraph{Runs: []Run{{Text: "x"}}}}}}
	out, err := writeEPUB(doc, StyleManuscript, Meta{Title: "T"}, coverPath)
	if err != nil {
		t.Fatal(err)
	}
	zr, _ := zip.NewReader(bytes.NewReader(out), int64(len(out)))
	found := false
	for _, f := range zr.File {
		if f.Name == "OEBPS/cover.png" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected OEBPS/cover.png in the archive when a valid PNG cover path is given")
	}
}

func TestWriteEPUBCoverImageInvalidFallsBackToText(t *testing.T) {
	doc := ManuscriptDoc{{Title: "one", Blocks: []Block{Paragraph{Runs: []Run{{Text: "x"}}}}}}
	// Nonexistent path.
	out, err := writeEPUB(doc, StyleManuscript, Meta{Title: "Repli"}, "/no/such/file.png")
	if err != nil {
		t.Fatalf("a missing cover path must not fail the export: %v", err)
	}
	zr, _ := zip.NewReader(bytes.NewReader(out), int64(len(out)))
	for _, f := range zr.File {
		if f.Name == "OEBPS/cover.xhtml" {
			rc, _ := f.Open()
			defer rc.Close()
			var buf bytes.Buffer
			buf.ReadFrom(rc)
			if !strings.Contains(buf.String(), "Repli") {
				t.Error("fallback cover.xhtml should contain the generated text title")
			}
			return
		}
	}
	t.Fatal("OEBPS/cover.xhtml not found")
}

func TestWriteEPUBCoverImageUnsupportedExtFallsBackToText(t *testing.T) {
	dir := t.TempDir()
	coverPath := dir + "/cover.gif"
	os.WriteFile(coverPath, []byte("GIF89a"), 0o644)
	doc := ManuscriptDoc{{Title: "one", Blocks: []Block{Paragraph{Runs: []Run{{Text: "x"}}}}}}
	out, err := writeEPUB(doc, StyleManuscript, Meta{Title: "T"}, coverPath)
	if err != nil {
		t.Fatal(err)
	}
	zr, _ := zip.NewReader(bytes.NewReader(out), int64(len(out)))
	for _, f := range zr.File {
		if strings.HasSuffix(f.Name, ".gif") {
			t.Fatal("a .gif cover must not be embedded — only .jpg/.jpeg/.png are supported")
		}
	}
}
