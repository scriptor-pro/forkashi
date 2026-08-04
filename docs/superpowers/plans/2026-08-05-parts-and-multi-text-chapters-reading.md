# Parties et chapitres multi-textes — Lecture/affichage Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rendre les Parties visuellement présentes partout où un manuscrit se lit (sidebar, hub,
corkboard, pager), et permettre d'ouvrir n'importe quel texte d'un chapitre multi-textes via un
sélecteur — sans encore toucher à l'export ni à la création/réorganisation (Parties, textes
supplémentaires), qui restent les sujets des deux plans suivants.

**Architecture:** `fileEntry` gagne un champ `isPartHeader bool` ; `filelist.SetDir`/`home.homeFilesFor`
insèrent une ligne d'en-tête non sélectionnable avant les chapitres de chaque Partie titrée,
`filelist.moveBy` saute ces lignes dans le sens du mouvement. `pager.load` insère une ligne d'en-tête
similaire dans le flux de lecture. `corkboard.corkboardView` relit `manuscriptView.parts` séparément du
staging `m.structureItems` pour afficher les cartes des vraies Parties (lecture seule) à côté des
cartes éditables des chapitres nus. Un nouvel écran `screenTextPicker` s'ouvre quand `⏎` sur un
chapitre à plusieurs textes ; `filelist.activate()` change de contrat de retour pour signaler ce cas
au lieu de naviguer dans le dossier comme un répertoire ordinaire.

**Tech Stack:** Go 1.25, `bubbletea`/`lipgloss` (nouvel écran modal), package `main`, tests `go test`.

## Global Constraints

- Aucun changement au manifest v2 ni à `manuscriptView`/`chapterRef`/`partRef`/`textRef` (posés par le
  plan Fondations, déjà fusionné dans `main`) — ce plan est purement UI/rendu par-dessus ce modèle.
- Toujours au moins un `partRef` dans `manuscriptView.parts` (la partie synthétique `title == ""`) —
  cette partie synthétique n'affiche JAMAIS d'en-tête (ni dans la sidebar, ni le hub, ni le corkboard,
  ni le pager) : seules les vraies Parties (`title != ""`) en ont un.
- `⏎` sur un chapitre à un seul texte reste inchangé (ouverture directe) — zéro régression perçue pour
  le cas courant (qui restera la majorité des manuscrits même après ce plan, puisque créer un second
  texte n'est pas encore possible dans l'UI).
