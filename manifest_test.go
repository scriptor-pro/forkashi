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
