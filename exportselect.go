package main

import (
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// exportSelectEntry is one flattened row on the "all texts" screen: a chapter header, an
// indented scene, a standalone scene, or a Resource.
type exportSelectEntry struct {
	file     string // path relative to the manuscript dir — the sidecar's exclusion key
	title    string
	preview  string // first 111 chars, whitespace-normalized, ellipsis if truncated
	words    int
	chars    int // runes, spaces included
	excluded bool
	indent   bool // true for a scene inside a chapter (rendered indented)
	isHeader bool // true for a chapter header row (its own words/chars don't count — its child scenes do)
}

type exportSelectModel struct {
	entries []exportSelectEntry
	sel     int
}

// previewOf returns the first 111 runes of data's content, internal whitespace collapsed to
// single spaces, with a trailing "…" if the source was longer than 111 runes.
func previewOf(data []byte) string {
	fields := strings.Fields(string(data)) // splits on any whitespace, drops runs
	joined := strings.Join(fields, " ")
	runes := []rune(joined)
	if len(runes) <= 111 {
		return joined
	}
	return string(runes[:111]) + "…"
}

// buildExportSelectEntries resolves dir's manuscript into the flat display list: chapter
// headers + indented scenes, then standalone scenes, then Resources (alphabetical, matching
// v.loose's existing order). excluded is the loaded sidecar map, keyed by the same relative
// path used as each entry's file field.
func buildExportSelectEntries(dir string, v manuscriptView, excluded map[string]bool) []exportSelectEntry {
	var out []exportSelectEntry
	for _, part := range v.parts {
		for _, ch := range part.chapters {
			if ch.scene {
				continue // standalone scenes are collected in a second pass below, after chapters
			}
			out = append(out, exportSelectEntry{title: ch.title, isHeader: true})
			for _, t := range ch.texts {
				rel := filepath.Join(ch.folder, t.file)
				out = append(out, buildEntry(dir, rel, t.title, true, excluded))
			}
		}
	}
	for _, part := range v.parts {
		for _, ch := range part.chapters {
			if !ch.scene || len(ch.texts) == 0 {
				continue
			}
			rel := ch.texts[0].file
			out = append(out, buildEntry(dir, rel, ch.title, false, excluded))
		}
	}
	for _, e := range v.loose {
		out = append(out, buildEntry(dir, e.name, sectionTitle(e.name), false, excluded))
	}
	return out
}

// buildEntry reads rel's content once to compute words/chars/preview and reports its excluded
// state from the sidecar map.
func buildEntry(dir, rel, title string, indent bool, excluded map[string]bool) exportSelectEntry {
	data, _ := os.ReadFile(filepath.Join(dir, rel)) // best-effort: an unreadable file gets zero counts, not a crash
	text := string(data)
	return exportSelectEntry{
		file:     rel,
		title:    title,
		preview:  previewOf(data),
		words:    wordCount(text),
		chars:    charCount(text),
		excluded: excluded[rel],
		indent:   indent,
	}
}

// enterExportSelect resolves the current manuscript into the flat display list and shows the
// screen. All I/O happens here, once, before m.screen flips — exportSelectView stays I/O-free.
func (m *model) enterExportSelect() {
	dir := m.files.dir
	v := resolveManuscript(dir, readEntries(dir))
	excluded := loadExportSelection(dir)
	m.exportSelect = exportSelectModel{entries: buildExportSelectEntries(dir, v, excluded)}
	m.screen = screenExportSelect
}

func (m model) updateExportSelect(msg tea.Msg) (tea.Model, tea.Cmd) {
	if sz, ok := msg.(tea.WindowSizeMsg); ok {
		m.width, m.height = sz.Width, sz.Height
		return m, nil
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.screen = screenWriting
		m.focus = focusSidebar
	case "up", "k":
		if m.exportSelect.sel > 0 {
			m.exportSelect.sel--
		}
	case "down", "j":
		if m.exportSelect.sel < len(m.exportSelect.entries)-1 {
			m.exportSelect.sel++
		}
	}
	return m, nil
}

// exportSelectTotals sums words/chars across every non-header, non-excluded entry.
func exportSelectTotals(entries []exportSelectEntry) (words, chars int) {
	for _, e := range entries {
		if e.isHeader || e.excluded {
			continue
		}
		words += e.words
		chars += e.chars
	}
	return words, chars
}

func exportSelectView(m model) string {
	a := m.exportSelect
	header := lipgloss.NewStyle().Foreground(accent).Bold(true).Render("── tous les textes ")

	var rows []string
	if len(a.entries) == 0 {
		rows = append(rows, lipgloss.NewStyle().Foreground(subtle).Render("  (aucun texte dans ce manuscrit)"))
	} else {
		width := max(10, min(m.width-8, 72))
		for i, e := range a.entries {
			box := "[ ]"
			if e.excluded {
				box = "[ ]"
			} else if !e.isHeader {
				box = "[x]"
			}
			indent := ""
			if e.indent {
				indent = "  "
			}
			line := indent + box + " " + ansi.Truncate(e.title, width, "…")
			if i == a.sel {
				line = selectedStyle.Render("▸ " + line)
			} else {
				line = "  " + line
			}
			rows = append(rows, line)
			if !e.isHeader {
				preview := lipgloss.NewStyle().Foreground(subtle).Render(indent + "    " + ansi.Truncate(e.preview, width, "…"))
				rows = append(rows, preview)
			}
		}
	}
	body := header + "\n\n" + strings.Join(rows, "\n")

	words, chars := exportSelectTotals(a.entries)
	footer := commafy(words) + " mots · " + commafy(chars) + " caractères (espaces comprises) inclus dans l'export"

	var b strings.Builder
	b.WriteString(lipgloss.Place(m.width, m.height-2, lipgloss.Center, lipgloss.Center, body))
	b.WriteString("\n" + lipgloss.PlaceHorizontal(m.width, lipgloss.Center, lipgloss.NewStyle().Foreground(accent).Render(footer)))
	foot := lipgloss.NewStyle().Foreground(subtle).Render("↑↓ sélectionner · espace inclure/exclure · shift+↑↓ déplacer · esc retour · F1 aide")
	b.WriteString("\n" + lipgloss.PlaceHorizontal(m.width, lipgloss.Center, foot))
	return b.String()
}
