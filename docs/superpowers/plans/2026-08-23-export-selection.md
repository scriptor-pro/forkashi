# Écran « tous les textes » (sélection d'export + déplacement) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A single screen (`v` from the sidebar/corkboard) listing every text in the current
manuscript — chapters, their scenes, standalone scenes, and Resources — each with a checkbox
controlling export inclusion, a 111-character preview, and the ability to move any line up/down
(including across chapter boundaries), while also fixing the preexisting export bug that only
ever reads a chapter's first scene.

**Architecture:** A new okashi-owned sidecar (`.okashi-export.json`, same family as
`.okashi-synopsis.json`) persists per-file export exclusions. A new screen
(`exportselect.go`) resolves the manuscript once on entry (I/O), builds a flat display list, and
renders a pure view — same pattern as `notes.go`'s `allNotesModel`/`allNotesView`. Movement
(`shift+↑/↓`) that crosses a chapter boundary reuses `safeMove` (already in `move.go`) plus a
manifest read-modify-write, applied immediately per keystroke (no staged buffer, unlike the
corkboard's structure mode). `manuscriptDocFromChapters` (export_ast.go) is fixed to iterate all
of a chapter's texts instead of only `texts[0]`, and `runExport` gains the exclusion filter plus
a new pass over checked Resources.

**Tech Stack:** Go 1.25, Bubble Tea (Elm-style Model/Update/View), no new dependencies.

**Spec:** `docs/superpowers/specs/2026-08-23-export-selection-design.md`

**Depends on:** `docs/superpowers/specs/2026-08-23-standalone-scene-design.md` and its plan
(`docs/superpowers/plans/2026-08-23-standalone-scene.md`) — this plan reads `chapterRef.scene`,
`manifestChapter.Scene`, and `findSceneByFile`, all delivered by that plan's Tasks 2-3. **Do not
start this plan's Task 3 onward until that plan's Tasks 1-6 show `complete` in its own ledger** —
earlier tasks here (1-2) only touch the new sidecar file and have no such dependency, so they
may proceed regardless.

## Global Constraints

- HARD GATE lifted for this fork (no companion macOS app) — confirmed already for the sibling
  standalone-scene plan; this plan's manifest writes reuse the existing v3 shape (retrofit
  `Texts[]`/`items[]` entries), introducing no new manifest field.
- The sidecar stores **exclusions only** (`Excluded map[string]bool`, keyed by file path
  relative to the manuscript dir) — default-included, so an untouched manuscript exports exactly
  as it does today.
- A checkbox toggle (`espace`) and a move (`shift+↑/↓`) both persist **immediately** — no staged
  buffer, no commit-on-exit (explicit choice, differs from the corkboard's structure mode).
- Character counts use rune count (`utf8.RuneCountInString`), not byte length — French text has
  accented characters `len()` would miscount.
- Word/char totals use the existing `commafy` helper (`main.go:2897`, thousands via `,` — e.g.
  `1,234`), NOT a space-separated format. The spec's mockup shows `1 234 mots` with a French
  space separator; `commafy` is the ONLY thousands-formatter in this codebase and is used
  everywhere else (corkboard, sidebar, goals) with `,`. Introducing a second, differently-styled
  number formatter for one screen would read as inconsistent with the rest of the app. **Ruling:**
  this plan uses `commafy` (comma-separated) for both totals, overriding the spec mockup's literal
  formatting — the spec's intent (a readable thousands-grouped total) is preserved, only the
  separator glyph changes to match the rest of the app.
- Every new status message stays in French, matching the existing tone (`createScene`,
  `createStandaloneScene`, `renameSceneTitle`).
- Move that crosses a chapter boundary uses `safeMove` (`move.go:128`), not a bare `os.Rename` —
  `safeMove` already handles the cross-device fallback the spec's `os.Rename` mention doesn't
  account for; same guarantee (same-manuscript moves are same-volume) with better failure
  handling for free.

---

## File Structure

- **New `exportselection.go`**: the sidecar — `exportSelectionFile` type,
  `loadExportSelection`/`saveExportSelection`, `exportSelectionPath`.
- **New `exportselect.go`**: the screen — `exportSelectEntry`/`exportSelectModel`,
  `enterExportSelect`, `updateExportSelect`, `exportSelectView`, `charCount`, and the move logic
  (`moveExportSelectEntry` or equivalent, doing the cross-boundary file move + manifest rewrite).
- **Modify `main.go`**: `screenExportSelect` constant; `model.exportSelect exportSelectModel`
  field; `v` key wiring from the sidebar-focused block (`main.go:1835-1874`); `Update`/`View`
  dispatch (mirroring the `screenAllNotes` dispatch at `main.go:1220-1222` and `1995-1997`).
- **Modify `export_ast.go`**: `manuscriptDocFromChapters` gains an `excluded map[string]bool`
  parameter and iterates every text, not just `texts[0]`.
- **Modify `export.go`**: `runExport` loads the sidecar and passes it through; adds a Resources
  pass.
- **Test files**: `exportselection_test.go` (new), `exportselect_test.go` (new),
  `export_ast_test.go` (modified — signature change ripples to any existing caller in tests),
  `export_wiring_test.go` (modified — new Resources-in-export coverage).

---

## Task 1: Export-selection sidecar (`.okashi-export.json`)

No dependency on the standalone-scene plan — this task only deals with a path string and a
map, agnostic to what the path represents.

**Files:**
- Create: `exportselection.go`
- Test: `exportselection_test.go`

**Interfaces:**
- Produces: `exportSelectionPath(dir string) string`; `loadExportSelection(dir string)
  map[string]bool`; `saveExportSelection(dir string, excluded map[string]bool, knownFiles
  map[string]bool) error`.

- [ ] **Step 1: Write the failing tests**

Create `exportselection_test.go`:
```go
package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadExportSelectionAbsentFileYieldsEmptyMap(t *testing.T) {
	dir := t.TempDir()
	got := loadExportSelection(dir)
	if len(got) != 0 {
		t.Fatalf("want empty map for a missing sidecar, got %+v", got)
	}
}

func TestLoadExportSelectionCorruptFileYieldsEmptyMap(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(exportSelectionPath(dir), []byte("{ not json"), 0o644)
	got := loadExportSelection(dir)
	if len(got) != 0 {
		t.Fatalf("want empty map for a corrupt sidecar, got %+v", got)
	}
}

func TestLoadExportSelectionUnsupportedSchemaYieldsEmptyMap(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(exportSelectionPath(dir), []byte(`{"schemaVersion":99,"excluded":{"a.md":true}}`), 0o644)
	got := loadExportSelection(dir)
	if len(got) != 0 {
		t.Fatalf("want empty map for an unsupported schema, got %+v", got)
	}
}

func TestSaveExportSelectionRoundTrips(t *testing.T) {
	dir := t.TempDir()
	excluded := map[string]bool{"draft.md": true, "chapter-1/scene-2.md": true}
	known := map[string]bool{"draft.md": true, "chapter-1/scene-2.md": true, "chapter-1/scene-1.md": true}
	if err := saveExportSelection(dir, excluded, known); err != nil {
		t.Fatal(err)
	}
	got := loadExportSelection(dir)
	if len(got) != 2 || !got["draft.md"] || !got["chapter-1/scene-2.md"] {
		t.Fatalf("round-trip = %+v, want the 2 saved exclusions", got)
	}
}

func TestSaveExportSelectionPrunesOrphanKeys(t *testing.T) {
	dir := t.TempDir()
	// "gone.md" is excluded but no longer a known file (deleted/renamed) — must be pruned.
	excluded := map[string]bool{"gone.md": true, "kept.md": true}
	known := map[string]bool{"kept.md": true}
	if err := saveExportSelection(dir, excluded, known); err != nil {
		t.Fatal(err)
	}
	got := loadExportSelection(dir)
	if len(got) != 1 || !got["kept.md"] {
		t.Fatalf("want only kept.md to survive pruning, got %+v", got)
	}
}

func TestSaveExportSelectionIsAtomic(t *testing.T) {
	dir := t.TempDir()
	if err := saveExportSelection(dir, map[string]bool{"a.md": true}, map[string]bool{"a.md": true}); err != nil {
		t.Fatal(err)
	}
	// No stray temp files left behind in the manuscript dir (atomicWrite's dot-prefixed temp
	// file must not survive a successful write — same guarantee writeManifest/saveSynopses rely on).
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != exportSelectionName {
			t.Fatalf("unexpected leftover file in manuscript dir: %s", e.Name())
		}
	}
}

func TestExportSelectionPathIsDotPrefixedSidecar(t *testing.T) {
	dir := t.TempDir()
	got := exportSelectionPath(dir)
	want := filepath.Join(dir, ".okashi-export.json")
	if got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test . -run 'TestLoadExportSelection|TestSaveExportSelection|TestExportSelectionPath' -v`
Expected: FAIL — `exportselection.go` doesn't exist yet (compile error).

- [ ] **Step 3: Write the sidecar**

Create `exportselection.go`:
```go
package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
)

const exportSelectionName = ".okashi-export.json"
const exportSelectionSchemaVersion = 1

// exportSelectionFile is the per-manuscript export-inclusion sidecar (okashi-owned, not the
// manifest). Keyed by file path relative to the manuscript dir. A key present means the file is
// EXCLUDED from export; absence means included — default-included, so an untouched manuscript
// exports exactly as it does today.
type exportSelectionFile struct {
	SchemaVersion int             `json:"schemaVersion"`
	Excluded      map[string]bool `json:"excluded"`
}

func exportSelectionPath(dir string) string { return filepath.Join(dir, exportSelectionName) }

// loadExportSelection reads the sidecar; missing/corrupt/unsupported-schema all yield an empty
// map — never an error (mirrors loadSynopses's tolerant load).
func loadExportSelection(dir string) map[string]bool {
	out := map[string]bool{}
	data, err := os.ReadFile(exportSelectionPath(dir))
	if err != nil {
		return out
	}
	var ef exportSelectionFile
	if json.Unmarshal(data, &ef) != nil || ef.SchemaVersion != exportSelectionSchemaVersion {
		return out
	}
	if ef.Excluded != nil {
		out = ef.Excluded
	}
	return out
}

// saveExportSelection writes the sidecar atomically. It prunes keys not in knownFiles
// (self-healing orphans left by a removed/renamed/moved file), so the file only ever holds
// live exclusions. Serialized like saveSynopses: 2-space indent, no HTML escaping, no trailing
// newline.
func saveExportSelection(dir string, excluded map[string]bool, knownFiles map[string]bool) error {
	pruned := map[string]bool{}
	for file, ex := range excluded {
		if ex && knownFiles[file] {
			pruned[file] = true
		}
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(exportSelectionFile{SchemaVersion: exportSelectionSchemaVersion, Excluded: pruned}); err != nil {
		return err
	}
	return atomicWrite(exportSelectionPath(dir), bytes.TrimRight(buf.Bytes(), "\n"), 0o644)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test . -run 'TestLoadExportSelection|TestSaveExportSelection|TestExportSelectionPath' -v`
Expected: PASS.

- [ ] **Step 5: Run the full suite**

Run: `go test ./... 2>&1 | tail -30`
Expected: PASS. (One known preexisting flake, `TestSnippetCacheReadsHeadAndInvalidates`, may
fail under full parallel load — confirmed unrelated to this plan; if it's the ONLY failure,
re-run `go test . -run TestSnippetCacheReadsHeadAndInvalidates -count=3 -v` to confirm it
passes in isolation, then proceed.)

- [ ] **Step 6: Commit**

```bash
git add exportselection.go exportselection_test.go
git commit -m "export: add .okashi-export.json sidecar for per-text export exclusion"
```

---

## Task 2: `charCount` helper

Independent of everything else — a pure function.

**Files:**
- Modify: `main.go` (add near `wordCount`, `main.go:2892`)
- Test: `main_test.go`

**Interfaces:**
- Produces: `charCount(s string) int`.

- [ ] **Step 1: Write the failing test**

Append to `main_test.go`:
```go
func TestCharCountCountsRunesNotBytes(t *testing.T) {
	// "café" is 4 runes but 5 bytes (é is 2 bytes in UTF-8) — len() would return 5.
	if got := charCount("café"); got != 4 {
		t.Fatalf("charCount(\"café\") = %d, want 4 (rune count, not byte length)", got)
	}
	if got := charCount("a b c"); got != 5 {
		t.Fatalf("charCount(\"a b c\") = %d, want 5 (spaces counted)", got)
	}
	if got := charCount(""); got != 0 {
		t.Fatalf("charCount(\"\") = %d, want 0", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test . -run TestCharCountCountsRunesNotBytes -v`
Expected: FAIL — `charCount` doesn't exist (compile error).

- [ ] **Step 3: Add `charCount`**

In `main.go`, immediately after `wordCount` (`main.go:2892-2894`):
```go
// charCount returns the rune count of s, spaces included — used by the export-selection
// screen's character total. utf8.RuneCountInString, not len(s): len would count bytes, wrong
// for accented French text.
func charCount(s string) int {
	return utf8.RuneCountInString(s)
}
```

Check `main.go`'s import block already has `"unicode/utf8"` — if not, add it (run `goimports`
or add it manually to the existing `import (...)` block at the top of the file).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test . -run TestCharCountCountsRunesNotBytes -v`
Expected: PASS.

- [ ] **Step 5: Run the full suite**

Run: `go test ./... 2>&1 | tail -30`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add main.go main_test.go
git commit -m "editor: add charCount (rune count) alongside wordCount"
```

