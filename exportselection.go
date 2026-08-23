package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
)

const exportSelectionName = ".okashi-export.json"
const exportSelectionSchemaVersion = 1

// exportSelectionFile is the per-manuscript export-inclusion sidecar (okashi-owned, not the
// manifest). Keyed by file path relative to the manuscript dir. A key present means the file is
// EXCLUDED from export; absence means included — default-included, so an untouched manuscript
// exports exactly as it does today.
type exportSelectionFile struct {
	SchemaVersion int             `json:"schemaVersion"`
	Excluded      map[string]bool `json:"excluded"`
}

func exportSelectionPath(dir string) string { return filepath.Join(dir, exportSelectionName) }

// loadExportSelection reads the sidecar; missing/corrupt/unsupported-schema all yield an empty
// map — never an error (mirrors loadSynopses's tolerant load).
func loadExportSelection(dir string) map[string]bool {
	out := map[string]bool{}
	data, err := os.ReadFile(exportSelectionPath(dir))
	if err != nil {
		return out
	}
	var ef exportSelectionFile
	if json.Unmarshal(data, &ef) != nil || ef.SchemaVersion != exportSelectionSchemaVersion {
		return out
	}
	if ef.Excluded != nil {
		out = ef.Excluded
	}
	return out
}

// saveExportSelection writes the sidecar atomically. It prunes keys not in knownFiles
// (self-healing orphans left by a removed/renamed/moved file), so the file only ever holds
// live exclusions. Serialized like saveSynopses: 2-space indent, no HTML escaping, no trailing
// newline.
func saveExportSelection(dir string, excluded map[string]bool, knownFiles map[string]bool) error {
	pruned := map[string]bool{}
	for file, ex := range excluded {
		if ex && knownFiles[file] {
			pruned[file] = true
		}
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(exportSelectionFile{SchemaVersion: exportSelectionSchemaVersion, Excluded: pruned}); err != nil {
		return err
	}
	return atomicWrite(exportSelectionPath(dir), bytes.TrimRight(buf.Bytes(), "\n"), 0o644)
}
