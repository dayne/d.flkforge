// Package source is where snippets come from (SPEC 3): built in
// (embedded), the user's directory, the app's directory, and later the
// hub. Later sources override earlier ones by id.
package source

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dayne/d.flkforge/internal/snippet"
)

// Source lists snippets and gets one by id.
type Source interface {
	Name() string
	List() ([]*snippet.Snippet, error)
}

// FS is a source over a file system laid out as NAMESPACE/NAME/snippet.toml.
// Directories whose names start with _ or . are skipped.
type FS struct {
	Label string
	FS    fs.FS
}

func (s FS) Name() string { return s.Label }

func (s FS) List() ([]*snippet.Snippet, error) {
	namespaces, err := fs.ReadDir(s.FS, ".")
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []*snippet.Snippet
	for _, ns := range namespaces {
		if !ns.IsDir() || skip(ns.Name()) {
			continue
		}
		names, err := fs.ReadDir(s.FS, ns.Name())
		if err != nil {
			return nil, err
		}
		for _, n := range names {
			if !n.IsDir() || skip(n.Name()) {
				continue
			}
			sn, err := snippet.Load(s.FS, path.Join(ns.Name(), n.Name()))
			if err != nil {
				return nil, fmt.Errorf("%s: %w", s.Label, err)
			}
			sn.Origin = s.Label
			out = append(out, sn)
		}
	}
	return out, nil
}

func skip(name string) bool { return strings.HasPrefix(name, "_") || strings.HasPrefix(name, ".") }

// Dir is a source over a directory on disk; a missing directory is empty.
func Dir(dir string) FS {
	return FS{Label: dir, FS: os.DirFS(dir)}
}

// UserDir is ~/.config/flkforge/snippets (or $XDG_CONFIG_HOME's).
func UserDir() string {
	if d, err := os.UserConfigDir(); err == nil {
		return filepath.Join(d, "flkforge", "snippets")
	}
	return ""
}

// AppDir is the app directory's own snippets: DIR/.flkforge/snippets.
func AppDir(appDir string) string { return filepath.Join(appDir, ".flkforge", "snippets") }

// Catalog is every snippet from a list of sources, later ones overriding
// earlier ones by id.
type Catalog struct {
	byID  map[string]*snippet.Snippet
	order []string
}

// Load reads every source.
func Load(sources ...Source) (*Catalog, error) {
	c := &Catalog{byID: map[string]*snippet.Snippet{}}
	for _, src := range sources {
		list, err := src.List()
		if err != nil {
			return nil, err
		}
		for _, s := range list {
			if _, ok := c.byID[s.ID]; !ok {
				c.order = append(c.order, s.ID)
			}
			c.byID[s.ID] = s
		}
	}
	sort.Strings(c.order)
	return c, nil
}

// All returns the snippets, sorted by id.
func (c *Catalog) All() []*snippet.Snippet {
	out := make([]*snippet.Snippet, len(c.order))
	for i, id := range c.order {
		out[i] = c.byID[id]
	}
	return out
}

// Get finds a snippet by id, or by name alone when only one namespace
// has it.
func (c *Catalog) Get(id string) (*snippet.Snippet, error) {
	if s, ok := c.byID[id]; ok {
		return s, nil
	}
	if !strings.Contains(id, "/") {
		var found []*snippet.Snippet
		for _, k := range c.order {
			if path.Base(k) == id {
				found = append(found, c.byID[k])
			}
		}
		switch len(found) {
		case 1:
			return found[0], nil
		case 0:
		default:
			var ids []string
			for _, s := range found {
				ids = append(ids, s.ID)
			}
			return nil, fmt.Errorf("snippet %q is ambiguous: %s", id, strings.Join(ids, ", "))
		}
	}
	return nil, fmt.Errorf("no snippet %q (flkforge snippets lists them)", id)
}
