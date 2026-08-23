package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseSectionBlocksAndRuns(t *testing.T) {
	src := []byte("# Dropped Title\n\nHello **bold** and *italic* world.\n\n---\n\nNext para.\n")
	blocks := parseSection(src)
	// Leading H1 dropped -> [Paragraph, SceneBreak, Paragraph]
	if len(blocks) != 3 {
		t.Fatalf("got %d blocks, want 3 (H1 dropped): %#v", len(blocks), blocks)
	}
	p, ok := blocks[0].(Paragraph)
	if !ok {
		t.Fatalf("block 0 should be a Paragraph, got %T", blocks[0])
	}
	var bold, italic bool
	for _, r := range p.Runs {
		if r.Text == "bold" && r.Bold {
			bold = true
		}
		if r.Text == "italic" && r.Italic {
			italic = true
		}
	}
	if !bold || !italic {
		t.Fatalf("paragraph runs should mark bold+italic: %#v", p.Runs)
	}
	if _, ok := blocks[1].(SceneBreak); !ok {
		t.Fatalf("--- should be a SceneBreak, got %T", blocks[1])
	}
}

func TestParseSectionSoftBreakKeepsSpace(t *testing.T) {
	// A soft-wrapped paragraph: goldmark strips the newline; we must re-insert a space.
	blocks := parseSection([]byte("alpha\nbeta\n"))
	p := blocks[0].(Paragraph)
	var joined string
	for _, r := range p.Runs {
		joined += r.Text
	}
	if joined != "alpha beta" {
		t.Fatalf("soft break should keep a space: %q", joined)
	}
}

func TestParseSectionLoneHashIsSceneBreak(t *testing.T) {
	blocks := parseSection([]byte("para one\n\n#\n\npara two\n"))
	if _, ok := blocks[1].(SceneBreak); !ok {
		t.Fatalf("a lone # should be a SceneBreak, got %T", blocks[1])
	}
}

func TestParseSectionList(t *testing.T) {
	blocks := parseSection([]byte("- first item\n- second **bold** item\n"))
	var lst List
	found := false
	for _, b := range blocks {
		if l, ok := b.(List); ok {
			lst, found = l, true
		}
	}
	if !found {
		t.Fatal("expected a List block")
	}
	if len(lst.Items) != 2 {
		t.Fatalf("expected 2 list items, got %d", len(lst.Items))
	}
	var first string
	for _, r := range lst.Items[0].Runs {
		first += r.Text
	}
	if first != "first item" {
		t.Fatalf("item 0 text = %q, want \"first item\"", first)
	}
	var bold bool
	for _, r := range lst.Items[1].Runs {
		if r.Text == "bold" && r.Bold {
			bold = true
		}
	}
	if !bold {
		t.Fatalf("item 1 should have a bold run: %#v", lst.Items[1].Runs)
	}
}

func TestManuscriptDocExcludesLooseAndTitlesFromFilename(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-opening.md"), []byte("first"), 0o644)
	os.WriteFile(filepath.Join(dir, "02-the-letter.md"), []byte("second"), 0o644)
	os.WriteFile(filepath.Join(dir, "notes.md"), []byte("loose"), 0o644)
	sections, _ := orderedSections([]fileEntry{
		{name: "01-opening.md"}, {name: "02-the-letter.md"}, {name: "notes.md"},
	})
	doc := manuscriptDoc(dir, sections)
	if len(doc) != 2 {
		t.Fatalf("manuscriptDoc should have 2 sections (loose excluded), got %d", len(doc))
	}
	if doc[0].Title != "opening" || doc[1].Title != "the letter" {
		t.Fatalf("titles should come from the filename: %q, %q", doc[0].Title, doc[1].Title)
	}
}

