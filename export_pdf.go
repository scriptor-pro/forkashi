package main

import (
	"bytes"
	"fmt"
	"strings"

	"codeberg.org/go-pdf/fpdf"
	"golang.org/x/text/encoding/charmap"
)

// cp1252 transcodes UTF-8 to Windows-1252 for fpdf's core fonts (Courier/Times), which
// can't render raw UTF-8. Un-encodable runes (e.g. emoji) fall back to '?'.
func cp1252(s string) string {
	enc := charmap.Windows1252.NewEncoder()
	if out, err := enc.String(s); err == nil {
		return out
	}
	var b strings.Builder
	for _, r := range s {
		if e, err := charmap.Windows1252.NewEncoder().String(string(r)); err == nil {
			b.WriteString(e)
		} else {
			b.WriteByte('?')
		}
	}
	return b.String()
}

// pdfEnc transcodes for the core-font (manuscript Courier) path; the Tufte path (embedded
// UTF-8 ET Book) strips astral runes that the font can't render.
func pdfEnc(st ExportStyle, s string) string {
	if st == StyleTufte {
		return stripAstral(s)
	}
	return cp1252(s)
}

// stripAstral replaces runes above the BMP with '?'. fpdf's UTF-8 (embedded-font)
// support is BMP-only, so an astral rune (e.g. an emoji) would otherwise error or
// panic the Tufte PDF. The manuscript path's cp1252 transcode already drops these.
func stripAstral(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r > 0xFFFF {
			b.WriteByte('?')
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func plainText(runs []Run) string {
	var b strings.Builder
	for _, r := range runs {
		b.WriteString(r.Text)
	}
	return b.String()
}

func hasEmphasis(runs []Run) bool {
	for _, r := range runs {
		if r.Bold || r.Italic {
			return true
		}
	}
	return false
}

// pdfStyle holds the per-style typography knobs.
type pdfStyle struct {
	font       string
	bodySize   float64
	titleSize  float64
	lineHeight float64
	indent     string
}

func writePDF(doc ManuscriptDoc, st ExportStyle, meta Meta) (out []byte, err error) {
	defer func() {
		if r := recover(); r != nil {
			out, err = nil, fmt.Errorf("pdf render failed: %v", r)
		}
	}()
	pdf := fpdf.New("P", "pt", "A4", "")
	cfg := pdfStyle{font: "Courier", bodySize: 12, titleSize: 14, lineHeight: 24, indent: "     "}
	if st == StyleTufte {
		registerETBook(pdf)
		cfg = pdfStyle{font: "etbook", bodySize: 12, titleSize: 16, lineHeight: 17, indent: ""}
		pdf.SetMargins(108, 90, 108)
	} else {
		pdf.SetMargins(72, 72, 72)
		pdf.AliasNbPages("{nb}")
		hasTitle := meta.TitlePage
		pdf.SetHeaderFunc(func() {
			if hasTitle && pdf.PageNo() == 1 {
				return // the title page carries no running header
			}
			pdf.SetFont("Courier", "", 12)
			hdr := fmt.Sprintf("%s / %s / %d", meta.Author, strings.ToUpper(meta.Title), pdf.PageNo())
			pdf.CellFormat(0, 14, pdfEnc(st, hdr), "", 0, "R", false, 0, "")
			pdf.Ln(24)
		})
	}
	pdf.SetAutoPageBreak(true, 72)

	if st == StyleManuscript && meta.TitlePage {
		writeTitlePagePDF(pdf, st, meta, manuscriptWordCount(doc))
	}
	for _, sec := range doc {
		pdf.AddPage()
		pdf.SetFont(cfg.font, "B", cfg.titleSize)
		pdf.CellFormat(0, cfg.lineHeight, pdfEnc(st, sec.Title), "", 1, "C", false, 0, "")
		pdf.Ln(cfg.lineHeight)
		pdf.SetFont(cfg.font, "", cfg.bodySize)
		for _, blk := range sec.Blocks {
			writeBlockPDF(pdf, blk, st, cfg)
		}
	}
	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// writeTitlePagePDF renders the Shunn title page as page 1: contact block top-left and word
// count top-right (same top band), title + byline centered near the vertical middle. The
// header func suppresses the running header on this page; the first section's AddPage follows.
func writeTitlePagePDF(pdf *fpdf.Fpdf, st ExportStyle, meta Meta, words int) {
	pdf.AddPage()
	pdf.SetFont("Courier", "", 12)
	topY := pdf.GetY()
	// Word count, top-right.
	pdf.CellFormat(0, 14, pdfEnc(st, approxWords(words)), "", 1, "R", false, 0, "")
	// Contact block, top-left, starting on the same band.
	pdf.SetXY(72, topY)
	var left []string
	if meta.Author != "" {
		left = append(left, meta.Author)
	}
	left = append(left, contactLines(meta.Contact)...)
	for _, ln := range left {
		pdf.CellFormat(0, 14, pdfEnc(st, ln), "", 1, "L", false, 0, "")
	}
	// Title + byline near the vertical middle (A4 is 841.89pt tall).
	pdf.SetY(383)
	pdf.CellFormat(0, 14, pdfEnc(st, meta.Title), "", 1, "C", false, 0, "")
	if meta.Author != "" {
		pdf.CellFormat(0, 14, pdfEnc(st, "by "+meta.Author), "", 1, "C", false, 0, "")
	}
}

func writeBlockPDF(pdf *fpdf.Fpdf, blk Block, st ExportStyle, cfg pdfStyle) {
	switch v := blk.(type) {
	case Paragraph:
		if hasEmphasis(v.Runs) {
			// MultiCell can't switch font mid-cell, so an emphasized paragraph renders
			// run-by-run with SetFont + Write (no HTML, so no entity escaping). This
			// loses the first-line indent, which is acceptable for emphasized paragraphs.
			for _, r := range v.Runs {
				style := ""
				if r.Bold {
					style += "B"
				}
				if r.Italic {
					style += "I"
				}
				pdf.SetFont(cfg.font, style, cfg.bodySize)
				pdf.Write(cfg.lineHeight, pdfEnc(st, r.Text))
			}
			pdf.SetFont(cfg.font, "", cfg.bodySize)
			pdf.Ln(cfg.lineHeight)
		} else {
			pdf.MultiCell(0, cfg.lineHeight, pdfEnc(st, cfg.indent+plainText(v.Runs)), "", "L", false)
		}
	case Heading:
		pdf.SetFont(cfg.font, "B", 13)
		pdf.MultiCell(0, cfg.lineHeight, pdfEnc(st, plainText(v.Runs)), "", "L", false)
		pdf.SetFont(cfg.font, "", cfg.bodySize)
	case Endnotes:
		pdf.Ln(cfg.lineHeight)
		pdf.SetFont(cfg.font, "B", cfg.bodySize)
		pdf.MultiCell(0, cfg.lineHeight, pdfEnc(st, "Notes"), "", "L", false)
		pdf.SetFont(cfg.font, "", cfg.bodySize)
		for _, e := range v.Items {
			pdf.MultiCell(0, cfg.lineHeight, pdfEnc(st, fmt.Sprintf("%d. %s", e.Num, plainText(e.Runs))), "", "L", false)
		}
	case SceneBreak:
		pdf.CellFormat(0, cfg.lineHeight, "#", "", 1, "C", false, 0, "")
	case Blockquote:
		for _, c := range v.Children {
			writeBlockPDF(pdf, c, st, cfg)
		}
	case List:
		for i, it := range v.Items {
			marker := "- "
			if v.Ordered {
				marker = fmt.Sprintf("%d. ", v.Start+i)
			}
			pdf.MultiCell(0, cfg.lineHeight, pdfEnc(st, marker+plainText(it.Runs)), "", "L", false)
		}
	}
}
