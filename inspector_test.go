package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestComputeDocStats(t *testing.T) {
	ds := computeDocStats("Hello world.\n\nSecond paragraph here.\n")
	if ds.words != 5 {
		t.Fatalf("words = %d, want 5", ds.words)
	}
	if ds.paragraphs != 2 {
		t.Fatalf("paragraphs = %d, want 2", ds.paragraphs)
	}
	if ds.chars == 0 {
		t.Fatal("chars should be non-zero")
	}
	// Trailing blank lines must not inflate the paragraph count.
	if got := computeDocStats("One.\n\n\n\nTwo.\n\n").paragraphs; got != 2 {
		t.Fatalf("paragraphs with extra blank lines = %d, want 2", got)
	}
	// Empty buffer → all zero.
	if z := computeDocStats(""); z.words != 0 || z.chars != 0 || z.paragraphs != 0 {
		t.Fatalf("empty stats = %+v, want zero", z)
	}
}

func TestComputeProjStatsManuscript(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-a.md"), []byte("one two three"), 0o644)      // 3
	os.WriteFile(filepath.Join(dir, "02-b.md"), []byte("four five"), 0o644)          // 2
	os.WriteFile(filepath.Join(dir, "notes.md"), []byte("six seven eight"), 0o644)   // 3 (loose)
	os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("loose note words"), 0o644) // 3 (loose)
	v := resolveManuscript(dir, readEntries(dir))
	ps := computeProjStats(dir, v, newWordCountCache())
	if !ps.manuscript {
		t.Fatal("expected manuscript = true (numbered chapters present)")
	}
	if ps.chapters != 2 {
		t.Fatalf("chapters = %d, want 2", ps.chapters)
	}
	// Now aggregates ALL files: chapters (3+2) + loose files (3+3) = 11 total words.
	if ps.words != 11 {
		t.Fatalf("project words = %d, want 11 (01-a.md[3] + 02-b.md[2] + notes.md[3] + notes.txt[3])", ps.words)
	}
}

func TestComputeProjStatsPlainFolder(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.md"), []byte("one two"), 0o644)
	os.WriteFile(filepath.Join(dir, "b.md"), []byte("three"), 0o644)
	v := resolveManuscript(dir, readEntries(dir))
	ps := computeProjStats(dir, v, newWordCountCache())
	if ps.manuscript {
		t.Fatal("plain folder must not be a manuscript")
	}
	if ps.words != 3 {
		t.Fatalf("folder words = %d, want 3 (sum of loose docs)", ps.words)
	}
}

func TestInspectorViewRendersWords(t *testing.T) {
	in := inspectorModel{visible: true}
	out := in.View(28, docStats{words: 1204, chars: 6830, paragraphs: 38}, projStats{words: 47032, chapters: 12, manuscript: true}, "", goalStats{}, analysisState{}, nil)
	for _, want := range []string{"Mots", "DOCUMENT", "PROJET", "1,204", "47,032", "Chapitres", "12"} {
		if !strings.Contains(out, want) {
			t.Fatalf("inspector view missing %q:\n%s", want, out)
		}
	}
	// Non-manuscript omits the Chapitres line.
	plain := in.View(28, docStats{words: 10}, projStats{words: 10, manuscript: false}, "", goalStats{}, analysisState{}, nil)
	if strings.Contains(plain, "Chapitres") {
		t.Fatal("non-manuscript inspector should omit 'Chapitres'")
	}
}

func TestInspectorCycle(t *testing.T) {
	in := inspectorModel{}
	in.cycle()
	if !in.visible || in.tab != tabWords {
		t.Fatalf("first cycle: visible=%v tab=%v, want visible Words", in.visible, in.tab)
	}
	in.cycle()
	if !in.visible || in.tab != tabOutline {
		t.Fatalf("second cycle: visible=%v tab=%v, want visible Outline", in.visible, in.tab)
	}
	in.cycle()
	if !in.visible || in.tab != tabGoals {
		t.Fatalf("third cycle: visible=%v tab=%v, want visible Goals", in.visible, in.tab)
	}
	in.cycle()
	if !in.visible || in.tab != tabAnalysis {
		t.Fatalf("fourth cycle: visible=%v tab=%v, want visible Analysis", in.visible, in.tab)
	}
	in.cycle()
	if in.visible {
		t.Fatal("fifth cycle should close the inspector (past the last tab)")
	}
	if in.tab != tabWords {
		t.Fatalf("closed cycle should reset tab to Words, got %v", in.tab)
	}
}

