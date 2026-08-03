# Export EPUB + refonte du choix de format à l'export Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a fifth export format (EPUB) to okashi/forkashi, and replace the current fixed
"always write RTF+PDF+DOCX+ODT" export behavior with an explicit checkbox screen where the
user picks which format(s) to produce plus the Manuscript/Tufte style, per the approved
design.

**Architecture:** A new `export_epub.go` writer follows the exact pattern of the existing
`export_docx.go`/`export_rtf.go`/`export_odt.go` writers — it walks the same style-agnostic
AST (`export_ast.go`) that's already shared across all formats, producing a ZIP (EPUB
container) instead of OOXML/RTF/ODF XML. Separately, `m.exportPrompt` (currently a bool that
triggers a one-line `m`/`t` status prompt) becomes a small dedicated model
(`exportChooserModel`) with 5 format checkboxes + a style radio, rendered as a `framedPanel`
(the same bordered-box primitive already used for the corkboard's add-picker and synopsis
editor). `runExport()` is rewritten to loop over the chosen formats instead of hardcoding all
four. A new Properties field (`Couverture`) stores an optional cover-image path in
`<project>/.okashi.json`, read only at EPUB export time.

**Tech Stack:** Go 1.25, Bubble Tea/lipgloss (TUI), `archive/zip` (already used by
DOCX/ODT/EPUB), `encoding/xml`-free hand-built XHTML/OPF strings (matches the existing
writers' style — no template engine, no new dependency).

## Global Constraints

- The Go module stays `okashi` in `go.mod` — do not change it.
- Use Go 1.25+: `export PATH="$HOME/.local/go/bin:$PATH"` before any `go build`/`go vet`/`go test`.
- No new third-party dependency — everything is hand-built with the stdlib (`archive/zip`,
  `strings`, `fmt`), exactly like the existing DOCX/ODT/RTF/PDF writers.
- All file writes go through `atomicWrite(path string, data []byte, perm os.FileMode) error`
  (`atomicwrite.go`) — never write in place. This applies to the `.epub` file itself and to
  any copied cover image.
- Footnotes/endnotes render the same way in EPUB as in the other four formats: a "Notes"
  section at the end of the chapter's XHTML, **not** EPUB3 popup footnotes. This is a
  deliberate consistency decision from the design spec — do not "improve" it into real
  popups.
- Cover images: only `.jpg`/`.jpeg` and `.png` are accepted. Any other extension, or a
  missing/unreadable file, falls back **silently** to the generated text cover, with a
  status warning appended — this must never make the whole export fail.
- Nothing coded in this plan touches `manifest.json`'s schema, `outline.md`'s grammar, or the
  Markdown flavor (goldmark + GFM + Footnote extensions) — none of the CLAUDE.md shared-contract
  hard gates are in scope here. If a task's implementer thinks a change would touch one of
  those, STOP and ask rather than proceeding.
- Every task ends with clean `go build ./...`, `go vet ./...`, and a fully green
  `go test ./... -count=1` (whole module, not a filtered subset). A known flaky test,
  `TestSnippetCacheReadsHeadAndInvalidates` (filesystem mtime granularity, unrelated to this
  work), may fail intermittently — rerun once; if it clears, it's not a regression from this
  plan.
- All new user-facing strings are in French, matching the rest of the already-translated UI
  (see `docs/superpowers/plans/2026-08-02-ui-fr-labels.md` for the established vocabulary:
  "échec de l'enregistrement", "annuler", "aucun", etc.). No English string should leak into
  a status message, panel title, or footer this plan adds.
- Commit after every task (minimum), following the existing repo's French commit-message
  style (see `git log` for examples) with a `Co-Authored-By: Claude Sonnet 5
  <noreply@anthropic.com>` trailer.

---

## File Structure

| File | Task | Responsibility |
|---|---|---|
| `export_epub.go` (new) | 1 | `writeEPUB()` — the EPUB writer: ZIP container, OPF manifest, NCX+nav TOC, per-chapter XHTML, cover (text or image). |
| `export_epub_test.go` (new) | 1 | Unit tests for `writeEPUB()`: valid ZIP, correct mimetype/OPF/TOC, cover fallback. |
| `settings.go` (modify) | 2 | Add `Cover *string` to `projectSettings`, `Cover string` to `effectiveSettings`, wire through `mergeSettings`. |
| `settings_test.go` (modify) | 2 | Cover through `mergeSettings`/`resolveSettings` precedence. |
| `properties.go` (modify) | 3 | Add `propCover` field kind, `cover textinput.Model`, wire into `dirty()`/`save()`/`focusInput()`/`blurInputs()`/`propertiesView()`. |
| `properties_test.go` (modify) | 3 | Cover field renders, saves, round-trips. |
| `export.go` (rewrite) | 4 | `exportChooserModel` (checkboxes + style radio), replaces the `exportPrompt bool` flow; `runExport()` takes the chosen format set instead of writing all four unconditionally. |
| `main.go` (modify) | 4 | Replace `exportPrompt bool` field and its two `case "ctrl+e":`/prompt-handling blocks with the new chooser model; update `composeStatus`. |
| `corkboard.go` (modify) | 4 | Same replacement for the corkboard's own `ctrl+e` entry point (currently duplicates the same `m`/`t` logic). |
| `export_wiring_test.go` (rewrite) | 5 | Update every existing export integration test to drive the new checkbox+style screen instead of a bare `'m'`/`'t'` keypress; add EPUB-specific integration tests. |

---

## Task 1: `writeEPUB()` — the EPUB writer

**Files:**
- Create: `export_epub.go`
- Create: `export_epub_test.go`

**Interfaces:**
- Consumes: `ManuscriptDoc`, `Section`, `Block`, `Paragraph`, `Heading`, `Blockquote`, `List`,
  `SceneBreak`, `Endnote`, `Endnotes`, `Run`, `ExportStyle` (`StyleManuscript`/`StyleTufte`),
  `Meta{Author, Title, Contact, TitlePage}` — all from `export_ast.go`, unchanged.
  `manuscriptWordCount(doc ManuscriptDoc) int` and `contactLines(contact string) []string`
  from `export_titlepage.go`, unchanged.
- Produces: `func writeEPUB(doc ManuscriptDoc, st ExportStyle, meta Meta, coverPath string) ([]byte, error)`
  — the fourth parameter, `coverPath`, is new relative to the other four writers (they only
  take `doc, st, meta`); it is the resolved absolute or project-relative path from
  Properties' new Cover field, or `""` if unset. Task 4 is the only later task that calls
  this function from `runExport()`.

- [x] **Step 1: Write the failing test for a minimal valid EPUB**

```go
package main

import (
	"archive/zip"
	"bytes"
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
```

- [x] **Step 2: Run tests to verify they fail**

Run: `export PATH="$HOME/.local/go/bin:$PATH" && go test -run TestWriteEPUB ./... -v 2>&1 | head -30`
Expected: FAIL — `writeEPUB` undefined (the function doesn't exist yet).

- [x] **Step 3: Write the AST-to-XHTML block renderer**

This mirrors `writeBlockDOCX` in `export_docx.go` — same switch over `Block` types, same
degrade decisions (list → `•`/numbered paragraphs, blockquote → indented, scene break →
centered `#`, endnotes → "Notes" heading + numbered paragraphs), but emitting XHTML instead
of OOXML.

```go
package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// xhtmlEsc escapes text for XHTML content (same five entities as HTML).
func xhtmlEsc(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&#39;")
	return r.Replace(s)
}

// epubRun renders a Run as an XHTML inline span (bold/italic nest as <strong>/<em>).
func epubRun(r Run) string {
	s := xhtmlEsc(r.Text)
	if r.Italic {
		s = "<em>" + s + "</em>"
	}
	if r.Bold {
		s = "<strong>" + s + "</strong>"
	}
	return s
}

func epubRuns(rs []Run) string {
	var b strings.Builder
	for _, r := range rs {
		b.WriteString(epubRun(r))
	}
	return b.String()
}

// writeBlockEPUB mirrors writeBlockDOCX's degrade decisions, emitting XHTML.
func writeBlockEPUB(b *strings.Builder, blk Block) {
	switch v := blk.(type) {
	case Paragraph:
		b.WriteString("<p>" + epubRuns(v.Runs) + "</p>\n")
	case Heading:
		level := v.Level
		if level < 1 {
			level = 1
		}
		if level > 6 {
			level = 6
		}
		fmt.Fprintf(b, "<h%d>%s</h%d>\n", level+1, epubRuns(v.Runs), level+1) // +1: chapter title owns <h1>
	case SceneBreak:
		b.WriteString(`<p class="scenebreak">#</p>` + "\n")
	case Blockquote:
		b.WriteString(`<blockquote>` + "\n")
		for _, c := range v.Children {
			writeBlockEPUB(b, c)
		}
		b.WriteString("</blockquote>\n")
	case List:
		tag := "ul"
		if v.Ordered {
			tag = "ol"
		}
		fmt.Fprintf(b, "<%s>\n", tag)
		for _, it := range v.Items {
			b.WriteString("<li>" + epubRuns(it.Runs) + "</li>\n")
		}
		fmt.Fprintf(b, "</%s>\n", tag)
	case Endnotes:
		if len(v.Items) == 0 {
			return
		}
		b.WriteString("<h2>Notes</h2>\n<ol>\n")
		for _, e := range v.Items {
			fmt.Fprintf(b, "<li>%s</li>\n", epubRuns(e.Runs))
		}
		b.WriteString("</ol>\n")
	}
}
```

- [x] **Step 4: Write the container/OPF/NCX/nav/cover scaffolding and `writeEPUB` itself**

```go
const epubMimetype = "application/epub+zip"

