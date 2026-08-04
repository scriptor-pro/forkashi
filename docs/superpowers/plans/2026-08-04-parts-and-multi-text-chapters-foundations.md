# Parties et chapitres multi-textes — Fondations Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remplacer le manifest v1 (chapitre = fichier plat) par le manifest v2 (Partie → Chapitre-dossier
→ Texte), avec migration automatique confirmée des manuscrits v1 existants, et adapter les 7 sites
consommateurs de `manuscriptView` pour parcourir mécaniquement la nouvelle hiérarchie (sans encore
ajouter l'habillage visuel Partie ni la concaténation multi-textes à l'export — ce sera le plan suivant).

**Architecture:** `manifest.go` gagne les types v2 imbriqués (`manifestText`/`manifestChapter`/
`manifestItem` à 3 niveaux) et une fonction `migrateManifestV1ToV2`. `manuscript.go` reconstruit
`manuscriptView` autour de `parts []partRef` (un manuscrit sans Partie expose un unique `partRef{title:
""}` synthétique portant tous ses chapitres, pour que tous les appelants n'aient qu'une seule forme à
parcourir) ; `resolveManuscript` lit désormais, pour chaque chapitre listé, le contenu de son dossier via
un `os.ReadDir` dédié. Le déclenchement de la migration (avec écran de confirmation) s'accroche à
`enterWriting()` dans `main.go`, le point commun à tous les chemins d'ouverture d'un manuscrit depuis le
hub.

**Tech Stack:** Go 1.25, `encoding/json`, `bubbletea`/`lipgloss` pour l'écran de confirmation. Package
`main`, tests `go test`.

## Global Constraints

- ⚠️ HARD GATE contrat partagé (CLAUDE.md §1) : ce plan change la *forme* du schéma `manifest.json`.
  Ne pas considérer ce plan comme déployable sans coordination avec le repo de l'app companion — hors
  périmètre de ce plan (implémentation okashi seule), mais à rappeler dans le message de fin de plan.
- Aucun préfixe numérique sur les noms de dossier-chapitre ni de fichier-texte : slug seul, dérivé du
  titre au moment de la création, birth-stable ensuite (jamais renommé par un retitre ou une
  réorganisation).
- Une Partie n'a jamais de dossier physique — uniquement une entrée dans `manifest.json`.
- Écritures manifest toujours atomiques via `atomicWrite` (`atomicwrite.go`), jamais en place.
- Migration v1→v2 : une seule passe, confirmée par un écran avant toute écriture disque ; si un
  déplacement de fichier échoue en cours de route, aucune écriture manifest n'a lieu (jamais d'état où
  le manifest v2 référence des fichiers pas encore déplacés).
- Build : `go build ./...`. Vet : `go vet ./...`. Tests : `go test ./...`.
- Ce plan NE couvre PAS : l'affichage des en-têtes de Partie (juste le parcours mécanique de la
  hiérarchie), la concaténation multi-textes à l'export (les exporteurs restent sur le premier texte de
  chaque chapitre pour l'instant), `ctrl+n` à 4 choix, le structure mode étendu à 3 niveaux, le sélecteur
  de texte à l'ouverture. Ces points sont le plan suivant.

---

### Task 1: Types du manifest v2 et lecture/écriture

**Files:**
- Modify: `manifest.go` (remplace les types v1 plats par les types v2 imbriqués, adapte
  `readManifest`/`writeManifest`)
- Test: `manifest_test.go`

**Interfaces:**
- Consumes: `atomicWrite(path string, data []byte, perm os.FileMode) error` (`atomicwrite.go`,
  inchangé).
- Produces:
  ```go
  type manifestText struct {
      File  string `json:"file"`
      Title string `json:"title"`
  }
  type manifestChapter struct {
      Folder string         `json:"folder"`
      Title  string         `json:"title"`
      Texts  []manifestText `json:"texts"`
  }
  type manifestItem struct {
      Part     string            `json:"part,omitempty"`
      Chapters []manifestChapter `json:"chapters,omitempty"`
      Chapter  *manifestChapter  `json:"chapter,omitempty"`
  }
  type manifest struct {
      SchemaVersion int            `json:"schemaVersion"`
      Title         string         `json:"title"`
      Items         []manifestItem `json:"items"`
  }
  ```
  consommé par Task 2 (`manuscript.go`) et Task 3 (migration).

- [ ] **Step 1: Write the failing tests**

Remplace le contenu de `manifest_test.go` par :

