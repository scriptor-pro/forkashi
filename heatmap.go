package main

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// heatmapModel backs the writing-history screen: a git-contributions-style grid of a project's
// per-day word counts.
type heatmapModel struct {
	title   string
	history map[string]int
	today   string
}

func newHeatmapModel(title string, history map[string]int, today string) heatmapModel {
	return heatmapModel{title: title, history: history, today: today}
}

// enterHeatmap opens the writing-history screen for the current project.
func (m *model) enterHeatmap() {
	pg := m.goalsAll[m.files.dir]
	m.heatmap = newHeatmapModel(m.files.paneLabel(), pg.History, today())
	m.screen = screenHeatmap
}

func (m model) updateHeatmap(msg tea.Msg) (tea.Model, tea.Cmd) {
	if sz, ok := msg.(tea.WindowSizeMsg); ok {
		m.width, m.height = sz.Width, sz.Height
		return m, nil
	}
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc", "g", "q":
			m.screen = screenWriting
			m.focus = focusSidebar
		}
	}
	return m, nil
}

var sparkRunes = []rune("▁▂▃▄▅▆▇█")

// sparkline renders values as unicode bars scaled to the window max; a zero day is a subtle dot.
func sparkline(vals []int) string {
	mx := 0
	for _, v := range vals {
		if v > mx {
			mx = v
		}
	}
	var b strings.Builder
	for _, v := range vals {
		if v <= 0 || mx == 0 {
			b.WriteString(lipgloss.NewStyle().Foreground(subtle).Render("·"))
			continue
		}
		idx := (v*(len(sparkRunes)-1) + mx - 1) / mx
		if idx >= len(sparkRunes) {
			idx = len(sparkRunes) - 1
		}
		b.WriteString(lipgloss.NewStyle().Foreground(accent).Render(string(sparkRunes[idx])))
	}
	return b.String()
}

// heatLevels are the 5 intensity styles for a contributions cell (empty → max).
var heatLevels = []lipgloss.Style{
	lipgloss.NewStyle().Foreground(subtle),               // 0 — no writing
	lipgloss.NewStyle().Foreground(lipgloss.Color("22")), // low
	lipgloss.NewStyle().Foreground(lipgloss.Color("28")), // ...
	lipgloss.NewStyle().Foreground(lipgloss.Color("34")), //
	lipgloss.NewStyle().Foreground(lipgloss.Color("40")), // high
}

// heatBucket maps a day's word count to an intensity level 0..4.
func heatBucket(v, max int) int {
	if v <= 0 || max <= 0 {
		return 0
	}
	lvl := (v*4 + max - 1) / max // ceil to 1..4
	if lvl > 4 {
		lvl = 4
	}
	return lvl
}

func heatCell(v, max int) string {
	lvl := heatBucket(v, max)
	if lvl == 0 {
		return heatLevels[0].Render("·")
	}
	return heatLevels[lvl].Render("■")
}

func (m model) heatmapView() string {
	h := m.heatmap
	header := lipgloss.NewStyle().Foreground(accent).Bold(true).Render("── writing history · " + h.title + " ")

	today, err := time.Parse("2006-01-02", h.today)
	if err != nil {
		return header + "\n\n  (no history yet)"
	}
	mx, total := 0, 0
	var mxDay string
	for day, v := range h.history {
		if v > mx {
			mx = v
			mxDay = day
		}
		total += v
	}

	// Columns = weeks, rows = Sun..Sat; the rightmost column is the current week.
	availCols := m.width - 6
	if availCols < 1 {
		availCols = 1
	}
	weeks := availCols
	if weeks > 53 {
		weeks = 53
	}
	thisSunday := today.AddDate(0, 0, -int(today.Weekday()))
	firstSunday := thisSunday.AddDate(0, 0, -(weeks-1)*7)

	rowLabels := []string{" ", "M", " ", "W", " ", "F", " "} // Sun..Sat, label alternate rows
	var rows []string
	for r := 0; r < 7; r++ {
		line := " " + rowLabels[r] + " "
		for c := 0; c < weeks; c++ {
			day := firstSunday.AddDate(0, 0, c*7+r)
			if day.After(today) {
				line += " "
				continue
			}
			line += heatCell(h.history[day.Format("2006-01-02")], mx)
		}
		rows = append(rows, line)
	}

	legend := "   moins " + heatCell(0, 1) + heatLevels[1].Render("■") + heatLevels[2].Render("■") +
		heatLevels[3].Render("■") + heatLevels[4].Render("■") + " plus"
	bestDay := ""
	if mxDay != "" {
		if d, err := time.Parse("2006-01-02", mxDay); err == nil {
			bestDay = " (" + frenchLongDate(d) + ")"
		}
	}
	stats := fmt.Sprintf("   série de %d jours · meilleur jour : %s%s · mots déjà écrits : %s",
		streak(h.history, h.today), commafy(mx), bestDay, commafy(total))

	body := header + "\n\n" + strings.Join(rows, "\n") + "\n\n" +
		lipgloss.NewStyle().Foreground(subtle).Render(legend) + "\n" +
		lipgloss.NewStyle().Foreground(subtle).Render(stats)
	foot := lipgloss.NewStyle().Foreground(subtle).Render("esc / g / q  retour")
	return lipgloss.Place(m.width, m.height-1, lipgloss.Center, lipgloss.Center, body) + "\n" +
		lipgloss.PlaceHorizontal(m.width, lipgloss.Center, foot)
}

var frenchWeekdays = [...]string{"dimanche", "lundi", "mardi", "mercredi", "jeudi", "vendredi", "samedi"}

var frenchMonths = [...]string{"janvier", "février", "mars", "avril", "mai", "juin",
	"juillet", "août", "septembre", "octobre", "novembre", "décembre"}

// frenchLongDate formats d as "jeudi 12 mai 2023" (Go's time package has no built-in French locale).
func frenchLongDate(d time.Time) string {
	return fmt.Sprintf("%s %d %s %d", frenchWeekdays[d.Weekday()], d.Day(), frenchMonths[d.Month()-1], d.Year())
}
