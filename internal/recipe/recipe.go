// Package recipe is flkforge.toml (SPEC 5): the app name, the snippets
// used with their params, the lock, and the hashes of the files the last
// render wrote.
package recipe

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/dayne/d.flkforge/internal/flktoml"
)

// File is the recipe's name in an app directory.
const File = "flkforge.toml"

// Recipe is a parsed flkforge.toml.
type Recipe struct {
	Flkforge string `toml:"flkforge"` // the flkforge that wrote it
	App      string `toml:"app"`
	Template string `toml:"template"` // where it started (informational)
	Uses     []Use  `toml:"use"`
	// Lock: while every snippet is built in, the flkforge version
	// ("flkforge" key) is the lock. Snippets from other sources get an
	// entry each (LockEntry) once there are any (SPEC 5).
	Lock map[string]any `toml:"lock"`
	// Render: the sha256 of each managed file the last render wrote, by
	// path (drift detection, SPEC 6.5).
	Render map[string]string `toml:"render"`
}

// Use is one [[use]]: a snippet and its params.
type Use struct {
	Snippet string         `toml:"snippet"`
	ID      string         `toml:"id"` // needed when a snippet is used more than once
	Params  map[string]any `toml:"params"`
}

// Key is the use's id: its own, or the snippet's name.
func (u Use) Key() string {
	if u.ID != "" {
		return u.ID
	}
	return u.Snippet[strings.LastIndex(u.Snippet, "/")+1:]
}

var appRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// CheckAppName applies flk's rule for app names.
func CheckAppName(name string) error {
	if !appRE.MatchString(name) {
		return fmt.Errorf("app name %q: lowercase letters, digits and dashes, starting with a letter or digit, at most 63 characters (flk's rule)", name)
	}
	return nil
}

// DefaultAppName is the house convention: the directory's name, with dots
// as dashes (foo.flk -> foo-flk).
func DefaultAppName(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = dir
	}
	n := strings.ToLower(filepath.Base(abs))
	n = strings.NewReplacer(".", "-", "_", "-", " ", "-").Replace(n)
	return strings.Trim(n, "-")
}

// Parse reads a recipe.
func Parse(data []byte) (*Recipe, error) {
	var r Recipe
	md, err := toml.Decode(string(data), &r)
	if err != nil {
		return nil, err
	}
	var unknown []string
	for _, k := range md.Undecoded() {
		// params are free-form here; the snippet checks them
		if len(k) >= 3 && k[0] == "use" && k[1] == "params" {
			continue
		}
		unknown = append(unknown, k.String())
	}
	if len(unknown) > 0 {
		return nil, fmt.Errorf("unknown keys: %s", strings.Join(unknown, ", "))
	}
	if r.App != "" {
		if err := CheckAppName(r.App); err != nil {
			return nil, err
		}
	}
	seen := map[string]bool{}
	for i, u := range r.Uses {
		if u.Snippet == "" {
			return nil, fmt.Errorf("[[use]] #%d has no snippet", i+1)
		}
		if seen[u.Key()] {
			return nil, fmt.Errorf("[[use]] %q appears twice: give each use of %s its own id", u.Key(), u.Snippet)
		}
		seen[u.Key()] = true
	}
	return &r, nil
}

// Read reads DIR/flkforge.toml, or the file itself if path is a file.
func Read(path string) (*Recipe, error) {
	if st, err := os.Stat(path); err == nil && st.IsDir() {
		path = filepath.Join(path, File)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	r, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return r, nil
}

const header = "# Written by flkforge. Change it with flkforge's commands or by hand, then\n# run `flkforge render`. The [lock] and [render] tables are flkforge's."

// Format writes the recipe as TOML.
func (r *Recipe) Format() string {
	root := &flktoml.Table{}
	root.Items = append(root.Items, &flktoml.KV{Key: "flkforge", Value: flktoml.Quote(r.Flkforge), Comment: strings.Split(header, "\n")})
	root.Set("app", flktoml.Quote(r.App))
	if r.Template != "" {
		root.Set("template", flktoml.Quote(r.Template))
	}
	for _, u := range r.Uses {
		t, _ := root.Append("use")
		t.Set("snippet", flktoml.Quote(u.Snippet))
		if u.ID != "" {
			t.Set("id", flktoml.Quote(u.ID))
		}
		if len(u.Params) == 0 {
			continue
		}
		multi := false
		for _, v := range u.Params {
			if s, ok := v.(string); ok && strings.Contains(s, "\n") {
				multi = true
			}
		}
		if !multi {
			t.Set("params", flktoml.Encode(u.Params, false))
			continue
		}
		p, _ := t.Sub("params", true)
		for _, k := range sortedKeys(u.Params) {
			p.Set(k, flktoml.Encode(u.Params[k], true))
		}
	}
	if len(r.Lock) > 0 {
		t, _ := root.Sub("lock", true)
		for _, k := range sortedKeys(r.Lock) {
			t.Set(k, flktoml.Encode(r.Lock[k], false))
		}
	}
	if len(r.Render) > 0 {
		t, _ := root.Sub("render", true)
		for _, k := range sortedKeys(r.Render) {
			t.Set(k, flktoml.Quote(r.Render[k]))
		}
	}
	return flktoml.Format(root, nil)
}

// Hash is the drift hash of a file's content.
func Hash(content []byte) string {
	sum := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
