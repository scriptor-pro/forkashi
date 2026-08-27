# Arborescence sidebar pliable — Plan d'implémentation

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Un chapitre multi-scènes affiche ses scènes inline dans la sidebar (indentées, sous
un nœud pliable/dépliable) au lieu de router `enter` vers l'écran séparé `screenTextPicker`.
L'état plié/déplié est persisté par chapitre dans un nouveau sidecar `.okashi-folded.json`,
replié par défaut.

**Architecture:** Nouveau fichier `fold.go` (sidecar JSON, calqué exactement sur
`synopsis.go`). `filelist.go` gagne un champ `isChildScene` sur `fileEntry`, un état
`folded map[string]bool` + `foldedRoot string` sur `filelist`, une insertion de lignes
enfants dans `SetDir`, et un nouveau branchement dans `activate()`. `main.go` retire les
deux call sites qui routaient `activateTextPicker` vers `enterTextPicker()` depuis la
sidebar (`screenTextPicker` reste utilisé par le corkboard et `ctrl+n`, non touchés ici).

**Tech Stack:** Go 1.25, Bubble Tea, lipgloss, encoding/json, tests `go test ./...`.

**Spec:** [docs/superpowers/specs/2026-08-27-sidebar-scene-tree-design.md](../specs/2026-08-27-sidebar-scene-tree-design.md)

## Global Constraints

- Sidecar okashi-owned, jamais dans `manifest.json` — pas de HARD GATE déclenché (schéma
  manifest inchangé).
- Écriture atomique obligatoire via `atomicWrite` (comme tout sidecar existant).
- Clé du sidecar = `corkKey(chapterRef)` (fonction existante, `corkboard.go:30-38`) —
  jamais une nouvelle fonction de clé.
- Absence de clé = **replié** (comportement par défaut demandé) ; le sidecar ne stocke que
  les chapitres explicitement dépliés (`true`).
- Un chapitre à 0 ou 1 texte, ou une scène standalone (`ch.scene`), n'affiche jamais de
  triangle ni de ligne enfant — comportement strictement inchangé pour ces cas.
- `screenTextPicker` n'est pas supprimé ni modifié — seuls ses deux call sites côté sidebar
  disparaissent.
- Sérialisation JSON : 2 espaces d'indentation, `SetEscapeHTML(false)`, trailing newline
  trimé (`bytes.TrimRight(buf.Bytes(), "\n")`) — identique à `writeManifest`/`saveSynopses`.

---

## Task 1: Sidecar `.okashi-folded.json` (`fold.go`)

**Files:**
- Create: `fold.go`
- Test: `fold_test.go`

**Interfaces:**
- Consumes: `atomicWrite(path string, data []byte, perm os.FileMode) error` (`atomicwrite.go`).
- Produces: `foldedPath(dir string) string`, `loadFolded(dir string) map[string]bool`,
  `saveFolded(dir string, folded map[string]bool, known map[string]bool) error` — utilisés
  par Task 2 (`filelist.go`).

- [ ] **Step 1: Write the failing tests**

```go
// fold_test.go
package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFoldedRoundTripAndPrune(t *testing.T) {
	dir := t.TempDir()
	known := map[string]bool{"01-a": true, "02-b": true}
	folded := map[string]bool{
		"01-a": true,
		"02-b": false, // false entries are never persisted
		"gone": true,  // orphan — not in known — must be pruned
	}
	if err := saveFolded(dir, folded, known); err != nil {
		t.Fatal(err)
	}
	got := loadFolded(dir)
	if !got["01-a"] {
		t.Fatalf("expected 01-a folded=true, got %+v", got)
	}
	if got["02-b"] {
		t.Fatalf("02-b was false, must not be persisted as true, got %+v", got)
	}
	if _, ok := got["gone"]; ok {
		t.Fatal("orphan key (not in known) must be pruned on save")
	}
}

func TestFoldedEmptyResultRemovesFile(t *testing.T) {
	dir := t.TempDir()
	known := map[string]bool{"01-a": true}
	// Seed a sidecar, then save an empty result and confirm the file disappears.
	if err := saveFolded(dir, map[string]bool{"01-a": true}, known); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(foldedPath(dir)); err != nil {
		t.Fatalf("sidecar should exist after first save: %v", err)
	}
	if err := saveFolded(dir, map[string]bool{}, known); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(foldedPath(dir)); !os.IsNotExist(err) {
		t.Fatalf("sidecar should be removed once empty, stat err = %v", err)
	}
	if len(loadFolded(dir)) != 0 {
		t.Fatal("loadFolded after removal must be empty")
	}
}

func TestFoldedTolerantLoad(t *testing.T) {
	dir := t.TempDir()
	if len(loadFolded(dir)) != 0 {
		t.Fatal("missing sidecar → empty map")
	}
	if err := os.WriteFile(foldedPath(dir), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if len(loadFolded(dir)) != 0 {
		t.Fatal("corrupt sidecar → empty map, no error")
	}
	if err := os.WriteFile(foldedPath(dir), []byte(`{"schemaVersion":99,"folded":{"a":true}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if len(loadFolded(dir)) != 0 {
		t.Fatal("unsupported schemaVersion → empty map")
	}
}

func TestFoldedNoLeftoverTempFile(t *testing.T) {
	dir := t.TempDir()
	if err := saveFolded(dir, map[string]bool{"01-a": true}, map[string]bool{"01-a": true}); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 || entries[0].Name() != foldedName {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("expected only %s, got %v", foldedName, names)
	}
}

