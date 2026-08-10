# Citation du jour — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Afficher une citation différente chaque jour, sous le logo du hub, tirée d'un cycle fixe
de 38 citations sur l'écriture et la persévérance, calculée à partir de la date du premier
lancement d'okashi.

**Architecture:** Les 38 citations vivent dans `quotes.json`, embarqué via `go:embed` et parsé par
`loadQuotes()` dans un nouveau fichier `quotes.go`. Une fonction pure `quoteIndexForToday` (aussi
dans `quotes.go`) calcule l'index du jour à partir d'un `userConfig.FirstLaunch` persisté (nouveau
champ dans `settings.go`), avec repli sur `time.Now().YearDay() % n` si aucun répertoire de config
utilisateur n'est disponible. `initialModel()` (`main.go`) résout la citation une fois au démarrage
et la stocke dans `model.todayQuote`; `homeContent()` (`home.go`) la rend, centrée, sous le logo.

**Tech Stack:** Go 1.25, `embed`, `encoding/json`, `time`, `github.com/charmbracelet/lipgloss`.

## Global Constraints

- Le module est `okashi`, `package main` à plat à la racine — tous les nouveaux fichiers Go
  (`quotes.go`, `quotes_test.go`) vivent à la racine, pas dans un sous-package.
- `go` n'est pas nécessairement sur PATH sur toutes les machines de dev — si `go build`/`go test`
  échouent avec "command not found", réessayer avec `/opt/homebrew/bin/go`.
- Écritures de fichiers : toujours atomiques via `atomicWrite` (déjà utilisé par
  `saveUserConfig` — ne pas contourner).
- Le texte utilisateur-visible est en français (guillemets « », tiret cadratin —).
- `View()` doit rester O(visible) — la citation est résolue une fois à `initialModel()`, jamais
  recalculée dans `homeContent()`/`homeView()`.