- Le structure mode (`m.structureItems []chapterRef`) reste scopé aux chapitres nus uniquement — ce
  plan n'ajoute AUCUNE édition de Partie (créer, renommer, réordonner un chapitre entre Parties), juste
  leur affichage en lecture seule là où elles existent déjà (créées à la main dans `manifest.json`, ou
  par un futur outil externe/l'app companion). L'édition est le sujet du plan Création/réorganisation.
- Pas de concaténation multi-texte à l'export, pas de rupture de section Partie à l'export — le sujet
  du plan Export. `manuscriptDocFromChapters` (`export_ast.go`) n'est pas touché par ce plan.
- Build : `go build ./...`. Vet : `go vet ./...`. Tests : `go test ./...`.

---

## Task 1: `fileEntry.isPartHeader` et le saut de curseur dans `filelist.go`

**Files:**
- Modify: `filelist.go` (`fileEntry`, `moveBy`, `scrollIntoView`, `selectRow`)
- Test: `filelist_test.go`

**Interfaces:**
- Consumes: `manuscriptView{parts []partRef}`, `partRef{title string, chapters []chapterRef}` (déjà
  posés par le plan Fondations, `manuscript.go`, inchangés).
- Produces: `type fileEntry struct { name string; isDir bool; isPartHeader bool }` — le champ
  `isPartHeader` est consommé par Task 2 (construction de `f.entries` dans `SetDir`) et par Task 4
  (`home.go`, même struct partagée).

- [ ] **Step 1: Write the failing tests**

Ajoute à `filelist_test.go` :

```go
func TestMoveByDownSkipsPartHeader(t *testing.T) {
	f := newFilelist()
	f.entries = []fileEntry{
		{name: "chap-un", isDir: true},
		{name: "Partie Deux", isPartHeader: true},
		{name: "chap-deux", isDir: true},
	}
	f.selected = 0
	f.moveBy(1)
	if f.selected != 2 {
		t.Fatalf("moveBy(1) from index 0 must skip the header at index 1 and land on 2, got %d", f.selected)
	}
}

func TestMoveByUpSkipsPartHeader(t *testing.T) {
	f := newFilelist()
	f.entries = []fileEntry{
		{name: "chap-un", isDir: true},
		{name: "Partie Deux", isPartHeader: true},
		{name: "chap-deux", isDir: true},
	}
	f.selected = 2
	f.moveBy(-1)
	if f.selected != 0 {
		t.Fatalf("moveBy(-1) from index 2 must skip the header at index 1 and land on 0, got %d", f.selected)
	}
}

func TestMoveByStopsAtEndWithoutInfiniteLoopWhenTrailingHeaders(t *testing.T) {
	f := newFilelist()
	f.entries = []fileEntry{
		{name: "chap-un", isDir: true},
		{name: "Partie Deux", isPartHeader: true},
	}
	f.selected = 0
	f.moveBy(1)
	// No selectable entry below index 0 other than the header — selection must clamp,
	// never land on a header, never loop forever.
	if f.selected != 0 {
		t.Fatalf("moveBy(1) with only a trailing header must clamp back to the last selectable index, got %d", f.selected)
	}
}

func TestSelectRowSkipsPartHeaderForward(t *testing.T) {
	f := newFilelist()
	f.entries = []fileEntry{
		{name: "chap-un", isDir: true},
		{name: "Partie Deux", isPartHeader: true},
		{name: "chap-deux", isDir: true},
	}
	f.height = 10
	f.selectRow(1) // clicking directly on the header row
	if f.selected != 2 {
		t.Fatalf("clicking a header row must select the next selectable entry (forward), got %d", f.selected)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./... -run "TestMoveBy|TestSelectRowSkipsPartHeader" -v 2>&1 | tail -60`
Expected: compile error — `fileEntry` has no field `isPartHeader` (does not compile).

- [ ] **Step 3: Add `isPartHeader` to `fileEntry`**

Dans `filelist.go`, trouve le type `fileEntry` (~ligne 24) et ajoute le champ :

```go
type fileEntry struct {
	name         string
	isDir        bool
	isPartHeader bool // a non-selectable Part title row; cursor movement skips it
}
```

- [ ] **Step 4: Make `moveBy` skip header rows in the direction of travel**

Remplace `moveBy` (`filelist.go:283-295`) :

```go
func (f *filelist) moveBy(n int) {
	if len(f.entries) == 0 {
		return
	}
	step := 1
	if n < 0 {
		step = -1
	}
	remaining := n
	if remaining < 0 {
		remaining = -remaining
	}
	pos := f.selected
	for remaining > 0 {
		next := pos + step
		if next < 0 || next >= len(f.entries) {
			break // hit an edge — stop, don't wrap, don't land past the list
		}
		pos = next
		if !f.entries[pos].isPartHeader {
			remaining--
		}
	}
	// pos may have landed on a header if every remaining entry in this direction
	// is a header (or the list ends in one) — walk further in the same direction
	// until a selectable entry is found, or give up and keep the prior selection.
	for pos >= 0 && pos < len(f.entries) && f.entries[pos].isPartHeader {
		next := pos + step
		if next < 0 || next >= len(f.entries) {
			pos = f.selected // no selectable entry this way — clamp back
			break
		}
		pos = next
	}
	if pos < 0 {
		pos = 0
	}
	if pos >= len(f.entries) {
		pos = len(f.entries) - 1
	}
	if f.entries[pos].isPartHeader {
		pos = f.selected // last-resort guard: never let a header become selected
	}
	f.selected = pos
	f.scrollIntoView()
}
```

Note d'implémentation : la double boucle gère séparément "avancer de N entrées sélectionnables" et "si
on atterrit quand même sur un en-tête (parce que la marche s'est arrêtée pile dessus), continuer dans
le même sens jusqu'à en sortir" — nécessaire car un `moveBy(1)` qui doit sauter exactement un en-tête
peut se retrouver dessus après le compte de `remaining`, pas seulement pendant.

- [ ] **Step 5: Make `selectRow` (mouse click) skip a header forward**

Remplace `selectRow` (`filelist.go:309-318`) :

```go
// selectRow sets the selection from a row index within the visible window. A click
// landing on a Part-header row selects the next selectable entry below it instead
// (headers are never selectable) — forward, matching moveBy's own default direction
// for an explicit position jump.
func (f *filelist) selectRow(visibleRow int) {
	if visibleRow < 0 {
		return
	}
	idx := f.offset + visibleRow
	if idx >= len(f.entries) {
		return
	}
	for idx < len(f.entries) && f.entries[idx].isPartHeader {
		idx++
	}
	if idx >= len(f.entries) {
		return // nothing selectable below the clicked header — leave selection as-is
	}
	f.selected = idx
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./... -run "TestMoveBy|TestSelectRowSkipsPartHeader" -v 2>&1 | tail -60`
Expected: all 4 tests PASS.

- [ ] **Step 7: Build, vet, full test suite**

Run: `go build ./... && go vet ./... && go test ./... -count=1 2>&1 | tail -30`
Expected: clean build, no vet warnings, all tests PASS (existing `filelist_test.go` tests still pass
since `isPartHeader` defaults to `false` and changes nothing for entries that never set it).

- [ ] **Step 8: Commit**

```bash
git add filelist.go filelist_test.go
git commit -m "$(cat <<'EOF'
Ajoute fileEntry.isPartHeader et le saut de curseur dans la sidebar

Une ligne d'en-tête de Partie n'est jamais sélectionnable : moveBy
avance dans le sens du mouvement jusqu'à la prochaine entrée
sélectionnable ; un clic sur la ligne d'en-tête sélectionne l'entrée
suivante. Pas encore câblé dans SetDir/View — cette tâche pose
uniquement le mécanisme de saut, consommé par la tâche suivante.
EOF
)"
```

---

## Task 2: En-têtes de Partie dans la sidebar (`filelist.go`)

**Files:**
- Modify: `filelist.go` (`SetDir`, `View`, `sectionRow`)
- Test: `filelist_test.go`

**Interfaces:**
- Consumes: `fileEntry.isPartHeader` (Task 1) ; `manuscriptView.parts []partRef`,
  `partRef{title string, chapters []chapterRef}` (Fondations, inchangés) ; `f.wc.count(path string)
  int` (`project.go`, inchangé).
- Produces: `f.entries` contient désormais des lignes `fileEntry{isPartHeader: true}` intercalées ;
  aucune nouvelle fonction exportée consommée par une tâche ultérieure (le rendu de `View()` est la
  fin de la chaîne pour la sidebar).

- [ ] **Step 1: Write the failing tests**

Ajoute à `filelist_test.go` :

```go
func TestSidebarShowsPartHeaderWithWordTotal(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "the-letter"), 0o755)
	os.WriteFile(filepath.Join(dir, "the-letter", "the-letter.md"), []byte("one two three four five"), 0o644)
	os.WriteFile(filepath.Join(dir, manifestName), []byte(
		`{"schemaVersion":2,"title":"Windermere","items":[`+
			`{"part":"Part One","chapters":[`+
			`{"folder":"the-letter","title":"The Letter","texts":[{"file":"the-letter.md","title":"The Letter"}]}]}]}`), 0o644)
	f := newFilelist()
	f.root = ""
	f.width, f.height = 60, 12
	f.SetDir(dir)
	view := f.View(-1, "")
	if !strings.Contains(view, "Part One") {
		t.Fatalf("sidebar must show the Part title, got:\n%s", view)
	}
	if !strings.Contains(view, "5 m") {
		t.Fatalf("sidebar's Part header must show the total word count of its chapters, got:\n%s", view)
	}
}

func TestSidebarOmitsHeaderForSyntheticUntitledPart(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "opening"), 0o755)
	os.WriteFile(filepath.Join(dir, "opening", "opening.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, manifestName), []byte(
		`{"schemaVersion":2,"title":"N","items":[`+
			`{"chapter":{"folder":"opening","title":"Opening","texts":[{"file":"opening.md","title":"Opening"}]}}]}`), 0o644)
	f := newFilelist()
	f.root = ""
	f.width, f.height = 60, 12
	f.SetDir(dir)
	for _, e := range f.entries {
		if e.isPartHeader {
			t.Fatalf("a manuscript with no real Part must not render any header row, got entries: %+v", f.entries)
		}
	}
}

func TestSidebarShowsMixedBareChaptersAndPart(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "prologue"), 0o755)
	os.MkdirAll(filepath.Join(dir, "the-letter"), 0o755)
	os.WriteFile(filepath.Join(dir, "prologue", "prologue.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, "the-letter", "the-letter.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, manifestName), []byte(
		`{"schemaVersion":2,"title":"N","items":[`+
			`{"chapter":{"folder":"prologue","title":"Prologue","texts":[{"file":"prologue.md","title":"Prologue"}]}},`+
			`{"part":"Part One","chapters":[`+
			`{"folder":"the-letter","title":"The Letter","texts":[{"file":"the-letter.md","title":"The Letter"}]}]}]}`), 0o644)
	f := newFilelist()
	f.root = ""
	f.width, f.height = 60, 12
	f.SetDir(dir)
	view := f.View(-1, "")
	iPrologue := strings.Index(view, "Prologue")
	iPartOne := strings.Index(view, "Part One")
	iLetter := strings.Index(view, "The Letter")
	if iPrologue == -1 || iPartOne == -1 || iLetter == -1 {
		t.Fatalf("all three must appear, got:\n%s", view)
	}
	if !(iPrologue < iPartOne && iPartOne < iLetter) {
		t.Fatalf("order must be Prologue (bare), then Part One header, then The Letter, got:\n%s", view)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./... -run "TestSidebarShowsPartHeader|TestSidebarOmitsHeaderForSynthetic|TestSidebarShowsMixedBareChaptersAndPart" -v 2>&1 | tail -60`
Expected: FAIL — no Part header rendered yet (current `SetDir` flattens `parts` without emitting a
header row).

- [ ] **Step 3: Build the Part-header word total helper**

Dans `filelist.go`, ajoute une fonction avant `SetDir` :

```go
// partWordTotal sums the word count of every text in every chapter of p, using wc
// (same cache filelist already threads through chapterWords) — the number shown on
// a Part's header row.
func partWordTotal(dir string, p partRef, wc *wordCountCache) int {
	total := 0
	for _, ch := range p.chapters {
		for _, t := range ch.texts {
			total += wc.count(filepath.Join(dir, ch.folder, t.file))
		}
	}
	return total
}
```

- [ ] **Step 4: Emit a header row per real Part in `SetDir`**

Remplace le bloc de construction des chapitres dans `SetDir` (`filelist.go`, la boucle
`for _, p := range f.view.parts { for _, ch := range p.chapters { ... } }` introduite par le plan
Fondations) :

```go
	f.entries = append(f.entries, plainDirs...)
	for _, p := range f.view.parts {
		if p.title != "" {
			label := p.title + "  " + commafy(partWordTotal(dir, p, f.wc)) + " m"
			f.entries = append(f.entries, fileEntry{name: label, isPartHeader: true})
		}
		for _, ch := range p.chapters {
			if ch.folder == "" {
				if len(ch.texts) > 0 {
					f.entries = append(f.entries, fileEntry{name: ch.texts[0].file})
				}
				continue
			}
			f.entries = append(f.entries, fileEntry{name: ch.folder, isDir: true})
		}
	}
	f.entries = append(f.entries, f.view.loose...)
```

Note d'implémentation : le libellé complet (titre + total de mots) est construit une fois dans `name`
plutôt que stocké en deux champs séparés — `isPartHeader` rows n'ont pas besoin de passer par
`sectionRow`/`chapterTitle` (qui résolvent un titre à partir d'un `folder`, non pertinent ici), donc un
rendu direct de `name` dans `View()` suffit (voir Step 5).

- [ ] **Step 5: Render header rows distinctly in `View()`**

Dans `View()` (`filelist.go:151-202`), le `switch` qui décide comment rendre chaque ligne teste déjà
`section := f.isChapterEntry(e)`. Ajoute un cas AVANT celui-ci pour les en-têtes :

```go
	for i := f.offset; i < end; i++ {
		e := f.entries[i]
		g := f.icons.iconFor(e)
		section := f.isChapterEntry(e)
		switch {
		case e.isPartHeader:
			row := lipgloss.NewStyle().Bold(true).Foreground(accent).Render(ansi.Truncate(e.name, f.width, "…"))
			b.WriteString(row)
		case editRow >= 0 && i == editRow:
			b.WriteString(editRowStyle.Render(ansi.Truncate(" "+editField, f.width, "")))
		case i == f.selected:
			var content string
			if section {
				content = f.sectionRow(e, false)
			} else {
				content = " " + renderIcon(g, true) + e.name
			}
			b.WriteString(selectedStyle.Width(f.width).Render(ansi.Truncate(content, f.width, "…")))
		case section:
			b.WriteString(f.sectionRow(e, true))
		case e.isDir:
			row := " " + renderIcon(g, false) + lipgloss.NewStyle().Foreground(accent).Render(e.name)
			b.WriteString(ansi.Truncate(row, f.width, "…"))
		default:
			ext := filepath.Ext(e.name)
			icon := " " + renderIcon(g, false)
			if ext != "" && lipgloss.Width(icon+e.name) <= f.width {
				stem := icon + strings.TrimSuffix(e.name, ext)
				b.WriteString(stem + lipgloss.NewStyle().Foreground(subtle).Render(ext))
			} else {
				b.WriteString(ansi.Truncate(icon+e.name, f.width, "…"))
			}
		}
		if i < end-1 {
			b.WriteByte('\n')
		}
	}
```

(Seul changement : le nouveau `case e.isPartHeader:` en tête du `switch`, placé avant `editRow >= 0 &&
i == editRow` pour qu'un en-tête ne soit jamais confondu avec une ligne en cours d'édition — un en-tête
n'est de toute façon jamais une cible d'édition puisqu'aucun chemin du code ne fixe `editRow` sur son
index.)

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./... -run "TestSidebarShowsPartHeader|TestSidebarOmitsHeaderForSynthetic|TestSidebarShowsMixedBareChaptersAndPart" -v 2>&1 | tail -60`
Expected: all 3 tests PASS.

- [ ] **Step 7: Run the full filelist test file to check nothing else broke**

Run: `go test ./... -run TestSidebar -v 2>&1 | tail -60`
Run: `go test ./... -run TestFilelist -v 2>&1 | tail -80`
Expected: all PASS, including the pre-existing `TestSidebarRendersManifestTitleAndOrder`,
`TestSidebarOrdersSectionsNumerically`, `TestSidebarShowsTitlesAndCounts` (none of them exercise a real
Part, so the header logic must be a no-op for them).

- [ ] **Step 8: Build, vet, full test suite**

Run: `go build ./... && go vet ./... && go test ./... -count=1 2>&1 | tail -30`
Expected: clean, all PASS.

- [ ] **Step 9: Commit**

```bash
git add filelist.go filelist_test.go
git commit -m "$(cat <<'EOF'
Affiche les en-têtes de Partie dans la sidebar

Chaque Partie réellement titrée gagne une ligne d'en-tête non
sélectionnable (titre + total de mots de ses chapitres) avant ses
chapitres, dans l'ordre du manifest. La partie synthétique (chapitres
hors-partie) n'affiche jamais d'en-tête — comportement inchangé pour
tout manuscrit n'ayant créé aucune Partie.
EOF
)"
```

---

## Task 3: Sélecteur de texte à l'ouverture d'un chapitre multi-textes

**Files:**
- Modify: `filelist.go` (`activate`), `main.go` (`model`, `Update`, `View`, les deux sites d'appel de
  `activate()`)
- Test: `filelist_test.go`, `main_test.go`

**Interfaces:**
- Consumes: `chapterRef{folder string, title string, texts []textRef}`, `textRef{file, title string,
  words int}` (Fondations, inchangés) ; `isChapterOf(v manuscriptView, folder string) bool`
  (`manuscript.go`, inchangé).
- Produces:
  ```go
  // activateResult is what activate() found at the cursor.
  type activateResult int
  const (
      activateNone activateResult = iota // navigated into a plain dir, or nothing selectable
      activateFile                        // a single file is ready to open — path is valid
      activateTextPicker                  // a multi-text chapter — caller must open the picker
  )
  func (f *filelist) activate() (path string, result activateResult)
  ```
  Signature change consommée par `main.go` (les deux sites d'appel existants) — c'est la seule
  interface publique nouvelle de cette tâche.

- [ ] **Step 1: Write the failing tests for `activate()`'s new contract**

Ajoute à `filelist_test.go` :

```go
func TestActivateSingleTextChapterOpensDirectly(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "opening"), 0o755)
	os.WriteFile(filepath.Join(dir, "opening", "opening.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, manifestName), []byte(
		`{"schemaVersion":2,"title":"N","items":[`+
			`{"chapter":{"folder":"opening","title":"Opening","texts":[{"file":"opening.md","title":"Opening"}]}}]}`), 0o644)
	f := newFilelist()
	f.root = ""
	f.width, f.height = 60, 12
	f.SetDir(dir)
	f.selectName("opening")
	path, result := f.activate()
	if result != activateFile {
		t.Fatalf("a single-text chapter must activate as activateFile, got %v", result)
	}
	want := filepath.Join(dir, "opening", "opening.md")
	if path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
}

