package main

import "testing"

func TestSpellOK(t *testing.T) {
	for _, w := range []string{"sauter", "reconnecté", "aujourd'hui", "café", "château", "où"} {
		if !spellOK(w) {
			t.Errorf("spellOK(%q) = false, want true", w)
		}
	}
	for _, w := range []string{"tè", "kikc", "brilig"} {
		if spellOK(w) {
			t.Errorf("spellOK(%q) = true, want false", w)
		}
	}
}

func TestSpellSuggest(t *testing.T) {
	got := spellSuggest("bnjour", 5)
	found := false
	for _, s := range got {
		if s == "bonjour" {
			found = true
		}
	}
	if !found {
		t.Fatalf("spellSuggest(bnjour) should include \"bonjour\", got %v", got)
	}
}

func TestSpellSuggestMemoized(t *testing.T) {
	a := spellSuggest("tè", 4)
	b := spellSuggest("tè", 4)
	if len(a) == 0 || len(a) != len(b) {
		t.Fatalf("memoized spellSuggest should return a stable non-empty list: %v vs %v", a, b)
	}
	if len(suggestCache) == 0 {
		t.Fatal("spellSuggest should populate suggestCache")
	}
}

func TestSpellDecoratorEngine(t *testing.T) {
	decos := spellDecorator("Le renard saute mais le tè chien njn attention")
	if len(decos) != 2 {
		t.Fatalf("expected exactly 2 flags (tè, njn), got %d: %+v", len(decos), decos)
	}
	if d := spellDecorator("NASA a envoyé 3 fusées"); len(d) != 0 {
		t.Fatalf("all-caps/digit tokens should be skipped, got %+v", d)
	}
}
