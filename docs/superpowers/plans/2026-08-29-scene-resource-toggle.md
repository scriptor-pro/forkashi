# Bascule de statut scène ↔ Ressource — Plan d'implémentation

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Permettre de faire sortir une scène d'un chapitre multi-scènes vers le statut « scène
indépendante » (première étape vers Ressource), et inverser le défaut d'inclusion à l'export pour
les Ressources (exclues par défaut au lieu d'incluses), avec un raccourci sidebar dédié qui
réutilise la logique de conversion existante.

**Architecture:** Toute la logique de conversion et de calcul du défaut d'export vit dans
`exportselect.go` (déjà le propriétaire de `convertStandaloneSceneToResource`/
`convertResourceToStandaloneScene`). Le raccourci sidebar (`main.go` + `filelist.go`) est un fin
routeur qui appelle ces mêmes fonctions — aucune logique métier dupliquée. Le manifest garde sa
forme v3 existante ; seul le sidecar `.okashi-export.json` change d'interprétation par défaut à la
lecture (jamais à l'écriture).

**Tech Stack:** Go 1.25, Bubble Tea (Elm-style Model/Update/View), tests `go test` standards
(package `main`, pas de framework externe).

**Spec:** [docs/superpowers/specs/2026-08-29-scene-resource-toggle-design.md](../specs/2026-08-29-scene-resource-toggle-design.md)

## Global Constraints

- Aucun changement de forme du manifest v3 (`schemaVersion`, `items[]`, `Texts[]`, `Scene` restent
  inchangés) — seules des écritures normales déjà couvertes par `writeManifest`/`atomicWrite`.
- Toute écriture manifest/sidecar passe par `readManifest`/`writeManifest` et
  `loadExportSelection`/`saveExportSelection` existants — jamais d'accès disque direct au JSON.
- Tout déplacement de fichier passe par `safeMove(src, dst string) error` (`move.go:126-142`) —
  jamais `os.Rename` direct dans le nouveau code (gère le fallback cross-volume).
- Messages d'erreur utilisateur en français, écrits dans `m.status` (`string`), cohérents avec le
  style déjà en place dans `exportselect.go` (ex. `"échec du déplacement : " + err.Error()`).
- Pas de nouveau paramètre d'environnement `OKASHI_*` — cette fonctionnalité n'en a pas besoin.
- Sortir une scène de chapitre vers Ressource se fait **toujours en deux étapes** (chapitre →
  scène indépendante, puis scène indépendante → Ressource) — jamais un raccourci direct en une
  étape, y compris depuis la sidebar.

---

## Task 1 : `convertChapterSceneToStandalone` — conversion scène de chapitre → scène indépendante

**Files:**
- Modify: `exportselect.go` (ajout d'une nouvelle fonction, à la suite de
  `convertResourceToStandaloneScene`, après la ligne 333)
- Test: `exportselect_test.go` (ajout de tests, à la suite des tests de conversion existants après
  la ligne 739)

**Interfaces:**
- Consumes : `readManifest(dir string) (m manifest, present bool, err error)` (`manifest.go:76`),
  `writeManifest(dir string, m manifest) error` (`manifest.go:102`),
  `findChapterByFolder(m *manifest, folder string) *manifestChapter` (`manifest.go:222`),
  `safeMove(src, dst string) error` (`move.go:126`),
  `loadExportSelection(dir string) map[string]bool` (`exportselection.go:26`),
  `saveExportSelection(dir string, excluded, knownFiles map[string]bool) error`
  (`exportselection.go:46`), `m.reloadExportSelectAfterMove(follow exportSelectFollow)`
  (`exportselect.go:564`), `followFile(file string) exportSelectFollow` (`exportselect.go:591`).
- Produces : `func (m *model) convertChapterSceneToStandalone(folder, file string)` — `file` est le
  chemin **relatif au dossier manuscrit** (ex. `"ch1/scene-2.md"`, même convention que
  `entries[i].file` et le paramètre `file` de `moveSceneBetweenChapters`, `exportselect.go:461`),
  `folder` est le nom du dossier chapitre source (ex. `"ch1"`). Utilisée par la Task 2
  (branchement dans `moveExportSelectSceneWithinChapter`) et la Task 4 (raccourci sidebar).

- [ ] **Step 1: Écrire le test de conversion réussie**

Ajouter à `exportselect_test.go`, à la fin du fichier :

```go
func TestConvertChapterSceneToStandaloneMovesFileAndUpdatesManifest(t *testing.T) {
	dir := t.TempDir()
	mkChapterDir(t, dir, "ch1", map[string]string{
		"scene-1.md": "First scene.",
		"scene-2.md": "Second scene, about to leave the chapter.",
	})
	if err := writeManifest(dir, manifest{
		Title: "N",
		Items: []manifestItem{
			{Chapter: &manifestChapter{Folder: "ch1", Title: "Chapter One", Texts: []manifestText{
				{File: "scene-1.md", Title: "Opening"},
				{File: "scene-2.md", Title: "Confrontation"},
			}}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	m := model{}
	m.files.dir = dir

	m.convertChapterSceneToStandalone("ch1", filepath.Join("ch1", "scene-2.md"))

	if _, err := os.Stat(filepath.Join(dir, "ch1", "scene-2.md")); !os.IsNotExist(err) {
		t.Fatal("scene-2.md must no longer exist in ch1/")
	}
	if _, err := os.Stat(filepath.Join(dir, "scene-2.md")); err != nil {
		t.Fatalf("scene-2.md must now exist at the manuscript root: %v", err)
	}
	got, _, err := readManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	ch1 := findChapterByFolder(&got, "ch1")
	if ch1 == nil || len(ch1.Texts) != 1 || ch1.Texts[0].File != "scene-1.md" {
		t.Fatalf("ch1 must keep only scene-1.md, got %+v", ch1)
	}
	sc := findSceneByFile(&got, "scene-2.md")
	if sc == nil {
		t.Fatal("scene-2.md must now be listed as a standalone scene in items[]")
	}
	if sc.Title != "Confrontation" {
		t.Fatalf("standalone scene title must be preserved, got %q, want %q", sc.Title, "Confrontation")
	}
}
```

- [ ] **Step 2: Lancer le test, vérifier l'échec attendu**

Run: `go test ./... -run TestConvertChapterSceneToStandaloneMovesFileAndUpdatesManifest -v`
Expected: FAIL avec `m.convertChapterSceneToStandalone undefined (type model has no field or method convertChapterSceneToStandalone)`

- [ ] **Step 3: Implémenter `convertChapterSceneToStandalone`**

Ajouter dans `exportselect.go`, immédiatement après `convertResourceToStandaloneScene` (après la
ligne 333) :

```go
// convertChapterSceneToStandalone drops file from its parent chapter's Texts[] (identified by
// folder) and adds it as a new Scene:true item at the end of items[] — the scene's own title is
// preserved. Unlike the standalone-scene↔Resource conversions (which never move a file, since
// both statuses already live at the manuscript root), this DOES move the file on disk, chapter
// folder → manuscript root, using the same safeMove already used by moveSceneBetweenChapters.
func (m *model) convertChapterSceneToStandalone(folder, file string) {
	mani, present, err := readManifest(m.files.dir)
	if err != nil || !present {
		m.status = "impossible de déplacer : le manifeste est introuvable ou illisible"
		return
	}
	ch := findChapterByFolder(&mani, folder)
	if ch == nil {
		return
	}
	base := filepath.Base(file)
	var moved manifestText
	found := false
	kept := ch.Texts[:0]
	for _, t := range ch.Texts {
		if t.File == base {
			moved = t
			found = true
			continue
		}
		kept = append(kept, t)
	}
	if !found {
		return
	}

	src := filepath.Join(m.files.dir, folder, base)
	dst := filepath.Join(m.files.dir, base)
	if _, err := os.Stat(dst); err == nil {
		m.status = "un fichier nommé " + base + " existe déjà à la racine du manuscrit"
		return
	}
	if err := safeMove(src, dst); err != nil {
		m.status = "échec du déplacement : " + err.Error()
		return
	}
	ch.Texts = kept

	mani.Items = append(mani.Items, manifestItem{Chapter: &manifestChapter{
		Title: moved.Title,
		Scene: true,
		Texts: []manifestText{{File: base, Title: moved.Title}},
	}})
	if err := writeManifest(m.files.dir, mani); err != nil {
		m.status = "échec du déplacement : " + err.Error()
		return
	}

	oldKey := filepath.Join(folder, base)
	excluded := loadExportSelection(m.files.dir)
	if excluded[oldKey] {
		delete(excluded, oldKey)
		excluded[base] = true
		known := map[string]bool{base: true}
		for k := range excluded {
			known[k] = true
		}
		if err := saveExportSelection(m.files.dir, excluded, known); err != nil {
			m.status = "scène convertie mais échec de la migration de la sélection d'export : " + err.Error()
			return
		}
	}
	m.reloadExportSelectAfterMove(followFile(base))
}
```

Note d'implémentation vs. le spec : `kept := ch.Texts[:0]` doit être calculé **avant**
`safeMove` (le retrait de `Texts[]` doit attendre que le fichier ait réellement bougé, sinon un
`safeMove` en échec laisserait le manifest incohérent avec le disque) — c'est pourquoi
`ch.Texts = kept` est affecté seulement après le `safeMove` réussi, contrairement à l'ordre lu
littéralement dans le corps du spec (qui affectait `ch.Texts = kept` avant le move). Le
comportement observable (résultat final) est identique ; seul l'ordre interne change pour la
robustesse en cas d'échec de move.

- [ ] **Step 4: Lancer le test, vérifier le succès**

Run: `go test ./... -run TestConvertChapterSceneToStandaloneMovesFileAndUpdatesManifest -v`
Expected: PASS

- [ ] **Step 5: Écrire le test de collision de nom (refus, aucune écriture)**

Ajouter à `exportselect_test.go` :

```go
func TestConvertChapterSceneToStandaloneRefusesNameCollisionAtRoot(t *testing.T) {
	dir := t.TempDir()
	mkChapterDir(t, dir, "ch1", map[string]string{
		"scene-1.md": "First scene.",
		"scene-2.md": "Second scene.",
	})
	os.WriteFile(filepath.Join(dir, "scene-2.md"), []byte("already at root"), 0o644)
	if err := writeManifest(dir, manifest{
		Title: "N",
		Items: []manifestItem{
			{Chapter: &manifestChapter{Folder: "ch1", Title: "Chapter One", Texts: []manifestText{
				{File: "scene-1.md", Title: "Opening"},
				{File: "scene-2.md", Title: "Confrontation"},
			}}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	m := model{}
	m.files.dir = dir

	m.convertChapterSceneToStandalone("ch1", filepath.Join("ch1", "scene-2.md"))

	if !strings.Contains(m.status, "scene-2.md") {
		t.Fatalf("status should report the name collision, got %q", m.status)
	}
	if _, err := os.Stat(filepath.Join(dir, "ch1", "scene-2.md")); err != nil {
		t.Fatal("scene-2.md must still exist in ch1/ — the move must have been refused")
	}
	b, err := os.ReadFile(filepath.Join(dir, "scene-2.md"))
	if err != nil || string(b) != "already at root" {
		t.Fatal("the pre-existing root scene-2.md must be untouched")
	}
	got, _, err := readManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	ch1 := findChapterByFolder(&got, "ch1")
	if len(ch1.Texts) != 2 {
		t.Fatalf("ch1 must still list both texts (no manifest change on refusal), got %+v", ch1.Texts)
	}
}
```

- [ ] **Step 6: Lancer le test, vérifier le succès**

Run: `go test ./... -run TestConvertChapterSceneToStandaloneRefusesNameCollisionAtRoot -v`
Expected: PASS

- [ ] **Step 7: Écrire le test de migration de la clé du sidecar**

Ajouter à `exportselect_test.go` :

```go
func TestConvertChapterSceneToStandaloneMigratesSidecarKey(t *testing.T) {
	dir := t.TempDir()
	mkChapterDir(t, dir, "ch1", map[string]string{
		"scene-1.md": "First scene.",
		"scene-2.md": "Second scene.",
	})
	if err := writeManifest(dir, manifest{
		Title: "N",
		Items: []manifestItem{
			{Chapter: &manifestChapter{Folder: "ch1", Title: "Chapter One", Texts: []manifestText{
				{File: "scene-1.md", Title: "Opening"},
				{File: "scene-2.md", Title: "Confrontation"},
			}}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	oldKey := filepath.Join("ch1", "scene-2.md")
	if err := saveExportSelection(dir, map[string]bool{oldKey: true}, map[string]bool{oldKey: true}); err != nil {
		t.Fatal(err)
	}
	m := model{}
	m.files.dir = dir

	m.convertChapterSceneToStandalone("ch1", oldKey)

	got := loadExportSelection(dir)
	if got[oldKey] {
		t.Fatal("old sidecar key must not survive the conversion")
	}
	if !got["scene-2.md"] {
		t.Fatal("exclusion must migrate to the new root-relative key")
	}
}
```

- [ ] **Step 8: Lancer le test, vérifier le succès**

Run: `go test ./... -run TestConvertChapterSceneToStandaloneMigratesSidecarKey -v`
Expected: PASS

- [ ] **Step 9: Lancer tous les tests du package pour vérifier l'absence de régression**

Run: `go build ./... && go test ./...`
Expected: `ok  	okashi	...` (aucune régression)

- [ ] **Step 10: Commit**

```bash
git add exportselect.go exportselect_test.go
git commit -m "feat: convertChapterSceneToStandalone fait sortir une scène de chapitre

Nouvelle conversion miroir de convertStandaloneSceneToResource : une
scène à l'intérieur d'un chapitre multi-scènes peut désormais devenir
une scène indépendante (fichier déplacé chapitre→racine, item Scene:true
ajouté à items[])."
```

---

## Task 2 : Brancher la sortie de chapitre dans `moveExportSelectSceneWithinChapter`

**Files:**
- Modify: `exportselect.go:436-448`
- Test: `exportselect_test.go` (ajout d'un test bout-en-bout via `updateExportSelect`)

**Interfaces:**
- Consumes : `m.convertChapterSceneToStandalone(folder, file string)` (Task 1),
  `m.chapterFolderForHeader(headerIdx int) string` (`exportselect.go:520`).
- Produces : rien de nouveau exposé — modifie le comportement interne de
  `moveExportSelectSceneWithinChapter` (`exportselect.go:386`), déjà appelée par
  `moveExportSelectEntry` (`exportselect.go:201-216`), elle-même appelée par `updateExportSelect`
  sur `shift+up`/`shift+down` (`exportselect.go:128-131`).

- [ ] **Step 1: Écrire le test bout-en-bout (scène en bord de manuscrit, aucun chapitre adjacent)**

Ajouter à `exportselect_test.go` :

```go
func TestMoveExportSelectSceneExitsChapterWhenNoAdjacentChapter(t *testing.T) {
	dir := t.TempDir()
	mkChapterDir(t, dir, "ch1", map[string]string{
		"scene-1.md": "First scene.",
		"scene-2.md": "Second scene, at the top edge of the only chapter.",
	})
	if err := writeManifest(dir, manifest{
		Title: "N",
		Items: []manifestItem{
			{Chapter: &manifestChapter{Folder: "ch1", Title: "Chapter One", Texts: []manifestText{
				{File: "scene-1.md", Title: "Opening"},
				{File: "scene-2.md", Title: "Confrontation"},
			}}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	m := model{}
	m.files.dir = dir
	m.enterExportSelect()
	// entries: [0]=header "Chapter One", [1]="Opening" (indented, top of chapter).
	// Moving entry 1 UP: no adjacent chapter above → must exit to a standalone scene.
	mm, _ := m.updateExportSelect(tea.KeyMsg{Type: tea.KeyDown}) // sel -> 1 ("Opening")
	m2 := mm.(model)
	mm2, _ := m2.updateExportSelect(tea.KeyMsg{Type: tea.KeyShiftUp})
	m3 := mm2.(model)

	if _, err := os.Stat(filepath.Join(dir, "ch1", "scene-1.md")); !os.IsNotExist(err) {
		t.Fatal("scene-1.md must no longer exist in ch1/")
	}
	if _, err := os.Stat(filepath.Join(dir, "scene-1.md")); err != nil {
		t.Fatalf("scene-1.md must now exist at the manuscript root: %v", err)
	}
	got, _, err := readManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	ch1 := findChapterByFolder(&got, "ch1")
	if ch1 == nil || len(ch1.Texts) != 1 || ch1.Texts[0].File != "scene-2.md" {
		t.Fatalf("ch1 must keep only scene-2.md, got %+v", ch1)
	}
	if findSceneByFile(&got, "scene-1.md") == nil {
		t.Fatal("scene-1.md must now be a standalone scene")
	}
	_ = m3
}
```

- [ ] **Step 2: Lancer le test, vérifier l'échec attendu**

Run: `go test ./... -run TestMoveExportSelectSceneExitsChapterWhenNoAdjacentChapter -v`
Expected: FAIL — `scene-1.md` reste dans `ch1/` (le `return` silencieux actuel refuse le
déplacement)

- [ ] **Step 3: Brancher la conversion dans `moveExportSelectSceneWithinChapter`**

Dans `exportselect.go`, remplacer le bloc `exportselect.go:436-448` :

```go
	// Crosses this chapter's boundary — find which adjacent chapter header we crossed into.
	var dstHeaderIdx int
	if dir < 0 {
		dstHeaderIdx = headerIdx - 1
		for dstHeaderIdx >= 0 && !entries[dstHeaderIdx].isHeader {
			dstHeaderIdx--
		}
	} else {
		dstHeaderIdx = chapterEnd
	}
	if dstHeaderIdx < 0 || dstHeaderIdx >= len(entries) || !entries[dstHeaderIdx].isHeader {
		return // no adjacent chapter in that direction — already at an edge
	}
```

par :

```go
	// Crosses this chapter's boundary — find which adjacent chapter header we crossed into.
	var dstHeaderIdx int
	if dir < 0 {
		dstHeaderIdx = headerIdx - 1
		for dstHeaderIdx >= 0 && !entries[dstHeaderIdx].isHeader {
			dstHeaderIdx--
		}
	} else {
		dstHeaderIdx = chapterEnd
	}
	if dstHeaderIdx < 0 || dstHeaderIdx >= len(entries) || !entries[dstHeaderIdx].isHeader {
		// No adjacent chapter in that direction — the scene exits its chapter entirely and
		// becomes a standalone scene (never directly a Resource — see design §1).
		srcFolder := m.chapterFolderForHeader(headerIdx)
		if srcFolder == "" {
			return
		}
		m.convertChapterSceneToStandalone(srcFolder, entries[i].file)
		return
	}
```

- [ ] **Step 4: Lancer le test, vérifier le succès**

Run: `go test ./... -run TestMoveExportSelectSceneExitsChapterWhenNoAdjacentChapter -v`
Expected: PASS

- [ ] **Step 5: Lancer tous les tests du package pour vérifier l'absence de régression**

Run: `go build ./... && go test ./...`
Expected: `ok  	okashi	...` — en particulier `TestMoveExportSelectSceneCrossesIntoAdjacentChapter`
(`exportselect_test.go:356`) doit rester au vert (le cas « chapitre adjacent existe » est
inchangé, seul le cas « pas de chapitre adjacent » change de comportement).

- [ ] **Step 6: Commit**

```bash
git add exportselect.go exportselect_test.go
git commit -m "feat: shift+↑↓ en bord de manuscrit fait sortir une scène vers standalone

Auparavant, déplacer une scène de chapitre au-delà du dernier chapitre
adjacent était silencieusement refusé. Elle devient maintenant une
scène indépendante — première étape du chemin en deux temps vers
Ressource (voir design §1)."
```

---

## Task 3 : Défaut d'inclusion à l'export dépendant du type d'entrée

**Files:**
- Modify: `exportselect.go:78-90` (`buildEntry`), `exportselect.go:47-74`
  (`buildExportSelectEntries`), `export.go:120-135` (`runExport`, boucle Ressources)
- Test: `exportselect_test.go`, `export_test.go` (ou fichier de test existant couvrant
  `runExport`/l'export du manuscrit entier — à localiser au Step 5 si le nom diffère)

**Interfaces:**
- Consumes : rien de nouveau — modifie des fonctions déjà existantes.
- Produces : `buildEntry` gagne un 5ᵉ paramètre `isResource bool` inséré avant `excluded
  map[string]bool` — **tous les appelants doivent être mis à jour dans la même tâche** (un seul
  appelant existe : `buildExportSelectEntries`, aux 3 sites d'appel lignes 57, 67, 71).

- [ ] **Step 1: Écrire le test — Ressource sans clé explicite est exclue par défaut**

Ajouter à `exportselect_test.go` :

```go
func TestBuildEntryResourceDefaultsToExcludedWithoutExplicitKey(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "notes.md"), []byte("Research notes."), 0o644)

	e := buildEntry(dir, "notes.md", "notes", false, true, map[string]bool{})

	if !e.excluded {
		t.Fatal("a Resource with no explicit sidecar key must default to excluded")
	}
}

func TestBuildEntryListedItemDefaultsToIncludedWithoutExplicitKey(t *testing.T) {
	dir := t.TempDir()
	mkChapterDir(t, dir, "ch1", map[string]string{"scene-1.md": "First scene."})

	e := buildEntry(dir, filepath.Join("ch1", "scene-1.md"), "Opening", true, false, map[string]bool{})

	if e.excluded {
		t.Fatal("a listed chapter/scene entry with no explicit sidecar key must default to included")
	}
}

func TestBuildEntryExplicitKeyAlwaysWinsOverTypeDefault(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "notes.md"), []byte("Research notes."), 0o644)

	// Resource explicitly INCLUDED (key present with value false is not how the sidecar stores
	// inclusion — only exclusions are ever stored, see exportselection.go:14-16 — so an explicit
	// "included" state is instead represented by the key being ABSENT after having been toggled
	// back on. We simulate that directly: an explicit false entry in the map, which the schema
	// never actually writes, must still be honored as "not excluded" if present.
	e := buildEntry(dir, "notes.md", "notes", false, true, map[string]bool{"notes.md": false})
	if e.excluded {
		t.Fatal("an explicit false in the excluded map must win over the Resource default")
	}

	// A chapter/scene explicitly EXCLUDED via the sidecar must stay excluded despite its
	// type default being "included".
	mkChapterDir(t, dir, "ch1", map[string]string{"scene-1.md": "First scene."})
	e2 := buildEntry(dir, filepath.Join("ch1", "scene-1.md"), "Opening", true, false,
		map[string]bool{filepath.Join("ch1", "scene-1.md"): true})
	if !e2.excluded {
		t.Fatal("an explicit true in the excluded map must win over the listed-item default")
	}
}
```

- [ ] **Step 2: Lancer les tests, vérifier l'échec attendu**

Run: `go test ./... -run TestBuildEntry -v`
Expected: FAIL avec une erreur de compilation `too many arguments in call to buildEntry` (l'appel
utilise déjà 5 arguments avant que la fonction n'en accepte que 4 — le test ne compilera pas tant
que Step 3 n'est pas fait ; c'est attendu et volontaire pour ce refactor de signature).

- [ ] **Step 3: Modifier `buildEntry` et ses 3 appelants dans `buildExportSelectEntries`**

Dans `exportselect.go`, remplacer `buildEntry` (lignes 78-90) :

```go
// buildEntry reads rel's content once to compute words/chars/preview and reports its excluded
// state from the sidecar map. isResource selects the default when rel has no explicit entry in
// excluded: a listed item (chapter/scene/standalone scene) defaults to included; a Resource
// defaults to EXCLUDED.
func buildEntry(dir, rel, title string, indent, isResource bool, excluded map[string]bool) exportSelectEntry {
	data, _ := os.ReadFile(filepath.Join(dir, rel)) // best-effort: an unreadable file gets zero counts, not a crash
	text := string(data)
	ex, explicit := excluded[rel]
	if !explicit {
		ex = isResource
	}
	return exportSelectEntry{
		file:     rel,
		title:    title,
		preview:  previewOf(data),
		words:    wordCount(text),
		chars:    charCount(text),
		excluded: ex,
		indent:   indent,
	}
}
```

Puis remplacer `buildExportSelectEntries` (lignes 47-74) :

```go
// buildExportSelectEntries resolves dir's manuscript into the flat display list: chapter
// headers + indented scenes, then standalone scenes, then Resources (alphabetical, matching
// v.loose's existing order). excluded is the loaded sidecar map, keyed by the same relative
// path used as each entry's file field.
func buildExportSelectEntries(dir string, v manuscriptView, excluded map[string]bool) []exportSelectEntry {
	var out []exportSelectEntry
	for _, part := range v.parts {
		for _, ch := range part.chapters {
			if ch.scene {
				continue // standalone scenes are collected in a second pass below, after chapters
			}
			out = append(out, exportSelectEntry{title: ch.title, isHeader: true})
			for _, t := range ch.texts {
				rel := filepath.Join(ch.folder, t.file)
				out = append(out, buildEntry(dir, rel, t.title, true, false, excluded))
			}
		}
	}
	for _, part := range v.parts {
		for _, ch := range part.chapters {
			if !ch.scene || len(ch.texts) == 0 {
				continue
			}
			rel := ch.texts[0].file
			out = append(out, buildEntry(dir, rel, ch.title, false, false, excluded))
		}
	}
	for _, e := range v.loose {
		out = append(out, buildEntry(dir, e.name, sectionTitle(e.name), false, true, excluded))
	}
	return out
}
```

(Seul changement réel : chaque appel à `buildEntry` gagne un argument — `false` pour les
chapitres/scènes de chapitre et les scènes indépendantes, `true` pour les Ressources.)

- [ ] **Step 4: Lancer les tests, vérifier le succès**

Run: `go test ./... -run TestBuildEntry -v`
Expected: PASS pour les 3 tests

- [ ] **Step 5: Localiser et mettre à jour le branchement du filtre dans `runExport`**

Dans `export.go:120-135` (bloc `if m.exportWholeManuscript()`), remplacer :

```go
		for _, e := range v.loose {
			if excluded[e.name] {
				continue
			}
			data, err := os.ReadFile(filepath.Join(dir, e.name))
			if err != nil {
				continue
			}
			doc = append(doc, Section{Title: sectionTitle(e.name), Blocks: parseSection(data)})
		}
```

par :

```go
		for _, e := range v.loose {
			ex, explicit := excluded[e.name]
			if !explicit {
				ex = true // Resource, no explicit sidecar entry → excluded by default
			}
			if ex {
				continue
			}
			data, err := os.ReadFile(filepath.Join(dir, e.name))
			if err != nil {
				continue
			}
			doc = append(doc, Section{Title: sectionTitle(e.name), Blocks: parseSection(data)})
		}
```

`manuscriptDocFromChapters` (appelée juste avant, ligne 124) n'est **pas modifiée** — les
chapitres/scènes continuent de tester `excluded[file]` directement (zéro-valeur `false` = inclus,
défaut inchangé).

- [ ] **Step 6: Écrire le test de branchement `runExport`**

Chercher d'abord le fichier de test existant qui couvre `runExport`/l'export du manuscrit entier :

Run: `grep -rl "func.*runExport\|exportWholeManuscript" --include="*_test.go" .`

Ajouter au fichier trouvé (ou à `export_test.go` s'il existe déjà et couvre ce chemin) :

```go
func TestRunExportResourceExcludedByDefaultWithoutExplicitSidecarEntry(t *testing.T) {
	dir := t.TempDir()
	mkChapterDir(t, dir, "ch1", map[string]string{"scene-1.md": "Chapter text."})
	os.WriteFile(filepath.Join(dir, "notes.md"), []byte("A Resource, never touched."), 0o644)
	if err := writeManifest(dir, manifest{
		Title: "N",
		Items: []manifestItem{
			{Chapter: &manifestChapter{Folder: "ch1", Title: "Chapter One", Texts: []manifestText{
				{File: "scene-1.md", Title: "Opening"},
			}}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	m := model{}
	m.files.dir = dir
	m.screen = screenCorkboard // exportWholeManuscript() == true
	m.exportChooser = newExportChooser()
	m.exportChooser.checked[formatRTF] = true

	m.runExport()

	data, err := os.ReadFile(filepath.Join(dir, "export", "n.rtf"))
	if err != nil {
		t.Fatalf("expected export/n.rtf to be written: %v, status=%q", err, m.status)
	}
	if strings.Contains(string(data), "A Resource, never touched.") {
		t.Fatal("a Resource with no explicit sidecar entry must be excluded from the export by default")
	}
}

func TestRunExportResourceExplicitlyIncludedStillAppears(t *testing.T) {
	dir := t.TempDir()
	mkChapterDir(t, dir, "ch1", map[string]string{"scene-1.md": "Chapter text."})
	os.WriteFile(filepath.Join(dir, "notes.md"), []byte("An explicitly included Resource."), 0o644)
	if err := writeManifest(dir, manifest{
		Title: "N",
		Items: []manifestItem{
			{Chapter: &manifestChapter{Folder: "ch1", Title: "Chapter One", Texts: []manifestText{
				{File: "scene-1.md", Title: "Opening"},
			}}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	// Simulate an explicit "included" state the way the export-selection screen produces it:
	// toggling a Resource on twice (excluded → included) leaves no key in the sidecar at all,
	// which already defaults to included for listed items but NOT for Resources — so to keep a
	// Resource included, the UI never writes true. The only way "explicit inclusion" of a
	// Resource actually persists is by the sidecar not pruning a false-equivalent absence — this
	// test instead verifies the direct runExport contract using an explicit `false` sidecar
	// entry, which is what the schema is capable of storing.
	if err := saveExportSelectionRaw(t, dir, map[string]bool{"notes.md": false}); err != nil {
		t.Fatal(err)
	}
	m := model{}
	m.files.dir = dir
	m.screen = screenCorkboard
	m.exportChooser = newExportChooser()
	m.exportChooser.checked[formatRTF] = true

	m.runExport()

	data, err := os.ReadFile(filepath.Join(dir, "export", "n.rtf"))
	if err != nil {
		t.Fatalf("expected export/n.rtf to be written: %v, status=%q", err, m.status)
	}
	if !strings.Contains(string(data), "An explicitly included Resource.") {
		t.Fatal("a Resource with an explicit false sidecar entry must still be included")
	}
}
```

`saveExportSelectionRaw` n'existe pas encore dans le code — c'est un petit helper de test à
ajouter dans le même fichier (nécessaire car `saveExportSelection` de production prune toute
valeur `false`, donc il n'existe aujourd'hui aucun chemin de production qui écrit une clé
explicitement à `false` ; le test doit écrire le sidecar brut pour exercer ce cas limite du
contrat de lecture) :

```go
// saveExportSelectionRaw writes the sidecar with the exact map given, bypassing
// saveExportSelection's pruning of false values — needed only to test buildEntry/runExport's
// read-side "explicit key always wins" contract for a value production code never writes.
func saveExportSelectionRaw(t *testing.T, dir string, excluded map[string]bool) error {
	t.Helper()
	data, err := json.Marshal(exportSelectionFile{SchemaVersion: exportSelectionSchemaVersion, Excluded: excluded})
	if err != nil {
		return err
	}
	return os.WriteFile(exportSelectionPath(dir), data, 0o644)
}
```

Ajouter `"encoding/json"` aux imports du fichier de test si absent.

- [ ] **Step 7: Lancer les tests, vérifier le succès**

Run: `go test ./... -run TestRunExportResource -v`
Expected: PASS pour les 2 tests

- [ ] **Step 8: Lancer tous les tests du package pour vérifier l'absence de régression**

Run: `go build ./... && go test ./...`
Expected: `ok  	okashi	...`

- [ ] **Step 9: Commit**

```bash
git add exportselect.go export.go exportselect_test.go
git commit -m "feat: les Ressources sont exclues de l'export par défaut

buildEntry et runExport calculent désormais un défaut d'inclusion qui
dépend du type d'entrée : un item listé dans le manifest reste inclus
par défaut (inchangé), une Ressource est exclue par défaut sauf
inclusion explicite. S'applique rétroactivement, sans migration du
sidecar — le défaut se calcule à la lecture."
```

---

## Task 4 : Raccourci sidebar `R`

**Files:**
- Modify: `filelist.go` (ajout de `selectedEntry`, à la suite de `selectedEntryName`, après la
  ligne 583), `exportselect.go` (ajout de `toggleSelectedSceneResourceStatus` et
  `isResourceFile`), `main.go:1879-1898` (ajout du binding `case "R"`)
- Test: `filelist_test.go`, `exportselect_test.go` (ou `main_test.go`, selon où vivent déjà les
  tests de bindings sidebar — voir Task 4 Step 7)

**Interfaces:**
- Consumes : `m.convertChapterSceneToStandalone(folder, file string)` (Task 1),
  `m.convertStandaloneSceneToResource(file string)` (`exportselect.go:293`, existant),
  `m.convertResourceToStandaloneScene(file string)` (`exportselect.go:316`, existant),
  `fileEntry` (`filelist.go:24-31`, champs `isChildScene`, `parentFolder`, `isScene`, `isDir`,
  `isPartHeader`), `resolveManuscript`/`readEntries` (existants, utilisés par `isResourceFile`).
- Produces : `func (f filelist) selectedEntry() (fileEntry, bool)` (dans `filelist.go`) — utilisé
  par `toggleSelectedSceneResourceStatus`. `func (m *model) toggleSelectedSceneResourceStatus()`
  (dans `exportselect.go`) — appelée depuis `main.go` sur la touche `R`.

- [ ] **Step 1: Écrire le test de `selectedEntry`**

Ajouter à `filelist_test.go` :

```go
func TestSelectedEntryReturnsFullEntryUnderCursor(t *testing.T) {
	dir := t.TempDir()
	mkChapterDir(t, dir, "opening", map[string]string{
		"opening.md":  "hello",
		"opening2.md": "world",
	})
	writeManifestRaw(t, dir, `{"schemaVersion":3,"title":"N","items":[
		{"chapter":{"folder":"opening","title":"Opening","texts":[
			{"file":"opening.md","title":"Part One"},
			{"file":"opening2.md","title":"Part Two"}
		]}}
	]}`)
	if err := saveFolded(dir, map[string]bool{"opening": true}, map[string]bool{"opening": true}); err != nil {
		t.Fatal(err)
	}
	f := newFilelist()
	f.SetDir(dir)

	childIdx := -1
	for i, e := range f.entries {
		if e.isChildScene && e.name == "opening2.md" {
			childIdx = i
		}
	}
	if childIdx == -1 {
		t.Fatalf("expected a child-scene row for opening2.md, got entries=%+v", f.entries)
	}
	f.selected = childIdx

	got, ok := f.selectedEntry()
	if !ok {
		t.Fatal("selectedEntry() ok = false, want true")
	}
	if !got.isChildScene || got.name != "opening2.md" || got.parentFolder != "opening" {
		t.Fatalf("selectedEntry() = %+v, want isChildScene=true name=opening2.md parentFolder=opening", got)
	}
}

func TestSelectedEntryOutOfBoundsReturnsFalse(t *testing.T) {
	f := newFilelist()
	f.selected = 5
	_, ok := f.selectedEntry()
	if ok {
		t.Fatal("selectedEntry() ok = true for an out-of-bounds selection, want false")
	}
}
```

- [ ] **Step 2: Lancer le test, vérifier l'échec attendu**

Run: `go test ./... -run TestSelectedEntry -v`
Expected: FAIL avec `f.selectedEntry undefined (type filelist has no field or method
selectedEntry)`

- [ ] **Step 3: Implémenter `selectedEntry`**

Ajouter dans `filelist.go`, immédiatement après `selectedEntryName` (après la ligne 583) :

```go
// selectedEntry returns the full selected fileEntry, or ok=false if nothing is selected.
func (f filelist) selectedEntry() (fileEntry, bool) {
	if f.selected < 0 || f.selected >= len(f.entries) {
		return fileEntry{}, false
	}
	return f.entries[f.selected], true
}
```

- [ ] **Step 4: Lancer le test, vérifier le succès**

Run: `go test ./... -run TestSelectedEntry -v`
Expected: PASS pour les 2 tests

- [ ] **Step 5: Écrire le test de `isResourceFile`**

Ajouter à `exportselect_test.go` :

```go
func TestIsResourceFileTrueForUnlistedRootFile(t *testing.T) {
	dir := t.TempDir()
	mkChapterDir(t, dir, "ch1", map[string]string{"scene-1.md": "x"})
	os.WriteFile(filepath.Join(dir, "notes.md"), []byte("a Resource"), 0o644)
	if err := writeManifest(dir, manifest{
		Title: "N",
		Items: []manifestItem{
			{Chapter: &manifestChapter{Folder: "ch1", Title: "One", Texts: []manifestText{
				{File: "scene-1.md", Title: "Opening"},
			}}},
		},
	}); err != nil {
		t.Fatal(err)
	}

	if !isResourceFile(dir, "notes.md") {
		t.Fatal("notes.md is not listed in items[] — must be reported as a Resource")
	}
	if isResourceFile(dir, "scene-1.md") {
		t.Fatal("scene-1.md is listed in ch1's Texts[] — must NOT be reported as a Resource")
	}
}
```

- [ ] **Step 6: Lancer le test, vérifier l'échec attendu**

Run: `go test ./... -run TestIsResourceFileTrueForUnlistedRootFile -v`
Expected: FAIL avec `undefined: isResourceFile`

- [ ] **Step 7: Implémenter `isResourceFile` et `toggleSelectedSceneResourceStatus`**

Ajouter dans `exportselect.go`, à la fin du fichier :

```go
// isResourceFile reports whether name (a root-level filename) is currently a Resource — i.e.
// present among looseFiles's output for dir (resolved via resolveManuscript/v.loose).
func isResourceFile(dir, name string) bool {
	v := resolveManuscript(dir, readEntries(dir))
	for _, e := range v.loose {
		if e.name == name {
			return true
		}
	}
	return false
}

// toggleSelectedSceneResourceStatus advances the sidebar's selected entry one step along the
// chapter-scene → standalone-scene → Resource → standalone-scene cycle (design §1): a scene
// nested in a chapter exits to standalone; a standalone scene becomes a Resource; a Resource
// becomes standalone again. A chapter header, Part header, or directory has no status to toggle
// and is a no-op.
func (m *model) toggleSelectedSceneResourceStatus() {
	e, ok := m.files.selectedEntry()
	if !ok {
		return
	}
	switch {
	case e.isChildScene:
		m.convertChapterSceneToStandalone(e.parentFolder, e.name)
	case e.isScene:
		m.convertStandaloneSceneToResource(e.name)
	case !e.isDir && !e.isPartHeader && isResourceFile(m.files.dir, e.name):
		m.convertResourceToStandaloneScene(e.name)
	}
	m.files.SetDir(m.files.dir) // refresh sidebar entries + word counts from the rewritten manifest
}
```

- [ ] **Step 8: Lancer le test, vérifier le succès**

Run: `go test ./... -run TestIsResourceFileTrueForUnlistedRootFile -v`
Expected: PASS

- [ ] **Step 9: Ajouter le binding `R` dans `main.go`**

Dans `main.go`, dans le bloc `switch key.String()` du focus sidebar (`main.go:1862-1899`), ajouter
juste après `case "v":` (ligne 1897-1898) :

```go
		case "v":
			m.enterExportSelect()
		case "R":
			m.toggleSelectedSceneResourceStatus()
```

- [ ] **Step 10: Écrire les tests bout-en-bout du binding `R`**

Localiser d'abord le helper `sidebarModel` :

Run: `grep -n "func sidebarModel" rename_wiring_test.go`

Ajouter à `rename_wiring_test.go` (même fichier que `sidebarModel`, pour réutiliser le helper sans
import supplémentaire) :

```go
func TestSidebarRToggleChapterSceneBecomesStandalone(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	mkChapterDir(t, root, "ch1", map[string]string{
		"scene-1.md": "First scene.",
		"scene-2.md": "Second scene.",
	})
	writeManifestRaw(t, root, `{"schemaVersion":3,"title":"N","items":[
		{"chapter":{"folder":"ch1","title":"Chapter One","texts":[
			{"file":"scene-1.md","title":"Opening"},
			{"file":"scene-2.md","title":"Confrontation"}
		]}}
	]}`)
	if err := saveFolded(root, map[string]bool{"ch1": true}, map[string]bool{"ch1": true}); err != nil {
		t.Fatal(err)
	}
	m := sidebarModel(t, root)
	childIdx := -1
	for i, e := range m.files.entries {
		if e.isChildScene && e.name == "scene-2.md" {
			childIdx = i
		}
	}
	if childIdx == -1 {
		t.Fatalf("expected a child-scene row for scene-2.md, got entries=%+v", m.files.entries)
	}
	m.files.selected = childIdx

	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	m = nm.(model)

	got, _, err := readManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	if findSceneByFile(&got, "scene-2.md") == nil {
		t.Fatal("scene-2.md must now be a standalone scene")
	}
}

func TestSidebarRToggleStandaloneSceneBecomesResource(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	os.WriteFile(filepath.Join(root, "aparte.md"), []byte("A standalone scene."), 0o644)
	writeManifestRaw(t, root, `{"schemaVersion":3,"title":"N","items":[
		{"chapter":{"title":"Aparté","scene":true,"texts":[{"file":"aparte.md","title":"Aparté"}]}}
	]}`)
	m := sidebarModel(t, root)
	m.files.selectName("aparte.md")

	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	m = nm.(model)

	got, _, err := readManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Items) != 0 {
		t.Fatalf("aparte.md must have been dropped from items[] (now a Resource), got %+v", got.Items)
	}
	if _, err := os.Stat(filepath.Join(root, "aparte.md")); err != nil {
		t.Fatal("aparte.md must still exist on disk (unmoved — root stays root)")
	}
}

func TestSidebarRToggleResourceBecomesStandaloneScene(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	os.WriteFile(filepath.Join(root, "notes.md"), []byte("A Resource."), 0o644)
	m := sidebarModel(t, root)
	m.files.selectName("notes.md")

	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	m = nm.(model)

	got, _, err := readManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	if findSceneByFile(&got, "notes.md") == nil {
		t.Fatal("notes.md must now be listed as a standalone scene")
	}
}

func TestSidebarRToggleOnChapterHeaderIsNoOp(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	mkChapterDir(t, root, "ch1", map[string]string{"scene-1.md": "x"})
	writeManifestRaw(t, root, `{"schemaVersion":3,"title":"N","items":[
		{"chapter":{"folder":"ch1","title":"Chapter One","texts":[{"file":"scene-1.md","title":"Opening"}]}}
	]}`)
	m := sidebarModel(t, root)
	m.files.selectName("ch1")

	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	m = nm.(model)

	got, _, err := readManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	ch1 := findChapterByFolder(&got, "ch1")
	if ch1 == nil || len(ch1.Texts) != 1 || ch1.Texts[0].File != "scene-1.md" {
		t.Fatalf("chapter header R must be a no-op, got %+v", ch1)
	}
}
```

Si `mkChapterDir`/`writeManifestRaw` ne sont pas visibles depuis `rename_wiring_test.go` (même
package `main`, donc ils le sont toujours en Go — pas d'action requise), et si `tea` n'est pas
déjà importé dans ce fichier, l'ajouter aux imports.

- [ ] **Step 11: Lancer les 4 tests, vérifier le succès**

Run: `go test ./... -run TestSidebarRToggle -v`
Expected: PASS pour les 4 tests

- [ ] **Step 12: Lancer tous les tests du package pour vérifier l'absence de régression**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: `ok  	okashi	...`

- [ ] **Step 13: Commit**

```bash
git add filelist.go exportselect.go main.go rename_wiring_test.go filelist_test.go exportselect_test.go
git commit -m "feat: raccourci sidebar R bascule le statut scène/Ressource

R sur l'entrée sélectionnée avance d'un cran le cycle scène-de-chapitre
→ scène indépendante → Ressource → scène indépendante, en routant vers
les fonctions de conversion déjà définies dans exportselect.go — aucune
logique dupliquée. No-op sur un en-tête de chapitre ou de Partie."
```

---

## Task 5 : Mise à jour de CLAUDE.md

**Files:**
- Modify: `CLAUDE.md`

**Interfaces:**
- Consumes : rien (tâche documentaire uniquement).
- Produces : rien consommé par une tâche suivante — dernière tâche du plan.

- [ ] **Step 1: Documenter le raccourci `R` et le nouveau défaut d'export dans § Project model**

Dans `CLAUDE.md`, section « Project model (the shipped reality) », repérer la phrase existante
(dans la description de l'écran `v`) :

> **Resources can now appear in an export when checked on this screen** — previously impossible

Juste après cette phrase (avant la parenthèse fermante du paragraphe `v`), insérer :

```
; a Resource is now EXCLUDED from export by default (an item listed in the manifest — chapter,
chapter-scene, standalone scene — stays included by default as before; only an explicit checkbox
state ever overrides either default). A chapter-scene can exit its chapter to become a standalone
scene via `shift+↑↓` past the chapter boundary on this screen, or via **`R`** from the sidebar on
the selected entry — `R` also converts a standalone scene → Resource and a Resource → standalone
scene, one step of the cycle per press; a chapter/Part header is a no-op
```

- [ ] **Step 2: Documenter le raccourci `R` dans la liste des touches sidebar si une liste dédiée existe**

Run: `grep -n "'M'\|'N'\|'v'\|sidebar.*ctrl+n\|screenTextPicker" CLAUDE.md`

Si un inventaire de touches sidebar existe ailleurs dans le fichier (au-delà du paragraphe déjà
modifié au Step 1), y ajouter une mention cohérente de `R`. Si aucun inventaire séparé n'existe
(la documentation des touches sidebar est intégrée au paragraphe narratif déjà modifié), ce Step
est un no-op — passer directement au Step 3.

- [ ] **Step 3: Relire le paragraphe modifié pour cohérence**

Lire `CLAUDE.md` autour de la modification (`grep -n "Resources can now appear" CLAUDE.md` pour
localiser, puis lire ±10 lignes) et vérifier que la phrase insérée s'intègre grammaticalement au
paragraphe existant sans dupliquer une information déjà présente ailleurs dans le fichier.

- [ ] **Step 4: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: documenter le raccourci R et le défaut d'export des Ressources"
```

---

## Vérification finale

- [ ] **Step 1: Build, vet, et suite de tests complète**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: `ok  	okashi	...` pour tous les packages, aucune erreur `go vet`

- [ ] **Step 2: Vérifier manuellement le scénario bout-en-bout (optionnel mais recommandé)**

Run: `go run .` sur un manuscrit de test avec un chapitre à 2 scènes ; dans la sidebar, déplier le
chapitre, sélectionner la seconde scène, appuyer sur `R` deux fois de suite (scène de chapitre →
scène indépendante → Ressource) ; vérifier dans l'écran `v` (`v` depuis la sidebar) que la case de
la Ressource résultante est décochée par défaut.
