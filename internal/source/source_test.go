package source

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dayne/d.flkforge/snippets"
)

// Every built-in snippet loads and validates.
func TestBuiltinsLoad(t *testing.T) {
	c, err := Load(FS{Label: "built in", FS: snippets.FS})
	if err != nil {
		t.Fatal(err)
	}
	if len(c.All()) < 8 {
		t.Errorf("only %d built-in snippets", len(c.All()))
	}
	for _, s := range c.All() {
		if s.Hints.What == "" || len(s.Hints.Docs) == 0 {
			t.Errorf("%s: hints need what and docs", s.ID)
		}
	}
}

// A user's snippet overrides a built-in one with the same id; a missing
// directory is just empty.
func TestOverride(t *testing.T) {
	dir := t.TempDir()
	sd := filepath.Join(dir, "core", "vm")
	os.MkdirAll(sd, 0o755)
	os.WriteFile(filepath.Join(sd, "snippet.toml"), []byte(`id = "core/vm"
version = "9.0.0"
title = "Mine"
category = "vm"
summary = "Mine."
`), 0o644)
	c, err := Load(FS{Label: "built in", FS: snippets.FS}, Dir(dir), Dir(filepath.Join(dir, "missing")))
	if err != nil {
		t.Fatal(err)
	}
	s, err := c.Get("vm")
	if err != nil || s.Version != "9.0.0" || s.Origin != dir {
		t.Errorf("got %v %v", s, err)
	}
}
