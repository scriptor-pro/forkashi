package main

import (
	"fmt"
	"math"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// sectionHeader renders an UPPERCASE accent label followed by a subtle rule to width.
func sectionHeader(label string, width int) string {
	up := strings.ToUpper(label)
	hs := lipgloss.NewStyle().Foreground(accent).Bold(true)
	fill := width - lipgloss.Width(up) - 1
	if fill < 0 {
		fill = 0
	}
	return hs.Render(up) + " " + lipgloss.NewStyle().Foreground(subtle).Render(strings.Repeat("─", fill))
}

// framedPanel wraps inner (multi-line) in a rounded box of the given total width/height,
// with title injected into the top border. Inner lines are padded/truncated ansi-aware.
// action, when non-empty, is rendered right-aligned in the top border before ╮.
func framedPanel(title, inner string, width, height int, action string) string {
	return framedPanelBg(title, inner, width, height, action, nil)
}

// framedPanelBg is framedPanel with an optional background color (bg == nil means no
// background, i.e. identical output to framedPanel) applied across the whole panel —
// border and content alike — used to show which pane currently has keyboard focus.
func framedPanelBg(title, inner string, width, height int, action string, bg lipgloss.TerminalColor) string {
	if width < 6 {
		width = 6
	}
	if height < 2 {
		height = 2
	}
	bs := lipgloss.NewStyle().Foreground(subtle)
	ts := lipgloss.NewStyle().Foreground(accent).Bold(true)
	if bg != nil {
		bs = bs.Background(bg)
		ts = ts.Background(bg)
	}
	contentW := width - 4 // │ <space> content <space> │

	rightSeg := ""
	if action != "" {
		rightSeg = " " + action
	}
	titleStr := title
	maxTitle := width - 4 - lipgloss.Width(rightSeg) // leave room for ╭╮, the title spaces, and the action
	if lipgloss.Width(titleStr) > maxTitle {
		titleStr = ansi.Truncate(titleStr, maxTitle, "")
	}
	fill := width - 2 - (lipgloss.Width(titleStr) + 2) - lipgloss.Width(rightSeg) // minus ╭╮, minus the two spaces around the title, minus action
	if fill < 0 {
		fill = 0
	}
	top := bs.Render("╭") + ts.Render(" "+titleStr+" ") + bs.Render(strings.Repeat("─", fill)) + ts.Render(rightSeg) + bs.Render("╮")

	// Pad to contentW; truncate FIRST (ansi-aware) so an over-long line never
	// wraps and breaks the frame.
	cell := lipgloss.NewStyle().Width(contentW)
	pad := " "
	if bg != nil {
		cell = cell.Background(bg)
		pad = lipgloss.NewStyle().Background(bg).Render(" ")
	}
	lines := strings.Split(inner, "\n")
	out := make([]string, 0, height)
	out = append(out, top)
	for r := 0; r < height-2; r++ {
		c := ""
		if r < len(lines) {
			c = ansi.Truncate(lines[r], contentW, "")
		}
		out = append(out, bs.Render("│")+pad+cell.Render(c)+pad+bs.Render("│"))
	}
	out = append(out, bs.Render("╰"+strings.Repeat("─", width-2)+"╯"))
	return strings.Join(out, "\n")
}

// inspectorInnerWidth returns the true inner content width of the inspector
// panel — the value main.go must pass to View() and the click handlers must
// use. The panel is framed at exactly inspectorWidth so render == reservation
// == offset; framedPanel uses 2 borders + 2 padding cols = 4, so inner = -4.
func inspectorInnerWidth() int { return inspectorWidth - 4 }

type inspectorTab int

const (
	tabWords inspectorTab = iota
	tabOutline
	tabGoals
	tabAnalysis
)

// inspectorTabLabels is the single source of the tab set — used by both the tab
// bar render and cycle() so they never diverge.
func inspectorTabLabels() []string { return []string{"Mots", "Plan", "Objectifs", "Analyse"} }

// inspectorModel is the read-only right-side panel: a tab bar + the active tab.
type inspectorModel struct {
	visible bool
	tab     inspectorTab

	// grammarBackend is the Name() of the active grammarChecker, or "" if none.
	// grammarChecking is true while an async grammar pass is in flight.
	// grammarAutoRecheck mirrors model.autoRecheck. All set in the model's View().
	grammarBackend     string
	grammarChecking    bool
	grammarAutoRecheck bool
}

// cycle advances the inspector: hidden → Words → Outline → … → hidden.
func (in *inspectorModel) cycle() {
	if !in.visible {
		in.visible = true
		in.tab = tabWords
		return
	}
	in.tab++
	if int(in.tab) >= len(inspectorTabLabels()) {
		in.visible = false
		in.tab = tabWords
	}
}

// renderOutline shows the outline read-only: top-level bullets in accent, nested
// lines plain, each truncated to width. Empty → a hint.
func renderOutline(text string, width int) string {
	if strings.TrimSpace(text) == "" {
		return lipgloss.NewStyle().Foreground(subtle).Render("(vide — ctrl+l pour éditer)")
	}
	var b strings.Builder
	for i, line := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		if i > 0 {
			b.WriteByte('\n')
		}
		shown := ansi.Truncate(line, width, "…")
		if len(line) > 0 && line[0] != ' ' && line[0] != '\t' {
			b.WriteString(lipgloss.NewStyle().Foreground(accent).Render(shown))
		} else {
			b.WriteString(shown)
		}
	}
	return b.String()
}

