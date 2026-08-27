package main

import (
	"os"
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
