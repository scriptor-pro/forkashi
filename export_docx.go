package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"strings"
)

const (
	docxContentTypes = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`

	docxRootRels = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`

	docxSectPr = `<w:sectPr><w:pgSz w:w="11906" w:h="16838"/><w:pgMar w:top="1440" w:right="1440" w:bottom="1440" w:left="1440"/></w:sectPr>`
)

// xmlEsc escapes text for an OOXML <w:t> value.
func xmlEsc(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(s)
}

func docxFont(st ExportStyle) string {
	if st == StyleTufte {
		return "Georgia"
	}
	return "Times New Roman"
}

// docxRun renders a Run as a <w:r>.
func docxRun(r Run, font string) string {
	var pr strings.Builder
	pr.WriteString("<w:rPr>")
	if r.Bold {
		pr.WriteString("<w:b/>")
	}
	if r.Italic {
		pr.WriteString("<w:i/>")
	}
	fmt.Fprintf(&pr, `<w:rFonts w:ascii="%s" w:hAnsi="%s"/><w:sz w:val="24"/>`, font, font)
	pr.WriteString("</w:rPr>")
	return "<w:r>" + pr.String() + `<w:t xml:space="preserve">` + xmlEsc(r.Text) + "</w:t></w:r>"
}

// docxPara builds a <w:p>. pPrExtra is inserted inside <w:pPr> (alignment/indent/spacing/break).
func docxPara(runs []Run, font, pPrExtra string) string {
	var b strings.Builder
	b.WriteString("<w:p><w:pPr>")
	b.WriteString(pPrExtra)
	b.WriteString("</w:pPr>")
	for _, r := range runs {
		b.WriteString(docxRun(r, font))
	}
	b.WriteString("</w:p>")
	return b.String()
}

func writeDOCX(doc ManuscriptDoc, st ExportStyle, meta Meta) ([]byte, error) {
	font := docxFont(st)
	bodyIndent := ""
	if st == StyleManuscript {
		bodyIndent = `<w:spacing w:line="480" w:lineRule="auto"/><w:ind w:firstLine="720"/>`
	}
	var body strings.Builder
	body.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n")
	body.WriteString(`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>`)
	titlePage := st == StyleManuscript && meta.TitlePage
	if titlePage {
		writeTitlePageDOCX(&body, meta, font, manuscriptWordCount(doc))
	}
	for i, sec := range doc {
		// Chapter title: centered, bold, new page (except the first — unless a title page precedes it).
		pageBreak := ""
		if i > 0 || titlePage {
			pageBreak = "<w:pageBreakBefore/>"
		}
		body.WriteString(docxPara([]Run{{Text: sec.Title, Bold: true}}, font,
			pageBreak+`<w:jc w:val="center"/><w:spacing w:before="480" w:after="240"/>`))
		for _, blk := range sec.Blocks {
			writeBlockDOCX(&body, blk, font, bodyIndent)
		}
	}
	body.WriteString(docxSectPr + "</w:body></w:document>")

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	add := func(name, content string) error {
		w, err := zw.Create(name)
		if err != nil {
			return err
		}
		_, err = w.Write([]byte(content))
		return err
	}
	if err := add("[Content_Types].xml", docxContentTypes); err != nil {
		return nil, err
	}
	if err := add("_rels/.rels", docxRootRels); err != nil {
		return nil, err
	}
	if err := add("word/document.xml", body.String()); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// writeTitlePageDOCX prepends a Shunn title page: word count top-right, contact block top-left,
// title + byline pushed toward the vertical middle. The first chapter gets pageBreakBefore.
func writeTitlePageDOCX(b *strings.Builder, meta Meta, font string, words int) {
	b.WriteString(docxPara([]Run{{Text: approxWords(words)}}, font, `<w:jc w:val="right"/>`))
	if meta.Author != "" {
		b.WriteString(docxPara([]Run{{Text: meta.Author}}, font, ""))
	}
	for _, ln := range contactLines(meta.Contact) {
		b.WriteString(docxPara([]Run{{Text: ln}}, font, ""))
	}
	b.WriteString(docxPara([]Run{{Text: meta.Title}}, font, `<w:jc w:val="center"/><w:spacing w:before="5760"/>`))
	if meta.Author != "" {
		b.WriteString(docxPara([]Run{{Text: "by " + meta.Author}}, font, `<w:jc w:val="center"/>`))
	}
}

// writeBlockDOCX mirrors writeBlockRTF's block handling. Degrade decisions:
// lists → literal-prefixed paragraphs, blockquote → indented italic,
// endnotes → a "Notes" heading + numbered paras, scene break → centered #.
func writeBlockDOCX(b *strings.Builder, blk Block, font, bodyIndent string) {
	switch v := blk.(type) {
	case Paragraph:
		b.WriteString(docxPara(v.Runs, font, bodyIndent))
	case Heading:
		b.WriteString(docxPara(boldRuns(v.Runs), font, `<w:spacing w:before="240" w:after="120"/>`))
	case SceneBreak:
		b.WriteString(docxPara([]Run{{Text: "#"}}, font, `<w:jc w:val="center"/><w:spacing w:before="240" w:after="240"/>`))
	case Blockquote:
		for _, c := range v.Children {
			if p, ok := c.(Paragraph); ok {
				b.WriteString(docxPara(italicRuns(p.Runs), font, `<w:ind w:left="720"/>`))
			} else {
				writeBlockDOCX(b, c, font, bodyIndent)
			}
		}
	case List:
		for i, it := range v.Items {
			prefix := "• "
			if v.Ordered {
				prefix = fmt.Sprintf("%d. ", v.Start+i)
			}
			runs := append([]Run{{Text: prefix}}, it.Runs...)
			b.WriteString(docxPara(runs, font, `<w:ind w:left="360"/>`))
		}
	case Endnotes:
		if len(v.Items) == 0 {
			return
		}
		b.WriteString(docxPara([]Run{{Text: "Notes", Bold: true}}, font, `<w:spacing w:before="480" w:after="120"/>`))
		for _, e := range v.Items {
			runs := append([]Run{{Text: fmt.Sprintf("%d. ", e.Num)}}, e.Runs...)
			b.WriteString(docxPara(runs, font, ""))
		}
	}
}

// boldRuns / italicRuns force a style on a run slice (copy — don't mutate the AST).
func boldRuns(rs []Run) []Run {
	out := make([]Run, len(rs))
	for i, r := range rs {
		r.Bold = true
		out[i] = r
	}
	return out
}

func italicRuns(rs []Run) []Run {
	out := make([]Run, len(rs))
	for i, r := range rs {
		r.Italic = true
		out[i] = r
	}
	return out
}