```go
package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadManifestAbsent(t *testing.T) {
	dir := t.TempDir()
	_, present, err := readManifest(dir)
	if present || err != nil {
		t.Fatalf("absent manifest: present=%v err=%v, want false,nil", present, err)
	}
	if hasManifest(dir) {
		t.Fatal("hasManifest should be false with no manifest.json")
	}
}

func TestReadManifestValidV2(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, manifestName), []byte(`{
		"schemaVersion": 2, "title": "Windermere",
		"items": [
			{"chapter": {"folder":"opening","title":"Chapter One","texts":[{"file":"opening.md","title":"Opening"}]}},
			{"part": "Part One", "chapters": [
				{"folder":"the-letter","title":"The Letter","texts":[{"file":"the-letter.md","title":"The Letter"}]}
			]}
		]}`), 0o644)
	m, present, err := readManifest(dir)
	if !present || err != nil {
		t.Fatalf("valid v2 manifest: present=%v err=%v, want true,nil", present, err)
	}
	if m.Title != "Windermere" || len(m.Items) != 2 {
		t.Fatalf("decoded = %+v, want title Windermere with 2 items", m)
	}
	if m.Items[0].Chapter == nil || m.Items[0].Chapter.Folder != "opening" {
		t.Fatalf("item 0 chapter = %+v", m.Items[0].Chapter)
	}
	if m.Items[1].Part != "Part One" || len(m.Items[1].Chapters) != 1 {
		t.Fatalf("item 1 part = %+v", m.Items[1])
	}
	if m.Items[1].Chapters[0].Texts[0].File != "the-letter.md" {
		t.Fatalf("item 1 chapter 0 text 0 = %+v", m.Items[1].Chapters[0].Texts[0])
	}
	if !hasManifest(dir) {
		t.Fatal("hasManifest should be true")
	}
}

func TestReadManifestRejectsV1(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, manifestName),
		[]byte(`{"schemaVersion":1,"title":"X","items":[{"file":"a.md","title":"A"}]}`), 0o644)
	_, present, err := readManifest(dir)
	if !present {
		t.Fatal("a present-but-unsupported manifest must report present=true")
	}
	if err == nil {
		t.Fatal("schemaVersion 1 must be refused by readManifest (migration is a separate explicit step, not silent read)")
	}
}

func TestReadManifestRejectsBadVersion(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, manifestName),
		[]byte(`{"schemaVersion":3,"title":"X","items":[]}`), 0o644)
	_, present, err := readManifest(dir)
	if !present {
		t.Fatal("a present-but-unsupported manifest must report present=true")
	}
	if err == nil {
		t.Fatal("schemaVersion 3 must be refused with an error, not silently read")
	}
}

func TestReadManifestRejectsMalformed(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, manifestName), []byte(`{ not json`), 0o644)
	_, present, err := readManifest(dir)
	if !present || err == nil {
		t.Fatalf("malformed manifest: present=%v err=%v, want true,non-nil", present, err)
	}
}

func TestWriteManifestV2RoundTrip(t *testing.T) {
	dir := t.TempDir()
	m := manifest{
		Title: "Windermere",
		Items: []manifestItem{
			{Chapter: &manifestChapter{Folder: "opening", Title: "Chapter One",
				Texts: []manifestText{{File: "opening.md", Title: "Opening"}}}},
			{Part: "Part One", Chapters: []manifestChapter{
				{Folder: "the-letter", Title: "The Letter",
					Texts: []manifestText{{File: "the-letter.md", Title: "The Letter"}}},
			}},
		},
	}
	if err := writeManifest(dir, m); err != nil {
		t.Fatal(err)
	}
	got, present, err := readManifest(dir)
	if !present || err != nil {
		t.Fatalf("round-trip read: present=%v err=%v", present, err)
	}
	if got.SchemaVersion != manifestSchemaVersion {
		t.Fatalf("schemaVersion = %d, want %d", got.SchemaVersion, manifestSchemaVersion)
	}
	if len(got.Items) != 2 || got.Items[0].Chapter.Folder != "opening" ||
		got.Items[1].Part != "Part One" || got.Items[1].Chapters[0].Folder != "the-letter" {
		t.Fatalf("round-trip items = %+v", got.Items)
	}
}

func TestWriteManifestEmptyItemsSerializesAsEmptyArray(t *testing.T) {
	dir := t.TempDir()
	if err := writeManifest(dir, manifest{Title: "Empty"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, manifestName))
	if err != nil {
		t.Fatal(err)
	}
	if !contains(string(data), `"items": []`) {
		t.Fatalf("empty items must serialize as [], got: %s", data)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || (len(s) > 0 && indexOf(s, sub) >= 0))
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./... -run TestReadManifest -v 2>&1 | tail -60`
Expected: FAIL — `m.Items[0].Chapter` undefined (the v1 flat `File`/`Title` fields don't have a
`Chapter`/`Part`/`Chapters` shape), compile errors against the current `manifestItem`.

- [ ] **Step 3: Rewrite the manifest v2 types and schema version**

Dans `manifest.go`, remplace le type `manifestItem` et la constante de version :

```go
const manifestSchemaVersion = 2

// manifestText is one ordered text (scene) inside a chapter: a bare filename (slug,
// no numeric prefix — order lives in the manifest, not the filename) plus a display title.
type manifestText struct {
	File  string `json:"file"`
	Title string `json:"title"`
}

// manifestChapter is one chapter: a folder (slug, birth-stable — never renamed by a
// retitle or reorder) holding one or more ordered texts.
type manifestChapter struct {
	Folder string         `json:"folder"`
	Title  string         `json:"title"`
	Texts  []manifestText `json:"texts"`
}

// manifestItem is one top-level manuscript entry: EITHER a bare chapter (Chapter set,
// Part empty, Chapters nil) OR a Part grouping its own ordered chapters (Part set,
// Chapters set, Chapter nil). Mutually exclusive — never both.
type manifestItem struct {
	Part     string            `json:"part,omitempty"`
	Chapters []manifestChapter `json:"chapters,omitempty"`
	Chapter  *manifestChapter  `json:"chapter,omitempty"`
}
```

`type manifest struct` ne change pas (déjà `SchemaVersion`/`Title`/`Items`).

- [ ] **Step 4: Update `readManifest`**

`readManifest` ne change pas de signature ni de logique — `m.SchemaVersion != manifestSchemaVersion`
compare déjà contre la constante, qui vaut maintenant `2`. Un manifest v1 (`schemaVersion: 1`) est donc
automatiquement refusé par cette fonction avec la même erreur `"unsupported manifest schemaVersion..."`
qu'un schéma totalement invalide — c'est voulu : la migration v1→v2 est un chemin séparé et explicite
(Task 3), jamais une lecture silencieuse.

- [ ] **Step 5: Update `writeManifest`**

Dans `writeManifest`, remplace la garde `nil→[]manifestItem{}` (elle reste valide telle quelle, le type
`items` change de forme mais pas la nécessité du garde) — aucune autre modification structurelle needed
ici, `writeManifest` sérialise déjà `m.Items` tel quel via le `map[string]any` générique. Vérifie
seulement que la construction du buffer compile toujours avec le nouveau type `manifestItem`.

- [ ] **Step 6: Delete `createManuscript`, `renameChapterTitle`, `manifestInsert`, `manifestRemove`,
  `manifestReorder`**

Ces cinq fonctions opèrent sur la forme plate v1 (`m.Items[i].File`, insertion/suppression/réorganisation
d'un item plat) — elles n'ont plus de sens telles quelles contre la forme v2 imbriquée. Supprime-les de
`manifest.go` entièrement (pas de shim de compatibilité). Elles seront réécrites dans le plan suivant
(création/réorganisation, Task 4 de la spec) une fois la forme v2 stabilisée par ce plan-ci ; ce plan ne
les réintroduit pas car aucun appelant actif n'en a besoin après Task 5 de CE plan (la Task 5 ci-dessous
réécrit leurs seuls appelants : `home.go`, `main.go`, `corkboard.go`).

- [ ] **Step 7: Run tests to verify they pass**

Run: `go test ./... -run TestReadManifest -v 2>&1 | tail -60`
Expected: `TestReadManifestAbsent`, `TestReadManifestValidV2`, `TestReadManifestRejectsV1`,
`TestReadManifestRejectsBadVersion`, `TestReadManifestRejectsMalformed` all PASS.

Run: `go test ./... -run TestWriteManifest -v 2>&1 | tail -60`
Expected: `TestWriteManifestV2RoundTrip`, `TestWriteManifestEmptyItemsSerializesAsEmptyArray` PASS.

Expected build errors remain at this point in every file that called the five deleted functions
(`home.go`, `main.go`, `corkboard.go`, `structure.go`) — these are fixed in Tasks 2 and 5. Confirm with:

Run: `go build ./... 2>&1 | head -40`
Expected: a list of "undefined: createManuscript" / "undefined: renameChapterTitle" /
"undefined: manifestInsert" / "undefined: manifestRemove" / "undefined: manifestReorder" errors —
exactly the fan-out this task intentionally defers to Tasks 2 and 5. Do not fix them here.

- [ ] **Step 8: Commit**

```bash
git add manifest.go manifest_test.go
git commit -m "$(cat <<'EOF'
Remplace le manifest v1 (chapitre=fichier) par le manifest v2 (Partie/Chapitre-dossier/Texte)

schemaVersion passe à 2 ; manifestItem devient soit un chapitre nu soit
une Partie groupant ses chapitres, chaque chapitre listant ses textes
ordonnés. readManifest refuse désormais v1 comme tout schéma non
supporté — la migration v1→v2 est un chemin explicite séparé (à venir).
Les writers v1 (createManuscript, renameChapterTitle, manifestInsert/
Remove/Reorder) sont supprimés ; le build reste rouge dans leurs
appelants jusqu'aux tâches suivantes de ce plan.
EOF
)"
```

---

### Task 2: `manuscriptView` à 3 niveaux dans `manuscript.go`

**Files:**
- Modify: `manuscript.go`
- Test: `manuscript_test.go`

**Interfaces:**
- Consumes: `manifestText`/`manifestChapter`/`manifestItem`/`manifest` (Task 1) ; `readManifest(dir
  string) (manifest, bool, error)` (Task 1, inchangée en signature) ; `fileEntry{name string, isDir
  bool}` (`filelist.go`, inchangé) ; `hasNumberedSections`, `orderedSections`, `sectionTitle`
  (`project.go`, inchangés — legacy fallback).
- Produces:
  ```go
  type textRef struct {
      file  string // relative to the chapter's folder, e.g. "scene-ouverture.md"
      title string
      words int
  }
  type chapterRef struct {
      folder string // "" only for a legacy chapter (pre-v2 display fallback, see below)
      title  string
      texts  []textRef
  }
  type partRef struct {
      title    string // "" for the synthetic part holding chapters outside any real Part
      chapters []chapterRef
  }
  type manuscriptView struct {
      source  manuscriptSource
      title   string
      parts   []partRef // ALWAYS at least one partRef
      loose   []fileEntry
      warning string
  }
  func resolveManuscript(dir string, entries []fileEntry) manuscriptView
  func isChapterOf(v manuscriptView, folder string) bool // signature changes: name → folder
  ```
  consommé par Task 5 (les 7 sites) et par la Task 3 (migration, qui n'utilise pas directement ce type
  mais doit produire un manifest v2 que `resolveManuscript` sait lire).

- [ ] **Step 1: Write the failing tests**

Remplace le contenu de `manuscript_test.go` par :

```go
package main

import (
	"os"
	"path/filepath"
	"testing"
)

func writeManifestRaw(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, manifestName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mkChapterDir(t *testing.T, manuscriptDir, folder string, texts map[string]string) {
	t.Helper()
	chDir := filepath.Join(manuscriptDir, folder)
	if err := os.MkdirAll(chDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range texts {
		if err := os.WriteFile(filepath.Join(chDir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestResolveManifestBareChaptersNoPart(t *testing.T) {
	dir := t.TempDir()
	mkChapterDir(t, dir, "opening", map[string]string{"opening.md": "hello world"})
	writeManifestRaw(t, dir, `{"schemaVersion":2,"title":"Windermere","items":[
		{"chapter":{"folder":"opening","title":"Chapter One","texts":[{"file":"opening.md","title":"Opening"}]}}
	]}`)
	v := resolveManuscript(dir, readEntries(dir))
	if v.source != sourceManifest || !v.ordered() {
		t.Fatalf("want manifest source, got %v", v.source)
	}
	if len(v.parts) != 1 || v.parts[0].title != "" {
		t.Fatalf("bare chapters must live under a single synthetic untitled part, got parts=%+v", v.parts)
	}
	if len(v.parts[0].chapters) != 1 || v.parts[0].chapters[0].folder != "opening" ||
		v.parts[0].chapters[0].title != "Chapter One" {
		t.Fatalf("chapter = %+v", v.parts[0].chapters)
	}
	if len(v.parts[0].chapters[0].texts) != 1 || v.parts[0].chapters[0].texts[0].file != "opening.md" {
		t.Fatalf("texts = %+v", v.parts[0].chapters[0].texts)
	}
}

func TestResolveManifestMixedPartsAndBareChapters(t *testing.T) {
	dir := t.TempDir()
	mkChapterDir(t, dir, "prologue", map[string]string{"prologue.md": "x"})
	mkChapterDir(t, dir, "the-letter", map[string]string{"the-letter.md": "y"})
	writeManifestRaw(t, dir, `{"schemaVersion":2,"title":"Windermere","items":[
		{"chapter":{"folder":"prologue","title":"Prologue","texts":[{"file":"prologue.md","title":"Prologue"}]}},
		{"part":"Part One","chapters":[
			{"folder":"the-letter","title":"The Letter","texts":[{"file":"the-letter.md","title":"The Letter"}]}
		]}
	]}`)
	v := resolveManuscript(dir, readEntries(dir))
	if len(v.parts) != 2 {
		t.Fatalf("want 2 parts (synthetic + Part One), got %+v", v.parts)
	}
	if v.parts[0].title != "" || len(v.parts[0].chapters) != 1 || v.parts[0].chapters[0].folder != "prologue" {
		t.Fatalf("part 0 (bare chapters) = %+v", v.parts[0])
	}
	if v.parts[1].title != "Part One" || len(v.parts[1].chapters) != 1 || v.parts[1].chapters[0].folder != "the-letter" {
		t.Fatalf("part 1 = %+v", v.parts[1])
	}
}

func TestResolveManifestMultiTextChapterOrder(t *testing.T) {
	dir := t.TempDir()
	mkChapterDir(t, dir, "chapitre-un", map[string]string{
		"scene-ouverture.md":     "a",
		"scene-confrontation.md": "b",
	})
	writeManifestRaw(t, dir, `{"schemaVersion":2,"title":"N","items":[
		{"chapter":{"folder":"chapitre-un","title":"Chapitre un","texts":[
			{"file":"scene-confrontation.md","title":"Confrontation"},
			{"file":"scene-ouverture.md","title":"Ouverture"}
		]}}
	]}`)
	v := resolveManuscript(dir, readEntries(dir))
	texts := v.parts[0].chapters[0].texts
	if len(texts) != 2 || texts[0].file != "scene-confrontation.md" || texts[1].file != "scene-ouverture.md" {
		t.Fatalf("manifest order must win, got %+v", texts)
	}
}

func TestResolveManifestChapterWithZeroTexts(t *testing.T) {
	dir := t.TempDir()
	mkChapterDir(t, dir, "empty-chapter", nil)
	writeManifestRaw(t, dir, `{"schemaVersion":2,"title":"N","items":[
		{"chapter":{"folder":"empty-chapter","title":"Vide","texts":[]}}
	]}`)
	v := resolveManuscript(dir, readEntries(dir))
	if len(v.parts[0].chapters) != 1 || v.parts[0].chapters[0].title != "Vide" {
		t.Fatalf("an empty chapter must still be listed (never auto-removed), got %+v", v.parts)
	}
	if len(v.parts[0].chapters[0].texts) != 0 {
		t.Fatalf("texts = %+v, want empty", v.parts[0].chapters[0].texts)
	}
}

func TestResolveManifestAbsentTextOmitted(t *testing.T) {
	dir := t.TempDir()
	mkChapterDir(t, dir, "opening", map[string]string{"opening.md": "x"}) // "gone.md" never written
	writeManifestRaw(t, dir, `{"schemaVersion":2,"title":"N","items":[
		{"chapter":{"folder":"opening","title":"One","texts":[
			{"file":"opening.md","title":"Opening"},{"file":"gone.md","title":"Lost"}
		]}}
	]}`)
	v := resolveManuscript(dir, readEntries(dir))
	texts := v.parts[0].chapters[0].texts
	if len(texts) != 1 || texts[0].file != "opening.md" {
		t.Fatalf("a truly-absent text must be omitted, got %+v", texts)
	}
}

func TestResolveManifestAbsentChapterFolderOmitted(t *testing.T) {
	dir := t.TempDir()
	mkChapterDir(t, dir, "opening", map[string]string{"opening.md": "x"}) // "gone" folder never created
	writeManifestRaw(t, dir, `{"schemaVersion":2,"title":"N","items":[
		{"chapter":{"folder":"opening","title":"One","texts":[{"file":"opening.md","title":"Opening"}]}},
		{"chapter":{"folder":"gone","title":"Lost","texts":[{"file":"gone.md","title":"Lost"}]}}
	]}`)
	v := resolveManuscript(dir, readEntries(dir))
	if len(v.parts[0].chapters) != 1 || v.parts[0].chapters[0].folder != "opening" {
		t.Fatalf("a truly-absent chapter folder must be omitted, got %+v", v.parts[0].chapters)
	}
}

func TestResolveManifestUnlistedIsLoose(t *testing.T) {
	dir := t.TempDir()
	mkChapterDir(t, dir, "a", map[string]string{"a.md": "x"})
	os.WriteFile(filepath.Join(dir, "notes.md"), []byte("y"), 0o644) // loose, at manuscript root
	writeManifestRaw(t, dir, `{"schemaVersion":2,"title":"N","items":[
		{"chapter":{"folder":"a","title":"One","texts":[{"file":"a.md","title":"A"}]}}
	]}`)
	v := resolveManuscript(dir, readEntries(dir))
	if len(v.loose) != 1 || v.loose[0].name != "notes.md" {
		t.Fatalf("loose = %+v, want notes.md", v.loose)
	}
}

func TestResolveLegacyFallback(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "02-b.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, "01-a.md"), []byte("y"), 0o644)
	os.WriteFile(filepath.Join(dir, "notes.md"), []byte("z"), 0o644)
	v := resolveManuscript(dir, readEntries(dir))
	if v.source != sourceLegacy || !v.ordered() {
		t.Fatalf("numbered files + no manifest -> legacy, got %v", v.source)
	}
	if len(v.parts) != 1 || v.parts[0].title != "" {
		t.Fatalf("legacy chapters must also live under a single synthetic untitled part, got %+v", v.parts)
	}
	chs := v.parts[0].chapters
	if chs[0].folder != "" || len(chs[0].texts) != 1 || chs[0].texts[0].file != "01-a.md" {
		t.Fatalf("legacy chapter 0 = %+v, want folder \"\" (flat file), single text 01-a.md", chs[0])
	}
	if chs[1].texts[0].file != "02-b.md" {
		t.Fatalf("legacy order = %+v, want numeric", chs)
	}
	if chs[0].title != "a" { // de-slugged
		t.Fatalf("legacy title = %q, want de-slugged 'a'", chs[0].title)
	}
	if len(v.loose) != 1 || v.loose[0].name != "notes.md" {
		t.Fatalf("legacy loose = %+v", v.loose)
	}
}

func TestResolveCategory(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "on-silence.md"), []byte("x"), 0o644)
	v := resolveManuscript(dir, readEntries(dir))
	if v.source != sourceNone || v.ordered() {
		t.Fatalf("plain folder -> category, got %v", v.source)
	}
}

func TestResolveUnreadableManifestRefuses(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-a.md"), []byte("x"), 0o644) // numbered, but...
	writeManifestRaw(t, dir, `{"schemaVersion":3,"title":"N","items":[]}`)
	v := resolveManuscript(dir, readEntries(dir))
	// Refuse to guess: NOT legacy, files shown flat as loose, warning set.
	if v.source != sourceManifest {
		t.Fatalf("unreadable manifest still marks the folder a manuscript, got %v", v.source)
	}
	if len(v.parts) != 0 {
		t.Fatalf("must not invent parts/chapters from a bad manifest, got %+v", v.parts)
	}
	if v.warning == "" {
		t.Fatal("an unreadable manifest must surface a warning")
	}
	if len(v.loose) != 1 || v.loose[0].name != "01-a.md" {
		t.Fatalf("files shown flat as loose, got %+v", v.loose)
	}
}

func TestIsChapterOf(t *testing.T) {
	dir := t.TempDir()
	mkChapterDir(t, dir, "opening", map[string]string{"opening.md": "x"})
	writeManifestRaw(t, dir, `{"schemaVersion":2,"title":"N","items":[
		{"chapter":{"folder":"opening","title":"One","texts":[{"file":"opening.md","title":"Opening"}]}}
	]}`)
	v := resolveManuscript(dir, readEntries(dir))
	if !isChapterOf(v, "opening") {
		t.Fatal("opening must be recognized as a chapter")
	}
	if isChapterOf(v, "nope") {
		t.Fatal("nope must not be recognized as a chapter")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go build ./... 2>&1 | head -40`
Expected: compile errors — `v.chapters` undefined (type now has `.parts`), `manifestView` still
referencing the old flat shape, `chapterRef.file`/`isChapterOf(v, name)` signature mismatches.

- [ ] **Step 3: Rewrite `manuscript.go`**

Remplace le contenu de `manuscript.go` (garde `manuscriptSource`, `filterFiles`, `docEntries`,
`isManuscript` inchangés — seuls `chapterRef`, `manuscriptView`, `manifestView`, `resolveManuscript`,
`isChapterOf` changent) :

```go
package main

import (
	"os"
	"path/filepath"
)

type manuscriptSource int

const (
	sourceNone     manuscriptSource = iota
	sourceManifest
	sourceLegacy
)

// textRef is one ordered text (scene) inside a chapter.
type textRef struct {
	file  string
	title string
	words int
}

// chapterRef is one ordered chapter: a folder (empty only for a legacy flat-file
// chapter, pre-v2 display fallback) holding one or more ordered texts.
type chapterRef struct {
	folder string
	title  string
	texts  []textRef
}

// partRef is one ordered group of chapters. title == "" is the synthetic part
// that holds chapters outside any real Part — every manuscriptView.parts has at
// least this one entry, even when the author never created a Part.
type partRef struct {
	title    string
	chapters []chapterRef
}

// manuscriptView is a folder's resolved structure. One resolver feeds the sidebar,
// outline, pager, and export so they never disagree (design §4; parts §
// 2026-08-04).
type manuscriptView struct {
	source  manuscriptSource
	title   string
	parts   []partRef
	loose   []fileEntry
	warning string
}

func (v manuscriptView) ordered() bool {
	return v.source == sourceManifest || v.source == sourceLegacy
}

func filterFiles(entries []fileEntry) []fileEntry {
	var out []fileEntry
	for _, e := range entries {
		if !e.isDir {
			out = append(out, e)
		}
	}
	return out
}

func docEntries(entries []fileEntry) []fileEntry {
	_, loose := orderedSections(filterFiles(entries))
	return loose
}

func isManuscript(dir string) bool {
	return hasManifest(dir)
}

// resolveManuscript decides a folder's structure into one of three mutually
// exclusive states. A readable manifest is the SOLE source of order, titles, and
// membership. For each chapter it lists, its folder's contents are read to
// resolve which listed texts actually exist on disk (§4.2: a truly-absent text or
// chapter folder is omitted from display, never invented). Absent a manifest,
// numbered files fall back to legacy prefix ordering (read-only, single-text
// chapters, folder "").
func resolveManuscript(dir string, entries []fileEntry) manuscriptView {
	m, present, err := readManifest(dir)
	if present {
		if err != nil {
			return manuscriptView{
				source:  sourceManifest,
				title:   projectTitle(filepath.Base(dir)),
				loose:   filterFiles(entries),
				warning: err.Error(),
			}
		}
		return manifestView(dir, m, entries)
	}
	if hasNumberedSections(entries) {
		sections, loose := orderedSections(filterFiles(entries))
		chapters := make([]chapterRef, 0, len(sections))
		for _, s := range sections {
			chapters = append(chapters, chapterRef{
				folder: "",
				title:  sectionTitle(s.name),
				texts:  []textRef{{file: s.name, title: sectionTitle(s.name)}},
			})
		}
		return manuscriptView{
			source: sourceLegacy,
			title:  projectTitle(filepath.Base(dir)),
			parts:  []partRef{{title: "", chapters: chapters}},
			loose:  loose,
		}
	}
	return manuscriptView{
		source: sourceNone,
		title:  projectTitle(filepath.Base(dir)),
		loose:  docEntries(entries),
	}
}

// isChapterOf reports whether folder is a chapter of the given manuscript view,
// across all parts (including the synthetic untitled one).
func isChapterOf(v manuscriptView, folder string) bool {
	for _, p := range v.parts {
		for _, c := range p.chapters {
			if c.folder == folder {
				return true
			}
		}
	}
	return false
}

// resolveChapterTexts reads chDir's on-disk .md files and returns, in manifest
// order, the texts from listed that actually exist — a truly-absent text is
// omitted (§4.2 applied one level down).
func resolveChapterTexts(chDir string, listed []manifestText) []textRef {
	onDisk := map[string]bool{}
	if items, err := os.ReadDir(chDir); err == nil {
		for _, it := range items {
			if !it.IsDir() {
				onDisk[it.Name()] = true
			}
		}
	}
	texts := make([]textRef, 0, len(listed))
	for _, t := range listed {
		if onDisk[t.File] {
			texts = append(texts, textRef{file: t.File, title: t.Title})
		}
	}
	return texts
}

// manifestView projects a readable v2 manifest onto on-disk entries: for each
// listed chapter whose folder exists, its texts are resolved via
// resolveChapterTexts; a chapter whose folder is entirely absent is omitted
// (§4.2). Chapters outside any Part are collected under a single synthetic
// partRef{title: ""} so every caller sees exactly one shape.
func manifestView(dir string, m manifest, entries []fileEntry) manuscriptView {
	dirSet := map[string]bool{}
	for _, e := range entries {
		if e.isDir {
			dirSet[e.name] = true
		}
	}
	resolve := func(mc manifestChapter) (chapterRef, bool) {
		if !dirSet[mc.Folder] {
			return chapterRef{}, false
		}
		return chapterRef{
			folder: mc.Folder,
			title:  mc.Title,
			texts:  resolveChapterTexts(filepath.Join(dir, mc.Folder), mc.Texts),
		}, true
	}

	var parts []partRef
	var bare []chapterRef
	for _, it := range m.Items {
		if it.Chapter != nil {
			if cr, ok := resolve(*it.Chapter); ok {
				bare = append(bare, cr)
			}
			continue
		}
		var chapters []chapterRef
		for _, mc := range it.Chapters {
			if cr, ok := resolve(mc); ok {
				chapters = append(chapters, cr)
			}
		}
		parts = append(parts, partRef{title: it.Part, chapters: chapters})
	}
	// The synthetic untitled part (bare chapters) always comes first, matching
	// manifest order semantics where bare chapters and real Parts interleave —
	// v1 of this feature keeps bare-chapter items and Part items in the SAME
	// items[] order; a bare chapter appended by manifestInsert lands wherever
	// its item sits, so this loop above already preserves interleaving via
	// `parts` and `bare` in item order — collect bare into a leading synthetic
	// part only when there is at least one, so a manuscript with zero bare
	// chapters doesn't grow a spurious empty part.
	all := parts
	if len(bare) > 0 || len(parts) == 0 {
		all = append([]partRef{{title: "", chapters: bare}}, parts...)
	}

	loose := looseFiles(m, entries)
	title := m.Title
	if title == "" {
		title = projectTitle(filepath.Base(dir))
	}
	return manuscriptView{source: sourceManifest, title: title, parts: all, loose: loose}
}

// looseFiles returns every root-level .md file not referenced as a text inside
// any listed chapter — a Resource (design: unlisted files are Resources).
func looseFiles(m manifest, entries []fileEntry) []fileEntry {
	// Root-level looseness only concerns files directly in the manuscript root;
	// chapter folders themselves are never loose (they are dirs, filtered by
	// filterFiles at call sites that need flat files). Nothing under
	// manifest v2 lists root .md files as chapters anymore (chapters are always
	// folders), so any root .md file is loose by construction.
	var out []fileEntry
	for _, e := range entries {
		if !e.isDir {
			out = append(out, e)
		}
	}
	return out
}
```

Note d'implémentation : `looseFiles` simplifie par rapport à v1 — en v2, un chapitre est TOUJOURS un
dossier, donc un fichier `.md` à la racine du manuscrit ne peut plus jamais être un chapitre listé (c'était
possible en v1 où `items[].file` pointait un fichier plat). Tout fichier `.md` à la racine est
mécaniquement une Ressource. Ceci diffère du commentaire "unlisted is Resource" de v1 par simplification,
pas par changement de règle observable : le résultat (Ressource) est identique dans tous les cas testés.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./... -run TestResolve -v 2>&1 | tail -100`
Expected: all tests from Step 1 PASS.

Run: `go test ./... -run TestIsChapterOf -v 2>&1 | tail -20`
Expected: PASS.

- [ ] **Step 5: Build and vet**

Run: `go build ./... 2>&1 | head -60`
Expected: remaining errors confined to `filelist.go`, `corkboard.go`, `pager.go`, `export.go`,
`inspector.go`, `home.go`, `main.go`, `structure.go` — every reference to the old `.chapters`/`ch.file`
shape. These are fixed in Task 5. Confirm `manuscript.go` and `manifest.go` themselves compile clean by
checking no error line names those two files.

- [ ] **Step 6: Commit**

```bash
git add manuscript.go manuscript_test.go
git commit -m "$(cat <<'EOF'
Reconstruit manuscriptView autour de parts []partRef (Partie→Chapitre→Texte)

resolveManuscript lit désormais, pour chaque chapitre listé, le contenu de
son dossier (resolveChapterTexts) pour résoudre ses textes réellement
présents sur disque — même principe §4.2 qu'avant (jamais d'invention),
appliqué un niveau plus bas. Un manuscrit sans Partie explicite garde une
unique forme (synthetic partRef{title:""}) pour que tous les appelants
n'aient qu'un seul parcours à écrire. Le fallback legacy (fichiers
numérotés sans manifest) produit des chapitres à texte unique, folder="".
Le build reste rouge dans les 7 sites consommateurs, corrigés en Task 5.
EOF
)"
```

---

### Task 3: `slugify` translittère les accents, puis migration v1 → v2

**Files:**
- Modify: `outline.go` (`slugify`)
- Test: `outline_test.go`
- Create: `migration.go`
- Test: `migration_test.go`

**Interfaces:**
- Consumes: `manifest`/`manifestItem`/`manifestChapter`/`manifestText` (Task 1) ; `writeManifest(dir
  string, m manifest) error` (Task 1, inchangée) ; `atomicWrite` (`atomicwrite.go`, inchangé).
- Produces: `slugify(title string) string` (signature inchangée, comportement étendu — voir Step 0) ;
  ```go
  // manifestV1 is the pre-migration flat shape, decoded independently of the
  // current (v2) manifest type so migration can read a v1 file without
  // readManifest's v2-only schemaVersion gate rejecting it first.
  type manifestV1 struct {
      SchemaVersion int             `json:"schemaVersion"`
      Title         string          `json:"title"`
      Items         []manifestV1Item `json:"items"`
  }
  type manifestV1Item struct {
      File  string `json:"file"`
      Title string `json:"title"`
  }
  func readManifestV1(dir string) (manifestV1, bool, error)
  func needsMigration(dir string) bool
  func migratePlan(dir string, v1 manifestV1) (migrationStep, error)
  type migrationStep struct {
      moves []migrationMove
      out   manifest
  }
  type migrationMove struct {
      fromFile string // e.g. "01-un.md"
      toFolder string // e.g. "chapitre-un"
      toFile   string // e.g. "chapitre-un.md"
  }
  func migrateV1ToV2(dir string, v1 manifestV1) error
  ```
  consommé par Task 4 (l'écran de confirmation dans `main.go`).

- [ ] **Step 1: Write the failing test for accented slugify**

Ajoute à `outline_test.go` (à la fin du fichier) :

```go
func TestSlugifyTransliteratesAccents(t *testing.T) {
	cases := map[string]string{
		"Été":            "ete",
		"Éveil":          "eveil",
		"À l'aube":       "a-laube",
		"Le café brûlé":  "le-cafe-brule",
		"Noël":           "noel",
		"déjà vu":        "deja-vu",
		"Chapitre Un":    "chapitre-un",
	}
	for in, want := range cases {
		if got := slugify(in); got != want {
			t.Errorf("slugify(%q) = %q, want %q", in, got, want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run TestSlugifyTransliteratesAccents -v 2>&1 | tail -30`
Expected: FAIL — accented characters are silently dropped by the current `slugify` (e.g.
`slugify("Été")` today returns `"t"`, not `"ete"`).

- [ ] **Step 3: Add accent transliteration to `slugify`**

Dans `outline.go`, remplace `slugify` :

```go
// accentFold maps a rune outside a-z/0-9 to its unaccented ASCII equivalent when
// one exists, so a slug built from an accented French title reads as the words
// it came from ("Été" → "ete") instead of silently dropping the letter
// ("Été" → "t"). Covers the accented letters that occur in French prose;
// anything not listed here still falls through to slugify's existing
// punctuation-stripping behavior.
var accentFold = map[rune]rune{
	'à': 'a', 'â': 'a', 'ä': 'a', 'á': 'a', 'ã': 'a', 'å': 'a',
	'è': 'e', 'é': 'e', 'ê': 'e', 'ë': 'e',
	'ì': 'i', 'í': 'i', 'î': 'i', 'ï': 'i',
	'ò': 'o', 'ó': 'o', 'ô': 'o', 'ö': 'o', 'õ': 'o',
	'ù': 'u', 'ú': 'u', 'û': 'u', 'ü': 'u',
	'ý': 'y', 'ÿ': 'y',
	'ç': 'c', 'ñ': 'n',
	'œ': 'o', 'æ': 'a', // digraphs fold to their leading vowel — good enough for a slug
}

// slugify turns a typed section title into a filename slug: lowercase, accented
// letters transliterated to their unaccented ASCII form, spaces and underscores
// to hyphens, stripped of other punctuation.
func slugify(title string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(title)) {
		if folded, ok := accentFold[r]; ok {
			r = folded
		}
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '_' || r == '-':
			b.WriteByte('-')
		}
	}
	s := strings.Trim(b.String(), "-")
	if s == "" {
		s = "section"
	}
	return s
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./... -run TestSlugifyTransliteratesAccents -v 2>&1 | tail -30`
Expected: PASS.

- [ ] **Step 5: Run the full existing test suite to confirm no regression**

Run: `go test ./... -count=1 2>&1 | tail -40`
Expected: `ok` everywhere — `slugify` is used by `export.go`, `export_epub.go`, `rename.go`; this change
only IMPROVES their output for accented titles, it doesn't change behavior for already-plain-ASCII
titles (every existing test fixture in the repo uses plain-ASCII titles, so none of them can observe a
difference).

- [ ] **Step 6: Commit**

```bash
git add outline.go outline_test.go
git commit -m "$(cat <<'EOF'
slugify() translittère les accents au lieu de les supprimer silencieusement

"Été" produisait "t" (perte du É et du é) ; produit maintenant "ete".
Affecte tous les appelants existants (export.go, export_epub.go,
rename.go) en amélioration seule pour les titres accentués — aucun
fixture de test existant n'utilise de titre accentué, donc aucune
régression observable sur le comportement déjà testé. Nécessaire avant
la migration v1→v2 (Task suivante) : un chapitre "Éveil" doit produire
un dossier "eveil", pas un dossier tronqué à une lettre.
EOF
)"
```

- [ ] **Step 7: Write the failing tests**

Crée `migration_test.go` :

```go
package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNeedsMigrationTrueForV1(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, manifestName),
		[]byte(`{"schemaVersion":1,"title":"X","items":[{"file":"a.md","title":"A"}]}`), 0o644)
	if !needsMigration(dir) {
		t.Fatal("a schemaVersion:1 manifest must need migration")
	}
}