func TestInspectorOutlineTab(t *testing.T) {
	in := inspectorModel{visible: true, tab: tabOutline}
	out := in.View(28, docStats{}, projStats{}, "- Top\n  - sub", goalStats{}, analysisState{}, nil)
	for _, want := range []string{"Plan", "Top", "sub"} {
		if !strings.Contains(out, want) {
			t.Fatalf("outline tab missing %q:\n%s", want, out)
		}
	}
	empty := in.View(28, docStats{}, projStats{}, "", goalStats{}, analysisState{}, nil)
	if !strings.Contains(empty, "vide") {
		t.Fatal("empty outline should show an (vide …) hint")
	}
}

func TestProgressBar(t *testing.T) {
	if b := progressBar(0, 100, 10); strings.Count(b, "█") != 0 {
		t.Fatalf("0%% bar should have no fill: %q", b)
	}
	half := progressBar(50, 100, 10)
	if n := strings.Count(half, "█"); n < 4 || n > 6 {
		t.Fatalf("50%% bar fill = %d cells, want ~5", n)
	}
	if n := strings.Count(progressBar(200, 100, 10), "█"); n != 10 {
		t.Fatalf(">=100%% bar should be full (10), got %d", n)
	}
	if strings.Count(progressBar(5, 0, 10), "█") != 0 {
		t.Fatal("goal 0 → empty bar")
	}
}

func TestInspectorGoalsTab(t *testing.T) {
	in := inspectorModel{visible: true, tab: tabGoals}
	out := in.View(28, docStats{}, projStats{}, "", goalStats{today: 312, dailyGoal: 500, project: 47032, projectGoal: 80000}, analysisState{}, nil)
	for _, w := range []string{"AUJOURD'HUI", "312", "500", "PROJET", "80,000"} {
		if !strings.Contains(out, w) {
			t.Fatalf("goals tab missing %q:\n%s", w, out)
		}
	}
	// projectGoal 0 → no Project section.
	noproj := in.View(28, docStats{}, projStats{}, "", goalStats{today: 10, dailyGoal: 500, project: 10, projectGoal: 0}, analysisState{}, nil)
	if strings.Contains(noproj, "PROJET") {
		t.Fatal("projectGoal 0 should omit the Project section")
	}
	// goal met.
	met := in.View(28, docStats{}, projStats{}, "", goalStats{today: 600, dailyGoal: 500, project: 1, projectGoal: 0}, analysisState{}, nil)
	if !strings.Contains(met, "atteint") {
		t.Fatal("today >= daily goal should show '✓ objectif atteint'")
	}
}

func TestInspectorTabAtX(t *testing.T) {
	// labels {"Mots","Plan","Objectifs","Analyse"} → no padding, single-space separated.
	// Mots(0..3) space(4) Plan(5..8) space(9) Objectifs(10..19) space(20) Analyse(21..27)
	if tb, ok := inspectorTabAtX(2); !ok || tb != tabWords {
		t.Fatalf("x=2 → %v ok=%v, want Mots", tb, ok)
	}
	if tb, ok := inspectorTabAtX(7); !ok || tb != tabOutline {
		t.Fatalf("x=7 → %v ok=%v, want Plan", tb, ok)
	}
	if tb, ok := inspectorTabAtX(15); !ok || tb != tabGoals {
		t.Fatalf("x=15 → %v ok=%v, want Objectifs", tb, ok)
	}
	if tb, ok := inspectorTabAtX(24); !ok || tb != tabAnalysis {
		t.Fatalf("x=24 → %v ok=%v, want Analyse", tb, ok)
	}
	if _, ok := inspectorTabAtX(100); ok {
		t.Fatal("x past the last chip → not ok")
	}
}