const epubContainerXML = `<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`

const epubCSS = `body { font-family: serif; line-height: 1.5; margin: 1em; }
h1 { text-align: center; }
.scenebreak { text-align: center; margin: 2em 0; }
.cover { text-align: center; margin-top: 30%; }
.cover h1 { font-size: 1.5em; }`

// epubCoverImage reads coverPath (only .jpg/.jpeg/.png accepted) and returns its bytes, the
// EPUB-internal filename ("cover.jpg" or "cover.png"), and the media type — or ok=false if
// the path is empty, unreadable, or not a supported image extension, in which case the
// caller falls back to a generated text cover.
func epubCoverImage(coverPath string) (data []byte, name, mediaType string, ok bool) {
	if coverPath == "" {
		return nil, "", "", false
	}
	ext := strings.ToLower(filepath.Ext(coverPath))
	switch ext {
	case ".jpg", ".jpeg":
		mediaType = "image/jpeg"
		name = "cover.jpg"
	case ".png":
		mediaType = "image/png"
		name = "cover.png"
	default:
		return nil, "", "", false
	}
	data, err := os.ReadFile(coverPath)
	if err != nil {
		return nil, "", "", false
	}
	return data, name, mediaType, true
}

// writeEPUB builds a minimal EPUB3 (container + OPF + NCX + nav + one XHTML file per
// chapter). coverPath is the resolved path from Properties' Cover field ("" if unset) —
// see epubCoverImage for the accepted formats and fallback behavior.
func writeEPUB(doc ManuscriptDoc, st ExportStyle, meta Meta, coverPath string) ([]byte, error) {
	coverData, coverName, coverMediaType, hasCoverImage := epubCoverImage(coverPath)

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	// mimetype MUST be the first entry and MUST be stored (uncompressed).
	mw, err := zw.CreateHeader(&zip.FileHeader{Name: "mimetype", Method: zip.Store})
	if err != nil {
		return nil, err
	}
	if _, err := mw.Write([]byte(epubMimetype)); err != nil {
		return nil, err
	}

	add := func(name, content string) error {
		w, err := zw.Create(name)
		if err != nil {
			return err
		}
		_, err = w.Write([]byte(content))
		return err
	}
	if err := add("META-INF/container.xml", epubContainerXML); err != nil {
		return nil, err
	}
	if err := add("OEBPS/style.css", epubCSS); err != nil {
		return nil, err
	}

	// Cover page + optional image.
	coverXHTML := epubCoverXHTML(meta, hasCoverImage, coverName)
	if err := add("OEBPS/cover.xhtml", coverXHTML); err != nil {
		return nil, err
	}
	if hasCoverImage {
		w, err := zw.Create("OEBPS/" + coverName)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(coverData); err != nil {
			return nil, err
		}
	}

	// One XHTML file per chapter.
	chapterFiles := make([]string, len(doc))
	for i, sec := range doc {
		name := fmt.Sprintf("chapter-%02d.xhtml", i+1)
		chapterFiles[i] = name
		var body strings.Builder
		body.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
		body.WriteString(`<html xmlns="http://www.w3.org/1999/xhtml"><head><title>` +
			xhtmlEsc(sec.Title) + `</title><link rel="stylesheet" type="text/css" href="style.css"/></head><body>`)
		body.WriteString("<h1>" + xhtmlEsc(sec.Title) + "</h1>\n")
		for _, blk := range sec.Blocks {
			writeBlockEPUB(&body, blk)
		}
		body.WriteString("</body></html>")
		if err := add("OEBPS/"+name, body.String()); err != nil {
			return nil, err
		}
	}

	if err := add("OEBPS/toc.ncx", epubTocNCX(doc, meta)); err != nil {
		return nil, err
	}
	if err := add("OEBPS/nav.xhtml", epubNavXHTML(doc)); err != nil {
		return nil, err
	}
	if err := add("OEBPS/content.opf", epubContentOPF(doc, meta, chapterFiles, hasCoverImage, coverName, coverMediaType)); err != nil {
		return nil, err
	}

	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// epubCoverXHTML is the cover page: the supplied image if hasCoverImage, else a generated
// text cover (title + author centered), per the design's fallback rule.
func epubCoverXHTML(meta Meta, hasCoverImage bool, coverName string) string {
	var body strings.Builder
	body.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	body.WriteString(`<html xmlns="http://www.w3.org/1999/xhtml"><head><title>Couverture</title>` +
		`<link rel="stylesheet" type="text/css" href="style.css"/></head><body>`)
	if hasCoverImage {
		fmt.Fprintf(&body, `<div class="cover"><img src="%s" alt="Couverture" style="max-width:100%%;"/></div>`, coverName)
	} else {
		body.WriteString(`<div class="cover"><h1>` + xhtmlEsc(meta.Title) + `</h1>`)
		if meta.Author != "" {
			body.WriteString(`<p>` + xhtmlEsc(meta.Author) + `</p>`)
		}
		body.WriteString(`</div>`)
	}
	body.WriteString("</body></html>")
	return body.String()
}

// epubTocNCX builds the EPUB2-compatible navigation control file, one navPoint per section.
func epubTocNCX(doc ManuscriptDoc, meta Meta) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<ncx xmlns="http://www.daisy.org/z3986/2005/ncx/" version="2005-1"><head></head><docTitle><text>`)
	b.WriteString(xhtmlEsc(meta.Title))
	b.WriteString(`</text></docTitle><navMap>`)
	for i, sec := range doc {
		fmt.Fprintf(&b, `<navPoint id="np%d" playOrder="%d"><navLabel><text>%s</text></navLabel><content src="chapter-%02d.xhtml"/></navPoint>`,
			i+1, i+1, xhtmlEsc(sec.Title), i+1)
	}
	b.WriteString(`</navMap></ncx>`)
	return b.String()
}

// epubNavXHTML builds the EPUB3 nav document (same TOC, XHTML5 <nav> form).
func epubNavXHTML(doc ManuscriptDoc) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops">` +
		`<head><title>Table des matières</title></head><body><nav epub:type="toc"><ol>`)
	for i, sec := range doc {
		fmt.Fprintf(&b, `<li><a href="chapter-%02d.xhtml">%s</a></li>`, i+1, xhtmlEsc(sec.Title))
	}
	b.WriteString(`</ol></nav></body></html>`)
	return b.String()
}