func TestActivateMultiTextChapterSignalsPicker(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "chapitre-un"), 0o755)
	os.WriteFile(filepath.Join(dir, "chapitre-un", "scene-un.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, "chapitre-un", "scene-deux.md"), []byte("y"), 0o644)
	os.WriteFile(filepath.Join(dir, manifestName), []byte(
		`{"schemaVersion":2,"title":"N","items":[`+
			`{"chapter":{"folder":"chapitre-un","title":"Chapitre Un","texts":[`+
			`{"file":"scene-un.md","title":"Scène Un"},{"file":"scene-deux.md","title":"Scène Deux"}]}}]}`), 0o644)
	f := newFilelist()
	f.root = ""
	f.width, f.height = 60, 12
	f.SetDir(dir)
	f.selectName("chapitre-un")
	_, result := f.activate()
	if result != activateTextPicker {
		t.Fatalf("a multi-text chapter must activate as activateTextPicker, got %v", result)
	}
}

func TestActivatePlainFolderStillNavigates(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "notes"), 0o755)
	f := newFilelist()
	f.root = ""
	f.width, f.height = 60, 12
	f.SetDir(dir)
	f.selectName("notes")
	_, result := f.activate()
	if result != activateNone {
		t.Fatalf("a plain (non-chapter) folder must still navigate (activateNone), got %v", result)
	}
	if f.dir != filepath.Join(dir, "notes") {
		t.Fatalf("navigating into a plain folder must update f.dir, got %q", f.dir)
	}
}

