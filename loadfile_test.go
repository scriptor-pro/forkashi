package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"okashi/internal/textarea"
)

func TestLoadFileNonUTF8Warns(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "latin1.md")
	// "Caf" + 0xE9 (é in Latin-1), a lone high byte that is invalid UTF-8.
	if err := os.WriteFile(file, []byte{0x43, 0x61, 0x66, 0xE9}, 0o644); err != nil {
		t.Fatal(err)
	}
	m := model{editor: textarea.New()}
	m.loadFile(file)

	if m.dirty {
		t.Fatal("loading a non-UTF-8 file must not mark the buffer dirty (no silent re-encode)")
	}
	if !strings.Contains(m.status, "UTF-8") {
		t.Fatalf("expected a UTF-8 warning in the status, got %q", m.status)
	}
}

func TestLoadFileValidUTF8OpensNormally(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "ok.md")
	if err := os.WriteFile(file, []byte("Café au lait"), 0o644); err != nil { // valid UTF-8
		t.Fatal(err)
	}
	m := model{editor: textarea.New()}
	m.loadFile(file)

	if !strings.Contains(m.status, "ouvert") {
		t.Fatalf("a valid UTF-8 file should open normally, got status %q", m.status)
	}
}
