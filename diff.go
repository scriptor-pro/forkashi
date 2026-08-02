package main

import (
	"regexp"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

var (
	diffDelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))            // red
	diffAddStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))            // green
	diffDelHi    = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true) // changed words
	diffAddHi    = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
)

// diffOp classifies a line in an edit script.
type diffOp int

const (
	opEqual diffOp = iota
	opDel
	opAdd
)

// diffLine is one line of a diff: an op and the line text.
type diffLine struct {
	op   diffOp
	text string
}

// diffLines computes a line-level shortest edit script turning a into b, via the Myers O(ND)
// algorithm (pure Go, no dependency). The returned ops, read top to bottom, are the diff:
// opEqual/opAdd lines reconstruct b; opEqual/opDel lines reconstruct a.
func diffLines(a, b []string) []diffLine {
	if len(a) == 0 && len(b) == 0 {
		return nil
	}
	trace, endX, endY := myersTrace(a, b)
	return myersBacktrack(a, b, trace, endX, endY)
}

// myersTrace runs the greedy Myers forward pass, snapshotting the frontier V at the start of each
// depth so the backtrack can reconstruct the path.
func myersTrace(a, b []string) (trace [][]int, endX, endY int) {
	n, m := len(a), len(b)
	max := n + m
	offset := max
	v := make([]int, 2*max+1)
	for d := 0; d <= max; d++ {
		snap := make([]int, len(v))
		copy(snap, v)
		trace = append(trace, snap)
		for k := -d; k <= d; k += 2 {
			var x int
			if k == -d || (k != d && v[offset+k-1] < v[offset+k+1]) {
				x = v[offset+k+1] // move down: an insertion from b
			} else {
				x = v[offset+k-1] + 1 // move right: a deletion from a
			}
			y := x - k
			for x < n && y < m && a[x] == b[y] {
				x++
				y++
			}
			v[offset+k] = x
			if x >= n && y >= m {
				return trace, n, m
			}
		}
	}
	return trace, n, m
}

// myersBacktrack walks the trace from the end to the origin, emitting ops in forward order.
func myersBacktrack(a, b []string, trace [][]int, x, y int) []diffLine {
	max := len(a) + len(b)
	offset := max
	var ops []diffLine
	emit := func(op diffOp, text string) { ops = append(ops, diffLine{op, text}) }

	for d := len(trace) - 1; d >= 0; d-- {
		v := trace[d]
		k := x - y
		var prevK int
		if k == -d || (k != d && v[offset+k-1] < v[offset+k+1]) {
			prevK = k + 1
		} else {
			prevK = k - 1
		}
		prevX := v[offset+prevK]
		prevY := prevX - prevK
		for x > prevX && y > prevY { // diagonal: equal lines
			emit(opEqual, a[x-1])
			x--
			y--
		}
		if d > 0 {
			if x == prevX {
				emit(opAdd, b[y-1]) // came from a down move
			} else {
				emit(opDel, a[x-1]) // came from a right move
			}
		}
		x, y = prevX, prevY
	}
	// ops were emitted from the end backward; reverse to forward order.
	for i, j := 0, len(ops)-1; i < j; i, j = i+1, j-1 {
		ops[i], ops[j] = ops[j], ops[i]
	}
	return ops
}

// wordRun is a span of a line for intra-line highlighting: changed marks it as inserted/deleted.
type wordRun struct {
	text    string
	changed bool
}

var wordTokenSplit = regexp.MustCompile(`\S+|\s+`)

// tokenizeWords splits s into alternating word / whitespace tokens, so joining them reconstructs s.
func tokenizeWords(s string) []string {
	return wordTokenSplit.FindAllString(s, -1)
}

// diffWords computes token-level runs for a changed line pair, so only the words that actually
// differ are highlighted (prose that changed a few words doesn't flash as a whole red/green block).
func diffWords(a, b string) (aRuns, bRuns []wordRun) {
	for _, op := range diffLines(tokenizeWords(a), tokenizeWords(b)) {
		switch op.op {
		case opEqual:
			aRuns = append(aRuns, wordRun{op.text, false})
			bRuns = append(bRuns, wordRun{op.text, false})
		case opDel:
			aRuns = append(aRuns, wordRun{op.text, true})
		case opAdd:
			bRuns = append(bRuns, wordRun{op.text, true})
		}
	}
	return aRuns, bRuns
}

// diffModel backs the diff screen: a rendered edit script with intra-line word highlights,
// windowed for O(visible) rendering.
type diffModel struct {
	aLabel, bLabel string
	lines          []diffLine
	wordRuns       map[int][]wordRun // line index → highlighted runs, for paired del/add lines
	offset         int
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimRight(s, "\n"), "\n")
}

