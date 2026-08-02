package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"okashi/internal/textarea"
)

// searchHit is one match: the file path, a display name (path relative to the search root),
// the 0-based source line, the RUNE column of the match, and the raw matched line.
type searchHit struct {
	file, name string
	line, col  int
	context    string
}

// searchText finds case-insensitive substring matches of query in text (one document),
// emitting a hit per occurrence (multiple per line) up to limit.
func searchText(name, path, text, query string, limit int) []searchHit {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" || limit <= 0 {
		return nil
	}
	var hits []searchHit
	for li, line := range strings.Split(text, "\n") {
		low := strings.ToLower(line)
		from := 0
		for from <= len(low) {
			idx := strings.Index(low[from:], q)
			if idx < 0 {
				break
			}
			byteCol := from + idx
			hits = append(hits, searchHit{
				file: path, name: name, line: li,
				// Rune index is invariant under ToLower, but byte length is NOT (Ⱥ→ⱥ grows
				// 2→3 bytes), so derive the column from the folded string, never `line`.
				col:     len([]rune(low[:byteCol])),
				context: strings.TrimSpace(line),
			})
			if len(hits) >= limit {
				return hits
			}
			from = byteCol + len(q)
		}
	}
	return hits
}

// searchProject walks root for document files (allowed extensions, no hidden) and collects
// matches, capped at limit. The display name is the path relative to root.
func searchProject(root string, allowed map[string]bool, query string, limit int) []searchHit {
	if strings.TrimSpace(query) == "" || limit <= 0 {
		return nil
	}
	var hits []searchHit
	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if path != root && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") || !allowed[strings.ToLower(filepath.Ext(d.Name()))] {
			return nil
		}
		data, e := os.ReadFile(path)
		if e != nil {
			return nil
		}
		name := strings.TrimPrefix(strings.TrimPrefix(path, root), string(filepath.Separator))
		for _, h := range searchText(name, path, string(data), query, limit-len(hits)) {
			hits = append(hits, h)
		}
		if len(hits) >= limit {
			return filepath.SkipAll
		}
		return nil
	})
	return hits
}

// searchAllSources searches every reachable library source's root, tagging each hit's display name
// with its source name so results read "Dropbox/notes.md:4". Capped at limit across all sources.
func searchAllSources(sources []source, allowed map[string]bool, query string, limit int) []searchHit {
	if strings.TrimSpace(query) == "" || limit <= 0 {
		return nil
	}
	var hits []searchHit
	for _, s := range sources {
		if !s.reachable() {
			continue
		}
		for _, h := range searchProject(s.root(), allowed, query, limit-len(hits)) {
			h.name = s.Name + "/" + h.name
			hits = append(hits, h)
		}
		if len(hits) >= limit {
			break
		}
	}
	return hits
}

// --- model wiring ---

var searchHitStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#282a36")).Background(lipgloss.Color("#f1fa8c")) // dark-on-yellow

func newSearchInput() textinput.Model {
	ti := textinput.New()
	ti.Placeholder = "rechercher…"
	ti.Prompt = ""
	ti.CharLimit = 120
	return ti
}

// recomputeSearch refreshes the hit list for the current query + scope.
func (m *model) recomputeSearch() {
	q := m.searchInput.Value()
	switch m.searchScope {
	case scopeDocument:
		name := filepath.Base(m.currentFile)
		if name == "." || name == "" {
			name = "ce document"
		}
		m.searchHits = searchText(name, m.currentFile, m.editor.Value(), q, searchLimit)
	case scopeAll:
		m.searchHits = searchAllSources(m.sources, m.files.allowed, q, searchLimit)
	default: // scopeProject
		m.searchHits = searchProject(m.files.root, m.files.allowed, q, searchLimit)
	}
	m.searchSel = 0
	m.searchOffset = 0
}

// searchDecorator highlights case-insensitive occurrences of query on a line (rune offsets).
func searchDecorator(line, query string) []textarea.Decoration {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil
	}
	low := strings.ToLower(line)
	var d []textarea.Decoration
	for from := 0; from <= len(low); {
		idx := strings.Index(low[from:], q)
		if idx < 0 {
			break
		}
		bc := from + idx
		// Rune indices from the folded string (ToLower preserves rune count, not bytes).
		rs := len([]rune(low[:bc]))
		re := rs + len([]rune(low[bc:bc+len(q)]))
		d = append(d, textarea.Decoration{Start: rs, End: re, Style: searchHitStyle})
		from = bc + len(q)
	}
	return d
}