func TestActivateEmptyChapterSignalsPickerWithNoTexts(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "vide"), 0o755)
	os.WriteFile(filepath.Join(dir, manifestName), []byte(
		`{"schemaVersion":2,"title":"N","items":[`+
			`{"chapter":{"folder":"vide","title":"Vide","texts":[]}}]}`), 0o644)
	f := newFilelist()
	f.root = ""
	f.width, f.height = 60, 12
	f.SetDir(dir)
	f.selectName("vide")
	_, result := f.activate()
	if result != activateTextPicker {
		t.Fatalf("an empty chapter must also route to the picker (which shows an empty state), not crash or navigate, got %v", result)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./... -run TestActivate -v 2>&1 | tail -60`
Expected: compile error — `activate()` still returns `(string, bool)`, `activateResult`/`activateFile`/
etc. undefined.

- [ ] **Step 3: Change `activate()`'s contract**

Remplace `activate()` (`filelist.go:322-336`) :

```go
// activateResult is what activate() found at the cursor: a plain-dir navigation (or
// nothing selectable) needs no further action from the caller; a single-text chapter
// or ordinary file is ready to open at path; a multi-text (or empty) chapter needs
// the caller to open the text picker instead of opening a file directly.
type activateResult int

const (
	activateNone activateResult = iota
	activateFile
	activateTextPicker
)

// activate acts on the selected entry. A "..", a plain directory, or a Part-header
// row (guarded above by moveBy/selectRow, but defensively checked here too) navigates
// and returns activateNone. A v2 chapter folder with exactly one text returns its
// path with activateFile — unchanged behavior from before multi-text chapters
// existed. A v2 chapter folder with zero or 2+ texts returns activateTextPicker; the
// caller opens the picker screen instead of a file. Anything else (an ordinary file,
// or a legacy single-file chapter) returns its path with activateFile.
func (f *filelist) activate() (string, activateResult) {
	if len(f.entries) == 0 || f.selected >= len(f.entries) {
		return "", activateNone
	}
	e := f.entries[f.selected]
	if e.isPartHeader {
		return "", activateNone // defensive: moveBy/selectRow never select a header
	}
	if e.isDir {
		if isChapterOf(f.view, e.name) {
			ch := chapterByFolder(f.view, e.name)
			if len(ch.texts) == 1 {
				return filepath.Join(f.dir, ch.folder, ch.texts[0].file), activateFile
			}
			return "", activateTextPicker
		}
		if e.name == ".." {
			f.SetDir(filepath.Dir(f.dir))
		} else {
			f.SetDir(filepath.Join(f.dir, e.name))
		}
		return "", activateNone
	}
	return filepath.Join(f.dir, e.name), activateFile
}

// chapterByFolder returns the chapterRef whose folder matches, across all parts. The
// caller (activate) only calls this after isChapterOf already confirmed a match, so
// the zero-value fallback is unreachable in practice.
func chapterByFolder(v manuscriptView, folder string) chapterRef {
	for _, p := range v.parts {
		for _, ch := range p.chapters {
			if ch.folder == folder {
				return ch
			}
		}
	}
	return chapterRef{}
}
```

- [ ] **Step 4: Update the two call sites in `main.go`**

Trouve les deux appels `m.files.activate()` (`main.go:1405` et `main.go:1613` au moment de la rédaction
de ce plan — confirme les numéros de ligne exacts avant d'éditer, ils ont pu se décaler).

Site 1 (double-clic souris, ~ligne 1405) :

```go
			if path, result := m.files.activate(); result == activateFile {
				m.loadFile(path)
				m.focus = focusEditor
				m.editor.Focus()
			} else if result == activateTextPicker {
				m.enterTextPicker()
			}
```

Site 2 (`enter`/`right`/`l` clavier, ~ligne 1613) :

```go
				case "enter", "right", "l":
					if path, result := m.files.activate(); result == activateFile {
						m.loadFile(path)
						m.focus = focusEditor
						m.editor.Focus()
					} else if result == activateTextPicker {
						m.enterTextPicker()
					}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./... -run TestActivate -v 2>&1 | tail -60`
Expected: 4 tests PASS. (`enterTextPicker` doesn't exist yet — this step's tests target `activate()`
directly, not the `main.go` wiring, so they compile and pass without it; Step 6 below fixes the
resulting `main.go` compile error from Step 4's edit.)

- [ ] **Step 6: Build to confirm the expected `main.go` compile error, then proceed to Task 4**

Run: `go build ./... 2>&1 | head -20`
Expected: `undefined: activateTextPicker` is NOT the error (that constant now exists) — instead:
`m.enterTextPicker undefined (type *model has no field or method enterTextPicker)`. This is expected:
Task 4 adds `enterTextPicker`. Do not implement it here — this task ends with `main.go` intentionally
not compiling, exactly like the Fondations plan's own tasks left the build red between steps.

- [ ] **Step 7: Commit**

```bash
git add filelist.go filelist_test.go main.go
git commit -m "$(cat <<'EOF'
Change le contrat de activate() pour signaler un chapitre multi-textes

activate() retourne désormais (path string, result activateResult) au
lieu de (string, bool) : activateFile pour une ouverture directe
(comportement inchangé pour tout chapitre à texte unique et tout
fichier ordinaire), activateTextPicker pour un chapitre à 0 ou 2+
textes, activateNone pour une navigation dans un dossier ordinaire.
main.go route activateTextPicker vers m.enterTextPicker() (Task
suivante — le build reste rouge jusque-là, cette fonction n'existe pas
encore).
EOF
)"
```

---

## Task 4: L'écran `screenTextPicker`

**Files:**
- Modify: `main.go` (`model`, `screen` enum, `Update`, `View`, `enterWriting`/nouveau `enterTextPicker`)
- Test: `main_test.go`

**Interfaces:**
- Consumes: `chapterRef{folder, title string, texts []textRef}`, `textRef{file, title string, words
  int}` (Fondations) ; `activateResult`/`activateTextPicker` (Task 3) ; `commafy(n int) string`
  (existant, utilisé ailleurs pour les compteurs de mots — vérifie son emplacement exact par grep avant
  utilisation).
- Produces: `model.textPickerChapter *chapterRef`, `model.textPickerSel int`,
  `model.textPickerDir string` (l'écran lui-même, consommé par personne d'autre) ; `func
  (m *model) enterTextPicker()` (consommée par Task 3, déjà câblée).

- [ ] **Step 1: Write the failing tests**

Ajoute à `main_test.go` (crée-le s'il n'existe pas déjà — le plan Fondations en a déjà créé un ; vérifie
avec `ls main_test.go` avant de choisir créer vs. ajouter) :

```go
func TestEnterTextPickerOpensOnMultiTextChapter(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "chapitre-un"), 0o755)
	os.WriteFile(filepath.Join(dir, "chapitre-un", "scene-un.md"), []byte("un deux trois"), 0o644)
	os.WriteFile(filepath.Join(dir, "chapitre-un", "scene-deux.md"), []byte("quatre cinq"), 0o644)
	os.WriteFile(filepath.Join(dir, manifestName), []byte(
		`{"schemaVersion":2,"title":"N","items":[`+
			`{"chapter":{"folder":"chapitre-un","title":"Chapitre Un","texts":[`+
			`{"file":"scene-un.md","title":"Scène Un"},{"file":"scene-deux.md","title":"Scène Deux"}]}}]}`), 0o644)

	t.Setenv("OKASHI_DIR", dir)
	m := initialModel()
	m.files.selectName("chapitre-un")
	m.enterTextPicker()

	if m.screen != screenTextPicker {
		t.Fatalf("enterTextPicker must switch to screenTextPicker, got %v", m.screen)
	}
	if m.textPickerChapter == nil || m.textPickerChapter.folder != "chapitre-un" {
		t.Fatalf("textPickerChapter must be set to the selected chapter, got %+v", m.textPickerChapter)
	}
}

func TestTextPickerViewShowsEachTextWithWordCount(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "chapitre-un"), 0o755)
	os.WriteFile(filepath.Join(dir, "chapitre-un", "scene-un.md"), []byte("un deux trois"), 0o644)
	os.WriteFile(filepath.Join(dir, "chapitre-un", "scene-deux.md"), []byte("quatre cinq"), 0o644)
	os.WriteFile(filepath.Join(dir, manifestName), []byte(
		`{"schemaVersion":2,"title":"N","items":[`+
			`{"chapter":{"folder":"chapitre-un","title":"Chapitre Un","texts":[`+
			`{"file":"scene-un.md","title":"Scène Un"},{"file":"scene-deux.md","title":"Scène Deux"}]}}]}`), 0o644)

	t.Setenv("OKASHI_DIR", dir)
	m := initialModel()
	m.files.selectName("chapitre-un")
	m.enterTextPicker()
	m.width, m.height = 60, 20
	view := m.View()

	if !strings.Contains(view, "Scène Un") || !strings.Contains(view, "Scène Deux") {
		t.Fatalf("picker must list both text titles, got:\n%s", view)
	}
	if !strings.Contains(view, "3 m") || !strings.Contains(view, "2 m") {
		t.Fatalf("picker must show each text's own word count, got:\n%s", view)
	}
}

func TestTextPickerEnterOpensSelectedTextInEditor(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "chapitre-un"), 0o755)
	os.WriteFile(filepath.Join(dir, "chapitre-un", "scene-un.md"), []byte("un"), 0o644)
	os.WriteFile(filepath.Join(dir, "chapitre-un", "scene-deux.md"), []byte("deux"), 0o644)
	os.WriteFile(filepath.Join(dir, manifestName), []byte(
		`{"schemaVersion":2,"title":"N","items":[`+
			`{"chapter":{"folder":"chapitre-un","title":"Chapitre Un","texts":[`+
			`{"file":"scene-un.md","title":"Scène Un"},{"file":"scene-deux.md","title":"Scène Deux"}]}}]}`), 0o644)

	t.Setenv("OKASHI_DIR", dir)
	m := initialModel()
	m.files.selectName("chapitre-un")
	m.enterTextPicker()
	m.textPickerSel = 1 // "Scène Deux"

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm := updated.(model)

	if mm.screen != screenWriting {
		t.Fatalf("confirming a text must enter the writing screen, got %v", mm.screen)
	}
	want := filepath.Join(dir, "chapitre-un", "scene-deux.md")
	if mm.currentFile != want {
		t.Fatalf("currentFile = %q, want %q", mm.currentFile, want)
	}
}

func TestTextPickerEscCancelsWithoutOpening(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "chapitre-un"), 0o755)
	os.WriteFile(filepath.Join(dir, "chapitre-un", "scene-un.md"), []byte("un"), 0o644)
	os.WriteFile(filepath.Join(dir, "chapitre-un", "scene-deux.md"), []byte("deux"), 0o644)
	os.WriteFile(filepath.Join(dir, manifestName), []byte(
		`{"schemaVersion":2,"title":"N","items":[`+
			`{"chapter":{"folder":"chapitre-un","title":"Chapitre Un","texts":[`+
			`{"file":"scene-un.md","title":"Scène Un"},{"file":"scene-deux.md","title":"Scène Deux"}]}}]}`), 0o644)

	t.Setenv("OKASHI_DIR", dir)
	m := initialModel()
	m.files.selectName("chapitre-un")
	m.enterTextPicker()

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	mm := updated.(model)

	if mm.screen == screenTextPicker {
		t.Fatal("esc must dismiss the picker")
	}
	if mm.currentFile != "" {
		t.Fatalf("esc must not open any file, got currentFile = %q", mm.currentFile)
	}
}

func TestTextPickerShowsEmptyStateForZeroTextChapter(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "vide"), 0o755)
	os.WriteFile(filepath.Join(dir, manifestName), []byte(
		`{"schemaVersion":2,"title":"N","items":[`+
			`{"chapter":{"folder":"vide","title":"Vide","texts":[]}}]}`), 0o644)

	t.Setenv("OKASHI_DIR", dir)
	m := initialModel()
	m.files.selectName("vide")
	m.enterTextPicker()
	m.width, m.height = 60, 20
	view := m.View()

	if !strings.Contains(view, "aucun texte") {
		t.Fatalf("an empty chapter's picker must show an empty-state message, got:\n%s", view)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go build ./... 2>&1 | head -20`
Expected: `m.enterTextPicker undefined` (already the state left by Task 3), plus now
`screenTextPicker undefined`, `m.textPickerChapter undefined`, `m.textPickerSel undefined` — does not
compile, as expected before implementation.

- [ ] **Step 3: Add `screenTextPicker` and the model fields**

Dans `main.go`, ajoute `screenTextPicker` à l'enum `screen` (~ligne 228, après `screenOutline`) :

```go
	screenOutline
	screenTextPicker
)
```

Trouve la déclaration du struct `model` (cherche `migrationPending *migrationStep` comme repère, posé
par le plan Fondations) et ajoute à proximité :

```go
	textPickerChapter *chapterRef // non-nil while the text-picker screen is showing
	textPickerSel     int         // selected index into textPickerChapter.texts
	textPickerDir     string      // manuscript root, for resolving the chapter's folder path