func TestInspectorAnalysisTab(t *testing.T) {
	in := inspectorModel{visible: true, tab: tabAnalysis}
	on := in.View(inspectorInnerWidth(), docStats{}, projStats{}, "", goalStats{}, analysisState{spell: true, adverb: false}, nil)
	if !strings.Contains(on, "Orthographe") || !strings.Contains(on, "SYNTAXE") {
		t.Fatalf("analysis tab should list Orthographe and SYNTAXE:\n%s", on)
	}
	if !strings.Contains(on, "[x] Orthographe") {
		t.Fatalf("spell on → checked box:\n%s", on)
	}
	off := in.View(inspectorInnerWidth(), docStats{}, projStats{}, "", goalStats{}, analysisState{}, nil)
	if !strings.Contains(off, "[ ] Orthographe") {
		t.Fatalf("spell off → empty box:\n%s", off)
	}
}

func TestInspectorAnalysisRowAtY(t *testing.T) {
	// Spellcheck is at analysisRowY(0); Grammar at analysisRowY(1).
	if r, ok := inspectorAnalysisRowAtY(analysisRowY(0)); !ok || r != 0 {
		t.Fatalf("Spellcheck row → %d ok=%v, want 0", r, ok)
	}
	if r, ok := inspectorAnalysisRowAtY(analysisRowY(1)); !ok || r != 1 {
		t.Fatalf("Grammar row → %d ok=%v, want 1", r, ok)
	}
	if _, ok := inspectorAnalysisRowAtY(0); ok {
		t.Fatal("the tab-bar row is not a checkbox row")
	}
}

func TestAnalysisTabPOSList(t *testing.T) {
	in := inspectorModel{visible: true, tab: tabAnalysis}
	out := in.View(inspectorInnerWidth(), docStats{}, projStats{}, "", goalStats{}, analysisState{spell: true, adverb: true}, nil)
	for _, w := range []string{"Orthographe", "Grammaire", "SYNTAXE", "Adverbe", "Adjectif", "Passif"} {
		if !strings.Contains(out, w) {
			t.Fatalf("analysis tab missing %q:\n%s", w, out)
		}
	}
	if !strings.Contains(out, "[x] Orthographe") || !strings.Contains(out, "[x] Adverbe") {
		t.Fatalf("toggled-on checkboxes should render [x]:\n%s", out)
	}
	// Verify rows render at the expected Y positions.
	// Row indices: 0=Orthographe, 1=Grammaire, 2=Adverbe, 3=Adjectif, 4=Passif.
	lines := strings.Split(out, "\n")
	for i, label := range []string{"Orthographe", "Grammaire", "Adverbe", "Adjectif", "Passif"} {
		y := analysisRowY(i)
		if y >= len(lines) || !strings.Contains(lines[y], label) {
			t.Fatalf("row %d (analysisRowY(%d)=%d) should contain %q, got %q", i, i, y, label, func() string {
				if y < len(lines) {
					return lines[y]
				}
				return "<out of range>"
			}())
		}
	}
}

func TestTabBarFitsOneRow(t *testing.T) {
	in := inspectorModel{visible: true}
	bar := in.tabBar()
	if lipgloss.Height(bar) != 1 {
		t.Fatalf("tab bar should be one row, got %d: %q", lipgloss.Height(bar), bar)
	}
	if lipgloss.Width(bar) > inspectorInnerWidth() {
		t.Fatalf("tab bar width %d exceeds inner width %d", lipgloss.Width(bar), inspectorInnerWidth())
	}
}

func TestAnalysisRowAtY(t *testing.T) {
	if r, ok := inspectorAnalysisRowAtY(analysisRowY(0)); !ok || r != 0 {
		t.Fatalf("Spellcheck row → %d ok=%v, want 0", r, ok)
	}
	if r, ok := inspectorAnalysisRowAtY(analysisRowY(4)); !ok || r != 4 {
		t.Fatalf("Passive row → %d ok=%v, want 4", r, ok)
	}
	if _, ok := inspectorAnalysisRowAtY(0); ok {
		t.Fatal("the tab-bar row is not a checkbox row")
	}
}