// noteMaxLines caps the word-wrapped height of a single note in renderNotesSection, so one
// long note can't push the rest of the inspector tab off the bottom of the (unscrolled) panel.
const noteMaxLines = 6

// renderNotesSection shows the revision notes attached to the current file, read-only: a
// "NOTES" header, then either a subtle empty-state hint or each note's full text, word-wrapped
// to width and clamped to noteMaxLines (with a trailing "…" if a note runs longer), separated
// by a blank line.
func renderNotesSection(notes []note, width int) string {
	var b strings.Builder
	b.WriteString(sectionHeader("Notes", width))
	if len(notes) == 0 {
		b.WriteString("\n" + lipgloss.NewStyle().Foreground(subtle).Render("(aucune note — n pour en ajouter)"))
		return b.String()
	}
	for i, nt := range notes {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString("\n  " + strings.ReplaceAll(wrapClamp(nt.Text, width-2, noteMaxLines), "\n", "\n  "))
	}
	return b.String()
}

type docStats struct {
	words, chars, paragraphs int
	readSecs                 int        // estimated reading time at 210 wpm (French silent-reading estimate)
	sentMean, sentStdDev     float64    // sentence length in words (mean ± population stddev)
	readabilityScore         float64    // Kandel-Moles score, clamped to [0, 100]; 0 when doc has no sentences
	readabilityLabel         string     // 7-tier label for readabilityScore ("" when no sentences)
	overused                 []wordFreq // top repeated content words
}

// wordFreq is one word and how often it appears — for the overused-word list.
type wordFreq struct {
	word string
	n    int
}

type projStats struct {
	words, chapters int
	manuscript      bool
}

type goalStats struct {
	today, dailyGoal, project, projectGoal, sessionSecs, sessionGoalMin, todayActiveSecs int
	idle                                                                                 bool
	pace                                                                                 string // deadline burndown line ("" = none)
	streakDays                                                                           int    // consecutive writing days
	spark                                                                                []int  // recent daily word counts for the sparkline
}

type analysisState struct{ spell, grammar bool }

// analysisRowY returns the inspector body row (y from the very top of the
// inspector, row 0 = tab bar) for each Analysis checkbox:
//
//	0 → Spellcheck  (tab-bar(0) + blank(1) + header(2) + blank(3) = 4)
//	1 → Grammar     (5)
func analysisRowY(i int) int {
	return [2]int{4, 5}[i]
}