```

- [ ] **Step 4: `enterTextPicker`**

Ajoute, à proximité de `enterWriting` (posée par le plan Fondations dans `main.go`) :

```go
// enterTextPicker opens the text-picker screen for the sidebar's currently selected
// chapter. Called only when filelist.activate() returned activateTextPicker — the
// caller (Update's sidebar key handling) already confirmed the selection is a
// chapter folder with zero or 2+ texts.
func (m *model) enterTextPicker() {
	folder, ok := m.files.selectedEntryName()
	if !ok {
		return
	}
	ch := chapterByFolder(m.files.view, folder)
	m.textPickerChapter = &ch
	m.textPickerSel = 0
	m.textPickerDir = m.files.dir
	m.screen = screenTextPicker
}
```

`selectedEntryName` n'existe pas encore dans `filelist.go` — ajoute-la (petit helper, symétrique de
`selectedFile` qui existe déjà mais renvoie un chemin absolu et exclut les dossiers ; celui-ci doit
renvoyer le nom brut de l'entrée sélectionnée, dossier ou fichier, pour que `enterTextPicker` puisse
retrouver le `folder` du chapitre) :

```go
// selectedEntryName returns the raw name of the selected entry (folder or file),
// or ok=false if nothing is selected. Used by enterTextPicker to recover which
// chapter folder the cursor was on.
func (f filelist) selectedEntryName() (string, bool) {
	if f.selected < 0 || f.selected >= len(f.entries) {
		return "", false
	}
	return f.entries[f.selected].name, true
}
```

- [ ] **Step 5: Wire `Update` for the picker screen**

Dans `Update`, à proximité des autres branches `if m.screen == screenXxx { return m.updateXxx(msg) }`
(cherche `if m.screen == screenNotes` comme repère), ajoute :

```go
	if m.screen == screenTextPicker {
		return m.updateTextPicker(msg)
	}
```

Puis ajoute la fonction, à la suite de `enterTextPicker` :

```go
// updateTextPicker handles input while the text-picker screen is showing: up/down
// move the selection, enter opens the chosen text and enters the writing screen,
// esc dismisses without opening anything.
func (m model) updateTextPicker(msg tea.Msg) (tea.Model, tea.Cmd) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	texts := m.textPickerChapter.texts
	switch km.Type {
	case tea.KeyUp:
		if m.textPickerSel > 0 {
			m.textPickerSel--
		}
	case tea.KeyDown:
		if m.textPickerSel < len(texts)-1 {
			m.textPickerSel++
		}
	case tea.KeyEnter:
		if len(texts) == 0 {
			return m, nil // empty state: nothing to open
		}
		t := texts[m.textPickerSel]
		path := filepath.Join(m.textPickerDir, m.textPickerChapter.folder, t.file)
		m.textPickerChapter = nil
		m.loadFile(path)
		m.focus = focusEditor
		m.editor.Focus()
		m.screen = screenWriting
	case tea.KeyEsc:
		m.textPickerChapter = nil
		m.screen = screenWriting
	}
	return m, nil
}
```

Note d'implémentation : `switch km.Type` (pas `km.String()`) car les tests dispatchent des
`tea.KeyMsg{Type: tea.KeyEnter}`/`{Type: tea.KeyEsc}` directement — cohérent avec le pattern déjà
utilisé par le garde de migration du plan Fondations (`km.String()` là-bas, mais celui-ci compare des
`Type` directement pour Up/Down/Enter/Esc, qui n'ont pas besoin de variantes `j`/`k` ici puisqu'aucune
autre touche n'a de sens sur cet écran).

- [ ] **Step 6: Render the picker in `View`**

Dans `View()`, à proximité des autres branches `if m.screen == screenXxx { return ... }`, ajoute :

```go
	if m.screen == screenTextPicker {
		return textPickerView(m.textPickerChapter, m.textPickerSel, m.width)
	}
```

Puis ajoute la fonction de rendu (à la suite de `updateTextPicker`) :

```go
// textPickerView renders the list of a chapter's texts, one per line, each with
// its own word count — or an empty-state message if the chapter has none.
func textPickerView(ch *chapterRef, sel int, width int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "── %s ──\n\n", ch.title)
	if len(ch.texts) == 0 {
		b.WriteString("  (aucun texte dans ce chapitre)")
		return b.String()
	}
	for i, t := range ch.texts {
		marker := "  "
		if i == sel {
			marker = selectedStyle.Render("▸ ")
		}
		fmt.Fprintf(&b, "%s%s\n", marker, t.title)
	}
	b.WriteString("\n↑↓ sélectionner · Entrée ouvrir · Échap annuler")
	return b.String()
}
```

Vérifie que `fmt` est déjà importé dans `main.go` (quasi certain, fichier volumineux) avant d'ajouter
l'import.

- [ ] **Step 7: Run tests to verify they pass**

Run: `go test ./... -run TestEnterTextPicker -v 2>&1 | tail -30`
Run: `go test ./... -run TestTextPicker -v 2>&1 | tail -80`
Expected: all 5 tests from Step 1 PASS.

- [ ] **Step 8: Build, vet, full test suite**

Run: `go build ./... && go vet ./... && go test ./... -count=1 2>&1 | tail -30`
Expected: clean build (this is the point where `main.go`'s Task 3 debt — the undefined
`enterTextPicker` — resolves), no vet warnings, all tests PASS.

- [ ] **Step 9: Commit**

```bash
git add main.go main_test.go filelist.go
git commit -m "$(cat <<'EOF'
Ajoute l'écran screenTextPicker pour ouvrir un texte d'un chapitre multi-textes

⏎ sur un chapitre à 2+ textes (ou 0, cas transitoire toléré) ouvre un
petit écran listant ses textes, chacun avec son propre compteur de
mots ; ↑↓ sélectionne, Entrée ouvre dans l'éditeur, Échap annule sans
rien ouvrir. Un chapitre à un seul texte continue de s'ouvrir
directement, comportement inchangé.
EOF
)"
```

---

## Task 5: En-têtes de Partie dans le hub (`home.go`)

**Files:**
- Modify: `home.go` (`homeFilesFor`)
- Test: `home_test.go`

**Interfaces:**
- Consumes: `fileEntry.isPartHeader` (Task 1) ; `manuscriptView.parts []partRef` (Fondations) ;
  `partWordTotal(dir string, p partRef, wc *wordCountCache) int` (Task 2, `filelist.go` — même
  fonction réutilisée, pas dupliquée).
- Produces: `homeFileItem` gagne un champ `isPartHeader bool` (miroir de `fileEntry`, puisque
  `homeFileItem` est un type distinct utilisé uniquement par le hub) — pas consommé ailleurs.

- [ ] **Step 1: Write the failing tests**

Ajoute à `home_test.go` :

```go
func TestHomeFilesForShowsPartHeaderWithWordTotal(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "the-letter"), 0o755)
	os.WriteFile(filepath.Join(dir, "the-letter", "the-letter.md"), []byte("one two three"), 0o644)
	os.WriteFile(filepath.Join(dir, manifestName), []byte(
		`{"schemaVersion":2,"title":"Windermere","items":[`+
			`{"part":"Part One","chapters":[`+
			`{"folder":"the-letter","title":"The Letter","texts":[{"file":"the-letter.md","title":"The Letter"}]}]}]}`), 0o644)

	t.Setenv("OKASHI_DIR", dir)
	m := initialModel()
	items := m.homeFilesFor(dir, true)

	found := false
	for _, it := range items {
		if it.isPartHeader && it.name == "Part One" {
			found = true
		}
	}
	if !found {
		t.Fatalf("homeFilesFor must include a Part-header item, got: %+v", items)
	}
}