---

## Task 3: Fix the multi-scene export bug (`manuscriptDocFromChapters`)

**Depends on the standalone-scene plan's Tasks 1-3 being complete** (needs `chapterRef.scene`
to exist so a standalone scene, `folder == ""`, is exercised correctly by this fix — though the
fix itself only touches the loop bound, the test fixture for a standalone scene needs that
field). If those tasks are not yet `complete` in `docs/superpowers/plans/2026-08-23-standalone-scene.md`'s
ledger, either wait or skip the standalone-scene-specific test case in Step 1 and add it in a
follow-up — do not block this task's core fix on that dependency, only its full test coverage.

**Files:**
- Modify: `export_ast.go`
- Test: `export_ast_test.go`

**Interfaces:**
- Consumes: `partRef`/`chapterRef` (`manuscript.go`), `Section`/`ManuscriptDoc`/`parseSection`
  (`export_ast.go`).
- Produces: `manuscriptDocFromChapters(dir string, parts []partRef, excluded map[string]bool)
  ManuscriptDoc` — signature CHANGES (gains the `excluded` parameter) — every call site must be
  updated in this same task.

- [ ] **Step 1: Write the failing tests**

Append to `export_ast_test.go`:
```go
func TestManuscriptDocFromChaptersIncludesAllScenes(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "ch1"), 0o755)
	os.WriteFile(filepath.Join(dir, "ch1", "scene-1.md"), []byte("First scene."), 0o644)
	os.WriteFile(filepath.Join(dir, "ch1", "scene-2.md"), []byte("Second scene."), 0o644)
	parts := []partRef{{title: "", chapters: []chapterRef{
		{folder: "ch1", title: "Chapter One", texts: []textRef{
			{file: "scene-1.md", title: "Opening"},
			{file: "scene-2.md", title: "Confrontation"},
		}},
	}}}
	doc := manuscriptDocFromChapters(dir, parts, nil)
	if len(doc) != 2 {
		t.Fatalf("want 2 sections (one per scene), got %d: %+v", len(doc), doc)
	}
	if doc[0].Title != "Opening" || doc[1].Title != "Confrontation" {
		t.Fatalf("section titles = %q, %q — want scene titles in order", doc[0].Title, doc[1].Title)
	}
}

func TestManuscriptDocFromChaptersRespectsExclusion(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "ch1"), 0o755)
	os.WriteFile(filepath.Join(dir, "ch1", "scene-1.md"), []byte("Kept."), 0o644)
	os.WriteFile(filepath.Join(dir, "ch1", "scene-2.md"), []byte("Excluded."), 0o644)
	parts := []partRef{{title: "", chapters: []chapterRef{
		{folder: "ch1", title: "Chapter One", texts: []textRef{
			{file: "scene-1.md", title: "Kept"},
			{file: "scene-2.md", title: "Dropped"},
		}},
	}}}
	excluded := map[string]bool{filepath.Join("ch1", "scene-2.md"): true}
	doc := manuscriptDocFromChapters(dir, parts, excluded)
	if len(doc) != 1 || doc[0].Title != "Kept" {
		t.Fatalf("want only the non-excluded scene, got %+v", doc)
	}
}

func TestManuscriptDocFromChaptersEmptyChapterStillProducesSection(t *testing.T) {
	// Preserves the pre-existing behavior: a chapter with zero texts still gets a Section
	// with nil Blocks, rather than being silently skipped.
	parts := []partRef{{title: "", chapters: []chapterRef{
		{folder: "empty", title: "Vide", texts: nil},
	}}}
	doc := manuscriptDocFromChapters(t.TempDir(), parts, nil)
	if len(doc) != 1 || doc[0].Title != "Vide" || doc[0].Blocks != nil {
		t.Fatalf("empty chapter = %+v, want 1 section titled Vide with nil Blocks", doc)
	}
}

func TestManuscriptDocFromChaptersStandaloneSceneReadsFromRoot(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "aparte.md"), []byte("Some aside text."), 0o644)
	parts := []partRef{{title: "", chapters: []chapterRef{
		{title: "Aparté", scene: true, texts: []textRef{{file: "aparte.md", title: "Aparté"}}},
	}}}
	doc := manuscriptDocFromChapters(dir, parts, nil)
	if len(doc) != 1 || doc[0].Title != "Aparté" || len(doc[0].Blocks) == 0 {
		t.Fatalf("standalone scene doc = %+v, want 1 non-empty section", doc)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test . -run TestManuscriptDocFromChapters -v`
Expected: FAIL — current signature takes 2 args, tests call it with 3 (compile error); once
fixed to compile, `TestManuscriptDocFromChaptersIncludesAllScenes` fails because only
`texts[0]` is read today.

- [ ] **Step 3: Fix `manuscriptDocFromChapters`**

In `export_ast.go:233-257`, replace:
```go
// manuscriptDocFromChapters builds the export doc from a resolved parts list,
// flattened across parts (Part headers are the next plan's job — this plan
// keeps export mechanically correct but visually flat). Title comes from
// chapterRef.title (manifest title or de-slugged filename), so manifest and
// legacy folders both produce correct section headings. Only the first text of
// each chapter is read (degraded-but-functional multi-text behavior; real
// concatenation is the next plan's job); an empty chapter (no texts) produces a
// Section with empty Blocks rather than being skipped or panicking.
func manuscriptDocFromChapters(dir string, parts []partRef) ManuscriptDoc {
	var doc ManuscriptDoc
	for _, p := range parts {
		for _, ch := range p.chapters {
			if len(ch.texts) == 0 {
				doc = append(doc, Section{Title: ch.title, Blocks: nil})
				continue
			}
			data, err := os.ReadFile(filepath.Join(dir, ch.folder, ch.texts[0].file))
			if err != nil {
				continue
			}
			doc = append(doc, Section{Title: ch.title, Blocks: parseSection(data)})
		}
	}
	return doc
}
```
with:
```go
// manuscriptDocFromChapters builds the export doc from a resolved parts list, flattened across
// parts (Part headers are the next plan's job — this plan keeps export mechanically correct but
// visually flat). Every text of every chapter is read — each scene becomes its own Section,
// titled with the SCENE's title (not the chapter's), a deliberate granularity change from the
// previous "first scene only" behavior. excluded is keyed by file path relative to dir
// (filepath.Join(ch.folder, text.file) — ch.folder == "" for a standalone scene, collapsing
// correctly via filepath.Join's empty-element handling); nil excluded means nothing is excluded.
// An empty chapter (no texts) produces a Section with empty Blocks rather than being skipped.
func manuscriptDocFromChapters(dir string, parts []partRef, excluded map[string]bool) ManuscriptDoc {
	var doc ManuscriptDoc
	for _, p := range parts {
		for _, ch := range p.chapters {
			if len(ch.texts) == 0 {
				doc = append(doc, Section{Title: ch.title, Blocks: nil})
				continue
			}
			for _, t := range ch.texts {
				file := filepath.Join(ch.folder, t.file)
				if excluded[file] {
					continue
				}
				data, err := os.ReadFile(filepath.Join(dir, file))
				if err != nil {
					continue
				}
				doc = append(doc, Section{Title: t.title, Blocks: parseSection(data)})
			}
		}
	}
	return doc
}
```

- [ ] **Step 4: Update the one call site**

In `export.go` (find with `grep -n manuscriptDocFromChapters export.go`), the current call
`doc = manuscriptDocFromChapters(dir, v.parts)` needs its new `excluded` argument. This task
does NOT yet load the sidecar (that's Task 5) — pass `nil` for now to keep this task's scope to
the export-doc-building fix alone:
```go
doc = manuscriptDocFromChapters(dir, v.parts, nil)
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test . -run TestManuscriptDocFromChapters -v`
Expected: PASS.

- [ ] **Step 6: Run the full suite**

Run: `go test ./... 2>&1 | tail -30`
Expected: PASS. Watch specifically for any OTHER test asserting the old "only first scene"
behavior via `runExport`/`m.exportWholeManuscript()` — if one exists (search: `grep -rn
"texts\[0\]" export_wiring_test.go export_ast_test.go`), it tested the bug, not a feature; update
it to expect all scenes, and note this in your task report.

- [ ] **Step 7: Commit**

```bash
git add export_ast.go export.go export_ast_test.go
git commit -m "export: include every scene of a chapter, not just the first (fixes silent data loss)"
```

---

## Task 4: The export-selection screen — read-only list (no checkbox, no move yet)

**Depends on the standalone-scene plan's Tasks 1-6 being complete** (needs `chapterRef.scene`,
`manifestChapter.Scene` for correct display-list construction). Confirm
`docs/superpowers/plans/2026-08-23-standalone-scene.md`'s ledger shows Tasks 1-6 `complete`
before starting.