func inspectorAnalysisRowAtY(localY int) (int, bool) {
	for i := 0; i < 2; i++ {
		if analysisRowY(i) == localY {
			return i, true
		}
	}
	return 0, false
}

func checkbox(on bool) string {
	if on {
		return "[x] "
	}
	return "[ ] "
}

// progressBar renders a width-cell bar: filled proportion in accent █, rest ░.
func progressBar(cur, goal, width int) string {
	filled := 0
	if goal > 0 {
		filled = (cur * width) / goal
		if filled > width {
			filled = width
		}
		if filled < 0 {
			filled = 0
		}
	}
	bar := lipgloss.NewStyle().Foreground(accent).Render(strings.Repeat("█", filled)) +
		lipgloss.NewStyle().Foreground(subtle).Render(strings.Repeat("░", width-filled))
	return "[" + bar + "]"
}

// tabBar renders the tab labels as a single row: labels separated by single
// spaces, the active label highlighted via selectedStyle. Total width fits
// within inspectorInnerWidth().
func (in inspectorModel) tabBar() string {
	var b strings.Builder
	for i, t := range inspectorTabLabels() {
		if i > 0 {
			b.WriteString(lipgloss.NewStyle().Foreground(subtle).Render(" "))
		}
		if inspectorTab(i) == in.tab {
			b.WriteString(selectedStyle.Render(t))
		} else {
			b.WriteString(lipgloss.NewStyle().Foreground(subtle).Render(t))
		}
	}
	return b.String()
}

// inspectorTabAtX maps an x offset within the tab bar to a tab. Labels are
// rendered as: label0 space label1 space label2 space label3 (no chip padding).
func inspectorTabAtX(localX int) (inspectorTab, bool) {
	x := 0
	for i, t := range inspectorTabLabels() {
		w := len(t)
		if localX >= x && localX < x+w {
			return inspectorTab(i), true
		}
		x += w + 1 // +1 for the separator space
	}
	return tabWords, false
}

var blankLineRe = regexp.MustCompile(`\n[ \t]*\n`)

// computeDocStats derives word/char/paragraph counts from the open buffer.
func computeDocStats(text string) docStats {
	if strings.TrimSpace(text) == "" {
		return docStats{}
	}
	ds := docStats{
		words: wordCount(text),
		chars: utf8.RuneCountInString(text),
	}
	for _, block := range blankLineRe.Split(text, -1) {
		if strings.TrimSpace(block) != "" {
			ds.paragraphs++
		}
	}
	// ~210 wpm French silent-reading estimate — a less rigorously sourced figure than the
	// prior 238 wpm (Brysbaert 2019 meta-analysis, English-specific); midpoint of a 180-230
	// wpm range found for French. Revisit if a better-sourced French figure surfaces.
	ds.readSecs = ds.words * 60 / 210
	ds.sentMean, ds.sentStdDev = sentenceStats(text)
	sentences := splitSentencesFR(text)
	if len(sentences) > 0 && ds.words > 0 {
		// syllablesPerWord must divide by the SAME token count used to sum totalSyllables
		// (wordTokenRe tokens), not ds.words (strings.Fields). The two counters diverge on
		// hyphenated compounds ("peut-être" is 1 Fields word but 2 wordTokenRe tokens: "peut",
		// "être") and on pure-digit "words" like "1958" (1 Fields word, 0 wordTokenRe tokens).
		// Mixing bases skews the ratio and can shift the score across a whole label tier.
		tokens := wordTokenRe.FindAllString(text, -1)
		if len(tokens) > 0 {
			totalSyllables := 0
			for _, w := range tokens {
				totalSyllables += syllableCountFR(w)
			}
			wordsPerSentence := float64(ds.words) / float64(len(sentences))
			syllablesPerWord := float64(totalSyllables) / float64(len(tokens))
			ds.readabilityScore = clampScore(kandelMolesScore(wordsPerSentence, syllablesPerWord))
			ds.readabilityLabel = readabilityLabel(ds.readabilityScore)
		}
	}
	ds.overused = overusedWords(text, 5)
	return ds
}

