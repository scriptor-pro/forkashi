# Scène indépendante (hors chapitre) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let a manuscript have standalone scenes — root-level ordered texts with no
containing chapter folder — creatable from `ctrl+n` without a chapter preselected, and fix
the preexisting corkboard bug where `enter` on a multi-scene chapter's card silently opens
only its first scene.

**Architecture:** A new `Scene bool` field on the existing `manifestChapter`/`chapterRef`
types marks a standalone scene (`Folder == ""`, exactly one text, file lives at the
manuscript root). Every consumer that already iterates chapters (resolver, sidebar,
corkboard, structure mode, export) sees a standalone scene as a `chapterRef` shaped exactly
like a single-text chapter, so most of the codebase needs zero changes — only the sites that
key off `Folder` (chapter identity) or that render a folder-vs-plain-file distinction need to
learn about `Scene`.

**Tech Stack:** Go 1.25, Bubble Tea (Elm-style Model/Update/View), no new dependencies.

**Spec:** `docs/superpowers/specs/2026-08-23-standalone-scene-design.md`

## Global Constraints

- HARD GATE lifted for this fork: forkashi has no known companion macOS app consuming
  `manifest.json` — manifest shape changes happen directly in this repo, no cross-repo
  coordination.
- `manifestSchemaVersion` moves from 2 to 3. `readManifest` must keep refusing any other
  version.
- Invariant to hold everywhere: `Scene == true` ⟺ `Folder == "" && len(Texts) == 1`.
- No sub-folder for a standalone scene — its file always lives directly at the manuscript
  root.
- No scene↔chapter promotion, no standalone scene inside a Part — both explicitly out of
  scope.
- Atomic writes only (`atomicWrite`) for every file/manifest write — never write in place.
- Every new/changed status message stays in French, matching the existing style (see
  `createChapter`, `createResource`, `createScene` for tone).

---

## File Structure

- **Modify `manifest.go`**: `manifestChapter` gains `Scene bool`; `manifestSchemaVersion`
  becomes 3; new `findSceneByFile`.
- **Modify `manuscript.go`**: `chapterRef` gains `scene bool`; `manifestView`'s `resolve()`
  handles `Scene: true` entries without touching disk as a folder; `isChapterOf` excludes
  scenes; `looseFiles` excludes files referenced by a standalone scene.
- **Modify `filelist.go`**: `fileEntry` gains `isScene bool`; `SetDir` sets it for
  `ch.scene`-true entries; `isChapterEntry` stops matching a standalone scene as a legacy
  chapter.
- **Modify `icons.go`**: new `scene` glyph in `iconSet`; `iconFor` returns it for a
  scene-flagged entry.
- **Modify `main.go`**: `ctrl+n` picker always offers `s`; new `createStandaloneScene`;
  `confirmCreate` routes `kind == 3` by whether `chapterFolder` is empty; the two duplicated
  picker status strings drop their conditional.
- **Modify `structure.go`**: `toManifestChapters` preserves `Scene` so a corkboard/structure
  commit doesn't silently drop a staged standalone scene's flag.
- **Modify `corkboard.go`**: `enter` routes to `screenTextPicker` when the selected card has
  2+ texts, instead of always opening `texts[0]`.
- **Test files touched**: `manifest_test.go`, `manuscript_test.go`, `filelist_test.go`,
  `main_test.go`, `structure_test.go` (new), `corkboard_test.go`, plus a global
  `"schemaVersion":2` → `"schemaVersion":3` fixture update across every `*_test.go` that
  embeds a literal manifest JSON string (`export_wiring_test.go`, `home_test.go`,
  `notes_test.go`, `pager_wiring_test.go`, `rename_wiring_test.go` — none of these need new
  test cases, only the literal version bump so their fixtures stay valid).

---

## Task 1: Bump `manifestSchemaVersion` to 3 and fix every literal-JSON test fixture

The whole codebase is currently pinned to `schemaVersion: 2`. Every test that writes a raw
JSON manifest string will start failing the moment the constant changes, so this has to land
as its own task, first, with nothing else riding on it — it's a pure mechanical version bump
plus two tests that must be *retargeted* (not just bumped) because they test rejection of an
unsupported version.

**Files:**
- Modify: `manifest.go:12`
- Modify: `manifest_test.go` (`TestReadManifestRejectsBadVersion`)
- Modify: `manuscript_test.go` (`TestResolveUnreadableManifestRefuses`)
- Modify: `export_wiring_test.go`, `filelist_test.go`, `home_test.go`, `main_test.go`,
  `notes_test.go`, `pager_wiring_test.go`, `rename_wiring_test.go`, `corkboard_test.go` (only
  where `writeManifestRaw`/`os.WriteFile` embeds a literal `"schemaVersion":2` or
  `"schemaVersion": 2` string — `corkboard_test.go`'s `seedCorkManuscript` calls
  `writeManifest` directly with `manifestSchemaVersion`, so it needs NO change)

**Interfaces:**
- Consumes: nothing new.
- Produces: `manifestSchemaVersion == 3`, read/written by every later task.

- [ ] **Step 1: Confirm the current rejection tests fail the way we expect before touching anything**

Run: `go test ./... -run 'TestReadManifestRejectsBadVersion|TestResolveUnreadableManifestRefuses' -v`
Expected: PASS (baseline, before any change — this just confirms the two tests exist and are
green today, so we know we're editing the right ones).

- [ ] **Step 2: Bump the constant**

In `manifest.go:12`, change:
```go
const manifestSchemaVersion = 2
```
to:
```go
const manifestSchemaVersion = 3
```

- [ ] **Step 3: Retarget the two explicit-rejection tests to schemaVersion 4**

These two tests exist specifically to prove `readManifest`/`resolveManuscript` refuse an
*unsupported* version — they must keep testing rejection, so bump their literal fixture past
the new supported version (3), not to it.

In `manifest_test.go`, `TestReadManifestRejectsBadVersion`:
```go
func TestReadManifestRejectsBadVersion(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, manifestName),
		[]byte(`{"schemaVersion":4,"title":"X","items":[]}`), 0o644)
	_, present, err := readManifest(dir)
	if !present {
		t.Fatal("a present-but-unsupported manifest must report present=true")
	}
	if err == nil {
		t.Fatal("schemaVersion 4 must be refused with an error, not silently read")
	}
}
```

In `manuscript_test.go`, `TestResolveUnreadableManifestRefuses`:
```go
func TestResolveUnreadableManifestRefuses(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-a.md"), []byte("x"), 0o644) // numbered, but...
	writeManifestRaw(t, dir, `{"schemaVersion":4,"title":"N","items":[]}`)
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
```

- [ ] **Step 4: Bulk-fix every remaining literal `schemaVersion:2` fixture**

Run this to find every remaining site (should exclude the two files just hand-edited, which
no longer contain `:2`, and exclude `corkboard_test.go` which uses `writeManifest` not a
literal string):

```bash
grep -rn '"schemaVersion":2\|"schemaVersion": 2' --include="*_test.go" .
```

For every match, replace `"schemaVersion":2` with `"schemaVersion":3` and
`"schemaVersion": 2` with `"schemaVersion": 3` (plain text substitution — these are JSON test
fixtures, not code referencing the constant). Do this file by file with your editor's
find-and-replace scoped to each file, so you can eyeball each match's context (a couple of
these files may have unrelated `2`s nearby — only touch the `schemaVersion` ones).

- [ ] **Step 5: Run the full suite**

Run: `go test ./... 2>&1 | tail -60`
Expected: PASS, zero failures. If anything still fails on a `schemaVersion` mismatch, grep
again — a fixture was missed.

- [ ] **Step 6: Commit**

```bash
git add manifest.go manifest_test.go manuscript_test.go export_wiring_test.go filelist_test.go home_test.go main_test.go notes_test.go pager_wiring_test.go rename_wiring_test.go
git commit -m "manifest: bump schemaVersion to 3, update test fixtures"
```

---

## Task 2: `manifestChapter.Scene` + `findSceneByFile`

**Files:**
- Modify: `manifest.go`
- Test: `manifest_test.go`

**Interfaces:**
- Consumes: `manifestChapter`, `manifestItem`, `manifest` (all existing, `manifest.go:16-44`).
- Produces: `manifestChapter.Scene bool` (JSON tag `"scene,omitempty"`); `findSceneByFile(m
  *manifest, file string) *manifestChapter`.

- [ ] **Step 1: Write the failing tests**

Append to `manifest_test.go`:

