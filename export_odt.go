package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"strings"
)

const (
	odtMimetype = "application/vnd.oasis.opendocument.text"

	odtManifest = `<?xml version="1.0" encoding="UTF-8"?>
<manifest:manifest xmlns:manifest="urn:oasis:names:tc:opendocument:xmlns:manifest:1.0" manifest:version="1.2"><manifest:file-entry manifest:full-path="/" manifest:version="1.2" manifest:media-type="application/vnd.oasis.opendocument.text"/><manifest:file-entry manifest:full-path="content.xml" manifest:media-type="text/xml"/><manifest:file-entry manifest:full-path="styles.xml" manifest:media-type="text/xml"/></manifest:manifest>`

	// styles.xml defines paragraph styles referenced by content.xml: Title (chapter
	// heading, centered bold), Body (manuscript double-spaced first-line indent, or
	// plain for Tufte), Quote (indented italic), SceneBreak (centered).
	odtStylesTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<office:document-styles xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0" xmlns:style="urn:oasis:names:tc:opendocument:xmlns:style:1.0" xmlns:fo="urn:oasis:names:tc:opendocument:xmlns:xsl-fo-compatible:1.0" office:version="1.2">
<office:styles>
<style:style style:name="Title" style:family="paragraph"><style:paragraph-properties fo:text-align="center" fo:break-before="page" fo:margin-top="0.4in" fo:margin-bottom="0.2in"/><style:text-properties fo:font-weight="bold" fo:font-size="14pt" style:font-name="%s"/></style:style>
<style:style style:name="Body" style:family="paragraph"><style:paragraph-properties %s/><style:text-properties style:font-name="%s" fo:font-size="12pt"/></style:style>
<style:style style:name="Quote" style:family="paragraph"><style:paragraph-properties fo:margin-left="0.5in"/><style:text-properties fo:font-style="italic" style:font-name="%s" fo:font-size="12pt"/></style:style>
<style:style style:name="SceneBreak" style:family="paragraph"><style:paragraph-properties fo:text-align="center" fo:margin-top="0.2in" fo:margin-bottom="0.2in"/><style:text-properties style:font-name="%s" fo:font-size="12pt"/></style:style>
<style:style style:name="Bold" style:family="text"><style:text-properties fo:font-weight="bold"/></style:style>
<style:style style:name="Italic" style:family="text"><style:text-properties fo:font-style="italic"/></style:style>
</office:styles>
</office:document-styles>`
)

func odtFont(st ExportStyle) string {
	if st == StyleTufte {
		return "Georgia"
	}
	return "Times New Roman"
}

func odtBodyParagraphProps(st ExportStyle) string {
	if st == StyleManuscript {
		return `fo:margin-top="0in" fo:line-height="200%" fo:text-indent="0.5in"`
	}
	return ""
}

// odtRun renders a Run as ODF <text:span> (plain runs skip the span wrapper).
func odtRun(r Run) string {
	esc := xmlEsc(r.Text)
	if r.Bold && r.Italic {
		return `<text:span text:style-name="Bold"><text:span text:style-name="Italic">` + esc + `</text:span></text:span>`
	}
	if r.Bold {
		return `<text:span text:style-name="Bold">` + esc + `</text:span>`
	}
	if r.Italic {
		return `<text:span text:style-name="Italic">` + esc + `</text:span>`
	}
	return esc
}

func odtPara(runs []Run, styleName string) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<text:p text:style-name="%s">`, styleName)
	for _, r := range runs {
		b.WriteString(odtRun(r))
	}
	b.WriteString("</text:p>")
	return b.String()
}

func writeODT(doc ManuscriptDoc, st ExportStyle, meta Meta) ([]byte, error) {
	font := odtFont(st)
	bodyProps := odtBodyParagraphProps(st)
	styles := fmt.Sprintf(odtStylesTemplate, font, bodyProps, font, font, font)

	var body strings.Builder
	body.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	body.WriteString(`<office:document-content xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0" xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0" office:version="1.2"><office:body><office:text>`)
	for _, sec := range doc {
		body.WriteString(odtPara([]Run{{Text: sec.Title}}, "Title"))
		for _, blk := range sec.Blocks {
			writeBlockODT(&body, blk, bodyProps == "")
		}
	}
	body.WriteString(`</office:text></office:body></office:document-content>`)

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	// mimetype MUST be the first entry, stored uncompressed (ODF spec requirement,
	// lets tools identify the format without decompressing).
	mw, err := zw.CreateHeader(&zip.FileHeader{Name: "mimetype", Method: zip.Store})
	if err != nil {
		return nil, err
	}
	if _, err := mw.Write([]byte(odtMimetype)); err != nil {
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
	if err := add("META-INF/manifest.xml", odtManifest); err != nil {
		return nil, err
	}
	if err := add("content.xml", body.String()); err != nil {
		return nil, err
	}
	if err := add("styles.xml", styles); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// writeBlockODT mirrors writeBlockDOCX's degrade decisions: lists → literal-prefixed
// paragraphs, blockquote → Quote-styled paragraphs, endnotes → heading + numbered
// paras, scene break → centered #. unused bool param kept symmetric with DOCX/RTF
// writers for readability at call sites (bodyStyle picks Body vs plain paragraph).
func writeBlockODT(b *strings.Builder, blk Block, _ bool) {
	switch v := blk.(type) {
	case Paragraph:
		b.WriteString(odtPara(v.Runs, "Body"))
	case Heading:
		b.WriteString(odtPara(boldRuns(v.Runs), "Title"))
	case SceneBreak:
		b.WriteString(odtPara([]Run{{Text: "#"}}, "SceneBreak"))
	case Blockquote:
		for _, c := range v.Children {
			if p, ok := c.(Paragraph); ok {
				b.WriteString(odtPara(p.Runs, "Quote"))
			} else {
				writeBlockODT(b, c, true)
			}
		}
	case List:
		for i, it := range v.Items {
			prefix := "• "
			if v.Ordered {
				prefix = fmt.Sprintf("%d. ", v.Start+i)
			}
			runs := append([]Run{{Text: prefix}}, it.Runs...)
			b.WriteString(odtPara(runs, "Body"))
		}
	case Endnotes:
		if len(v.Items) == 0 {
			return
		}
		b.WriteString(odtPara([]Run{{Text: "Notes"}}, "Title"))
		for _, e := range v.Items {
			runs := append([]Run{{Text: fmt.Sprintf("%d. ", e.Num)}}, e.Runs...)
			b.WriteString(odtPara(runs, "Body"))
		}
	}
}