// frAbbreviations lists common French abbreviations whose trailing period must NOT be
// treated as a sentence end (splitSentencesFR checks for these immediately before a
// period). Kept as a closed heuristic list, not a linguistic abbreviation detector —
// consistent with the rest of this file's "cheap, useful signal" approach.
var frAbbreviations = []string{
	"M", "Mme", "Mlle", "Dr", "etc", "cf", "p", "ex", "ch", "art", "vol", "éd", "trad", "av", "apr", "J.-C",
}

// sentenceEndRe matches one or more sentence-ending punctuation marks (. ! ? or the
// ellipsis character …), collapsing a run of them (e.g. "...", "?!") into a single split
// point rather than one split per character.
var sentenceEndRe = regexp.MustCompile(`(?:\.{3}|…|[.!?])+`)

// endsWithAbbreviation reports whether s (text immediately preceding a sentence-ending
// match) ends with one of frAbbreviations, meaning the period is part of the abbreviation
// and does not end the sentence.
//
// The check requires a word boundary before the abbreviation: it compares the LAST
// whitespace-delimited token of s against frAbbreviations, not an arbitrary trailing
// substring. A naive strings.HasSuffix(trimmed, abbr) would also match ordinary words that
// merely end with the same letters as a short abbreviation — e.g. "trop", "loup", "coup",
// "beaucoup" all end in "p" (the abbreviation for "page"), and "match", "riche" end in "ch"
// (the abbreviation for "chapitre"), which would wrongly suppress real sentence breaks.
func endsWithAbbreviation(s string) bool {
	trimmed := strings.TrimRight(s, " \t")
	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return false
	}
	lastWord := fields[len(fields)-1]
	for _, abbr := range frAbbreviations {
		if lastWord == abbr {
			return true
		}
	}
	return false
}

// splitSentencesFR splits text into sentences, treating runs of terminal punctuation
// (. ! ? ... …) as one boundary and skipping boundaries that immediately follow a known
// French abbreviation (M., etc., p., ...). A closed-list heuristic, not a full sentence
// tokenizer — no handling of nested quotes or abbreviations at a true sentence end.
func splitSentencesFR(text string) []string {
	var sentences []string
	last := 0
	matches := sentenceEndRe.FindAllStringIndex(text, -1)
	for _, m := range matches {
		start, end := m[0], m[1]
		if endsWithAbbreviation(text[last:start]) {
			continue
		}
		sentences = append(sentences, text[last:end])
		last = end
	}
	if last < len(text) {
		sentences = append(sentences, text[last:])
	}
	return sentences
}

// sentenceStats returns the mean and population standard deviation of sentence length (in
// words), using splitSentencesFR — a heuristic split (abbreviations end a "sentence" was
// the old failure mode; French abbreviations are now excluded), but still a cheap, useful
// signal for prose rhythm rather than a full sentence tokenizer.
func sentenceStats(text string) (mean, std float64) {
	var lens []float64
	for _, s := range splitSentencesFR(text) {
		if n := len(strings.Fields(s)); n > 0 {
			lens = append(lens, float64(n))
		}
	}
	if len(lens) == 0 {
		return 0, 0
	}
	var sum float64
	for _, l := range lens {
		sum += l
	}
	mean = sum / float64(len(lens))
	var v float64
	for _, l := range lens {
		d := l - mean
		v += d * d
	}
	return mean, math.Sqrt(v / float64(len(lens)))
}

// frVowels are the letters (lowercase) counted as vowel-group starts for the syllable
// heuristic: base vowels plus accented forms. This is an orthographic proxy, not a
// phonetic analysis — consistent with how Flesch/Kandel-Moles themselves count syllables
// (consecutive-vowel-group counting, not true syllabification).
var frVowels = map[rune]bool{
	'a': true, 'e': true, 'i': true, 'o': true, 'u': true, 'y': true,
	'é': true, 'è': true, 'ê': true, 'à': true, 'â': true,
	'ù': true, 'û': true, 'î': true, 'ï': true, 'ô': true, 'œ': true, 'æ': true,
}

