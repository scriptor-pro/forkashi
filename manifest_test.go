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
		"schemaVersion": 3, "title": "Windermere",
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
		[]byte(`{"schemaVersion":4,"title":"X","items":[]}`), 0o644)
	_, present, err := readManifest(dir)
	if !present {
		t.Fatal("a present-but-unsupported manifest must report present=true")
	}
	if err == nil {
		t.Fatal("schemaVersion 4 must be refused with an error, not silently read")
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