- L'ordre des 38 citations dans `quotes.json` DOIT suivre exactement l'annexe du spec
  (`docs/superpowers/specs/2026-08-10-citation-du-jour-design.md`, section "Annexe : contenu de
  `quotes.json`") — c'est l'ordonnancement thème/longueur validé par l'utilisateur, pas un ordre
  arbitraire.

---

### Task 1: `quotes.json` + `loadQuotes()`

**Files:**
- Create: `quotes.json`
- Create: `quotes.go`
- Test: `quotes_test.go`

**Interfaces:**
- Produces: `type quote struct { Text, Author string }`; `func loadQuotes() []quote`

- [ ] **Step 1: Créer `quotes.json` avec les 38 citations dans l'ordre exact du spec**

Copier l'ordre de l'annexe du spec (`docs/superpowers/specs/2026-08-10-citation-du-jour-design.md`,
lignes 150-187) dans ce fichier JSON. Contenu complet (respecter cet ordre — c'est l'ordonnancement
thème/longueur validé, pas un choix libre) :

```json
[
  { "text": "Écrire, c'est une façon de parler sans être interrompu.", "author": "Jules Renard" },
  { "text": "Il n'y a pas de vent favorable pour celui qui ne sait où il va.", "author": "Sénèque" },
  { "text": "Il n'y a pas de grande œuvre qui ne soit le fruit d'une obstination.", "author": "Colette" },
  { "text": "Le succès, c'est se déplacer d'échec en échec sans perdre son enthousiasme.", "author": "Winston Churchill" },
  { "text": "L'inspiration existe, mais il faut qu'elle vous trouve en train de travailler.", "author": "Pablo Picasso" },
  { "text": "La persévérance est un talent tout comme les autres, et peut-être plus rare.", "author": "Louis Pergaud" },
  { "text": "The first draft of anything is shit.", "author": "Ernest Hemingway" },
  { "text": "Tomber sept fois, se relever huit.", "author": "proverbe" },
  { "text": "Un écrivain, c'est quelqu'un pour qui écrire est plus difficile que pour les autres.", "author": "Thomas Mann" },
  { "text": "Ce n'est pas la charge qui vous casse, c'est la façon dont vous la portez.", "author": "Lou Holtz" },
  { "text": "J'écris pour me délivrer, pour ordonner un chaos qui, autrement, resterait obscur.", "author": "Marguerite Yourcenar" },
  { "text": "Il faut beaucoup de patience et un peu d'audace pour aller jusqu'au bout de ce qu'on a commencé.", "author": "" },
  { "text": "Écrire, c'est une façon de vivre deux fois.", "author": "Anaïs Nin" },
  { "text": "On ne subit pas l'avenir, on le fait.", "author": "Georges Bernanos" },
  { "text": "Écris ce que tu ne dois pas oublier.", "author": "Isabel Allende" },
  { "text": "La différence entre l'ordinaire et l'extraordinaire, c'est ce petit extra.", "author": "Jimmy Johnson" },
  { "text": "La page blanche n'existe pas ; il n'y a que des débuts qu'on n'a pas encore osé écrire.", "author": "inspirée d'Anne Hébert" },
  { "text": "Il n'est jamais trop tard pour être ce que tu aurais pu être.", "author": "George Eliot" },
  { "text": "Un livre doit être la hache pour la mer gelée en nous.", "author": "Franz Kafka" },
  { "text": "Continue d'avancer. Ne t'arrête jamais.", "author": "Walt Disney" },
  { "text": "N'attends pas d'être inspiré. Assieds-toi et mets-toi au travail.", "author": "Stephen King" },
  { "text": "Ce n'est pas parce que les choses sont difficiles que nous n'osons pas, c'est parce que nous n'osons pas qu'elles sont difficiles.", "author": "Sénèque" },
  { "text": "Vous pouvez toujours corriger une mauvaise page. Vous ne pouvez rien tirer d'une page blanche.", "author": "Jodi Picoult" },
  { "text": "Beaucoup d'échecs dans la vie sont dus à des gens qui ne réalisaient pas à quel point ils étaient proches du succès quand ils ont abandonné.", "author": "Thomas Edison" },
  { "text": "Le talent, c'est 1% d'inspiration et 99% de transpiration.", "author": "Thomas Edison" },
  { "text": "Notre plus grande gloire n'est pas de ne jamais tomber, mais de nous relever à chaque chute.", "author": "Confucius" },
  { "text": "Un écrivain n'est jamais aussi bon que ses meilleures pages, ni aussi mauvais que ses pires.", "author": "Ernest Hemingway" },
  { "text": "La qualité n'est jamais un accident ; elle est toujours le résultat d'un effort intelligent.", "author": "John Ruskin" },
  { "text": "Un roman, c'est un miroir qu'on promène le long d'un chemin.", "author": "Stendhal" },
  { "text": "Il n'y a pas d'ascenseur pour la réussite, il faut prendre l'escalier.", "author": "Zig Ziglar" },
  { "text": "Écrire, c'est tenter de savoir ce qu'on écrirait si on écrivait — on ne le sait qu'après.", "author": "Marguerite Duras" },
  { "text": "Le voyage de mille lieues commence toujours par un premier pas.", "author": "Lao Tseu" },
  { "text": "L'écriture est la peinture de la voix.", "author": "Voltaire" },
  { "text": "Fais de ton mieux jusqu'à ce que tu saches faire mieux. Alors, quand tu sais mieux, fais mieux.", "author": "Maya Angelou" },
  { "text": "Un livre est un miroir. Si un singe s'y regarde, ce n'est pas l'image d'un apôtre qui apparaît.", "author": "Georg Christoph Lichtenberg" },
  { "text": "Le succès n'est pas final, l'échec n'est pas fatal : c'est le courage de continuer qui compte.", "author": "Winston Churchill" },
  { "text": "Il faut toujours viser la lune, car même en cas d'échec, on atterrit dans les étoiles.", "author": "Oscar Wilde" },
  { "text": "La motivation, c'est ce qui vous permet de commencer. L'habitude, c'est ce qui vous permet de continuer.", "author": "Jim Ryun" }
]
```

Vérifier que ce fichier contient exactement 38 entrées : `python3 -c "import json; print(len(json.load(open('quotes.json'))))"` doit afficher `38`.

- [ ] **Step 2: Écrire le test de `loadQuotes()`**

```go
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
	if last.Author != "Jim Ryun" {
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
```

- [ ] **Step 3: Lancer les tests pour vérifier qu'ils échouent (types/fonctions n'existent pas encore)**

