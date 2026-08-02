package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
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