func TestFoldChapterSet(t *testing.T) {
	dir := t.TempDir()
	mkChapterDir(t, dir, "opening", map[string]string{"opening.md": "hello"})
	mkChapterDir(t, dir, "closing", map[string]string{"closing.md": "bye"})
	writeManifestRaw(t, dir, `{"schemaVersion":3,"title":"N","items":[
		{"chapter":{"folder":"opening","title":"Opening","texts":[{"file":"opening.md","title":"Opening"},{"file":"opening2.md","title":"Part Two"}]}},
		{"chapter":{"folder":"closing","title":"Closing","texts":[{"file":"closing.md","title":"Closing"}]}}
	]}`)
	// opening2.md doesn't exist on disk — resolveManuscript will still resolve the chapter
	// (opening.md exists), just with one text instead of two; foldChapterSet only cares about
	// which chapters resolve at all.
	got := foldChapterSet(dir)
	if !got["opening"] || !got["closing"] {
		t.Fatalf("expected both chapter keys present, got %+v", got)
	}
	if len(got) != 2 {
		t.Fatalf("expected exactly 2 keys, got %+v", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail (undefined symbols)**

Run: `go test ./... -run 'TestFolded|TestFoldChapterSet' -v`
Expected: FAIL — `undefined: saveFolded`, `undefined: loadFolded`, `undefined: foldedPath`,
`undefined: foldedName`, `undefined: foldChapterSet`.

- [ ] **Step 3: Write `fold.go`**

```go
package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
)

const foldedName = ".okashi-folded.json"
const foldedSchemaVersion = 1

// foldedFile is the per-manuscript sidebar fold-state sidecar (okashi-owned, NOT the
// manifest — the shared contract HARD GATE stays untriggered). Keyed by corkKey(chapterRef)
// (corkboard.go) → true when the chapter's scenes are expanded inline in the sidebar. A
// chapter with no key is collapsed (the default) — only expanded chapters are persisted, so
// an untouched manuscript never grows this sidecar.
type foldedFile struct {
	SchemaVersion int             `json:"schemaVersion"`
	Folded        map[string]bool `json:"folded"`
}

func foldedPath(dir string) string { return filepath.Join(dir, foldedName) }

// loadFolded reads the sidecar; missing/corrupt/unsupported-schema all yield an empty map —
// never an error (mirrors loadSynopses's tolerant load).
func loadFolded(dir string) map[string]bool {
	out := map[string]bool{}
	data, err := os.ReadFile(foldedPath(dir))
	if err != nil {
		return out
	}
	var ff foldedFile
	if json.Unmarshal(data, &ff) != nil || ff.SchemaVersion != foldedSchemaVersion {
		return out
	}
	if ff.Folded != nil {
		out = ff.Folded
	}
	return out
}

// saveFolded writes the sidecar atomically. It prunes keys not in known (self-healing orphans
// left by a removed/renamed-away chapter) and drops false entries, so the file only ever holds
// live, expanded chapters. An empty result removes the sidecar file entirely rather than
// writing an empty object, mirroring saveNotes's empty-list removal. Serialized like
// writeManifest/saveSynopses: 2-space indent, no HTML escaping, no trailing newline.
func saveFolded(dir string, folded map[string]bool, known map[string]bool) error {
	pruned := map[string]bool{}
	for key, v := range folded {
		if known[key] && v {
			pruned[key] = true
		}
	}
	if len(pruned) == 0 {
		err := os.Remove(foldedPath(dir))
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(foldedFile{SchemaVersion: foldedSchemaVersion, Folded: pruned}); err != nil {
		return err
	}
	return atomicWrite(foldedPath(dir), bytes.TrimRight(buf.Bytes(), "\n"), 0o644)
}

// foldChapterSet is the ON-DISK chapter/scene key set for dir — the safe prune target for a
// fold-state write, mirroring corkChapterSet (corkboard.go). Keyed by corkKey, across all
// parts, so a folded chapter inside a real Part is never wrongly pruned.
func foldChapterSet(dir string) map[string]bool {
	s := map[string]bool{}
	v := resolveManuscript(dir, readEntries(dir))
	for _, p := range v.parts {
		for _, ch := range p.chapters {
			s[corkKey(ch)] = true
		}
	}
	return s
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./... -run 'TestFolded|TestFoldChapterSet' -v`
Expected: PASS (all 5 tests).

- [ ] **Step 5: Commit**

```bash
git add fold.go fold_test.go
git commit -m "feat: .okashi-folded.json sidecar for sidebar chapter fold state"
```

---

## Task 2: `fileEntry` gains `isChildScene` — insertion in `filelist.SetDir`

**Files:**
- Modify: `filelist.go:24-29` (`fileEntry` struct), `filelist.go:32-44` (`filelist` struct),
  `filelist.go:86-161` (`SetDir`)
- Test: `filelist_test.go`

**Interfaces:**
- Consumes: `loadFolded(dir string) map[string]bool`, `foldChapterSet(dir string) map[string]bool`
  (Task 1); `corkKey(ch chapterRef) string` (`corkboard.go:30`, existing); `manuscriptView.ordered()
  bool` (`manuscript.go:56`, existing).
- Produces: `fileEntry.isChildScene bool`, `fileEntry.parentFolder string`; `filelist.folded
  map[string]bool`, `filelist.foldedRoot string` — used by Task 3 (`activate`/render).

- [ ] **Step 1: Write the failing tests**

```go
// filelist_test.go — append

func TestSetDirCollapsedChapterHidesChildScenes(t *testing.T) {
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
	f := newFilelist()
	f.SetDir(dir)
	for _, e := range f.entries {
		if e.isChildScene {
			t.Fatalf("collapsed chapter must not have child-scene rows, got entries=%+v", f.entries)
		}
	}
}

func TestSetDirExpandedChapterShowsChildScenesInOrder(t *testing.T) {
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

	var got []fileEntry
	for _, e := range f.entries {
		if e.isChildScene {
			got = append(got, e)
		}
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 child-scene rows, got %+v", got)
	}
	if got[0].name != "opening.md" || got[0].parentFolder != "opening" {
		t.Fatalf("first child scene wrong: %+v", got[0])
	}
	if got[1].name != "opening2.md" || got[1].parentFolder != "opening" {
		t.Fatalf("second child scene wrong: %+v", got[1])
	}
	// Child rows must be positioned immediately after their chapter's own row.
	chIdx, child1Idx := -1, -1
	for i, e := range f.entries {
		if e.name == "opening" && e.isDir {
			chIdx = i
		}
		if e.name == "opening.md" && e.isChildScene {
			child1Idx = i
		}
	}
	if chIdx == -1 || child1Idx != chIdx+1 {
		t.Fatalf("child scene must immediately follow its chapter row: chIdx=%d child1Idx=%d", chIdx, child1Idx)
	}
}

func TestSetDirSingleTextChapterNeverExpandsEvenIfFolded(t *testing.T) {
	dir := t.TempDir()
	mkChapterDir(t, dir, "opening", map[string]string{"opening.md": "hello"})
	writeManifestRaw(t, dir, `{"schemaVersion":3,"title":"N","items":[
		{"chapter":{"folder":"opening","title":"Opening","texts":[{"file":"opening.md","title":"Opening"}]}}
	]}`)
	// Force a folded=true entry for a single-text chapter — SetDir must ignore it.
	if err := saveFolded(dir, map[string]bool{"opening": true}, map[string]bool{"opening": true}); err != nil {
		t.Fatal(err)
	}
	f := newFilelist()
	f.SetDir(dir)
	for _, e := range f.entries {
		if e.isChildScene {
			t.Fatalf("single-text chapter must never show child-scene rows, got entries=%+v", f.entries)
		}
	}
}

func TestSetDirLoadsFoldedOnceForSameManuscriptRoot(t *testing.T) {
	dir := t.TempDir()
	mkChapterDir(t, dir, "opening", map[string]string{"opening.md": "a", "opening2.md": "b"})
	writeManifestRaw(t, dir, `{"schemaVersion":3,"title":"N","items":[
		{"chapter":{"folder":"opening","title":"Opening","texts":[
			{"file":"opening.md","title":"P1"},{"file":"opening2.md","title":"P2"}
		]}}
	]}`)
	f := newFilelist()
	f.SetDir(dir)
	if f.foldedRoot != dir {
		t.Fatalf("foldedRoot = %q, want %q", f.foldedRoot, dir)
	}
	// Mutate the sidecar on disk directly, then call SetDir again on the SAME dir — the
	// in-memory f.folded must NOT pick up the change (loaded once per manuscript root, by
	// design — see spec's "Chargement du sidecar" section).
	if err := saveFolded(dir, map[string]bool{"opening": true}, map[string]bool{"opening": true}); err != nil {
		t.Fatal(err)
	}
	f.SetDir(dir)
	for _, e := range f.entries {
		if e.isChildScene {
			t.Fatal("second SetDir on the same manuscript root must not reload the sidecar")
		}
	}
}

func TestSetDirNonManuscriptFolderNeverLoadsFolded(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "note.md"), "x") // plain category, no manifest
	f := newFilelist()
	f.SetDir(dir)
	if f.foldedRoot != "" {
		t.Fatalf("foldedRoot should stay empty for a non-manuscript folder, got %q", f.foldedRoot)
	}
	if len(f.folded) != 0 {
		t.Fatalf("folded should stay empty for a non-manuscript folder, got %+v", f.folded)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./... -run TestSetDir -v`
Expected: FAIL — compile error (`isChildScene`/`parentFolder`/`foldedRoot`/`f.folded`
undefined on the current structs), or logic failures once it compiles minimally.

- [ ] **Step 3: Modify `fileEntry` and `filelist` structs, and `SetDir`**

In `filelist.go`, extend `fileEntry` (replaces lines 24-29):

```go
type fileEntry struct {
	name         string
	isDir        bool
	isPartHeader bool // a non-selectable Part title row; cursor movement skips it
	isScene      bool // a standalone scene (manifest v3, chapterRef.scene) — distinct glyph
	isChildScene bool // a scene row nested under an expanded multi-text chapter (fold state)
	parentFolder string // isChildScene only: the owning chapter's folder, for activate()
}
```

Extend `filelist` (replaces lines 32-44):

```go
type filelist struct {
	dir        string
	root       string
	entries    []fileEntry
	view       manuscriptView // resolved structure of dir (chapters, loose, source)
	selected   int
	offset     int // index of the top visible row
	width      int
	height     int
	allowed    map[string]bool
	folded     map[string]bool // corkKey(chapterRef) → true if EXPANDED (absence = collapsed)
	foldedRoot string          // manuscript dir folded was loaded for; "" when not a manuscript
	icons      iconSet
	wc         *wordCountCache
}
```

In `SetDir`, right after `f.view = resolveManuscript(dir, files)` (existing line 125), insert
the fold-state load:

```go
	f.view = resolveManuscript(dir, files)

	if f.view.ordered() {
		if dir != f.foldedRoot {
			f.folded = loadFolded(dir)
			f.foldedRoot = dir
		}
	} else {
		f.folded = nil
		f.foldedRoot = ""
	}
```

Then, in the chapter-emitting loop (existing lines 150-158), replace:

```go
		for _, ch := range p.chapters {
			if ch.folder == "" {
				if len(ch.texts) > 0 {
					f.entries = append(f.entries, fileEntry{name: ch.texts[0].file, isScene: ch.scene})
				}
				continue
			}
			f.entries = append(f.entries, fileEntry{name: ch.folder, isDir: true})
		}
```

with:

```go
		for _, ch := range p.chapters {
			if ch.folder == "" {
				if len(ch.texts) > 0 {
					f.entries = append(f.entries, fileEntry{name: ch.texts[0].file, isScene: ch.scene})
				}
				continue
			}
			f.entries = append(f.entries, fileEntry{name: ch.folder, isDir: true})
			if len(ch.texts) >= 2 && f.folded[corkKey(ch)] {
				for _, t := range ch.texts {
					f.entries = append(f.entries, fileEntry{
						name:         t.file,
						isChildScene: true,
						parentFolder: ch.folder,
					})
				}
			}
		}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./... -run 'TestSetDir|TestFilelist' -v`
Expected: PASS. Also run the full existing filelist suite to confirm no regression:
`go test ./... -run TestFilelist -v` and `go test ./... -run TestSidebar -v` and
`go test ./... -run TestActivate -v` — all previously-passing tests must still pass (the
`activate()` behavior itself is unchanged in this task; Task 3 changes it).

- [ ] **Step 5: Commit**

```bash
git add filelist.go filelist_test.go
git commit -m "feat: SetDir inserts child-scene rows under an expanded multi-text chapter"
```

---

## Task 3: `activate()` toggles fold instead of opening the text-picker; child-scene rows open their file

**Files:**
- Modify: `filelist.go:436-460` (`activate`)
- Test: `filelist_test.go`

**Interfaces:**
- Consumes: `f.folded`, `f.foldedRoot`, `corkKey` (Task 1/2); `saveFolded`, `foldChapterSet`
  (Task 1).
- Produces: `activate()`'s new behavior — a 2+-text chapter never returns `activateTextPicker`
  anymore; it returns `activateNone` after toggling `f.folded` and rebuilding `f.entries`. A
  `isChildScene` entry returns `activateFile` on `filepath.Join(f.dir, e.parentFolder, e.name)`.
  This is the interface Task 4 (`main.go`) relies on to remove the sidebar's
  `enterTextPicker()` call sites.

- [ ] **Step 1: Write the failing tests**

```go
// filelist_test.go — append

func TestActivateMultiTextChapterTogglesFoldInsteadOfPicker(t *testing.T) {
	dir := t.TempDir()
	mkChapterDir(t, dir, "opening", map[string]string{"opening.md": "a", "opening2.md": "b"})
	writeManifestRaw(t, dir, `{"schemaVersion":3,"title":"N","items":[
		{"chapter":{"folder":"opening","title":"Opening","texts":[
			{"file":"opening.md","title":"P1"},{"file":"opening2.md","title":"P2"}
		]}}
	]}`)
	f := newFilelist()
	f.SetDir(dir)
	f.selectName("opening") // chapter row, collapsed

	path, result := f.activate()
	if result != activateNone || path != "" {
		t.Fatalf("expanding a chapter must return activateNone/\"\", got %q/%v", path, result)
	}
	if !f.folded["opening"] {
		t.Fatal("activate on a collapsed multi-text chapter must expand it")
	}
	// f.entries must have been rebuilt with the child rows now visible.
	found := false
	for _, e := range f.entries {
		if e.isChildScene && e.name == "opening.md" {
			found = true
		}
	}
	if !found {
		t.Fatal("after expanding, child-scene rows must be present in f.entries")
	}

	// Activating again on the (still selected) chapter row collapses it back.
	f.selectName("opening")
	path, result = f.activate()
	if result != activateNone || path != "" {
		t.Fatalf("collapsing a chapter must return activateNone/\"\", got %q/%v", path, result)
	}
	if f.folded["opening"] {
		t.Fatal("activate on an expanded multi-text chapter must collapse it")
	}
}

func TestActivateFoldTogglePersistsToSidecar(t *testing.T) {
	dir := t.TempDir()
	mkChapterDir(t, dir, "opening", map[string]string{"opening.md": "a", "opening2.md": "b"})
	writeManifestRaw(t, dir, `{"schemaVersion":3,"title":"N","items":[
		{"chapter":{"folder":"opening","title":"Opening","texts":[
			{"file":"opening.md","title":"P1"},{"file":"opening2.md","title":"P2"}
		]}}
	]}`)
	f := newFilelist()
	f.SetDir(dir)
	f.selectName("opening")
	f.activate() // expand

	// Fresh filelist re-reading the same manuscript root must see the persisted state.
	f2 := newFilelist()
	f2.SetDir(dir)
	found := false
	for _, e := range f2.entries {
		if e.isChildScene {
			found = true
		}
	}
	if !found {
		t.Fatal("fold toggle must be persisted to .okashi-folded.json, not just in-memory")
	}
}

func TestActivateChildSceneOpensItsFile(t *testing.T) {
	dir := t.TempDir()
	mkChapterDir(t, dir, "opening", map[string]string{"opening.md": "a", "opening2.md": "b"})
	writeManifestRaw(t, dir, `{"schemaVersion":3,"title":"N","items":[
		{"chapter":{"folder":"opening","title":"Opening","texts":[
			{"file":"opening.md","title":"P1"},{"file":"opening2.md","title":"P2"}
		]}}
	]}`)
	if err := saveFolded(dir, map[string]bool{"opening": true}, map[string]bool{"opening": true}); err != nil {
		t.Fatal(err)
	}
	f := newFilelist()
	f.SetDir(dir)
	f.selectName("opening2.md") // second child-scene row

	path, result := f.activate()
	if result != activateFile {
		t.Fatalf("activating a child-scene row must return activateFile, got %v", result)
	}
	want := filepath.Join(dir, "opening", "opening2.md")
	if path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
}

func TestActivateSingleTextChapterUnchanged(t *testing.T) {
	// Regression guard: single-text chapters must keep opening directly, never toggle a fold.
	dir := t.TempDir()
	mkChapterDir(t, dir, "opening", map[string]string{"opening.md": "hello"})
	writeManifestRaw(t, dir, `{"schemaVersion":3,"title":"N","items":[
		{"chapter":{"folder":"opening","title":"Opening","texts":[{"file":"opening.md","title":"Opening"}]}}
	]}`)
	f := newFilelist()
	f.SetDir(dir)
	f.selectName("opening")
	path, result := f.activate()
	if result != activateFile || path != filepath.Join(dir, "opening", "opening.md") {
		t.Fatalf("single-text chapter activate: path=%q result=%v", path, result)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./... -run TestActivate -v`
Expected: FAIL — `TestActivateMultiTextChapterTogglesFoldInsteadOfPicker` and siblings fail
because `activate()` still returns `activateTextPicker` for a 2+-text chapter (current
behavior, `filelist.go:444-450`).

- [ ] **Step 3: Modify `activate()`**

Replace the `isDir` branch of `activate()` (`filelist.go:444-457`):

```go
	if e.isDir {
		if isChapterOf(f.view, e.name) {
			ch := chapterByFolder(f.view, e.name)
			if len(ch.texts) == 1 {
				return filepath.Join(f.dir, ch.folder, ch.texts[0].file), activateFile
			}
			if len(ch.texts) == 0 {
				return "", activateTextPicker
			}
			// 2+ texts: toggle this chapter's fold state instead of opening a picker.
			key := corkKey(ch)
			if f.folded[key] {
				delete(f.folded, key)
			} else {
				if f.folded == nil {
					f.folded = map[string]bool{}
				}
				f.folded[key] = true
			}
			if err := saveFolded(f.foldedRoot, f.folded, foldChapterSet(f.foldedRoot)); err == nil {
				f.SetDir(f.dir) // rebuild f.entries with/without the child rows; also reloads
				// f.folded from what was just saved — harmless (same content) and keeps SetDir
				// as the single source of truth for entry construction.
			}
			return "", activateNone
		}
		if e.name == ".." {
			f.SetDir(filepath.Dir(f.dir))
		} else {
			f.SetDir(filepath.Join(f.dir, e.name))
		}
		return "", activateNone
	}
	if e.isChildScene {
		return filepath.Join(f.dir, e.parentFolder, e.name), activateFile
	}
	return filepath.Join(f.dir, e.name), activateFile
```

Note: this replaces the whole `isDir` block AND the final `return` of `activate()` (previously
just `return filepath.Join(f.dir, e.name), activateFile`) — the new `isChildScene` check must
come before that fallback since a child-scene row has `isDir == false`.

Also update `activate`'s doc comment (immediately above the function, `filelist.go:429-435`)
to mention the new toggle behavior:

```go
// activate acts on the selected entry. A "..", a plain directory, or a Part-header
// row (guarded above by moveBy/selectRow, but defensively checked here too) navigates
// and returns activateNone. A v2 chapter folder with exactly one text returns its
// path with activateFile — unchanged behavior from before multi-text chapters
// existed. A v2 chapter folder with zero texts returns activateTextPicker (caller opens
// the picker to create the first scene). A v2 chapter folder with 2+ texts TOGGLES its
// sidebar fold state (persisted via saveFolded) and returns activateNone — the caller
// does nothing further; f.entries already reflects the new state. A child-scene row
// (isChildScene, only present when its chapter is expanded) returns its own file path
// with activateFile. Anything else (an ordinary file, or a legacy single-file chapter)
// returns its path with activateFile.
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./... -run TestActivate -v`
Expected: PASS (all, including pre-existing `TestActivatePlainFolderStillNavigates` and
`TestActivateEmptyChapterSignalsPickerWithNoTexts` — both untouched cases).

Run full package test suite to catch any regression: `go test ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add filelist.go filelist_test.go
git commit -m "feat: activate() toggles chapter fold state instead of opening the text picker"
```

---

## Task 4: Render — fold indicator glyph + indented child-scene rows

**Files:**
- Modify: `filelist.go:170-224` (`View`), `filelist.go:252-271` (`sectionRow`)
- Test: `filelist_test.go`

**Interfaces:**
- Consumes: `fileEntry.isChildScene`, `fileEntry.parentFolder` (Task 2); `f.folded`,
  `corkKey` (Task 1/2).
- Produces: no new exported behavior — this task only changes rendered output.

- [ ] **Step 1: Write the failing tests**

```go
// filelist_test.go — append

func TestSectionRowShowsCollapsedTriangleForMultiTextChapter(t *testing.T) {
	dir := t.TempDir()
	mkChapterDir(t, dir, "opening", map[string]string{"opening.md": "a", "opening2.md": "b"})
	writeManifestRaw(t, dir, `{"schemaVersion":3,"title":"N","items":[
		{"chapter":{"folder":"opening","title":"Opening","texts":[
			{"file":"opening.md","title":"P1"},{"file":"opening2.md","title":"P2"}
		]}}
	]}`)
	f := newFilelist()
	f.width = 40
	f.height = 10
	f.SetDir(dir)
	out := f.View(-1, "")
	if !strings.Contains(out, "▸") {
		t.Fatalf("collapsed multi-text chapter row must show a ▸ triangle, got:\n%s", out)
	}
	if strings.Contains(out, "▾") {
		t.Fatalf("collapsed chapter must not show the expanded ▾ triangle, got:\n%s", out)
	}
}

func TestSectionRowShowsExpandedTriangleAndIndentedChildren(t *testing.T) {
	dir := t.TempDir()
	mkChapterDir(t, dir, "opening", map[string]string{"opening.md": "a", "opening2.md": "b"})
	writeManifestRaw(t, dir, `{"schemaVersion":3,"title":"N","items":[
		{"chapter":{"folder":"opening","title":"Opening","texts":[
			{"file":"opening.md","title":"Part One"},{"file":"opening2.md","title":"Part Two"}
		]}}
	]}`)
	if err := saveFolded(dir, map[string]bool{"opening": true}, map[string]bool{"opening": true}); err != nil {
		t.Fatal(err)
	}
	f := newFilelist()
	f.width = 40
	f.height = 10
	f.SetDir(dir)
	out := f.View(-1, "")
	if !strings.Contains(out, "▾") {
		t.Fatalf("expanded multi-text chapter row must show a ▾ triangle, got:\n%s", out)
	}
	if !strings.Contains(out, "Part One") || !strings.Contains(out, "Part Two") {
		t.Fatalf("expanded chapter must render its scene titles, got:\n%s", out)
	}
}

func TestSectionRowNoTriangleForSingleTextChapter(t *testing.T) {
	dir := t.TempDir()
	mkChapterDir(t, dir, "opening", map[string]string{"opening.md": "hello"})
	writeManifestRaw(t, dir, `{"schemaVersion":3,"title":"N","items":[
		{"chapter":{"folder":"opening","title":"Opening","texts":[{"file":"opening.md","title":"Opening"}]}}
	]}`)
	f := newFilelist()
	f.width = 40
	f.height = 10
	f.SetDir(dir)
	out := f.View(-1, "")
	if strings.Contains(out, "▸") || strings.Contains(out, "▾") {
		t.Fatalf("single-text chapter must never show a fold triangle, got:\n%s", out)
	}
}

func TestChildSceneRowFallsBackToFilenameWhenTitleEmpty(t *testing.T) {
	dir := t.TempDir()
	mkChapterDir(t, dir, "opening", map[string]string{"opening.md": "a", "opening2.md": "b"})
	writeManifestRaw(t, dir, `{"schemaVersion":3,"title":"N","items":[
		{"chapter":{"folder":"opening","title":"Opening","texts":[
			{"file":"opening.md","title":""},{"file":"opening2.md","title":"Part Two"}
		]}}
	]}`)
	if err := saveFolded(dir, map[string]bool{"opening": true}, map[string]bool{"opening": true}); err != nil {
		t.Fatal(err)
	}
	f := newFilelist()
	f.width = 40
	f.height = 10
	f.SetDir(dir)
	out := f.View(-1, "")
	// sectionTitle("opening.md") de-slugs to "Opening" — the fallback must not render an
	// empty label for the first child row.
	if !strings.Contains(out, sectionTitle("opening.md")) {
		t.Fatalf("empty scene title must fall back to a de-slugged filename, got:\n%s", out)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./... -run 'TestSectionRow|TestChildSceneRow' -v`
Expected: FAIL — no triangle rendered yet (current `sectionRow` has no fold indicator), and
`isChildScene` rows currently fall into `View`'s `default` (plain file) branch, rendering the
raw filename instead of a resolved/indented title.

- [ ] **Step 3: Modify `sectionRow` and `View`**

`sectionRow` gains the fold indicator. Replace `sectionRow` (`filelist.go:252-271`):

```go
// sectionRow builds a width-f.width row for a manuscript section: gutter+fold-indicator+icon+
// title on the left, the word count right-aligned. dimCount styles the count subtle (used for
// non-selected rows; the selected bar keeps it plain). fold is "" for a chapter with 0/1 text
// (no indicator), "▸ " collapsed, "▾ " expanded — composed as a literal prefix rather than
// added to iconSet, since it signals fold STATE, not entry TYPE (iconFor's byExt/isDir/isScene
// switch stays type-only).
func (f filelist) sectionRow(e fileEntry, dimCount bool) string {
	n := f.chapterWords(e.name)
	count := commafy(n) + " m"
	g := f.icons.iconFor(e)
	fold := f.foldIndicator(e.name)
	left := " " + fold + renderIcon(g, !dimCount) + f.chapterTitle(e.name)
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

// foldIndicator returns "▸ " (collapsed) or "▾ " (expanded) for a multi-text chapter folder
// name, "" for anything else (0/1-text chapter, standalone scene, legacy flat chapter) — those
// never show a triangle since there is nothing to expand.
func (f filelist) foldIndicator(folderName string) string {
	for _, p := range f.view.parts {
		for _, ch := range p.chapters {
			if ch.folder != folderName || len(ch.texts) < 2 {
				continue
			}
			if f.folded[corkKey(ch)] {
				return "▾ "
			}
			return "▸ "
		}
	}
	return ""
}
```

In `View` (`filelist.go:186-222`), the loop currently has a `section := f.isChapterEntry(e)`
guard and a `default` branch for plain files. Add a dedicated branch for `isChildScene` right
before the `default` case (insert after the `case e.isDir:` branch, `filelist.go:206-208`):

```go
		case e.isChildScene:
			title := f.childSceneTitle(e)
			row := "   " + renderIcon(f.icons.iconFor(e), false) + title
			b.WriteString(ansi.Truncate(row, f.width, "…"))
```

Note: this new `case` must be placed so it's checked before the generic `default:` file-render
branch, and the `i == f.selected` branch above it already handles the selected-row styling
generically (`content = " " + renderIcon(g, true) + e.name` for a non-section entry) — a
selected child-scene row will render with `e.name` (the filename) rather than its resolved
title under the existing selected-row branch. Fix that too: replace the `case i == f.selected:`
body (`filelist.go:196-203`) to special-case `isChildScene`:

```go
		case i == f.selected:
			var content string
			switch {
			case section:
				content = f.sectionRow(e, false) // selected: count + icon plain
			case e.isChildScene:
				content = "   " + renderIcon(g, true) + f.childSceneTitle(e)
			default:
				content = " " + renderIcon(g, true) + e.name
			}
			b.WriteString(selectedStyle.Width(f.width).Render(ansi.Truncate(content, f.width, "…")))
```

Add `childSceneTitle`, placed near `chapterTitle` (`filelist.go:296-314`):

```go
// childSceneTitle resolves an isChildScene entry's display title: the manifest-provided
// textRef.title when non-empty, else a de-slugged fallback from its filename (textRef.title
// has no built-in fallback — unlike chapterRef/manuscriptView's title, which always resolve to
// something non-empty — see manuscript.go's resolveChapterTexts, which passes t.Title through
// verbatim from the JSON).
func (f filelist) childSceneTitle(e fileEntry) string {
	for _, p := range f.view.parts {
		for _, ch := range p.chapters {
			if ch.folder != e.parentFolder {
				continue
			}
			for _, t := range ch.texts {
				if t.file == e.name && t.title != "" {
					return t.title
				}
			}
		}
	}
	return sectionTitle(e.name)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./... -run 'TestSectionRow|TestChildSceneRow' -v`
Expected: PASS.

Run the full existing filelist/sidebar suite for regressions:
`go test ./... -run 'TestFilelist|TestSidebar|TestBreadcrumb|TestPane|TestMoveBy|TestSelectRow|TestActivate|TestIsChapterEntry|TestSetDir' -v`
Expected: PASS, no regression.

- [ ] **Step 5: Commit**

```bash
git add filelist.go filelist_test.go
git commit -m "feat: render fold triangle and indented child-scene rows in the sidebar"
```

---

## Task 5: `main.go` — retire the sidebar's two `enterTextPicker()` call sites

**Files:**
- Modify: `main.go:1650-1658` (double-click handler), `main.go:1866-1873` (keyboard
  `enter/right/l` handler)
- Test: `main_test.go` (if a test already exercises this path — see Step 1) or a new
  targeted test in `filelist_test.go`-adjacent style within `main_test.go`.

**Interfaces:**
- Consumes: `filelist.activate()`'s new contract from Task 3 — a 2+-text chapter now returns
  `activateNone` (fold already toggled internally), never `activateTextPicker`.
- Produces: nothing new — this task only removes now-unreachable branches. `enterTextPicker()`
  itself is NOT deleted (still used by the corkboard's `enter` on a 2+-text card,
  `corkboard.go:365-374`, and by `ctrl+n`'s in-picker scene creation, `main.go:206-245`'s
  `updateTextPicker`). Confirm via `grep -n "enterTextPicker" *.go` before editing that only
  these two sidebar call sites reference it outside its own definition and
  `updateTextPicker`/corkboard.

- [ ] **Step 1: Confirm current call sites and write the regression test**

Run: `grep -n "enterTextPicker\b" /home/Baudouin/Documents/Projets/forkashi/*.go`
Expected output should show exactly: the function definition (`main.go:187`), the two sidebar
call sites being removed (`main.go` double-click block and keyboard block), and the corkboard's
own inline construction of the same screen state (`corkboard.go:369`, which does NOT call
`enterTextPicker()` — it sets the fields directly, per the design spec's note that corkboard
and sidebar are disjoint paths). If corkboard's `enter` handler is found to call
`m.enterTextPicker()` directly instead of inlining the fields, STOP and re ‑confirm against
corkboard.go before proceeding — the removal below assumes it does not.

Add a regression test confirming the sidebar keyboard path no longer opens the picker screen
for a multi-text chapter:

```go
// main_test.go — append (adjust setup helpers to match existing main_test.go conventions;
// if main_test.go already has a helper that constructs a full *model with a sidebar pointed
// at a temp manuscript dir, reuse it instead of duplicating construction here).

func TestSidebarEnterOnMultiTextChapterNeverOpensTextPicker(t *testing.T) {
	dir := t.TempDir()
	mkChapterDir(t, dir, "opening", map[string]string{"opening.md": "a", "opening2.md": "b"})
	writeManifestRaw(t, dir, `{"schemaVersion":3,"title":"N","items":[
		{"chapter":{"folder":"opening","title":"Opening","texts":[
			{"file":"opening.md","title":"P1"},{"file":"opening2.md","title":"P2"}
		]}}
	]}`)
	m := initialModel()
	m.files.root = dir
	m.files.SetDir(dir)
	m.screen = screenWriting
	m.focus = focusSidebar
	m.width, m.height = 80, 24
	m.files.selectName("opening")

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := updated.(model)
	if m2.screen == screenTextPicker {
		t.Fatal("enter on a multi-text chapter must no longer open screenTextPicker from the sidebar")
	}
	if m2.screen != screenWriting {
		t.Fatalf("screen should stay screenWriting (fold toggled in place), got %v", m2.screen)
	}
}
```

Note: verify `initialModel()`'s exact signature and any required setup (e.g. icons/wc cache
initialization) against an existing `main_test.go` test before finalizing this step — reuse
whatever construction helper the existing test suite already has for a full `model`, rather
than hand-rolling one that might skip required initialization (`f.icons`, `f.wc`, etc., are
set by `newFilelist()` — confirm `initialModel()` goes through it).

- [ ] **Step 2: Run test to verify it passes**

Run: `go test ./... -run TestSidebarEnterOnMultiTextChapterNeverOpensTextPicker -v`
Expected: PASS already at this point — Task 3's change to `filelist.activate()` is sufficient
on its own to satisfy this test; no `main.go` behavior change is needed. This test exists as
an explicit regression guard at the `Update` level (confirming the wiring end-to-end), not
just at the `filelist.activate()` unit level covered by Task 3's own tests. If it fails here,
STOP — it means Task 3 did not land as specified; do not proceed to Step 3 until it passes.

- [ ] **Step 3: No source edit — add a clarifying comment at both call sites**

Task 3 kept `activateTextPicker` alive for the 0-text case (an empty chapter still needs the
picker to offer scene creation via `ctrl+n`, per the spec's "0 texte : inchangé" clause). Only
the 2+-text path stopped returning it — `filelist.activate()`'s contract change already makes
both call sites correct with zero edits, since they dispatch generically on `result`. This
step only adds a one-line comment at each site documenting the narrowed contract, so a future
reader doesn't have to re-derive it from `filelist.go`.

In `main.go`, the double-click handler (existing lines 1650-1658) and the keyboard handler
(existing lines 1866-1873) both currently read:

```go
			if path, result := m.files.activate(); result == activateFile {
				m.loadFile(path)
				m.focus = focusEditor
				m.editor.Focus()
			} else if result == activateTextPicker {
				m.enterTextPicker()
			}
```

At both sites, change only the trailing comment:

```go
			if path, result := m.files.activate(); result == activateFile {
				m.loadFile(path)
				m.focus = focusEditor
				m.editor.Focus()
			} else if result == activateTextPicker {
				m.enterTextPicker() // only reachable for a 0-text chapter — a 2+-text chapter
				// now toggles its own fold state inside activate() and returns activateNone
			}
```

- [ ] **Step 4: Run full test suite**

Run: `go test ./...`
Expected: PASS, no regression anywhere (mover, export-selection, corkboard, outline, etc. are
all untouched by this plan).

- [ ] **Step 5: Commit**

```bash
git add main_test.go
git commit -m "test: confirm sidebar enter on a multi-text chapter never opens the text picker"
```

---

## Task 6: `icons_test.go` guard — confirm no new `iconSet` field was introduced

**Files:**
- Verify only: `icons_test.go` (`TestNerdIconsAreRealGlyphs`)

**Interfaces:**
- Consumes: nothing new.
- Produces: nothing — this is a verification task confirming Task 4's design choice (fold
  triangles composed as literal `"▸ "`/`"▾ "` strings in `sectionRow`, not added to `iconSet`)
  didn't silently need a glyph-set change.

- [ ] **Step 1: Run the existing nerd-glyph guard test**

Run: `go test ./... -run TestNerdIconsAreRealGlyphs -v`
Expected: PASS unchanged — Task 4 deliberately did not touch `iconSet`/`glyph`/`resolveIcons`,
so this test's coverage list (`s.folder, s.parent, s.file, s.action, s.scene` + `byExt`) needs
no update. If a future iteration adds a Nerd Font-specific fold glyph (nerd mode currently
reuses the same plain "▸ "/"▾ " ASCII triangles regardless of `OKASHI_ICONS`, per this plan —
acceptable since a triangle renders fine as ASCII in every terminal, unlike file-type PUA
glyphs), that field would need adding to this test's `all` slice — out of scope here.

- [ ] **Step 2: Commit**

No code changes in this task — skip commit if `go test` step 1 passes as expected (nothing
staged). If it unexpectedly fails, STOP: it means an assumption in Task 4 was wrong and needs
re-examination before continuing.

---

## Task 7: Update CLAUDE.md per the spec's "Impact CLAUDE.md" section

**Files:**
- Modify: `CLAUDE.md` (§ Project model, the "corkboard" bullet describing the sidebar/binder)

**Interfaces:**
- Consumes: nothing.
- Produces: nothing — documentation only.

- [ ] **Step 1: Locate the relevant bullet**

Run: `grep -n "manuscript-aware sidebar\|per-chapter word" /home/Baudouin/Documents/Projets/forkashi/CLAUDE.md`

Find the sentence describing the manuscript-aware sidebar (§ Project model, "Shipped
features" bullet list) — add a clause after it.

- [ ] **Step 2: Edit CLAUDE.md**

Add, immediately after the existing sidebar description (exact insertion point determined by
Step 1's grep result — insert as a new clause in the same bullet, following the established
style of parenthetical additions elsewhere in that bullet list):

```
a multi-scene chapter's scenes show inline, indented, under a pliable/foldable chapter row
(`⏎` toggles; state persists per-chapter in `.okashi-folded.json`, collapsed by default) —
the sidebar's own text-picker hand-off is retired for this case; `screenTextPicker` remains
used by the corkboard and by `ctrl+n` on an empty chapter
```

- [ ] **Step 3: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: document the sidebar's foldable multi-scene chapter rows"
```

---

## Final verification

- [ ] Run the complete test suite: `go test ./...` — expect PASS, zero regressions.
- [ ] Run `go vet ./...` — expect clean.
- [ ] Manual smoke test (see the `run` skill or launch `go run .` directly): open a manuscript
  with a 2+-scene chapter, confirm `▸` shows, press `enter` to expand (scenes appear indented,
  `▾` shows), press `enter` again to collapse, navigate into a child scene row and confirm
  `enter` opens that scene's file in the editor. Close and reopen okashi on the same manuscript
  — confirm the expanded chapter is still expanded (sidecar persistence).
