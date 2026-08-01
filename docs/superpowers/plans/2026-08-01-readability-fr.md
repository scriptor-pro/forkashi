# Lisibilité adaptée au français Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Adapter le panneau READABILITY de l'inspecteur (`inspector.go`) au français : découpage en phrases robuste aux abréviations, vitesse de lecture recalibrée, et ajout du score de lisibilité Kandel-Moles en complément de la mesure existante.

**Architecture:** Trois modifications indépendantes et cumulables dans `inspector.go` et son fichier de test : une fonction de découpage en phrases plus robuste (remplace le split naïf), une constante ajustée, et une nouvelle fonction de calcul de score branchée dans `docStats`/le rendu du panneau Readability.

**Tech Stack:** Go 1.25, stdlib uniquement (`regexp`, `strings`, `math`, `unicode`) — aucune nouvelle dépendance.

## Global Constraints

- Le module Go reste `okashi` dans `go.mod` — ne pas le modifier.
- Chaque tâche doit se terminer avec `go build ./...` et `go vet ./...` propres, et `go test ./...` vert. Utiliser Go 1.25+ (`export PATH="$HOME/.local/go/bin:$PATH"` si le `go` système est plus ancien — le `go` par défaut sur cette machine est 1.19).
- La formule Kandel-Moles est exactement : `Score = 207 − 1,015 × (mots/phrases) − 73,6 × (syllabes/mots)` — constante **207**, pas 209 (vérifiée sur plusieurs sources indépendantes lors du design).
- La vitesse de lecture française est **210 mots/minute** — une estimation moins rigoureusement sourcée que l'ancienne valeur anglophone (238, méta-analyse Brysbaert 2019) ; documenter ce fait dans le commentaire du code, pas comme une vérité aussi établie.
- La mesure existante (moyenne±écart-type de longueur de phrase, `sentMean`/`sentStdDev`) reste affichée telle quelle — le score Kandel-Moles s'ajoute, ne la remplace pas.
- L'échelle d'étiquette du score est fixe à 7 paliers (voir Tâche 3) — ne pas en inventer une différente.
- Commits fréquents, un commit par tâche minimum.

---

## File Structure

| Fichier | Statut | Responsabilité |
|---|---|---|
| `inspector.go` | Modifier | `sentenceStats` utilise le nouveau découpage ; constante de vitesse de lecture ajustée ; nouvelle fonction `syllableCount`/`kandelMolesScore`/`readabilityLabel` ; rendu du panneau Readability étendu |
| `inspector_test.go` | Modifier | Tests du nouveau découpage de phrases, de la formule Kandel-Moles, du comptage de syllabes, et des étiquettes |

Une seule tâche : les trois axes du spec (découpage phrases, vitesse lecture, score Kandel-Moles) touchent le même fichier de ~40 lignes de logique métier concentrées, sans interface intermédiaire à stabiliser entre eux — les séparer en plusieurs tâches ajouterait de la coordination sans bénéfice (voir Task Right-Sizing : « fold setup... into the task whose deliverable needs them »). Une seule tâche avec plusieurs étapes de test/implémentation séquentielles est le bon grain ici.

---

## Task 1: Découpage en phrases, vitesse de lecture, et score Kandel-Moles

**Files:**
- Modify: `inspector.go:143-144` (docStats struct), `inspector.go:262-294` (computeDocStats, sentenceSplitRe, sentenceStats), `inspector.go:470-471` (rendu Readability)
- Test: `inspector_test.go`

