package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"okashi/internal/textarea"
)

const notesDirName = ".okashi-notes"
const notesSchemaVersion = 1

// note is one author annotation. v1 ships chapter-scoped notes; the quote/prefix/suffix/lineHint
// fields are reserved for v2 line/sentence anchoring and stay absent for chapter notes.
type note struct {
	ID        string `json:"id"`
	Scope     string `json:"scope"` // "chapter" (v1); "line" reserved for v2
	Text      string `json:"text"`
	CreatedAt string `json:"createdAt,omitempty"`
	Quote     string `json:"quote,omitempty"`
	Prefix    string `json:"prefix,omitempty"`
	Suffix    string `json:"suffix,omitempty"`
	LineHint  int    `json:"lineHint,omitempty"`
}

type notesFile struct {
	SchemaVersion int    `json:"schemaVersion"`
	Notes         []note `json:"notes"`
}

// notesPath is <dir>/.okashi-notes/<base>.json for a source file (okashi-owned sidecar, NOT the
// manifest — the shared-contract HARD GATE stays untriggered).
func notesPath(file string) string {
	base := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
	return filepath.Join(filepath.Dir(file), notesDirName, base+".json")
}

// loadNotes reads a file's notes; missing/corrupt/unsupported-schema → nil (tolerant).
func loadNotes(file string) []note {
	data, err := os.ReadFile(notesPath(file))
	if err != nil {
		return nil
	}
	var nf notesFile
	if json.Unmarshal(data, &nf) != nil || nf.SchemaVersion != notesSchemaVersion {
		return nil
	}
	return nf.Notes
}

// saveNotes writes a file's notes atomically; an empty set removes the sidecar so no empty files linger.
func saveNotes(file string, notes []note) error {
	if len(notes) == 0 {
		_ = os.Remove(notesPath(file))
		return nil
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(notesFile{SchemaVersion: notesSchemaVersion, Notes: notes}); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(notesPath(file)), 0o755); err != nil {
		return err
	}
	return atomicWrite(notesPath(file), bytes.TrimRight(buf.Bytes(), "\n"), 0o644)
}

// moveNotes moves a file's notes sidecar to follow a disk rename (best-effort; no-op if none).
func moveNotes(oldFile, newFile string) {
	op := notesPath(oldFile)
	if _, err := os.Stat(op); err != nil {
		return
	}
	np := notesPath(newFile)
	_ = os.MkdirAll(filepath.Dir(np), 0o755)
	_ = os.Rename(op, np)
}

// deleteNotes drops a file's notes sidecar (on file delete).
func deleteNotes(file string) { _ = os.Remove(notesPath(file)) }

// --- Notes screen -----------------------------------------------------------------------------

type notesModel struct {
	file          string
	notes         []note
	sel           int
	editing       bool // editing an existing note
	adding        bool // composing a new note
	area          textarea.Model
	confirmDelete bool
}

func newNotesArea(val string) textarea.Model {
	ta := textarea.New()
	ta.Prompt = ""
	ta.ShowLineNumbers = false
	ta.CharLimit = 0
	ta.MaxHeight = 0
	ta.SetWidth(60)
	ta.SetHeight(3)
	ta.SetValue(val)
	return ta
}

// enterNotes opens the Notes screen for the selected sidebar file.
func (m *model) enterNotes() {
	file, ok := m.files.selectedFile()
	if !ok {
		m.status = "sélectionnez un fichier pour ajouter des notes"
		return
	}
	m.notes = notesModel{file: file, notes: loadNotes(file)}
	m.screen = screenNotes
}

func (m *model) notesSave() {
	if err := saveNotes(m.notes.file, m.notes.notes); err != nil {
		m.status = "échec de l'enregistrement des notes : " + err.Error()
	}
}