func TestParseSectionFootnoteEndnotes(t *testing.T) {
	src := []byte("She paused[^a] there.\n\n[^a]: A note about pausing.\n")
	blocks := parseSection(src)
	// The body paragraph carries a [1] marker run.
	p, ok := blocks[0].(Paragraph)
	if !ok {
		t.Fatalf("block 0 should be a Paragraph, got %T", blocks[0])
	}
	var marker bool
	for _, r := range p.Runs {
		if r.Text == "[1]" {
			marker = true
		}
	}
	if !marker {
		t.Fatalf("footnote ref should render a [1] marker run: %#v", p.Runs)
	}
	// The last block is the chapter's Endnotes.
	last := blocks[len(blocks)-1]
	en, ok := last.(Endnotes)
	if !ok {
		t.Fatalf("last block should be Endnotes, got %T", last)
	}
	if len(en.Items) != 1 || en.Items[0].Num != 1 {
		t.Fatalf("expected 1 endnote numbered 1, got %#v", en.Items)
	}
	var body string
	for _, r := range en.Items[0].Runs {
		body += r.Text
	}
	if body != "A note about pausing." {
		t.Fatalf("endnote body = %q", body)
	}
}

func TestParseSectionTaskListAndStrike(t *testing.T) {
	blocks := parseSection([]byte("- [ ] todo\n- [x] done\n"))
	lst, ok := blocks[0].(List)
	if !ok {
		t.Fatalf("block 0 should be a List, got %T", blocks[0])
	}
	first, second := "", ""
	for _, r := range lst.Items[0].Runs {
		first += r.Text
	}
	for _, r := range lst.Items[1].Runs {
		second += r.Text
	}
	if first != "[ ] todo" || second != "[x] done" {
		t.Fatalf("task list items = %q / %q, want '[ ] todo' / '[x] done'", first, second)
	}

	// Strikethrough degrades to plain text (handled by the default recurse).
	sb := parseSection([]byte("a ~~struck~~ b"))
	var joined string
	for _, r := range sb[0].(Paragraph).Runs {
		joined += r.Text
	}
	if joined != "a struck b" {
		t.Fatalf("strikethrough should degrade to plain text: %q", joined)
	}
}

func TestParseSectionTableDegrades(t *testing.T) {
	src := []byte("| A | B |\n|---|---|\n| 1 | 2 |\n")
	blocks := parseSection(src)
	bq, ok := blocks[0].(Blockquote)
	if !ok {
		t.Fatalf("a table should degrade to a Blockquote of rows, got %T", blocks[0])
	}
	// header row "A | B", body row "1 | 2"
	row0 := bq.Children[0].(Paragraph)
	var r0 string
	for _, r := range row0.Runs {
		r0 += r.Text
	}
	if r0 != "A | B" {
		t.Fatalf("first table row = %q, want 'A | B'", r0)
	}
}

func TestParseSectionAutolinkSurvives(t *testing.T) {
	for _, src := range []string{"see https://example.com here", "mail me@example.com now"} {
		blocks := parseSection([]byte(src))
		var joined string
		for _, r := range blocks[0].(Paragraph).Runs {
			joined += r.Text
		}
		if !strings.Contains(joined, "example.com") {
			t.Fatalf("autolink text dropped for %q -> %q", src, joined)
		}
	}
}

func TestManuscriptDocFromChaptersIncludesStandaloneScene(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "aparte.md"), []byte("Some aside text."), 0o644)
	parts := []partRef{{title: "", chapters: []chapterRef{
		{title: "Aparté", scene: true, texts: []textRef{{file: "aparte.md", title: "Aparté"}}},
	}}}
	doc := manuscriptDocFromChapters(dir, parts)
	if len(doc) != 1 {
		t.Fatalf("want 1 section, got %d: %+v", len(doc), doc)
	}
	if doc[0].Title != "Aparté" {
		t.Fatalf("section title = %q, want %q", doc[0].Title, "Aparté")
	}
	if len(doc[0].Blocks) == 0 {
		t.Fatal("standalone scene's content must have been read and parsed, got zero blocks")
	}
}