**Interfaces:**
- Consumes: `docStats` struct existante (`inspector.go:141-146`), `kvStrRow(label, val string, width int) string` (`inspector.go:366`, inchangée), `sectionHeader(label string, width int) string` (`inspector.go:18`, inchangée), `wordCount(s string) int` (`main.go:2421`, inchangée).
- Produces: `docStats.readabilityScore float64` et `docStats.readabilityLabel string` (nouveaux champs) ; `func splitSentencesFR(text string) []string` (nouvelle fonction, remplace l'usage direct de `sentenceSplitRe.Split`) ; `func syllableCountFR(word string) int` ; `func kandelMolesScore(wordsPerSentence, syllablesPerWord float64) float64` ; `func readabilityLabel(score float64) string`.

- [ ] **Step 1: Écrire les tests du découpage en phrases robuste**

Dans `inspector_test.go`, ajouter :

```go
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

	// Une ellipse (trois points ou caractère unique) est UN SEUL séparateur, pas trois.
	got = splitSentencesFR("Il hésita... puis se décida. Ce fut rapide.")
	if len(got) != 2 {
		t.Fatalf("expected 2 sentences with '...' as one separator, got %d: %+v", len(got), got)
	}

	got = splitSentencesFR("Il hésita… puis se décida. Ce fut rapide.")
	if len(got) != 2 {
		t.Fatalf("expected 2 sentences with '…' as one separator, got %d: %+v", len(got), got)
	}

	// Une vraie fin de phrase après une abréviation compte bien comme fin de phrase.
	got = splitSentencesFR("Il a tout vendu : meubles, livres, etc. Puis il est parti.")
	if len(got) != 2 {
		t.Fatalf("expected 2 sentences ('etc.' followed by new sentence), got %d: %+v", len(got), got)
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
```

- [ ] **Step 2: Lancer les tests pour vérifier qu'ils échouent**

Run: `export PATH="$HOME/.local/go/bin:$PATH" && go test ./... -run "TestSplitSentencesFR|TestSentenceStatsUsesRobustSplit" -v`
Expected: `TestSplitSentencesFR` FAIL avec `undefined: splitSentencesFR`. `TestSentenceStatsUsesRobustSplit` FAIL (mean actuel sera différent de 6, car `sentenceStats` utilise encore l'ancien split naïf qui casse sur "M." et "Dupont est arrivé.").

- [ ] **Step 3: Implémenter splitSentencesFR et rebrancher sentenceStats**

Dans `inspector.go`, remplacer la variable et le commentaire existants :

```go
var sentenceSplitRe = regexp.MustCompile(`[.!?]+`)

// sentenceStats returns the mean and population standard deviation of sentence length (in
// words), splitting on runs of . ! ? — an approximation (abbreviations end a "sentence"),
// but a cheap, useful signal for prose rhythm.
func sentenceStats(text string) (mean, std float64) {
	var lens []float64
	for _, s := range sentenceSplitRe.Split(text, -1) {
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
```

par :

```go
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
func endsWithAbbreviation(s string) bool {
	trimmed := strings.TrimRight(s, " \t")
	for _, abbr := range frAbbreviations {
		if strings.HasSuffix(trimmed, abbr) {
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
```

- [ ] **Step 4: Lancer les tests pour vérifier qu'ils passent**

Run: `export PATH="$HOME/.local/go/bin:$PATH" && go test ./... -run "TestSplitSentencesFR|TestSentenceStatsUsesRobustSplit" -v`
Expected: PASS pour les deux tests.

- [ ] **Step 5: Écrire les tests du comptage de syllabes et du score Kandel-Moles**

Dans `inspector_test.go`, ajouter :

```go
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
```

- [ ] **Step 6: Lancer les tests pour vérifier qu'ils échouent**

Run: `export PATH="$HOME/.local/go/bin:$PATH" && go test ./... -run "TestSyllableCountFR|TestKandelMolesScore|TestReadabilityLabel|TestComputeDocStatsIncludesReadabilityScore" -v`
Expected: FAIL avec `undefined: syllableCountFR`, `undefined: kandelMolesScore`, `undefined: readabilityLabel`, et `ds.readabilityScore undefined (type docStats has no field or method readabilityScore)`.

- [ ] **Step 7: Implémenter syllableCountFR, kandelMolesScore, readabilityLabel**

Dans `inspector.go`, ajouter après `sentenceStats` (avant `var wordTokenRe`) :

```go
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
```

- [ ] **Step 8: Lancer les tests pour vérifier qu'ils passent**

Run: `export PATH="$HOME/.local/go/bin:$PATH" && go test ./... -run "TestSyllableCountFR|TestKandelMolesScore|TestReadabilityLabel" -v`
Expected: PASS pour `TestSyllableCountFR` et `TestKandelMolesScore`. `TestReadabilityLabel` doit aussi PASS. `TestComputeDocStatsIncludesReadabilityScore` FAIL encore (le champ `docStats.readabilityScore` n'existe pas encore) — normal à ce stade, corrigé au Step suivant.

- [ ] **Step 9: Ajuster la vitesse de lecture, ajouter les champs docStats, et brancher le calcul dans computeDocStats**

Dans `inspector.go`, remplacer le struct `docStats` :

```go
type docStats struct {
	words, chars, paragraphs int
	readSecs                 int        // estimated reading time at 238 wpm
	sentMean, sentStdDev     float64    // sentence length in words (mean ± population stddev)
	overused                 []wordFreq // top repeated content words
}
```

par :

```go
type docStats struct {
	words, chars, paragraphs int
	readSecs                 int     // estimated reading time at 210 wpm (French silent-reading estimate)
	sentMean, sentStdDev     float64 // sentence length in words (mean ± population stddev)
	readabilityScore         float64 // Kandel-Moles score, clamped to [0, 100]; 0 when doc has no sentences
	readabilityLabel         string  // 7-tier label for readabilityScore ("" when no sentences)
	overused                 []wordFreq // top repeated content words
}
```

Puis remplacer `computeDocStats` :

```go
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
	ds.readSecs = ds.words * 60 / 238 // ~238 wpm silent adult reading
	ds.sentMean, ds.sentStdDev = sentenceStats(text)
	ds.overused = overusedWords(text, 5)
	return ds
}
```

par :

```go
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
		totalSyllables := 0
		for _, w := range wordTokenRe.FindAllString(text, -1) {
			totalSyllables += syllableCountFR(w)
		}
		wordsPerSentence := float64(ds.words) / float64(len(sentences))
		syllablesPerWord := float64(totalSyllables) / float64(ds.words)
		ds.readabilityScore = clampScore(kandelMolesScore(wordsPerSentence, syllablesPerWord))
		ds.readabilityLabel = readabilityLabel(ds.readabilityScore)
	}
	ds.overused = overusedWords(text, 5)
	return ds
}
```

`wordTokenRe` (défini plus bas dans le fichier, `inspector.go:296`, pattern `[\p{L}']+`) est réutilisé tel quel plutôt que de recompiler une regex identique localement — c'est une variable de package, donc accessible depuis `computeDocStats` indépendamment de l'ordre des déclarations dans le fichier (l'ordre des déclarations top-level n'a pas d'importance en Go).

- [ ] **Step 10: Lancer tous les tests de cette tâche pour vérifier qu'ils passent**

Run: `export PATH="$HOME/.local/go/bin:$PATH" && go test ./... -run "TestSplitSentencesFR|TestSentenceStatsUsesRobustSplit|TestSyllableCountFR|TestKandelMolesScore|TestReadabilityLabel|TestComputeDocStatsIncludesReadabilityScore|TestComputeDocStats$" -v`
Expected: PASS pour tous, y compris `TestComputeDocStats` (test préexistant, doit continuer à passer sans changement de comportement sur `words`/`chars`/`paragraphs`).

- [ ] **Step 11: Ajouter la ligne de score dans le rendu du panneau Readability**

Dans `inspector.go`, remplacer :

```go
		if doc.words > 0 {
			b.WriteString("\n\n" + sectionHeader("Readability", width) + "\n")
			b.WriteString("  " + kvStrRow("Reading time", fmtReadTime(doc.readSecs), width-2) + "\n")
			b.WriteString("  " + kvStrRow("Avg sentence", fmt.Sprintf("%.0f±%.0f wd", doc.sentMean, doc.sentStdDev), width-2))
```

par :

```go
		if doc.words > 0 {
			b.WriteString("\n\n" + sectionHeader("Readability", width) + "\n")
			b.WriteString("  " + kvStrRow("Reading time", fmtReadTime(doc.readSecs), width-2) + "\n")
			b.WriteString("  " + kvStrRow("Avg sentence", fmt.Sprintf("%.0f±%.0f wd", doc.sentMean, doc.sentStdDev), width-2))
			if doc.readabilityLabel != "" {
				score := fmt.Sprintf("%.0f · %s", doc.readabilityScore, doc.readabilityLabel)
				b.WriteString("\n  " + kvStrRow("Score", score, width-2))
			}
```

(le reste de la fonction, à partir de `if len(doc.overused) > 0 {`, reste inchangé — cette insertion se place juste avant.)

- [ ] **Step 12: Vérifier le rendu visuellement via un test de smoke existant si applicable**

```bash
grep -n "Readability\|Avg sentence" smoke_test.go 2>&1
```

Si un test dans `smoke_test.go` fait des assertions sur le contenu exact du panneau Readability (recherche de texte comme "Reading time" ou "Avg sentence" dans une sortie de rendu), le lire en entier et vérifier qu'il continue de passer après l'ajout de la ligne "Score" (l'ajout ne devrait rien casser puisqu'il s'agit d'une ligne supplémentaire, pas d'une modification des lignes existantes) — sinon, passer à l'étape suivante sans modification.

- [ ] **Step 13: Build, vet, et suite complète**

Run: `export PATH="$HOME/.local/go/bin:$PATH" && go build ./... && go vet ./... && go test ./... -count=1`
Expected: build et vet propres, tous les tests PASS (aucune régression sur les 481 tests existants avant cette tâche).

- [ ] **Step 14: Commit**

```bash
git add inspector.go inspector_test.go
git commit -m "$(cat <<'EOF'
Adapte le calcul de lisibilité au français

- Découpage en phrases robuste aux abréviations françaises courantes
  (M., etc., p., ...) et aux ellipses (traitées comme un seul séparateur
  au lieu d'un par point) : splitSentencesFR remplace le split naïf
  sur [.!?]+.
- Vitesse de lecture ajustée de 238 à 210 mots/minute (estimation
  française, moins rigoureusement sourcée que la valeur anglophone
  d'origine — voir commentaire dans le code).
- Ajoute le score de lisibilité Kandel-Moles (1958, adaptation
  française de Flesch Reading Ease) en complément de la mesure
  moyenne±écart-type existante, avec étiquette sur l'échelle standard
  à 7 paliers (Très facile → Très difficile).

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

## Self-Review

**Couverture du spec** :
- §1 (découpage en phrases) : Steps 1-4.
- §2 (vitesse de lecture 210 mpm) : Step 9.
- §3 (score Kandel-Moles + étiquette 7 paliers) : Steps 5-8, 11.
- Hors périmètre (§ Hors périmètre du spec) : aucune tâche n'implémente de tokenizer complet, de calibration par genre, ni de recherche supplémentaire sur la vitesse de lecture — conforme.

**Cohérence des types** : `docStats.readabilityScore float64` et `docStats.readabilityLabel string` (Step 9) correspondent exactement à ce que Step 5 teste (`ds.readabilityScore`, `ds.readabilityLabel`) et à ce que Step 11 affiche. `kandelMolesScore(wordsPerSentence, syllablesPerWord float64) float64` a la même signature dans son test (Step 5) et son implémentation (Step 7). `splitSentencesFR(text string) []string` cohérent entre Step 1 (tests), Step 3 (implémentation), et son usage dans `sentenceStats`/`computeDocStats` (Step 3, Step 9).