// newDiffModel diffs two contents and precomputes word-level highlights for changed line pairs.
func newDiffModel(aLabel, aContent, bLabel, bContent string) diffModel {
	ops := diffLines(splitLines(aContent), splitLines(bContent))
	wr := map[int][]wordRun{}
	// Pair a run of deletions immediately followed by an equal-length run of additions; when the
	// paired lines are similar prose, highlight only the words that differ.
	i := 0
	for i < len(ops) {
		if ops[i].op != opDel {
			i++
			continue
		}
		ds := i
		for i < len(ops) && ops[i].op == opDel {
			i++
		}
		de := i
		as := i
		for i < len(ops) && ops[i].op == opAdd {
			i++
		}
		ae := i
		if de-ds == ae-as {
			for j := 0; j < de-ds; j++ {
				delIdx, addIdx := ds+j, as+j
				if similarLine(ops[delIdx].text, ops[addIdx].text) {
					aR, bR := diffWords(ops[delIdx].text, ops[addIdx].text)
					wr[delIdx] = aR
					wr[addIdx] = bR
				}
			}
		}
	}
	return diffModel{aLabel: aLabel, bLabel: bLabel, lines: ops, wordRuns: wr}
}

// similarLine reports whether two lines share enough words to be worth word-level highlighting.
func similarLine(a, b string) bool {
	at, bt := strings.Fields(a), strings.Fields(b)
	if len(at) == 0 || len(bt) == 0 {
		return false
	}
	set := map[string]bool{}
	for _, w := range at {
		set[w] = true
	}
	common := 0
	for _, w := range bt {
		if set[w] {
			common++
		}
	}
	minLen := len(at)
	if len(bt) < minLen {
		minLen = len(bt)
	}
	return float64(common)/float64(minLen) >= 0.3
}

// jumpChange returns the offset of the next/prev changed line relative to the current top.
func (d diffModel) jumpChange(offset, dir, maxOff int) int {
	if dir > 0 {
		for i := offset + 1; i < len(d.lines); i++ {
			if d.lines[i].op != opEqual {
				return min(i, maxOff)
			}
		}
		return offset
	}
	for i := offset - 1; i >= 0; i-- {
		if d.lines[i].op != opEqual {
			return i
		}
	}
	return offset
}

func renderDiffLine(dl diffLine, runs []wordRun) string {
	var gutter string
	var base, hi lipgloss.Style
	switch dl.op {
	case opDel:
		gutter, base, hi = "-", diffDelStyle, diffDelHi
	case opAdd:
		gutter, base, hi = "+", diffAddStyle, diffAddHi
	default:
		gutter, base = " ", lipgloss.NewStyle().Foreground(subtle)
	}
	var body string
	if runs != nil {
		var b strings.Builder
		for _, r := range runs {
			if r.changed {
				b.WriteString(hi.Render(r.text))
			} else {
				b.WriteString(base.Render(r.text))
			}
		}
		body = b.String()
	} else {
		body = base.Render(dl.text)
	}
	return base.Render(gutter+" ") + body
}

func (m model) diffView() string {
	d := m.diff
	header := lipgloss.NewStyle().Foreground(accent).Bold(true).Render("── diff · " + d.aLabel + " → " + d.bLabel + " ")
	h := m.height - 4
	if h < 1 {
		h = 1
	}
	end := d.offset + h
	if end > len(d.lines) {
		end = len(d.lines)
	}
	var rows []string
	for i := d.offset; i < end; i++ {
		rows = append(rows, ansi.Truncate(renderDiffLine(d.lines[i], d.wordRuns[i]), max(4, m.width-1), "…"))
	}
	if len(d.lines) == 0 {
		rows = append(rows, lipgloss.NewStyle().Foreground(subtle).Render("  (fichiers identiques)"))
	}
	for len(rows) < h {
		rows = append(rows, "")
	}
	foot := lipgloss.NewStyle().Foreground(subtle).Render("↑↓ défiler · n/N modif suiv/préc · esc retour")
	return header + "\n\n" + strings.Join(rows, "\n") + "\n" + foot
}

func (m model) updateDiff(msg tea.Msg) (tea.Model, tea.Cmd) {
	if sz, ok := msg.(tea.WindowSizeMsg); ok {
		m.width, m.height = sz.Width, sz.Height
		return m, nil
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	d := &m.diff
	h := m.height - 4
	if h < 1 {
		h = 1
	}
	maxOff := len(d.lines) - h
	if maxOff < 0 {
		maxOff = 0
	}
	switch key.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "d":
		m.screen = screenSnapshots
	case "up", "k":
		if d.offset > 0 {
			d.offset--
		}
	case "down", "j":
		if d.offset < maxOff {
			d.offset++
		}
	case "pgup":
		if d.offset -= h; d.offset < 0 {
			d.offset = 0
		}
	case "pgdown", " ":
		if d.offset += h; d.offset > maxOff {
			d.offset = maxOff
		}
	case "n":
		d.offset = d.jumpChange(d.offset, 1, maxOff)
	case "N":
		d.offset = d.jumpChange(d.offset, -1, maxOff)
	}
	return m, nil
}