func TestAnalysisGrammarRow(t *testing.T) {
	in := inspectorModel{visible: true, tab: tabAnalysis}
	out := in.View(inspectorInnerWidth(), docStats{}, projStats{}, "", goalStats{}, analysisState{grammar: true}, nil)
	if !strings.Contains(out, "Grammaire") || !strings.Contains(out, "[x] Grammaire") {
		t.Fatalf("Analysis tab should show a checked Grammaire row:\n%s", out)
	}
}

func TestAnalysisRowYWithGrammar(t *testing.T) {
	// 5 checkboxes now: Spellcheck, Grammar, Adverb, Adjective, Passive.
	for i := 0; i < 5; i++ {
		y := analysisRowY(i)
		if r, ok := inspectorAnalysisRowAtY(y); !ok || r != i {
			t.Fatalf("analysisRowY(%d)=%d → inspectorAnalysisRowAtY=%d,%v", i, y, r, ok)
		}
	}
}

func TestFramedPanel(t *testing.T) {
	out := framedPanel("Mots", "alpha\nbeta", 20, 6, "")
	lines := strings.Split(ansi.Strip(out), "\n")
	if len(lines) != 6 {
		t.Fatalf("framedPanel height: want 6 lines, got %d", len(lines))
	}
	if !strings.HasPrefix(lines[0], "╭") || !strings.Contains(lines[0], "Mots") || !strings.HasSuffix(lines[0], "╮") {
		t.Fatalf("top border malformed: %q", lines[0])
	}
	if !strings.HasPrefix(lines[5], "╰") || !strings.HasSuffix(lines[5], "╯") {
		t.Fatalf("bottom border malformed: %q", lines[5])
	}
	for i, ln := range lines {
		if w := lipgloss.Width(ln); w != 20 {
			t.Fatalf("line %d width = %d, want 20: %q", i, w, ln)
		}
	}
	// content present and bordered
	if !strings.Contains(lines[1], "alpha") || !strings.HasPrefix(lines[1], "│") || !strings.HasSuffix(lines[1], "│") {
		t.Fatalf("content line malformed: %q", lines[1])
	}
}

func TestSectionHeader(t *testing.T) {
	out := ansi.Strip(sectionHeader("Document", 24))
	if !strings.HasPrefix(out, "DOCUMENT ") {
		t.Fatalf("section header should start with UPPERCASE label + space: %q", out)
	}
	if !strings.Contains(out, "─") {
		t.Fatalf("section header should have a rule fill: %q", out)
	}
	if lipgloss.Width(out) != 24 {
		t.Fatalf("section header width = %d, want 24", lipgloss.Width(out))
	}
}

func TestFramedPanelTruncatesLongLine(t *testing.T) {
	// An inner line wider than the content area must be truncated, not wrapped —
	// every output line stays exactly `width` and there are exactly `height` lines.
	long := "Words Outline Goals Analysis Extra Overflowing Tabs"
	out := framedPanel("X", long, 20, 4, "")
	lines := strings.Split(ansi.Strip(out), "\n")
	if len(lines) != 4 {
		t.Fatalf("over-long inner line wrapped: want 4 lines, got %d:\n%s", len(lines), ansi.Strip(out))
	}
	for i, ln := range lines {
		if lipgloss.Width(ln) != 20 {
			t.Fatalf("line %d width %d != 20 (overflow): %q", i, lipgloss.Width(ln), ln)
		}
		if i > 0 && i < 3 && (!strings.HasPrefix(ln, "│") || !strings.HasSuffix(ln, "│")) {
			t.Fatalf("content line %d lost its border (wrap): %q", i, ln)
		}
	}
}

func TestFramedPanelAction(t *testing.T) {
	out := ansi.Strip(framedPanel("Files", "x", 20, 4, "+"))
	top := strings.Split(out, "\n")[0]
	if !strings.Contains(top, "+") || !strings.HasPrefix(top, "╭") || !strings.HasSuffix(top, "╮") {
		t.Fatalf("top border should carry the + action: %q", top)
	}
	if strings.Contains(strings.Split(ansi.Strip(framedPanel("Files", "x", 20, 4, "")), "\n")[0], "+") {
		t.Fatal("no action → no + in the border")
	}
}