func TestNeedsMigrationFalseForV2(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, manifestName),
		[]byte(`{"schemaVersion":2,"title":"X","items":[]}`), 0o644)
	if needsMigration(dir) {
		t.Fatal("a schemaVersion:2 manifest must not need migration")
	}
}

func TestNeedsMigrationFalseForAbsentManifest(t *testing.T) {
	dir := t.TempDir()
	if needsMigration(dir) {
		t.Fatal("no manifest.json at all must not need migration")
	}
}

func TestMigratePlanBuildsMovesAndV2Manifest(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-un.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, "02-deux.md"), []byte("y"), 0o644)
	v1 := manifestV1{SchemaVersion: 1, Title: "Windermere", Items: []manifestV1Item{
		{File: "01-un.md", Title: "Un"},
		{File: "02-deux.md", Title: "Deux"},
	}}
	plan, err := migratePlan(dir, v1)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.moves) != 2 {
		t.Fatalf("moves = %+v, want 2", plan.moves)
	}
	if plan.moves[0].fromFile != "01-un.md" || plan.moves[0].toFolder != "un" || plan.moves[0].toFile != "un.md" {
		t.Fatalf("move 0 = %+v", plan.moves[0])
	}
	if plan.out.Title != "Windermere" || len(plan.out.Items) != 2 {
		t.Fatalf("out = %+v", plan.out)
	}
	if plan.out.Items[0].Chapter == nil || plan.out.Items[0].Chapter.Folder != "un" ||
		plan.out.Items[0].Chapter.Title != "Un" || plan.out.Items[0].Chapter.Texts[0].File != "un.md" {
		t.Fatalf("out item 0 = %+v", plan.out.Items[0])
	}
}

