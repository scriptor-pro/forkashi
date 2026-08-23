package main

import (
	"os"
	"path/filepath"
)

type manuscriptSource int

const (
	sourceNone     manuscriptSource = iota // category: no manifest, no numbered files
	sourceManifest                         // manifest.json present and readable
	sourceLegacy                           // no manifest, numbered files (read-only fallback)
)

// textRef is one ordered text (scene) inside a chapter.
type textRef struct {
	file  string
	title string
	words int
}

// chapterRef is one ordered chapter: a folder (empty only for a legacy flat-file chapter,
// pre-v2 display fallback, OR a standalone scene — see scene) holding one or more ordered
// texts. scene == true means folder == "" for a DIFFERENT reason than the legacy fallback: a
// standalone scene has no chapter folder by design (manifest v3), not because it predates
// the manifest.
type chapterRef struct {
	folder string
	title  string
	texts  []textRef
	scene  bool
}

// partRef is one ordered group of chapters. title == "" is the synthetic part
// that holds chapters outside any real Part — every manuscriptView.parts has at
// least this one entry, even when the author never created a Part.
type partRef struct {
	title    string
	chapters []chapterRef
}

// manuscriptView is a folder's resolved structure. One resolver feeds the sidebar,
// outline, pager, and export so they never disagree (design §4; parts §
// 2026-08-04).
type manuscriptView struct {
	source  manuscriptSource
	title   string
	parts   []partRef
	loose   []fileEntry // resources / loose files
	warning string      // non-empty when a manifest was present but unreadable (§4.1)
}

// ordered reports whether the folder renders as an ordered manuscript (manifest or
// legacy). Categories and the resolver's never-ordered states return false.
func (v manuscriptView) ordered() bool {
	return v.source == sourceManifest || v.source == sourceLegacy
}

// filterFiles drops directories, keeping document fileEntry values.
func filterFiles(entries []fileEntry) []fileEntry {
	var out []fileEntry
	for _, e := range entries {
		if !e.isDir {
			out = append(out, e)
		}
	}
	return out
}

// docEntries returns the non-dir document entries (loose display order: alpha,
// matching orderedSections' loose sort) for category folders.
func docEntries(entries []fileEntry) []fileEntry {
	_, loose := orderedSections(filterFiles(entries))
	return loose
}

// isManuscript reports whether dir has a manifest.json — the companion app's manuscript
// marker (design §4). This is the contract-precise definition; use
// hasNumberedSections for the legacy-prefix heuristic.
func isManuscript(dir string) bool {
	return hasManifest(dir)
}

// resolveManuscript decides a folder's structure into one of three mutually
// exclusive states. A readable manifest is the SOLE source of order, titles, and
// membership. For each chapter it lists, its folder's contents are read to
// resolve which listed texts actually exist on disk (§4.2: a truly-absent text or
// chapter folder is omitted from display, never invented). Absent a manifest,
// numbered files fall back to legacy prefix ordering (read-only, single-text
// chapters, folder "").
func resolveManuscript(dir string, entries []fileEntry) manuscriptView {
	m, present, err := readManifest(dir)
	if present {
		if err != nil {
			return manuscriptView{
				source:  sourceManifest,
				title:   projectTitle(filepath.Base(dir)),
				loose:   filterFiles(entries),
				warning: err.Error(),
			}
		}
		return manifestView(dir, m, entries)
	}
	if hasNumberedSections(entries) {
		sections, loose := orderedSections(filterFiles(entries))
		chapters := make([]chapterRef, 0, len(sections))
		for _, s := range sections {
			chapters = append(chapters, chapterRef{
				folder: "",
				title:  sectionTitle(s.name),
				texts:  []textRef{{file: s.name, title: sectionTitle(s.name)}},
			})
		}
		return manuscriptView{
			source: sourceLegacy,
			title:  projectTitle(filepath.Base(dir)),
			parts:  []partRef{{title: "", chapters: chapters}},
			loose:  loose,
		}
	}
	return manuscriptView{
		source: sourceNone,
		title:  projectTitle(filepath.Base(dir)),
		loose:  docEntries(entries),
	}
}

// isChapterOf reports whether folder is a chapter of the given manuscript view, across all
// parts (including the synthetic untitled one). A standalone scene never matches, even
// though its folder is also "" — folder alone can't distinguish a legacy flat-file chapter
// from a scene, so scene entries are filtered out explicitly.
func isChapterOf(v manuscriptView, folder string) bool {
	for _, p := range v.parts {
		for _, c := range p.chapters {
			if c.scene {
				continue
			}
			if c.folder == folder {
				return true
			}
		}
	}
	return false
}

