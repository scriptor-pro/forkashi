package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// exportWholeManuscript reports whether ctrl+e should export the whole manuscript (from the
// full-screen corkboard) rather than just the current document.
func (m model) exportWholeManuscript() bool {
	return m.screen == screenCorkboard
}

// runExport builds the export doc for the current scope (whole manuscript from the corkboard,
// else the current document) and writes <slug>.rtf + <slug>.pdf under <dir>/export/.
func (m *model) runExport(st ExportStyle) {
	defer func() {
		if r := recover(); r != nil {
			m.status = fmt.Sprintf("export failed: %v", r)
		}
	}()
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

	// A Shunn title page is for a whole-manuscript submission, not a single-chapter export.
	// Identity resolves through Properties (personal config) with the OKASHI_* env as fallback.
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
	rtfPath := filepath.Join(outDir, slug+".rtf")
	pdfPath := filepath.Join(outDir, slug+".pdf")
	if err := atomicWrite(rtfPath, writeRTF(doc, st, meta), 0o644); err != nil {
		m.status = "export failed: " + err.Error()
		return
	}
	pdfBytes, err := writePDF(doc, st, meta)
	if err != nil {
		m.status = "export failed (pdf): " + err.Error()
		return
	}
	if err := atomicWrite(pdfPath, pdfBytes, 0o644); err != nil {
		m.status = "export failed: " + err.Error()
		return
	}
	docxPath := filepath.Join(outDir, slug+".docx")
	docxBytes, err := writeDOCX(doc, st, meta)
	if err != nil {
		m.status = "export failed (docx): " + err.Error()
		return
	}
	if err := atomicWrite(docxPath, docxBytes, 0o644); err != nil {
		m.status = "export failed: " + err.Error()
		return
	}
	odtPath := filepath.Join(outDir, slug+".odt")
	odtBytes, err := writeODT(doc, st, meta)
	if err != nil {
		m.status = "export failed (odt): " + err.Error()
		return
	}
	if err := atomicWrite(odtPath, odtBytes, 0o644); err != nil {
		m.status = "export failed: " + err.Error()
		return
	}
	m.status = "exported " + slug + ".rtf + .pdf + .docx + .odt to export/"
}