// syllableCountFR estimates a word's syllable count by counting runs of consecutive
// vowel-group letters, then applying the French "e muet" rule: a word-final unaccented
// "e" does not count as its own syllable if the word has at least one other vowel group.
// Heuristic, not phonetic — matches the level of approximation Kandel-Moles itself uses.
func syllableCountFR(word string) int {
	runes := []rune(strings.ToLower(word))
	if len(runes) == 0 {
		return 0
	}
	groups := 0
	inGroup := false
	for _, r := range runes {
		if frVowels[r] {
			if !inGroup {
				groups++
				inGroup = true
			}
		} else {
			inGroup = false
		}
	}
	// E muet final: a trailing unaccented "e" that formed its own trailing vowel group
	// doesn't count, as long as the word has another vowel group.
	if groups > 1 && runes[len(runes)-1] == 'e' {
		groups--
	}
	if groups == 0 {
		groups = 1 // every word counts as at least one syllable
	}
	return groups
}

// kandelMolesScore computes the Kandel & Moles (1958) French adaptation of the Flesch
// Reading Ease formula. Higher is easier to read. The raw formula can exceed [0,100] on
// extreme texts — callers should clamp for display (standard Flesch/Kandel-Moles
// convention).
func kandelMolesScore(wordsPerSentence, syllablesPerWord float64) float64 {
	return 207 - 1.015*wordsPerSentence - 73.6*syllablesPerWord
}

// readabilityLabel maps a Kandel-Moles score to the standard 7-tier French interpretation
// scale.
func readabilityLabel(score float64) string {
	switch {
	case score >= 80:
		return "Très facile"
	case score >= 70:
		return "Facile"
	case score >= 60:
		return "Assez facile"
	case score >= 50:
		return "Moyen"
	case score >= 40:
		return "Assez difficile"
	case score >= 30:
		return "Difficile"
	default:
		return "Très difficile"
	}
}

// clampScore bounds a raw readability score to [0, 100] for display.
func clampScore(score float64) float64 {
	if score < 0 {
		return 0
	}
	if score > 100 {
		return 100
	}
	return score
}

var wordTokenRe = regexp.MustCompile(`[\p{L}']+`)

// overusedWords returns the top content words by frequency (case-folded, stop words and
// very short words removed), each appearing at least 3 times, ordered by count then
// alphabetically for stable rendering. Returns at most `top` entries.
func overusedWords(text string, top int) []wordFreq {
	counts := map[string]int{}
	for _, w := range wordTokenRe.FindAllString(strings.ToLower(text), -1) {
		w = strings.Trim(w, "'")
		if len(w) < 4 || stopWords[w] {
			continue
		}
		counts[w]++
	}
	var fs []wordFreq
	for w, n := range counts {
		if n >= 3 {
			fs = append(fs, wordFreq{w, n})
		}
	}
	sort.Slice(fs, func(i, j int) bool {
		if fs[i].n != fs[j].n {
			return fs[i].n > fs[j].n
		}
		return fs[i].word < fs[j].word
	})
	if len(fs) > top {
		fs = fs[:top]
	}
	return fs
}

// stopWords are common function words excluded from the overused-word list (they're always
// frequent and carry no craft signal). Length-<4 words are already dropped, so this only
// needs the frequent 4+-letter function words.
var stopWords = map[string]bool{
	"that": true, "this": true, "with": true, "from": true, "they": true, "them": true,
	"then": true, "than": true, "were": true, "have": true, "here": true, "there": true,
	"what": true, "when": true, "which": true, "while": true, "would": true, "could": true,
	"should": true, "been": true, "your": true, "yours": true, "into": true,
	"over": true, "some": true, "such": true, "only": true, "will": true, "shall": true,
	"upon": true, "about": true, "their": true, "these": true, "those": true, "does": true,
	// Crutch words like "just", "very", "really" are deliberately NOT excluded — flagging
	// their overuse is the point.
}