// resolveChapterTexts reads chDir's on-disk .md files and returns, in manifest
// order, the texts from listed that actually exist — a truly-absent text is
// omitted (§4.2 applied one level down).
func resolveChapterTexts(chDir string, listed []manifestText) []textRef {
	onDisk := map[string]bool{}
	if items, err := os.ReadDir(chDir); err == nil {
		for _, it := range items {
			if !it.IsDir() {
				onDisk[it.Name()] = true
			}
		}
	}
	texts := make([]textRef, 0, len(listed))
	for _, t := range listed {
		if onDisk[t.File] {
			texts = append(texts, textRef{file: t.File, title: t.Title})
		}
	}
	return texts
}

// manifestView projects a readable v2 manifest onto on-disk entries: for each
// listed chapter whose folder exists, its texts are resolved via
// resolveChapterTexts; a chapter whose folder is entirely absent is omitted
// (§4.2). Chapters outside any Part are collected under a single synthetic
// partRef{title: ""} so every caller sees exactly one shape.
func manifestView(dir string, m manifest, entries []fileEntry) manuscriptView {
	// entries is a files-only list (readEntries/filelist.go's SetDir both exclude
	// directories from what they hand resolveManuscript), so chapter-folder
	// presence can't be read off entries — it's checked directly on disk here,
	// the same way resolveChapterTexts reads a chapter folder's own contents.
	resolve := func(mc manifestChapter) (chapterRef, bool) {
		if mc.Scene {
			if len(mc.Texts) == 0 {
				return chapterRef{}, false
			}
			if _, err := os.Stat(filepath.Join(dir, mc.Texts[0].File)); err != nil {
				return chapterRef{}, false
			}
			return chapterRef{
				title: mc.Title,
				texts: []textRef{{file: mc.Texts[0].File, title: mc.Texts[0].Title}},
				scene: true,
			}, true
		}
		info, err := os.Stat(filepath.Join(dir, mc.Folder))
		if err != nil || !info.IsDir() {
			return chapterRef{}, false
		}
		return chapterRef{
			folder: mc.Folder,
			title:  mc.Title,
			texts:  resolveChapterTexts(filepath.Join(dir, mc.Folder), mc.Texts),
		}, true
	}

	var parts []partRef
	var bare []chapterRef
	for _, it := range m.Items {
		if it.Chapter != nil {
			if cr, ok := resolve(*it.Chapter); ok {
				bare = append(bare, cr)
			}
			continue
		}
		var chapters []chapterRef
		for _, mc := range it.Chapters {
			if cr, ok := resolve(mc); ok {
				chapters = append(chapters, cr)
			}
		}
		parts = append(parts, partRef{title: it.Part, chapters: chapters})
	}
	// The synthetic untitled part (bare chapters) always comes first, matching
	// manifest order semantics where bare chapters and real Parts interleave —
	// v1 of this feature keeps bare-chapter items and Part items in the SAME
	// items[] order; a bare chapter appended by manifestInsert lands wherever
	// its item sits, so this loop above already preserves interleaving via
	// `parts` and `bare` in item order — collect bare into a leading synthetic
	// part only when there is at least one, so a manuscript with zero bare
	// chapters doesn't grow a spurious empty part.
	all := parts
	if len(bare) > 0 || len(parts) == 0 {
		all = append([]partRef{{title: "", chapters: bare}}, parts...)
	}

	loose := looseFiles(m, entries)
	title := m.Title
	if title == "" {
		title = projectTitle(filepath.Base(dir))
	}
	return manuscriptView{source: sourceManifest, title: title, parts: all, loose: loose}
}

// looseFiles returns every root-level .md file not referenced as a text inside any listed
// chapter, AND not referenced by a standalone scene — a Resource (design: unlisted files are
// Resources). Ordinary v2 chapters are always folders, so a root-level .md file can only ever
// collide with a standalone scene (v3), never with an ordinary chapter's text.
func looseFiles(m manifest, entries []fileEntry) []fileEntry {
	sceneFiles := map[string]bool{}
	for _, it := range m.Items {
		if it.Chapter != nil && it.Chapter.Scene && len(it.Chapter.Texts) > 0 {
			sceneFiles[it.Chapter.Texts[0].File] = true
		}
	}
	var out []fileEntry
	for _, e := range entries {
		if !e.isDir && !sceneFiles[e.name] {
			out = append(out, e)
		}
	}
	return out
}