func TestInspectorGoalsSessionSection(t *testing.T) {
	in := inspectorModel{visible: true, tab: tabGoals}
	out := ansi.Strip(in.View(28, docStats{}, projStats{}, "", goalStats{sessionSecs: 300, todayActiveSecs: 600, sessionGoalMin: 30, idle: true}, analysisState{}, nil))
	if !strings.Contains(out, "TEMPS") || !strings.Contains(out, "10 / 30 min") {
		t.Fatalf("expected a TEMPS section with 10/30 min, got:\n%s", out)
	}
	if !strings.Contains(out, "⏸") {
		t.Fatalf("idle should show ⏸, got:\n%s", out)
	}
}

func TestFramedPanelActionNoOverflow(t *testing.T) {
	// A title longer than the panel + an action must not overflow the width.
	for _, w := range []int{18, 34, 40} {
		top := strings.Split(ansi.Strip(framedPanel(strings.Repeat("x", w+8), "y", w, 4, "+")), "\n")[0]
		if lipgloss.Width(top) != w {
			t.Fatalf("framedPanel(width %d, long title, action) top width = %d, want %d", w, lipgloss.Width(top), w)
		}
		if !strings.Contains(top, "+") {
			t.Fatalf("width %d dropped the action", w)
		}
	}
}

func TestComputeProjStatsAggregatesChaptersAndLoose(t *testing.T) {
	dir := t.TempDir()
	// A manifest-driven chapter.
	if err := os.WriteFile(filepath.Join(dir, "chapter-one.md"), []byte("un deux trois quatre cinq"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A loose Resources file, not listed in any manifest.
	if err := os.WriteFile(filepath.Join(dir, "notes.md"), []byte("six sept huit"), 0o644); err != nil {
		t.Fatal(err)
	}

	v := manuscriptView{
		source:   sourceManifest,
		chapters: []chapterRef{{file: "chapter-one.md", title: "Chapter One"}},
		loose:    []fileEntry{{name: "notes.md"}},
	}
	wc := newWordCountCache()

	ps := computeProjStats(dir, v, wc)

	// 5 words in the chapter + 3 in the loose file = 8 total.
	if ps.words != 8 {
		t.Fatalf("expected words=8 (chapters + loose combined), got %d", ps.words)
	}
	if !ps.manuscript {
		t.Fatalf("expected manuscript=true when chapters are present")
	}
	if ps.chapters != 1 {
		t.Fatalf("expected chapters=1, got %d", ps.chapters)
	}
}

func TestComputeProjStatsLooseOnly(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "draft.md"), []byte("un deux"), 0o644); err != nil {
		t.Fatal(err)
	}
	v := manuscriptView{
		source: sourceNone,
		loose:  []fileEntry{{name: "draft.md"}},
	}
	wc := newWordCountCache()

	ps := computeProjStats(dir, v, wc)

	if ps.words != 2 {
		t.Fatalf("expected words=2, got %d", ps.words)
	}
	if ps.manuscript {
		t.Fatalf("expected manuscript=false when there are no chapters")
	}
}