// computeProjStats sums word counts across the WHOLE project folder: manifest
// chapters plus any loose/Resources files, so a writing goal set at the
// project level tracks total output regardless of which file words land in
// (design §4 — a goal split across files still counts toward the total).
func computeProjStats(dir string, v manuscriptView, wc *wordCountCache) projStats {
	if wc == nil {
		return projStats{}
	}
	ps := projStats{}
	for _, p := range v.parts {
		ps.chapters += len(p.chapters)
		for _, ch := range p.chapters {
			if len(ch.texts) == 0 {
				continue
			}
			ps.words += wc.count(filepath.Join(dir, ch.folder, ch.texts[0].file))
		}
	}
	ps.manuscript = ps.chapters > 0
	for _, e := range v.loose {
		ps.words += wc.count(filepath.Join(dir, e.name))
	}
	return ps
}

// kvRow renders "  label" left, a subtle right-aligned number, fit to width.
func kvRow(label string, n, width int) string {
	return kvStrRow(label, commafy(n), width)
}

// kvStrRow is kvRow with a pre-formatted string value.
func kvStrRow(label, val string, width int) string {
	lbl := "  " + label
	gap := width - lipgloss.Width(lbl) - lipgloss.Width(val)
	if gap < 1 {
		gap = 1
	}
	return lbl + strings.Repeat(" ", gap) + lipgloss.NewStyle().Foreground(subtle).Render(val)
}

// fmtReadTime formats a reading-time estimate as m:ss.
func fmtReadTime(secs int) string {
	return fmt.Sprintf("%d:%02d", secs/60, secs%60)
}

