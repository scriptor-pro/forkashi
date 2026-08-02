package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// projectGoals is one project's goal state, stored in the global goals.json
// keyed by project dir path.
type projectGoals struct {
	DailyGoal       int            `json:"dailyGoal"`
	ProjectGoal     int            `json:"projectGoal"`
	SessionGoalMin  int            `json:"sessionGoalMin"`
	SprintMin       int            `json:"sprintMin"`
	DayBaseline     int            `json:"dayBaseline"`
	Day             string         `json:"day"`
	ActiveSecsToday int            `json:"activeSecsToday"`
	TimeDay         string         `json:"timeDay"`
	Deadline        string         `json:"deadline,omitempty"` // "2006-01-02" target date for ProjectGoal
	History         map[string]int `json:"history,omitempty"`  // date "2006-01-02" → net words that day
}

func goalsPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "okashi", "goals.json")
}

func loadGoals(path string) map[string]projectGoals {
	m := map[string]projectGoals{}
	if path == "" {
		return m
	}
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &m)
	}
	return m
}

func saveGoals(path string, m map[string]projectGoals) {
	if path == "" {
		return
	}
	data, err := json.Marshal(m)
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	_ = atomicWrite(path, data, 0o644)
}

func envGoal(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			return n
		}
	}
	return def
}

// applyEnvDefaults fills zero goals from env (OKASHI_DAILY_GOAL=500, OKASHI_PROJECT_GOAL=0).
func (pg projectGoals) applyEnvDefaults() projectGoals {
	if pg.DailyGoal == 0 {
		pg.DailyGoal = envGoal("OKASHI_DAILY_GOAL", 500)
	}
	if pg.ProjectGoal == 0 {
		pg.ProjectGoal = envGoal("OKASHI_PROJECT_GOAL", 0)
	}
	if pg.SprintMin == 0 {
		pg.SprintMin = 25
	}
	return pg
}

func today() string { return time.Now().Format("2006-01-02") }

// rolloverActive resets the daily active-time accumulator when the date changed.
func rolloverActive(pg projectGoals, today string) (projectGoals, bool) {
	if pg.TimeDay != today {
		pg.TimeDay = today
		pg.ActiveSecsToday = 0
		return pg, true
	}
	return pg, false
}

// rolloverIfNeeded resets the daily baseline when the date changed; changed=true
// means the caller should persist.
func rolloverIfNeeded(pg projectGoals, total int, today string) (projectGoals, bool) {
	if pg.Day != today {
		pg.Day = today
		pg.DayBaseline = total
		return pg, true
	}
	return pg, false
}

// todayWords is net words written today (clamped at 0).
func todayWords(pg projectGoals, total int) int {
	if d := total - pg.DayBaseline; d > 0 {
		return d
	}
	return 0
}

// recordToday stores today's net word count in the history. changed=true when the stored value
// moved, so the caller can decide whether to persist. Past days are never touched (only today's
// entry updates), so they freeze at their final value once the date rolls over.
func recordToday(pg projectGoals, total int, today string) (projectGoals, bool) {
	words := todayWords(pg, total)
	if pg.History == nil {
		pg.History = map[string]int{}
	}
	if pg.History[today] == words {
		return pg, false
	}
	pg.History[today] = words
	return pg, true
}

// recordDay is the tick-safe history recorder: it rolls the word baseline for today (if the date
// changed or the baseline is stale) BEFORE recording, then records today's net words. DayBaseline is
// otherwise maintained only by syncGoal on keystrokes, so a tick firing in a project/day not yet
// keyed would record a garbage delta (e.g. the whole manuscript after a project switch, or
// yesterday's delta just after midnight). Rolling here first makes such a first-record a correct 0.
func recordDay(pg projectGoals, total int, today string) (projectGoals, bool) {
	pg, rolled := rolloverIfNeeded(pg, total, today)
	pg, recorded := recordToday(pg, total, today)
	return pg, rolled || recorded
}

// paceLine describes the pace needed to hit ProjectGoal by Deadline given the current project word
// count. ok=false means there's nothing to show (no deadline or no project goal set).
func paceLine(pg projectGoals, projectWords int, today string) (string, bool) {
	if pg.Deadline == "" || pg.ProjectGoal <= 0 {
		return "", false
	}
	due, err := time.Parse("2006-01-02", pg.Deadline)
	if err != nil {
		return "", false
	}
	if projectWords >= pg.ProjectGoal {
		return "✓ objectif atteint", true
	}
	now, _ := time.Parse("2006-01-02", today)
	daysLeft := int(due.Sub(now).Hours() / 24)
	remaining := pg.ProjectGoal - projectWords
	if daysLeft < 0 {
		return "date limite dépassée · " + commafy(remaining) + " restants", true
	}
	if daysLeft == 0 {
		return "échéance aujourd'hui · " + commafy(remaining) + " restants", true
	}
	perDay := (remaining + daysLeft - 1) / daysLeft // ceil
	return "≈" + commafy(perDay) + "/jour pour atteindre " + commafy(pg.ProjectGoal) + " avant le " + pg.Deadline + " (" + strconv.Itoa(daysLeft) + "j)", true
}

// recentHistory returns the last n days' word counts (oldest first) ending at today, for a sparkline.
func recentHistory(history map[string]int, today string, n int) []int {
	d, err := time.Parse("2006-01-02", today)
	if err != nil || n <= 0 {
		return nil
	}
	out := make([]int, n)
	for i := 0; i < n; i++ {
		out[i] = history[d.AddDate(0, 0, -(n-1-i)).Format("2006-01-02")]
	}
	return out
}

// streak counts consecutive days up to today with words written (>0).
func streak(history map[string]int, today string) int {
	if len(history) == 0 {
		return 0
	}
	d, err := time.Parse("2006-01-02", today)
	if err != nil {
		return 0
	}
	n := 0
	for {
		key := d.Format("2006-01-02")
		if history[key] > 0 {
			n++
		} else if key != today {
			break // a prior day with no words ends the streak
		}
		// today with 0 words neither counts nor breaks — you still have the day
		d = d.AddDate(0, 0, -1)
	}
	return n
}
