# Création de scène via ctrl+n Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a third `ctrl+n` option — "scène" — that appends a new blank text to an
existing chapter's `Texts[]`, reachable both from the sidebar's chapter/resource picker and
from the text-picker screen; rename the user-facing concept "texte" to "scène" everywhere it
already leaked into the UI.

**Architecture:** `createScene(folder, name string)` mirrors the existing `createChapter` —
write the blank file atomically, then read-modify-write the manifest — but appends a
`manifestText` to an existing `manifestChapter.Texts` instead of creating a new chapter. A new
manifest helper `findChapterByFolder` (mirroring the traversal already inlined in
`renameChapterTitle`) locates the target `*manifestChapter` for mutation. The picker gains a
third branch (`kind == 3`) gated on the sidebar selection resolving to a chapter via the
existing `chapterByFolder`/`isChapterOf` pair. `updateTextPicker` gains a `ctrl+n` case reusing
the same prompt, targeting `m.textPickerChapter.folder` directly (already resolved, no lookup
needed).

**Tech Stack:** Go, Bubble Tea (`tea.Msg`/`Update`/`View`), existing `manifest.go`/
`manuscript.go`/`filelist.go`/`main.go` — no new dependencies.

**Spec:** [docs/superpowers/specs/2026-08-16-scene-creation-design.md](../specs/2026-08-16-scene-creation-design.md)

## Global Constraints

- Renamed user-facing term is **"scène"** (not "texte") in every string the user sees; Go
  identifiers `manifestText`/`textRef`/`Texts`/`texts` are NOT renamed (JSON contract,
  `json:"texts"` stays as-is).
- Writes to `manifest.json` are always read-modify-write via `readManifest`/`writeManifest`
  (already atomic — `writeManifest` uses `atomicWrite` internally); never hand-edit JSON.
- New blank scene files are written via `atomicWrite`, exactly like `createChapter`/
  `createResource` do.
- No manifest schema change — `manifestText`/`manifestChapter.Texts` already exist (v2, shipped
  2026-08-04). This plan touches no `json:"..."` tag.
- Follow existing test patterns: `create_prompt_test.go` for `ctrl+n` picker flows (drives
  `m.Update` with `tea.KeyMsg`), `main_test.go`'s `TestTextPicker*` family for text-picker
  screen flows (`initialModel()` + `t.Setenv("OKASHI_DIR", dir)`).

---

## File Structure

| File | Change |
|---|---|
| `manifest.go` | Add `findChapterByFolder(m *manifest, folder string) *manifestChapter` |
| `manifest_test.go` | Add tests for `findChapterByFolder` |
| `main.go` | Add `model.createChapterFolder string` field; add `createScene` method; add `kind == 3` branch in `confirmCreate`; extend the `createPicker` key handling (`case "s":`) gated on sidebar selection being a chapter; extend `updateTextPicker` with a `ctrl+n` case; update the 3 status/help strings (`ctrl+n` help line, the two `createPicker` prompts) to mention "s scène"; rename "texte"→"scène" in the empty-state string (`main.go:235`) |
| `corkboard.go` | Rename "texte"→"scène" in the status string (`corkboard.go:337`) |
| `create_prompt_test.go` | Add tests: scene option appears/absent depending on sidebar selection, `createScene` appends to an existing chapter's `Texts[]`, collision handling |
| `main_test.go` | Add tests: `ctrl+n` from `screenTextPicker` opens the naming prompt targeting the picker's chapter; update `TestTextPickerShowsEmptyStateForZeroTextChapter` to expect "aucune scène" |
| `docs/CLAUDE.md` | Update "Project model" section: `ctrl+n` now offers chapter/resource/scene; text-picker screen gains a creation action |

---

## Task 1: `findChapterByFolder` manifest helper

**Files:**
- Modify: `manifest.go` (add function after `renameChapterTitle`, `manifest.go:178`)
- Test: `manifest_test.go`

