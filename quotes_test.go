package main

import "testing"

func TestLoadQuotesCount(t *testing.T) {
	qs := loadQuotes()
	if len(qs) != 38 {
		t.Fatalf("want 38 quotes, got %d", len(qs))
	}
}

func TestLoadQuotesFirstAndLast(t *testing.T) {
	qs := loadQuotes()
	if qs[0].Text != "Écrire, c'est une façon de parler sans être interrompu." || qs[0].Author != "Jules Renard" {
		t.Fatalf("first quote mismatch: %+v", qs[0])
	}
	last := qs[len(qs)-1]
	if last.Author != "Voltaire" || last.Text != "L'écriture est la peinture de la voix." {
		t.Fatalf("last quote mismatch: %+v", last)
	}
}

func TestLoadQuotesAllNonEmpty(t *testing.T) {
	for i, q := range loadQuotes() {
		if q.Text == "" {
			t.Fatalf("quote %d has empty text", i)
		}
	}
}
