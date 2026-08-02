package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGoalsRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "goals.json")
	in := map[string]projectGoals{"/proj": {DailyGoal: 500, ProjectGoal: 80000, DayBaseline: 1200, Day: "2026-06-29"}}
	saveGoals(p, in)
	out := loadGoals(p)
	// projectGoals contains a map now, so compare the scalar fields explicitly.
	if g := out["/proj"]; g.DailyGoal != 500 || g.ProjectGoal != 80000 || g.DayBaseline != 1200 || g.Day != "2026-06-29" {
		t.Fatalf("round-trip = %+v, want the input fields", g)
	}
	if len(loadGoals(filepath.Join(t.TempDir(), "missing.json"))) != 0 {
		t.Fatal("missing file should load an empty map")
	}
}

func TestApplyEnvDefaults(t *testing.T) {
	t.Setenv("OKASHI_DAILY_GOAL", "750")
	t.Setenv("OKASHI_PROJECT_GOAL", "")
	pg := projectGoals{}.applyEnvDefaults()
	if pg.DailyGoal != 750 {
		t.Fatalf("daily default = %d, want 750", pg.DailyGoal)
	}
	if pg.ProjectGoal != 0 {
		t.Fatalf("project default = %d, want 0", pg.ProjectGoal)
	}
	// A non-zero stored value is kept.
	if got := (projectGoals{DailyGoal: 300}).applyEnvDefaults().DailyGoal; got != 300 {
		t.Fatalf("stored daily kept = %d, want 300", got)
	}
}

func TestRolloverAndTodayWords(t *testing.T) {
	pg := projectGoals{Day: "2026-06-28", DayBaseline: 100}
	pg2, changed := rolloverIfNeeded(pg, 450, "2026-06-29")
	if !changed || pg2.Day != "2026-06-29" || pg2.DayBaseline != 450 {
		t.Fatalf("rollover = %+v changed=%v, want day reset + baseline 450", pg2, changed)
	}
	if _, changed := rolloverIfNeeded(pg2, 500, "2026-06-29"); changed {
		t.Fatal("same day should not change")
	}
	if w := todayWords(pg2, 470); w != 20 {
		t.Fatalf("todayWords = %d, want 20", w)
	}
	if w := todayWords(pg2, 400); w != 0 {
		t.Fatalf("todayWords below baseline = %d, want 0 (clamped)", w)
	}
}

func TestGoalsPathUnderConfig(t *testing.T) {
	if p := goalsPath(); p != "" && filepath.Base(p) != "goals.json" {
		t.Fatalf("goalsPath base = %q, want goals.json", filepath.Base(p))
	}
	_ = os.Getenv // keep os imported
}

func TestSessionGoalRoundTrips(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "goals.json")
	saveGoals(path, map[string]projectGoals{"/p": {DailyGoal: 500, SessionGoalMin: 30}})
	got := loadGoals(path)
	if got["/p"].SessionGoalMin != 30 {
		t.Fatalf("SessionGoalMin round-trip = %d, want 30", got["/p"].SessionGoalMin)
	}
}

func TestRolloverActive(t *testing.T) {
	pg := projectGoals{TimeDay: "2026-07-01", ActiveSecsToday: 900}
	got, changed := rolloverActive(pg, "2026-07-02")
	if !changed || got.ActiveSecsToday != 0 || got.TimeDay != "2026-07-02" {
		t.Fatalf("date change should reset: %+v changed=%v", got, changed)
	}
	same, changed2 := rolloverActive(got, "2026-07-02")
	if changed2 || same.ActiveSecsToday != 0 {
		t.Fatalf("same day should be a no-op: %+v changed=%v", same, changed2)
	}
}

func TestActiveSecsTodayRoundTrips(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "goals.json")
	saveGoals(path, map[string]projectGoals{"/p": {ActiveSecsToday: 1234, TimeDay: "2026-07-02"}})
	got := loadGoals(path)
	if got["/p"].ActiveSecsToday != 1234 || got["/p"].TimeDay != "2026-07-02" {
		t.Fatalf("round-trip lost active time: %+v", got["/p"])
	}
}