func TestSplitSentencesFR(t *testing.T) {
	// Une abréviation suivie d'un point ne doit pas terminer la phrase.
	got := splitSentencesFR("M. Dupont est arrivé. Il a souri.")
	if len(got) != 2 {
		t.Fatalf("expected 2 sentences, got %d: %+v", len(got), got)
	}

	got = splitSentencesFR("Voir p. 12 pour plus de détails. La suite au chapitre suivant.")
	if len(got) != 2 {
		t.Fatalf("expected 2 sentences (abbreviation 'p.' should not split), got %d: %+v", len(got), got)
	}

	// Une ellipse (trois points ou caractère unique) est UN SEUL séparateur de fin de
	// phrase (comme '.', '!' ou '?'), pas un par point : le texte a 3 fins de phrase
	// ("hésita...", "décida.", "rapide.") donc 3 segments, mais le premier segment doit
	// rester intact ("Il hésita...") plutôt que d'être fragmenté en 3 par un split naïf
	// caractère par caractère sur "...".
	got = splitSentencesFR("Il hésita... puis se décida. Ce fut rapide.")
	if len(got) != 3 {
		t.Fatalf("expected 3 sentences with '...' as one separator (not fragmented per dot), got %d: %+v", len(got), got)
	}
	if strings.TrimSpace(got[0]) != "Il hésita..." {
		t.Fatalf("expected first sentence to be %q (ellipsis kept as one boundary), got %q", "Il hésita...", strings.TrimSpace(got[0]))
	}

	got = splitSentencesFR("Il hésita… puis se décida. Ce fut rapide.")
	if len(got) != 3 {
		t.Fatalf("expected 3 sentences with '…' as one separator, got %d: %+v", len(got), got)
	}
	if strings.TrimSpace(got[0]) != "Il hésita…" {
		t.Fatalf("expected first sentence to be %q (ellipsis kept as one boundary), got %q", "Il hésita…", strings.TrimSpace(got[0]))
	}

	// Cas limite documenté comme hors périmètre par le design : un point d'abréviation en
	// fin de phrase réelle (« ...etc. Puis... ») n'est pas distingué d'un point
	// d'abréviation en milieu de phrase — la liste fermée d'abréviations skippe le point
	// dans les deux cas, donc le texte reste une seule phrase ici.
	got = splitSentencesFR("Il a tout vendu : meubles, livres, etc. Puis il est parti.")
	if len(got) != 1 {
		t.Fatalf("expected 1 sentence ('etc.' always treated as abbreviation, known heuristic limitation), got %d: %+v", len(got), got)
	}
}

func TestSplitSentencesFRDoesNotMatchOrdinaryWordsEndingLikeAbbreviations(t *testing.T) {
	// Régression : endsWithAbbreviation utilisait strings.HasSuffix sur la chaîne entière,
	// donc "trop" (se terminant par "p", l'abréviation de "page"), "loup" et "match" (se
	// terminant par "ch", l'abréviation de "chapitre") étaient à tort traités comme des
	// abréviations, fusionnant la phrase suivante au lieu de la séparer.
	got := splitSentencesFR("Il en fit trop. Le loup rôdait dans la forêt sombre. Elle rentra vite.")
	if len(got) != 3 {
		t.Fatalf("expected 3 sentences ('trop.' and 'loup.' are real sentence ends, not abbreviations), got %d: %+v", len(got), got)
	}
	if strings.TrimSpace(got[0]) != "Il en fit trop." {
		t.Fatalf("expected first sentence %q, got %q", "Il en fit trop.", strings.TrimSpace(got[0]))
	}

	got = splitSentencesFR("Quel beau match. Il rentra chez lui.")
	if len(got) != 2 {
		t.Fatalf("expected 2 sentences ('match.' ends in 'ch' but is not the abbreviation 'ch.'), got %d: %+v", len(got), got)
	}

	got = splitSentencesFR("Il a fait beaucoup. Elle a fait sa part. La riche idée.")
	if len(got) != 3 {
		t.Fatalf("expected 3 sentences ('beaucoup.', 'part.' are not abbreviations), got %d: %+v", len(got), got)
	}

	// Les vraies abréviations doivent continuer à ne PAS terminer la phrase.
	got = splitSentencesFR("M. Dupont est arrivé. Voir ch. 3 pour la suite.")
	if len(got) != 2 {
		t.Fatalf("expected 2 sentences (real abbreviations 'M.' and 'ch.' must still not split), got %d: %+v", len(got), got)
	}
}

func TestSentenceStatsUsesRobustSplit(t *testing.T) {
	// Avant la correction, "M. Dupont est arrivé. Il a souri." aurait été compté comme 3
	// phrases (split naïf sur chaque point) au lieu de 2 — vérifie que sentenceStats
	// utilise bien le nouveau découpage.
	mean, _ := sentenceStats("M. Dupont est arrivé. Il a souri à tout le monde présent.")
	// 2 phrases : "M. Dupont est arrivé" (4 mots) + "Il a souri à tout le monde présent" (8 mots)
	// moyenne = 6
	if mean != 6 {
		t.Fatalf("sentenceStats mean = %v, want 6 (2 sentences via robust split, not 3 via naive split)", mean)
	}
}

