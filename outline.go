package main

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// enterOutline opens the full-screen outline mode on the current folder's outline.md (created seeded
// if missing), remembering the file to return to on esc.
func (m *model) enterOutline() {
	m.save() // flush the current chapter before loadFile reloads over it
	outlinePath := filepath.Join(m.files.dir, "outline.md")
	if _, err := os.Stat(outlinePath); err != nil {
		if werr := atomicWrite(outlinePath, []byte("- \n"), 0o644); werr != nil {
			m.status = "impossible de créer le plan : " + werr.Error()
			return
		}
		m.files.SetDir(m.files.dir) // surface outline.md in the sidebar
	}
	m.outlineReturnFile = m.currentFile
	m.loadFile(outlinePath)
	m.editor.Dim = false // no sentence-dim in the outline
	m.screen = screenOutline
	m.focus = focusEditor
	m.editor.Focus()
}

// exitOutline saves outline.md, reflects any promoted chapters, and returns to the prior file.
func (m *model) exitOutline() {
	m.save()
	m.files.SetDir(m.files.dir) // reflect promoted chapters in the pane/manifest
	if m.outlineReturnFile != "" {
		m.loadFile(m.outlineReturnFile)
	}
	m.syncDim() // restore the writing-screen dim setting
	m.screen = screenWriting
	m.focus = focusEditor
	m.editor.Focus()
}

func (m model) updateOutline(msg tea.Msg) (tea.Model, tea.Cmd) {
	if sz, ok := msg.(tea.WindowSizeMsg); ok {
		m.width, m.height = sz.Width, sz.Height
		m.layout()
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
		m.exitOutline()
		return m, nil
	case "shift+up", "alt+up": // shift+↑ is the Terminal.app-safe move (Option isn't Meta there)
		m.moveOutlineBeat(-1)
		return m, nil
	case "shift+down", "alt+down":
		m.moveOutlineBeat(1)
		return m, nil
	case "ctrl+p", "alt+enter", "ctrl+enter": // ctrl+p = Terminal.app-safe promote (mnemonic; the
		// editor's ctrl+p prev-line is redundant with ↑ in the outline)
		m.promoteOutlineBeat()
		return m, nil
	case "enter":
		if m.editor.AtLineEnd() {
			if prefix, clear, ok := listContinuation(m.editor.CurrentLine()); ok {
				if clear {
					m.editor.ClearLine()
				} else {
					m.editor.InsertString("\n" + prefix)
				}
				m.dirty = true
				m.lastEditAt = time.Now()
				return m, nil
			}
		}
	}
	// Only mark dirty on an actual edit (mirrors the writing screen) — otherwise pure navigation keys
	// churn outline.md on every autosave tick and inflate the active-writing/pace counters.
	before := m.editor.Value()
	var cmd tea.Cmd
	m.editor, cmd = m.editor.Update(msg)
	if m.editor.Value() != before {
		m.dirty = true
		m.lastEditAt = time.Now()
	}
	return m, cmd
}

// moveOutlineBeat reorders the beat block under the cursor (dir -1 up / +1 down) and saves.
func (m *model) moveOutlineBeat(dir int) {
	out, nc, ok := moveBeat(strings.Split(m.editor.Value(), "\n"), m.editor.Line(), dir)
	if !ok {
		m.status = "déplacer : placez le curseur sur un beat"
		return
	}
	m.editor.SetValue(strings.Join(out, "\n"))
	m.editor.MoveToLine(nc)
	m.dirty = true
	m.save()
}

// promoteOutlineBeat turns the beat under the cursor into a new chapter of the current manuscript:
// it creates the (birth-stable) file, appends the manifest with the beat text as the title, seeds the
// synopsis from the beat's notes, and marks the beat done. One-way; no back-sync.
func (m *model) promoteOutlineBeat() {
	lines := strings.Split(m.editor.Value(), "\n")
	b, ok := blockAt(lines, m.editor.Line())
	if !ok {
		m.status = "promouvoir : placez le curseur sur un beat"
		return
	}
	if beatIsPromoted(lines[b.start]) {
		m.status = "déjà promu"
		return
	}
	title := beatTitle(lines[b.start])
	if title == "" {
		m.status = "promouvoir : ce beat n'a pas de titre"
		return
	}
	dir := m.files.dir
	mani, present, err := readManifest(dir)
	if err != nil {
		m.status = "promotion impossible — le manifest.json de ce manuscrit est illisible (corrompu ou version plus récente)"
		return
	}
	if !present {
		m.status = "la promotion ne fonctionne que dans un manuscrit — ceci est un simple dossier"
		return
	}
	// Two-file op (manifest + outline mark). If the manifest write lands but the [x] mark save below
	// doesn't, a re-promote appends a second same-title chapter — low-probability and non-destructive
	// (uniqueChapterFolder never overwrites); the manifest, not the mark, is the source of truth.
	taken := map[string]bool{}
	for _, it := range mani.Items {
		if it.Chapter != nil {
			taken[it.Chapter.Folder] = true
		}
		for _, mc := range it.Chapters {
			taken[mc.Folder] = true
		}
	}
	folder := uniqueChapterFolder(dir, taken)
	file := folder + ".md"
	chDir := filepath.Join(dir, folder)
	if werr := os.MkdirAll(chDir, 0o755); werr != nil {
		m.status = "échec de la promotion : " + werr.Error()
		return
	}
	if werr := atomicWrite(filepath.Join(chDir, file), []byte(""), 0o644); werr != nil {
		m.status = "échec de la promotion : " + werr.Error()
		return
	}
	mani.Items = append(mani.Items, manifestItem{Chapter: &manifestChapter{
		Folder: folder,
		Title:  title,
		Texts:  []manifestText{{File: file, Title: title}},
	}})
	if werr := writeManifest(dir, mani); werr != nil {
		m.status = "échec de la promotion : " + werr.Error()
		return
	}
	if notes := beatNotes(lines, b); len(notes) > 0 {
		syn := loadSynopses(dir)
		if syn == nil {
			syn = map[string]string{}
		}
		syn[folder] = strings.Join(notes, "\n")
		chapters := map[string]bool{}
		for _, it := range mani.Items {
			if it.Chapter != nil {
				chapters[it.Chapter.Folder] = true
			}
			for _, mc := range it.Chapters {
				chapters[mc.Folder] = true
			}
		}
		_ = saveSynopses(dir, syn, chapters)
	}
	lines[b.start] = markBeatPromoted(lines[b.start])
	m.editor.SetValue(strings.Join(lines, "\n"))
	m.dirty = true
	m.save()
	m.status = "« " + title + " » promu"
}

