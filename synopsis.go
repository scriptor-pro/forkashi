package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

const synopsisName = ".okashi-synopsis.json"
const synopsisSchemaVersion = 1

// synopsisFile is the per-manuscript synopsis sidecar (okashi-owned, NOT the manifest — the shared
// contract HARD GATE stays untriggered). Keyed by corkKey(chapterRef) (corkboard.go) →
// synopsis text: a chapter's bare folder, or a standalone scene's "scene:"-prefixed filename.
type synopsisFile struct {
	SchemaVersion int               `json:"schemaVersion"`
	Synopses      map[string]string `json:"synopses"`
}

func synopsisPath(dir string) string { return filepath.Join(dir, synopsisName) }

// firstProseLine returns a file's first non-blank, non-heading line (skipping a leading `#` markdown
// heading and blank lines) — used as a dimmed fallback preview on the corkboard when a chapter has
// no authored synopsis. "" if the file is empty/unreadable or has no prose line.
func firstProseLine(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		return t
	}
	return ""
}

// loadSynopses reads the sidecar; missing/corrupt/unsupported-schema all yield an empty map — never
// an error (mirrors recent.json's tolerant load).
func loadSynopses(dir string) map[string]string {
	out := map[string]string{}
	data, err := os.ReadFile(synopsisPath(dir))
	if err != nil {
		return out
	}
	var sf synopsisFile
	if json.Unmarshal(data, &sf) != nil || sf.SchemaVersion != synopsisSchemaVersion {
		return out
	}
	if sf.Synopses != nil {
		out = sf.Synopses
	}
	return out
}

// saveSynopses writes the sidecar atomically. It prunes keys not in chapterSet (self-healing
// orphans left by a removed/renamed chapter) and drops empty synopses, so the file only ever holds
// live, non-empty entries. Serialized like writeManifest: 2-space indent, no HTML escaping, no
// trailing newline — diff-legible if it ever churns.
func saveSynopses(dir string, syn map[string]string, chapterSet map[string]bool) error {
	pruned := map[string]string{}
	for file, text := range syn {
		if chapterSet[file] && text != "" {
			pruned[file] = text
		}
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(synopsisFile{SchemaVersion: synopsisSchemaVersion, Synopses: pruned}); err != nil {
		return err
	}
	return atomicWrite(synopsisPath(dir), bytes.TrimRight(buf.Bytes(), "\n"), 0o644)
}
