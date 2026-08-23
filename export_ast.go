package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	xast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
)

// exportParser parses with GFM + footnotes so the export matches the shared corpus flavor
// (CLAUDE.md shared-contract §2). Built once.
var exportParser = goldmark.New(goldmark.WithExtensions(extension.GFM, extension.Footnote)).Parser()

// ExportStyle selects the render-time typography; the AST is style-agnostic.
type ExportStyle int

const (
	StyleManuscript ExportStyle = iota
	StyleTufte
)

// Meta is the document-level metadata both writers stamp in.
type Meta struct {
	Author  string
	Title   string
	Contact string // OKASHI_CONTACT: free-text address/email block for the title page
	// TitlePage requests a Shunn-style manuscript title page (whole-manuscript exports only).
	TitlePage bool
}

// Run is a span of text with emphasis flags.
type Run struct {
	Text   string
	Bold   bool
	Italic bool
}

// Block is one prose block; the goldmark walk produces these, both writers consume them.
type Block interface{ isBlock() }

type Paragraph struct{ Runs []Run }
type Heading struct {
	Level int
	Runs  []Run
}
type Blockquote struct{ Children []Block }
type List struct {
	Ordered bool
	Start   int
	Items   []Paragraph
}
type SceneBreak struct{}

// Endnote is one footnote, collected into a chapter's Endnotes.
type Endnote struct {
	Num  int
	Runs []Run
}

// Endnotes is the chapter's footnote bodies, rendered as a "Notes" section at the end.
type Endnotes struct{ Items []Endnote }

func (Paragraph) isBlock()  {}
func (Heading) isBlock()    {}
func (Blockquote) isBlock() {}
func (List) isBlock()       {}
func (SceneBreak) isBlock() {}
func (Endnotes) isBlock()   {}

// Section is one chapter; Title comes from the FILENAME, never the content.
type Section struct {
	Title  string
	Blocks []Block
}

// ManuscriptDoc is the whole export payload — one Section for a single doc, or one per
// ordered section for a whole manuscript.
type ManuscriptDoc []Section

// parseSection parses a section's markdown into our block subset.
func parseSection(src []byte) []Block {
	root := exportParser.Parse(text.NewReader(src))
	var blocks []Block
	first := true
	for n := root.FirstChild(); n != nil; n = n.NextSibling() {
		b, drop := blockFrom(n, src, first)
		first = false
		if drop || b == nil {
			continue
		}
		blocks = append(blocks, b)
	}
	return blocks
}

// blockFrom converts one top-level goldmark node into a Block. drop=true skips it
// (a leading H1 that would duplicate the filename title).
func blockFrom(n ast.Node, src []byte, isFirst bool) (Block, bool) {
	switch t := n.(type) {
	case *ast.Heading:
		if t.ChildCount() == 0 { // a lone "#"
			return SceneBreak{}, false
		}
		if isFirst && t.Level == 1 {
			return nil, true
		}
		return Heading{Level: t.Level, Runs: inlineRuns(n, src, 0)}, false
	case *ast.ThematicBreak:
		return SceneBreak{}, false
	case *ast.Paragraph, *ast.TextBlock:
		return Paragraph{Runs: inlineRuns(n, src, 0)}, false
	case *ast.Blockquote:
		var ch []Block
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			b, drop := blockFrom(c, src, false)
			if !drop && b != nil {
				ch = append(ch, b)
			}
		}
		return Blockquote{Children: ch}, false
	case *ast.List:
		lst := List{Ordered: t.IsOrdered()}
		if lst.Ordered {
			lst.Start = t.Start
		}
		for li := n.FirstChild(); li != nil; li = li.NextSibling() {
			lst.Items = append(lst.Items, Paragraph{Runs: itemRuns(li, src)})
		}
		return lst, false
	case *xast.FootnoteList:
		var en Endnotes
		for f := n.FirstChild(); f != nil; f = f.NextSibling() {
			fn, ok := f.(*xast.Footnote)
			if !ok {
				continue
			}
			en.Items = append(en.Items, Endnote{Num: fn.Index, Runs: itemRuns(f, src)})
		}
		if len(en.Items) == 0 {
			return nil, true
		}
		return en, false
	case *xast.Table:
		var rows []Block
		for r := n.FirstChild(); r != nil; r = r.NextSibling() {
			var cells []string
			for c := r.FirstChild(); c != nil; c = c.NextSibling() {
				cells = append(cells, plainText(inlineRuns(c, src, 0)))
			}
			rows = append(rows, Paragraph{Runs: []Run{{Text: strings.Join(cells, " | ")}}})
		}
		return Blockquote{Children: rows}, false
	default:
		runs := inlineRuns(n, src, 0)
		if len(runs) == 0 {
			return nil, true
		}
		return Paragraph{Runs: runs}, false
	}
}