func (m model) updateNotes(msg tea.Msg) (tea.Model, tea.Cmd) {
	if sz, ok := msg.(tea.WindowSizeMsg); ok {
		m.width, m.height = sz.Width, sz.Height
		return m, nil
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	n := &m.notes
	ks := key.String()

	if ks == "ctrl+c" {
		return m, tea.Quit
	}

	if n.adding || n.editing {
		if ks == "esc" { // esc commits (⏎ inserts line breaks)
			text := strings.TrimRight(n.area.Value(), "\n")
			if n.adding {
				if text != "" {
					n.notes = append(n.notes, note{
						ID:        fmt.Sprintf("n%d", time.Now().UnixNano()),
						Scope:     "chapter",
						Text:      text,
						CreatedAt: time.Now().UTC().Format(time.RFC3339),
					})
					n.sel = len(n.notes) - 1
				}
			} else if n.sel < len(n.notes) {
				if text == "" {
					n.notes = append(n.notes[:n.sel], n.notes[n.sel+1:]...)
					if n.sel >= len(n.notes) && n.sel > 0 {
						n.sel--
					}
				} else {
					n.notes[n.sel].Text = text
				}
			}
			n.adding, n.editing = false, false
			n.area.Blur()
			m.notesSave()
			return m, nil
		}
		var cmd tea.Cmd
		n.area, cmd = n.area.Update(key)
		return m, cmd
	}

	if n.confirmDelete {
		switch ks {
		case "y":
			if n.sel < len(n.notes) {
				n.notes = append(n.notes[:n.sel], n.notes[n.sel+1:]...)
				if n.sel >= len(n.notes) && n.sel > 0 {
					n.sel--
				}
				m.notesSave()
			}
			n.confirmDelete = false
		case "esc", "n":
			n.confirmDelete = false
		}
		return m, nil
	}

	switch ks {
	case "esc":
		m.screen = screenWriting
		m.focus = focusSidebar
	case "up", "k":
		if n.sel > 0 {
			n.sel--
		}
	case "down", "j":
		if n.sel < len(n.notes)-1 {
			n.sel++
		}
	case "a":
		n.adding = true
		n.area = newNotesArea("")
		n.area.Focus()
	case "e", "enter":
		if n.sel < len(n.notes) {
			n.editing = true
			n.area = newNotesArea(n.notes[n.sel].Text)
			n.area.Focus()
		}
	case "d", "delete":
		if n.sel < len(n.notes) {
			n.confirmDelete = true
		}
	}
	return m, nil
}

// --- All-notes screen (project-wide, read-only) ------------------------------------------------

// allNotesEntry is one flattened row: a note plus the chapter/file it belongs to.
type allNotesEntry struct {
	file         string // path relative to the manuscript dir, for display grouping
	chapterTitle string
	n            note
}

type allNotesModel struct {
	entries []allNotesEntry
	sel     int
}

// enterAllNotes loads every chapter's notes across the current manuscript into one flat,
// read-only list. All I/O happens here, once, before m.screen flips — allNotesView stays I/O-free.
func (m *model) enterAllNotes() {
	dir := m.files.dir
	v := resolveManuscript(dir, readEntries(dir))
	var entries []allNotesEntry
	for _, part := range v.parts {
		for _, ch := range part.chapters {
			for _, t := range ch.texts {
				file := filepath.Join(ch.folder, t.file)
				for _, n := range loadNotes(filepath.Join(dir, file)) {
					entries = append(entries, allNotesEntry{file: file, chapterTitle: ch.title, n: n})
				}
			}
		}
	}
	m.allNotes = allNotesModel{entries: entries}
	m.screen = screenAllNotes
}

func (m model) updateAllNotes(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		if m.allNotes.sel > 0 {
			m.allNotes.sel--
		}
	case "down", "j":
		if m.allNotes.sel < len(m.allNotes.entries)-1 {
			m.allNotes.sel++
		}
	}
	return m, nil
}

func allNotesView(m model) string {
	a := m.allNotes
	header := lipgloss.NewStyle().Foreground(accent).Bold(true).Render("── toutes les notes ")

	var rows []string
	if len(a.entries) == 0 {
		rows = append(rows, lipgloss.NewStyle().Foreground(subtle).Render("  (aucune note dans ce projet)"))
	} else {
		width := max(10, min(m.width-8, 72))
		lastChapter := ""
		for i, e := range a.entries {
			if e.chapterTitle != lastChapter {
				rows = append(rows, lipgloss.NewStyle().Foreground(accent).Render(ansi.Truncate(e.chapterTitle, width, "…")))
				lastChapter = e.chapterTitle
			}
			first := e.n.Text
			if idx := strings.IndexByte(first, '\n'); idx >= 0 {
				first = first[:idx] + " …"
			}
			row := "  " + ansi.Truncate(first, width, "…")
			if i == a.sel {
				row = selectedStyle.Render("▸ " + ansi.Truncate(first, width, "…"))
			}
			rows = append(rows, row)
		}
	}
	body := header + "\n\n" + strings.Join(rows, "\n")

	var b strings.Builder
	b.WriteString(lipgloss.Place(m.width, m.height-1, lipgloss.Center, lipgloss.Center, body))
	foot := lipgloss.NewStyle().Foreground(subtle).Render("↑↓ sélectionner · esc retour · F1 aide")
	b.WriteString("\n" + lipgloss.PlaceHorizontal(m.width, lipgloss.Center, foot))
	return b.String()
}

func (m model) notesView() string {
	n := m.notes
	header := lipgloss.NewStyle().Foreground(accent).Bold(true).Render("── notes · " + filepath.Base(n.file) + " ")

	var rows []string
	if len(n.notes) == 0 {
		rows = append(rows, lipgloss.NewStyle().Foreground(subtle).Render("  (aucune note — appuyez sur a pour en ajouter une)"))
	} else {
		for i, nt := range n.notes {
			first := nt.Text
			if idx := strings.IndexByte(first, '\n'); idx >= 0 {
				first = first[:idx] + " …"
			}
			row := "  " + ansi.Truncate(first, max(10, min(m.width-8, 72)), "…")
			if i == n.sel {
				row = selectedStyle.Render("▸ " + ansi.Truncate(first, max(10, min(m.width-8, 72)), "…"))
			}
			rows = append(rows, row)
		}
	}
	body := header + "\n\n" + strings.Join(rows, "\n")

	var b strings.Builder
	b.WriteString(lipgloss.Place(m.width, m.height-1, lipgloss.Center, lipgloss.Center, body))
	if n.adding || n.editing {
		lbl := "nouvelle note"
		if n.editing {
			lbl = "modifier la note"
		}
		edit := framedPanel(lbl, n.area.View(), max(40, min(m.width-8, 72)), 5, "esc enregistrer")
		b.WriteString("\n" + lipgloss.PlaceHorizontal(m.width, lipgloss.Center, edit))
		return b.String()
	}
	if n.confirmDelete {
		bar := lipgloss.NewStyle().Foreground(accent).Render("supprimer cette note ? y supprimer · esc annuler")
		b.WriteString("\n" + lipgloss.PlaceHorizontal(m.width, lipgloss.Center, bar))
		return b.String()
	}
	foot := lipgloss.NewStyle().Foreground(subtle).Render("↑↓ sélectionner · a ajouter · e éditer · d supprimer · esc retour · F1 aide")
	b.WriteString("\n" + lipgloss.PlaceHorizontal(m.width, lipgloss.Center, foot))
	return b.String()
}