// searchJump opens the selected hit's file at its line and returns to writing.
func (m *model) searchJump() {
	if m.searchSel < 0 || m.searchSel >= len(m.searchHits) {
		return
	}
	h := m.searchHits[m.searchSel]
	if h.file != m.currentFile {
		m.loadFile(h.file)
	}
	m.editor.MoveToLine(h.line)
	m.editor.SetCursor(h.col)
	m.searchHighlight = m.searchInput.Value()
	m.screen = screenWriting
	m.focus = focusEditor
	m.editor.Focus()
	m.applyDecorator()
	m.layout()
}

func (m *model) searchMove(d int) {
	if len(m.searchHits) == 0 {
		return
	}
	m.searchSel += d
	if m.searchSel < 0 {
		m.searchSel = 0
	}
	if m.searchSel >= len(m.searchHits) {
		m.searchSel = len(m.searchHits) - 1
	}
	vis := m.searchVisibleRows()
	if m.searchSel < m.searchOffset {
		m.searchOffset = m.searchSel
	} else if m.searchSel >= m.searchOffset+vis {
		m.searchOffset = m.searchSel - vis + 1
	}
}

func (m model) searchVisibleRows() int {
	h := m.height - 5 // input, two rules, footer
	if h < 1 {
		h = 1
	}
	return h
}

// updateSearch handles input on the search screen.
func (m model) updateSearch(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tea.MouseMsg:
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			m.searchMove(-1)
		case tea.MouseButtonWheelDown:
			m.searchMove(1)
		}
		return m, nil
	case tea.KeyMsg:
		// Replace mode: typing the replacement for the current query; enter replaces all in the doc.
		if m.replaceMode {
			switch msg.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "esc":
				m.replaceMode = false
				m.replaceInput.Blur()
				m.searchInput.Focus()
				return m, nil
			case "enter":
				m.replaceAllInDoc()
				return m, nil
			}
			var cmd tea.Cmd
			m.replaceInput, cmd = m.replaceInput.Update(msg)
			return m, cmd
		}
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			m.screen = m.searchReturn
			if m.screen == screenSearch {
				m.screen = screenWriting
			}
			return m, nil
		case "ctrl+r":
			// Enter replace mode for the current query (replaces in the current chapter only).
			if strings.TrimSpace(m.searchInput.Value()) != "" {
				m.replaceMode = true
				m.searchInput.Blur()
				m.replaceInput.SetValue("")
				m.replaceInput.Focus()
			}
			return m, nil
		case "enter":
			m.searchJump()
			return m, nil
		case "ctrl+a":
			m.searchScope = scopeAll
			m.recomputeSearch()
			return m, nil
		case "tab":
			if m.searchScope == scopeProject {
				m.searchScope = scopeDocument
			} else {
				m.searchScope = scopeProject
			}
			m.recomputeSearch()
			return m, nil
		case "up":
			m.searchMove(-1)
			return m, nil
		case "down":
			m.searchMove(1)
			return m, nil
		}
		var cmd tea.Cmd
		m.searchInput, cmd = m.searchInput.Update(msg)
		m.recomputeSearch()
		return m, cmd
	}
	return m, nil
}

// replaceAllInDoc replaces every literal occurrence of the current query with the replacement text
// in the OPEN document (current chapter only), then closes search. checkpointUndo makes it undoable.
func (m *model) replaceAllInDoc() {
	q := m.searchInput.Value()
	r := m.replaceInput.Value()
	m.replaceMode = false
	m.replaceInput.Blur()
	m.screen = m.searchReturn
	if m.screen == screenSearch {
		m.screen = screenWriting
	}
	if q == "" {
		return
	}
	out, status, changed := replaceStatus(m.editor.Value(), q, r)
	if changed {
		m.checkpointUndo()
		m.editor.SetValue(out)
		m.dirty = true
	}
	m.status = status
}

// replaceStatus computes the literal (case-sensitive) replace-all of q→r in text, plus a status line
// that is honest about the mismatch with search: search is case-insensitive, so it can highlight
// matches this literal replace won't touch. The status flags those case-variants rather than silently
// leaving them (a surprise on a destructive action). Replace stays literal on purpose — a
// case-insensitive replace would clobber capitalization (turning "The" into "a").
func replaceStatus(text, q, r string) (out, status string, changed bool) {
	if q == "" {
		return text, "", false
	}
	n := strings.Count(text, q)
	ci := countFold(text, q)
	if n == 0 {
		if ci > 0 {
			return text, "aucune correspondance exacte (casse) pour « " + q + " » (la recherche ignore la casse ; le remplacement en tient compte)", false
		}
		return text, "aucune correspondance pour « " + q + " »", false
	}
	out = strings.ReplaceAll(text, q, r)
	status = fmt.Sprintf("%d remplacement(s) × « %s » → « %s »", n, q, r)
	if ci > n {
		status += fmt.Sprintf(" · %d variante(s) de casse restante(s) (le remplacement respecte la casse)", ci-n)
	}
	return out, status, true
}

