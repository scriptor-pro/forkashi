package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// grammalecteBackend is a grammarChecker that calls a locally running Grammalecte
// HTTP server (https://www.grammalecte.net/), started and stopped manually by the
// user (v1 — see docs/superpowers/specs/2026-08-01-forkashi-v1-design.md §3).
type grammalecteBackend struct {
	baseURL string
	client  *http.Client
}

func grammalecteHost() string {
	if h := os.Getenv("OKASHI_GRAMMALECTE_HOST"); h != "" {
		return h
	}
	return "localhost"
}

func grammalectePort() string {
	if p := os.Getenv("OKASHI_GRAMMALECTE_PORT"); p != "" {
		return p
	}
	return "8080"
}

// grammalecteChecker is the constructor wired into newGrammarChecker.
func grammalecteChecker() grammarChecker {
	return &grammalecteBackend{
		baseURL: fmt.Sprintf("http://%s:%s", grammalecteHost(), grammalectePort()),
		client:  &http.Client{Timeout: 5 * time.Second},
	}
}

func (g *grammalecteBackend) Name() string { return "Grammalecte" }

// Available does a lightweight reachability check against the server root.
// Best-effort: any error (server not running, wrong port) means unavailable —
// okashi must run fine without French grammar checking rather than crash.
func (g *grammalecteBackend) Available() bool {
	resp, err := g.client.Get(g.baseURL + "/")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode < 500
}

type grammalecteResponse struct {
	Error string                  `json:"error"`
	Data  []grammalecteParagraph  `json:"data"`
}

type grammalecteParagraph struct {
	IParagraph      int                  `json:"iParagraph"`
	LGrammarErrors  []grammalecteGrammar `json:"lGrammarErrors"`
	LSpellingErrors []grammalecteSpell   `json:"lSpellingErrors"`
}

type grammalecteGrammar struct {
	NStart       int      `json:"nStart"`
	NEnd         int      `json:"nEnd"`
	SMessage     string   `json:"sMessage"`
	ASuggestions []string `json:"aSuggestions"`
	SRuleId      string   `json:"sRuleId"`
}

type grammalecteSpell struct {
	NStart int    `json:"nStart"`
	NEnd   int    `json:"nEnd"`
	SValue string `json:"sValue"`
}

// Check sends the whole document to the server and maps paragraph-relative rune
// offsets back to (line, column) pairs. Grammalecte splits on blank lines into
// paragraphs; okashi's editor works line-by-line, so each paragraph's findings are
// relocated to the correct source line by tracking how many lines precede it.
func (g *grammalecteBackend) Check(text string) ([]grammarFinding, error) {
	form := url.Values{"text": {text}}
	resp, err := g.client.PostForm(g.baseURL+"/gc_text/fr", form)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var parsed grammalecteResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("grammalecte: invalid JSON response: %w", err)
	}
	if parsed.Error != "" {
		return nil, fmt.Errorf("grammalecte: %s", parsed.Error)
	}

	paragraphs := splitParagraphs(text)
	var findings []grammarFinding
	for _, p := range parsed.Data {
		idx := p.IParagraph - 1
		if idx < 0 || idx >= len(paragraphs) {
			continue
		}
		line, colOffset := paragraphs[idx].startLine, 0
		_ = colOffset
		for _, ge := range p.LGrammarErrors {
			l, s, e := relocate(paragraphs[idx].text, paragraphs[idx].startLine, ge.NStart, ge.NEnd)
			findings = append(findings, grammarFinding{
				Line: l, Start: s, End: e,
				Message:      ge.SMessage,
				Replacements: ge.ASuggestions,
			})
		}
		_ = line
	}
	return findings, nil
}

type paragraphSpan struct {
	text      string
	startLine int
}

// splitParagraphs mirrors Grammalecte's own paragraph splitting (blank-line
// separated) and records each paragraph's first line index (0-based) in the
// original text, so Check can map paragraph-relative offsets back to document lines.
func splitParagraphs(text string) []paragraphSpan {
	lines := strings.Split(text, "\n")
	var spans []paragraphSpan
	var cur []string
	curStart := 0
	flush := func(endLine int) {
		if len(cur) == 0 {
			return
		}
		spans = append(spans, paragraphSpan{text: strings.Join(cur, "\n"), startLine: curStart})
		cur = nil
	}
	for i, ln := range lines {
		if strings.TrimSpace(ln) == "" {
			flush(i)
			curStart = i + 1
			continue
		}
		if len(cur) == 0 {
			curStart = i
		}
		cur = append(cur, ln)
	}
	flush(len(lines))
	return spans
}

// relocate converts a paragraph-relative rune range [nStart,nEnd) into an
// absolute (line, startCol, endCol) triple within the paragraph's lines.
// Multi-line-spanning findings are clamped to their starting line — okashi's
// decorator model is per-line, and Grammalecte findings rarely span lines.
func relocate(paragraphText string, startLine, nStart, nEnd int) (line, start, end int) {
	lines := strings.Split(paragraphText, "\n")
	pos := 0
	for i, ln := range lines {
		lnRunes := len([]rune(ln))
		if nStart <= pos+lnRunes {
			line = startLine + i
			start = nStart - pos
			end = nEnd - pos
			if end > lnRunes {
				end = lnRunes
			}
			if start < 0 {
				start = 0
			}
			return
		}
		pos += lnRunes + 1 // +1 for the '\n'
	}
	last := len(lines) - 1
	if last < 0 {
		return startLine, 0, 0
	}
	return startLine + last, 0, len([]rune(lines[last]))
}