```go
func TestManifestChapterSceneFieldRoundTrips(t *testing.T) {
	dir := t.TempDir()
	m := manifest{
		Title: "Windermere",
		Items: []manifestItem{
			{Chapter: &manifestChapter{Folder: "opening", Title: "Chapter One",
				Texts: []manifestText{{File: "opening.md", Title: "Opening"}}}},
			{Chapter: &manifestChapter{Title: "A Stray Thought", Scene: true,
				Texts: []manifestText{{File: "a-stray-thought.md", Title: "A Stray Thought"}}}},
		},
	}
	if err := writeManifest(dir, m); err != nil {
		t.Fatal(err)
	}
	got, present, err := readManifest(dir)
	if !present || err != nil {
		t.Fatalf("round-trip read: present=%v err=%v", present, err)
	}
	if got.Items[0].Chapter.Scene {
		t.Fatalf("ordinary chapter must decode Scene=false, got true")
	}
	if !got.Items[1].Chapter.Scene {
		t.Fatalf("standalone scene must decode Scene=true, got false")
	}
	if got.Items[1].Chapter.Folder != "" {
		t.Fatalf("standalone scene must have empty Folder, got %q", got.Items[1].Chapter.Folder)
	}
}

func TestManifestV2WithoutSceneFieldDefaultsFalse(t *testing.T) {
	dir := t.TempDir()
	writeManifestRaw(t, dir, `{"schemaVersion":3,"title":"N","items":[
		{"chapter":{"folder":"a","title":"A","texts":[{"file":"a.md","title":"A"}]}}
	]}`)
	m, present, err := readManifest(dir)
	if !present || err != nil {
		t.Fatalf("present=%v err=%v", present, err)
	}
	if m.Items[0].Chapter.Scene {
		t.Fatal("a manifest entry with no scene field must decode Scene=false")
	}
}

func TestFindSceneByFileFinds(t *testing.T) {
	m := manifest{SchemaVersion: manifestSchemaVersion, Items: []manifestItem{
		{Chapter: &manifestChapter{Folder: "un", Title: "Un",
			Texts: []manifestText{{File: "un.md", Title: "Un"}}}},
		{Chapter: &manifestChapter{Title: "Aparté", Scene: true,
			Texts: []manifestText{{File: "aparte.md", Title: "Aparté"}}}},
	}}
	sc := findSceneByFile(&m, "aparte.md")
	if sc == nil || !sc.Scene || sc.Title != "Aparté" {
		t.Fatalf("expected to find the standalone scene, got %+v", sc)
	}
}

func TestFindSceneByFileIgnoresNonSceneChapters(t *testing.T) {
	m := manifest{SchemaVersion: manifestSchemaVersion, Items: []manifestItem{
		{Chapter: &manifestChapter{Folder: "un", Title: "Un",
			Texts: []manifestText{{File: "un.md", Title: "Un"}}}},
	}}
	// "un.md" is a chapter's text, not a standalone scene's file — must not match.
	if sc := findSceneByFile(&m, "un.md"); sc != nil {
		t.Fatalf("expected nil (un.md belongs to a folder chapter, not a scene), got %+v", sc)
	}
}

func TestFindSceneByFileNotFound(t *testing.T) {
	m := manifest{SchemaVersion: manifestSchemaVersion}
	if sc := findSceneByFile(&m, "nope.md"); sc != nil {
		t.Fatalf("expected nil for an absent file, got %+v", sc)
	}
}

func TestFindSceneByFileMutatesThroughPointer(t *testing.T) {
	m := manifest{SchemaVersion: manifestSchemaVersion, Items: []manifestItem{
		{Chapter: &manifestChapter{Title: "Aparté", Scene: true,
			Texts: []manifestText{{File: "aparte.md", Title: "Aparté"}}}},
	}}
	sc := findSceneByFile(&m, "aparte.md")
	sc.Title = "Renamed"
	if m.Items[0].Chapter.Title != "Renamed" {
		t.Fatalf("mutating through the returned pointer must mutate m, got %q", m.Items[0].Chapter.Title)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test . -run 'TestManifestChapterSceneField|TestManifestV2WithoutSceneField|TestFindSceneByFile' -v`
Expected: FAIL — `Scene` field and `findSceneByFile` don't exist yet (compile error).

- [ ] **Step 3: Add the `Scene` field**

In `manifest.go`, replace:
```go
// manifestChapter is one chapter: a folder (slug, birth-stable — never renamed by a
// retitle or reorder) holding one or more ordered texts.
type manifestChapter struct {
	Folder string         `json:"folder"`
	Title  string         `json:"title"`
	Texts  []manifestText `json:"texts"`
}
```
with:
```go
// manifestChapter is one chapter — OR, when Scene is true, one standalone scene: a single
// ordered text with no sub-texts and no folder of its own (its file lives at the manuscript
// root, named by Texts[0].File). Folder is "" for a scene. Invariant: Scene == true iff
// Folder == "" && len(Texts) == 1.
type manifestChapter struct {
	Folder string         `json:"folder"`
	Title  string         `json:"title"`
	Texts  []manifestText `json:"texts"`
	Scene  bool           `json:"scene,omitempty"`
}
```

- [ ] **Step 4: Add `findSceneByFile`**

In `manifest.go`, after `findChapterByFolder` (end of file):
```go
// findSceneByFile returns a pointer to the standalone-scene manifestChapter whose
// Texts[0].File matches file, at the manuscript root — the scene identity lookup mirroring
// findChapterByFolder (a scene has no Folder, so its birth-stable identity is its filename
// instead). Returns nil if no standalone scene with that file exists.
func findSceneByFile(m *manifest, file string) *manifestChapter {
	for i := range m.Items {
		if m.Items[i].Chapter != nil && m.Items[i].Chapter.Scene &&
			len(m.Items[i].Chapter.Texts) > 0 && m.Items[i].Chapter.Texts[0].File == file {
			return m.Items[i].Chapter
		}
	}
	return nil
}
```

Note: a standalone scene can only ever be a bare item (never inside a Part — out of scope per
spec), so this doesn't need to descend into `Chapters[]` the way `findChapterByFolder` does.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test . -run 'TestManifestChapterSceneField|TestManifestV2WithoutSceneField|TestFindSceneByFile' -v`
Expected: PASS.

- [ ] **Step 6: Run the full suite**

Run: `go test ./... 2>&1 | tail -30`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add manifest.go manifest_test.go
git commit -m "manifest: add manifestChapter.Scene and findSceneByFile"
```

---

## Task 3: `chapterRef.scene` + resolver support in `manuscript.go`

**Files:**
- Modify: `manuscript.go`
- Test: `manuscript_test.go`

**Interfaces:**
- Consumes: `manifestChapter.Scene` (Task 2), `chapterRef` (`manuscript.go:25-29`),
  `manifestView`'s internal `resolve()` closure (`manuscript.go:169-179`).
- Produces: `chapterRef.scene bool`; `manifestView` correctly resolves `Scene: true` items;
  `isChapterOf` returns `false` for a standalone scene; `looseFiles` excludes a standalone
  scene's file.

- [ ] **Step 1: Write the failing tests**

Append to `manuscript_test.go`:

```go
func TestResolveManifestStandaloneScenePresent(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "un-aparte.md"), []byte("hello"), 0o644)
	writeManifestRaw(t, dir, `{"schemaVersion":3,"title":"N","items":[
		{"chapter":{"folder":"opening","title":"Opening","texts":[{"file":"opening.md","title":"Opening"}]}},
		{"chapter":{"title":"Un aparté","scene":true,"texts":[{"file":"un-aparte.md","title":"Un aparté"}]}}
	]}`)
	mkChapterDir(t, dir, "opening", map[string]string{"opening.md": "x"})
	v := resolveManuscript(dir, readEntries(dir))
	if len(v.parts[0].chapters) != 2 {
		t.Fatalf("want 2 entries (chapter + standalone scene), got %+v", v.parts[0].chapters)
	}
	sc := v.parts[0].chapters[1]
	if !sc.scene || sc.folder != "" || sc.title != "Un aparté" {
		t.Fatalf("standalone scene chapterRef = %+v", sc)
	}
	if len(sc.texts) != 1 || sc.texts[0].file != "un-aparte.md" {
		t.Fatalf("standalone scene texts = %+v", sc.texts)
	}
}

func TestResolveManifestStandaloneSceneAbsentFileOmitted(t *testing.T) {
	dir := t.TempDir()
	// "gone.md" is listed but never created on disk.
	writeManifestRaw(t, dir, `{"schemaVersion":3,"title":"N","items":[
		{"chapter":{"title":"Gone","scene":true,"texts":[{"file":"gone.md","title":"Gone"}]}}
	]}`)
	v := resolveManuscript(dir, readEntries(dir))
	if len(v.parts) != 0 && len(v.parts[0].chapters) != 0 {
		t.Fatalf("a standalone scene whose file is absent must be omitted, got %+v", v.parts)
	}
}

func TestIsChapterOfExcludesStandaloneScene(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "aparte.md"), []byte("x"), 0o644)
	writeManifestRaw(t, dir, `{"schemaVersion":3,"title":"N","items":[
		{"chapter":{"title":"Aparté","scene":true,"texts":[{"file":"aparte.md","title":"Aparté"}]}}
	]}`)
	v := resolveManuscript(dir, readEntries(dir))
	if isChapterOf(v, "") {
		t.Fatal("a standalone scene (folder \"\") must never be reported as a chapter by isChapterOf")
	}
}