// markBeatPromoted rewrites a top-level beat line to a checked task item, preserving its marker.
func markBeatPromoted(line string) string {
	return string(line[0]) + " [x] " + beatTitle(line)
}

func (m model) outlineView() string {
	title := projectTitle(filepath.Base(m.files.dir))
	header := sectionHeader("PLAN · "+title, m.width)
	foot := lipgloss.NewStyle().Foreground(subtle).Render("shift/alt+↑↓ déplacer beat · ctrl+p promouvoir · esc terminé · F1 aide")
	body := lipgloss.Place(m.width, m.height-2, lipgloss.Center, lipgloss.Top, m.editor.View())
	return lipgloss.JoinVertical(lipgloss.Left, header, body,
		lipgloss.PlaceHorizontal(m.width, lipgloss.Center, foot))
}

// splitPrefix splits name into its leading run of digits and the remainder
// (everything after the digits, verbatim). "02-the-letter.md" -> ("02",
// "-the-letter.md"); "notes.md" -> ("", "notes.md"). Renumbering keeps rest
// untouched, so the title slug, separator, and extension survive losslessly.
func splitPrefix(name string) (digits, rest string) {
	i := 0
	for i < len(name) && name[i] >= '0' && name[i] <= '9' {
		i++
	}
	return name[:i], name[i:]
}

// projectTitle de-slugs a manuscript folder name for display: drop a trailing
// extension if any, turn -/_ into spaces. Unlike sectionTitle it does NOT strip a
// leading digit run ("2024-trip-journal" -> "2024 trip journal").
func projectTitle(name string) string {
	s := strings.TrimSuffix(name, filepath.Ext(name))
	s = strings.ReplaceAll(s, "-", " ")
	s = strings.ReplaceAll(s, "_", " ")
	return strings.TrimSpace(s)
}

// accentFold maps a rune outside a-z/0-9 to its unaccented ASCII equivalent when
// one exists, so a slug built from an accented French title reads as the words
// it came from ("Été" → "ete") instead of silently dropping the letter
// ("Été" → "t"). Covers the accented letters that occur in French prose;
// anything not listed here still falls through to slugify's existing
// punctuation-stripping behavior.
var accentFold = map[rune]rune{
	'à': 'a', 'â': 'a', 'ä': 'a', 'á': 'a', 'ã': 'a', 'å': 'a',
	'è': 'e', 'é': 'e', 'ê': 'e', 'ë': 'e',
	'ì': 'i', 'í': 'i', 'î': 'i', 'ï': 'i',
	'ò': 'o', 'ó': 'o', 'ô': 'o', 'ö': 'o', 'õ': 'o',
	'ù': 'u', 'ú': 'u', 'û': 'u', 'ü': 'u',
	'ý': 'y', 'ÿ': 'y',
	'ç': 'c', 'ñ': 'n',
	'œ': 'o', 'æ': 'a', // digraphs fold to their leading vowel — good enough for a slug
}

// slugify turns a typed section title into a filename slug: lowercase, accented
// letters transliterated to their unaccented ASCII form, spaces and underscores
// to hyphens, stripped of other punctuation.
func slugify(title string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(title)) {
		if folded, ok := accentFold[r]; ok {
			r = folded
		}
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '_' || r == '-':
			b.WriteByte('-')
		}
	}
	s := strings.Trim(b.String(), "-")
	if s == "" {
		s = "section"
	}
	return s
}

const outlineHeaderHeight = 2 // title line + blank spacer

// outlineRow is one selectable row: a numbered section or a loose file.

// readEntries lists dir's non-hidden document files (allowedDocExts) as fileEntry
// values (dirs excluded). Same filter as the sidebar, so the outline and the
// pane show the same files for a folder.
func readEntries(dir string) []fileEntry {
	items, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []fileEntry
	for _, it := range items {
		name := it.Name()
		if strings.HasPrefix(name, ".") || it.IsDir() {
			continue
		}
		if !allowedDocExts[strings.ToLower(filepath.Ext(name))] {
			continue
		}
		out = append(out, fileEntry{name: name})
	}
	return out
}