This task builds the screen's data model and view WITHOUT the checkbox-toggle or move
mechanics — those are Tasks 5 and 7, so this task's diff and review stay a manageable size.

**Files:**
- Create: `exportselect.go`
- Test: `exportselect_test.go`
- Modify: `main.go` (screen constant, model field, key wiring, Update/View dispatch)

**Interfaces:**
- Consumes: `manuscriptView`/`partRef`/`chapterRef`/`textRef` (`manuscript.go`),
  `loadExportSelection` (Task 1), `charCount`/`wordCount`/`commafy` (`main.go`),
  `resolveManuscript`/`readEntries`.
- Produces: `exportSelectEntry` struct; `exportSelectModel` struct; `(m *model)
  enterExportSelect()`; `(m model) updateExportSelect(msg tea.Msg) (tea.Model, tea.Cmd)`;
  `exportSelectView(m model) string`; `screenExportSelect` screen constant; `model.exportSelect
  exportSelectModel` field.

- [ ] **Step 1: Write the failing tests**

Create `exportselect_test.go`:
```go
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func seedExportSelectManuscript(t *testing.T) (dir string) {
	t.Helper()
	dir = t.TempDir()
	os.MkdirAll(filepath.Join(dir, "ch1"), 0o755)
	os.WriteFile(filepath.Join(dir, "ch1", "scene-1.md"), []byte(strings.Repeat("word ", 30)), 0o644)
	os.WriteFile(filepath.Join(dir, "ch1", "scene-2.md"), []byte("short scene two"), 0o644)
	os.WriteFile(filepath.Join(dir, "aparte.md"), []byte("A standalone scene's text."), 0o644)
	os.WriteFile(filepath.Join(dir, "notes.md"), []byte("Research notes, a Resource."), 0o644)
	if err := writeManifest(dir, manifest{
		Title: "N",
		Items: []manifestItem{
			{Chapter: &manifestChapter{Folder: "ch1", Title: "Chapter One", Texts: []manifestText{
				{File: "scene-1.md", Title: "Opening"},
				{File: "scene-2.md", Title: "Confrontation"},
			}}},
			{Chapter: &manifestChapter{Title: "Aparté", Scene: true,
				Texts: []manifestText{{File: "aparte.md", Title: "Aparté"}}}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestEnterExportSelectBuildsOrderedList(t *testing.T) {
	dir := seedExportSelectManuscript(t)
	m := model{}
	m.files.dir = dir
	m.enterExportSelect()
	if m.screen != screenExportSelect {
		t.Fatalf("screen = %v, want screenExportSelect", m.screen)
	}
	entries := m.exportSelect.entries
	// Order: chapter header, its 2 scenes (indented), the standalone scene, then the Resource.
	if len(entries) != 5 {
		t.Fatalf("want 5 entries (1 header + 2 scenes + 1 standalone scene + 1 resource), got %d: %+v", len(entries), entries)
	}
	if !entries[0].isHeader || entries[0].title != "Chapter One" {
		t.Fatalf("entry 0 = %+v, want chapter header 'Chapter One'", entries[0])
	}
	if entries[1].isHeader || !entries[1].indent || entries[1].title != "Opening" {
		t.Fatalf("entry 1 = %+v, want indented scene 'Opening'", entries[1])
	}
	if entries[2].isHeader || !entries[2].indent || entries[2].title != "Confrontation" {
		t.Fatalf("entry 2 = %+v, want indented scene 'Confrontation'", entries[2])
	}
	if entries[3].isHeader || entries[3].indent || entries[3].title != "Aparté" {
		t.Fatalf("entry 3 = %+v, want non-indented standalone scene 'Aparté'", entries[3])
	}
	if entries[4].isHeader || entries[4].indent || entries[4].title != "notes" {
		t.Fatalf("entry 4 = %+v, want Resource 'notes' (de-slugged filename)", entries[4])
	}
}

func TestEnterExportSelectComputesWordsAndCharsAndPreview(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "ch1"), 0o755)
	os.WriteFile(filepath.Join(dir, "ch1", "a.md"), []byte("one two three"), 0o644)
	if err := writeManifest(dir, manifest{
		Title: "N",
		Items: []manifestItem{
			{Chapter: &manifestChapter{Folder: "ch1", Title: "C1", Texts: []manifestText{{File: "a.md", Title: "A"}}}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	m := model{}
	m.files.dir = dir
	m.enterExportSelect()
	var sceneEntry *exportSelectEntry
	for i := range m.exportSelect.entries {
		if m.exportSelect.entries[i].title == "A" {
			sceneEntry = &m.exportSelect.entries[i]
		}
	}
	if sceneEntry == nil {
		t.Fatal("scene entry 'A' not found")
	}
	if sceneEntry.words != 3 {
		t.Fatalf("words = %d, want 3", sceneEntry.words)
	}
	if sceneEntry.chars != charCount("one two three") {
		t.Fatalf("chars = %d, want %d", sceneEntry.chars, charCount("one two three"))
	}
	if sceneEntry.preview != "one two three" {
		t.Fatalf("preview = %q, want %q (shorter than 111 chars, shown whole)", sceneEntry.preview, "one two three")
	}
}

func TestEnterExportSelectPreviewTruncatesAt111CharsAndNormalizesWhitespace(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "ch1"), 0o755)
	long := strings.Repeat("a", 150)
	os.WriteFile(filepath.Join(dir, "ch1", "a.md"), []byte("line one\nline two "+long), 0o644)
	if err := writeManifest(dir, manifest{
		Title: "N",
		Items: []manifestItem{
			{Chapter: &manifestChapter{Folder: "ch1", Title: "C1", Texts: []manifestText{{File: "a.md", Title: "A"}}}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	m := model{}
	m.files.dir = dir
	m.enterExportSelect()
	var sceneEntry *exportSelectEntry
	for i := range m.exportSelect.entries {
		if m.exportSelect.entries[i].title == "A" {
			sceneEntry = &m.exportSelect.entries[i]
		}
	}
	if sceneEntry == nil {
		t.Fatal("scene entry 'A' not found")
	}
	if strings.Contains(sceneEntry.preview, "\n") {
		t.Fatalf("preview must have internal newlines collapsed to spaces, got %q", sceneEntry.preview)
	}
	runes := []rune(sceneEntry.preview)
	if len(runes) > 112 { // 111 chars + possible "…" = 112 runes max
		t.Fatalf("preview too long: %d runes, want <= 112 (111 + ellipsis)", len(runes))
	}
	if !strings.HasSuffix(sceneEntry.preview, "…") {
		t.Fatalf("preview of a text longer than 111 chars must end with an ellipsis, got %q", sceneEntry.preview)
	}
}

func TestEnterExportSelectLoadsExistingExclusions(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "ch1"), 0o755)
	os.WriteFile(filepath.Join(dir, "ch1", "a.md"), []byte("x"), 0o644)
	if err := writeManifest(dir, manifest{
		Title: "N",
		Items: []manifestItem{
			{Chapter: &manifestChapter{Folder: "ch1", Title: "C1", Texts: []manifestText{{File: "a.md", Title: "A"}}}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := saveExportSelection(dir, map[string]bool{filepath.Join("ch1", "a.md"): true},
		map[string]bool{filepath.Join("ch1", "a.md"): true}); err != nil {
		t.Fatal(err)
	}
	m := model{}
	m.files.dir = dir
	m.enterExportSelect()
	var sceneEntry *exportSelectEntry
	for i := range m.exportSelect.entries {
		if m.exportSelect.entries[i].title == "A" {
			sceneEntry = &m.exportSelect.entries[i]
		}
	}
	if sceneEntry == nil || !sceneEntry.excluded {
		t.Fatalf("entry = %+v, want excluded=true (loaded from sidecar)", sceneEntry)
	}
}

func TestExportSelectViewShowsFooterTotals(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "ch1"), 0o755)
	os.WriteFile(filepath.Join(dir, "ch1", "a.md"), []byte("one two three"), 0o644)
	if err := writeManifest(dir, manifest{
		Title: "N",
		Items: []manifestItem{
			{Chapter: &manifestChapter{Folder: "ch1", Title: "C1", Texts: []manifestText{{File: "a.md", Title: "A"}}}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	m := model{width: 80, height: 24}
	m.files.dir = dir
	m.enterExportSelect()
	view := exportSelectView(m)
	if !strings.Contains(view, "3") { // word count
		t.Fatalf("view must show the word total, got:\n%s", view)
	}
}

func TestUpdateExportSelectMovesSelection(t *testing.T) {
	dir := seedExportSelectManuscript(t)
	m := model{}
	m.files.dir = dir
	m.enterExportSelect()
	if m.exportSelect.sel != 0 {
		t.Fatalf("initial sel = %d, want 0", m.exportSelect.sel)
	}
	mm, _ := m.updateExportSelect(downKey())
	m2 := mm.(model)
	if m2.exportSelect.sel != 1 {
		t.Fatalf("sel after down = %d, want 1", m2.exportSelect.sel)
	}
}

func TestUpdateExportSelectEscReturnsToWriting(t *testing.T) {
	dir := seedExportSelectManuscript(t)
	m := model{}
	m.files.dir = dir
	m.enterExportSelect()
	mm, _ := m.updateExportSelect(escKey())
	m2 := mm.(model)
	if m2.screen != screenWriting {
		t.Fatalf("screen after esc = %v, want screenWriting", m2.screen)
	}
}
```

`downKey()`/`escKey()` are small `tea.KeyMsg` helpers — check whether equivalent helpers
already exist elsewhere in the test suite (search: `grep -rn "func downKey\|func escKey"
*_test.go`). If none exist, add them at the bottom of `exportselect_test.go`:
```go
func downKey() tea.Msg { return tea.KeyMsg{Type: tea.KeyDown} }
func escKey() tea.Msg  { return tea.KeyMsg{Type: tea.KeyEsc} }
```
(add `tea "github.com/charmbracelet/bubbletea"` to the file's imports if using this fallback).

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test . -run 'TestEnterExportSelect|TestExportSelectView|TestUpdateExportSelect' -v`
Expected: FAIL — `exportSelectEntry`, `enterExportSelect`, etc. don't exist yet (compile error).

- [ ] **Step 3: Write the screen's data model and resolution**

Create `exportselect.go`:
```go
package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// exportSelectEntry is one flattened row on the "all texts" screen: a chapter header, an
// indented scene, a standalone scene, or a Resource.
type exportSelectEntry struct {
	file     string // path relative to the manuscript dir — the sidecar's exclusion key
	title    string
	preview  string // first 111 chars, whitespace-normalized, ellipsis if truncated
	words    int
	chars    int  // runes, spaces included
	excluded bool
	indent   bool // true for a scene inside a chapter (rendered indented)
	isHeader bool // true for a chapter header row (its own words/chars don't count — its child scenes do)
}

type exportSelectModel struct {
	entries []exportSelectEntry
	sel     int
}

// previewOf returns the first 111 runes of data's content, internal whitespace collapsed to
// single spaces, with a trailing "…" if the source was longer than 111 runes.
func previewOf(data []byte) string {
	fields := strings.Fields(string(data)) // splits on any whitespace, drops runs
	joined := strings.Join(fields, " ")
	runes := []rune(joined)
	if len(runes) <= 111 {
		return joined
	}
	return string(runes[:111]) + "…"
}

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
				out = append(out, buildEntry(dir, rel, t.title, true, excluded))
			}
		}
	}
	for _, part := range v.parts {
		for _, ch := range part.chapters {
			if !ch.scene || len(ch.texts) == 0 {
				continue
			}
			rel := ch.texts[0].file
			out = append(out, buildEntry(dir, rel, ch.title, false, excluded))
		}
	}
	for _, e := range v.loose {
		out = append(out, buildEntry(dir, e.name, sectionTitle(e.name), false, excluded))
	}
	return out
}