// View renders the tab bar + the active tab's body, fit to the given inner width.
func (in inspectorModel) View(width int, doc docStats, proj projStats, outline string, goals goalStats, analysis analysisState, notes []note) string {
	var b strings.Builder
	b.WriteString(in.tabBar())
	b.WriteString("\n\n")
	switch in.tab {
	case tabAnalysis:
		b.WriteString(sectionHeader("Analyse", width) + "\n\n")
		b.WriteString("  " + checkbox(analysis.spell) + "Orthographe\n")
		b.WriteString("  " + checkbox(analysis.grammar) + grammarStyle.Render("Grammaire"))
		if analysis.grammar && in.grammarBackend != "" {
			// Stable layout so the click rows never shift: action (analysisActionRowY=7),
			// backend name (8, dim — keeps a long name off the action row), Auto-recheck (9).
			action := "▸ Vérifier la grammaire"
			if in.grammarChecking {
				action = "vérification…"
			}
			b.WriteString("\n\n  " + action)
			b.WriteString("\n  " + lipgloss.NewStyle().Foreground(subtle).Render(in.grammarBackend))
			b.WriteString("\n  " + checkbox(in.grammarAutoRecheck) + "Revérification auto")
		}
	case tabOutline:
		b.WriteString(sectionHeader("Plan", width) + "\n\n")
		outLines := strings.Split(renderOutline(outline, width-2), "\n")
		indented := make([]string, len(outLines))
		for i, l := range outLines {
			indented[i] = "  " + l
		}
		b.WriteString(strings.Join(indented, "\n"))
	case tabGoals:
		b.WriteString(sectionHeader("Aujourd'hui", width) + "\n")
		b.WriteString("  " + progressBar(goals.today, goals.dailyGoal, max(4, width-10)) + "\n")
		b.WriteString("  " + fmt.Sprintf("%s / %s\n", commafy(goals.today), commafy(goals.dailyGoal)))
		if goals.today >= goals.dailyGoal && goals.dailyGoal > 0 {
			b.WriteString("  " + lipgloss.NewStyle().Foreground(accent).Render("✓ objectif atteint"))
		} else {
			b.WriteString("  " + lipgloss.NewStyle().Foreground(subtle).Render(commafy(goals.dailyGoal-goals.today)+" restants"))
		}
		if goals.projectGoal > 0 {
			b.WriteString("\n\n" + sectionHeader("Projet", width) + "\n")
			b.WriteString("  " + progressBar(goals.project, goals.projectGoal, max(4, width-10)) + "\n")
			b.WriteString("  " + fmt.Sprintf("%s / %s", commafy(goals.project), commafy(goals.projectGoal)))
			if goals.pace != "" {
				b.WriteString("\n  " + lipgloss.NewStyle().Foreground(accent).Render(goals.pace))
			}
		}
		b.WriteString("\n\n" + sectionHeader("Temps", width) + "\n")
		sess := "  Session   " + fmtDuration(time.Duration(goals.sessionSecs)*time.Second)
		if goals.idle {
			sess += " ⏸"
		}
		b.WriteString(sess + "\n")
		if goals.sessionGoalMin > 0 {
			mins := goals.todayActiveSecs / 60
			b.WriteString("  Aujourd'hui\n")
			b.WriteString("  " + progressBar(mins, goals.sessionGoalMin, max(4, width-10)) + "\n")
			b.WriteString("  " + fmt.Sprintf("%d / %d min\n", mins, goals.sessionGoalMin))
			if mins >= goals.sessionGoalMin {
				b.WriteString("  " + lipgloss.NewStyle().Foreground(accent).Render("✓ objectif de temps atteint"))
			} else {
				b.WriteString("  " + lipgloss.NewStyle().Foreground(subtle).Render(fmt.Sprintf("%d min restantes", goals.sessionGoalMin-mins)))
			}
		} else {
			b.WriteString("  Aujourd'hui  " + fmtDuration(time.Duration(goals.todayActiveSecs)*time.Second) + "\n")
		}
		if len(goals.spark) > 0 {
			b.WriteString("\n\n" + sectionHeader("Historique", width) + "\n")
			hist := "  " + sparkline(goals.spark)
			if goals.streakDays > 0 {
				hist += lipgloss.NewStyle().Foreground(accent).Render(fmt.Sprintf("  série de %d jours", goals.streakDays))
			}
			b.WriteString(hist + "\n  " + lipgloss.NewStyle().Foreground(subtle).Render("g → historique complet"))
		}
	default: // tabWords
		b.WriteString(sectionHeader("Document", width) + "\n")
		b.WriteString("  " + kvRow("Mots", doc.words, width-2) + "\n")
		b.WriteString("  " + kvRow("Caractères", doc.chars, width-2) + "\n")
		b.WriteString("  " + kvRow("Paragraphes", doc.paragraphs, width-2) + "\n\n")
		b.WriteString(sectionHeader("Projet", width) + "\n")
		b.WriteString("  " + kvRow("Mots", proj.words, width-2))
		if proj.manuscript {
			b.WriteString("\n  " + kvRow("Chapitres", proj.chapters, width-2))
		}
		if doc.words > 0 {
			b.WriteString("\n\n" + sectionHeader("Lisibilité", width) + "\n")
			b.WriteString("  " + kvStrRow("Temps de lecture", fmtReadTime(doc.readSecs), width-2) + "\n")
			b.WriteString("  " + kvStrRow("Phrase moy.", fmt.Sprintf("%.0f±%.0f mots", doc.sentMean, doc.sentStdDev), width-2))
			if doc.readabilityLabel != "" {
				score := fmt.Sprintf("%.0f · %s", doc.readabilityScore, doc.readabilityLabel)
				b.WriteString("\n  " + kvStrRow("Score", score, width-2))
			}
			if len(doc.overused) > 0 {
				b.WriteString("\n\n" + sectionHeader("Surutilisés", width) + "\n")
				for i, wf := range doc.overused {
					b.WriteString("  " + kvRow(wf.word, wf.n, width-2))
					if i < len(doc.overused)-1 {
						b.WriteString("\n")
					}
				}
			}
			b.WriteString("\n\n" + renderNotesSection(notes, width))
		}
	}
	return b.String()
}
