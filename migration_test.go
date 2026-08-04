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