// buildEntry reads rel's content once to compute words/chars/preview and reports its excluded
// state from the sidecar map.
func buildEntry(dir, rel, title string, indent bool, excluded map[string]bool) exportSelectEntry {
	data, _ := os.ReadFile(filepath.Join(dir, rel)) // best-effort: an unreadable file gets zero counts, not a crash
	text := string(data)
	return exportSelectEntry{
		file:     rel,
		title:    title,
		preview:  previewOf(data),
		words:    wordCount(text),
		chars:    charCount(text),
		excluded: excluded[rel],
		indent:   indent,
	}
}

// enterExportSelect resolves the current manuscript into the flat display list and shows the
// screen. All I/O happens here, once, before m.screen flips — exportSelectView stays I/O-free.
func (m *model) enterExportSelect() {
	dir := m.files.dir
	v := resolveManuscript(dir, readEntries(dir))
	excluded := loadExportSelection(dir)
	m.exportSelect = exportSelectModel{entries: buildExportSelectEntries(dir, v, excluded)}
	m.screen = screenExportSelect
}

func (m model) updateExportSelect(msg tea.Msg) (tea.Model, tea.Cmd) {
	if sz, ok := msg.(tea.WindowSizeMsg); ok {
		m.width, m.height = sz.Width, sz.Height
		return m, nil
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.screen = screenWriting
		m.focus = focusSidebar
	case "up", "k":
		if m.exportSelect.sel > 0 {
			m.exportSelect.sel--
		}
	case "down", "j":
		if m.exportSelect.sel < len(m.exportSelect.entries)-1 {
			m.exportSelect.sel++
		}
	}
	return m, nil
}

// exportSelectTotals sums words/chars across every non-header, non-excluded entry.
func exportSelectTotals(entries []exportSelectEntry) (words, chars int) {
	for _, e := range entries {
		if e.isHeader || e.excluded {
			continue
		}
		words += e.words
		chars += e.chars
	}
	return words, chars
}

func exportSelectView(m model) string {
	a := m.exportSelect
	header := lipgloss.NewStyle().Foreground(accent).Bold(true).Render("── tous les textes ")

	var rows []string
	if len(a.entries) == 0 {
		rows = append(rows, lipgloss.NewStyle().Foreground(subtle).Render("  (aucun texte dans ce manuscrit)"))
	} else {
		width := max(10, min(m.width-8, 72))
		for i, e := range a.entries {
			box := "[ ]"
			if e.excluded {
				box = "[ ]"
			} else if !e.isHeader {
				box = "[x]"
			}
			indent := ""
			if e.indent {
				indent = "  "
			}
			line := indent + box + " " + ansi.Truncate(e.title, width, "…")
			if i == a.sel {
				line = selectedStyle.Render("▸ " + line)
			} else {
				line = "  " + line
			}
			rows = append(rows, line)
			if !e.isHeader {
				preview := lipgloss.NewStyle().Foreground(subtle).Render(indent + "    " + ansi.Truncate(e.preview, width, "…"))
				rows = append(rows, preview)
			}
		}
	}
	body := header + "\n\n" + strings.Join(rows, "\n")

	words, chars := exportSelectTotals(a.entries)
	footer := commafy(words) + " mots · " + commafy(chars) + " caractères (espaces comprises) inclus dans l'export"

	var b strings.Builder
	b.WriteString(lipgloss.Place(m.width, m.height-2, lipgloss.Center, lipgloss.Center, body))
	b.WriteString("\n" + lipgloss.PlaceHorizontal(m.width, lipgloss.Center, lipgloss.NewStyle().Foreground(accent).Render(footer)))
	foot := lipgloss.NewStyle().Foreground(subtle).Render("↑↓ sélectionner · espace inclure/exclure · shift+↑↓ déplacer · esc retour · F1 aide")
	b.WriteString("\n" + lipgloss.PlaceHorizontal(m.width, lipgloss.Center, foot))
	return b.String()
}
```

Note: `box` above always shows `[x]` for a non-excluded, non-header entry and `[ ]` for an
excluded one — the seemingly-redundant `if e.excluded { box = "[ ]" }` line is there because
Task 5 will replace this with the derived-header-checkbox logic (a header's own box reflects
whether ANY child scene is included); leave the placeholder exactly as shown here for THIS
task (headers render `[ ]` unconditionally for now, since no per-chapter derivation exists
yet) — Task 5 corrects it.

Add missing imports if `tea` isn't already imported in this file (it will be needed for
`tea.Msg`/`tea.KeyMsg`/`tea.WindowSizeMsg`/`tea.Cmd`):
```go
tea "github.com/charmbracelet/bubbletea"
```

- [ ] **Step 4: Wire the screen into `main.go`**

Add the screen constant. In `main.go:350-365`, insert `screenExportSelect` after
`screenTextPicker`:
```go
const (
	screenHome screen = iota
	screenWriting
	screenManuscript
	screenSearch
	screenMover
	screenProperties
	screenSnapshots
	screenDiff
	screenHeatmap
	screenCorkboard
	screenNotes
	screenAllNotes
	screenOutline
	screenTextPicker
	screenExportSelect
)
```

Add the model field. In `main.go`, near the `allNotes allNotesModel` field (around
`main.go:543`), add:
```go
	exportSelect exportSelectModel
```

Wire the `v` key. In `main.go`'s sidebar-focused key block (`main.go:1835-1874`), add a case
alongside the existing `case "N": m.enterAllNotes()` / `case "c": m.enterCorkboard()`:
```go
				case "v":
					m.enterExportSelect()
```

Wire `Update` dispatch. Near `main.go:1220-1222` (the `screenAllNotes` dispatch), add:
```go
	if m.screen == screenExportSelect {
		return m.updateExportSelect(msg)
	}
```

Wire `View` dispatch. Near `main.go:1995-1997` (the `screenAllNotes` view dispatch), add:
```go
	if m.screen == screenExportSelect {
		return exportSelectView(m)
	}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test . -run 'TestEnterExportSelect|TestExportSelectView|TestUpdateExportSelect' -v`
Expected: PASS.

- [ ] **Step 6: Run the full suite**

Run: `go test ./... 2>&1 | tail -30`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add exportselect.go exportselect_test.go main.go
git commit -m "editor: add the all-texts screen (v), read-only list with word/char totals"
```

---

## Task 5: Checkbox toggle (`espace`) with derived chapter-header state

**Files:**
- Modify: `exportselect.go`
- Test: `exportselect_test.go`

**Interfaces:**
- Consumes: `exportSelectEntry`/`exportSelectModel` (Task 4), `saveExportSelection` (Task 1).
- Produces: `updateExportSelect` handles `" "` (space) to toggle; a chapter header's `excluded`
  becomes a DERIVED value (not stored), recomputed after every toggle.

- [ ] **Step 1: Write the failing tests**

Append to `exportselect_test.go`:
```go
func TestUpdateExportSelectToggleExcludesSceneAndPersists(t *testing.T) {
	dir := seedExportSelectManuscript(t)
	m := model{}
	m.files.dir = dir
	m.enterExportSelect()
	// entry[1] is the first scene ("Opening") in seedExportSelectManuscript.
	mm, _ := m.updateExportSelect(tea.KeyMsg{Type: tea.KeyDown}) // move to entry 1
	m2 := mm.(model)
	mm2, _ := m2.updateExportSelect(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	m3 := mm2.(model)
	if !m3.exportSelect.entries[1].excluded {
		t.Fatal("entry 1 must be excluded after toggling space")
	}
	// Persisted: a fresh enterExportSelect reload must show the same exclusion.
	m4 := model{}
	m4.files.dir = dir
	m4.enterExportSelect()
	if !m4.exportSelect.entries[1].excluded {
		t.Fatal("exclusion must survive a reload (persisted to the sidecar)")
	}
}

func TestUpdateExportSelectToggleTwiceReincludes(t *testing.T) {
	dir := seedExportSelectManuscript(t)
	m := model{}
	m.files.dir = dir
	m.enterExportSelect()
	space := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")}
	mm, _ := m.updateExportSelect(tea.KeyMsg{Type: tea.KeyDown})
	m2 := mm.(model)
	mm2, _ := m2.updateExportSelect(space)
	m3 := mm2.(model)
	mm3, _ := m3.updateExportSelect(space)
	m4 := mm3.(model)
	if m4.exportSelect.entries[1].excluded {
		t.Fatal("toggling twice must re-include the entry")
	}
}

func TestUpdateExportSelectToggleOnHeaderExcludesAllChildren(t *testing.T) {
	dir := seedExportSelectManuscript(t)
	m := model{}
	m.files.dir = dir
	m.enterExportSelect()
	// entry[0] is the chapter header "Chapter One".
	space := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")}
	mm, _ := m.updateExportSelect(space)
	m2 := mm.(model)
	if !m2.exportSelect.entries[1].excluded || !m2.exportSelect.entries[2].excluded {
		t.Fatalf("toggling the header must exclude both child scenes, got entries=%+v", m2.exportSelect.entries[:3])
	}
}

func TestUpdateExportSelectHeaderStateIsDerivedIncludedIfAnyChildIncluded(t *testing.T) {
	dir := seedExportSelectManuscript(t)
	m := model{}
	m.files.dir = dir
	m.enterExportSelect()
	space := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")}
	// Exclude only the FIRST child scene (entry 1), leave the second (entry 2) included.
	mm, _ := m.updateExportSelect(tea.KeyMsg{Type: tea.KeyDown})
	m2 := mm.(model)
	mm2, _ := m2.updateExportSelect(space)
	m3 := mm2.(model)
	// The header (entry 0) must read as included, since at least one child (entry 2) still is.
	if m3.exportSelect.entries[0].excluded {
		t.Fatal("header must show included when at least one child scene is included")
	}
}

func TestExportSelectTotalsExcludeExcludedEntries(t *testing.T) {
	entries := []exportSelectEntry{
		{isHeader: true, title: "C1"},
		{words: 10, chars: 50},
		{words: 5, chars: 25, excluded: true},
	}
	w, c := exportSelectTotals(entries)
	if w != 10 || c != 50 {
		t.Fatalf("totals = (%d, %d), want (10, 50) — excluded entry and header must not count", w, c)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test . -run 'TestUpdateExportSelectToggle|TestExportSelectTotals' -v`