func TestSyllableCountFR(t *testing.T) {
	cases := []struct {
		word string
		want int
	}{
		{"chat", 1},     // une voyelle
		{"maison", 2},   // ai + o
		{"éléphant", 3}, // é, é, a (an compte comme un groupe)
		{"table", 1},    // e muet final ne compte pas (a, puis e final exclu)
		{"vie", 1},      // i, e final ne compte pas car mot a déjà une voyelle
	}
	for _, c := range cases {
		if got := syllableCountFR(c.word); got != c.want {
			t.Errorf("syllableCountFR(%q) = %d, want %d", c.word, got, c.want)
		}
	}
}

func TestKandelMolesScore(t *testing.T) {
	// Score = 207 - 1.015*(mots/phrases) - 73.6*(syllabes/mots)
	// Exemple : 10 mots/phrase, 2 syllabes/mot en moyenne
	// = 207 - 1.015*10 - 73.6*2 = 207 - 10.15 - 147.2 = 49.65
	got := kandelMolesScore(10, 2)
	want := 49.65
	if diff := got - want; diff > 0.01 || diff < -0.01 {
		t.Fatalf("kandelMolesScore(10, 2) = %v, want ~%v", got, want)
	}
}

func TestReadabilityLabel(t *testing.T) {
	cases := []struct {
		score float64
		want  string
	}{
		{95, "Très facile"},
		{75, "Facile"},
		{65, "Assez facile"},
		{55, "Moyen"},
		{45, "Assez difficile"},
		{35, "Difficile"},
		{10, "Très difficile"},
		// bornes exactes
		{80, "Très facile"},
		{79.9, "Facile"},
	}
	for _, c := range cases {
		if got := readabilityLabel(c.score); got != c.want {
			t.Errorf("readabilityLabel(%v) = %q, want %q", c.score, got, c.want)
		}
	}
}

func TestComputeDocStatsIncludesReadabilityScore(t *testing.T) {
	ds := computeDocStats("Le chat mange la souris. Le chien dort près du feu chaud.")
	if ds.readabilityScore <= 0 || ds.readabilityScore > 100 {
		t.Fatalf("readabilityScore = %v, want a value clamped to (0, 100]", ds.readabilityScore)
	}
	if ds.readabilityLabel == "" {
		t.Fatal("readabilityLabel should not be empty for non-trivial text")
	}
}

func TestComputeDocStatsSyllablesPerWordUsesConsistentTokenBase(t *testing.T) {
	// Régression : le numérateur (syllabes totales) était sommé sur les tokens de
	// wordTokenRe ([\p{L}']+, qui découpe "peut-être" en 2 tokens : "peut" et "être"), mais
	// le dénominateur utilisait ds.words (strings.Fields, qui compte "peut-être" comme 1 seul
	// mot). Sur un texte avec des mots composés à tiret, cette incohérence de base gonflait
	// syllablesPerWord et faussait le score de lisibilité.
	//
	// Le score doit être calculé comme s'il utilisait directement les tokens de wordTokenRe
	// au numérateur ET au dénominateur — on le recalcule ici indépendamment et on compare.
	text := "En 1958, Kandel-Moles publièrent leur formule. C'est-à-dire il y a longtemps déjà."
	ds := computeDocStats(text)

	sentences := splitSentencesFR(text)
	tokens := wordTokenRe.FindAllString(text, -1)
	totalSyllables := 0
	for _, w := range tokens {
		totalSyllables += syllableCountFR(w)
	}
	wantWordsPerSentence := float64(ds.words) / float64(len(sentences))
	wantSyllablesPerWord := float64(totalSyllables) / float64(len(tokens))
	wantScore := clampScore(kandelMolesScore(wantWordsPerSentence, wantSyllablesPerWord))

	if diff := ds.readabilityScore - wantScore; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("readabilityScore = %v, want %v (syllablesPerWord must divide by len(wordTokenRe tokens), not ds.words)", ds.readabilityScore, wantScore)
	}

	// ds.words itself (the user-facing word count) must remain the strings.Fields count —
	// only the internal syllables/word ratio changes base, not the displayed word count.
	if ds.words != wordCount(text) {
		t.Fatalf("ds.words = %d, want %d (wordCount/strings.Fields — must not change)", ds.words, wordCount(text))
	}
}

