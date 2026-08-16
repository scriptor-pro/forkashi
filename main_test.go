package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// These tests set OKASHI_DIR before calling initialModel(), rather than calling
// m.files.SetDir(dir) after (the task-4 brief's literal draft) — filelist.SetDir
// confines navigation to f.root (set from writingDir() inside initialModel(), which
// itself resolves from OKASHI_DIR), so a post-hoc SetDir to a t.TempDir() outside that
// root is silently redirected back to f.root and never actually points m.files.dir at
// the fixture. Setting OKASHI_DIR first — the pattern already used throughout this
// codebase's own tests (home_test.go, outline_test.go, dict_test.go, …) — makes root
// and dir agree from the start. Verified empirically: the brief's literal pattern made
// TestEnterWritingFlagsV1ManifestForMigration and TestConfirmMigrationExecutesAndEntersWriting
// fail (migrationPending stayed nil because m.files.dir never left the real writingDir()).
func TestEnterWritingFlagsV1ManifestForMigration(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-un.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, manifestName),
		[]byte(`{"schemaVersion":1,"title":"N","items":[{"file":"01-un.md","title":"Un"}]}`), 0o644)

	t.Setenv("OKASHI_DIR", dir)
	m := initialModel()
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

	t.Setenv("OKASHI_DIR", dir)
	m := initialModel()
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

	t.Setenv("OKASHI_DIR", dir)
	m := initialModel()
	m.enterWriting()
	if m.migrationPending == nil {
		t.Fatal("setup: expected migrationPending")
	}

	statusBeforeConfirm := m.status

	m.confirmMigration()

	if m.migrationPending != nil {
		t.Fatal("confirmMigration must clear migrationPending")
	}
	if m.screen != screenWriting {
		t.Fatal("confirmMigration must enter the writing screen")
	}
	if m.status != statusBeforeConfirm {
		t.Fatalf("confirmMigration must not touch m.status on a successful migration, got: %q, want unchanged from: %q", m.status, statusBeforeConfirm)
	}
	if _, err := os.Stat(filepath.Join(dir, "un", "un.md")); err != nil {
		t.Fatalf("migrated file must exist: %v", err)
	}

	// Regression coverage for the Task 6 fix (commit 25cb692): confirmMigration
	// must refresh the sidebar (m.files.SetDir) after migrating, not just leave
	// migration's on-disk effect for a future manual reload. Before that fix,
	// m.files.entries still listed the old flat v1 filename here.
	foundOldFlatFile := false
	foundNewChapterFolder := false
	for _, e := range m.files.entries {
		if e.name == "01-un.md" {
			foundOldFlatFile = true
		}
		if e.name == "un" && e.isDir {
			foundNewChapterFolder = true
		}
	}
	if foundOldFlatFile {
		t.Fatalf("m.files.entries must not still list the old v1 flat file after migration, got: %+v", m.files.entries)
	}
	if !foundNewChapterFolder {
		t.Fatalf("m.files.entries must list the new v2 chapter folder after migration, got: %+v", m.files.entries)
	}
}

// TestConfirmMigrationShowsErrorStatusOnFailure reproduces the real bug: before
// this fix, confirmMigration discarded migrateV1ToV2's error with `_ =`, so a v1
// manifest referencing a missing/typo'd filename failed silently — the user saw
// no indication anything went wrong, and the migration prompt would return on
// every subsequent launch with no clue why. After the fix, the error surfaces in
// m.status, naming the offending file.
func TestConfirmMigrationShowsErrorStatusOnFailure(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-un.md"), []byte("x"), 0o644)
	// "missing.md" is declared but never created on disk — migration will fail.
	os.WriteFile(filepath.Join(dir, manifestName),
		[]byte(`{"schemaVersion":1,"title":"N","items":[
			{"file":"01-un.md","title":"Un"},{"file":"missing.md","title":"Deux"}]}`), 0o644)

	t.Setenv("OKASHI_DIR", dir)
	m := initialModel()
	m.enterWriting()
	if m.migrationPending == nil {
		t.Fatal("setup: expected migrationPending")
	}

	m.confirmMigration()

	if m.migrationPending != nil {
		t.Fatal("confirmMigration must clear migrationPending even on failure")
	}
	if m.screen != screenWriting {
		t.Fatal("confirmMigration must enter the writing screen even on failure")
	}
	if !strings.Contains(m.status, "missing.md") {
		t.Fatalf("m.status must name the missing file after a failed migration, got: %q", m.status)
	}

	// The manifest must remain v1 — migration must not have partially succeeded.
	data, err := os.ReadFile(filepath.Join(dir, manifestName))
	if err != nil {
		t.Fatal(err)
	}
	if !contains(string(data), `"schemaVersion":1`) {
		t.Fatalf("manifest must still be v1 after a failed migration, got: %s", data)
	}
}

func TestCancelMigrationLeavesV1ManifestUntouched(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-un.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, manifestName),
		[]byte(`{"schemaVersion":1,"title":"N","items":[{"file":"01-un.md","title":"Un"}]}`), 0o644)

	t.Setenv("OKASHI_DIR", dir)
	m := initialModel()
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

	if !strings.Contains(view, "aucune scène") {
		t.Fatalf("an empty chapter's picker must show an empty-state message, got:\n%s", view)
	}
}

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