Expected: FAIL — space key isn't handled yet (toggle tests fail); header-derivation tests fail
since nothing recomputes header state.

- [ ] **Step 3: Implement the toggle + header derivation**

In `exportselect.go`, add a helper that finds which header owns a given scene index and one
that recomputes every header's derived `excluded` after any change, then wire `" "` in
`updateExportSelect`.

Replace the `updateExportSelect` function body from Task 4 with:
```go
func (m model) updateExportSelect(msg tea.Msg) (tea.Model, tea.Cmd) {
	if sz, ok := msg.(tea.WindowSizeMsg); ok {
		m.width, m.height = sz.Width, sz.Height
		return m, nil
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.screen = screenWriting
		m.focus = focusSidebar
	case "up", "k":
		if m.exportSelect.sel > 0 {
			m.exportSelect.sel--
		}
	case "down", "j":
		if m.exportSelect.sel < len(m.exportSelect.entries)-1 {
			m.exportSelect.sel++
		}
	case " ":
		m.toggleExportSelectAtCursor()
		m.saveExportSelectState()
	}
	return m, nil
}

// toggleExportSelectAtCursor flips the selected entry's excluded state. Toggling a chapter
// header cascades to all of its child scenes (the rows immediately following it whose indent
// is true, up to the next header or the end of the list). Toggling a scene, standalone scene,
// or Resource only affects that one row; afterward every header's OWN excluded state is
// recomputed as derived (a header reads excluded only when ALL its children are excluded).
func (m *model) toggleExportSelectAtCursor() {
	entries := m.exportSelect.entries
	i := m.exportSelect.sel
	if i >= len(entries) {
		return
	}
	if entries[i].isHeader {
		newState := !entries[i].excluded
		for j := i + 1; j < len(entries) && entries[j].indent; j++ {
			entries[j].excluded = newState
		}
	} else {
		entries[i].excluded = !entries[i].excluded
	}
	recomputeExportSelectHeaders(entries)
}

// recomputeExportSelectHeaders sets each header's excluded field to true only when every one
// of its child (indented) rows is excluded — false (included) as soon as at least one child is
// included, including a header with zero children (nothing to exclude).
func recomputeExportSelectHeaders(entries []exportSelectEntry) {
	for i := range entries {
		if !entries[i].isHeader {
			continue
		}
		anyIncluded := false
		anyChild := false
		for j := i + 1; j < len(entries) && entries[j].indent; j++ {
			anyChild = true
			if !entries[j].excluded {
				anyIncluded = true
			}
		}
		entries[i].excluded = anyChild && !anyIncluded
	}
}

// saveExportSelectState persists every non-header entry's excluded state to the sidecar.
func (m *model) saveExportSelectState() {
	excluded := map[string]bool{}
	known := map[string]bool{}
	for _, e := range m.exportSelect.entries {
		if e.isHeader {
			continue
		}
		known[e.file] = true
		if e.excluded {
			excluded[e.file] = true
		}
	}
	if err := saveExportSelection(m.files.dir, excluded, known); err != nil {
		m.status = "échec de l'enregistrement de la sélection d'export : " + err.Error()
	}
}
```

Now fix `exportSelectView`'s checkbox rendering from Task 4 — replace the placeholder box logic:
```go
			box := "[ ]"
			if e.excluded {
				box = "[ ]"
			} else if !e.isHeader {
				box = "[x]"
			}
```
with the correct derived rendering (a header shows `[x]`/`[ ]` from its own now-correctly-derived
`excluded` field, same as any other row):
```go
			box := "[x]"
			if e.excluded {
				box = "[ ]"
			}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test . -run 'TestUpdateExportSelectToggle|TestExportSelectTotals' -v`
Expected: PASS.

- [ ] **Step 5: Run the full suite**

