package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGrammalecteBackendCheck(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/gc_text/fr" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		resp := grammalecteResponse{
			Data: []grammalecteParagraph{{
				IParagraph: 1,
				LGrammarErrors: []grammalecteGrammar{
					{NStart: 5, NEnd: 9, SMessage: "devrait être au subjonctif", ASuggestions: []string{"fasse"}},
				},
			}},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	host, port, _ := strings.Cut(strings.TrimPrefix(srv.URL, "http://"), ":")
	t.Setenv("OKASHI_GRAMMALECTE_HOST", host)
	t.Setenv("OKASHI_GRAMMALECTE_PORT", port)

	backend := grammalecteChecker()
	findings, err := backend.Check("Il faut que")
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d: %+v", len(findings), findings)
	}
	if findings[0].Line != 0 || findings[0].Start != 5 || findings[0].End != 9 {
		t.Fatalf("unexpected location: %+v", findings[0])
	}
	if len(findings[0].Replacements) != 1 || findings[0].Replacements[0] != "fasse" {
		t.Fatalf("unexpected replacements: %+v", findings[0].Replacements)
	}
}

func TestGrammalecteBackendUnavailable(t *testing.T) {
	t.Setenv("OKASHI_GRAMMALECTE_HOST", "localhost")
	t.Setenv("OKASHI_GRAMMALECTE_PORT", "1") // nothing listens on port 1
	backend := grammalecteChecker()
	if backend.Available() {
		t.Fatal("Available() should be false when nothing is listening")
	}
	if _, err := backend.Check("some text"); err == nil {
		t.Fatal("Check() should return an error when the server is unreachable, not panic or silently succeed")
	}
}

func TestSplitParagraphs(t *testing.T) {
	spans := splitParagraphs("Ligne un.\nLigne deux.\n\nDeuxième paragraphe.")
	if len(spans) != 2 {
		t.Fatalf("expected 2 paragraphs, got %d: %+v", len(spans), spans)
	}
	if spans[0].startLine != 0 {
		t.Fatalf("first paragraph should start at line 0, got %d", spans[0].startLine)
	}
	if spans[1].startLine != 3 {
		t.Fatalf("second paragraph should start at line 3 (after blank line 2), got %d", spans[1].startLine)
	}
}

func TestRelocate(t *testing.T) {
	line, start, end := relocate("Ligne un.\nLigne deux.", 5, 10, 15)
	if line != 6 { // startLine(5) + 1 (second line of the paragraph)
		t.Fatalf("expected line 6, got %d", line)
	}
	if start != 0 || end != 5 { // "Ligne un.\n" is 10 runes; offset 10 is the start of "Ligne deux."
		t.Fatalf("expected start=0 end=5, got start=%d end=%d", start, end)
	}
}