func TestMigratePlanEmptyTitleGetsDefaultChapterTitle(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-un.md"), []byte("x"), 0o644)
	v1 := manifestV1{SchemaVersion: 1, Title: "N", Items: []manifestV1Item{
		{File: "01-un.md", Title: ""}, // no title — safety-net path
	}}
	plan, err := migratePlan(dir, v1)
	if err != nil {
		t.Fatal(err)
	}
	if plan.out.Items[0].Chapter.Title != "Chapitre un" {
		t.Fatalf("default title = %q, want \"Chapitre un\"", plan.out.Items[0].Chapter.Title)
	}
}

func TestMigrateV1ToV2MovesFilesAndWritesV2Manifest(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-un.md"), []byte("contenu un"), 0o644)
	os.WriteFile(filepath.Join(dir, "02-deux.md"), []byte("contenu deux"), 0o644)
	os.WriteFile(filepath.Join(dir, manifestName),
		[]byte(`{"schemaVersion":1,"title":"Windermere","items":[
			{"file":"01-un.md","title":"Un"},{"file":"02-deux.md","title":"Deux"}]}`), 0o644)

	v1, present, err := readManifestV1(dir)
	if !present || err != nil {
		t.Fatalf("readManifestV1: present=%v err=%v", present, err)
	}
	if err := migrateV1ToV2(dir, v1); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dir, "01-un.md")); !os.IsNotExist(err) {
		t.Fatal("01-un.md must no longer exist at the manuscript root")
	}
	data, err := os.ReadFile(filepath.Join(dir, "un", "un.md"))
	if err != nil || string(data) != "contenu un" {
		t.Fatalf("un/un.md: data=%q err=%v", data, err)
	}
	data2, err := os.ReadFile(filepath.Join(dir, "deux", "deux.md"))
	if err != nil || string(data2) != "contenu deux" {
		t.Fatalf("deux/deux.md: data=%q err=%v", data2, err)
	}

	m, present, err := readManifest(dir)
	if !present || err != nil {
		t.Fatalf("readManifest post-migration: present=%v err=%v", present, err)
	}
	if m.SchemaVersion != manifestSchemaVersion {
		t.Fatalf("schemaVersion = %d, want %d", m.SchemaVersion, manifestSchemaVersion)
	}
	if len(m.Items) != 2 || m.Items[0].Chapter.Folder != "un" || m.Items[1].Chapter.Folder != "deux" {
		t.Fatalf("post-migration items = %+v", m.Items)
	}
}