func TestRenderNotesSectionEmpty(t *testing.T) {
	out := renderNotesSection(nil, 28)
	if !strings.Contains(out, "NOTES") {
		t.Fatalf("empty notes section missing header:\n%s", out)
	}
	if !strings.Contains(out, "aucune note") {
		t.Fatalf("empty notes section missing placeholder:\n%s", out)
	}
}

func TestRenderNotesSectionSingleLineNote(t *testing.T) {
	out := renderNotesSection([]note{{ID: "n1", Text: "Vérifier la continuité du prénom"}}, 40)
	if !strings.Contains(out, "Vérifier la continuité du prénom") {
		t.Fatalf("notes section missing note text:\n%s", out)
	}
	if strings.Contains(out, "aucune note") {
		t.Fatalf("notes section should not show the empty placeholder when notes exist:\n%s", out)
	}
}

func TestRenderNotesSectionMultiLineNoteShowsFirstLineOnly(t *testing.T) {
	out := renderNotesSection([]note{{ID: "n1", Text: "Rythme à retravailler\nvoir chapitre 3"}}, 40)
	if !strings.Contains(out, "Rythme à retravailler …") {
		t.Fatalf("multi-line note should show first line + ellipsis marker:\n%s", out)
	}
	if strings.Contains(out, "voir chapitre 3") {
		t.Fatalf("multi-line note should NOT show the second line:\n%s", out)
	}
}

func TestRenderNotesSectionTruncatesToWidth(t *testing.T) {
	long := strings.Repeat("mot ", 30) // far longer than any reasonable inspector width
	out := renderNotesSection([]note{{ID: "n1", Text: long}}, 20)
	for _, line := range strings.Split(out, "\n") {
		if lipgloss.Width(line) > 20 {
			t.Fatalf("line exceeds width 20: %q (width %d)", line, lipgloss.Width(line))
		}
	}
}

func TestRenderNotesSectionListsMultipleNotesInOrder(t *testing.T) {
	out := renderNotesSection([]note{
		{ID: "n1", Text: "Première note"},
		{ID: "n2", Text: "Deuxième note"},
	}, 40)
	i1 := strings.Index(out, "Première note")
	i2 := strings.Index(out, "Deuxième note")
	if i1 == -1 || i2 == -1 {
		t.Fatalf("both notes should appear:\n%s", out)
	}
	if i1 > i2 {
		t.Fatalf("notes should appear in storage order (first note first):\n%s", out)
	}
}

func TestInspectorViewShowsNotesWhenPresent(t *testing.T) {
	in := inspectorModel{visible: true}
	notes := []note{{ID: "n1", Text: "Continuité à vérifier"}}
	out := in.View(28, docStats{words: 10}, projStats{words: 10}, "", goalStats{}, analysisState{}, notes)
	if !strings.Contains(out, "Continuité à vérifier") {
		t.Fatalf("inspector Mots tab should show note text when notes exist:\n%s", out)
	}
}

func TestInspectorViewShowsPlaceholderWhenNoNotes(t *testing.T) {
	in := inspectorModel{visible: true}
	out := in.View(28, docStats{words: 10}, projStats{words: 10}, "", goalStats{}, analysisState{}, nil)
	if !strings.Contains(out, "aucune note") {
		t.Fatalf("inspector Mots tab should show the empty-notes placeholder when a file is open but has no notes:\n%s", out)
	}
}

func TestInspectorViewOmitsNotesSectionWhenNoFileOpen(t *testing.T) {
	in := inspectorModel{visible: true}
	out := in.View(28, docStats{}, projStats{}, "", goalStats{}, analysisState{}, nil)
	if strings.Contains(out, "NOTES") {
		t.Fatalf("inspector Mots tab should omit the Notes section entirely when no file is open (doc.words == 0):\n%s", out)
	}
}