func TestHomeFilesForOmitsHeaderForSyntheticUntitledPart(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "opening"), 0o755)
	os.WriteFile(filepath.Join(dir, "opening", "opening.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, manifestName), []byte(
		`{"schemaVersion":2,"title":"N","items":[`+
			`{"chapter":{"folder":"opening","title":"Opening","texts":[{"file":"opening.md","title":"Opening"}]}}]}`), 0o644)

	t.Setenv("OKASHI_DIR", dir)
	m := initialModel()
	items := m.homeFilesFor(dir, true)

	for _, it := range items {
		if it.isPartHeader {
			t.Fatalf("no real Part exists — must not synthesize a header, got: %+v", items)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./... -run TestHomeFilesForShowsPartHeader -v 2>&1 | tail -40`
Expected: compile error — `homeFileItem` has no field `isPartHeader` (does not compile).

- [ ] **Step 3: Add `isPartHeader` to `homeFileItem` and emit it in `homeFilesFor`**

Trouve le type `homeFileItem` (`home.go`, cherche `type homeFileItem struct`) et ajoute le champ :

```go
type homeFileItem struct {
	name         string
	path         string
	isDir        bool
	words        int
	snippet      string
	isPartHeader bool
}
```

Dans `homeFilesFor` (`home.go:85-121` au moment de ce plan), après la ligne `view := resolveManuscript(dir, fes)`
et avant la construction de `folders`/`mk`, insère la boucle d'assemblage des chapitres groupée par
Partie — remplace la fin de la fonction (la partie qui construit les items de chapitre/loose après
`mk`, à identifier précisément en lisant le fichier réel avant d'éditer) pour qu'elle parcoure
`view.parts` au lieu d'aplatir directement :

```go
	out := folders
	for _, p := range view.parts {
		if p.title != "" {
			out = append(out, homeFileItem{
				name:         p.title + "  " + commafy(partWordTotal(dir, p, m.files.wc)) + " m",
				isPartHeader: true,
			})
		}
		for _, ch := range p.chapters {
			if ch.folder == "" {
				if len(ch.texts) > 0 {
					out = append(out, mk(ch.title, ch.texts[0].file))
				}
				continue
			}
			if len(ch.texts) > 0 {
				out = append(out, mk(ch.title, filepath.Join(ch.folder, ch.texts[0].file)))
			}
		}
	}
	for _, l := range view.loose {
		out = append(out, mk(l.name, l.name))
	}
	return out
```

Avant d'appliquer ce remplacement, lis la fin réelle de `homeFilesFor` (au-delà de ce que ce plan a pu
citer de mémoire) pour t'assurer que la boucle `for _, ch := range view.chapters` existante (posée par
le plan Fondations) est bien remplacée par celle-ci, et pas dupliquée à côté.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./... -run TestHomeFilesFor -v 2>&1 | tail -60`
Expected: all tests PASS, including the 2 new ones and pre-existing `TestClassifyLibraryAndFiles` (or
equivalent — check by name in `home_test.go`).

- [ ] **Step 5: Build, vet, full test suite**

Run: `go build ./... && go vet ./... && go test ./... -count=1 2>&1 | tail -30`
Expected: clean, all PASS.

- [ ] **Step 6: Commit**

```bash
git add home.go home_test.go
git commit -m "$(cat <<'EOF'
Affiche les en-têtes de Partie dans le hub

Même traitement que la sidebar (Task 2 de ce plan) : chaque Partie
réellement titrée gagne une ligne d'en-tête (titre + total de mots)
dans le panneau FICHIERS du hub. La fonction de total de mots
(partWordTotal) est réutilisée telle quelle depuis filelist.go, pas
dupliquée.
EOF
)"
```

---

## Task 6: En-têtes de Partie dans le pager (lecture continue)

**Files:**
- Modify: `pager.go` (`load`)
- Test: `pager_wiring_test.go`

**Interfaces:**
- Consumes: `manuscriptView.parts []partRef`, `partRef{title string, chapters []chapterRef}`
  (Fondations, inchangés).
- Produces: aucune nouvelle interface publique — `pagerLine` garde sa forme, une ligne de titre de
  Partie est juste une `pagerLine{header: true, ...}` de plus dans le flux, comme les lignes de titre
  de chapitre existantes.

- [ ] **Step 1: Write the failing test**

Ajoute à `pager_wiring_test.go` :

```go
func TestPagerShowsPartHeaderBeforeItsChapters(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "the-letter"), 0o755)
	os.WriteFile(filepath.Join(dir, "the-letter", "the-letter.md"), []byte("Contenu."), 0o644)
	os.WriteFile(filepath.Join(dir, manifestName), []byte(
		`{"schemaVersion":2,"title":"N","items":[`+
			`{"part":"Part One","chapters":[`+
			`{"folder":"the-letter","title":"The Letter","texts":[{"file":"the-letter.md","title":"The Letter"}]}]}]}`), 0o644)

	var p pagerModel
	p.load(dir, 60)

	iPart := -1
	iChapter := -1
	for i, l := range p.lines {
		if l.header && strings.Contains(l.text, "Part One") {
			iPart = i
		}
		if l.header && strings.Contains(l.text, "The Letter") {
			iChapter = i
		}
	}
	if iPart == -1 {
		t.Fatalf("pager must render a header line for the Part title, got lines: %+v", p.lines)
	}
	if iPart >= iChapter {
		t.Fatalf("the Part header must come BEFORE its chapter's own header, got Part at %d, chapter at %d", iPart, iChapter)
	}
}

func TestPagerOmitsHeaderForSyntheticUntitledPart(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "opening"), 0o755)
	os.WriteFile(filepath.Join(dir, "opening", "opening.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, manifestName), []byte(
		`{"schemaVersion":2,"title":"N","items":[`+
			`{"chapter":{"folder":"opening","title":"Opening","texts":[{"file":"opening.md","title":"Opening"}]}}]}`), 0o644)

	var p pagerModel
	p.load(dir, 60)

	for _, l := range p.lines {
		if l.header && l.text != "── Opening ──" {
			t.Fatalf("no real Part exists — the only header line must be the chapter's own, got: %q", l.text)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./... -run TestPagerShowsPartHeader -v 2>&1 | tail -40`
Expected: FAIL — no Part header line is currently emitted (`load` only emits chapter header lines).

- [ ] **Step 3: Emit a Part header line in `load`**

Dans `pager.go`, `load` (introduite/adaptée par le plan Fondations), modifie la boucle externe pour
émettre une ligne d'en-tête avant les chapitres d'une Partie titrée :

```go
	running := 0
	for _, part := range v.parts {
		if part.title != "" {
			p.lines = append(p.lines, pagerLine{
				text:     "── " + part.title + " ──",
				file:     "",
				src:      -1,
				header:   true,
				cumWords: running,
			})
		}
		for _, ch := range part.chapters {
			if len(ch.texts) == 0 {
				continue
			}
			file := filepath.Join(ch.folder, ch.texts[0].file)
			p.lines = append(p.lines, pagerLine{
				text:     "── " + ch.title + " ──",
				file:     file,
				src:      -1,
				header:   true,
				cumWords: running,
			})
			data, err := os.ReadFile(filepath.Join(dir, ch.folder, ch.texts[0].file))
			if err != nil {
				continue
			}
			body := strings.TrimSuffix(string(data), "\n")
			for srcIdx, srcLine := range strings.Split(body, "\n") {
				for _, row := range strings.Split(ansi.Wrap(srcLine, width, ""), "\n") {
					running += wordCount(row)
					p.lines = append(p.lines, pagerLine{
						text:     row,
						file:     file,
						src:      srcIdx,
						header:   false,
						cumWords: running,
					})
				}
			}
		}
	}
	p.total = running
```

Note d'implémentation : la ligne d'en-tête de Partie a `file: ""` (contrairement aux en-têtes de
chapitre, qui portent `file` pour le jump-to-edit) — cliquer/valider sur une ligne de titre de Partie
ne doit rien ouvrir, puisqu'aucun fichier ne lui correspond directement. Vérifié dans le code actuel :
`jumpTarget()` (`pager.go:122-134`) a déjà `if l.file == "" { return "", 0, false }` juste après avoir
récupéré la ligne — donc une ligne de titre de Partie avec `file: ""` est automatiquement traitée comme
"rien à ouvrir" sans planter. Aucune garde supplémentaire n'est nécessaire dans `jumpTarget()` pour
cette tâche.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./... -run TestPagerShowsPartHeader -v 2>&1 | tail -40`
Run: `go test ./... -run TestPagerOmitsHeaderForSynthetic -v 2>&1 | tail -40`
Expected: both PASS.

- [ ] **Step 5: Run the full pager test file to check nothing else broke**

Run: `go test ./... -run TestPager -v 2>&1 | tail -80`
Expected: all PASS, including pre-existing tests (none of which exercise a real Part, so the header
logic must be a no-op for them — e.g. jump-to-edit on an ordinary chapter line must be unaffected).

- [ ] **Step 6: Build, vet, full test suite**

Run: `go build ./... && go vet ./... && go test ./... -count=1 2>&1 | tail -30`
Expected: clean, all PASS.

- [ ] **Step 7: Commit**

```bash
git add pager.go pager_wiring_test.go
git commit -m "$(cat <<'EOF'
Affiche les en-têtes de Partie dans le pager (lecture continue)

Une ligne "── Titre de Partie ──" précède les chapitres de chaque
Partie réellement titrée, avant leur propre ligne de titre de
chapitre — cohérence avec le rendu prévu à l'export (plan suivant). La
ligne de Partie n'a pas de file associé (rien à ouvrir en jump-to-edit
dessus), contrairement aux lignes de titre de chapitre.
EOF
)"
```

---

## Task 7: En-têtes de Partie dans le corkboard

**Files:**
- Modify: `corkboard.go` (`corkboardView`)
- Test: `corkboard_test.go`

**Interfaces:**
- Consumes: `manuscriptView.parts []partRef` (Fondations) ; `m.structureItems []chapterRef`
  (Fondations, staging des chapitres nus, inchangé — reste la SEULE source éditée par ce plan) ;
  `resolveManuscript(dir string, entries []fileEntry) manuscriptView`, `readEntries(dir string)
  []fileEntry` (existants, déjà utilisés ailleurs dans `corkboard.go` via `corkChapterSet`).
- Produces: aucune nouvelle interface publique — changement de rendu uniquement.

- [ ] **Step 1: Write the failing test**

Ajoute à `corkboard_test.go` :

```go
func TestCorkboardViewShowsRealPartHeaderReadOnly(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "prologue"), 0o755)
	os.MkdirAll(filepath.Join(dir, "the-letter"), 0o755)
	os.WriteFile(filepath.Join(dir, "prologue", "prologue.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, "the-letter", "the-letter.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, manifestName), []byte(
		`{"schemaVersion":2,"title":"N","items":[`+
			`{"chapter":{"folder":"prologue","title":"Prologue","texts":[{"file":"prologue.md","title":"Prologue"}]}},`+
			`{"part":"Part One","chapters":[`+
			`{"folder":"the-letter","title":"The Letter","texts":[{"file":"the-letter.md","title":"The Letter"}]}]}]}`), 0o644)

	t.Setenv("OKASHI_DIR", dir)
	m := initialModel()
	m.files.SetDir(dir)
	m.enterCorkboard()
	m.width, m.height = 100, 30
	view := m.corkboardView()

	if !strings.Contains(view, "Part One") {
		t.Fatalf("corkboard must show the real Part's title, got:\n%s", view)
	}
	// The staged (editable) chapters remain only the bare ones — Part One's chapter
	// is shown but not part of m.structureItems (structure mode doesn't edit Parts yet).
	if len(m.structureItems) != 1 || m.structureItems[0].folder != "prologue" {
		t.Fatalf("structureItems must still only stage the bare chapter, got %+v", m.structureItems)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run TestCorkboardViewShowsRealPartHeader -v 2>&1 | tail -40`
Expected: FAIL — "Part One" not found in `view` (current `corkboardView` only renders
`m.structureItems`, which never includes real Parts).

- [ ] **Step 3: Render real-Part cards read-only in `corkboardView`**

Lis `corkboardView` en entier (`corkboard.go`, ~ligne 429 au moment de ce plan) avant d'éditer — la
boucle actuelle (`for i := off; i < len(m.structureItems) && len(cards) < vis; i++`) doit devenir une
double boucle qui insère, pour chaque vraie Partie du manuscrit (relue séparément via
`resolveManuscript`, PAS via `m.structureItems`), un en-tête suivi de cartes non sélectionnables pour
ses chapitres. Le calcul de fenêtre visible (`off`, `vis`) reste basé sur `len(m.structureItems)`
(nombre de cartes ÉDITABLES) — les cartes des vraies Parties s'ajoutent en plus, après les cartes
éditables des chapitres nus, pas mélangées dans la même pagination.

```go
func (m model) corkboardView() string {
	const bodyRows = 3
	cardRows := bodyRows + 2
	perCard := cardRows + 1
	vis := (m.height - 5) / perCard
	if vis < 1 {
		vis = 1
	}
	off := homeWindowOffset(len(m.structureItems), m.structureSel, vis)
	cardW := max(40, min(m.width-8, 76))

	var cards []string
	for i := off; i < len(m.structureItems) && len(cards) < vis; i++ {
		it := m.structureItems[i]
		wc := ""
		var chPath string
		if len(it.texts) > 0 {
			chPath = filepath.Join(m.structureDir, it.folder, it.texts[0].file)
			if m.files.wc != nil {
				wc = commafy(m.files.wc.count(chPath)) + " m"
			}
		}
		isCurrent := m.currentFile != "" && chPath != "" && chPath == m.currentFile
		openMark, rawBody, dim := corkboardCardMeta(isCurrent, m.synopses[it.folder], m.corkFirstLines[it.folder])
		var body string
		if rawBody == "" {
			body = lipgloss.NewStyle().Foreground(subtle).Render("(pas de synopsis — e pour éditer)")
		} else {
			body = wrapClamp(rawBody, cardW-4, bodyRows)
			if dim {
				body = lipgloss.NewStyle().Foreground(subtle).Render(body)
			}
		}
		marker := "  "
		if i == m.structureSel {
			marker = selectedStyle.Render("▸ ")
		}
		hdr := marker + fmtNum(i+1) + " · " + openMark + it.title
		cards = append(cards, framedPanel(hdr, body, cardW, cardRows, wc))
	}

	// Real Parts (not staged in m.structureItems — structure mode doesn't edit
	// Parts yet) render read-only, after the editable bare-chapter cards.
	rv := resolveManuscript(m.structureDir, readEntries(m.structureDir))
	for _, p := range rv.parts {
		if p.title == "" {
			continue // synthetic part — its chapters are already the editable cards above
		}
		header := lipgloss.NewStyle().Bold(true).Foreground(accent).Render(
			p.title + "  " + commafy(partWordTotal(m.structureDir, p, m.files.wc)) + " m")
		cards = append(cards, header)
		for _, ch := range p.chapters {
			wc := ""
			if len(ch.texts) > 0 {
				wc = commafy(m.files.wc.count(filepath.Join(m.structureDir, ch.folder, ch.texts[0].file))) + " m"
			}
			_, rawBody, dim := corkboardCardMeta(false, m.synopses[ch.folder], m.corkFirstLines[ch.folder])
			var body string
			if rawBody == "" {
				body = lipgloss.NewStyle().Foreground(subtle).Render("(pas de synopsis)")
			} else {
				body = wrapClamp(rawBody, cardW-4, bodyRows)
				if dim {
					body = lipgloss.NewStyle().Foreground(subtle).Render(body)
				}
			}
			cards = append(cards, framedPanel("  "+ch.title, body, cardW, cardRows, wc))
		}
	}

	if len(cards) == 0 {
		cards = append(cards, lipgloss.NewStyle().Foreground(subtle).Render("(aucun chapitre)"))
	}

	var b strings.Builder
	board := strings.Join(cards, "\n")
	hdr := lipgloss.NewStyle().Foreground(subtle).Render(
		corkboardStatusLine(m.structureItems, m.structureDir, m.files.wc, m.goalsAll[m.structureDir].applyEnvDefaults()))
	b.WriteString(lipgloss.PlaceHorizontal(m.width, lipgloss.Center, hdr) + "\n")
	b.WriteString(lipgloss.Place(m.width, m.height-2, lipgloss.Center, lipgloss.Center, board))

	if m.synEditing {
		edit := framedPanel("synopsis · "+m.structureItems[m.structureSel].title, m.synArea.View(), cardW, 5, "esc enregistrer")
		b.WriteString("\n" + lipgloss.PlaceHorizontal(m.width, lipgloss.Center, edit))
		return b.String()
	}
	if m.structureRenaming {
		field := "renommer ▸ " + m.nameInput.View()
		b.WriteString("\n" + lipgloss.PlaceHorizontal(m.width, lipgloss.Center, field))
		return b.String()
	}
	if m.structureAdding {
		var picks []string
		for i, c := range m.structureAddChoices() {
			label := c.label
			if i == m.structureAddSel {
				label = selectedStyle.Render(label)
			}
			picks = append(picks, label)
		}
		if len(picks) == 0 {
			picks = append(picks, lipgloss.NewStyle().Foreground(subtle).Render("(aucune ressource à promouvoir)"))
		}
		pick := framedPanel("ajouter", strings.Join(picks, "\n"), max(30, min(m.width-8, 44)), len(picks)+2, "")
		b.WriteString("\n" + lipgloss.PlaceHorizontal(m.width, lipgloss.Center, pick))
		return b.String()
	}
	if m.structureConfirm {
		bar := lipgloss.NewStyle().Foreground(accent).Render("appliquer les modifications ? y appliquer · esc annuler")
		b.WriteString("\n" + lipgloss.PlaceHorizontal(m.width, lipgloss.Center, bar))
	}
	return b.String()
}
```

(Le reste de la fonction — `m.synEditing`, `m.structureRenaming`, `m.structureAdding`,
`m.structureConfirm` — est inchangé, reproduit ici pour clarté sur l'emplacement exact de l'insertion :
juste après le calcul de `cards` issu de `m.structureItems`, avant le test `len(cards) == 0`.)

Note d'implémentation : `partWordTotal` (Task 2, `filelist.go`) est réutilisée ici telle quelle —
aucune duplication. `readEntries` est déjà utilisée ailleurs dans `corkboard.go` (`corkChapterSet`),
donc son import est déjà couvert.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./... -run TestCorkboardViewShowsRealPartHeader -v 2>&1 | tail -40`
Expected: PASS.

- [ ] **Step 5: Run the full corkboard test file to check nothing else broke**

Run: `go test ./... -run TestCorkboard -v 2>&1 | tail -100`
Expected: all PASS, including pre-existing tests (none of which create a real Part, so the new
read-only section must render nothing extra for them — verify by reading a couple of existing test
assertions if any count cards or check exact `View()` output length).

- [ ] **Step 6: Build, vet, full test suite**

Run: `go build ./... && go vet ./... && go test ./... -count=1 2>&1 | tail -30`
Expected: clean, all PASS.

- [ ] **Step 7: Commit**

```bash
git add corkboard.go corkboard_test.go
git commit -m "$(cat <<'EOF'
Affiche les cartes des vraies Parties en lecture seule dans le corkboard

Le corkboard relit manuscriptView.parts séparément de m.structureItems
(qui reste le seul staging éditable, limité aux chapitres nus) pour
afficher, après les cartes éditables, un en-tête + les cartes de
chaque vraie Partie du manifest — navigation/promotion/suppression
inchangées, ces cartes ne sont pas sélectionnables. L'édition des
Parties reste le sujet du plan Création/réorganisation.
EOF
)"
```

---

## Task 8: Vérification manuelle

**Files:** none — vérification uniquement, aucun changement de code.

**Interfaces:**
- Consumes: la fonctionnalité complète des Tasks 1–7.
- Produces: rien de nouveau — confirme que le rendu Partie et le sélecteur de texte fonctionnent
  ensemble sur un vrai manuscrit, dans un vrai terminal.

- [ ] **Step 1: Full build, vet, test suite**

Run: `go build ./... && go vet ./... && go test ./... -count=1 2>&1 | tail -40`
Expected: clean build, no vet warnings, all tests PASS.

- [ ] **Step 2: Manual walkthrough in tmux — construit un manuscrit v2 avec une Partie et un chapitre multi-textes**

```bash
go build -o /tmp/forkashi-reading-check .
rm -rf /tmp/reading-check-home /tmp/reading-check-proj
mkdir -p /tmp/reading-check-home /tmp/reading-check-proj/Livre/prologue /tmp/reading-check-proj/Livre/chapitre-un
cat > /tmp/reading-check-proj/Livre/manifest.json <<'EOF'
{"schemaVersion":2,"title":"Mon Livre","items":[
  {"chapter":{"folder":"prologue","title":"Prologue","texts":[{"file":"prologue.md","title":"Prologue"}]}},
  {"part":"Partie Un","chapters":[
    {"folder":"chapitre-un","title":"Chapitre Un","texts":[
      {"file":"scene-un.md","title":"Scène d'ouverture"},
      {"file":"scene-deux.md","title":"Scène de confrontation"}
    ]}
  ]}
]}
EOF
echo "Il était une fois." > /tmp/reading-check-proj/Livre/prologue/prologue.md
echo "Le matin se leva." > /tmp/reading-check-proj/Livre/chapitre-un/scene-un.md
echo "Elle le confronta enfin." > /tmp/reading-check-proj/Livre/chapitre-un/scene-deux.md
tmux new-session -d -s readingcheck -x 120 -y 40 \
  "cd /tmp/reading-check-proj && HOME=/tmp/reading-check-home XDG_CONFIG_HOME=/tmp/reading-check-home/.config OKASHI_DIR=/tmp/reading-check-proj /tmp/forkashi-reading-check"
sleep 1
tmux capture-pane -t readingcheck -p
```

Walk through, in order:

1. Ouvre "Livre" depuis le hub — confirme que le panneau FICHIERS du hub affiche déjà l'en-tête
   "Partie Un" avec son total de mots (4 mots : "Le matin se leva." + "Elle le confronta enfin."),
   avant l'entrée "Chapitre Un".
2. `⏎` pour entrer dans le manuscrit — confirme que la sidebar affiche "Prologue" (chapitre nu),
   puis une ligne d'en-tête "Partie Un · 4 m" (non surlignable), puis "Chapitre Un" en dessous.
3. Navigue avec `↓` depuis "Prologue" — confirme que le curseur saute directement de "Prologue" à
   "Chapitre Un", sans jamais s'arrêter sur la ligne d'en-tête "Partie Un".
4. `⏎` sur "Prologue" — confirme l'ouverture directe dans l'éditeur (comportement inchangé, un seul
   texte).
5. Retour à la sidebar (`esc` ou navigation), `⏎` sur "Chapitre Un" — confirme l'ouverture du
   sélecteur de texte, listant "Scène d'ouverture" (3 mots) et "Scène de confrontation" (4 mots),
   chacun avec son propre compteur.
6. `↓` puis `⏎` sur "Scène de confrontation" — confirme l'ouverture de ce texte précis dans l'éditeur
   (`scene-deux.md`, contenu "Elle le confronta enfin.").
7. Retour à la sidebar, `⏎` sur "Chapitre Un" à nouveau, cette fois `Échap` — confirme le retour à la
   sidebar sans qu'aucun fichier ne s'ouvre.
8. `c` pour ouvrir le corkboard — confirme que la carte "Prologue" est éditable/navigable (staging), et
   qu'une carte d'en-tête "Partie Un" apparaît après, suivie d'une carte "Chapitre Un" non sélectionnable
   (vérifie que `J`/`K` sur la sélection courante ne peut pas atteindre cette carte).
9. `m` pour ouvrir le pager (lecture continue) — confirme l'ordre : "── Prologue ──", son texte, puis
   "── Partie Un ──", puis "── Chapitre Un ──", puis le texte du premier texte du chapitre (seul le
   premier — comportement dégradé assumé, la concaténation est le plan Export).

Documente toute anomalie visuelle (désalignement, débordement, curseur mal positionné) trouvée pendant
ce parcours — fichier:ligne si la cause est identifiable, sinon comme finding à traiter.

- [ ] **Step 3: Clean up**

```bash
tmux send-keys -t readingcheck 'C-c'
tmux kill-session -t readingcheck 2>/dev/null
rm -f /tmp/forkashi-reading-check
rm -rf /tmp/reading-check-home /tmp/reading-check-proj
```

- [ ] **Step 4: Report findings**

No commit for this task (verification only). Si une anomalie apparaît, la signaler — corriger
seulement si c'est trivial et manifestement sûr (comme le correctif de rafraîchissement de sidebar
trouvé à la Task 6 du plan Fondations), sinon la traiter comme un suivi pour le plan Export ou
Création/réorganisation.