// epubContentOPF builds the package manifest: metadata, manifest items, spine (reading order).
func epubContentOPF(doc ManuscriptDoc, meta Meta, chapterFiles []string, hasCoverImage bool, coverName, coverMediaType string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="bookid">`)
	b.WriteString(`<metadata xmlns:dc="http://purl.org/dc/elements/1.1/">`)
	fmt.Fprintf(&b, `<dc:identifier id="bookid">urn:uuid:%s</dc:identifier>`, slugify(meta.Title))
	fmt.Fprintf(&b, `<dc:title>%s</dc:title>`, xhtmlEsc(meta.Title))
	if meta.Author != "" {
		fmt.Fprintf(&b, `<dc:creator>%s</dc:creator>`, xhtmlEsc(meta.Author))
	}
	b.WriteString(`<dc:language>fr</dc:language>`)
	if hasCoverImage {
		b.WriteString(`<meta name="cover" content="cover-image"/>`)
	}
	b.WriteString(`</metadata><manifest>`)
	b.WriteString(`<item id="ncx" href="toc.ncx" media-type="application/x-dtbncx+xml"/>`)
	b.WriteString(`<item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>`)
	b.WriteString(`<item id="css" href="style.css" media-type="text/css"/>`)
	b.WriteString(`<item id="cover-page" href="cover.xhtml" media-type="application/xhtml+xml"/>`)
	if hasCoverImage {
		fmt.Fprintf(&b, `<item id="cover-image" href="%s" media-type="%s"/>`, coverName, coverMediaType)
	}
	for i, name := range chapterFiles {
		fmt.Fprintf(&b, `<item id="chap%d" href="%s" media-type="application/xhtml+xml"/>`, i+1, name)
	}
	b.WriteString(`</manifest><spine toc="ncx"><itemref idref="cover-page"/>`)
	for i := range chapterFiles {
		fmt.Fprintf(&b, `<itemref idref="chap%d"/>`, i+1)
	}
	b.WriteString(`</spine></package>`)
	return b.String()
}
```

- [x] **Step 5: Run tests to verify they pass**

Run: `export PATH="$HOME/.local/go/bin:$PATH" && go test -run TestWriteEPUB ./... -v`
Expected: all three PASS.

- [x] **Step 6: Add the TOC and cover-fallback tests**

```go
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
```

- [x] **Step 7: Run all EPUB tests**

Run: `export PATH="$HOME/.local/go/bin:$PATH" && go test -run TestWriteEPUB ./... -v`
Expected: all PASS.

- [x] **Step 8: Build, vet, full test suite**

Run: `go build ./... && go vet ./... && go test ./... -count=1`
Expected: clean build/vet, 0 FAIL (rerun once if only `TestSnippetCacheReadsHeadAndInvalidates` fails — see Global Constraints).

- [x] **Step 9: Commit**

```bash
git add export_epub.go export_epub_test.go
git commit -m "$(cat <<'EOF'
Ajoute le writer d'export EPUB

Nouveau format d'export EPUB3 minimal : container ZIP, manifeste OPF,
table des matières (NCX + nav XHTML), un fichier XHTML par chapitre.
Réutilise l'AST partagé (export_ast.go) sans aucun changement au
parsing Markdown. Notes de bas de page en fin de chapitre, cohérent
avec RTF/PDF/DOCX/ODT (pas de popup EPUB3). Couverture : texte généré
par défaut (titre + auteur), image JPEG/PNG optionnelle avec repli
silencieux si le chemin est invalide.

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: `Cover` setting — storage plumbing

**Files:**
- Modify: `settings.go`
- Modify: `settings_test.go`

**Interfaces:**
- Consumes: `projectSettings` struct (`settings.go:21-24`), `effectiveSettings` struct
  (`settings.go:27-31`), `mergeSettings(uc userConfig, ps projectSettings) effectiveSettings`
  (`settings.go:111-137`).
- Produces: `projectSettings.Cover *string` (new field, same nil-vs-empty pattern as
  `Width`/`Smartquotes`), `effectiveSettings.Cover string` (new field). Task 3 reads
  `resolveSettings(dir).Cover` to seed the Properties form; Task 1's `writeEPUB` (already
  written) is called by Task 4 with whatever cover path Task 3/4 resolve — this task only
  adds the storage, no caller changes yet.

- [x] **Step 1: Write the failing test for Cover in `mergeSettings`**

Add to `settings_test.go` (find the existing `TestMergeSettings*` tests and follow their
pattern — read the file first to match the exact table-driven style already there):

```go
func TestMergeSettingsCover(t *testing.T) {
	uc := userConfig{}
	cover := "cover.jpg"
	ps := projectSettings{Cover: &cover}
	eff := mergeSettings(uc, ps)
	if eff.Cover != "cover.jpg" {
		t.Errorf("Cover = %q, want %q", eff.Cover, "cover.jpg")
	}
}

func TestMergeSettingsCoverUnsetIsEmpty(t *testing.T) {
	eff := mergeSettings(userConfig{}, projectSettings{})
	if eff.Cover != "" {
		t.Errorf("Cover with no ps.Cover set = %q, want empty", eff.Cover)
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `export PATH="$HOME/.local/go/bin:$PATH" && go test -run TestMergeSettingsCover ./... -v`
Expected: FAIL — `projectSettings` has no field `Cover` / `effectiveSettings` has no field `Cover`.

- [x] **Step 3: Add the field and wire it through**

In `settings.go`, modify the `projectSettings` struct:

```go
type projectSettings struct {
	Width       *int    `json:"width,omitempty"`
	Smartquotes *bool   `json:"smartquotes,omitempty"`
	Cover       *string `json:"cover,omitempty"`
}
```

Modify `effectiveSettings`:

```go
type effectiveSettings struct {
	Author, Contact string
	Width           int
	Smartquotes     bool
	Cover           string
}
```

Modify `mergeSettings` — add after the `Smartquotes` block:

```go
	if ps.Cover != nil {
		eff.Cover = *ps.Cover
	}
```

(No env-var default for Cover — unlike Width/Smartquotes, there is no `OKASHI_COVER`
equivalent; the design spec puts this in Properties only, never in env. Leave `eff.Cover`
at its zero value `""` unless `ps.Cover` is set.)

- [x] **Step 4: Run test to verify it passes**

Run: `export PATH="$HOME/.local/go/bin:$PATH" && go test -run TestMergeSettingsCover ./... -v`
Expected: both PASS.

- [x] **Step 5: Build, vet, full test suite**

Run: `go build ./... && go vet ./... && go test ./... -count=1`
Expected: clean, 0 FAIL.

- [x] **Step 6: Commit**

```bash
git add settings.go settings_test.go
git commit -m "$(cat <<'EOF'
Ajoute le réglage Cover (chemin de couverture EPUB) au stockage par projet

projectSettings.Cover / effectiveSettings.Cover, même pattern
pointeur-nilable que Width/Smartquotes, stocké dans .okashi.json.
Pas d'équivalent OKASHI_* : ce réglage n'existe que via Properties.

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Properties — the Couverture field

**Files:**
- Modify: `properties.go`
- Modify: `properties_test.go`

**Interfaces:**
- Consumes: `effectiveSettings.Cover` (Task 2), `projectSettings.Cover *string` (Task 2),
  `loadProjectSettings(dir string) projectSettings`, `saveProjectSettings(dir string, s
  projectSettings) error` (`settings.go`, unchanged), `newPropInput(val string, width int)
  textinput.Model` (`properties.go:52-59`, unchanged), `propRow(label, val string, focused
  bool) string` (`properties.go:308-321`, unchanged).
- Produces: `propCover` added to the `propKind` enum; `propertiesModel.cover
  textinput.Model` field. Task 4 reads `m.properties.cover.Value()` (via
  `resolveSettings(dir).Cover`, already persisted by this task's `save()`) when building the
  export chooser's cover path — Task 4 does not touch `properties.go` itself, it only calls
  `resolveSettings(m.files.dir).Cover`.

- [x] **Step 1: Write the failing test for the Couverture field**

Read `properties_test.go` first to match its existing setup helpers (likely a `newTestProps`
or direct `newPropertiesModel(dir)` calls — follow whatever pattern is already there). Then
add:

```go
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
```

(Add `"os"` to the test file's imports if not already present.)

- [x] **Step 2: Run test to verify it fails**

Run: `export PATH="$HOME/.local/go/bin:$PATH" && go test -run TestPropertiesCover ./... -v`
Expected: FAIL — `propCover` undefined / `p.cover` undefined.

- [x] **Step 3: Add `propCover` to the enum**

In `properties.go`, modify the `propKind` const block:

```go
const (
	propTitle propKind = iota
	propAuthor
	propContact
	propWidth
	propSmartquotes
	propCover
)
```

- [x] **Step 4: Add the `cover` field to `propertiesModel` and its `origCover` baseline**

```go
type propertiesModel struct {
	dir          string
	isManuscript bool

	title       textinput.Model
	author      textinput.Model
	width       textinput.Model
	contact     textarea.Model
	cover       textinput.Model
	smartquotes bool

	fields      []propKind
	focus       int
	editing     bool
	confirmExit bool

	origTitle       string
	origAuthor      string
	origContact     string
	origWidth       int
	origSmartquotes bool
	origCover       string
}
```

- [x] **Step 5: Wire `cover` into `newPropertiesModel`, `fields`, `dirty`, `focusInput`, `blurInputs`, `save`**

In `newPropertiesModel` (`properties.go:69-103`), add the cover input and baseline:

```go
	p := propertiesModel{
		dir:             dir,
		isManuscript:    isMs,
		title:           newPropInput(title, 40),
		author:          newPropInput(eff.Author, 40),
		width:           newPropInput(strconv.Itoa(eff.Width), 6),
		contact:         ca,
		cover:           newPropInput(eff.Cover, 40),
		smartquotes:     eff.Smartquotes,
		origTitle:       title,
		origAuthor:      eff.Author,
		origContact:     eff.Contact,
		origWidth:       eff.Width,
		origSmartquotes: eff.Smartquotes,
		origCover:       eff.Cover,
	}
	if isMs {
		p.fields = []propKind{propTitle, propAuthor, propContact, propWidth, propSmartquotes, propCover}
	} else {
		p.fields = []propKind{propAuthor, propContact, propWidth, propSmartquotes, propCover}
	}
```

In `dirty()` (`properties.go:106-120`), add:

```go
	if p.cover.Value() != p.origCover {
		return true
	}
```

In `focusInput()` (`properties.go:122-133`), add a case:

```go
	case propCover:
		p.cover.Focus()
```

In `blurInputs()` (`properties.go:135-140`), add:

```go
	p.cover.Blur()
```

In `save()` (`properties.go:144-192`), add a block alongside the width/smartquotes
per-project write (Cover lives in the same `.okashi.json` file — fold it into the existing
`if widthChanged || sqChanged` write rather than a separate file write):

```go
	coverChanged := p.cover.Value() != p.origCover
	if widthChanged || sqChanged || coverChanged {
		ps := loadProjectSettings(p.dir)
		if widthChanged {
			wv := clampWidth(w)
			ps.Width = &wv
		}
		if sqChanged {
			sv := p.smartquotes
			ps.Smartquotes = &sv
		}
		if coverChanged {
			cv := p.cover.Value()
			ps.Cover = &cv
		}
		if serr := saveProjectSettings(p.dir, ps); serr != nil {
			return false, serr
		}
		projectChanged = true
	}
```

This *replaces* the existing `if widthChanged || sqChanged {` block — same body, condition
extended with `|| coverChanged`, and one extra `if coverChanged` inside. At the end of
`save()`, add to the baseline reset:

```go
	p.origCover = p.cover.Value()
```

- [x] **Step 6: Wire `cover` into `updatePropertiesEditing`**

In `properties.go:272-304`, add a case to the `switch kind` block:

```go
	case propCover:
		p.cover, cmd = p.cover.Update(key)
```

- [x] **Step 7: Wire `cover` into `propertiesView`**

In `properties.go:330-381`, add a case to the `switch kind` block (around line 342-364):

```go
		case propCover:
			label = "Couverture"
			if editing {
				val = p.cover.View()
			} else if v := p.cover.Value(); v != "" {
				val = v
			} else {
				val = lipgloss.NewStyle().Foreground(subtle).Render("(aucune)")
			}
```

- [x] **Step 8: Run tests to verify they pass**

Run: `export PATH="$HOME/.local/go/bin:$PATH" && go test -run TestPropertiesCover ./... -v`
Expected: both PASS.

- [x] **Step 9: Run the full Properties test file to check nothing else broke**

Run: `export PATH="$HOME/.local/go/bin:$PATH" && go test -run TestProperties ./... -v`
Expected: all PASS — in particular, `dirty()`/`save()` tests for the *other* fields must
still pass unmodified, since Step 5's edit to `save()` only adds a condition, it doesn't
remove existing behavior.

- [x] **Step 10: Build, vet, full test suite**

Run: `go build ./... && go vet ./... && go test ./... -count=1`
Expected: clean, 0 FAIL.

- [x] **Step 11: Commit**

```bash
git add properties.go properties_test.go
git commit -m "$(cat <<'EOF'
Ajoute le champ Couverture à l'écran Properties

Nouveau champ texte "Couverture" (chemin vers une image de
couverture EPUB, JPEG/PNG), stocké dans .okashi.json aux côtés de
Width/Smartquotes — réglage par projet. Champ libre, édité comme
Titre/Auteur/Contact/Largeur existants.

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Export chooser screen — checkboxes + style radio

**Files:**
- Rewrite: `export.go`
- Modify: `main.go`
- Modify: `corkboard.go`

**Interfaces:**
- Consumes: `writeRTF`, `writePDF`, `writeDOCX`, `writeODT` (unchanged signatures,
  `(doc ManuscriptDoc, st ExportStyle, meta Meta) ([]byte, error)` or `[]byte` for RTF),
  `writeEPUB(doc ManuscriptDoc, st ExportStyle, meta Meta, coverPath string) ([]byte, error)`
  (Task 1), `resolveSettings(dir string) effectiveSettings` (Task 2, `.Cover` field),
  `atomicWrite`, `framedPanel(title, inner string, width, height int, action string) string`
  (`inspector.go:31`, unchanged), `selectedStyle` (existing lipgloss style var used
  throughout the codebase for highlighted rows — confirm its exact name with
  `grep -n "selectedStyle" main.go` before use, it is already imported/available package-wide).
- Produces: `type exportFormat int` with named constants `formatRTF, formatPDF, formatDOCX,
  formatODT, formatEPUB`; `type exportChooserModel struct{ checked map[exportFormat]bool;
  style ExportStyle; cursor int }`; `func newExportChooser() exportChooserModel`; `func
  (c *exportChooserModel) toggleAtCursor()`; `func (c exportChooserModel) anyChecked() bool`;
  `func (m *model) runExport()` — **signature change**: no longer takes an `ExportStyle`
  parameter, reads `m.exportChooser.style` and `m.exportChooser.checked` instead. `main.go`'s
  `model` struct field `exportPrompt bool` is replaced by `exportChooser
  *exportChooserModel` (nil = not showing; a non-nil pointer both signals "the screen is up"
  and carries its own state — replaces the separate bool).

- [x] **Step 1: Write the failing unit tests for the chooser model**

```go
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
```

Save as `export_chooser_test.go`.

- [x] **Step 2: Run tests to verify they fail**

Run: `export PATH="$HOME/.local/go/bin:$PATH" && go test -run TestExportChooser ./... -v && go test -run TestNewExportChooser ./... -v`
Expected: FAIL — none of these types/functions exist yet.

- [x] **Step 3: Read the current `export.go`, `exportPrompt` usages, and `selectedStyle`**

Before rewriting, run these to confirm every call site that must change:

```bash
grep -n "exportPrompt" main.go corkboard.go
grep -n "^var selectedStyle\|selectedStyle =" *.go
grep -n "runExport(" *.go
```

Expected output: `exportPrompt` appears in `main.go` (struct field, `case "ctrl+e":` in the
writing-screen key handler, the `if m.exportPrompt {` prompt-handling block, and in
`composeStatus`) and in `corkboard.go` (`case "ctrl+e":`, the `if m.exportPrompt {`
prompt-handling block). `runExport(` appears once as the definition in `export.go` and at
each `m.runExport(StyleManuscript)` / `m.runExport(StyleTufte)` call site in both files —
these all need updating in this task.

- [x] **Step 4: Rewrite `export.go`**

```go
package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// exportFormat is one of the five file types runExport can produce.
type exportFormat int

const (
	formatRTF exportFormat = iota
	formatPDF
	formatDOCX
	formatODT
	formatEPUB
)

// exportFormatOrder is the fixed display/iteration order for the chooser and for runExport's
// write loop — RTF, PDF, DOCX, ODT, EPUB, matching the design spec's screen mockup.
var exportFormatOrder = []exportFormat{formatRTF, formatPDF, formatDOCX, formatODT, formatEPUB}

func (f exportFormat) label() string {
	switch f {
	case formatRTF:
		return "RTF"
	case formatPDF:
		return "PDF"
	case formatDOCX:
		return "DOCX"
	case formatODT:
		return "ODT"
	case formatEPUB:
		return "EPUB"
	}
	return ""
}

func (f exportFormat) ext() string {
	switch f {
	case formatRTF:
		return ".rtf"
	case formatPDF:
		return ".pdf"
	case formatDOCX:
		return ".docx"
	case formatODT:
		return ".odt"
	case formatEPUB:
		return ".epub"
	}
	return ""
}

// exportChooserRowCount is the number of navigable rows: 5 format checkboxes + 1 style radio.
const exportChooserRowCount = len(exportFormatOrder) + 1

// exportChooserModel backs the ctrl+e screen: which formats to write, and which style.
// cursor indexes into exportFormatOrder for rows 0..4, and the style row is row 5 (the last).
type exportChooserModel struct {
	checked map[exportFormat]bool
	style   ExportStyle
	cursor  int
}

func newExportChooser() exportChooserModel {
	return exportChooserModel{checked: make(map[exportFormat]bool), style: StyleManuscript}
}

func (c *exportChooserModel) toggleAtCursor() {
	if c.cursor < len(exportFormatOrder) {
		f := exportFormatOrder[c.cursor]
		c.checked[f] = !c.checked[f]
		return
	}
	// Style row: space toggles between the two styles.
	if c.style == StyleManuscript {
		c.style = StyleTufte
	} else {
		c.style = StyleManuscript
	}
}

// setStyle is used by the ←/→ shortcut on the style row (direct set, not a toggle).
func (c *exportChooserModel) setStyle(st ExportStyle) {
	c.style = st
}

func (c exportChooserModel) anyChecked() bool {
	for _, f := range exportFormatOrder {
		if c.checked[f] {
			return true
		}
	}
	return false
}

// exportWholeManuscript reports whether ctrl+e should export the whole manuscript (from the
// full-screen corkboard) rather than just the current document.
func (m model) exportWholeManuscript() bool {
	return m.screen == screenCorkboard
}

// runExport builds the export doc for the current scope (whole manuscript from the corkboard,
// else the current document) and writes one file per format checked in m.exportChooser,
// under <dir>/export/. Style and format selection come from m.exportChooser — call this only
// after confirming m.exportChooser.anyChecked().
func (m *model) runExport() {
	defer func() {
		if r := recover(); r != nil {
			m.status = fmt.Sprintf("export failed: %v", r)
		}
	}()
	chooser := m.exportChooser
	dir := m.files.dir
	var doc ManuscriptDoc
	var title string
	if m.exportWholeManuscript() {
		entries := readEntries(dir)
		v := resolveManuscript(dir, entries)
		doc = manuscriptDocFromChapters(dir, v.chapters)
		title = v.title
	} else {
		if m.currentFile == "" {
			m.status = "nothing to export"
			return
		}
		dir = filepath.Dir(m.currentFile)
		base := filepath.Base(m.currentFile)
		data, err := os.ReadFile(m.currentFile)
		if err != nil {
			m.status = "export failed: " + err.Error()
			return
		}
		title = sectionTitle(base)
		doc = ManuscriptDoc{{Title: title, Blocks: parseSection(data)}}
	}
	if len(doc) == 0 {
		m.status = "nothing to export"
		return
	}

	eff := resolveSettings(dir)
	meta := Meta{
		Author:    eff.Author,
		Title:     title,
		Contact:   eff.Contact,
		TitlePage: m.exportWholeManuscript(),
	}
	outDir := filepath.Join(dir, "export")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		m.status = "export failed: " + err.Error()
		return
	}
	slug := slugify(title)

	coverPath := eff.Cover
	if coverPath != "" && !filepath.IsAbs(coverPath) {
		coverPath = filepath.Join(dir, coverPath)
	}
	coverWarning := false
	if coverPath != "" {
		if _, err := os.Stat(coverPath); err != nil {
			coverWarning = true
		}
	}

	var written []string
	for _, f := range exportFormatOrder {
		if !chooser.checked[f] {
			continue
		}
		path := filepath.Join(outDir, slug+f.ext())
		var data []byte
		var err error
		switch f {
		case formatRTF:
			data = writeRTF(doc, chooser.style, meta)
		case formatPDF:
			data, err = writePDF(doc, chooser.style, meta)
		case formatDOCX:
			data, err = writeDOCX(doc, chooser.style, meta)
		case formatODT:
			data, err = writeODT(doc, chooser.style, meta)
		case formatEPUB:
			data, err = writeEPUB(doc, chooser.style, meta, coverPath)
		}
		if err != nil {
			m.status = fmt.Sprintf("export failed (%s) : %s", f.label(), err.Error())
			return
		}
		if err := atomicWrite(path, data, 0o644); err != nil {
			m.status = "export failed : " + err.Error()
			return
		}
		written = append(written, f.ext())
	}

	msg := "exporté " + slug
	for _, ext := range written {
		msg += " + " + ext
	}
	msg += " vers export/"
	if coverWarning {
		msg += " — couverture introuvable, page de titre utilisée"
	}
	m.status = msg
}
```

Note: `"nothing to export"` and the RTF/PDF/DOCX/ODT error-message English fragments
(`"export failed: "`) were already present verbatim in the pre-existing `export.go` before
this task — this rewrite preserves them unchanged (they are not part of this plan's French
translation scope; the file was already like this before Task 4 started). Do not "fix" them
as a drive-by — that would be outside this plan's stated scope. `fmt.Sprintf("export failed (%s) : %s", ...)`
follows the same pre-existing `"export failed (pdf): "` pattern, just parameterized by
`f.label()` instead of one hardcoded format name per format.

- [x] **Step 5: Update `main.go` — struct field, key handler, prompt-handling block, `composeStatus`**

Find the `exportPrompt bool` field (`main.go:304`) and replace it:

```go
	exportChooser *exportChooserModel // nil when the screen is not showing
```

Find `case "ctrl+e":` in the writing-screen key handler (`main.go:1418-1421`) and replace:

```go
		case "ctrl+e":
			c := newExportChooser()
			m.exportChooser = &c
			return m, nil
```

Find the `if m.exportPrompt {` block (`main.go:1030-1046` — read the surrounding ~20 lines
first to see the exact current `case` arms before editing) and replace the whole block with:

```go
	if m.exportChooser != nil {
		if key, ok := msg.(tea.KeyMsg); ok {
			switch key.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "up", "k":
				m.exportChooser.cursor = (m.exportChooser.cursor - 1 + exportChooserRowCount) % exportChooserRowCount
			case "down", "j":
				m.exportChooser.cursor = (m.exportChooser.cursor + 1) % exportChooserRowCount
			case "left", "right":
				if m.exportChooser.cursor == exportChooserRowCount-1 {
					if m.exportChooser.style == StyleManuscript {
						m.exportChooser.setStyle(StyleTufte)
					} else {
						m.exportChooser.setStyle(StyleManuscript)
					}
				}
			case " ":
				m.exportChooser.toggleAtCursor()
			case "enter":
				if !m.exportChooser.anyChecked() {
					m.status = "choisissez au moins un format"
					return m, nil
				}
				m.runExport()
				m.exportChooser = nil
			case "esc":
				m.exportChooser = nil
				m.status = "export annulé"
			}
		}
		return m, nil
	}
```

Find `composeStatus`/`m.exportPrompt` in the status-bar rendering (`main.go:2570-2572`) and
replace:

```go
	if m.exportChooser != nil {
		return "export : espace cocher · ↑↓ naviguer · ←→ style · ↵ exporter · esc annuler"
	}
```

Grep once more for any remaining `m.exportPrompt` reference in `main.go` after these edits
(`grep -n "exportPrompt" main.go`) — there must be none left; if any remain, they were missed
above and need the same treatment.

- [x] **Step 6: Update `corkboard.go` — key handler and prompt-handling block**

Find `case "ctrl+e":` in `corkboard.go` (`corkboard.go:272-274`) and replace:

```go
		case "ctrl+e":
			c := newExportChooser()
			m.exportChooser = &c
```

Find the `if m.exportPrompt {` block in `corkboard.go` (`corkboard.go:148-163`) and replace
with the same block used in `main.go` Step 5 (identical logic — both entry points share the
one `m.exportChooser` field and the one `runExport()`):

```go
	if m.exportChooser != nil {
		switch key.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			m.exportChooser.cursor = (m.exportChooser.cursor - 1 + exportChooserRowCount) % exportChooserRowCount
		case "down", "j":
			m.exportChooser.cursor = (m.exportChooser.cursor + 1) % exportChooserRowCount
		case "left", "right":
			if m.exportChooser.cursor == exportChooserRowCount-1 {
				if m.exportChooser.style == StyleManuscript {
					m.exportChooser.setStyle(StyleTufte)
				} else {
					m.exportChooser.setStyle(StyleManuscript)
				}
			}
		case " ":
			m.exportChooser.toggleAtCursor()
		case "enter":
			if !m.exportChooser.anyChecked() {
				m.status = "choisissez au moins un format"
				return m, nil
			}
			m.runExport()
			m.exportChooser = nil
		case "esc":
			m.exportChooser = nil
			m.status = "export annulé"
		}
		return m, nil
	}
```

(Note: this block is inside a function that already extracted `key, ok := msg.(tea.KeyMsg)`
earlier — `corkboard.go`'s existing structure at that point in the function already has `key`
available without the `if key, ok := msg.(tea.KeyMsg); ok` wrapper that `main.go` needed;
confirm this by reading the ~10 lines above the block being replaced before pasting.)

Grep once more (`grep -n "exportPrompt" corkboard.go`) — none should remain.

- [x] **Step 7: Render the chooser screen — add `exportChooserView` and wire it into both View() functions**

Add to `export.go` (or a new small block at the end of it):

```go
// exportChooserView renders the ctrl+e screen: 5 format checkboxes + a style radio, framed.
func exportChooserView(c exportChooserModel, width int) string {
	var lines []string
	for i, f := range exportFormatOrder {
		box := "[ ]"
		if c.checked[f] {
			box = "[x]"
		}
		line := box + " " + f.label()
		if i == c.cursor {
			line = selectedStyle.Render(line)
		}
		lines = append(lines, line)
	}
	styleLine := ""
	manuscriptMark, tufteMark := "( )", "( )"
	if c.style == StyleManuscript {
		manuscriptMark = "(•)"
	} else {
		tufteMark = "(•)"
	}
	styleLine = fmt.Sprintf("%s Manuscript   %s Tufte", manuscriptMark, tufteMark)
	if c.cursor == len(exportFormatOrder) {
		styleLine = selectedStyle.Render(styleLine)
	}
	lines = append(lines, "", "Style :", "  "+styleLine)
	inner := strings.Join(lines, "\n")
	return framedPanel("Export", inner, min(width-8, 40), len(lines)+2, "")
}
```

Add `"strings"` to `export.go`'s imports if not already present after Step 4's rewrite
(check — Step 4's version does not use `strings`, so this Step 7 addition needs it).

In `main.go`'s main `View()`/rendering function (search for where `m.exportPrompt` used to
gate a status-line-only render — since the old version was a one-liner in `composeStatus`
with no separate panel, this is a **new** render path, not a replacement). Find where other
full-screen overlays are composited (search `grep -n "m.moverPhase\|m.suggesting" main.go`
for an example of an existing bool/state-gated overlay render inside the main `View()`
function) and add, at an equivalent point:

```go
	if m.exportChooser != nil {
		panel := exportChooserView(*m.exportChooser, m.width)
		return lipgloss.PlaceHorizontal(m.width, lipgloss.Center, panel) // adapt to match
		// the exact compositing pattern used by the sibling overlay found above — read
		// that code path first and mirror its structure (some overlays are centered over
		// the existing screen with lipgloss.Place, not a bare replacement).
	}
```

**This step requires reading the existing `View()` structure in `main.go` before writing the
final version of this hook** — the exact insertion point and compositing pattern (full
replace vs. overlay-on-top) must match how `corkboard.go`'s own `exportPrompt` bar (a single
line appended via `lipgloss.PlaceHorizontal`, per the pre-Task-4 code at
`corkboard.go:461-464` — that block is itself replaced in Step 6, but is the reference for
"how does this codebase render a status/prompt bar over a screen") already does it, and how
`corkboard.go`'s `corkboardView()` needs a parallel hook for when a corkboard-launched
export chooser is up. Wire the equivalent render into `corkboardView()` as well, following
the same `framedPanel` + `lipgloss.PlaceHorizontal` centering pattern already used for
`m.structureAdding`'s `pick` panel at `corkboard.go:445-459`.

- [x] **Step 8: Run the chooser model unit tests**

Run: `export PATH="$HOME/.local/go/bin:$PATH" && go test -run TestExportChooser ./... -v && go test -run TestNewExportChooser ./... -v`
Expected: all PASS.

- [x] **Step 9: Build and vet**

Run: `go build ./... && go vet ./...`
Expected: clean. This will surface any leftover `m.exportPrompt`/`m.runExport(StyleX)`
call-site mismatches as compile errors — fix any found before proceeding (Task 5 will also
catch these via the integration tests, but a compile error here means Task 5 cannot start).

- [x] **Step 10: Commit**

```bash
git add export.go main.go corkboard.go export_chooser_test.go
git commit -m "$(cat <<'EOF'
Refond l'écran d'export en cases à cocher par format + style

Remplace le prompt m/t (qui écrivait toujours .rtf+.pdf+.docx+.odt)
par un écran combiné : 5 cases à cocher de format (RTF/PDF/DOCX/ODT/
EPUB, rien coché par défaut) + un choix de style radio
(Manuscript/Tufte), navigable ↑↓, espace coche, ←→ bascule le style,
↵ exporte (refuse si aucun format coché), esc annule. runExport() ne
produit plus que les formats effectivement cochés. m.exportPrompt
bool devient m.exportChooser *exportChooserModel (nil = fermé).

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Update export integration tests + add EPUB integration coverage

**Files:**
- Rewrite: `export_wiring_test.go`

**Interfaces:**
- Consumes: everything from Tasks 1-4 (`exportChooserModel`, `runExport()`,
  `writeEPUB`, `m.exportChooser`).
- Produces: nothing new — this task only updates test drivers to match the new interaction
  model and adds EPUB-specific coverage mirroring the existing per-format tests.

- [x] **Step 1: Read the current `export_wiring_test.go` in full**

Already read during plan research — six tests: `TestExportSingleDocFromEditor`,
`TestExportWholeManuscriptFromCorkboard`, `TestExportManifestManuscriptUsesManifestOrder`
(this one doesn't drive the UI at all, it calls `manuscriptDocFromChapters` directly — no
change needed), `TestExportEmitsDOCX`, `TestExportEmitsODT`, `TestExportCancel`. Every test
except `TestExportManifestManuscriptUsesManifestOrder` currently does:

```go
nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlE})
m = nm.(model)
if !m.exportPrompt { t.Fatal(...) }
nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
m = nm.(model)
```

This must become: press ctrl+e, check `m.exportChooser != nil`, move the cursor to the
target format row and press space, press enter.

- [x] **Step 2: Rewrite the file**

```go
package main

import (
	"archive/zip"
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

var _ = zip.Store // keep archive/zip imported if hasZipEntry lives in another file already importing it — remove this line if export_docx_test.go's import covers it package-wide (it does not; Go imports are per-file). Verify at Step 3 whether this line is actually needed by checking if any test in THIS file uses archive/zip directly — if not, delete both the import and this line.
```

- [x] **Step 3: Reconcile the `archive/zip` import**

`hasZipEntry` (defined in `export_docx_test.go`) is called by name in this file, which is
fine (same package, `main`) — but `export_wiring_test.go` itself doesn't need to import
`archive/zip` unless a test in it calls zip functions directly, which none of the above do.
Remove the `"archive/zip"` import and the placeholder `var _ = zip.Store` line from the
Step 2 code before saving — they were flagged deliberately as a checklist, not meant to ship.
The final import block should be:

```go
import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)
```

- [x] **Step 4: Run the full export test file**

Run: `export PATH="$HOME/.local/go/bin:$PATH" && go test -run TestExport ./... -v`
Expected: all PASS — `TestExportSingleDocFromEditor`, `TestExportWholeManuscriptFromCorkboard`,
`TestExportManifestManuscriptUsesManifestOrder`, `TestExportEmitsDOCX`, `TestExportEmitsODT`,
`TestExportEmitsEPUB`, `TestExportOnlyWritesCheckedFormats`,
`TestExportEnterWithNothingCheckedShowsError`, `TestExportCancel`.

- [x] **Step 5: Build, vet, full test suite**

Run: `go build ./... && go vet ./... && go test ./... -count=1`
Expected: clean build/vet, 0 FAIL across the whole module (rerun once if only
`TestSnippetCacheReadsHeadAndInvalidates` fails).

- [x] **Step 6: Commit**

```bash
git add export_wiring_test.go
git commit -m "$(cat <<'EOF'
Adapte les tests d'intégration export au nouvel écran de cases à cocher

Les tests pilotaient l'ancien prompt m/t par une frappe directe ;
adaptés pour naviguer la nouvelle liste de cases à cocher (↓ + espace
par format, ↵ pour valider). Ajoute TestExportEmitsEPUB,
TestExportOnlyWritesCheckedFormats (confirme qu'un format non coché
n'est pas écrit) et TestExportEnterWithNothingCheckedShowsError.

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Manual verification

**Files:** none — verification only, no code changes.

**Interfaces:**
- Consumes: the complete feature from Tasks 1-5.
- Produces: nothing new — confirms the whole thing works together visually, the way Task 13
  of the earlier UI-translation plan did.

- [x] **Step 1: Full build, vet, test suite**

Run: `export PATH="$HOME/.local/go/bin:$PATH" && go build ./... && go vet ./... && go test ./... -count=1 -v 2>&1 | tail -80`
Expected: clean build, no vet warnings, all tests PASS.

- [x] **Step 2: Manual walkthrough in tmux**

```bash
go build -o /tmp/forkashi-epub-check .
```

Set up an isolated test manuscript and an isolated `HOME`/`XDG_CONFIG_HOME` (to avoid leaking
any real personal projects into the RECENTS list — see this session's earlier precedent for
why this matters):

```bash
rm -rf /tmp/epub-check-home /tmp/epub-check-proj
mkdir -p /tmp/epub-check-home /tmp/epub-check-proj/Livre
cat > /tmp/epub-check-proj/Livre/manifest.json <<'EOF'
{"schemaVersion":1,"title":"Mon Livre","items":[{"file":"01-un.md","title":"Un"},{"file":"02-deux.md","title":"Deux"}]}
EOF
echo "Premier chapitre de test." > /tmp/epub-check-proj/Livre/01-un.md
echo "Second chapitre de test." > /tmp/epub-check-proj/Livre/02-deux.md
tmux new-session -d -s epubcheck -x 120 -y 40 \
  "cd /tmp/epub-check-proj/Livre && HOME=/tmp/epub-check-home XDG_CONFIG_HOME=/tmp/epub-check-home/.config OKASHI_DIR=/tmp/epub-check-proj/Livre /tmp/forkashi-epub-check"
sleep 1
tmux capture-pane -t epubcheck -p
```

Walk through, in order:

1. Confirm `(aucun fichier récent)` in RECENTS (confirms the isolated HOME took effect —
   otherwise stop and fix the isolation before continuing, per the earlier session's incident).
2. Navigate into the manuscript, open the corkboard (`ctrl+k`), press `ctrl+e`.
3. Confirm the chooser panel appears with 5 unchecked boxes and a style radio defaulted to
   Manuscript, **not** the old one-line `export : m manuscrit · t tufte` prompt.
4. Press space on the EPUB row, press space on the PDF row, press enter.
5. Confirm the status line reports something like `exporté mon-livre.pdf + .epub vers export/`.
6. `tmux send-keys -t epubcheck 'q'` is not a thing here — instead verify on disk:
   `ls /tmp/epub-check-proj/Livre/export/` should show exactly `mon-livre.pdf` and
   `mon-livre.epub`, **not** `.rtf`/`.docx`/`.odt`.
7. Open Properties (`i` from the hub — navigate back to home first with `ctrl+o`), confirm a
   "Couverture" field is present, showing `(aucune)` by default.
8. Re-open `ctrl+e` and confirm nothing is pre-checked from the previous export (fresh
   chooser each time, per the design's "rien coché par défaut" decision).

Document any visual anomaly (misalignment, overflow, wrong default) found during this walk —
note file:line if the cause is identifiable, without necessarily fixing it in this task if
it's minor; flag it as a finding instead.

- [x] **Step 3: Validate the EPUB with `pdfinfo`-equivalent tooling if available**

```bash
epub_file=/tmp/epub-check-proj/Livre/export/mon-livre.epub
python3 -c "
import zipfile
z = zipfile.ZipFile('$epub_file')
print('Entries:', z.namelist())
print('mimetype content:', z.read('mimetype'))
print('First entry:', z.infolist()[0].filename, 'compress_type =', z.infolist()[0].compress_type)
"
```

Expected: `mimetype` is the first entry, `compress_type = 0` (stored, not deflated), content
is exactly `application/epub+zip`, and `OEBPS/chapter-01.xhtml`/`chapter-02.xhtml` are both
present.

- [x] **Step 4: Clean up**

```bash
tmux send-keys -t epubcheck 'C-c'
tmux kill-session -t epubcheck 2>/dev/null
rm -f /tmp/forkashi-epub-check
rm -rf /tmp/epub-check-home /tmp/epub-check-proj
```

- [x] **Step 5: Report findings**

No commit for this task (verification only). If Step 2 surfaced any visual anomaly, report
it — do not silently fix it under this task unless it's a one-line, obviously-safe
correction; otherwise treat it as a follow-up.

---

## Self-Review

**Spec coverage:**
- §1 (EPUB writer architecture, reuses shared AST) → Task 1.
- §2 (combined chooser screen, nothing checked by default, ↑↓/space/←→/↵/esc) → Task 4.
- §3 (Properties Couverture field, JPEG/PNG only, silent fallback + status warning) → Tasks
  2 (storage), 3 (UI field), 1 (the actual fallback logic in `epubCoverImage`), 4 (wiring the
  resolved cover path + warning into `runExport`).
- §4 (atomic writes, error handling, stop-on-first-error) → Task 4's `runExport` rewrite.
- §5 (tests mirroring existing writer test patterns, EPUB-only ZIP validation, no
  `epubcheck`) → Task 1 (unit), Task 5 (integration).
- Hors périmètre (no popup footnotes, no CSS customization, no file picker, no MOBI/AZW3) —
  none of these are implemented by any task above; confirmed absent by design.

**Placeholder scan:** no "TBD"/"TODO"/"add appropriate error handling" found. Task 4 Step 7
contains an explicit instruction to *read existing code before finalizing* rather than a
placeholder — this is intentional: the exact `View()` compositing point cannot be pinned to a
line number without risking staleness, so the step directs the implementer to the concrete
reference pattern (`corkboard.go`'s pre-existing `m.structureAdding` panel) rather than
leaving a vague "wire it in" instruction.

**Type consistency:** `writeEPUB(doc ManuscriptDoc, st ExportStyle, meta Meta, coverPath
string) ([]byte, error)` — signature fixed in Task 1, used identically in Task 4's
`runExport`. `exportFormat`/`exportFormatOrder`/`exportChooserModel`/`newExportChooser`/
`toggleAtCursor`/`anyChecked`/`setStyle` — all defined once in Task 4 Step 4, used
identically in Task 4 Steps 5-7 and Task 5's test helpers. `effectiveSettings.Cover` (Task 2)
→ read as `eff.Cover` in Task 4's `runExport` — consistent field name throughout.