// countFold counts case-insensitive, non-overlapping occurrences of sub in s.
func countFold(s, sub string) int {
	if sub == "" {
		return 0
	}
	return strings.Count(strings.ToLower(s), strings.ToLower(sub))
}

// searchView renders the search screen.
func (m model) searchView() string {
	width := m.width
	scope := "Projet"
	if m.searchScope == scopeDocument {
		scope = "Ce document"
	} else if m.searchScope == scopeAll {
		scope = "Toutes les sources"
	}
	head := "Recherche ▸ " + m.searchInput.View()
	right := lipgloss.NewStyle().Foreground(accent).Render(scope) + lipgloss.NewStyle().Foreground(subtle).Render("  (Tab)")
	gap := width - lipgloss.Width(head) - lipgloss.Width(scope+"  (Tab)")
	if gap < 1 {
		gap = 1
	}
	rule := lipgloss.NewStyle().Foreground(subtle).Render(strings.Repeat("─", max(1, width)))
	var b strings.Builder
	b.WriteString(head + strings.Repeat(" ", gap) + right + "\n")
	b.WriteString(rule + "\n")
	if m.replaceMode {
		b.WriteString("Remplacer ▸ " + m.replaceInput.View() + "\n")
		b.WriteString(rule + "\n")
	}

	vis := m.searchVisibleRows()
	end := m.searchOffset + vis
	if end > len(m.searchHits) {
		end = len(m.searchHits)
	}
	q := m.searchInput.Value()
	for i := m.searchOffset; i < end; i++ {
		h := m.searchHits[i]
		loc := fmt.Sprintf("%s:%d", h.name, h.line+1)
		locW := 24
		loc = ansi.Truncate(loc, locW, "…")
		loc = loc + strings.Repeat(" ", max(0, locW-lipgloss.Width(loc)))
		ctx := highlightQuery(h.context, q, width-locW-3)
		row := loc + "  " + ctx
		if i == m.searchSel {
			row = selectedStyle.Render(ansi.Truncate(ansi.Strip(loc+"  "+h.context), width, "…"))
		}
		b.WriteString(row + "\n")
	}
	for i := end - m.searchOffset; i < vis; i++ {
		b.WriteString("\n")
	}
	b.WriteString(rule + "\n")
	files := map[string]bool{}
	for _, h := range m.searchHits {
		files[h.file] = true
	}
	note := fmt.Sprintf("%d résultats dans %d fichiers", len(m.searchHits), len(files))
	if len(m.searchHits) >= searchLimit {
		note += " (limité)"
	}
	if q == "" {
		note = "tapez pour rechercher"
	} else if len(m.searchHits) == 0 {
		note = "(aucun résultat)"
	}
	footText := note + " · ↑↓ sélectionner · ⏎ ouvrir · Tab portée · ctrl+r remplacer · esc retour · F1 aide"
	if m.replaceMode {
		footText = "⏎ remplacer tout dans ce chapitre · esc annuler · F1 aide"
	}
	foot := lipgloss.NewStyle().Foreground(subtle).Render(footText)
	b.WriteString(foot)
	return b.String()
}

// highlightQuery returns context with case-insensitive occurrences of q emphasized,
// truncated to w display columns.
func highlightQuery(context, q string, w int) string {
	context = ansi.Truncate(context, max(1, w), "…")
	if strings.TrimSpace(q) == "" {
		return context
	}
	low := strings.ToLower(context)
	lq := strings.ToLower(strings.TrimSpace(q))
	cr := []rune(context) // emit ORIGINAL-case text, sliced by rune index
	var b strings.Builder
	prev := 0
	for from := 0; ; {
		idx := strings.Index(low[from:], lq)
		if idx < 0 {
			break
		}
		bc := from + idx
		// Byte offsets in `low` → rune indices (invariant under ToLower); slice `cr` by rune.
		start := len([]rune(low[:bc]))
		end := start + len([]rune(low[bc:bc+len(lq)]))
		b.WriteString(string(cr[prev:start]))
		b.WriteString(searchHitStyle.Render(string(cr[start:end])))
		prev = end
		from = bc + len(lq)
	}
	b.WriteString(string(cr[prev:]))
	return b.String()
}

// wordUnderCursorOrEmpty seeds the search box with the word under the cursor, if any.
func (m model) wordUnderCursorOrEmpty() string {
	if w, _, _, ok := m.wordUnderCursor(); ok {
		return w
	}
	return ""
}