func TestMigrateV1ToV2FailsAtomicallyIfAMoveFails(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-un.md"), []byte("x"), 0o644)
	// "deux.md" is declared but never created on disk — its rename will fail.
	os.WriteFile(filepath.Join(dir, manifestName),
		[]byte(`{"schemaVersion":1,"title":"N","items":[
			{"file":"01-un.md","title":"Un"},{"file":"missing.md","title":"Deux"}]}`), 0o644)

	v1, _, _ := readManifestV1(dir)
	err := migrateV1ToV2(dir, v1)
	if err == nil {
		t.Fatal("migration must fail when a listed file is missing")
	}
	// The manifest must remain the ORIGINAL v1 file — never partially rewritten.
	data, readErr := os.ReadFile(filepath.Join(dir, manifestName))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !contains(string(data), `"schemaVersion":1`) {
		t.Fatalf("manifest must still be the original v1 content after a failed migration, got: %s", data)
	}
}
```

- [ ] **Step 8: Run tests to verify they fail**

Run: `go test ./... -run TestMigrat -v 2>&1 | tail -60`
Expected: FAIL — `needsMigration`/`migratePlan`/`migrateV1ToV2`/`readManifestV1`/`manifestV1` undefined
(does not compile).

- [ ] **Step 9: Implement `migration.go`**

```go
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// manifestV1 is the pre-v2 flat shape: one item per chapter, File pointing
// directly at a manuscript-root .md file. Decoded independently of the current
// manifest type so migration can inspect a v1 file without readManifest's
// v2-only schemaVersion gate rejecting it first.
type manifestV1 struct {
	SchemaVersion int              `json:"schemaVersion"`
	Title         string           `json:"title"`
	Items         []manifestV1Item `json:"items"`
}

type manifestV1Item struct {
	File  string `json:"file"`
	Title string `json:"title"`
}

// readManifestV1 loads dir/manifest.json as the v1 shape, regardless of its
// schemaVersion field's actual value — the caller (needsMigration) decides
// whether migration applies.
func readManifestV1(dir string) (manifestV1, bool, error) {
	data, err := os.ReadFile(filepath.Join(dir, manifestName))
	if os.IsNotExist(err) {
		return manifestV1{}, false, nil
	}
	if err != nil {
		return manifestV1{}, true, err
	}
	var m manifestV1
	if err := json.Unmarshal(data, &m); err != nil {
		return manifestV1{}, true, err
	}
	return m, true, nil
}

// needsMigration reports whether dir has a manifest.json whose schemaVersion is
// exactly 1 — the one supported migration source. Any other version (including
// unreadable/malformed) is NOT this function's concern; readManifest's normal
// refuse-to-guess path handles those.
func needsMigration(dir string) bool {
	v1, present, err := readManifestV1(dir)
	return present && err == nil && v1.SchemaVersion == 1
}

// frenchOrdinalWords covers the first 20 chapters with a written-out French
// ordinal ("Chapitre un".."Chapitre vingt") for the empty-title safety net; a
// v1 manifest with more than 20 untitled chapters is not expected in practice
// (a valid v1 manifest already has items[].title filled in), so beyond this the
// title falls back to "Chapitre N" numerically rather than growing this table.
var frenchOrdinalWords = []string{
	"", "un", "deux", "trois", "quatre", "cinq", "six", "sept", "huit", "neuf", "dix",
	"onze", "douze", "treize", "quatorze", "quinze", "seize", "dix-sept", "dix-huit",
	"dix-neuf", "vingt",
}

func defaultChapterTitle(n int) string {
	if n >= 1 && n < len(frenchOrdinalWords) {
		return "Chapitre " + frenchOrdinalWords[n]
	}
	return fmt.Sprintf("Chapitre %d", n)
}

// migrationMove is one file relocation: a v1 root file into its new v2 chapter
// folder.
type migrationMove struct {
	fromFile string
	toFolder string
	toFile   string
}

// migrationStep is a fully-computed migration: the file moves to perform, and
// the v2 manifest to write once every move has succeeded.
type migrationStep struct {
	moves []migrationMove
	out   manifest
}

// migratePlan computes the migration without touching disk: for each v1 item,
// the folder/file slug is derived from its title (falling back to a default
// "Chapitre N" title only if the v1 title is empty — the safety net documented
// in the design; a valid v1 manifest already has titles filled in).
func migratePlan(dir string, v1 manifestV1) (migrationStep, error) {
	var step migrationStep
	step.out.Title = v1.Title
	for i, it := range v1.Items {
		title := it.Title
		if title == "" {
			title = defaultChapterTitle(i + 1)
		}
		slug := slugify(title)
		toFile := slug + ".md"
		step.moves = append(step.moves, migrationMove{
			fromFile: it.File,
			toFolder: slug,
			toFile:   toFile,
		})
		step.out.Items = append(step.out.Items, manifestItem{
			Chapter: &manifestChapter{
				Folder: slug,
				Title:  title,
				Texts:  []manifestText{{File: toFile, Title: title}},
			},
		})
	}
	return step, nil
}