Run: `go test ./... -run TestLoadQuotes -v`
Expected: FAIL avec une erreur de compilation (`undefined: loadQuotes`)

- [ ] **Step 4: Créer `quotes.go` avec le type `quote` et `loadQuotes()`**

```go
package main

import (
	_ "embed"
	"encoding/json"
)

// quote is one entry in the "quote of the day" rotation shown under the hub logo.
type quote struct {
	Text   string `json:"text"`
	Author string `json:"author"`
}

//go:embed quotes.json
var quotesJSON []byte

// loadQuotes parses the embedded quote set. A parse failure (should not happen for an
// embedded, build-time-checked asset) yields a nil slice — the quote line simply doesn't
// render, mirroring the tolerant missing/corrupt → zero value pattern used by loadUserConfig.
func loadQuotes() []quote {
	var qs []quote
	if err := json.Unmarshal(quotesJSON, &qs); err != nil {
		return nil
	}
	return qs
}
```

- [ ] **Step 5: Lancer les tests pour vérifier qu'ils passent**

Run: `go test ./... -run TestLoadQuotes -v`
Expected: PASS (3 tests)

- [ ] **Step 6: Commit**

```bash
git add quotes.json quotes.go quotes_test.go
git commit -m "$(cat <<'EOF'
citation: ajoute les 38 citations embarquées du hub

quotes.json (ordre thème/longueur validé) + loadQuotes() dans
quotes.go, préparant la citation du jour affichée sous le logo.
EOF
)"
```

---

### Task 2: `FirstLaunch` sur `userConfig` + `quoteIndexForToday`

**Files:**
- Modify: `settings.go` (struct `userConfig`, ligne 13-16)
- Modify: `quotes.go`
- Test: `quotes_test.go`

**Interfaces:**
- Consumes: `type userConfig struct { Author, Contact string }` (existant, `settings.go:13`)
- Produces: `userConfig.FirstLaunch string`; `func quoteIndexForToday(uc userConfig, today time.Time, hasConfigDir bool, n int) (index int, needsSave bool)`

- [ ] **Step 1: Écrire les tests de `quoteIndexForToday`**

Ajouter à `quotes_test.go` :

```go
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
```

- [ ] **Step 2: Lancer les tests pour vérifier qu'ils échouent**

Run: `go test ./... -run TestQuoteIndexForToday -v`
Expected: FAIL avec une erreur de compilation (`undefined: quoteIndexForToday`, `unknown field FirstLaunch`)

- [ ] **Step 3: Ajouter `FirstLaunch` à `userConfig` dans `settings.go`**

```go
// userConfig is personal, machine-global identity (stored in the OS user-config dir under
// okashi/config.json — macOS: ~/Library/Application Support/okashi; Linux: ~/.config/okashi —
// alongside recent.json) used on the export title page. Applies to every project.
type userConfig struct {
	Author      string `json:"author,omitempty"`
	Contact     string `json:"contact,omitempty"`
	FirstLaunch string `json:"firstLaunch,omitempty"` // "2006-01-02"; seeds the quote-of-the-day cycle
}
```

- [ ] **Step 4: Implémenter `quoteIndexForToday` dans `quotes.go`**

Ajouter l'import `"time"` en haut de `quotes.go`, puis :

