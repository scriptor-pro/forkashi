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
