# Numéro de version sur le hub — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Afficher le numéro de version d'okashi (`main.version`) sur la même ligne que le logo
« o k a s h i » du hub, décalé de deux espaces, dans un style discret — sans toucher la 2e ligne
du logo (la règle `───────────`).

**Architecture:** `homeContent()` (`home.go`) construit déjà le rendu du logo en itérant sur les
lignes de `bannerArt` (2 lignes : le mot-symbole, puis une règle). Cette tâche modifie uniquement
le traitement de la 1re ligne pour y concaténer le numéro de version stylé avant le centrage — la
variable globale `version` (`main.go:30`, déjà overridable via `-ldflags`) est lue directement,
aucun nouveau champ sur `model`, aucun nouveau mécanisme d'injection.

**Tech Stack:** Go 1.25, `github.com/charmbracelet/lipgloss`.

## Global Constraints

- Le module est `okashi`, `package main` à plat à la racine.
- `go` n'est pas nécessairement sur PATH — si `go build`/`go test` échouent avec "command not
  found", réessayer avec `/opt/homebrew/bin/go`.
- La variable `version` (`main.go:30`) reste la seule source de vérité pour le numéro de version —
  ne pas créer de champ `model.version` ni de nouveau mécanisme d'injection.
- Seule la 1re ligne de `bannerArt` (le mot-symbole) reçoit le numéro de version ; la 2e ligne (la
  règle) est rendue sans modification.
- Séparateur : exactement deux espaces littéraux entre le logo et le numéro de version.
- Préfixe `v` devant le numéro seulement si `version != "dev"` — le cas `"dev"` s'affiche tel quel,
  sans préfixe.

---

### Task 1: Afficher la version à côté du logo dans `homeContent()`