```go
const quoteDateLayout = "2006-01-02"

// quoteIndexForToday resolves which of the n quotes to show today. today is injected for
// testability. hasConfigDir distinguishes "no persistable config" (os.UserConfigDir() failed)
// from "config dir exists but firstLaunch not yet recorded" — only the latter requests a save.
func quoteIndexForToday(uc userConfig, today time.Time, hasConfigDir bool, n int) (index int, needsSave bool) {
	if !hasConfigDir {
		return today.YearDay() % n, false
	}
	first, err := time.Parse(quoteDateLayout, uc.FirstLaunch)
	if uc.FirstLaunch == "" || err != nil {
		return 0, true
	}
	days := int(today.Sub(first).Hours() / 24)
	if days < 0 {
		days = -days
	}
	return days % n, false
}
```

- [ ] **Step 5: Lancer les tests pour vérifier qu'ils passent**

Run: `go test ./... -run TestQuoteIndexForToday -v`
Expected: PASS (6 tests)

- [ ] **Step 6: Lancer toute la suite pour vérifier l'absence de régression**

Run: `go test ./...`
Expected: PASS (aucune régression dans `settings_test.go` ou ailleurs)

- [ ] **Step 7: Commit**

```bash
git add settings.go quotes.go quotes_test.go
git commit -m "$(cat <<'EOF'
citation: calcule l'index du jour depuis le premier lancement

FirstLaunch sur userConfig + quoteIndexForToday : cycle de 38 jours
ancré à la première utilisation d'okashi, repli sur YearDay quand
aucun répertoire de config utilisateur n'est disponible.
EOF
)"
```

---

### Task 3: Résolution au démarrage (`initialModel`) + persistance

**Files:**
- Modify: `main.go` (struct `model`, ligne ~458 ; `initialModel()`, ligne 533-621)
- Test: `main_test.go` (créer un test ciblé si absent d'un fichier existant — vérifier d'abord)

**Interfaces:**
- Consumes: `loadQuotes() []quote`, `quoteIndexForToday(userConfig, time.Time, bool, int) (int, bool)` (Task 1/2), `userConfigPath() string`, `loadUserConfig(string) userConfig`, `saveUserConfig(string, userConfig) error` (existants dans `settings.go`)
- Produces: `model.todayQuote quote`

- [ ] **Step 1: Vérifier l'existence d'un fichier de test pour `initialModel`**

Run: `grep -rn "func TestInitialModel" *.go`

S'il n'existe aucun test de ce nom, créer le test dans un nouveau fichier `quotes_test.go` (déjà
présent depuis Task 1) — pas besoin de nouveau fichier de test.

- [ ] **Step 2: Écrire le test d'intégration démarrage → `model.todayQuote`**

Ajouter à `quotes_test.go` :

```go
func TestInitialModelPopulatesTodayQuote(t *testing.T) {
	m := initialModel()
	if m.todayQuote.Text == "" {
		t.Fatal("initialModel() must populate todayQuote from the embedded quote set")
	}
}
```

- [ ] **Step 3: Lancer le test pour vérifier qu'il échoue**

Run: `go test ./... -run TestInitialModelPopulatesTodayQuote -v`
Expected: FAIL avec une erreur de compilation (`m.todayQuote undefined`)

- [ ] **Step 4: Ajouter le champ `todayQuote` à `model` dans `main.go`**

Dans le bloc de champs résolus une fois au démarrage (à côté de `icons iconSet`, `main.go`, autour
de la ligne 458) :

```go
	icons             iconSet
	todayQuote        quote // resolved once at startup; never recomputed in View()
```

- [ ] **Step 5: Résoudre `todayQuote` dans `initialModel()`**

Dans `main.go`, `initialModel()` (ligne 533-621), juste après la ligne `startupSettings :=
resolveSettings(writingDir())` (ligne 538), ajouter :