func TestLooseFilesExcludesStandaloneSceneFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "aparte.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, "notes.md"), []byte("y"), 0o644) // a real Resource
	writeManifestRaw(t, dir, `{"schemaVersion":3,"title":"N","items":[
		{"chapter":{"title":"Aparté","scene":true,"texts":[{"file":"aparte.md","title":"Aparté"}]}}
	]}`)
	v := resolveManuscript(dir, readEntries(dir))
	if len(v.loose) != 1 || v.loose[0].name != "notes.md" {
		t.Fatalf("loose = %+v, want only notes.md (aparte.md belongs to a scene, not loose)", v.loose)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test . -run 'TestResolveManifestStandaloneScene|TestIsChapterOfExcludesStandaloneScene|TestLooseFilesExcludesStandaloneSceneFile' -v`
Expected: FAIL — `chapterRef.scene` doesn't exist (compile error), and once that's stubbed in,
the standalone-scene resolution and loose-exclusion behaviors are missing.

- [ ] **Step 3: Add `scene` to `chapterRef`**

In `manuscript.go:25-29`, replace:
```go
// chapterRef is one ordered chapter: a folder (empty only for a legacy flat-file
// chapter, pre-v2 display fallback) holding one or more ordered texts.
type chapterRef struct {
	folder string
	title  string
	texts  []textRef
}
```
with:
```go
// chapterRef is one ordered chapter: a folder (empty only for a legacy flat-file chapter,
// pre-v2 display fallback, OR a standalone scene — see scene) holding one or more ordered
// texts. scene == true means folder == "" for a DIFFERENT reason than the legacy fallback: a
// standalone scene has no chapter folder by design (manifest v3), not because it predates
// the manifest.
type chapterRef struct {
	folder string
	title  string
	texts  []textRef
	scene  bool
}
```

- [ ] **Step 4: Handle `Scene: true` in `manifestView`'s `resolve()`**

In `manuscript.go:159-179`, the `resolve` closure currently does:
```go
	resolve := func(mc manifestChapter) (chapterRef, bool) {
		info, err := os.Stat(filepath.Join(dir, mc.Folder))
		if err != nil || !info.IsDir() {
			return chapterRef{}, false
		}
		return chapterRef{
			folder: mc.Folder,
			title:  mc.Title,
			texts:  resolveChapterTexts(filepath.Join(dir, mc.Folder), mc.Texts),
		}, true
	}
```

Replace it with:
```go
	resolve := func(mc manifestChapter) (chapterRef, bool) {
		if mc.Scene {
			if len(mc.Texts) == 0 {
				return chapterRef{}, false
			}
			if _, err := os.Stat(filepath.Join(dir, mc.Texts[0].File)); err != nil {
				return chapterRef{}, false
			}
			return chapterRef{
				title:  mc.Title,
				texts:  []textRef{{file: mc.Texts[0].File, title: mc.Texts[0].Title}},
				scene:  true,
			}, true
		}
		info, err := os.Stat(filepath.Join(dir, mc.Folder))
		if err != nil || !info.IsDir() {
			return chapterRef{}, false
		}
		return chapterRef{
			folder: mc.Folder,
			title:  mc.Title,
			texts:  resolveChapterTexts(filepath.Join(dir, mc.Folder), mc.Texts),
		}, true
	}
```

- [ ] **Step 5: Exclude scenes from `isChapterOf`**

In `manuscript.go:125-136`, replace:
```go
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
```
with:
```go
// isChapterOf reports whether folder is a chapter of the given manuscript view, across all
// parts (including the synthetic untitled one). A standalone scene never matches, even
// though its folder is also "" — folder alone can't distinguish a legacy flat-file chapter
// from a scene, so scene entries are filtered out explicitly.
func isChapterOf(v manuscriptView, folder string) bool {
	for _, p := range v.parts {
		for _, c := range p.chapters {
			if c.scene {
				continue
			}
			if c.folder == folder {
				return true
			}
		}
	}
	return false
}
```

- [ ] **Step 6: Exclude a standalone scene's file from `looseFiles`**

In `manuscript.go:219-234`, replace:
```go
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
with:
```go
// looseFiles returns every root-level .md file not referenced as a text inside any listed
// chapter, AND not referenced by a standalone scene — a Resource (design: unlisted files are
// Resources). Ordinary v2 chapters are always folders, so a root-level .md file can only ever
// collide with a standalone scene (v3), never with an ordinary chapter's text.
func looseFiles(m manifest, entries []fileEntry) []fileEntry {
	sceneFiles := map[string]bool{}
	for _, it := range m.Items {
		if it.Chapter != nil && it.Chapter.Scene && len(it.Chapter.Texts) > 0 {
			sceneFiles[it.Chapter.Texts[0].File] = true
		}
	}
	var out []fileEntry
	for _, e := range entries {
		if !e.isDir && !sceneFiles[e.name] {
			out = append(out, e)
		}
	}
	return out
}
```

- [ ] **Step 7: Run tests to verify they pass**

Run: `go test . -run 'TestResolveManifestStandaloneScene|TestIsChapterOfExcludesStandaloneScene|TestLooseFilesExcludesStandaloneSceneFile' -v`
Expected: PASS.

- [ ] **Step 8: Run the full suite**

Run: `go test ./... 2>&1 | tail -30`
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add manuscript.go manuscript_test.go
git commit -m "manuscript: resolve standalone scenes, exclude them from isChapterOf/looseFiles"
```

---

## Task 4: Sidebar display — `fileEntry.isScene` + a distinct icon

**Files:**
- Modify: `filelist.go`
- Modify: `icons.go`
- Test: `filelist_test.go`, `icons_test.go`

**Interfaces:**
- Consumes: `chapterRef.scene` (Task 3), `fileEntry` (`filelist.go:24-28`), `iconSet`
  (`icons.go:21-24`), `iconFor` (`icons.go:84-95`).
- Produces: `fileEntry.isScene bool`; `iconSet.scene glyph`; `iconFor` returns it for a
  scene-flagged entry; `isChapterEntry` no longer mistakes a standalone scene for a legacy
  chapter.

- [ ] **Step 1: Write the failing tests**

Append to `filelist_test.go` (check its top imports match — it's in `package main`, same
style as `manuscript_test.go`'s `mkChapterDir`/`writeManifestRaw` helpers, already available
package-wide):

```go
func TestSetDirMarksStandaloneSceneEntry(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "aparte.md"), []byte("x"), 0o644)
	writeManifestRaw(t, dir, `{"schemaVersion":3,"title":"N","items":[
		{"chapter":{"title":"Aparté","scene":true,"texts":[{"file":"aparte.md","title":"Aparté"}]}}
	]}`)
	f := newFilelist()
	f.SetDir(dir)
	var found bool
	for _, e := range f.entries {
		if e.name == "aparte.md" {
			found = true
			if !e.isScene {
				t.Fatalf("standalone scene entry must have isScene=true, got %+v", e)
			}
		}
	}
	if !found {
		t.Fatal("aparte.md must appear in f.entries")
	}
}

func TestSetDirLegacyFlatChapterIsNotMarkedScene(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-a.md"), []byte("x"), 0o644)
	f := newFilelist()
	f.SetDir(dir)
	for _, e := range f.entries {
		if e.name == "01-a.md" && e.isScene {
			t.Fatalf("a legacy flat-file chapter must NOT be marked isScene, got %+v", e)
		}
	}
}