// migrateV1ToV2 executes the migration for dir: moves each v1 chapter file into
// its own new folder, then writes the v2 manifest ONCE at the end — only if
// every move succeeded. If any move fails partway through, migration stops and
// returns the error without writing the manifest, so the original v1 manifest
// (and whatever files were already moved) is the only state change on disk; the
// caller is expected to surface the error rather than retry automatically.
func migrateV1ToV2(dir string, v1 manifestV1) error {
	plan, err := migratePlan(dir, v1)
	if err != nil {
		return err
	}
	for _, mv := range plan.moves {
		fromPath := filepath.Join(dir, mv.fromFile)
		if _, err := os.Stat(fromPath); err != nil {
			return fmt.Errorf("migration: %s: %w", mv.fromFile, err)
		}
		toDir := filepath.Join(dir, mv.toFolder)
		if err := os.MkdirAll(toDir, 0o755); err != nil {
			return fmt.Errorf("migration: mkdir %s: %w", mv.toFolder, err)
		}
		if err := os.Rename(fromPath, filepath.Join(toDir, mv.toFile)); err != nil {
			return fmt.Errorf("migration: move %s: %w", mv.fromFile, err)
		}
	}
	return writeManifest(dir, plan.out)
}
```

Le fichier final importe `encoding/json`, `fmt`, `os`, `path/filepath` — rien d'autre.

- [ ] **Step 10: Run tests to verify they pass**

Run: `go test ./... -run TestNeedsMigration -v 2>&1 | tail -40`
Expected: PASS.

Run: `go test ./... -run TestMigratePlan -v 2>&1 | tail -60`
Expected: PASS.

Run: `go test ./... -run TestMigrateV1ToV2 -v 2>&1 | tail -80`
Expected: PASS, including the partial-failure test (`TestMigrateV1ToV2FailsAtomicallyIfAMoveFails`).

- [ ] **Step 11: Build and vet**

Run: `go build ./... 2>&1 | head -60`
Expected: same remaining errors as end of Task 2 (the 7 consumer sites) — `migration.go` itself compiles
clean.

- [ ] **Step 12: Commit**

```bash
git add migration.go migration_test.go
git commit -m "$(cat <<'EOF'
Ajoute la migration v1→v2 du manifest (fichier plat → dossier-chapitre)

migrateV1ToV2 déplace chaque fichier-chapitre v1 dans un nouveau dossier
nommé par slug du titre (safety-net "Chapitre N" si le titre v1 est
vide), puis écrit le manifest v2 UNE SEULE fois à la fin — seulement si
tous les déplacements ont réussi. Un échec partiel laisse le manifest v1
original intact (aucune réécriture), remonte l'erreur, ne retente rien
automatiquement. needsMigration/readManifestV1 lisent le fichier
indépendamment de la porte schemaVersion=2 de readManifest.
EOF
)"
```

---

### Task 4: Écran de confirmation de migration

**Files:**
- Modify: `main.go` (état `model`, accroche dans `enterWriting`, gestion clavier, rendu)
- Test: `main_test.go` (ou `migration_test.go` si `main_test.go` n'existe pas — vérifier lequel existe
  avant d'écrire)

**Interfaces:**
- Consumes: `needsMigration(dir string) bool`, `migratePlan(dir string, v1 manifestV1)
  (migrationStep, error)`, `migrateV1ToV2(dir string, v1 manifestV1) error`, `readManifestV1(dir
  string) (manifestV1, bool, error)` (Task 3).
- Produces: `model.migrationPending *migrationStep` (nil = pas d'écran affiché) et
  `model.migrationDir string`, consommés uniquement en interne à `main.go` — aucun task suivant n'en
  dépend directement (Task 5 ne touche pas à cet écran).

- [ ] **Step 1: Check which test file already exists for `main.go` model-level flows**

Run: `ls main_test.go 2>&1`

Si le fichier existe, ajoute les tests ci-dessous à la fin. S'il n'existe pas, crée-le avec le
`package main` + imports nécessaires (`os`, `path/filepath`, `testing`) en tête.

- [ ] **Step 2: Write the failing tests**

```go
func TestEnterWritingFlagsV1ManifestForMigration(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-un.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, manifestName),
		[]byte(`{"schemaVersion":1,"title":"N","items":[{"file":"01-un.md","title":"Un"}]}`), 0o644)

	m := initialModel()
	m.files.SetDir(dir)
	m.enterWriting()

	if m.migrationPending == nil {
		t.Fatal("enterWriting on a v1 manuscript must set migrationPending")
	}
	if m.screen == screenWriting {
		t.Fatal("the writing screen must not be entered until migration is confirmed or cancelled")
	}
}

func TestEnterWritingSkipsMigrationForV2Manifest(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "un"), 0o755)
	os.WriteFile(filepath.Join(dir, "un", "un.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, manifestName),
		[]byte(`{"schemaVersion":2,"title":"N","items":[{"chapter":{"folder":"un","title":"Un","texts":[{"file":"un.md","title":"Un"}]}}]}`), 0o644)

	m := initialModel()
	m.files.SetDir(dir)
	m.enterWriting()

	if m.migrationPending != nil {
		t.Fatal("a v2 manuscript must never trigger migrationPending")
	}
	if m.screen != screenWriting {
		t.Fatal("a v2 manuscript must enter the writing screen directly")
	}
}

func TestConfirmMigrationExecutesAndEntersWriting(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-un.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, manifestName),
		[]byte(`{"schemaVersion":1,"title":"N","items":[{"file":"01-un.md","title":"Un"}]}`), 0o644)

	m := initialModel()
	m.files.SetDir(dir)
	m.enterWriting()
	if m.migrationPending == nil {
		t.Fatal("setup: expected migrationPending")
	}

	m.confirmMigration()

	if m.migrationPending != nil {
		t.Fatal("confirmMigration must clear migrationPending")
	}
	if m.screen != screenWriting {
		t.Fatal("confirmMigration must enter the writing screen")
	}
	if _, err := os.Stat(filepath.Join(dir, "un", "un.md")); err != nil {
		t.Fatalf("migrated file must exist: %v", err)
	}
}

func TestCancelMigrationLeavesV1ManifestUntouched(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-un.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, manifestName),
		[]byte(`{"schemaVersion":1,"title":"N","items":[{"file":"01-un.md","title":"Un"}]}`), 0o644)

	m := initialModel()
	m.files.SetDir(dir)
	m.enterWriting()

	m.cancelMigration()

	if m.migrationPending != nil {
		t.Fatal("cancelMigration must clear migrationPending")
	}
	data, err := os.ReadFile(filepath.Join(dir, manifestName))
	if err != nil {
		t.Fatal(err)
	}
	if !contains(string(data), `"schemaVersion":1`) {
		t.Fatalf("cancelling must leave the v1 manifest untouched, got: %s", data)
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go build ./... 2>&1 | head -20`
Expected: `m.migrationPending undefined`, `m.confirmMigration undefined`, `m.cancelMigration undefined` —
does not compile (expected at this point; the 7 consumer sites are still also broken from Task 2, ignore
those errors specifically and focus on the new migration-related ones).

- [ ] **Step 4: Add migration state to `model` and wire `enterWriting`**

Dans `main.go`, trouve la déclaration du struct `model` (cherche `structureItems      []manifestItem`
comme point de repère, ~ligne 224) et ajoute deux champs à proximité des autres champs d'état d'écran
modal (près de `structureConfirm bool`) :

```go
	migrationPending *migrationStep // non-nil while the v1→v2 confirm screen is showing
	migrationDir     string         // dir being migrated (for confirmMigration/cancelMigration)
```

Remplace `enterWriting` :

```go
func (m *model) enterWriting() {
	if needsMigration(m.files.dir) {
		v1, _, err := readManifestV1(m.files.dir)
		if err == nil {
			if plan, planErr := migratePlan(m.files.dir, v1); planErr == nil {
				m.migrationPending = &plan
				m.migrationDir = m.files.dir
				return
			}
		}
		// A v1 manifest that fails to plan (malformed title data, etc.) falls
		// through to the normal refuse-to-guess path: applyProjectSettings +
		// screenWriting proceed, and resolveManuscript's existing "unsupported
		// schemaVersion" warning path (readManifest still gates on v2) surfaces
		// the problem to the author exactly like any other unreadable manifest.
	}
	m.applyProjectSettings()
	m.screen = screenWriting
}

// confirmMigration executes the pending v1→v2 migration and enters the writing
// screen. Called when the author accepts the migration confirm screen.
func (m *model) confirmMigration() {
	if m.migrationPending == nil {
		return
	}
	v1, _, err := readManifestV1(m.migrationDir)
	if err == nil {
		_ = migrateV1ToV2(m.migrationDir, v1) // best-effort; errors surface via the
		// manuscript's warning path on next resolveManuscript, consistent with
		// every other write path in this codebase (no modal error dialog).
	}
	m.migrationPending = nil
	m.applyProjectSettings()
	m.screen = screenWriting
}

// cancelMigration dismisses the pending confirm screen without touching disk;
// the manuscript is left exactly as it was (still v1, still readable next time
// via needsMigration).
func (m *model) cancelMigration() {
	m.migrationPending = nil
	m.applyProjectSettings()
	m.screen = screenWriting
}
```

- [ ] **Step 5: Wire key handling for the confirm screen**

Trouve le début de `Update` (la fonction `func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd)`) et,
juste après le `case tea.KeyMsg:` initial (avant tout autre traitement de touche — ce garde doit
intercepter les touches AVANT le reste de la logique d'écran puisque `m.screen` reste `screenHome`
tant que `migrationPending != nil`), ajoute :

```go
	if km, ok := msg.(tea.KeyMsg); ok && m.migrationPending != nil {
		switch km.String() {
		case "enter":
			m.confirmMigration()
		case "esc":
			m.cancelMigration()
		}
		return m, nil
	}
```

Vérifie l'emplacement exact en cherchant la ligne `func (m model) Update(msg tea.Msg)` dans `main.go` et
place ce bloc comme la toute première chose évaluée dans le corps de la fonction, avant le `switch
msg := msg.(type)` existant.

- [ ] **Step 6: Render the confirm screen**

Trouve `func (m model) View() string` dans `main.go`. Juste après son ouverture (avant le `switch
m.screen` existant), ajoute :

```go
	if m.migrationPending != nil {
		return migrationConfirmView(m.migrationPending, m.width)
	}
