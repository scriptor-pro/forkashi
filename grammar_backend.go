package main

import (
	"encoding/json"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// grammarStyle is the shared decoration style for grammar findings (green underline,
// distinct from the red spellcheck underline), used both by the editor decorator and
// the inspector's toggle label.
var grammarStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#50fa7b")).Underline(true)

// grammarFinding is one issue located within a source line (RUNE offsets within that
// line), produced by an optional on-device "deep grammar" backend (Tier 2). Rune
// offsets let findings slot straight into the editor's per-line Decorator + click-to-fix.
type grammarFinding struct {
	Line, Start, End int
	Message          string
	Replacements     []string
}

// grammarChecker is an optional deep-grammar backend. In forkashi the default (and only
// wired-in) implementation is grammalecteBackend, talking to a locally running Grammalecte
// HTTP server (see grammar_grammalecte.go). It is genuinely optional: initialModel() probes
// Available() once at startup and downgrades to nil when no server is reachable, so a nil
// m.grammarChecker means "no backend usable right now" and every call site that gates on
// `m.grammarChecker != nil` degrades silently instead of failing at click time.
type grammarChecker interface {
	Name() string                                // "Grammalecte" | "Apple Intelligence" | ...
	Available() bool                             // runtime availability
	Check(text string) ([]grammarFinding, error) // whole-document → per-line findings
}

// newGrammarChecker is the constructor the model calls at startup. It is a package var
// so tests can inject a fake.
//
// This fork deliberately wires in grammalecteChecker (French grammar via Grammalecte) as
// the ONE default backend, bypassing the Tier 2 macOS backend (appleGrammarChecker in
// grammar_apple_darwin.go / grammar_apple_stub.go) entirely — even on a `darwin && cgo &&
// applegrammar` build, since this var is untagged and always wins. That's intentional: this
// fork's purpose is French-language support, and NSSpellChecker/Apple Intelligence don't
// give useful French grammar checking, so Grammalecte is the right default everywhere
// rather than only off-macOS. See docs/superpowers/specs/2026-08-01-forkashi-v1-design.md
// §3. The Apple backend and its build tag are left in place (unused, but still compiling
// and independently tested) rather than deleted, in case a future revert wants it back.
var newGrammarChecker = grammalecteChecker

// fmIssue is one issue as reported by the Foundation Models bridge (the wrong substring
// verbatim + a correction); JSON-decoded from the Swift side.
type fmIssue struct {
	Wrong  string `json:"wrong"`
	Fix    string `json:"fix"`
	Reason string `json:"reason"`
}

// fmFindings parses the Foundation Models JSON ({"issues":[...]}) and locates each issue
// in text. Pure-Go (no cgo) so it is unit-testable in the default build.
func fmFindings(jsonStr, text string) ([]grammarFinding, error) {
	var parsed struct {
		Issues []fmIssue `json:"issues"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		return nil, err
	}
	return locateFMFindings(text, parsed.Issues), nil
}

// locateFMFindings maps model-reported (wrong, fix) pairs to per-line rune-range findings.
// The model returns the wrong substring, not an offset, so each is located in document
// order, claiming successive occurrences (a word repeated on a line maps to distinct
// spans). Matching is case-INSENSITIVE because the model frequently changes the case of
// `wrong`; no-op echoes (wrong==fix) are skipped. LIMITATION: a substring occurring before
// the flagged instance, or reordered issues, can mislocate; re-running refines it.
func locateFMFindings(text string, issues []fmIssue) []grammarFinding {
	lines := strings.Split(text, "\n")
	var findings []grammarFinding
	claimedFrom := map[int]int{} // line index → byte offset to resume searching from
	for _, is := range issues {
		if is.Wrong == "" || strings.EqualFold(strings.TrimSpace(is.Wrong), strings.TrimSpace(is.Fix)) {
			continue
		}
		for li, ln := range lines {
			from := claimedFrom[li]
			if from > len(ln) {
				continue
			}
			hay, needle := strings.ToLower(ln), strings.ToLower(is.Wrong)
			if len(hay) != len(ln) { // ToLower changed byte length → match exactly instead
				hay, needle = ln, is.Wrong
			}
			rel := strings.Index(hay[from:], needle)
			if rel < 0 {
				continue
			}
			idx := from + rel
			sc := len([]rune(ln[:idx]))
			ec := sc + len([]rune(is.Wrong))
			findings = append(findings, grammarFinding{
				Line: li, Start: sc, End: ec,
				Message:      is.Reason,
				Replacements: []string{is.Fix},
			})
			claimedFrom[li] = idx + len(is.Wrong)
			break
		}
	}
	return findings
}

// utf16ToRune converts a UTF-16 code-unit offset into a rune offset in s. NSSpellChecker
// reports ranges in UTF-16 code units; the editor works in runes.
func utf16ToRune(s string, u16 int) int {
	units, runes := 0, 0
	for _, r := range s {
		if units >= u16 {
			break
		}
		if r > 0xFFFF {
			units += 2
		} else {
			units++
		}
		runes++
	}
	return runes
}

// runeOffsetToLine maps an absolute rune offset (newlines counting as one rune) to a
// (lineIndex, runeColumn) pair within lines.
func runeOffsetToLine(lines []string, abs int) (int, int) {
	for i, ln := range lines {
		n := len([]rune(ln))
		if abs <= n {
			return i, abs
		}
		abs -= n + 1 // the '\n'
	}
	last := len(lines) - 1
	if last < 0 {
		return 0, 0
	}
	return last, len([]rune(lines[last]))
}