**Files:**
- Modify: `styles.go` (ajout d'un nouveau style, à côté de `bannerStyle`, ligne 26-28)
- Modify: `home.go` (`homeContent()`, lignes 1107-1109)
- Test: `home_test.go`

**Interfaces:**
- Consumes: `var version string` (existant, `main.go:30`, package-level dans `main`) ;
  `var bannerStyle lipgloss.Style`, `const bannerArt string` (existants, `styles.go`)
- Produces: `var versionStyle lipgloss.Style` (nouveau, `styles.go`) — consommé uniquement par
  `home.go`

- [ ] **Step 1: Écrire le test du rendu de la version sur la ligne du logo**

Ajouter à `home_test.go` (le fichier importe déjà `strings`, `github.com/charmbracelet/x/ansi`, et
`tea "github.com/charmbracelet/bubbletea"` — vérifier ces imports sont déjà présents avant
d'ajouter le test, ne pas les redéclarer) :

```go
func TestHomeContentShowsVersionOnLogoLine(t *testing.T) {
	oldVersion := version
	version = "0.1.126"
	defer func() { version = oldVersion }()

	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = nm.(model)

	lines, _, _ := m.homeContent()
	if len(lines) == 0 {
		t.Fatal("homeContent() returned no lines")
	}
	logoLine := ansi.Strip(lines[0])
	if !strings.Contains(logoLine, "o k a s h i") {
		t.Fatalf("first line should contain the logo, got %q", logoLine)
	}
	if !strings.Contains(logoLine, "v0.1.126") {
		t.Fatalf("first line should contain the version with a 'v' prefix, got %q", logoLine)
	}

	// The rule line (line 1, 0-indexed) must stay unmodified — no version text bleeding into it.
	ruleLine := ansi.Strip(lines[1])
	if strings.Contains(ruleLine, "0.1.126") {
		t.Fatalf("rule line should not contain the version, got %q", ruleLine)
	}
}

func TestHomeContentShowsDevVersionWithoutPrefix(t *testing.T) {
	oldVersion := version
	version = "dev"
	defer func() { version = oldVersion }()

	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = nm.(model)

	lines, _, _ := m.homeContent()
	logoLine := ansi.Strip(lines[0])
	if !strings.Contains(logoLine, "dev") {
		t.Fatalf("first line should contain 'dev', got %q", logoLine)
	}
	if strings.Contains(logoLine, "vdev") {
		t.Fatalf("dev version must not get a 'v' prefix, got %q", logoLine)
	}
}
```

- [ ] **Step 2: Lancer les tests pour vérifier qu'ils échouent**

Run: `go test ./... -run TestHomeContentShowsVersion -v` puis
`go test ./... -run TestHomeContentShowsDevVersion -v`

Expected: les deux échouent — la ligne du logo ne contient pas encore le numéro de version (le
premier test échoue sur `"first line should contain the version"`, le second sur
`"first line should contain 'dev'"`).

- [ ] **Step 3: Ajouter `versionStyle` dans `styles.go`**

Insérer juste après `bannerStyle` (`styles.go:26-28`) :

```go
var bannerStyle = lipgloss.NewStyle().
	Foreground(accent).
	Bold(true)

var versionStyle = lipgloss.NewStyle().
	Foreground(subtle)
```

- [ ] **Step 4: Modifier le rendu du logo dans `homeContent()`**

Dans `home.go`, remplacer (lignes 1107-1109) :

```go
	for _, l := range strings.Split(bannerArt, "\n") {
		lines = append(lines, pad(bannerStyle.Render(l)))
	}
```

par :

```go
	bannerLines := strings.Split(bannerArt, "\n")
	versionLabel := version
	if version != "dev" {
		versionLabel = "v" + version
	}
	first := bannerStyle.Render(bannerLines[0]) + "  " + versionStyle.Render(versionLabel)
	lines = append(lines, pad(first))
	for _, l := range bannerLines[1:] {
		lines = append(lines, pad(bannerStyle.Render(l)))
	}
```

Note : `version` est la variable package-level de `main.go:30` — même package `main`, donc
directement accessible sans import ni champ supplémentaire.

- [ ] **Step 5: Lancer les tests pour vérifier qu'ils passent**

Run: `go test ./... -run TestHomeContentShowsVersion -v` puis
`go test ./... -run TestHomeContentShowsDevVersion -v`

Expected: PASS pour les deux.

- [ ] **Step 6: Lancer toute la suite pour vérifier l'absence de régression**

Run: `go test ./...`

Expected: PASS. Porter une attention particulière à `TestHomeContentAndHitTest` (`home_test.go:332`)
et `TestHomeContentShowsTodayQuote`/`TestHomeContentHitTestUnaffectedByQuote` (ajoutés par la
fonctionnalité précédente, citation du jour) — la ligne du logo change de contenu mais son nombre
de lignes rendues ne change pas (toujours 2 lignes pour `bannerArt`), donc aucun décalage de
`homeCell` ne doit apparaître. Si un flake préexistant et sans rapport
(`TestSnippetCacheReadsHeadAndInvalidates`, timing mtime) apparaît, il est connu et non lié à ce
changement — le documenter dans le rapport plutôt que de tenter de le corriger.

- [ ] **Step 7: Vérifier visuellement (optionnel si `go run .` n'est pas exécutable dans l'environnement)**

Compiler avec une version injectée pour observer le rendu réel :

```bash
go build -ldflags "-X main.version=0.1.126" -o /tmp/okashi-version-check . && /tmp/okashi-version-check --version
```

Expected: affiche `okashi 0.1.126`. Si un terminal interactif est disponible, lancer
`/tmp/okashi-version-check` et observer le hub : la ligne du logo doit afficher
`o k a s h i  v0.1.126` centrée, la règle en dessous reste inchangée et plus courte. Supprimer le
binaire temporaire après vérification (`rm /tmp/okashi-version-check`).

- [ ] **Step 8: Commit**

```bash
git add styles.go home.go home_test.go
git commit -m "$(cat <<'EOF'
hub: affiche le numéro de version à côté du logo

versionStyle discret + concaténation sur la 1re ligne de bannerArt
avant centrage, réutilisant main.version (déjà injectable via
-ldflags) sans nouveau mécanisme ni champ sur model.
EOF
)"
```

## Self-Review Notes

- **Couverture du spec** : la section "Rendu dans homeContent()" → Step 4 ; "Cas version == dev" →
  couvert par la condition `if version != "dev"` (Step 4) et testé explicitement
  (`TestHomeContentShowsDevVersionWithoutPrefix`, Step 1) ; "Style" → Step 3. La section "Hors
  périmètre" du spec (pas de champ `model.version`, pas de nouveau mécanisme d'injection, pas de
  modification de `bannerView()`) ne demande aucune tâche — c'est explicitement ce qu'on ne
  construit pas, et le plan ne touche ni `main.go` ni `bannerView()`.
- **Cohérence des types** : `versionStyle lipgloss.Style` (Step 3) est utilisé tel quel dans le
  seul autre endroit qui le référence (Step 4, `home.go`) — aucune divergence de nom.
- **Effet de bord du test** : les deux tests de Step 1 modifient la variable globale `version` et
  la restaurent via `defer` — nécessaire car `version` est un état partagé au niveau package: sans
  restauration, l'ordre d'exécution des tests dans le paquet pourrait faire fuiter la valeur
  modifiée vers d'autres tests qui inspectent le rendu du hub.