```go
	quotes := loadQuotes()
	var todayQuote quote
	if len(quotes) > 0 {
		cfgPath := userConfigPath()
		uc := loadUserConfig(cfgPath)
		idx, needsSave := quoteIndexForToday(uc, time.Now(), cfgPath != "", len(quotes))
		todayQuote = quotes[idx]
		if needsSave {
			uc.FirstLaunch = time.Now().Format(quoteDateLayout)
			_ = saveUserConfig(cfgPath, uc) // best-effort: a write failure just re-seeds day 1 next launch
		}
	}
```

Puis, dans le literal `m := model{...}` (ligne 591), ajouter le champ :

```go
		icons:           resolveIcons(),
		todayQuote:      todayQuote,
```

- [ ] **Step 6: Lancer le test pour vérifier qu'il passe**

Run: `go test ./... -run TestInitialModelPopulatesTodayQuote -v`
Expected: PASS

- [ ] **Step 7: Lancer toute la suite pour vérifier l'absence de régression**

Run: `go test ./...`
Expected: PASS. Vérifier en particulier `TestHomeContentAndHitTest` (`home_test.go:332`) — la
citation ne doit produire aucune `homeCell` cliquable, donc ce test doit rester à 5 cellules sans
modification (voir Task 4).

- [ ] **Step 8: Commit**

```bash
git add main.go quotes_test.go
git commit -m "$(cat <<'EOF'
citation: résout la citation du jour une fois au démarrage

initialModel() peuple model.todayQuote via quoteIndexForToday et
persiste FirstLaunch au premier lancement (best-effort, écriture
atomique existante de saveUserConfig).
EOF
)"
```

---

### Task 4: Rendu dans `homeContent()`

**Files:**
- Modify: `styles.go` (nouveaux styles)
- Modify: `home.go` (`homeContent()`, ligne ~1099-1110)
- Test: `home_test.go`

**Interfaces:**
- Consumes: `model.todayQuote quote` (Task 3), `bannerStyle`/`subtle` (existants, `styles.go`)
- Produces: `quoteStyle`, `quoteAuthorStyle` (styles lipgloss, consommés uniquement par `home.go`)

- [ ] **Step 1: Écrire le test de rendu de la citation dans `homeContent()`**

Ajouter à `home_test.go` :

```go
func TestHomeContentShowsTodayQuote(t *testing.T) {
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = nm.(model)
	m.todayQuote = quote{Text: "Une phrase de test bien identifiable.", Author: "Auteur Test"}

	lines, _, _ := m.homeContent()
	joined := strings.Join(lines, "\n")
	if !strings.Contains(ansi.Strip(joined), "Une phrase de test bien identifiable.") {
		t.Fatal("homeContent() should render todayQuote.Text under the logo")
	}
	if !strings.Contains(ansi.Strip(joined), "Auteur Test") {
		t.Fatal("homeContent() should render todayQuote.Author under the quote")
	}
}

func TestHomeContentHitTestUnaffectedByQuote(t *testing.T) {
	t.Setenv("OKASHI_ICONS", "plain")
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 40})
	m = nm.(model)
	m.todayQuote = quote{Text: "Citation de test.", Author: "Test"}
	m.homeItems = []homeItem{
		{kind: homeProject, label: "novel", path: "/p/novel"},
		{kind: homeRecentFile, label: "ch.md", path: "/r/ch.md"},
		{kind: homeNewDocument, label: "New document"},
	}
	m.resetHomeSelection()

	_, cells, _ := m.homeContent()
	// Same 5 cells as without a quote — the quote line adds no clickable cell.
	if len(cells) != 5 {
		t.Fatalf("want 5 cells (quote must not add hit-test cells), got %d", len(cells))
	}
}
```

- [ ] **Step 2: Lancer les tests pour vérifier qu'ils échouent**