```

Ajoute la fonction de rendu (dans `main.go`, à la suite de `View`, ou dans `migration.go` si tu préfères
garder tout ce qui touche à la migration groupé — dans ce cas ajoute-la à la fin de `migration.go` avec
un import `"strings"` supplémentaire) :

```go
// migrationConfirmView renders the v1→v2 migration confirm screen: the list of
// file→folder moves about to happen, and the two available actions.
func migrationConfirmView(plan *migrationStep, width int) string {
	var b strings.Builder
	b.WriteString("Ce manuscrit utilise l'ancien format de chapitres.\n\n")
	fmt.Fprintf(&b, "  %d chapitres seront convertis en dossiers :\n", len(plan.moves))
	for _, mv := range plan.moves {
		fmt.Fprintf(&b, "    %s → %s/%s\n", mv.fromFile, mv.toFolder, mv.toFile)
	}
	b.WriteString("\n  Chaque chapitre gagnera un titre par défaut si nécessaire\n")
	b.WriteString("  (« Chapitre un », « Chapitre deux », …) — les titres déjà\n")
	b.WriteString("  personnalisés dans le manifest sont conservés tels quels.\n\n")
	b.WriteString("  [Entrée] convertir maintenant   [Échap] annuler et fermer")
	return b.String()
}
```

Si `migration.go` reçoit cette fonction, ajoute `"strings"` et `"fmt"` à ses imports (vérifie que `fmt`
n'y est pas déjà importé avant de le dupliquer).

- [ ] **Step 7: Run tests to verify they pass**

Run: `go test ./... -run "TestEnterWriting|TestConfirmMigration|TestCancelMigration" -v 2>&1 | tail -80`
Expected: all 4 tests from Step 2 PASS.

- [ ] **Step 8: Build and vet**

Run: `go build ./... 2>&1 | head -60`
Expected: same remaining errors as end of Task 3 (the 7 consumer sites, Task 5) — no NEW errors
introduced by this task.

- [ ] **Step 9: Commit**

```bash
git add main.go main_test.go migration.go
git commit -m "$(cat <<'EOF'
Ajoute l'écran de confirmation de migration v1→v2 à l'ouverture

enterWriting() détecte un manifest v1 et bascule vers un écran de
confirmation listant les déplacements fichier→dossier à venir, avant
toute écriture disque. Entrée confirme et exécute migrateV1ToV2 ; Échap
annule et laisse le manifest v1 intact — okashi redemandera à la
prochaine ouverture, pas de blocage permanent ni de contournement
silencieux.
EOF
)"
```

---

### Task 5: Adapter les 7 sites consommateurs à `manuscriptView.parts`

**Files:**
- Modify: `filelist.go`, `corkboard.go`, `pager.go`, `export.go`, `inspector.go`, `home.go`,
  `structure.go`
- Test: `filelist_test.go`, `corkboard_test.go`, `pager_test.go`, `export_wiring_test.go`,
  `inspector_test.go`, `home_test.go` (adapter les tests existants qui référencent `.chapters`/`ch.file`
  à la nouvelle forme — vérifier au cas par cas lesquels existent avant modification)

**Interfaces:**
- Consumes: `manuscriptView{parts []partRef, loose []fileEntry, ...}`, `partRef{title string,
  chapters []chapterRef}`, `chapterRef{folder string, title string, texts []textRef}`,
  `textRef{file string, title string, words int}` (Task 2) ; `isChapterOf(v manuscriptView, folder
  string) bool` (Task 2).
- Produces: rien de nouveau consommé par un task ultérieur — ce plan s'arrête ici ; le plan suivant
  (Lecture/affichage) construira le rendu visuel Partie par-dessus ce parcours mécanique.

Cette tâche est purement mécanique par construction (validé dans la conception ci-dessus) : chaque site
remplace sa boucle `for _, ch := range v.chapters` par une boucle imbriquée `for _, p := range v.parts {
for _, ch := range p.chapters { ... } }`, sans encore afficher d'en-tête de Partie ni concaténer les
textes d'un chapitre — seul le **premier texte** de chaque chapitre est utilisé pour l'instant partout où
v1 utilisait `ch.file` (comportement dégradé mais fonctionnel : le plan suivant remplacera ce
"premier texte seulement" par le vrai traitement multi-texte).

- [ ] **Step 1: `filelist.go` — `SetDir`, `sectionRow`, `chapterTitle`**

Dans `SetDir` (`filelist.go:71-118`), remplace :

```go
	f.view = resolveManuscript(dir, files)

	// Build the ordered entry list: dirs first, then chapters in view order, then loose.
	f.entries = append(f.entries, dirs...)
	for _, ch := range f.view.chapters {
		f.entries = append(f.entries, fileEntry{name: ch.file})
	}
	f.entries = append(f.entries, f.view.loose...)
```

par :

```go
	f.view = resolveManuscript(dir, files)

	// Build the ordered entry list: dirs first, then chapters in view order
	// (flattened across parts — Part headers are the next plan's job), then loose.
	// A chapter is represented in f.entries by its folder name, since it is now
	// a directory on disk, not a direct file.
	f.entries = append(f.entries, dirs...)
	for _, p := range f.view.parts {
		for _, ch := range p.chapters {
			f.entries = append(f.entries, fileEntry{name: ch.folder, isDir: true})
		}
	}
	f.entries = append(f.entries, f.view.loose...)
```

Remplace `sectionRow` (`filelist.go:189-211`) — le compteur de mots devient la somme des textes du
chapitre au lieu d'un seul fichier :

```go
func (f filelist) sectionRow(e fileEntry, dimCount bool) string {
	n := f.chapterWords(e.name)
	count := commafy(n) + " m"
	g := f.icons.iconFor(e)
	left := " " + renderIcon(g, !dimCount) + f.chapterTitle(e.name)
	maxLeft := f.width - lipgloss.Width(count) - 1
	if maxLeft < 1 {
		maxLeft = 1
	}
	left = ansi.Truncate(left, maxLeft, "…")
	gap := f.width - lipgloss.Width(left) - lipgloss.Width(count)
	if gap < 1 {
		gap = 1
	}
	rendered := count
	if dimCount {
		rendered = lipgloss.NewStyle().Foreground(subtle).Render(count)
	}
	return left + strings.Repeat(" ", gap) + rendered
}

// chapterWords sums the word counts of every text in the chapter whose folder is
// name, looking it up from the resolved view. Falls back to a direct file count
// (via f.wc) for legacy/zero-view entries where folder == "" doesn't apply.
func (f filelist) chapterWords(folder string) int {
	for _, p := range f.view.parts {
		for _, ch := range p.chapters {
			if ch.folder != folder {
				continue
			}
			total := 0
			for _, t := range ch.texts {
				total += f.wc.count(filepath.Join(f.dir, ch.folder, t.file))
			}
			return total
		}
	}
	if f.wc != nil {
		return f.wc.count(filepath.Join(f.dir, folder))
	}
	return 0
}
```

Remplace `chapterTitle` (`filelist.go:215-222`) :

```go
// chapterTitle returns the display title for a chapter folder, looking it up
// from the resolved view. Falls back to sectionTitle (for zero-view or legacy
// entries where folder is actually a flat filename).
func (f filelist) chapterTitle(folder string) string {
	for _, p := range f.view.parts {
		for _, ch := range p.chapters {
			if ch.folder == folder {
				return ch.title
			}
		}
	}
	return sectionTitle(folder)
}
```

- [ ] **Step 2: `corkboard.go`, `pager.go`, `export.go`, `inspector.go`, `home.go` — aplatir `v.parts`**

Dans chacun des fichiers suivants, chaque occurrence de `for _, ch := range v.chapters` (ou `sm.Items`/
`mani.Items` là où le code parcourt encore directement le manifest plutôt que la vue résolue — vérifier
au cas par cas) devient `for _, p := range v.parts { for _, ch := range p.chapters { ... } }`, et chaque
usage de `ch.file` (le fichier direct v1) devient `ch.texts[0].file` (le premier texte du chapitre —
comportement dégradé assumé pour ce plan, voir intro de la Task).

Sites précis à corriger (grep `\.chapters\b` et `ch\.file\b`/`it\.File\b` sur le manuscrit résolu, PAS
sur `manifestItem` qui n'a plus de `.File` du tout depuis Task 1 — tout appelant encore sur
`manifestItem.File` est une régression à corriger ici) :

- `corkboard.go` : la construction `m.structureItems` (staging du structure mode) référence
  aujourd'hui `manifestItem` directement — depuis Task 1, `manifestItem` n'a plus de champ `.File`/
  `.Title` plat. Pour CE plan (le structure mode plein niveau 3 est le plan suivant), réduis le staging
  à parcourir uniquement les chapitres nus de la vue (`v.parts[0].chapters` si `v.parts[0].title ==
  ""`) en `[]chapterRef`, ignore les Parties pour l'instant (elles ne sont pas éditables depuis le
  structure mode dans ce plan) : change `m.structureItems []manifestItem` en
  `m.structureItems []chapterRef` et adapte chaque site qui le lit/écrit (`.File`→`.folder`,
  `.Title`→`.title`) en conséquence ; le commit de sortie du structure mode réécrit uniquement les
  chapitres nus, en préservant les Parties existantes du manifest telles quelles (lis le manifest
  courant, ne remplace que la portion `Chapter != nil` correspondant à l'ordre de
  `m.structureItems`, sans toucher aux items `Part != ""`).
- `pager.go` `load` (`pager.go:39-79`) : la boucle `for _, ch := range v.chapters` devient l'itération
  imbriquée ; `pagerLine.file` devient le chemin relatif complet `filepath.Join(ch.folder,
  ch.texts[0].file)` (pas juste `ch.texts[0].file`), pour que `filepath.Join(dir, file)` au site
  d'appel (`main.go`, déjà `filepath.Join(m.pager.dir, file)`) continue de résoudre au bon fichier sans
  changement côté `main.go`. `os.ReadFile(filepath.Join(dir, ch.file))` devient
  `os.ReadFile(filepath.Join(dir, ch.folder, ch.texts[0].file))`, protégé par `if len(ch.texts) == 0 {
  continue }` (chapitre vide toléré, pas de panique sur `ch.texts[0]`).
- `export.go` (`manuscriptDocFromChapters(dir, v.chapters)`, ligne 123) : la signature devient
  `manuscriptDocFromChapters(dir string, parts []partRef) ManuscriptDoc` — aplatit en interne (`for _,
  p := range parts { for _, ch := range p.chapters { ... } }`), lit `ch.texts[0].file` si `len(ch.texts)
  > 0` sinon produit une `Section` avec un `Blocks` vide (chapitre vide toléré, pas de crash).
- `inspector.go` `computeProjStats` (`inspector.go:531-539`) : `ps.chapters` devient le compte total
  aplati (`for _, p := range v.parts { ps.chapters += len(p.chapters) }`), `ps.words` somme
  `wc.count(filepath.Join(dir, ch.folder, ch.texts[0].file))` pour chaque chapitre non vide.
- `home.go` (`mk(ch.title, ch.file)`, ligne ~113) : `mk(ch.title, filepath.Join(ch.folder,
  ch.texts[0].file))` à l'intérieur de la boucle imbriquée, protégé par le même garde `len(ch.texts) >
  0`.

Pour chaque site, applique le changement mécaniquement en suivant le pattern ci-dessus — la boucle
externe `for _, p := range v.parts` et interne `for _, ch := range p.chapters` remplace toujours la
simple `for _, ch := range v.chapters` d'origine ; aucun autre changement de logique n'est nécessaire
dans ce plan.

- [ ] **Step 3: `structure.go` — adapter au nouveau `chapterRef`**

Ouvre `structure.go` et cherche toute référence à `c.file`/`c.title` sur un `chapterRef` — avec Task 2,
`chapterRef` a maintenant `folder`/`title`/`texts` (plus de `.file`). Remplace chaque `c.file` par
`c.folder`. Si `structure.go` référence `manifestInsert`/`manifestRemove`/`manifestReorder` (supprimées
en Task 1), ce fichier doit être réduit pour cette tâche à ce qui reste utilisable sans ces fonctions —
si son seul rôle est de servir ces trois fonctions et n'a plus d'autre appelant après cette Task, vérifie
avec `grep -rn "structure\." *.go | grep -v _test | grep -v structure.go` si un symbole de ce fichier est
encore référencé ailleurs ; si aucun ne l'est, supprime le fichier entièrement plutôt que de le laisser
à moitié fonctionnel (le structure mode complet revient dans le plan suivant, réécrit contre la forme
v2).

- [ ] **Step 4: Update existing tests referencing the old flat shape**

Pour chacun des fichiers de test suivants, cherche les occurrences de `.chapters` / `ch.file` /
`manifestItem{File:...}` sur les types touchés par cette tâche et adapte-les à la nouvelle forme, en
suivant exactement les mêmes fixtures que celles introduites dans `manuscript_test.go` (Task 2, helper
`mkChapterDir`) — un test qui créait `os.WriteFile(dir/"01-a.md", ...)` puis un manifest v1 doit
maintenant créer un dossier-chapitre (`mkChapterDir`) et un manifest v2. Exécute d'abord :

Run: `go build ./... 2>&1 | head -80`

et corrige fichier par fichier dans l'ordre des erreurs de compilation rapportées, en commençant par
`filelist_test.go`, puis `corkboard_test.go`, `pager_test.go`, `export_wiring_test.go`,
`inspector_test.go`, `home_test.go` — chacun affiche l'erreur exacte (champ manquant, type incompatible)
qui indique précisément quoi corriger.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./... -count=1 2>&1 | tail -100`
Expected: `ok` for every package — zero FAIL, zero build error.