// inlineRuns flattens inline children into styled runs. emph is a bitmask: 1=italic, 2=bold.
// Link/Image/CodeSpan/unknown degrade to their child text.
func inlineRuns(n ast.Node, src []byte, emph int) []Run {
	var runs []Run
	bold, italic := emph&2 != 0, emph&1 != 0
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		switch t := c.(type) {
		case *ast.Text:
			runs = append(runs, Run{Text: string(t.Segment.Value(src)), Bold: bold, Italic: italic})
			if t.SoftLineBreak() {
				runs = append(runs, Run{Text: " ", Bold: bold, Italic: italic})
			}
		case *ast.String:
			runs = append(runs, Run{Text: string(t.Value), Bold: bold, Italic: italic})
		case *ast.Emphasis:
			bit := 1
			if t.Level == 2 {
				bit = 2
			}
			runs = append(runs, inlineRuns(c, src, emph|bit)...)
		case *xast.FootnoteLink:
			runs = append(runs, Run{Text: fmt.Sprintf("[%d]", t.Index), Bold: bold, Italic: italic})
		case *xast.TaskCheckBox:
			mark := "[ ] "
			if t.IsChecked {
				mark = "[x] "
			}
			runs = append(runs, Run{Text: mark, Bold: bold, Italic: italic})
		case *ast.AutoLink:
			runs = append(runs, Run{Text: string(t.Label(src)), Bold: bold, Italic: italic})
		default:
			runs = append(runs, inlineRuns(c, src, emph)...)
		}
	}
	return runs
}

// itemRuns gathers a list item's text. A list item wraps one or more block
// children (TextBlock/Paragraph); join their inline runs with a space so a
// multi-paragraph item doesn't fuse its words.
func itemRuns(li ast.Node, src []byte) []Run {
	var runs []Run
	for c := li.FirstChild(); c != nil; c = c.NextSibling() {
		if len(runs) > 0 {
			runs = append(runs, Run{Text: " "})
		}
		runs = append(runs, inlineRuns(c, src, 0)...)
	}
	return runs
}

// manuscriptDoc builds the doc from ordered section files (loose already excluded by the
// caller). Title comes from each filename via sectionTitle.
func manuscriptDoc(dir string, sections []fileEntry) ManuscriptDoc {
	var doc ManuscriptDoc
	for _, s := range sections {
		data, err := os.ReadFile(filepath.Join(dir, s.name))
		if err != nil {
			continue
		}
		doc = append(doc, Section{Title: sectionTitle(s.name), Blocks: parseSection(data)})
	}
	return doc
}

// manuscriptDocFromChapters builds the export doc from a resolved parts list, flattened across
// parts (Part headers are the next plan's job — this plan keeps export mechanically correct but
// visually flat). Every text of every chapter is read — each scene becomes its own Section,
// titled with the SCENE's title (not the chapter's), a deliberate granularity change from the
// previous "first scene only" behavior. excluded is keyed by file path relative to dir
// (filepath.Join(ch.folder, text.file) — ch.folder == "" for a standalone scene, collapsing
// correctly via filepath.Join's empty-element handling); nil excluded means nothing is excluded.
// An empty chapter (no texts) produces a Section with empty Blocks rather than being skipped.
func manuscriptDocFromChapters(dir string, parts []partRef, excluded map[string]bool) ManuscriptDoc {
	var doc ManuscriptDoc
	for _, p := range parts {
		for _, ch := range p.chapters {
			if len(ch.texts) == 0 {
				doc = append(doc, Section{Title: ch.title, Blocks: nil})
				continue
			}
			for _, t := range ch.texts {
				file := filepath.Join(ch.folder, t.file)
				if excluded[file] {
					continue
				}
				data, err := os.ReadFile(filepath.Join(dir, file))
				if err != nil {
					continue
				}
				doc = append(doc, Section{Title: t.title, Blocks: parseSection(data)})
			}
		}
	}
	return doc
}
