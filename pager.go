package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// pagerLine is one display row of the manuscript pager: text already wrapped to
// the measure width, plus the source it maps back to. Header (chapter-rule) lines
// carry their section file with src = -1.
type pagerLine struct {
	text     string
	file     string // section base name ("" only if there are no sections)
	src      int    // 0-based source line within file; -1 for a header line
	header   bool
	cumWords int // running word count from the manuscript start through this line
}

// pagerModel is the full-screen read-through pager: a manual scroller that owns
// the cursor line and the line→source map. Built once by load.
type pagerModel struct {
	dir    string
	lines  []pagerLine
	total  int
	cursor int
	offset int
	width  int
	height int
}

// load concatenates the dir's chapters (in resolver order, loose excluded) into
// wrapped, mapped lines. Each chapter contributes a "── Title ──" header line then
// its body, each source line wrapped to width. Reads each file exactly once.
func (p *pagerModel) load(dir string, width int) {
	if width < 1 {
		width = 1
	}
	entries := readEntries(dir) // markdown/text files, dirs excluded (from outline.go)
	v := resolveManuscript(dir, entries)

	p.dir = dir
	p.lines = nil
	p.cursor = 0
	p.offset = 0

	running := 0
	for _, part := range v.parts {
		if part.title != "" {
			p.lines = append(p.lines, pagerLine{
				text:     "── " + part.title + " ──",
				file:     "",
				src:      -1,
				header:   true,
				cumWords: running,
			})
		}
		for _, ch := range part.chapters {
			if len(ch.texts) == 0 {
				continue // empty chapter: header with no body would panic on ch.texts[0]
			}
			file := filepath.Join(ch.folder, ch.texts[0].file)
			p.lines = append(p.lines, pagerLine{
				text:     "── " + ch.title + " ──",
				file:     file,
				src:      -1,
				header:   true,
				cumWords: running,
			})
			data, err := os.ReadFile(filepath.Join(dir, ch.folder, ch.texts[0].file))
			if err != nil {
				continue
			}
			body := strings.TrimSuffix(string(data), "\n")
			for srcIdx, srcLine := range strings.Split(body, "\n") {
				for _, row := range strings.Split(ansi.Wrap(srcLine, width, ""), "\n") {
					running += wordCount(row)
					p.lines = append(p.lines, pagerLine{
						text:     row,
						file:     file,
						src:      srcIdx,
						header:   false,
						cumWords: running,
					})
				}
			}
		}
	}
	p.total = running
}

const pagerHeaderHeight = 2 // the running-count header line + a blank spacer

// moveCursor moves the cursor by d lines (clamped) and scrolls to keep it visible.
func (p *pagerModel) moveCursor(d int) {
	if len(p.lines) == 0 {
		return
	}
	p.cursor += d
	if p.cursor < 0 {
		p.cursor = 0
	}
	if p.cursor >= len(p.lines) {
		p.cursor = len(p.lines) - 1
	}
	p.ensureVisible()
}

// page moves the cursor by d full screens.
func (p *pagerModel) page(d int) { p.moveCursor(d * p.height) }

// ensureVisible scrolls offset so the cursor sits within the visible window.
func (p *pagerModel) ensureVisible() {
	if p.cursor < p.offset {
		p.offset = p.cursor
	}
	if p.height > 0 && p.cursor >= p.offset+p.height {
		p.offset = p.cursor - p.height + 1
	}
	if p.offset < 0 {
		p.offset = 0
	}
}

// jumpTarget resolves the cursor line to the (file, source line) to open. A header
// line opens its section at line 0. ok is false when there's nothing to open.
func (p pagerModel) jumpTarget() (file string, src int, ok bool) {
	if p.cursor < 0 || p.cursor >= len(p.lines) {
		return "", 0, false
	}
	l := p.lines[p.cursor]
	if l.file == "" {
		return "", 0, false
	}
	if l.header {
		return l.file, 0, true
	}
	return l.file, l.src, true
}

// dimMarkdown colours markdown punctuation (#, *, _, `) subtle without changing the
// line's width, so the prose reads cleaner while the line→source map stays exact.
func dimMarkdown(line string) string {
	dim := lipgloss.NewStyle().Foreground(subtle)
	var b strings.Builder
	for _, r := range line {
		switch r {
		case '#', '*', '_', '`':
			b.WriteString(dim.Render(string(r)))
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// View renders the header line plus the visible window of lines (O(height)).
func (p pagerModel) View() string {
	cum := 0
	if p.cursor >= 0 && p.cursor < len(p.lines) {
		cum = p.lines[p.cursor].cumWords
	}
	head := fmt.Sprintf("%s · %s / %sw", projectTitle(filepath.Base(p.dir)), commafy(cum), commafy(p.total))
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Foreground(accent).Render(ansi.Truncate(head, p.width, "…")))
	b.WriteString(strings.Repeat("\n", pagerHeaderHeight)) // header line + spacer rows

	end := p.offset + p.height
	if end > len(p.lines) {
		end = len(p.lines)
	}
	for i := p.offset; i < end; i++ {
		l := p.lines[i]
		var row string
		switch {
		case i == p.cursor:
			row = selectedStyle.Width(p.width).Render(ansi.Truncate(l.text, p.width, "…"))
		case l.header:
			row = lipgloss.NewStyle().Foreground(accent).Render(ansi.Truncate(l.text, p.width, "…"))
		default:
			row = dimMarkdown(l.text)
		}
		b.WriteString(row)
		if i < end-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}
