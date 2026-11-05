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
	"slices"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/dayne/d.flkforge/internal/recipe"
	"github.com/dayne/d.flkforge/internal/snippet"
)

// Source lists snippets, templates and limits.
type Source interface {
	Name() string
	List() ([]*snippet.Snippet, error)
	Templates() ([]*Template, error)
	Limits() ([]Limit, error)
}

// Limit is an ask that isn't a snippet (SPEC 8.1): NAMESPACE/_limits.toml.
type Limit struct {
	ID      string `toml:"id" json:"id"`
	Ask     string `toml:"ask" json:"ask"`
	Group   string `toml:"group" json:"group"` // flk, root, flk-change, impossible
	Why     string `toml:"why" json:"why"`
	Instead string `toml:"instead" json:"instead"`
}

// Limits reads NAMESPACE/_limits.toml.
func (s FS) Limits() ([]Limit, error) {
	namespaces, err := fs.ReadDir(s.FS, ".")
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Limit
	for _, ns := range namespaces {
		if !ns.IsDir() || skip(ns.Name()) {
			continue
		}
		p := path.Join(ns.Name(), "_limits.toml")
		data, err := fs.ReadFile(s.FS, p)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		var f struct {
			Limit []Limit `toml:"limit"`
		}
		md, err := toml.Decode(string(data), &f)
		if err != nil {
			return nil, fmt.Errorf("%s: %s: %w", s.Label, p, err)
		}
		if und := md.Undecoded(); len(und) > 0 {
			return nil, fmt.Errorf("%s: %s: unknown key %s", s.Label, p, und[0])
		}
		for _, l := range f.Limit {
			switch {
			case l.ID == "" || l.Ask == "" || l.Why == "" || l.Instead == "":
				return nil, fmt.Errorf("%s: %s: a limit needs id, ask, why and instead", s.Label, p)
			case !slices.Contains([]string{"flk", "root", "flk-change", "impossible"}, l.Group):
				return nil, fmt.Errorf("%s: %s: %s: group must be flk, root, flk-change or impossible", s.Label, p, l.ID)
			}
			out = append(out, l)
		}
	}
	return out, nil
}

// Template is a starting recipe in the catalog: NAMESPACE/_templates/NAME.toml.
type Template struct {
	ID     string
	Recipe *recipe.Recipe
	Origin string
}

// Templates lists NAMESPACE/_templates/*.toml.
func (s FS) Templates() ([]*Template, error) {
	namespaces, err := fs.ReadDir(s.FS, ".")
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []*Template
	for _, ns := range namespaces {
		if !ns.IsDir() || skip(ns.Name()) {
			continue
		}
		files, err := fs.ReadDir(s.FS, path.Join(ns.Name(), "_templates"))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".toml") {
				continue
			}
			p := path.Join(ns.Name(), "_templates", f.Name())
			data, err := fs.ReadFile(s.FS, p)
			if err != nil {
				return nil, err
			}
			r, err := recipe.Parse(data)
			if err != nil {
				return nil, fmt.Errorf("%s: %s: %w", s.Label, p, err)
			}
			if r.Title == "" || r.Summary == "" {
				return nil, fmt.Errorf("%s: %s: a template needs a title and a summary", s.Label, p)
			}
			out = append(out, &Template{ID: ns.Name() + "/" + strings.TrimSuffix(f.Name(), ".toml"), Recipe: r, Origin: s.Label})
		}
	}
	return out, nil
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
	byID      map[string]*snippet.Snippet
	order     []string
	templates map[string]*Template
	limits    map[string]Limit
}

// Load reads every source.
func Load(sources ...Source) (*Catalog, error) {
	c := &Catalog{byID: map[string]*snippet.Snippet{}, templates: map[string]*Template{}, limits: map[string]Limit{}}
	for _, src := range sources {
		list, err := src.List()
		if err != nil {
			return nil, err
		}
		tl, err := src.Templates()
		if err != nil {
			return nil, err
		}
		for _, t := range tl {
			c.templates[t.ID] = t
		}
		ll, err := src.Limits()
		if err != nil {
			return nil, err
		}
		for _, l := range ll {
			c.limits[l.ID] = l
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

// Templates returns the templates, sorted by id.
func (c *Catalog) Templates() []*Template {
	var out []*Template
	for _, t := range c.templates {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Template finds a template by id, or by name alone when it's unique.
func (c *Catalog) Template(id string) (*Template, error) {
	if t, ok := c.templates[id]; ok {
		return t, nil
	}
	var found []*Template
	for _, t := range c.Templates() {
		if path.Base(t.ID) == id {
			found = append(found, t)
		}
	}
	if len(found) == 1 {
		return found[0], nil
	}
	if len(found) > 1 {
		return nil, fmt.Errorf("template %q is ambiguous: give its namespace", id)
	}
	return nil, fmt.Errorf("no template %q (flkforge templates lists them)", id)
}

// Limits returns the asks that aren't snippets, sorted by id.
func (c *Catalog) Limits() []Limit {
	var out []Limit
	for _, l := range c.limits {
		out = append(out, l)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Limit finds one by id.
func (c *Catalog) Limit(id string) (Limit, bool) {
	l, ok := c.limits[id]
	return l, ok
}