func TestRecordTodayFreezesPastDays(t *testing.T) {
	pg := projectGoals{DayBaseline: 100, Day: "2026-07-04"}
	// Simulate an earlier day already recorded.
	pg.History = map[string]int{"2026-07-03": 500}

	pg, changed := recordToday(pg, 100+250, "2026-07-04") // 250 net words today
	if !changed || pg.History["2026-07-04"] != 250 {
		t.Fatalf("today should record 250 (changed=%v history=%v)", changed, pg.History)
	}
	if pg.History["2026-07-03"] != 500 {
		t.Fatal("a past day's entry must not be touched")
	}
	// Re-recording the same value is a no-op.
	if _, changed := recordToday(pg, 350, "2026-07-04"); changed {
		t.Fatal("re-recording the same count should report no change")
	}
}

func TestProjectGoalsHistoryRoundTripAndBackCompat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "goals.json")
	m := map[string]projectGoals{
		"/proj": {ProjectGoal: 80000, Deadline: "2026-12-01", History: map[string]int{"2026-07-04": 250}},
	}
	saveGoals(path, m)
	got := loadGoals(path)
	if got["/proj"].History["2026-07-04"] != 250 || got["/proj"].Deadline != "2026-12-01" {
		t.Fatalf("round-trip lost data: %+v", got["/proj"])
	}
	// An old goals.json with no history/deadline keys loads fine.
	if err := os.WriteFile(path, []byte(`{"/old":{"dailyGoal":500}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	old := loadGoals(path)
	if old["/old"].DailyGoal != 500 || old["/old"].History != nil {
		t.Fatalf("back-compat load failed: %+v", old["/old"])
	}
}

func TestPaceLine(t *testing.T) {
	base := projectGoals{ProjectGoal: 80000, Deadline: "2026-08-01"}
	// 10 days out, 20000 to go → 2000/day.
	if s, ok := paceLine(base, 60000, "2026-07-22"); !ok || s != "≈2,000/jour pour atteindre 80,000 avant le 2026-08-01 (10j)" {
		t.Fatalf("normal pace: %q ok=%v", s, ok)
	}
	if s, ok := paceLine(base, 80000, "2026-07-22"); !ok || s != "✓ objectif atteint" {
		t.Fatalf("met: %q", s)
	}
	if s, ok := paceLine(base, 60000, "2026-09-01"); !ok || !contains(s, "date limite dépassée") {
		t.Fatalf("past: %q", s)
	}
	if _, ok := paceLine(projectGoals{ProjectGoal: 80000}, 10, "2026-07-22"); ok {
		t.Fatal("no deadline → nothing to show")
	}
}

func TestStreak(t *testing.T) {
	h := map[string]int{"2026-07-04": 0, "2026-07-03": 200, "2026-07-02": 150, "2026-06-30": 100}
	// today (04) is 0 but doesn't break; 03 and 02 count; 01 is a gap → stop.
	if n := streak(h, "2026-07-04"); n != 2 {
		t.Fatalf("streak = %d, want 2", n)
	}
	// today has words → counts too.
	h["2026-07-04"] = 300
	if n := streak(h, "2026-07-04"); n != 3 {
		t.Fatalf("streak with today = %d, want 3", n)
	}
	if n := streak(map[string]int{}, "2026-07-04"); n != 0 {
		t.Fatalf("empty history streak = %d, want 0", n)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}

// A tick recording history in a project/day not yet keyed must roll the baseline first, so it
// records 0 for today rather than backfilling the whole word count (project-switch / midnight bug).
func TestRecordDayRollsStaleBaseline(t *testing.T) {
	pg := projectGoals{Day: "2026-07-03", DayBaseline: 0} // stale: last seen a different day
	pg, changed := recordDay(pg, 5000, "2026-07-04")      // total 5000, nothing written today yet
	if pg.History["2026-07-04"] != 0 {
		t.Fatalf("stale-baseline tick recorded %d, want 0", pg.History["2026-07-04"])
	}
	if !changed || pg.Day != "2026-07-04" || pg.DayBaseline != 5000 {
		t.Fatalf("recordDay should roll the baseline: %+v (changed=%v)", pg, changed)
	}
	// A subsequent same-day record with real progress counts correctly.
	pg, _ = recordDay(pg, 5250, "2026-07-04")
	if pg.History["2026-07-04"] != 250 {
		t.Fatalf("same-day progress = %d, want 250", pg.History["2026-07-04"])
	}
}
