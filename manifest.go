package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const manifestName = "manifest.json"
const manifestSchemaVersion = 2

// manifestText is one ordered text (scene) inside a chapter: a bare filename (slug,
// no numeric prefix — order lives in the manifest, not the filename) plus a display title.
type manifestText struct {
	File  string `json:"file"`
	Title string `json:"title"`
}

// manifestChapter is one chapter: a folder (slug, birth-stable — never renamed by a
// retitle or reorder) holding one or more ordered texts.
type manifestChapter struct {
	Folder string         `json:"folder"`
	Title  string         `json:"title"`
	Texts  []manifestText `json:"texts"`
}

// manifestItem is one top-level manuscript entry: EITHER a bare chapter (Chapter set,
// Part empty, Chapters nil) OR a Part grouping its own ordered chapters (Part set,
// Chapters set, Chapter nil). Mutually exclusive — never both.
type manifestItem struct {
	Part     string            `json:"part,omitempty"`
	Chapters []manifestChapter `json:"chapters,omitempty"`
	Chapter  *manifestChapter  `json:"chapter,omitempty"`
}

// manifest is the shared per-manuscript order/membership/title file. okashi reads it AND
// writes it — create + chapter-title retitle today, structural edits later (see design §0).
type manifest struct {
	SchemaVersion int            `json:"schemaVersion"`
	Title         string         `json:"title"`
	Items         []manifestItem `json:"items"`
}

// hasManifest reports whether dir contains a manifest.json — the companion app's manuscript
// marker (design §4: folder with manifest = manuscript).
func hasManifest(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, manifestName))
	return err == nil
}

// readManifest loads dir/manifest.json. When the file is absent it returns
// present=false, err=nil. A present-but-unreadable manifest (malformed JSON or an
// unsupported schemaVersion) returns present=true with a non-nil err: okashi
// REFUSES to guess structure and NEVER writes the file back (design §4.1).
func readManifest(dir string) (m manifest, present bool, err error) {
	data, readErr := os.ReadFile(filepath.Join(dir, manifestName))
	if os.IsNotExist(readErr) {
		return manifest{}, false, nil
	}
	if readErr != nil {
		return manifest{}, true, readErr
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return manifest{}, true, err
	}
	if m.SchemaVersion != manifestSchemaVersion {
		return manifest{}, true, fmt.Errorf(
			"unsupported manifest schemaVersion %d (okashi supports %d)",
			m.SchemaVersion, manifestSchemaVersion)
	}
	return m, true, nil
}

// writeManifest serializes m to dir/manifest.json atomically. okashi owns manifest writes for
// its own AND the shared corpus (design §0); the schema is forced to EXACTLY
// manifestSchemaVersion so the companion app reads it verbatim. The serialization matches the companion app's
// JSONEncoder(.prettyPrinted, .sortedKeys): alphabetically-sorted keys, 2-space indent, no
// trailing newline — so when the two apps alternate writes the NSFileVersion diff stays small
// and legible instead of a whole-file reformat (storage-spine §67-69). Go sorts map keys
// alphabetically, yielding the same items/schemaVersion/title (and file/title) order the companion app emits.
func writeManifest(dir string, m manifest) error {
	m.SchemaVersion = manifestSchemaVersion
	// Force a non-nil slice so an empty manuscript serializes as `[]`, not `null` (which
	// the companion app's [ManifestItem] decode would reject).
	items := m.Items
	if items == nil {
		items = []manifestItem{}
	}
	top := map[string]any{
		"schemaVersion": m.SchemaVersion,
		"title":         m.Title,
		"items":         items,
	}
	// SetEscapeHTML(false): Swift's JSONEncoder emits &, <, > literally; Go's default escapes
	// them to their \uXXXX forms, which would churn the whole file whenever a title contains one
	// ("Tom & Jerry"). Encoder also appends a trailing '\n'; trim it to match Swift (no newline).
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(top); err != nil {
		return err
	}
	return atomicWrite(filepath.Join(dir, manifestName), bytes.TrimRight(buf.Bytes(), "\n"), 0o644)
}
