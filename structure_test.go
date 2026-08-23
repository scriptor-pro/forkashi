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