---

## Self-Review

**Spec coverage** (contre `docs/superpowers/specs/2026-08-04-parts-and-multi-text-chapters-design.md`,
sections dans le périmètre de ce plan) :
- Sidebar : en-tête de Partie (titre + total de mots), non sélectionnable → Task 1 + Task 2.
- Ouverture d'un chapitre (§"Ouverture d'un chapitre depuis la sidebar") : texte unique → ouverture
  directe inchangée ; 2+ textes → sélecteur avec compteur par texte → Task 3 + Task 4.
- Hub : en-têtes de Partie (décision prise en clarification avec l'utilisateur, au-delà de la spec
  écrite qui ne mentionnait pas le hub) → Task 5.
- Pager : ligne de titre de Partie avant ses chapitres → Task 6.
- Corkboard : regroupement en en-têtes de section (§"Corkboard : parties" de la spec) → Task 7, avec la
  contrainte que l'édition (structure mode) reste hors périmètre — seule la lecture est couverte.
- Hors périmètre annoncé dans Global Constraints, non traité par ce plan : export (concaténation,
  rupture de section), création/réorganisation de Partie, ajout d'un texte à un chapitre existant —
  tous explicitement réservés aux deux plans suivants.

**Placeholder scan:** aucun "TBD"/"TODO" ; chaque step de code contient le code réel à écrire. Les
quelques instructions "lis le fichier réel avant d'éditer, les numéros de ligne ont pu se décaler"
(Task 3 Step 4, Task 5 Step 3, Task 7 Step 3) ne sont pas des placeholders — elles reflètent une leçon
directe du plan Fondations, où plusieurs implémenteurs ont dû vérifier empiriquement le code réel
avant d'accepter un fragment littéral du plan, et le documenter comme déviation le cas échéant.

**Type consistency:** `fileEntry{name, isDir, isPartHeader}` (Task 1) est le type utilisé
identiquement dans Task 2 (sidebar) et Task 5 (`homeFileItem`, type distinct mais même principe de
champ, pas de confusion entre les deux). `activateResult`/`activateFile`/`activateTextPicker`/
`activateNone` (Task 3) sont utilisés identiquement dans Task 4 (main.go). `chapterByFolder` (Task 3)
est réutilisée telle quelle par Task 4 (`enterTextPicker`) — pas de redéfinition. `partWordTotal`
(Task 2, `filelist.go`) est réutilisée telle quelle par Task 5 (`home.go`) et Task 7 (`corkboard.go`)
— une seule définition, trois appelants, cohérent avec DRY.