- [ ] **Step 6: Build and vet**

Run: `go build ./... && go vet ./... 2>&1 | head -60`
Expected: clean build, no vet warnings.

- [ ] **Step 7: Commit**

```bash
git add filelist.go corkboard.go pager.go export.go inspector.go home.go structure.go \
  filelist_test.go corkboard_test.go pager_test.go export_wiring_test.go inspector_test.go home_test.go
git commit -m "$(cat <<'EOF'
Adapte les 7 sites consommateurs à manuscriptView.parts (Partie→Chapitre→Texte)

Chaque site (sidebar, corkboard, pager, export, inspecteur, hub) parcourt
désormais parts→chapters au lieu de chapters directement ; un chapitre
est représenté par son dossier (fileEntry{name: folder, isDir: true})
dans la sidebar plutôt qu'un fichier direct. Comportement dégradé assumé
pour ce plan : seul le premier texte de chaque chapitre est lu/affiché/
exporté (pas d'en-tête de Partie, pas de concaténation multi-texte) — le
plan suivant ajoute le rendu visuel complet par-dessus ce parcours
mécanique déjà correct. structure.go réduit/supprimé (le structure mode
à 3 niveaux revient réécrit dans le plan suivant).
EOF
)"
```

---

### Task 6: Vérification manuelle

**Files:** none — vérification uniquement, aucun changement de code.

**Interfaces:**
- Consumes: la fonctionnalité complète des Tasks 1–5.
- Produces: rien de nouveau — confirme que le modèle v2, la migration et le parcours mécanique
  fonctionnent ensemble sur un vrai manuscrit, dans un vrai terminal.

- [ ] **Step 1: Full build, vet, test suite**

Run: `go build ./... && go vet ./... && go test ./... -count=1 2>&1 | tail -40`
Expected: clean build, no vet warnings, all tests PASS.

- [ ] **Step 2: Manual walkthrough in tmux — migration d'un manuscrit v1**

```bash
go build -o /tmp/forkashi-parts-check .
rm -rf /tmp/parts-check-home /tmp/parts-check-proj
mkdir -p /tmp/parts-check-home /tmp/parts-check-proj/Livre
cat > /tmp/parts-check-proj/Livre/manifest.json <<'EOF'
{"schemaVersion":1,"title":"Mon Livre","items":[{"file":"01-un.md","title":"Chapitre Un"},{"file":"02-deux.md","title":"Chapitre Deux"}]}
EOF
echo "Contenu du premier chapitre." > /tmp/parts-check-proj/Livre/01-un.md
echo "Contenu du second chapitre." > /tmp/parts-check-proj/Livre/02-deux.md
tmux new-session -d -s partscheck -x 120 -y 40 \
  "cd /tmp/parts-check-proj/Livre && HOME=/tmp/parts-check-home XDG_CONFIG_HOME=/tmp/parts-check-home/.config OKASHI_DIR=/tmp/parts-check-proj/Livre /tmp/forkashi-parts-check"
sleep 1
tmux capture-pane -t partscheck -p
```

Walk through, in order:

1. Depuis le hub, ouvre le projet "Livre" (`⏎`) — confirme que l'écran de confirmation de migration
   s'affiche, listant `01-un.md → chapitre-un/chapitre-un.md` et `02-deux.md → chapitre-deux/
   chapitre-deux.md` (les slugs exacts dépendent de `slugify` — vérifie ce que la capture affiche
   réellement plutôt que de supposer le slug).
2. `Entrée` pour confirmer — confirme que l'écran bascule vers la sidebar normale, affichant les deux
   chapitres.
3. `tmux send-keys -t partscheck 'C-c'` puis relance la même commande `tmux new-session` — rouvre le
   même projet, confirme que l'écran de migration NE réapparaît PAS cette fois (le manifest est
   maintenant v2).
4. Ouvre le chapitre "Chapitre Un" (`⏎`) — confirme qu'il s'ouvre directement dans l'éditeur (un seul
   texte, pas de sélecteur) et affiche bien "Contenu du premier chapitre."

Documente toute anomalie visuelle trouvée pendant ce parcours.

- [ ] **Step 3: Verify on-disk state directly**

```bash
find /tmp/parts-check-proj/Livre -type f | sort
cat /tmp/parts-check-proj/Livre/manifest.json
```

Expected: `manifest.json` a `"schemaVersion": 2`, et deux dossiers de chapitre existent, chacun contenant
son fichier texte unique ; `01-un.md`/`02-deux.md` n'existent plus à la racine.

- [ ] **Step 4: Clean up**

```bash
tmux send-keys -t partscheck 'C-c'
tmux kill-session -t partscheck 2>/dev/null
rm -f /tmp/forkashi-parts-check
rm -rf /tmp/parts-check-home /tmp/parts-check-proj
```

- [ ] **Step 5: Report findings**

No commit for this task (verification only). Si l'étape 2 ou 3 a fait apparaître une anomalie, la
signaler — ne pas la corriger silencieusement sous cette tâche sauf correction triviale et manifestement
sûre ; sinon la traiter comme un suivi pour le plan suivant.

---

## Self-Review

**Spec coverage** (contre `docs/superpowers/specs/2026-08-04-parts-and-multi-text-chapters-design.md`) :
- Modèle de données (manifest v2, disque, `manuscriptView`) → Task 1 + Task 2.
- Migration v1→v2 (déclenchement, écran, exécution atomique) → Task 3 + Task 4.
- Sites consommateurs (§"Sites consommateurs impactés") → Task 5, avec le comportement dégradé
  "premier texte seulement" explicitement assumé et documenté comme tel (le rendu Partie/multi-texte
  complet est le plan suivant, hors périmètre annoncé dans Global Constraints).
- UX de création/réorganisation (`ctrl+n`, structure mode étendu, sélecteur de texte) : explicitement
  HORS PÉRIMÈTRE de ce plan (annoncé dans Global Constraints) — sujet du plan suivant.
- Export (concaténation, rupture de Partie, avertissements vide) : explicitement HORS PÉRIMÈTRE de ce
  plan — sujet du plan suivant (Task 5 ne fait que router `manuscriptDocFromChapters` vers le premier
  texte, sans rupture de section ni avertissement).
- Promotion depuis l'outline : HORS PÉRIMÈTRE de ce plan (pas encore de création de chapitre/Partie
  dans ce plan).

**Placeholder scan:** aucun "TBD"/"TODO" ; chaque step de code contient le code réel à écrire, prêt à
copier sans nettoyage après coup.

**Type consistency:** `chapterRef{folder, title, texts}` (Task 2) est le type utilisé identiquement dans
Task 4 (`migrationStep`/`migrationConfirmView` ne le référencent pas directement — ils travaillent sur
`manifestChapter`/`manifestText`, cohérent) et Task 5 (tous les sites). `partRef{title, chapters}` (Task
2) est utilisé identiquement dans Task 5. `textRef{file, title, words}` (Task 2) — le champ `words` est
déclaré mais non encore peuplé par `resolveChapterTexts` dans Task 2 (il reste à 0, calculé à la demande
par les sites consommateurs via `wc.count` comme aujourd'hui) ; ce n'est pas un bug pour CE plan — le
sélecteur de texte qui a besoin de `words` par texte à l'affichage est explicitly le plan suivant
(§"Ouverture d'un chapitre depuis la sidebar" dans la spec), donc `textRef.words` reste un champ présent
mais non consommé dans ce plan, cohérent avec le fait qu'aucune Task de ce plan ne le lit.
