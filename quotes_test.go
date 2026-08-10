package main

import (
	"testing"
	"time"
)

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

func TestQuoteIndexForTodayNoConfigDirFallsBackToYearDay(t *testing.T) {
	today := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC) // day 222 of 2026
	idx, needsSave := quoteIndexForToday(userConfig{}, today, false, 38)
	want := today.YearDay() % 38
	if idx != want {
		t.Fatalf("index = %d, want %d", idx, want)
	}
	if needsSave {
		t.Fatal("no-config-dir path must never request a save")
	}
}

func TestQuoteIndexForTodayFirstLaunchEmptyStartsAtZero(t *testing.T) {
	today := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	idx, needsSave := quoteIndexForToday(userConfig{}, today, true, 38)
	if idx != 0 {
		t.Fatalf("index = %d, want 0 (day 1 of the cycle)", idx)
	}
	if !needsSave {
		t.Fatal("empty FirstLaunch must request a save")
	}
}

func TestQuoteIndexForTodayFirstLaunchCorruptStartsAtZero(t *testing.T) {
	today := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	idx, needsSave := quoteIndexForToday(userConfig{FirstLaunch: "not-a-date"}, today, true, 38)
	if idx != 0 || !needsSave {
		t.Fatalf("corrupt FirstLaunch: index=%d needsSave=%v, want 0/true", idx, needsSave)
	}
}

func TestQuoteIndexForTodayAdvancesWithDays(t *testing.T) {
	first := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	today := time.Date(2026, 8, 6, 0, 0, 0, 0, time.UTC) // 5 days later
	idx, needsSave := quoteIndexForToday(userConfig{FirstLaunch: first.Format("2006-01-02")}, today, true, 38)
	if idx != 5 {
		t.Fatalf("index = %d, want 5", idx)
	}
	if needsSave {
		t.Fatal("an already-recorded FirstLaunch must never request a re-save")
	}
}

func TestQuoteIndexForTodayWrapsAfterCycle(t *testing.T) {
	first := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	today := first.AddDate(0, 0, 38) // exactly one full cycle later
	idx, _ := quoteIndexForToday(userConfig{FirstLaunch: first.Format("2006-01-02")}, today, true, 38)
	if idx != 0 {
		t.Fatalf("index = %d, want 0 (wrapped)", idx)
	}
}

func TestQuoteIndexForTodayFirstLaunchInFuture(t *testing.T) {
	first := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	today := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC) // before FirstLaunch — clock skew
	idx, needsSave := quoteIndexForToday(userConfig{FirstLaunch: first.Format("2006-01-02")}, today, true, 38)
	if idx < 0 || idx >= 38 {
		t.Fatalf("index = %d out of range [0,38) for a future FirstLaunch", idx)
	}
	if needsSave {
		t.Fatal("a parseable (if skewed) FirstLaunch must never request a re-save")
	}
}
