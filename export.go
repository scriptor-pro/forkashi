package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
var exportChooserRowCount = len(exportFormatOrder) + 1

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
		doc = manuscriptDocFromChapters(dir, v.parts, nil)
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
