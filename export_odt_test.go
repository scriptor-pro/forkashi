package main

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

// odtPart unzips an .odt and returns the named part's content as a string.
func odtPart(t *testing.T, data []byte, name string) string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("output is not a valid zip: %v", err)
	}
	for _, f := range zr.File {
		if f.Name == name {
			rc, _ := f.Open()
			var b bytes.Buffer
			b.ReadFrom(rc)
			rc.Close()
			return b.String()
		}
	}
	t.Fatalf("part %q missing", name)
	return ""
}

func TestWriteODTStructureAndContent(t *testing.T) {
	doc := ManuscriptDoc{{
		Title: "Chapitre Un",
		Blocks: []Block{
			Paragraph{Runs: []Run{{Text: "Texte "}, {Text: "gras", Bold: true}, {Text: " et fin."}}},
			SceneBreak{},
			Paragraph{Runs: []Run{{Text: "Après la coupure."}}},
		},
	}}
	out, err := writeODT(doc, StyleManuscript, Meta{Author: "A. Autrice", Title: "Livre"})
	if err != nil {
		t.Fatal(err)
	}

	// mimetype must be the first entry, stored (uncompressed) — required by the ODF spec.
	zr, err := zip.NewReader(bytes.NewReader(out), int64(len(out)))
	if err != nil {
		t.Fatalf("output is not a valid zip: %v", err)
	}
	if len(zr.File) == 0 || zr.File[0].Name != "mimetype" {
		t.Fatalf("mimetype must be the first zip entry, got %+v", zr.File)
	}
	if zr.File[0].Method != zip.Store {
		t.Fatalf("mimetype must be stored uncompressed, got method %d", zr.File[0].Method)
	}

	for _, part := range []string{"mimetype", "META-INF/manifest.xml", "content.xml", "styles.xml"} {
		found := false
		for _, f := range zr.File {
			if f.Name == part {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing required part %q", part)
		}
	}

	content := odtPart(t, out, "content.xml")
	if !strings.Contains(content, "Chapitre Un") {
		t.Fatalf("chapter title missing:\n%s", content)
	}
	if !strings.Contains(content, "Après la coupure.") {
		t.Fatalf("post-scene-break text missing")
	}
	if !strings.Contains(content, "gras") {
		t.Fatalf("bold run text missing")
	}
}

func TestWriteODTEscapesXML(t *testing.T) {
	doc := ManuscriptDoc{{
		Title:  "T",
		Blocks: []Block{Paragraph{Runs: []Run{{Text: "A & B < C"}}}},
	}}
	out, err := writeODT(doc, StyleManuscript, Meta{Title: "T"})
	if err != nil {
		t.Fatal(err)
	}
	content := odtPart(t, out, "content.xml")
	if !strings.Contains(content, "&amp;") {
		t.Fatalf("& must be XML-escaped to &amp;:\n%s", content)
	}
	if strings.Contains(content, "A & B") {
		t.Fatalf("raw & must not appear unescaped in XML")
	}
}