Run: `go test ./... 2>&1 | tail -30`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add exportselect.go exportselect_test.go
git commit -m "editor: toggle export inclusion (space), derive chapter-header state from its scenes"
```

---

## Task 6: Branch the exclusion filter into `runExport` + export checked Resources

**Files:**
- Modify: `export.go`
- Test: `export_wiring_test.go`

**Interfaces:**
- Consumes: `loadExportSelection` (Task 1), `manuscriptDocFromChapters(dir, parts, excluded)`
  (Task 3), `v.loose` (`manuscriptView`).

- [ ] **Step 1: Write the failing tests**

Append to `export_wiring_test.go`:
```go
func TestRunExportWholeManuscriptExcludesSelectedText(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	proj := filepath.Join(root, "novel")
	os.MkdirAll(filepath.Join(proj, "ch1"), 0o755)
	os.WriteFile(filepath.Join(proj, "ch1", "a.md"), []byte("Kept prose."), 0o644)
	os.WriteFile(filepath.Join(proj, "ch1", "b.md"), []byte("Excluded prose."), 0o644)
	if err := writeManifest(proj, manifest{
		Title: "Novel",
		Items: []manifestItem{
			{Chapter: &manifestChapter{Folder: "ch1", Title: "C1", Texts: []manifestText{
				{File: "a.md", Title: "Kept"},
				{File: "b.md", Title: "Excluded"},
			}}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := saveExportSelection(proj, map[string]bool{filepath.Join("ch1", "b.md"): true},
		map[string]bool{filepath.Join("ch1", "a.md"): true, filepath.Join("ch1", "b.md"): true}); err != nil {
		t.Fatal(err)
	}

	m := model{}
	m.files.dir = proj
	m.screen = screenCorkboard // exportWholeManuscript() gates on this
	m.currentFile = filepath.Join(proj, "ch1", "a.md")
	m.exportChooser = &exportChooserModel{checked: map[exportFormat]bool{formatRTF: true}, style: StyleManuscript}
	m.runExport()

	out, err := os.ReadFile(filepath.Join(proj, "export", "Novel.rtf"))
	if err != nil {
		t.Fatalf("export file not written: %v", err)
	}
	body := string(out)
	if !strings.Contains(body, "Kept") {
		t.Fatal("exported RTF must contain the non-excluded scene's content")
	}
	if strings.Contains(body, "Excluded prose") {
		t.Fatal("exported RTF must NOT contain the excluded scene's content")
	}
}

func TestRunExportWholeManuscriptIncludesCheckedResource(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	proj := filepath.Join(root, "novel")
	os.MkdirAll(filepath.Join(proj, "ch1"), 0o755)
	os.WriteFile(filepath.Join(proj, "ch1", "a.md"), []byte("Chapter prose."), 0o644)
	os.WriteFile(filepath.Join(proj, "sidenote.md"), []byte("A resource, opted into export."), 0o644)
	if err := writeManifest(proj, manifest{
		Title: "Novel",
		Items: []manifestItem{
			{Chapter: &manifestChapter{Folder: "ch1", Title: "C1", Texts: []manifestText{{File: "a.md", Title: "A"}}}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	// sidenote.md is NOT excluded — default-included, so it must appear.

	m := model{}
	m.files.dir = proj
	m.screen = screenCorkboard
	m.exportChooser = &exportChooserModel{checked: map[exportFormat]bool{formatRTF: true}, style: StyleManuscript}
	m.runExport()

	out, err := os.ReadFile(filepath.Join(proj, "export", "Novel.rtf"))
	if err != nil {
		t.Fatalf("export file not written: %v", err)
	}
	if !strings.Contains(string(out), "resource") {
		t.Fatal("exported RTF must include the Resource's content (default-included)")
	}
}

func TestRunExportCurrentDocumentUnaffectedBySelection(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	proj := filepath.Join(root, "novel")
	os.MkdirAll(filepath.Join(proj, "ch1"), 0o755)
	os.WriteFile(filepath.Join(proj, "ch1", "a.md"), []byte("Solo document text."), 0o644)
	if err := writeManifest(proj, manifest{
		Title: "Novel",
		Items: []manifestItem{
			{Chapter: &manifestChapter{Folder: "ch1", Title: "C1", Texts: []manifestText{{File: "a.md", Title: "A"}}}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	// Exclude a.md in the sidecar — must have NO effect on a single-document export.
	if err := saveExportSelection(proj, map[string]bool{filepath.Join("ch1", "a.md"): true},
		map[string]bool{filepath.Join("ch1", "a.md"): true}); err != nil {
		t.Fatal(err)
	}

	m := model{}
	m.files.dir = proj
	m.screen = screenWriting // NOT corkboard — single-document export path
	m.currentFile = filepath.Join(proj, "ch1", "a.md")
	m.exportChooser = &exportChooserModel{checked: map[exportFormat]bool{formatRTF: true}, style: StyleManuscript}
	m.runExport()

	out, err := os.ReadFile(filepath.Join(proj, "export", "A.rtf"))
	if err != nil {
		t.Fatalf("export file not written: %v", err)
	}
	if !strings.Contains(string(out), "Solo document") {
		t.Fatal("single-document export must be unaffected by the manuscript-wide exclusion sidecar")
	}
}
```

Check `export_wiring_test.go`'s existing imports include `"strings"` — add it if missing.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test . -run TestRunExportWholeManuscript -v -run TestRunExportCurrentDocumentUnaffectedBySelection`
Expected: FAIL — exclusion is not yet loaded/applied in `runExport`, and Resources are never
exported today.

- [ ] **Step 3: Wire the filter and the Resources pass in `runExport`**

In `export.go`, find the whole-manuscript branch (currently, after Task 3's Step 4, reads
`doc = manuscriptDocFromChapters(dir, v.parts, nil)`). Replace:
```go
	if m.exportWholeManuscript() {
		entries := readEntries(dir)
		v := resolveManuscript(dir, entries)
		doc = manuscriptDocFromChapters(dir, v.parts, nil)
		title = v.title
	} else {
```
with:
```go
	if m.exportWholeManuscript() {
		entries := readEntries(dir)
		v := resolveManuscript(dir, entries)
		excluded := loadExportSelection(dir)
		doc = manuscriptDocFromChapters(dir, v.parts, excluded)
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
		title = v.title
	} else {
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test . -run 'TestRunExportWholeManuscript|TestRunExportCurrentDocumentUnaffected' -v`
Expected: PASS.

- [ ] **Step 5: Run the full suite**

Run: `go test ./... 2>&1 | tail -30`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add export.go export_wiring_test.go
git commit -m "export: apply the exclusion sidecar and include checked Resources in whole-manuscript export"
```

---

## Task 7: Movement within a chapter and among top-level entries (no cross-boundary yet)

This task handles `shift+↑/↓` for the cases that do NOT change a text's container: reordering
scenes within their own chapter's `Texts[]`, reordering whole chapters among themselves, and
reordering standalone-scene/Resource rows among other top-level rows of the SAME kind. Crossing
a chapter boundary (Task 8) is deliberately split out — this task's diff stays reviewable and
already delivers most of the value (in-place reordering) before the riskier cross-container
move logic lands.

**Files:**
- Modify: `exportselect.go`
- Test: `exportselect_test.go`

**Interfaces:**
- Consumes: `exportSelectModel`, `writeManifest`/`readManifest` (`manifest.go`),
  `findChapterByFolder` (`manifest.go`).
- Produces: `updateExportSelect` handles `shift+up`/`shift+down` for same-container moves.

- [ ] **Step 1: Write the failing tests**

Append to `exportselect_test.go`:
```go
func TestMoveExportSelectSceneWithinSameChapterReordersTextsOnly(t *testing.T) {
	dir := seedExportSelectManuscript(t)
	m := model{}
	m.files.dir = dir
	m.enterExportSelect()
	// entry[1] = "Opening", entry[2] = "Confrontation", both in ch1. Move entry 1 down.
	mm, _ := m.updateExportSelect(tea.KeyMsg{Type: tea.KeyDown})
	m2 := mm.(model)
	mm2, _ := m2.updateExportSelect(tea.KeyMsg{Type: tea.KeyDown, Shift: true})
	m3 := mm2.(model)
	if m3.exportSelect.entries[1].title != "Confrontation" || m3.exportSelect.entries[2].title != "Opening" {
		t.Fatalf("want scenes swapped, got %q, %q", m3.exportSelect.entries[1].title, m3.exportSelect.entries[2].title)
	}
	got, _, _ := readManifest(dir)
	ch := findChapterByFolder(&got, "ch1")
	if ch == nil || ch.Texts[0].File != "scene-2.md" || ch.Texts[1].File != "scene-1.md" {
		t.Fatalf("manifest Texts[] must reflect the swap, got %+v", ch)
	}
	// No file should have moved on disk — same chapter, same folder.
	if _, err := os.Stat(filepath.Join(dir, "ch1", "scene-1.md")); err != nil {
		t.Fatal("scene-1.md must still exist in ch1/")
	}
}

func TestMoveExportSelectChapterHeaderMovesWholeBlock(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "ch1"), 0o755)
	os.MkdirAll(filepath.Join(dir, "ch2"), 0o755)
	os.WriteFile(filepath.Join(dir, "ch1", "a.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, "ch2", "b.md"), []byte("y"), 0o644)
	if err := writeManifest(dir, manifest{
		Title: "N",
		Items: []manifestItem{
			{Chapter: &manifestChapter{Folder: "ch1", Title: "First", Texts: []manifestText{{File: "a.md", Title: "A"}}}},
			{Chapter: &manifestChapter{Folder: "ch2", Title: "Second", Texts: []manifestText{{File: "b.md", Title: "B"}}}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	m := model{}
	m.files.dir = dir
	m.enterExportSelect()
	// entry[0] = header "First". Move it down past "Second"'s header + its scene.
	mm, _ := m.updateExportSelect(tea.KeyMsg{Type: tea.KeyDown, Shift: true})
	m2 := mm.(model)
	got, _, _ := readManifest(dir)
	if got.Items[0].Chapter.Folder != "ch2" || got.Items[1].Chapter.Folder != "ch1" {
		t.Fatalf("manifest items[] must reflect First/Second swap, got %+v", got.Items)
	}
	// Entries list must be rebuilt in the new order too.
	if m2.exportSelect.entries[0].title != "Second" {
		t.Fatalf("entries[0] = %q, want %q after moving First past Second", m2.exportSelect.entries[0].title, "Second")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test . -run TestMoveExportSelect -v`
Expected: FAIL — `shift+down` isn't handled yet.

- [ ] **Step 3: Implement same-container movement**

In `exportselect.go`, add `case "shift+up", "shift+down":` to `updateExportSelect` (from Task
5) and the supporting move function. Replace the `switch key.String() { ... }` block inside
`updateExportSelect` — add this case alongside the existing ones:
```go
	case "shift+up":
		m.moveExportSelectEntry(-1)
	case "shift+down":
		m.moveExportSelectEntry(1)
```

Add the move function (this task's version handles same-chapter and same-kind-top-level moves
only — Task 8 extends it for cross-boundary moves):
```go
// moveExportSelectEntry moves the selected row by dir (-1 up, +1 down) within its current
// container: a chapter header moves as a block (itself + all its scenes); a scene moves within
// its own chapter's Texts[]; a standalone scene or Resource moves among the other top-level
// rows. Persists the manifest (and re-resolves the entry list) immediately. Task 8 extends this
// to handle a scene/standalone-scene/Resource crossing into a DIFFERENT container.
func (m *model) moveExportSelectEntry(dir int) {
	entries := m.exportSelect.entries
	i := m.exportSelect.sel
	if i < 0 || i >= len(entries) {
		return
	}
	if entries[i].isHeader {
		m.moveExportSelectChapterBlock(i, dir)
		return
	}
	if entries[i].indent {
		m.moveExportSelectSceneWithinChapter(i, dir)
		return
	}
	// Standalone scene / Resource — Task 8 will replace this branch with full cross-boundary
	// logic. For this task, only reorder among entries of the SAME kind (a real chapter
	// boundary crossing is out of scope here).
}

// moveExportSelectChapterBlock moves the chapter header at index i (plus all its indented
// child rows) up or down past the ADJACENT chapter block, by swapping the two blocks' order in
// the on-disk manifest's bare-chapter items, then re-resolving the entry list from scratch.
func (m *model) moveExportSelectChapterBlock(i, dir int) {
	mani, present, err := readManifest(m.files.dir)
	if err != nil || !present {
		m.status = "impossible de déplacer : le manifeste est introuvable ou illisible"
		return
	}
	// Find this header's chapter folder and the adjacent header's folder by walking entries.
	folder := m.exportSelect.entries[i].file // headers don't set file in Task 4; fix: see note below
	_ = folder
	// Locate bare-chapter indices in mani.Items (Chapter != nil, Scene == false) in order.
	var bareIdx []int
	for idx, it := range mani.Items {
		if it.Chapter != nil && !it.Chapter.Scene {
			bareIdx = append(bareIdx, idx)
		}
	}
	// Map this header's position among headers in m.exportSelect.entries to a position in
	// bareIdx by counting headers seen up to i.
	headerPos := -1
	seen := 0
	for idx, e := range m.exportSelect.entries {
		if e.isHeader {
			if idx == i {
				headerPos = seen
				break
			}
			seen++
		}
	}
	if headerPos < 0 {
		return
	}
	target := headerPos + dir
	if target < 0 || target >= len(bareIdx) {
		return // already at an edge
	}
	mani.Items[bareIdx[headerPos]], mani.Items[bareIdx[target]] = mani.Items[bareIdx[target]], mani.Items[bareIdx[headerPos]]
	if err := writeManifest(m.files.dir, mani); err != nil {
		m.status = "échec du déplacement : " + err.Error()
		return
	}
	m.reloadExportSelectAfterMove()
}

// moveExportSelectSceneWithinChapter moves the scene at index i up/down within its own
// chapter's Texts[], refusing to cross the chapter's own header/footer boundary (that's Task
// 8's job). i's owning chapter is found by walking backward to the nearest header.
func (m *model) moveExportSelectSceneWithinChapter(i, dir int) {
	entries := m.exportSelect.entries
	headerIdx := -1
	for j := i - 1; j >= 0; j-- {
		if entries[j].isHeader {
			headerIdx = j
			break
		}
	}
	if headerIdx < 0 {
		return
	}
	// This chapter's scene rows span (headerIdx, chapterEnd).
	chapterEnd := len(entries)
	for j := headerIdx + 1; j < len(entries); j++ {
		if entries[j].isHeader || !entries[j].indent {
			chapterEnd = j
			break
		}
	}
	target := i + dir
	if target <= headerIdx || target >= chapterEnd {
		return // would leave this chapter — Task 8's job, not this task's
	}
	folder := m.chapterFolderForHeader(headerIdx)
	if folder == "" {
		return
	}
	mani, present, err := readManifest(m.files.dir)
	if err != nil || !present {
		m.status = "impossible de déplacer : le manifeste est introuvable ou illisible"
		return
	}
	ch := findChapterByFolder(&mani, folder)
	if ch == nil {
		return
	}
	sceneIdx := i - headerIdx - 1
	targetIdx := target - headerIdx - 1
	if sceneIdx < 0 || sceneIdx >= len(ch.Texts) || targetIdx < 0 || targetIdx >= len(ch.Texts) {
		return
	}
	ch.Texts[sceneIdx], ch.Texts[targetIdx] = ch.Texts[targetIdx], ch.Texts[sceneIdx]
	if err := writeManifest(m.files.dir, mani); err != nil {
		m.status = "échec du déplacement : " + err.Error()
		return
	}
	m.reloadExportSelectAfterMove()
}

// chapterFolderForHeader re-resolves the manuscript to find the folder of the chapter whose
// header is at entries[headerIdx] — counting headers up to headerIdx and matching against the
// resolved view's bare chapters in the same order buildExportSelectEntries used.
func (m model) chapterFolderForHeader(headerIdx int) string {
	v := resolveManuscript(m.files.dir, readEntries(m.files.dir))
	seen := 0
	for _, part := range v.parts {
		for _, ch := range part.chapters {
			if ch.scene {
				continue
			}
			if seen == countHeadersBefore(m.exportSelect.entries, headerIdx) {
				return ch.folder
			}
			seen++
		}
	}
	return ""
}

// countHeadersBefore counts header rows at indices strictly less than upTo. If entries[upTo]
// is itself a header, it is NOT counted — its own 0-indexed position among headers equals this
// count, which is exactly the alignment chapterFolderForHeader's "seen == countHeadersBefore(...)"
// comparison relies on (the Nth header, 0-indexed, is preceded by exactly N earlier headers).
func countHeadersBefore(entries []exportSelectEntry, upTo int) int {
	n := 0
	for i := 0; i < upTo && i < len(entries); i++ {
		if entries[i].isHeader {
			n++
		}
	}
	return n
}

// reloadExportSelectAfterMove re-resolves the manuscript and rebuilds the entry list — the
// simplest way to keep the display consistent with a just-written manifest, at the cost of a
// full re-read per move (acceptable: moves are an interactive, human-paced action, not a hot
// path — same tradeoff enterExportSelect already makes on screen entry).
func (m *model) reloadExportSelectAfterMove() {
	dir := m.files.dir
	v := resolveManuscript(dir, readEntries(dir))
	excluded := loadExportSelection(dir)
	sel := m.exportSelect.sel
	m.exportSelect = exportSelectModel{entries: buildExportSelectEntries(dir, v, excluded), sel: sel}
	if m.exportSelect.sel >= len(m.exportSelect.entries) {
		m.exportSelect.sel = len(m.exportSelect.entries) - 1
	}
}
```

**Known rough edge in this task's `moveExportSelectChapterBlock`:** the `folder :=
m.exportSelect.entries[i].file` line is dead code (a header's `file` field is always empty per
Task 4's `buildExportSelectEntries`, which never sets `file` on header rows) — it's set then
discarded (`_ = folder`) to make this visible rather than silently wrong; the actual chapter
lookup instead goes through `headerPos`/`bareIdx` counting. If your test run reveals this
counting approach doesn't correctly identify the right chapter (e.g., a Part-grouped manifest
where bare-chapter counting and the `parts` iteration order diverge), that's a real bug to fix,
not a brief inconsistency to route around — this codebase's chapters, per the standalone-scene
plan's explicit scope, never live inside a Part for THIS feature's purposes, so `bareIdx`
counting should stay aligned with `buildExportSelectEntries`'s iteration, but verify this with
the test before trusting it.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test . -run TestMoveExportSelect -v`
Expected: PASS. If `TestMoveExportSelectChapterHeaderMovesWholeBlock` fails because the header
matching logic above is wrong, debug it — do not weaken the test. Add print-debugging or a
`t.Logf` of intermediate state if needed; the fix belongs in `moveExportSelectChapterBlock`/
`chapterFolderForHeader`, not in loosening the assertion.

- [ ] **Step 5: Run the full suite**

Run: `go test ./... 2>&1 | tail -30`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add exportselect.go exportselect_test.go
git commit -m "editor: move scenes within a chapter and chapters among themselves (shift+up/down)"
```

---

## Task 8: Cross-container movement — scene ↔ different chapter, standalone scene ↔ Resource

The highest-risk task in this plan: moves a real file on disk (`safeMove`) and rewrites the
manifest in the same operation. Kept as its own task, last among the movement work, so its
review gets full attention without competing with the simpler same-container logic already
proven working by Task 7.

**Files:**
- Modify: `exportselect.go`
- Test: `exportselect_test.go`

**Interfaces:**
- Consumes: `safeMove` (`move.go:128`), `findChapterByFolder`/`findSceneByFile`
  (`manifest.go`), `saveExportSelection` (Task 1, for migrating the sidecar key).

- [ ] **Step 1: Write the failing tests**

Append to `exportselect_test.go`:
```go
func TestMoveExportSelectSceneCrossesIntoAdjacentChapter(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "ch1"), 0o755)
	os.MkdirAll(filepath.Join(dir, "ch2"), 0o755)
	os.WriteFile(filepath.Join(dir, "ch1", "only.md"), []byte("The only scene of ch1."), 0o644)
	os.WriteFile(filepath.Join(dir, "ch2", "first.md"), []byte("ch2's first scene."), 0o644)
	if err := writeManifest(dir, manifest{
		Title: "N",
		Items: []manifestItem{
			{Chapter: &manifestChapter{Folder: "ch1", Title: "First", Texts: []manifestText{{File: "only.md", Title: "Only"}}}},
			{Chapter: &manifestChapter{Folder: "ch2", Title: "Second", Texts: []manifestText{{File: "first.md", Title: "First"}}}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	m := model{}
	m.files.dir = dir
	m.enterExportSelect()
	// entries: [0]=header First, [1]="Only" (indented), [2]=header Second, [3]="First" (indented).
	// Move "Only" (index 1) down, crossing into ch2.
	mm, _ := m.updateExportSelect(tea.KeyMsg{Type: tea.KeyDown})
	m2 := mm.(model)
	mm2, _ := m2.updateExportSelect(tea.KeyMsg{Type: tea.KeyDown, Shift: true})
	m3 := mm2.(model)

	if _, err := os.Stat(filepath.Join(dir, "ch2", "only.md")); err != nil {
		t.Fatalf("only.md must have moved to ch2/, got err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "ch1", "only.md")); !os.IsNotExist(err) {
		t.Fatal("only.md must no longer exist in ch1/")
	}
	got, _, _ := readManifest(dir)
	ch1 := findChapterByFolder(&got, "ch1")
	ch2 := findChapterByFolder(&got, "ch2")
	if len(ch1.Texts) != 0 {
		t.Fatalf("ch1 must now have 0 texts, got %+v", ch1.Texts)
	}
	found := false
	for _, t := range ch2.Texts {
		if t.File == "only.md" {
			found = true
		}
	}
	if !found {
		t.Fatalf("ch2 must now list only.md, got %+v", ch2.Texts)
	}
	_ = m3
}

func TestMoveExportSelectMigratesSidecarKeyOnCrossBoundaryMove(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "ch1"), 0o755)
	os.MkdirAll(filepath.Join(dir, "ch2"), 0o755)
	os.WriteFile(filepath.Join(dir, "ch1", "only.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, "ch2", "first.md"), []byte("y"), 0o644)
	if err := writeManifest(dir, manifest{
		Title: "N",
		Items: []manifestItem{
			{Chapter: &manifestChapter{Folder: "ch1", Title: "First", Texts: []manifestText{{File: "only.md", Title: "Only"}}}},
			{Chapter: &manifestChapter{Folder: "ch2", Title: "Second", Texts: []manifestText{{File: "first.md", Title: "First"}}}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	oldKey := filepath.Join("ch1", "only.md")
	if err := saveExportSelection(dir, map[string]bool{oldKey: true}, map[string]bool{oldKey: true}); err != nil {
		t.Fatal(err)
	}
	m := model{}
	m.files.dir = dir
	m.enterExportSelect()
	mm, _ := m.updateExportSelect(tea.KeyMsg{Type: tea.KeyDown})
	m2 := mm.(model)
	m2.updateExportSelect(tea.KeyMsg{Type: tea.KeyDown, Shift: true})

	newKey := filepath.Join("ch2", "only.md")
	got := loadExportSelection(dir)
	if got[oldKey] {
		t.Fatal("old sidecar key must not survive the move")
	}
	if !got[newKey] {
		t.Fatal("exclusion must migrate to the new path")
	}
}

func TestMoveExportSelectRefusesNameCollisionInDestination(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "ch1"), 0o755)
	os.MkdirAll(filepath.Join(dir, "ch2"), 0o755)
	os.WriteFile(filepath.Join(dir, "ch1", "same.md"), []byte("from ch1"), 0o644)
	os.WriteFile(filepath.Join(dir, "ch2", "same.md"), []byte("already in ch2"), 0o644)
	if err := writeManifest(dir, manifest{
		Title: "N",
		Items: []manifestItem{
			{Chapter: &manifestChapter{Folder: "ch1", Title: "First", Texts: []manifestText{{File: "same.md", Title: "Same"}}}},
			{Chapter: &manifestChapter{Folder: "ch2", Title: "Second", Texts: []manifestText{{File: "same.md", Title: "Same"}}}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	m := model{}
	m.files.dir = dir
	m.enterExportSelect()
	mm, _ := m.updateExportSelect(tea.KeyMsg{Type: tea.KeyDown})
	m2 := mm.(model)
	mm2, _ := m2.updateExportSelect(tea.KeyMsg{Type: tea.KeyDown, Shift: true})
	m3 := mm2.(model)

	if _, err := os.Stat(filepath.Join(dir, "ch1", "same.md")); err != nil {
		t.Fatal("ch1/same.md must NOT have been moved away (collision refused)")
	}
	data, _ := os.ReadFile(filepath.Join(dir, "ch2", "same.md"))
	if string(data) != "already in ch2" {
		t.Fatal("ch2/same.md must be untouched (not overwritten)")
	}
	if m3.status == "" {
		t.Fatal("a status message must explain the refused move")
	}
}

func TestMoveExportSelectStandaloneSceneBecomesResourceCrossingBoundary(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "aparte.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, "notes.md"), []byte("y"), 0o644)
	if err := writeManifest(dir, manifest{
		Title: "N",
		Items: []manifestItem{
			{Chapter: &manifestChapter{Title: "Aparté", Scene: true, Texts: []manifestText{{File: "aparte.md", Title: "Aparté"}}}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	m := model{}
	m.files.dir = dir
	m.enterExportSelect()
	// entries: [0] = standalone scene "Aparté", [1] = Resource "notes". Move Aparté past notes.
	mm, _ := m.updateExportSelect(tea.KeyMsg{Type: tea.KeyDown, Shift: true})
	m2 := mm.(model)
	got, _, _ := readManifest(dir)
	if len(got.Items) != 0 {
		t.Fatalf("Aparté must have been dropped from items[] (now a plain Resource), got %+v", got.Items)
	}
	if _, err := os.Stat(filepath.Join(dir, "aparte.md")); err != nil {
		t.Fatal("aparte.md must still exist on disk (unmoved — root stays root)")
	}
	_ = m2
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test . -run 'TestMoveExportSelectSceneCrosses|TestMoveExportSelectMigrates|TestMoveExportSelectRefuses|TestMoveExportSelectStandalone' -v`
Expected: FAIL — Task 7 left the "crosses a boundary" case as a no-op.

- [ ] **Step 3: Implement cross-container movement**

In `exportselect.go`, extend `moveExportSelectSceneWithinChapter` to handle the crossing case,
and give the standalone-scene/Resource branch real logic. Replace the whole function from Task
7 (`moveExportSelectSceneWithinChapter`) with:
```go
// moveExportSelectSceneWithinChapter moves the scene at index i up/down. If the target stays
// inside the same chapter's scene span, it's a simple Texts[] reorder (no disk change). If the
// target crosses into an ADJACENT chapter, the scene's file is moved on disk (safeMove) and the
// manifest is rewritten to drop it from the source chapter's Texts[] and append it to the
// target chapter's Texts[] at the crossed position; the sidecar's exclusion key (if any)
// migrates to the new path.
func (m *model) moveExportSelectSceneWithinChapter(i, dir int) {
	entries := m.exportSelect.entries
	headerIdx := -1
	for j := i - 1; j >= 0; j-- {
		if entries[j].isHeader {
			headerIdx = j
			break
		}
	}
	if headerIdx < 0 {
		return
	}
	chapterEnd := len(entries)
	for j := headerIdx + 1; j < len(entries); j++ {
		if entries[j].isHeader || !entries[j].indent {
			chapterEnd = j
			break
		}
	}
	target := i + dir

	if target > headerIdx && target < chapterEnd {
		// Same-chapter reorder — Task 7's logic, unchanged.
		folder := m.chapterFolderForHeader(headerIdx)
		if folder == "" {
			return
		}
		mani, present, err := readManifest(m.files.dir)
		if err != nil || !present {
			m.status = "impossible de déplacer : le manifeste est introuvable ou illisible"
			return
		}
		ch := findChapterByFolder(&mani, folder)
		if ch == nil {
			return
		}
		sceneIdx := i - headerIdx - 1
		targetIdx := target - headerIdx - 1
		if sceneIdx < 0 || sceneIdx >= len(ch.Texts) || targetIdx < 0 || targetIdx >= len(ch.Texts) {
			return
		}
		ch.Texts[sceneIdx], ch.Texts[targetIdx] = ch.Texts[targetIdx], ch.Texts[sceneIdx]
		if err := writeManifest(m.files.dir, mani); err != nil {
			m.status = "échec du déplacement : " + err.Error()
			return
		}
		m.reloadExportSelectAfterMove()
		return
	}

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
	srcFolder := m.chapterFolderForHeader(headerIdx)
	dstFolder := m.chapterFolderForHeader(dstHeaderIdx)
	if srcFolder == "" || dstFolder == "" {
		return
	}
	m.moveSceneBetweenChapters(srcFolder, entries[i].file, dstFolder)
}

// moveSceneBetweenChapters moves file (currently in srcFolder's Texts[]) into dstFolder: the
// on-disk file first (safeMove; refused-collision leaves everything untouched), then a single
// manifest read-modify-write that drops it from srcFolder and appends it to dstFolder, then
// migrates the sidecar's exclusion key if the file had one.
func (m *model) moveSceneBetweenChapters(srcFolder, file, dstFolder string) {
	base := filepath.Base(file)
	src := filepath.Join(m.files.dir, srcFolder, base)
	dst := filepath.Join(m.files.dir, dstFolder, base)
	if _, err := os.Stat(dst); err == nil {
		m.status = "un fichier nommé " + base + " existe déjà dans le chapitre de destination"
		return
	}
	if err := safeMove(src, dst); err != nil {
		m.status = "échec du déplacement : " + err.Error()
		return
	}
	mani, present, err := readManifest(m.files.dir)
	if err != nil || !present {
		m.status = "scène déplacée mais le manifeste est introuvable ou illisible"
		return
	}
	srcCh := findChapterByFolder(&mani, srcFolder)
	dstCh := findChapterByFolder(&mani, dstFolder)
	if srcCh == nil || dstCh == nil {
		m.status = "scène déplacée mais un chapitre est introuvable dans le manifeste"
		return
	}
	var moved manifestText
	kept := srcCh.Texts[:0]
	for _, t := range srcCh.Texts {
		if t.File == base {
			moved = t
			continue
		}
		kept = append(kept, t)
	}
	srcCh.Texts = kept
	dstCh.Texts = append(dstCh.Texts, moved)
	if err := writeManifest(m.files.dir, mani); err != nil {
		m.status = "échec de la mise à jour du manifeste après déplacement : " + err.Error()
		return
	}
	oldKey := filepath.Join(srcFolder, base)
	newKey := filepath.Join(dstFolder, base)
	excluded := loadExportSelection(m.files.dir)
	if excluded[oldKey] {
		delete(excluded, oldKey)
		excluded[newKey] = true
		known := map[string]bool{newKey: true}
		for k := range excluded {
			known[k] = true
		}
		if err := saveExportSelection(m.files.dir, excluded, known); err != nil {
			m.status = "scène déplacée mais échec de la migration de la sélection d'export : " + err.Error()
			return
		}
	}
	m.reloadExportSelectAfterMove()
}
```

Now handle the standalone-scene/Resource branch of `moveExportSelectEntry` (from Task 7,
currently empty). Replace:
```go
	// Standalone scene / Resource — Task 8 will replace this branch with full cross-boundary
	// logic. For this task, only reorder among entries of the SAME kind (a real chapter
	// boundary crossing is out of scope here).
```
with:
```go
	m.moveTopLevelEntry(i, dir)
}

// moveTopLevelEntry handles a standalone scene or a Resource crossing into the OTHER kind
// (scene→Resource drops the manifest item, file stays at root; Resource→scene adds a new
// Scene:true item, file stays at root — neither moves a file, since both live at the
// manuscript root already) or reordering among entries of the same kind (Resources sort
// alphabetically today and are not manifest-ordered, so "moving" a Resource past another
// Resource has no manifest effect; moving a standalone scene past another standalone scene
// reorders items[]).
func (m *model) moveTopLevelEntry(i, dir int) {
	entries := m.exportSelect.entries
	target := i + dir
	if target < 0 || target >= len(entries) || entries[target].isHeader || entries[target].indent {
		return
	}
	movingIsScene := !entries[i].isHeader && !entries[i].indent && isStandaloneSceneFile(m.files.dir, entries[i].file)
	targetIsScene := !entries[target].isHeader && !entries[target].indent && isStandaloneSceneFile(m.files.dir, entries[target].file)

	if movingIsScene && targetIsScene {
		m.reorderStandaloneScenes(entries[i].file, entries[target].file)
		return
	}
	if movingIsScene && !targetIsScene {
		m.convertStandaloneSceneToResource(entries[i].file)
		return
	}
	if !movingIsScene && targetIsScene {
		m.convertResourceToStandaloneScene(entries[i].file)
		return
	}
	// Both Resources: no manifest to touch (Resources aren't ordered by items[]) — nothing to persist.
}

// isStandaloneSceneFile reports whether file (relative to dir) is currently listed as a
// Scene:true item's Texts[0].File in dir's manifest.
func isStandaloneSceneFile(dir, file string) bool {
	mani, present, err := readManifest(dir)
	if err != nil || !present {
		return false
	}
	return findSceneByFile(&mani, file) != nil
}

// reorderStandaloneScenes swaps the items[] positions of the two standalone scenes referencing
// fileA and fileB — no file moves, both already live at the manuscript root.
func (m *model) reorderStandaloneScenes(fileA, fileB string) {
	mani, present, err := readManifest(m.files.dir)
	if err != nil || !present {
		m.status = "impossible de déplacer : le manifeste est introuvable ou illisible"
		return
	}
	idxA, idxB := -1, -1
	for idx, it := range mani.Items {
		if it.Chapter == nil || !it.Chapter.Scene || len(it.Chapter.Texts) == 0 {
			continue
		}
		switch it.Chapter.Texts[0].File {
		case fileA:
			idxA = idx
		case fileB:
			idxB = idx
		}
	}
	if idxA < 0 || idxB < 0 {
		return
	}
	mani.Items[idxA], mani.Items[idxB] = mani.Items[idxB], mani.Items[idxA]
	if err := writeManifest(m.files.dir, mani); err != nil {
		m.status = "échec du déplacement : " + err.Error()
		return
	}
	m.reloadExportSelectAfterMove()
}

// convertStandaloneSceneToResource drops file's Scene:true item from items[] — the file stays
// on disk at the manuscript root, unmoved, and becomes a Resource by the existing definition
// (an unlisted root .md file).
func (m *model) convertStandaloneSceneToResource(file string) {
	mani, present, err := readManifest(m.files.dir)
	if err != nil || !present {
		m.status = "impossible de déplacer : le manifeste est introuvable ou illisible"
		return
	}
	var kept []manifestItem
	for _, it := range mani.Items {
		if it.Chapter != nil && it.Chapter.Scene && len(it.Chapter.Texts) > 0 && it.Chapter.Texts[0].File == file {
			continue
		}
		kept = append(kept, it)
	}
	mani.Items = kept
	if err := writeManifest(m.files.dir, mani); err != nil {
		m.status = "échec du déplacement : " + err.Error()
		return
	}
	m.reloadExportSelectAfterMove()
}

// convertResourceToStandaloneScene adds file as a new Scene:true item at the end of items[] —
// the file stays on disk at the manuscript root, unmoved.
func (m *model) convertResourceToStandaloneScene(file string) {
	mani, present, err := readManifest(m.files.dir)
	if err != nil || !present {
		m.status = "impossible de déplacer : le manifeste est introuvable ou illisible"
		return
	}
	title := sectionTitle(file)
	mani.Items = append(mani.Items, manifestItem{Chapter: &manifestChapter{
		Title: title,
		Scene: true,
		Texts: []manifestText{{File: file, Title: title}},
	}})
	if err := writeManifest(m.files.dir, mani); err != nil {
		m.status = "échec du déplacement : " + err.Error()
		return
	}
	m.reloadExportSelectAfterMove()
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test . -run 'TestMoveExportSelectSceneCrosses|TestMoveExportSelectMigrates|TestMoveExportSelectRefuses|TestMoveExportSelectStandalone' -v`
Expected: PASS.

- [ ] **Step 5: Run the full suite**

Run: `go test ./... 2>&1 | tail -30`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add exportselect.go exportselect_test.go
git commit -m "editor: cross-chapter scene move and standalone-scene <-> resource conversion (shift+up/down)"
```

---

## Task 9: Update CLAUDE.md per the spec's "Impact CLAUDE.md" section

**Files:**
- Modify: `CLAUDE.md`

**Interfaces:** none (documentation only).

- [ ] **Step 1: Add the Project model note**

In `CLAUDE.md`, under `## Project model (the shipped reality)`, document: the new `v` screen
(from the sidebar/corkboard) listing every text with an export-inclusion checkbox and
word/character totals; the `.okashi-export.json` sidecar; that a Resource can now appear in an
export (previously never possible) when checked on this screen.

- [ ] **Step 2: Add a Shared Contracts §1 note**

Note that this plan's cross-container moves rewrite `manifest.json` using the existing v3
shape (retrofit `Texts[]`/`items[]` entries) — no new field, no schema-shape change.

- [ ] **Step 3: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: document the all-texts export-selection screen in CLAUDE.md"
```

---

## Self-Review Notes (for the plan author, already applied above)

- **Spec coverage:** sidecar (Task 1), screen + display model (Task 4), checkbox toggle +
  derived header state (Task 5), export bug fix (Task 3) + filter wiring (Task 6), movement —
  split into same-container (Task 7, lower risk) and cross-container (Task 8, highest risk,
  isolated for focused review) — CLAUDE.md (Task 9). `charCount` (Task 2) factored out since
  it's a pure, independent utility multiple later tasks depend on.
- **Ruling on the total's number format:** the spec's mockup uses a space-separated thousands
  format (`1 234 mots`); this plan uses `commafy` (`1,234`) instead, since that's the only
  thousands-formatter in the codebase and is used everywhere else. Documented in Global
  Constraints as an explicit, reasoned deviation — not silently applied.
- **Type consistency:** `exportSelectEntry`/`exportSelectModel` (Task 4) are used identically
  in Tasks 5, 7, 8 — `file`/`title`/`preview`/`words`/`chars`/`excluded`/`indent`/`isHeader`
  field names never drift. `loadExportSelection`/`saveExportSelection` (Task 1) signatures are
  reused verbatim by Tasks 4, 5, 6, 8.
- **Explicit dependency on the sibling plan:** Tasks 3-8 depend on `chapterRef.scene`/
  `manifestChapter.Scene`/`findSceneByFile` from `docs/superpowers/plans/2026-08-23-standalone-scene.md`.
  Each dependent task's header states this and tells the executor to check that plan's ledger
  before starting — this plan does not re-implement or duplicate that work.
- **Known rough edge flagged inline, not hidden:** Task 7's `moveExportSelectChapterBlock` has
  a documented dead-code line (`folder := ... ; _ = folder`) plus a call-out that the
  header-counting approach must be verified by its own test rather than assumed correct — this
  is flagged explicitly in the task text so an implementer doesn't skip verifying it.