func TestIsChapterEntryFalseForStandaloneScene(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "aparte.md"), []byte("x"), 0o644)
	writeManifestRaw(t, dir, `{"schemaVersion":3,"title":"N","items":[
		{"chapter":{"title":"Aparté","scene":true,"texts":[{"file":"aparte.md","title":"Aparté"}]}}
	]}`)
	f := newFilelist()
	f.SetDir(dir)
	var entry fileEntry
	for _, e := range f.entries {
		if e.name == "aparte.md" {
			entry = e
		}
	}
	if f.isChapterEntry(entry) {
		t.Fatal("a standalone scene must not be reported as a chapter entry (it has its own icon/behavior, not the legacy-chapter path)")
	}
}
```

Append to `icons_test.go`:

```go
func TestIconForSceneEntry(t *testing.T) {
	s := resolveIcons()
	g := s.iconFor(fileEntry{name: "aparte.md", isScene: true})
	if g.ch != s.scene.ch {
		t.Fatalf("a scene entry must render the scene glyph, got %+v want %+v", g, s.scene)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test . -run 'TestSetDirMarksStandaloneSceneEntry|TestSetDirLegacyFlatChapterIsNotMarkedScene|TestIsChapterEntryFalseForStandaloneScene|TestIconForSceneEntry' -v`
Expected: FAIL — `isScene` field doesn't exist (compile error).

- [ ] **Step 3: Add `isScene` to `fileEntry`**

In `filelist.go:24-28`, replace:
```go
type fileEntry struct {
	name         string
	isDir        bool
	isPartHeader bool // a non-selectable Part title row; cursor movement skips it
}
```
with:
```go
type fileEntry struct {
	name         string
	isDir        bool
	isPartHeader bool // a non-selectable Part title row; cursor movement skips it
	isScene      bool // a standalone scene (manifest v3, chapterRef.scene) — distinct glyph
}
```

- [ ] **Step 4: Set `isScene` in `SetDir`**

In `filelist.go:149-157`, replace:
```go
		for _, ch := range p.chapters {
			if ch.folder == "" {
				if len(ch.texts) > 0 {
					f.entries = append(f.entries, fileEntry{name: ch.texts[0].file})
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
		}
```

- [ ] **Step 5: Make `isChapterEntry` exclude standalone scenes**

In `filelist.go:225-241`, replace:
```go
// isChapterEntry reports whether e (an entry from f.entries, as built by SetDir) represents a
// manuscript chapter — either a v2 chapter folder (matched by isChapterOf on e.name as a
// folder) or a legacy single-file chapter (matched by its flat filename against texts[0].file,
// since a legacy chapterRef always has folder == ""; isChapterOf can't see those by name).
func (f filelist) isChapterEntry(e fileEntry) bool {
	if e.isDir {
		return isChapterOf(f.view, e.name)
	}
	for _, p := range f.view.parts {
		for _, ch := range p.chapters {
			if ch.folder == "" && len(ch.texts) > 0 && ch.texts[0].file == e.name {
				return true
			}
		}
	}
	return false
}
```
with:
```go
// isChapterEntry reports whether e (an entry from f.entries, as built by SetDir) represents a
// manuscript chapter — either a v2 chapter folder (matched by isChapterOf on e.name as a
// folder) or a legacy single-file chapter (matched by its flat filename against texts[0].file,
// since a legacy chapterRef always has folder == ""; isChapterOf can't see those by name). A
// standalone scene (e.isScene) shares the same folder=="" shape as a legacy flat-file chapter
// but is NOT a chapter — excluded explicitly so it gets its own render path.
func (f filelist) isChapterEntry(e fileEntry) bool {
	if e.isScene {
		return false
	}
	if e.isDir {
		return isChapterOf(f.view, e.name)
	}
	for _, p := range f.view.parts {
		for _, ch := range p.chapters {
			if ch.folder == "" && !ch.scene && len(ch.texts) > 0 && ch.texts[0].file == e.name {
				return true
			}
		}
	}
	return false
}
```

- [ ] **Step 6: Add a `scene` glyph to `iconSet` and wire `iconFor`**

In `icons.go:21-24`, replace:
```go
// iconSet is the glyph set for the file pane and launch lists.
type iconSet struct {
	folder, parent, file, action glyph
	byExt                        map[string]glyph
}
```
with:
```go
// iconSet is the glyph set for the file pane and launch lists.
type iconSet struct {
	folder, parent, file, action, scene glyph
	byExt                                map[string]glyph
}
```

In `icons.go:45-54` (the `plain`/`ascii` branch), replace:
```go
	case "plain", "ascii":
		return iconSet{
			folder: glyph{ch: "▸ "},
			parent: glyph{ch: "↑ "},
			file:   glyph{ch: "  "},
			action: glyph{ch: "+ "},
			byExt:  map[string]glyph{},
		}
```
with:
```go
	case "plain", "ascii":
		return iconSet{
			folder: glyph{ch: "▸ "},
			parent: glyph{ch: "↑ "},
			file:   glyph{ch: "  "},
			action: glyph{ch: "+ "},
			scene:  glyph{ch: "· "},
			byExt:  map[string]glyph{},
		}
```

In `icons.go:58-63` (the nerd-font branch), replace:
```go
	return iconSet{
		folder: glyph{ch: " ", color: iconFolderColor},  // nf-fa-folder
		parent: glyph{ch: " ", color: iconParentColor},  // nf-fa-arrow_up
		file:   glyph{ch: " ", color: iconGenericColor}, // nf-fa-file
		action: glyph{ch: " ", color: accent},           // nf-fa-plus
```
with:
```go
	return iconSet{
		folder: glyph{ch: " ", color: iconFolderColor},  // nf-fa-folder
		parent: glyph{ch: " ", color: iconParentColor},  // nf-fa-arrow_up
		file:   glyph{ch: " ", color: iconGenericColor}, // nf-fa-file
		action: glyph{ch: " ", color: accent},           // nf-fa-plus
		scene:  glyph{ch: " ", color: iconTextColor},    // nf-fa-align_left — a single ordered text, no folder
```

(leave the rest of that struct literal — `byExt: map[string]glyph{...}` — untouched, just
close the literal the same way it already does after this point).

In `icons.go:84-95`, replace:
```go
// iconFor returns the glyph (ch + color) for an entry.
func (s iconSet) iconFor(e fileEntry) glyph {
	switch {
	case e.name == "..":
		return s.parent
	case e.isDir:
		return s.folder
	}
	if g, ok := s.byExt[strings.ToLower(filepath.Ext(e.name))]; ok {
		return g
	}
	return s.file
}
```
with:
```go
// iconFor returns the glyph (ch + color) for an entry.
func (s iconSet) iconFor(e fileEntry) glyph {
	switch {
	case e.name == "..":
		return s.parent
	case e.isScene:
		return s.scene
	case e.isDir:
		return s.folder
	}
	if g, ok := s.byExt[strings.ToLower(filepath.Ext(e.name))]; ok {
		return g
	}
	return s.file
}
```

- [ ] **Step 7: Run tests to verify they pass**

Run: `go test . -run 'TestSetDirMarksStandaloneSceneEntry|TestSetDirLegacyFlatChapterIsNotMarkedScene|TestIsChapterEntryFalseForStandaloneScene|TestIconForSceneEntry' -v`
Expected: PASS.

- [ ] **Step 8: Run the full suite**

Run: `go test ./... 2>&1 | tail -30`
Expected: PASS. (`TestNerdIconsAreRealGlyphs`, referenced by `icons.go:14`'s comment, must
still pass — it presumably iterates every glyph in the nerd set, so the new `scene` glyph
needs to be a real Nerd Font codepoint, not a placeholder. If that test fails on the new
glyph, check the existing `text`/`code` glyphs in `icons.go:55-57` for the correct escape
style and substitute a real nf-fa glyph rather than guessing.)

- [ ] **Step 9: Commit**

```bash
git add filelist.go icons.go filelist_test.go icons_test.go
git commit -m "sidebar: give standalone scenes a distinct icon, exclude them from isChapterEntry"
```

---

## Task 5: `createStandaloneScene` + `ctrl+n` picker always offers `s`

**Files:**
- Modify: `main.go`
- Test: `main_test.go`, `create_prompt_test.go`

**Interfaces:**
- Consumes: `model.createPicker`, `model.createKind`, `model.createChapterFolder`
  (`main.go:439-441`), `confirmCreate` (`main.go:2439-2467`), `createChapter`
  (`main.go:2319-2356`), `createScene` (`main.go:2400-2437`, pre-existing).
- Produces: `(m *model) createStandaloneScene(name string)`; `confirmCreate` routes `kind ==
  3` between `createScene` and `createStandaloneScene` by whether `chapterFolder == ""`; the
  `ctrl+n` picker's `s` case no longer ignores a stray keypress when no chapter is selected;
  both picker status strings always include `s scène`.

- [ ] **Step 1: Write the failing tests**

Append to `main_test.go` (check its existing helpers — likely reuses `mkChapterDir`,
`writeManifestRaw`/`writeManifest` from `manuscript_test.go`/`corkboard_test.go`, same
package):

```go
func TestCreateStandaloneSceneCreatesFileAndManifestEntry(t *testing.T) {
	dir := t.TempDir()
	if err := writeManifest(dir, manifest{
		Title: "N",
		Items: []manifestItem{
			{Chapter: &manifestChapter{Folder: "opening", Title: "Opening",
				Texts: []manifestText{{File: "opening.md", Title: "Opening"}}}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	mkChapterDir(t, dir, "opening", map[string]string{"opening.md": "x"})

	m := model{}
	m.files.dir = dir
	m.files.SetDir(dir)
	m.createStandaloneScene("Un aparté")

	if _, err := os.Stat(filepath.Join(dir, "un-aparte.md")); err != nil {
		t.Fatalf("expected un-aparte.md at manuscript root, got err=%v", err)
	}
	got, present, err := readManifest(dir)
	if !present || err != nil {
		t.Fatalf("present=%v err=%v", present, err)
	}
	if len(got.Items) != 2 {
		t.Fatalf("want 2 items (existing chapter + new scene), got %d: %+v", len(got.Items), got.Items)
	}
	last := got.Items[1].Chapter
	if last == nil || !last.Scene || last.Title != "Un aparté" || last.Texts[0].File != "un-aparte.md" {
		t.Fatalf("new item = %+v", got.Items[1])
	}
	if m.status != "nouvelle scène Un aparté" {
		t.Fatalf("status = %q", m.status)
	}
}

func TestCreateStandaloneSceneRejectsPathSeparator(t *testing.T) {
	dir := t.TempDir()
	m := model{}
	m.files.dir = dir
	m.createStandaloneScene("sous/dossier")
	if m.status != "un nom de scène ne peut pas contenir de séparateur de chemin" {
		t.Fatalf("status = %q", m.status)
	}
	if _, err := os.ReadDir(dir); err == nil {
		entries, _ := os.ReadDir(dir)
		if len(entries) != 0 {
			t.Fatalf("no file should have been created, got %+v", entries)
		}
	}
}

func TestCreateStandaloneSceneRejectsNameCollision(t *testing.T) {
	dir := t.TempDir()
	if err := writeManifest(dir, manifest{Title: "N"}); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "notes.md"), []byte("existing"), 0o644)
	m := model{}
	m.files.dir = dir
	m.createStandaloneScene("Notes")
	if m.status != "un fichier nommé notes.md existe déjà dans ce manuscrit" {
		t.Fatalf("status = %q", m.status)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "notes.md"))
	if string(data) != "existing" {
		t.Fatal("existing file must not be overwritten")
	}
}

func TestConfirmCreateRoutesKind3ByChapterFolder(t *testing.T) {
	dir := t.TempDir()
	if err := writeManifest(dir, manifest{
		Title: "N",
		Items: []manifestItem{
			{Chapter: &manifestChapter{Folder: "un", Title: "Un",
				Texts: []manifestText{{File: "un.md", Title: "Un"}}}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	mkChapterDir(t, dir, "un", map[string]string{"un.md": "x"})

	// With chapterFolder set: routes to createScene (adds to that chapter's Texts).
	m := model{}
	m.files.dir = dir
	m.files.SetDir(dir)
	m.createKind = 3
	m.createChapterFolder = "un"
	m.nameInput.SetValue("Deuxième scène")
	m.confirmCreate()
	got, _, _ := readManifest(dir)
	if len(got.Items[0].Chapter.Texts) != 2 {
		t.Fatalf("scene should have been added to chapter 'un', got %+v", got.Items[0].Chapter.Texts)
	}

	// With chapterFolder empty: routes to createStandaloneScene (new top-level item).
	m2 := model{}
	m2.files.dir = dir
	m2.files.SetDir(dir)
	m2.createKind = 3
	m2.createChapterFolder = ""
	m2.nameInput.SetValue("Scène libre")
	m2.confirmCreate()
	got2, _, _ := readManifest(dir)
	if len(got2.Items) != 2 || !got2.Items[1].Chapter.Scene {
		t.Fatalf("want a new standalone scene item, got %+v", got2.Items)
	}
}

func TestCtrlNPickerAlwaysOffersSceneOption(t *testing.T) {
	dir := t.TempDir()
	if err := writeManifest(dir, manifest{Title: "N"}); err != nil {
		t.Fatal(err)
	}
	m := model{}
	m.files.dir = dir
	m.files.SetDir(dir)
	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlN})
	m = mm.(model)
	if !m.createPicker {
		t.Fatal("ctrl+n in a manifest manuscript must open the picker")
	}
	if m.createChapterFolder != "" {
		t.Fatalf("no chapter selected -> createChapterFolder must be empty, got %q", m.createChapterFolder)
	}
	if !strings.Contains(m.status, "s scène") {
		t.Fatalf("status must always offer 's scène' even with no chapter selected, got %q", m.status)
	}

	// 's' must now be accepted (not a defensive no-op) when no chapter is selected.
	mm2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	m2 := mm2.(model)
	if m2.createKind != 3 || m2.createPicker {
		t.Fatalf("'s' with no chapter selected must proceed to naming a standalone scene, got createKind=%d createPicker=%v", m2.createKind, m2.createPicker)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test . -run 'TestCreateStandaloneScene|TestConfirmCreateRoutesKind3ByChapterFolder|TestCtrlNPickerAlwaysOffersSceneOption' -v`
Expected: FAIL — `createStandaloneScene` doesn't exist (compile error); once stubbed, the
picker still no-ops on `s` with an empty `createChapterFolder`.

- [ ] **Step 3: Add `createStandaloneScene`**

In `main.go`, immediately after `createScene` (after `main.go:2437`):
```go
// createStandaloneScene creates a new blank scene at the manuscript root — not inside any
// chapter — and appends it as a new manifestItem (Scene: true) at the end of items[]. name is
// the scene's display title, slugified into its birth-stable root-level filename.
func (m *model) createStandaloneScene(name string) {
	if strings.Contains(name, "/") {
		m.status = "un nom de scène ne peut pas contenir de séparateur de chemin"
		return
	}
	title := name
	file := slugify(title) + ".md"
	dst := filepath.Join(m.files.dir, file)
	if _, err := os.Stat(dst); err == nil {
		m.status = "un fichier nommé " + file + " existe déjà dans ce manuscrit"
		return
	}
	if err := atomicWrite(dst, []byte(""), 0o644); err != nil {
		m.status = "impossible de créer la scène : " + err.Error()
		return
	}
	mani, present, err := readManifest(m.files.dir)
	if err != nil || !present {
		m.status = "scène créée mais le manifeste est introuvable ou illisible"
		return
	}
	mani.Items = append(mani.Items, manifestItem{Chapter: &manifestChapter{
		Title: title,
		Scene: true,
		Texts: []manifestText{{File: file, Title: title}},
	}})
	if werr := writeManifest(m.files.dir, mani); werr != nil {
		m.status = "scène créée mais échec de la mise à jour du manifeste : " + werr.Error()
		return
	}
	m.files.SetDir(m.files.dir)
	m.loadFile(dst)
	m.focus = focusEditor
	m.editor.Focus()
	m.status = "nouvelle scène " + title
}
```

- [ ] **Step 4: Route `confirmCreate`'s `kind == 3` by `chapterFolder`**

In `main.go:2464-2467`, replace:
```go
	if kind == 3 {
		m.createScene(chapterFolder, name)
		return
	}
```
with:
```go
	if kind == 3 {
		if chapterFolder != "" {
			m.createScene(chapterFolder, name) // scene added to an existing chapter's Texts
		} else {
			m.createStandaloneScene(name) // standalone scene: new top-level item
		}
		return
	}
```

- [ ] **Step 5: Let the picker's `s` case proceed with no chapter selected**

In `main.go:1246-1252`, replace:
```go
			case "s":
				if m.createChapterFolder == "" {
					return m, nil // defensive: option wasn't offered, ignore stray keypress
				}
				m.createPicker, m.createKind = false, 3
				m.startInPaneCreate()
				return m, textinput.Blink
```
with:
```go
			case "s":
				m.createPicker, m.createKind = false, 3
				m.startInPaneCreate()
				return m, textinput.Blink
```

- [ ] **Step 6: Make both picker status strings unconditional**

In `main.go:1675-1692`, replace:
```go
		case "ctrl+n":
			if hasManifest(m.files.dir) {
				// In a manuscript, ask: chapter, resource, or (if a chapter is selected) scene.
				m.createPicker = true
				m.createChapterFolder = ""
				if name, ok := m.files.selectedEntryName(); ok && isChapterOf(m.files.view, name) {
					m.createChapterFolder = name
				}
				if m.createChapterFolder != "" {
					m.status = "nouveau : c chapitre (ordonné) · r ressource (doc libre) · s scène · esc annuler"
				} else {
					m.status = "nouveau : c chapitre (ordonné) · r ressource (doc libre) · esc annuler"
				}
				return m, nil
			}
```
with:
```go
		case "ctrl+n":
			if hasManifest(m.files.dir) {
				// In a manuscript, ask: chapter, resource, or scene — scene targets the
				// selected chapter's Texts when one is selected, else it's a standalone scene.
				m.createPicker = true
				m.createChapterFolder = ""
				if name, ok := m.files.selectedEntryName(); ok && isChapterOf(m.files.view, name) {
					m.createChapterFolder = name
				}
				m.status = "nouveau : c chapitre (ordonné) · r ressource (doc libre) · s scène · esc annuler"
				return m, nil
			}
```

In `main.go:3001-3006`, replace:
```go
	if m.createPicker {
		if m.createChapterFolder != "" {
			return "nouveau : c chapitre · r ressource · s scène · esc annuler"
		}
		return "nouveau : c chapitre · r ressource · esc annuler"
	}
```
with:
```go
	if m.createPicker {
		return "nouveau : c chapitre · r ressource · s scène · esc annuler"
	}
```

- [ ] **Step 7: Run tests to verify they pass**

Run: `go test . -run 'TestCreateStandaloneScene|TestConfirmCreateRoutesKind3ByChapterFolder|TestCtrlNPickerAlwaysOffersSceneOption' -v`
Expected: PASS.

- [ ] **Step 8: Run the full suite**

Run: `go test ./... 2>&1 | tail -30`
Expected: PASS. Pay attention to any pre-existing `create_prompt_test.go` test that asserted
the OLD conditional status string (`"nouveau : c chapitre (ordonné) · r ressource (doc libre)
· esc annuler"` with no chapter selected) — that assertion is now wrong by design and must be
updated to expect `s scène` unconditionally. Search: `grep -n "esc annuler" create_prompt_test.go main_test.go`.

- [ ] **Step 9: Commit**

```bash
git add main.go main_test.go create_prompt_test.go
git commit -m "editor: create a standalone scene from ctrl+n with no chapter selected"
```

---

## Task 6: Retitle (`r`) routes a standalone scene through `findSceneByFile`

**Files:**
- Modify: `manifest.go`
- Modify: `main.go`
- Test: `manifest_test.go`, `rename_wiring_test.go`

**Interfaces:**
- Consumes: `findSceneByFile` (Task 2), `renameChapterTitle` (`manifest.go:148-178`),
  `renameTarget` (`main.go:376-382`), `startRename`/`confirmRename` (`main.go:2551-2705`),
  `filelist.isChapterEntry` (Task 4, now `false` for a scene entry).

**Design note before writing code:** `startRename` currently branches on
`m.files.isChapterEntry(e)`. After Task 4, that's `false` for a standalone scene — so without
a further change, retitling a standalone scene would fall through to the generic
non-chapter/non-section rename path (`renameTarget{dir, name: e.name, isDir: e.isDir}` at
`main.go:2583`), which does a plain **on-disk file rename**, not a manifest-title edit. That
contradicts the spec ("file unchanged (birth-stable), like a chapter — only
`items[].chapter.title` changes"). So `startRename` needs its own scene branch, checked
*before* falling through to the generic path, using `e.isScene` (Task 4) rather than
`isChapterEntry`.

- [ ] **Step 1: Write the failing tests**

Append to `manifest_test.go`:

```go
func TestRenameSceneTitleUpdatesManifestOnly(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "aparte.md"), []byte("x"), 0o644)
	if err := writeManifest(dir, manifest{
		Title: "N",
		Items: []manifestItem{
			{Chapter: &manifestChapter{Title: "Aparté", Scene: true,
				Texts: []manifestText{{File: "aparte.md", Title: "Aparté"}}}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := renameSceneTitle(dir, "aparte.md", "Nouveau titre"); err != nil {
		t.Fatal(err)
	}
	got, _, _ := readManifest(dir)
	if got.Items[0].Chapter.Title != "Nouveau titre" {
		t.Fatalf("title = %q, want %q", got.Items[0].Chapter.Title, "Nouveau titre")
	}
	if got.Items[0].Chapter.Texts[0].File != "aparte.md" {
		t.Fatal("file must stay birth-stable — only the title changes")
	}
	if _, err := os.Stat(filepath.Join(dir, "aparte.md")); err != nil {
		t.Fatal("aparte.md must still exist on disk under its original name")
	}
}

func TestRenameSceneTitleRefusesUnknownFile(t *testing.T) {
	dir := t.TempDir()
	if err := writeManifest(dir, manifest{Title: "N"}); err != nil {
		t.Fatal(err)
	}
	if err := renameSceneTitle(dir, "nope.md", "X"); err == nil {
		t.Fatal("expected an error for a file that isn't a standalone scene")
	}
}
```

Append to `rename_wiring_test.go` (check its existing style — it likely drives `startRename`/
`confirmRename` through `model` the same way `main_test.go`'s picker test does):

```go
func TestStartRenameOnStandaloneSceneTargetsManifestTitle(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "aparte.md"), []byte("x"), 0o644)
	if err := writeManifest(dir, manifest{
		Title: "N",
		Items: []manifestItem{
			{Chapter: &manifestChapter{Title: "Aparté", Scene: true,
				Texts: []manifestText{{File: "aparte.md", Title: "Aparté"}}}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	m := model{}
	m.files.dir = dir
	m.files.SetDir(dir)
	m.files.selected = 0
	for i, e := range m.files.entries {
		if e.name == "aparte.md" {
			m.files.selected = i
		}
	}
	m.startRename()
	if !m.renameTarget.manifestChapter && !m.renameTarget.isDir {
		// scene target must resolve via findSceneByFile, not a plain filesystem rename
	}
	m.nameInput.SetValue("Titre modifié")
	m.confirmRename()
	got, _, _ := readManifest(dir)
	if got.Items[0].Chapter.Title != "Titre modifié" {
		t.Fatalf("title = %q, want %q", got.Items[0].Chapter.Title, "Titre modifié")
	}
	if _, err := os.Stat(filepath.Join(dir, "aparte.md")); err != nil {
		t.Fatal("aparte.md must still exist on disk — a scene retitle must never rename the file")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test . -run 'TestRenameSceneTitle|TestStartRenameOnStandaloneSceneTargetsManifestTitle' -v`
Expected: FAIL — `renameSceneTitle` doesn't exist (compile error).

- [ ] **Step 3: Add `renameSceneTitle`**

In `manifest.go`, after `renameChapterTitle` (after `main.go:178`... i.e. after
`manifest.go:178`):
```go
// renameSceneTitle edits ONLY the items[].chapter.title of the standalone scene whose
// Texts[0].File matches file — the scene counterpart of renameChapterTitle, keyed by
// filename instead of folder (a scene has no folder). The file itself is never renamed
// (birth-stable, mirroring a chapter's folder). Read-modify-writes like renameChapterTitle.
func renameSceneTitle(dir, file, newTitle string) error {
	m, present, err := readManifest(dir)
	if err != nil {
		return err
	}
	if !present {
		return fmt.Errorf("no manifest in %s", dir)
	}
	sc := findSceneByFile(&m, file)
	if sc == nil {
		return fmt.Errorf("%s is not a standalone scene in %s", file, dir)
	}
	sc.Title = newTitle
	return writeManifest(dir, m)
}
```

- [ ] **Step 4: Add a `renameTarget` flag for a standalone scene**

In `main.go:376-382`, replace:
```go
// renameTarget is the item a pending rename prompt will rename.
type renameTarget struct {
	dir             string // directory containing the item
	name            string // current base name
	isDir           bool
	section         bool // a numbered section -> title-only rename (legacy file rename)
	manifestChapter bool // a manifest chapter -> edit items[].title, filename birth-stable
}
```
with:
```go
// renameTarget is the item a pending rename prompt will rename.
type renameTarget struct {
	dir             string // directory containing the item
	name            string // current base name
	isDir           bool
	section         bool // a numbered section -> title-only rename (legacy file rename)
	manifestChapter bool // a manifest chapter -> edit items[].title, filename birth-stable
	standaloneScene bool // a manifest v3 standalone scene -> edit items[].chapter.title, filename birth-stable
}
```

- [ ] **Step 5: Branch `startRename` on `e.isScene` before the chapter-entry check**

In `main.go:2565-2580`, replace:
```go
	if m.files.isChapterEntry(e) {
		if v.source == sourceManifest {
			// manifest manuscript: retitle the manifest entry; filename is birth-stable (§5.7).
			m.renamingInPane = true
			m.nameInput.Width = m.files.width
			m.beginRename(renameTarget{dir: m.files.dir, name: e.name, manifestChapter: true},
				m.files.chapterTitle(e.name))
			return
		}
		// legacy (manifest-less) folder: retain pre-manifest prefix-preserving retitle (O1).
		m.renamingInPane = true
		m.nameInput.Width = m.files.width
		m.beginRename(renameTarget{dir: m.files.dir, name: e.name, isDir: e.isDir, section: true},
			sectionTitle(e.name))
		return
	}
```
with:
```go
	if e.isScene {
		// standalone scene (manifest v3): retitle the manifest entry; filename is
		// birth-stable, same principle as a chapter but keyed by filename (no folder).
		m.renamingInPane = true
		m.nameInput.Width = m.files.width
		m.beginRename(renameTarget{dir: m.files.dir, name: e.name, standaloneScene: true}, e.name)
		return
	}
	if m.files.isChapterEntry(e) {
		if v.source == sourceManifest {
			// manifest manuscript: retitle the manifest entry; filename is birth-stable (§5.7).
			m.renamingInPane = true
			m.nameInput.Width = m.files.width
			m.beginRename(renameTarget{dir: m.files.dir, name: e.name, manifestChapter: true},
				m.files.chapterTitle(e.name))
			return
		}
		// legacy (manifest-less) folder: retain pre-manifest prefix-preserving retitle (O1).
		m.renamingInPane = true
		m.nameInput.Width = m.files.width
		m.beginRename(renameTarget{dir: m.files.dir, name: e.name, isDir: e.isDir, section: true},
			sectionTitle(e.name))
		return
	}
```

Note: the prefill value passed to `beginRename` for the scene case uses `e.name` (the raw
filename) rather than a resolved display title — check how `m.files.chapterTitle` resolves a
title and, if a scene-aware title lookup would be a better prefill (the field the user is
about to edit should probably show the CURRENT title, not the filename), extend
`filelist.chapterTitle` (`filelist.go:293-302`) to also match `ch.scene` entries by comparing
`ch.texts[0].file == name` — mirroring how `chapterWords`/`chapterTitle` already fall back to
`sectionTitle` for entries their `ch.folder == name` check can't reach. If you make that
change, use it here instead of the raw `e.name` prefill.

- [ ] **Step 6: Route `confirmRename` for a standalone-scene target**

In `main.go:2696-2705`, replace:
```go
	if t.manifestChapter {
		if err := renameChapterTitle(t.dir, t.name, typed); err != nil {
			m.status = "échec du retitrage : " + err.Error()
		} else {
			m.status = "retitré en " + typed
		}
		m.refreshAfterRename()
		return
	}
```
with:
```go
	if t.manifestChapter {
		if err := renameChapterTitle(t.dir, t.name, typed); err != nil {
			m.status = "échec du retitrage : " + err.Error()
		} else {
			m.status = "retitré en " + typed
		}
		m.refreshAfterRename()
		return
	}

	if t.standaloneScene {
		if err := renameSceneTitle(t.dir, t.name, typed); err != nil {
			m.status = "échec du retitrage : " + err.Error()
		} else {
			m.status = "retitré en " + typed
		}
		m.refreshAfterRename()
		return
	}
```

- [ ] **Step 7: Run tests to verify they pass**

Run: `go test . -run 'TestRenameSceneTitle|TestStartRenameOnStandaloneSceneTargetsManifestTitle' -v`
Expected: PASS.

- [ ] **Step 8: Run the full suite**

Run: `go test ./... 2>&1 | tail -30`
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add manifest.go main.go manifest_test.go rename_wiring_test.go
git commit -m "rename: retitle a standalone scene via the manifest, filename stays birth-stable"
```

---

## Task 7: Preserve `Scene` through the corkboard/structure-mode staged buffer

Discovered during codebase exploration for this plan (not explicitly called out at this
level of detail in the spec): `structure.go`'s `toManifestChapters` — used by
`commitStructure`, which both the corkboard (`ctrl+k`) and standalone structure mode commit
through — rebuilds each `manifestChapter` from the staged `chapterRef` buffer WITHOUT copying
`scene`. Since a standalone scene resolves into that same staged buffer (Task 3's `bare`
chapters, which `enterCorkboard` stages wholesale), reordering chapters on the corkboard and
committing would silently rewrite a standalone scene back into an ordinary
`manifestChapter{Folder: "", Scene: false, ...}` — indistinguishable from the legacy
flat-file fallback on the next read. This must be fixed before Task 8 (which also touches
`corkboard.go`), so both corkboard changes land together with confidence the buffer round-trips.

**Files:**
- Modify: `structure.go`
- Test: create `structure_test.go` (none exists yet)

**Interfaces:**
- Consumes: `chapterRef.scene` (Task 3), `toManifestChapters` (`structure.go:97-108`).
- Produces: `toManifestChapters` output preserves `Scene` for any staged `chapterRef` with
  `scene == true`.

- [ ] **Step 1: Write the failing test**

Create `structure_test.go`:
```go
package main

import "testing"

func TestToManifestChaptersPreservesScene(t *testing.T) {
	m := model{structureItems: []chapterRef{
		{folder: "un", title: "Un", texts: []textRef{{file: "un.md", title: "Un"}}},
		{title: "Aparté", scene: true, texts: []textRef{{file: "aparte.md", title: "Aparté"}}},
	}}
	out := m.toManifestChapters()
	if len(out) != 2 {
		t.Fatalf("want 2 manifestChapters, got %d", len(out))
	}
	if out[0].Scene {
		t.Fatalf("ordinary chapter must not become Scene:true, got %+v", out[0])
	}
	if !out[1].Scene {
		t.Fatalf("a staged standalone scene must round-trip Scene:true, got %+v", out[1])
	}
	if out[1].Folder != "" || out[1].Texts[0].File != "aparte.md" {
		t.Fatalf("standalone scene shape must be preserved, got %+v", out[1])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test . -run TestToManifestChaptersPreservesScene -v`
Expected: FAIL — `out[1].Scene` is `false` (the field isn't copied yet).

- [ ] **Step 3: Fix `toManifestChapters`**

In `structure.go:97-108`, replace:
```go
// toManifestChapters converts the staged bare-chapter buffer to manifest wire shape.
func (m model) toManifestChapters() []manifestChapter {
	out := make([]manifestChapter, 0, len(m.structureItems))
	for _, ch := range m.structureItems {
		texts := make([]manifestText, 0, len(ch.texts))
		for _, t := range ch.texts {
			texts = append(texts, manifestText{File: t.file, Title: t.title})
		}
		out = append(out, manifestChapter{Folder: ch.folder, Title: ch.title, Texts: texts})
	}
	return out
}
```
with:
```go
// toManifestChapters converts the staged bare-chapter buffer to manifest wire shape. A
// staged standalone scene (ch.scene) must round-trip Scene:true — otherwise committing a
// reorder that merely passes a scene through the buffer would silently demote it back to an
// ordinary (legacy-shaped) chapter on the next manifest read.
func (m model) toManifestChapters() []manifestChapter {
	out := make([]manifestChapter, 0, len(m.structureItems))
	for _, ch := range m.structureItems {
		texts := make([]manifestText, 0, len(ch.texts))
		for _, t := range ch.texts {
			texts = append(texts, manifestText{File: t.file, Title: t.title})
		}
		out = append(out, manifestChapter{Folder: ch.folder, Title: ch.title, Texts: texts, Scene: ch.scene})
	}
	return out
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test . -run TestToManifestChaptersPreservesScene -v`
Expected: PASS.

- [ ] **Step 5: Run the full suite**

Run: `go test ./... 2>&1 | tail -30`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add structure.go structure_test.go
git commit -m "structure: preserve Scene when a staged buffer commit rewrites the manifest"
```

---

## Task 8: Fix corkboard `enter` on a multi-scene chapter's card

**Files:**
- Modify: `corkboard.go`
- Test: `corkboard_test.go`

**Interfaces:**
- Consumes: `updateCorkboard`'s `"enter"` case (`corkboard.go:329-345`), `screenTextPicker`,
  `model.textPickerChapter`/`textPickerSel`/`textPickerDir` (`main.go:415-417`),
  `exitCorkboard` (`corkboard.go:358-367`).
- Produces: `enter` on a card with `len(ch.texts) >= 2` opens `screenTextPicker` instead of
  silently loading `texts[0]`; `len(ch.texts) <= 1` behavior is unchanged.

- [ ] **Step 1: Write the failing tests**

Append to `corkboard_test.go`:
```go
func seedCorkManuscriptWithMultiSceneChapter(t *testing.T) (dir string) {
	t.Helper()
	dir = t.TempDir()
	mkChapterDir(t, dir, "a", map[string]string{
		"a-1.md": "body of a-1",
		"a-2.md": "body of a-2",
	})
	if err := writeManifest(dir, manifest{
		SchemaVersion: manifestSchemaVersion,
		Title:         "The Work",
		Items: []manifestItem{
			{Chapter: &manifestChapter{Folder: "a", Title: "One", Texts: []manifestText{
				{File: "a-1.md", Title: "First"},
				{File: "a-2.md", Title: "Second"},
			}}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestCorkboardEnterOnMultiSceneChapterOpensTextPicker(t *testing.T) {
	dir := seedCorkManuscriptWithMultiSceneChapter(t)
	m := model{}
	m.files.dir = dir
	m.enterCorkboard()
	mm, _ := m.updateCorkboard(tea.KeyMsg{Type: tea.KeyEnter})
	m = mm.(model)
	if m.screen != screenTextPicker {
		t.Fatalf("enter on a 2-text chapter card must open the text picker, got screen=%v", m.screen)
	}
	if m.textPickerChapter == nil || m.textPickerChapter.folder != "a" || len(m.textPickerChapter.texts) != 2 {
		t.Fatalf("textPickerChapter = %+v", m.textPickerChapter)
	}
	if m.textPickerDir != dir {
		t.Fatalf("textPickerDir = %q, want %q", m.textPickerDir, dir)
	}
}

func TestCorkboardEnterOnSingleSceneChapterOpensDirectly(t *testing.T) {
	dir := seedCorkManuscript(t) // a, b, c — each single-text
	m := model{}
	m.files.dir = dir
	m.enterCorkboard()
	mm, _ := m.updateCorkboard(tea.KeyMsg{Type: tea.KeyEnter})
	m = mm.(model)
	if m.screen != screenWriting {
		t.Fatalf("enter on a 1-text chapter card must open it directly, got screen=%v", m.screen)
	}
	if m.currentFile != filepath.Join(dir, "a", "a.md") {
		t.Fatalf("currentFile = %q", m.currentFile)
	}
}

func TestCorkboardEnterOnEmptyChapterShowsStatus(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "empty"), 0o755)
	if err := writeManifest(dir, manifest{
		SchemaVersion: manifestSchemaVersion,
		Title:         "N",
		Items: []manifestItem{
			{Chapter: &manifestChapter{Folder: "empty", Title: "Vide", Texts: []manifestText{}}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	m := model{}
	m.files.dir = dir
	m.enterCorkboard()
	mm, _ := m.updateCorkboard(tea.KeyMsg{Type: tea.KeyEnter})
	m = mm.(model)
	if m.status != "ce chapitre n'a pas encore de scène" {
		t.Fatalf("status = %q", m.status)
	}
	if m.screen != screenCorkboard {
		t.Fatal("empty chapter must not leave the corkboard")
	}
}
```

Check `corkboard_test.go`'s existing imports include `path/filepath` and `os` (it already
does, per the earlier read of its header) — no new imports needed.

- [ ] **Step 2: Run tests to verify the new one fails**

Run: `go test . -run 'TestCorkboardEnterOnMultiSceneChapterOpensTextPicker|TestCorkboardEnterOnSingleSceneChapterOpensDirectly|TestCorkboardEnterOnEmptyChapterShowsStatus' -v`
Expected: `TestCorkboardEnterOnMultiSceneChapterOpensTextPicker` FAILS (today it silently opens
`a-1.md` directly instead of the text picker); the other two PASS already (they document
current, unchanged behavior — keep them as regression coverage).

- [ ] **Step 3: Fix the `enter` case**

In `corkboard.go:329-345`, replace:
```go
	case "enter":
		// Open the selected chapter; staged changes must be resolved first.
		if m.structureDirty {
			m.structureConfirm = true
			m.status = "appliquer les modifications d'abord — y appliquer · esc annuler"
		} else if m.structureSel < len(m.structureItems) {
			ch := m.structureItems[m.structureSel]
			if len(ch.texts) == 0 {
				m.status = "ce chapitre n'a pas encore de scène"
				return m, nil
			}
			file := filepath.Join(m.structureDir, ch.folder, ch.texts[0].file)
			m.exitCorkboard()
			m.loadFile(file)
			m.focus = focusEditor
			m.editor.Focus()
		}
```
with:
```go
	case "enter":
		// Open the selected chapter; staged changes must be resolved first.
		if m.structureDirty {
			m.structureConfirm = true
			m.status = "appliquer les modifications d'abord — y appliquer · esc annuler"
		} else if m.structureSel < len(m.structureItems) {
			ch := m.structureItems[m.structureSel]
			if len(ch.texts) == 0 {
				m.status = "ce chapitre n'a pas encore de scène"
				return m, nil
			}
			if len(ch.texts) >= 2 {
				// Multi-scene chapter: let the reader pick which scene, instead of always
				// silently opening the first one.
				chCopy := ch
				m.textPickerChapter = &chCopy
				m.textPickerSel = 0
				m.textPickerDir = m.structureDir
				m.exitCorkboard()
				m.screen = screenTextPicker
				return m, nil
			}
			file := filepath.Join(m.structureDir, ch.folder, ch.texts[0].file)
			m.exitCorkboard()
			m.loadFile(file)
			m.focus = focusEditor
			m.editor.Focus()
		}
```

Note: `exitCorkboard()` sets `m.screen = screenWriting` (see `corkboard.go:358-367`) — the
multi-scene branch calls it for its other side effects (clearing the staged buffer, reloading
`m.files`) and then overrides `m.screen` to `screenTextPicker` immediately after, exactly
mirroring how `enterTextPicker` (`main.go:178-188`) sets the same three `textPicker*` fields
from the sidebar path. `chCopy := ch` avoids taking the address of the loop/range variable
`ch` directly (it's a local copy from indexing `m.structureItems[m.structureSel]`, but making
the copy explicit keeps the intent obvious and matches Go vet's usual guidance for storing a
pointer to a value pulled out of a slice).

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test . -run 'TestCorkboardEnterOnMultiSceneChapterOpensTextPicker|TestCorkboardEnterOnSingleSceneChapterOpensDirectly|TestCorkboardEnterOnEmptyChapterShowsStatus' -v`
Expected: PASS.

- [ ] **Step 5: Run the full suite**

Run: `go test ./... 2>&1 | tail -30`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add corkboard.go corkboard_test.go
git commit -m "corkboard: enter on a multi-scene chapter opens the text picker instead of always the first scene"
```

---

## Task 9: Export — audit `folder == ""` handling for a standalone scene

The spec asserts this should need no logic changes, on the grounds that the legacy
`folder == ""` fallback already exercises every `filepath.Join(dir, chapterRef.folder,
text.file)` call site. This task verifies that claim with an actual export test using a
standalone scene, rather than trusting the assertion.

**Files:**
- Modify: none expected (verification task) — but see Step 3 for the contingency if the audit
  finds a real gap.
- Test: `export_ast_test.go`

**Interfaces:**
- Consumes: `manuscriptDocFromChapters` (`export_ast.go:241-257`), `chapterRef.scene` (Task 3).

- [ ] **Step 1: Write the test**

Append to `export_ast_test.go` (check its existing helpers for building a `[]partRef` fixture
— reuse whatever pattern the file already uses for a `manuscriptDocFromChapters` test, if one
exists; otherwise construct `partRef`/`chapterRef` directly as below):

```go
func TestManuscriptDocFromChaptersIncludesStandaloneScene(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "aparte.md"), []byte("Some aside text."), 0o644)
	parts := []partRef{{title: "", chapters: []chapterRef{
		{title: "Aparté", scene: true, texts: []textRef{{file: "aparte.md", title: "Aparté"}}},
	}}}
	doc := manuscriptDocFromChapters(dir, parts)
	if len(doc) != 1 {
		t.Fatalf("want 1 section, got %d: %+v", len(doc), doc)
	}
	if doc[0].Title != "Aparté" {
		t.Fatalf("section title = %q, want %q", doc[0].Title, "Aparté")
	}
	if len(doc[0].Blocks) == 0 {
		t.Fatal("standalone scene's content must have been read and parsed, got zero blocks")
	}
}
```

- [ ] **Step 2: Run the test**

Run: `go test . -run TestManuscriptDocFromChaptersIncludesStandaloneScene -v`
Expected: PASS. `manuscriptDocFromChapters` (`export_ast.go:249`) does
`filepath.Join(dir, ch.folder, ch.texts[0].file)` — with `ch.folder == ""`,
`filepath.Join(dir, "", "aparte.md")` correctly collapses to `filepath.Join(dir,
"aparte.md")` (Go's `filepath.Join` drops empty path elements), so this should pass with zero
production-code changes, confirming the spec's claim.

- [ ] **Step 3 (contingency, only if Step 2 fails): investigate and fix**

If the test fails, read the actual error — most likely candidates, in order of likelihood:
(a) `manuscriptDocFromChapters` reading `ch.texts[0]` only (a DIFFERENT, already-known bug —
see the export-selection design's separate fix, out of scope for THIS plan, which only
implements the standalone-scene spec); (b) some other export path
(`export_rtf.go`/`export_pdf.go`/etc.) constructing its own file path independently rather
than going through `ManuscriptDoc`. If it's (a), that's expected and fine — this plan's scope
is standalone scenes, not the multi-scene export bug (tracked separately in the
export-selection design's own plan). If it's (b), report back rather than guessing a fix — it
would mean the spec's audit claim was wrong for a site this plan didn't anticipate, and needs
a design decision, not a silent patch.

- [ ] **Step 4: Run the full suite**

Run: `go test ./... 2>&1 | tail -30`
Expected: PASS.

- [ ] **Step 5: Commit** (only if Step 1's test file changed — if Step 3's contingency fired
and required a production fix, include those files too)

```bash
git add export_ast_test.go
git commit -m "export: verify a standalone scene exports correctly (folder==\"\" path)"
```

---

## Task 10: Update CLAUDE.md per the spec's "Impact CLAUDE.md" section

**Files:**
- Modify: `CLAUDE.md`

**Interfaces:** none (documentation only).

- [ ] **Step 1: Add the Shared Contracts §1 note**

In `CLAUDE.md`, under the `### 1. Manuscript ordering & membership — RESOLVED (2026-06-26)`
section, add a paragraph (after the existing bullets, before the next `###` heading) noting:
this fork (forkashi) has no known companion macOS app consuming `manifest.json` (`git remote
-v` shows only `origin`/`scriptor-pro/forkashi` and `upstream`/`snackztime/okashi`, no Swift
consumer), so the HARD GATE requiring cross-repo coordination on manifest shape changes does
not apply here; `manifestSchemaVersion` moved from 2 to 3 on 2026-08-23 with the addition of
`manifestChapter.Scene`, a deliberate divergence from the upstream/companion schema.

- [ ] **Step 2: Add the Project model note**

Under `## Project model (the shipped reality)`, in the bullet describing `ctrl+n`'s
chapter/resource/scene picker, add that the scene option is now always offered — even with no
chapter selected in the sidebar, in which case it creates a **standalone scene**: a top-level
ordered item with no containing chapter folder, its file living at the manuscript root.

- [ ] **Step 3: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: document standalone scenes in CLAUDE.md"
```

---

## Self-Review Notes (for the plan author, already applied above)

- **Spec coverage:** every spec section has a task — data model (Task 2), resolver/display
  (Task 3-4), creation (Task 5), retitle (Task 6), export (Task 9), corkboard bug fix (Task
  8), CLAUDE.md (Task 10). The one gap the spec didn't anticipate at this detail level —
  `toManifestChapters` dropping `Scene` on a staged commit — got its own task (7) rather than
  being silently folded into Task 8, since it's a distinct data-loss risk with its own test.
- **Type consistency:** `chapterRef.scene`, `manifestChapter.Scene`, `fileEntry.isScene`,
  `renameTarget.standaloneScene` are named consistently with their existing siblings
  (`chapterRef.folder`, `manifestChapter.Folder`, `fileEntry.isDir`,
  `renameTarget.manifestChapter`) — lowercase unexported Go fields vs exported JSON-tagged
  fields, matching each type's existing convention.
- **Ambiguity resolved inline:** Task 6 flags and resolves an underspecified point in the
  spec (how `startRename` distinguishes a scene from other entries) rather than leaving it as
  a TODO — decided to key off `fileEntry.isScene` (Task 4) checked before the existing
  `isChapterEntry` branch, since `isChapterEntry` is now `false` for a scene by construction.
- **Deliberately out of scope, left in the design doc, not duplicated here:** the export
  multi-scene bug (`manuscriptDocFromChapters` reading only `texts[0]`) belongs to the
  export-selection design/plan, not this one — Task 9's contingency step calls this out
  explicitly so an implementer doesn't accidentally scope-creep into fixing it here.

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-08-23-standalone-scene.md`. Two
execution options:

1. **Subagent-Driven (recommended)** - I dispatch a fresh subagent per task, review between
   tasks, fast iteration
2. **Inline Execution** - Execute tasks in this session using executing-plans, batch
   execution with checkpoints

Which approach?