**Interfaces:**
- Produces: `findChapterByFolder(m *manifest, folder string) *manifestChapter` — returns a
  pointer into `m.Items` (mutating through it mutates `m`), or `nil` if no chapter with that
  `Folder` exists anywhere in `m.Items` (root-level `Chapter` or any Part's `Chapters[]`).

- [ ] **Step 1: Write the failing test**

Add to `manifest_test.go`:

```go
func TestFindChapterByFolderRootChapter(t *testing.T) {
	m := manifest{SchemaVersion: manifestSchemaVersion, Items: []manifestItem{
		{Chapter: &manifestChapter{Folder: "un", Title: "Un", Texts: []manifestText{{File: "un.md", Title: "Un"}}}},
	}}
	ch := findChapterByFolder(&m, "un")
	if ch == nil || ch.Folder != "un" {
		t.Fatalf("expected to find root chapter %q, got %+v", "un", ch)
	}
}

func TestFindChapterByFolderInsidePart(t *testing.T) {
	m := manifest{SchemaVersion: manifestSchemaVersion, Items: []manifestItem{
		{Part: "Acte I", Chapters: []manifestChapter{
			{Folder: "deux", Title: "Deux", Texts: []manifestText{{File: "deux.md", Title: "Deux"}}},
		}},
	}}
	ch := findChapterByFolder(&m, "deux")
	if ch == nil || ch.Folder != "deux" {
		t.Fatalf("expected to find chapter %q inside a Part, got %+v", "deux", ch)
	}
}

func TestFindChapterByFolderMutatesThroughPointer(t *testing.T) {
	m := manifest{SchemaVersion: manifestSchemaVersion, Items: []manifestItem{
		{Chapter: &manifestChapter{Folder: "un", Title: "Un", Texts: []manifestText{{File: "un.md", Title: "Un"}}}},
	}}
	ch := findChapterByFolder(&m, "un")
	ch.Texts = append(ch.Texts, manifestText{File: "deux.md", Title: "Deux"})
	if len(m.Items[0].Chapter.Texts) != 2 {
		t.Fatalf("mutating through the returned pointer must mutate m, got %d texts", len(m.Items[0].Chapter.Texts))
	}
}

func TestFindChapterByFolderNotFound(t *testing.T) {
	m := manifest{SchemaVersion: manifestSchemaVersion, Items: []manifestItem{
		{Chapter: &manifestChapter{Folder: "un", Title: "Un"}},
	}}
	if ch := findChapterByFolder(&m, "inexistant"); ch != nil {
		t.Fatalf("expected nil for a folder not in the manifest, got %+v", ch)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./... -run TestFindChapterByFolder -v`
Expected: FAIL — `findChapterByFolder` undefined.

- [ ] **Step 3: Write the implementation**

Add to `manifest.go`, after `renameChapterTitle` (after line 178):

```go
// findChapterByFolder returns a pointer to the manifestChapter matching folder — across the
// root and every Part's Chapters — so the caller can mutate it (e.g. append a Texts entry)
// before a single writeManifest. Returns nil if no chapter with that folder exists.
func findChapterByFolder(m *manifest, folder string) *manifestChapter {
	for i := range m.Items {
		if m.Items[i].Chapter != nil && m.Items[i].Chapter.Folder == folder {
			return m.Items[i].Chapter
		}
		for j := range m.Items[i].Chapters {
			if m.Items[i].Chapters[j].Folder == folder {
				return &m.Items[i].Chapters[j]
			}
		}
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./... -run TestFindChapterByFolder -v`
Expected: PASS (4 tests)

- [ ] **Step 5: Commit**

```bash
git add manifest.go manifest_test.go
git commit -m "manifest: ajoute findChapterByFolder pour muter un chapitre existant"
```

---

## Task 2: `createScene` core function

**Files:**
- Modify: `main.go` (add method near `createChapter`/`createResource`, after line 2369)
- Test: `create_prompt_test.go`

**Interfaces:**
- Consumes: `findChapterByFolder` (Task 1), `readManifest`/`writeManifest`/`atomicWrite`
  (existing), `m.files.SetDir`/`m.loadFile` (existing, same as `createChapter`).
- Produces: `(m *model) createScene(folder, name string)` — on success, writes the blank file,
  appends to the manifest, opens the file in the editor, sets `m.status`. On any failure sets
  `m.status` to an error string and returns without partial manifest writes.

- [ ] **Step 1: Write the failing tests**

Add to `create_prompt_test.go`:

```go
func TestCreateSceneAppendsToExistingChapter(t *testing.T) {
	m, dir := createFlowModel(t)
	m.createScene("a", "deuxième scène")

	if _, err := os.Stat(filepath.Join(dir, "a", "deuxieme-scene.md")); err != nil {
		t.Fatal("scene file should be created inside the chapter folder")
	}
	mani, _, _ := readManifest(dir)
	if len(mani.Items) != 1 {
		t.Fatalf("scene creation must not add a new manifest item, got %d items", len(mani.Items))
	}
	texts := mani.Items[0].Chapter.Texts
	if len(texts) != 2 || texts[1].File != "deuxieme-scene.md" || texts[1].Title != "deuxième scène" {
		t.Fatalf("scene should be appended to the chapter's Texts, got %+v", texts)
	}
}

func TestCreateSceneOpensInEditor(t *testing.T) {
	m, dir := createFlowModel(t)
	m.createScene("a", "deuxième scène")

	want := filepath.Join(dir, "a", "deuxieme-scene.md")
	if m.currentFile != want {
		t.Fatalf("currentFile = %q, want %q", m.currentFile, want)
	}
	if m.focus != focusEditor {
		t.Fatalf("focus should move to the editor, got %v", m.focus)
	}
}

func TestCreateSceneCollisionRefusesCreation(t *testing.T) {
	m, dir := createFlowModel(t)
	// "a.md" already exists in chapter "a" from createFlowModel's mkChapterDir.
	m.createScene("a", "a")

	mani, _, _ := readManifest(dir)
	if len(mani.Items[0].Chapter.Texts) != 1 {
		t.Fatalf("colliding scene name must not be added, got %+v", mani.Items[0].Chapter.Texts)
	}
	if !strings.Contains(m.status, "existe déjà") {
		t.Fatalf("status should report the collision, got %q", m.status)
	}
}

func TestCreateSceneUnknownChapterFolder(t *testing.T) {
	m, dir := createFlowModel(t)
	m.createScene("inexistant", "scène perdue")

	if _, err := os.Stat(filepath.Join(dir, "inexistant")); err == nil {
		t.Fatal("no folder should be created for an unknown chapter target")
	}
	if !strings.Contains(m.status, "introuvable") {
		t.Fatalf("status should report the missing chapter, got %q", m.status)
	}
}
```

`strings` is already imported by `create_prompt_test.go`'s package (`main`); if this file
doesn't import it yet, add `"strings"` to its import block.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./... -run TestCreateScene -v`
Expected: FAIL — `createScene` undefined.

- [ ] **Step 3: Write the implementation**

Add to `main.go`, after `createResource` (after line 2369):

```go
// createScene adds a new blank scene to an existing chapter — appends a manifestText to its
// Texts and opens the new file. folder identifies the target chapter (birth-stable, resolved
// by the caller at the ctrl+n picker or from the open text-picker screen); name is the
// scene's display title, slugified into its birth-stable filename.
func (m *model) createScene(folder, name string) {
	if strings.Contains(name, "/") {
		m.status = "un nom de scène ne peut pas contenir de séparateur de chemin"
		return
	}
	title := name
	file := slugify(title) + ".md"
	chDir := filepath.Join(m.files.dir, folder)
	dst := filepath.Join(chDir, file)
	if _, err := os.Stat(dst); err == nil {
		m.status = "une scène nommée " + file + " existe déjà dans ce chapitre"
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
	ch := findChapterByFolder(&mani, folder)
	if ch == nil {
		m.status = "scène créée mais le chapitre est introuvable dans le manifeste"
		return
	}
	ch.Texts = append(ch.Texts, manifestText{File: file, Title: title})
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

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./... -run TestCreateScene -v`
Expected: PASS (4 tests)

- [ ] **Step 5: Commit**

```bash
git add main.go create_prompt_test.go
git commit -m "editor: ajoute createScene pour ajouter une scène à un chapitre existant"
```

---

## Task 3: Wire the `ctrl+n` picker's third option ("s" — scène)

**Files:**
- Modify: `main.go` — `model` struct field (near `createKind`, line 432), `createPicker` key
  handling (`main.go:1223-1243`), `confirmCreate` (`main.go:2385-2393`), the two `createPicker`
  status strings (`main.go:1662`, `main.go:2928`), the `ctrl+n` help line (`main.go:52`)
- Test: `create_prompt_test.go`

**Interfaces:**
- Consumes: `createScene` (Task 2), `m.files.selectedEntryName()` (existing,
  `filelist.go:467`), `chapterByFolder(v manuscriptView, folder string) chapterRef` (existing,
  `filelist.go:453`), `isChapterOf(v manuscriptView, folder string) bool` (existing,
  `manuscript.go:127`).
- Produces: `model.createChapterFolder string` field — holds the target chapter's folder
  between the picker's `s` keypress and `confirmCreate`'s `kind == 3` branch; always reset to
  `""` after use (mirrors `createKind`'s reset in `confirmCreate`, `main.go:2375`).

- [ ] **Step 1: Write the failing tests**

Add to `create_prompt_test.go`:

```go
func TestCtrlNSceneOptionAppearsOnChapterSelection(t *testing.T) {
	m, _ := createFlowModel(t)
	m.files.selectName("a") // "a" is the chapter folder created by createFlowModel
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlN})
	m = nm.(model)

	if !strings.Contains(m.statusBar(), "s scène") {
		t.Fatalf("picker prompt should offer the scene option when a chapter is selected, got %q", m.statusBar())
	}
}

func TestCtrlNSceneOptionAbsentOutsideChapterSelection(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "loose.md"), []byte("x"), 0o644)
	writeManifest(dir, manifest{SchemaVersion: manifestSchemaVersion, Title: "W"})
	fl := newFilelist()
	fl.root, fl.width, fl.height = dir, 30, 30
	fl.SetDir(dir)
	m := model{width: 100, height: 30, files: fl, screen: screenWriting, focus: focusSidebar,
		sidebarVisible: true, editor: textarea.New(), nameInput: textinput.New()}
	m.files.selectName("loose.md")
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlN})
	m = nm.(model)

	if strings.Contains(m.statusBar(), "s scène") {
		t.Fatalf("picker prompt should not offer the scene option outside a chapter selection, got %q", m.statusBar())
	}
}

func TestCtrlNSceneAppendsToSelectedChapter(t *testing.T) {
	m, dir := createFlowModel(t)
	m.files.selectName("a")
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlN})
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}}) // scene
	m = nm.(model)
	m = typeName(m, "deuxième scène")

	mani, _, _ := readManifest(dir)
	texts := mani.Items[0].Chapter.Texts
	if len(texts) != 2 || texts[1].Title != "deuxième scène" {
		t.Fatalf("scene should be appended to chapter %q, got %+v", "a", texts)
	}
}
```

`statusBar` is the exact existing method backing the string at `main.go:2928`
(`func (m model) statusBar() string`, `main.go:2868`) — the function containing the
`if m.createPicker` block edited in Step 7 below.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./... -run TestCtrlNScene -v`
Expected: FAIL — `s scène` never appears in the prompt (picker only knows `c`/`r`).

- [ ] **Step 3: Add the `createChapterFolder` field**

In `main.go`, next to `createKind` (line 432):

```go
	createPicker        bool   // manuscript ctrl+n: choosing chapter vs resource vs scene
	createKind          int    // 0 normal, 1 chapter, 2 resource, 3 scene (set by the picker)
	createChapterFolder string // target chapter's folder when createKind == 3 (set by the picker or the text-picker screen)
```

- [ ] **Step 4: Gate the "s" option on the sidebar selection**

Replace `main.go:1658-1667` (the `case "ctrl+n":` block):

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
			m.createKind = 0
			m.startInPaneCreate()
			return m, textinput.Blink
```

- [ ] **Step 5: Add the "s" key case to the picker handler**

In the `if m.createPicker {` block (`main.go:1224-1243`), add a case alongside `"c"`/`"r"`:

```go
			case "s":
				if m.createChapterFolder == "" {
					return m, nil // defensive: option wasn't offered, ignore stray keypress
				}
				m.createPicker, m.createKind = false, 3
				m.startInPaneCreate()
				return m, textinput.Blink
```

- [ ] **Step 6: Route `kind == 3` in `confirmCreate`**

In `confirmCreate` (`main.go:2385-2393`), add after the `kind == 2` branch:

```go
	if kind == 3 {
		folder := m.createChapterFolder
		m.createChapterFolder = ""
		m.createScene(folder, name)
		return
	}
```

Also reset `m.createChapterFolder = ""` in the `esc` cancellation path of the `creatingFile`
block (`main.go:1251-1258`), mirroring the existing `m.createKind = 0` reset there, so a
cancelled scene-name prompt doesn't leak into the next unrelated create:

```go
			case "esc":
				m.creatingFile = false
				m.creatingInPane = false
				m.creatingFolder = false
				m.createKind = 0
				m.createChapterFolder = ""
				m.nameInput.Blur()
				m.status = "création annulée"
				return m, nil
```

- [ ] **Step 7: Update the dynamic status string** (`main.go:2927-2929`)

```go
	if m.createPicker {
		if m.createChapterFolder != "" {
			return "nouveau : c chapitre · r ressource · s scène · esc annuler"
		}
		return "nouveau : c chapitre · r ressource · esc annuler"
	}
```

- [ ] **Step 8: Update the help line** (`main.go:52`)

```go
  ctrl+n nouveau · chapitre|ressource|scène   r renommer (F2)
```

- [ ] **Step 9: Run tests to verify they pass**

Run: `go test ./... -run 'TestCtrlN' -v`
Expected: PASS, including the two pre-existing `TestCtrlNChapterAppendsToManifest` /
`TestCtrlNResourceLooseAndFolder` / `TestCtrlNOutsideManuscriptUnchanged` tests (no
regressions) plus the three new ones.

- [ ] **Step 10: Commit**

```bash
git add main.go create_prompt_test.go
git commit -m "editor: ajoute l'option scène au picker ctrl+n quand un chapitre est sélectionné"
```

---

## Task 4: Wire scene creation from the text-picker screen

**Files:**
- Modify: `main.go` — `updateTextPicker` (`main.go:193-224`)
- Test: `main_test.go`

**Interfaces:**
- Consumes: `createScene` is NOT called directly here — `ctrl+n` from this screen opens the
  same naming prompt as Task 3 (`m.createKind = 3`, `m.createChapterFolder` set), so
  `confirmCreate`'s existing `kind == 3` branch (Task 3, Step 6) handles the actual creation
  uniformly for both entry points.

- [ ] **Step 1: Write the failing test**

Add to `main_test.go` (near the other `TestTextPicker*` tests, after line 297):

```go
func TestTextPickerCtrlNOpensSceneNamingPrompt(t *testing.T) {
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

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlN})
	mm := updated.(model)

	if !mm.creatingFile {
		t.Fatal("ctrl+n from the text picker should open the naming prompt")
	}
	if mm.createKind != 3 || mm.createChapterFolder != "chapitre-un" {
		t.Fatalf("naming prompt should target chapitre-un as a scene, got kind=%d folder=%q",
			mm.createKind, mm.createChapterFolder)
	}
}

func TestTextPickerCtrlNThenConfirmAddsScene(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "chapitre-un"), 0o755)
	os.WriteFile(filepath.Join(dir, "chapitre-un", "scene-un.md"), []byte("un"), 0o644)
	os.WriteFile(filepath.Join(dir, manifestName), []byte(
		`{"schemaVersion":2,"title":"N","items":[`+
			`{"chapter":{"folder":"chapitre-un","title":"Chapitre Un","texts":[`+
			`{"file":"scene-un.md","title":"Scène Un"}]}}]}`), 0o644)

	t.Setenv("OKASHI_DIR", dir)
	m := initialModel()
	m.files.selectName("chapitre-un")
	m.enterTextPicker()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlN})
	mm := updated.(model)
	mm = typeName(mm, "troisième scène")

	mani, _, _ := readManifest(dir)
	texts := mani.Items[0].Chapter.Texts
	if len(texts) != 2 || texts[1].Title != "troisième scène" {
		t.Fatalf("scene should be appended to chapitre-un, got %+v", texts)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./... -run TestTextPickerCtrlN -v`
Expected: FAIL — `ctrl+n` on `screenTextPicker` does nothing today (`updateTextPicker` has no
such case, and `Update`'s top-level `screenTextPicker` dispatch — `main.go:1219-1221` — routes
every key there, including `ctrl+n`, before the `ctrl+n` case at line 1658 is ever reached).

- [ ] **Step 3: Add the `ctrl+n` case to `updateTextPicker`**

In `updateTextPicker` (`main.go:193-224`), add a case to the `switch km.Type`:

```go
	case tea.KeyCtrlN:
		folder := m.textPickerChapter.folder
		m.textPickerChapter = nil
		m.createChapterFolder = folder
		m.createKind = 3
		m.screen = screenWriting
		m.startInPaneCreate()
		return m, textinput.Blink
```

Place it before the `default` fallthrough (Go's `switch` on `km.Type` has no explicit
`default` here per the existing code — add it as another `case` alongside `tea.KeyUp`,
`tea.KeyDown`, `tea.KeyEnter`, `tea.KeyEsc`). Since `updateTextPicker` returns `(tea.Model,
tea.Cmd)` and the existing cases return `m, nil` at the function's end, this new case's early
`return m, textinput.Blink` requires no other structural change — check the function's exact
current return shape (`main.go:223`, `return m, nil`) and keep the rest of the switch's
fallthrough-to-that-return behavior intact for the other cases.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./... -run TestTextPickerCtrlN -v`
Expected: PASS (2 tests)

- [ ] **Step 5: Run the full text-picker and ctrl+n suites for regressions**

Run: `go test ./... -run 'TestTextPicker|TestCtrlN|TestEnterTextPicker' -v`
Expected: PASS, no regressions in the pre-existing 5 `TestTextPicker*` tests or the Task 3
tests.

- [ ] **Step 6: Commit**

```bash
git add main.go main_test.go
git commit -m "editor: permet de créer une scène depuis l'écran de sélection de scènes"
```

---

## Task 5: Rename "texte" → "scène" in already-shipped UI strings

**Files:**
- Modify: `main.go:235`, `corkboard.go:337`
- Test: `main_test.go:294` (existing test, update expectation)

**Interfaces:** None — pure string rename, no signature changes.

- [ ] **Step 1: Update the failing expectation first**

In `main_test.go`, `TestTextPickerShowsEmptyStateForZeroTextChapter` (around line 294):

```go
	if !strings.Contains(view, "aucune scène") {
		t.Fatalf("an empty chapter's picker must show an empty-state message, got:\n%s", view)
	}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./... -run TestTextPickerShowsEmptyStateForZeroTextChapter -v`
Expected: FAIL — view still contains "aucun texte", not "aucune scène".

- [ ] **Step 3: Rename the two strings**

`main.go:235`:

```go
		b.WriteString("  (aucune scène dans ce chapitre)")
```

`corkboard.go:337`:

```go
				m.status = "ce chapitre n'a pas encore de scène"
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./... -run TestTextPickerShowsEmptyStateForZeroTextChapter -v`
Expected: PASS

- [ ] **Step 5: Search for any other stray "texte" UI string this plan missed**

Run: `grep -n "texte" main.go corkboard.go filelist.go manuscript.go outline.go pager.go 2>/dev/null`

Expected: only Go-identifier occurrences (`manifestText`, `textRef`, comments) — no remaining
user-facing "texte" string. If any turn up, rename them the same way and add/update a covering
test before moving on.

- [ ] **Step 6: Run the full test suite**

Run: `go test ./...`
Expected: PASS, no regressions anywhere in the repo.

- [ ] **Step 7: Commit**

```bash
git add main.go main_test.go corkboard.go
git commit -m "ui: renomme texte en scène dans les chaînes déjà livrées"
```

---

## Task 6: Update CLAUDE.md

**Files:**
- Modify: `CLAUDE.md` — "Project model (the shipped reality)" section

**Interfaces:** None — documentation only.

- [ ] **Step 1: Update the shipped-features bullet describing `ctrl+n`**

In the "Project model" section's bullet list, find the sentence describing `ctrl+n` (`ctrl+n`
in a manuscript → chapter|resource picker...) and extend it:

```
`ctrl+n` in a manuscript → chapter|resource|**scene** picker (resource loose or into a
subfolder); scene only offered when the sidebar selection is a chapter, appends to that
chapter's ordered texts; the text-picker screen (opened on a multi-scene or empty chapter)
also creates a scene via `ctrl+n`.
```

Integrate this into the existing corkboard/manuscript-navigator bullet where `ctrl+n` is
already mentioned (search for `ctrl+n` in CLAUDE.md to find the exact spot — it's inside the
long corkboard bullet describing sidebar keys) rather than adding a disconnected new bullet.

- [ ] **Step 2: Verify no shared-contract section needs touching**

Confirm §1 (Manuscript ordering & membership) is untouched — this feature writes existing
`Texts[]` entries, no new manifest field, no schema shape change. No edit needed there; this
step is a check, not a code change.

- [ ] **Step 3: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: documente la création de scène via ctrl+n dans CLAUDE.md"
```

---

## Task 7: Full regression pass

**Files:** None modified — verification only.

- [ ] **Step 1: Run the full test suite**

Run: `go test ./...`
Expected: PASS, all packages.

- [ ] **Step 2: Run go vet**

Run: `go vet ./...`
Expected: no issues.

- [ ] **Step 3: Build**

Run: `go build ./...`
Expected: succeeds.

- [ ] **Step 4: Manual smoke test**

Run: `go run .` against a scratch manuscript with a manifest v2 chapter (or reuse an existing
test fixture directory copied to a temp location). Verify:
1. Selecting a chapter in the sidebar, `ctrl+n` shows `c chapitre · r ressource · s scène`.
2. Selecting a non-chapter entry, `ctrl+n` shows only `c chapitre · r ressource`.
3. Pressing `s`, typing a name, `enter` — new scene file appears in the chapter's folder, opens
   in the editor.
4. Opening a multi-scene chapter (or an empty one) via `⏎` shows the text-picker; `ctrl+n`
   there opens the same naming prompt; confirming adds the scene and the picker's list would
   show it on next open.