Run: `go test ./... -run TestHomeContentShowsTodayQuote -v`
Expected: FAIL (la citation n'est pas encore rendue — `strings.Contains` retourne false)

- [ ] **Step 3: Ajouter les styles dans `styles.go`**

Après `var breadcrumbStyle = ...` (ligne 43-45), ajouter :

```go
var quoteStyle = lipgloss.NewStyle().
	Foreground(subtle).
	Italic(true)

var quoteAuthorStyle = lipgloss.NewStyle().
	Foreground(subtle)
```

- [ ] **Step 4: Rendre la citation dans `homeContent()`**

Dans `home.go`, remplacer le bloc autour de la ligne 1107-1110 :

```go
	for _, l := range strings.Split(bannerArt, "\n") {
		lines = append(lines, pad(bannerStyle.Render(l)))
	}
	lines = append(lines, "")
```

par :

```go
	for _, l := range strings.Split(bannerArt, "\n") {
		lines = append(lines, pad(bannerStyle.Render(l)))
	}
	if m.todayQuote.Text != "" {
		q := quoteStyle.Width(blockW).Align(lipgloss.Center).Render("« " + m.todayQuote.Text + " »")
		lines = append(lines, strings.Split(q, "\n")...)
		if m.todayQuote.Author != "" {
			a := quoteAuthorStyle.Width(blockW).Align(lipgloss.Center).Render("— " + m.todayQuote.Author)
			lines = append(lines, a)
		}
	}
	lines = append(lines, "")
```

Note : `blockW` est déjà calculé plus haut dans `homeContent()` (ligne 1089-1097), avant ce bloc —
vérifier que l'ordre des opérations dans le fichier le permet toujours (c'est déjà le cas, le bloc
logo suit immédiatement le calcul de `blockW`).

- [ ] **Step 5: Lancer les tests pour vérifier qu'ils passent**

Run: `go test ./... -run TestHomeContent -v`
Expected: PASS (`TestHomeContentShowsTodayQuote`, `TestHomeContentHitTestUnaffectedByQuote`,
`TestHomeContentAndHitTest` tous PASS)

- [ ] **Step 6: Lancer toute la suite**

Run: `go test ./...`
Expected: PASS, aucune régression.

- [ ] **Step 7: Vérifier visuellement dans l'app réelle**

Run: `go run .` (ou `/opt/homebrew/bin/go run .` si `go` n'est pas sur PATH), observer le hub :
la citation doit apparaître centrée sous le logo « o k a s h i », en italique discret, avec
l'auteur sur la ligne suivante précédé d'un tiret cadratin. Quitter avec `ctrl+c` ou `q` depuis le
hub.

- [ ] **Step 8: Commit**

```bash
git add styles.go home.go home_test.go
git commit -m "$(cat <<'EOF'
citation: affiche la citation du jour sous le logo du hub

quoteStyle/quoteAuthorStyle (italique discret, tiret cadratin) et
rendu centré dans homeContent(), sans ajouter de cellule cliquable
(le hit-test du hub reste inchangé).
EOF
)"
```

---

## Self-Review Notes

- **Couverture du spec** : §1 (stockage) → Task 1 ; §2 (calcul d'index + persistance) → Task 2 +
  Task 3 ; §3 (rendu) → Task 4 ; Annexe (contenu exact) → Task 1 Step 1. Le point "Hors périmètre"
  du spec (pas de rotation manuelle, pas de config on/off, cycle 38 ≠ mois calendaire) ne demande
  aucune tâche — c'est explicitement ce qu'on ne construit pas.
- **Cohérence des types** : `quote{Text, Author string}` (Task 1) est le seul type utilisé tel
  quel dans Task 3 (`model.todayQuote quote`) et Task 4 (`m.todayQuote.Text`,
  `m.todayQuote.Author`) — aucune divergence de nom de champ.
- **`hasConfigDir`** : Task 3 passe `cfgPath != ""` comme `hasConfigDir` à `quoteIndexForToday` —
  cohérent avec `userConfigPath()` qui retourne `""` en cas d'échec de `os.UserConfigDir()`
  (`settings.go:36-42`), donc pas besoin d'un appel séparé à `os.UserConfigDir()`.
