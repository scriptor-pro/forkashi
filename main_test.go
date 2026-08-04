package main

import (
	"os"
	"path/filepath"
	"testing"
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
