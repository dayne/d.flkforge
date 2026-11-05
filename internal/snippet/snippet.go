// Package snippet is the snippet format (SPEC 4): a directory with a
// snippet.toml (metadata, typed params, Dockerfile and flk.toml fragments,
// extra files, hints, next steps) and optional files/ and README.md.
package snippet

import (
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
)

// Categories, in the order the wizard and `snippets` list them.
var Categories = []string{"base", "fetch", "wrapper", "user", "data", "network", "check", "backup", "restart", "vm", "env", "misc"}

// Slots of the final Dockerfile stage, in order (SPEC 6.1).
var Slots = []string{"header", "from", "packages", "env", "fetch", "install", "copy", "dirs", "user", "entry"}

// Snippet is one parsed snippet.toml.
type Snippet struct {
	ID        string   `toml:"id"`
	Version   string   `toml:"version"`
	Title     string   `toml:"title"`
	Category  string   `toml:"category"`
	Summary   string   `toml:"summary"`
	Tags      []string `toml:"tags"`
	Requires  []string `toml:"requires"`  // ids, or "category:NAME"
	Conflicts []string `toml:"conflicts"` // ids, or "category:NAME"
	Suggests  []string `toml:"suggests"`
	// Multi: the snippet can be used more than once in a recipe, each use
	// with its own id (a service, a volume).
	Multi  bool   `toml:"multi"`
	Source string `toml:"source"` // where the pattern was learned (docs only)

	Params     map[string]*Param `toml:"params"`
	ParamOrder []string          `toml:"-"` // params in the order written

	Base     *Base  `toml:"base"`
	Packages string `toml:"packages"` // a template: package names, space-separated
	// User: a template naming the user the app runs as (core/user-1000).
	// At most one snippet in a recipe sets it; the others see it as .App.User.
	User       string            `toml:"user"`
	Dockerfile []DockerFragment  `toml:"dockerfile"`
	FlkToml    []FlkFragment     `toml:"flktoml"`
	Files      []FileFragment    `toml:"file"`
	Merge      map[string]string `toml:"merge"` // flk.toml key path -> merge rule ("max")
	Hints      Hints             `toml:"hints"`
	Next       []Next            `toml:"next"`

	// Set by the loader
	Dir    string `toml:"-"` // the snippet's directory in its FS
	FS     fs.FS  `toml:"-"`
	Readme string `toml:"-"`
	Origin string `toml:"-"` // which source it came from ("built in", a directory)
}

// Name is the id without its namespace.
func (s *Snippet) Name() string { return path.Base(s.ID) }

// Base is what a base snippet tells the others about its image.
type Base struct {
	Image string `toml:"image"` // a template, full name: docker.io/library/alpine:{{.P.version}}
	// Family is the package manager: apt, apk or dnf.
	Family string `toml:"family"`
	// User is the image's uid-1000 user, if it has one (ubuntu).
	User string `toml:"user"`
	// ClearEntrypoint: the image sets an ENTRYPOINT that must be cleared.
	ClearEntrypoint bool `toml:"clear_entrypoint"`
	// Busybox: no setpriv or chroot --userspec, so a non-root USER needs a package (SPEC 6.4).
	Busybox bool `toml:"busybox"`
}

// DockerFragment adds lines to one slot of the Dockerfile.
type DockerFragment struct {
	Stage string `toml:"stage"` // its own build stage; empty: the final stage
	Slot  string `toml:"slot"`
	Text  string `toml:"text"`
	When  string `toml:"when"`
}

// FlkFragment is commented TOML text merged into flk.toml (SPEC 6.2).
type FlkFragment struct {
	Text string `toml:"text"`
	When string `toml:"when"`
}

// Ownership of a written file.
const (
	// Managed files are flkforge's: re-rendered, and checked for drift.
	Managed = "managed"
	// Seed files are written once, when missing, and are then the admin's.
	Seed = "seed"
)

// FileFragment is an extra file written into the app directory.
type FileFragment struct {
	Path     string `toml:"path"`     // relative to the app directory; a template
	Mode     string `toml:"mode"`     // octal, default "644"
	Text     string `toml:"text"`     // inline template
	Template string `toml:"template"` // or a template in the snippet's files/
	Verbatim bool   `toml:"verbatim"` // copy Template as is, no templating
	Own      string `toml:"own"`      // managed (default) or seed
	When     string `toml:"when"`
}

// Hints are the snippet's documentation (SPEC 2.1).
type Hints struct {
	What         string   `toml:"what"`
	UnderTheHood string   `toml:"under_the_hood"`
	Gotchas      []string `toml:"gotchas"`
	Docs         []string `toml:"docs"`
}

// Kinds of next steps, the parts of the report (SPEC 8, step 13).
const (
	NextRun     = "run"     // a flk command for the admin to run
	NextRoot    = "root"    // a root step: shown, never run
	NextPending = "pending" // asked for, but not in effect until something else happens
	NextGap     = "gap"     // something flk can't do yet
)

// Next is a step the snippet implies, after render.
type Next struct {
	Kind  string `toml:"kind"`  // run (default), root, pending, gap
	Text  string `toml:"text"`  // a template
	Why   string `toml:"why"`   // a template; shown after the step
	Order int    `toml:"order"` // run steps sort by it (launch is 0, deploy is 100); default 50
	When  string `toml:"when"`
	// UntilEdited: a seed file's path (a template). The step goes away once
	// the file on disk differs from what the snippet first wrote (a stub
	// that has been filled in).
	UntilEdited string `toml:"until_edited"`
}

var (
	idRE    = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*/[a-z0-9][a-z0-9-]*$`)
	paramRE = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
)

// Parse reads a snippet.toml.
func Parse(data []byte) (*Snippet, error) {
	var s Snippet
	md, err := toml.Decode(string(data), &s)
	if err != nil {
		return nil, err
	}
	if und := md.Undecoded(); len(und) > 0 {
		var keys []string
		for _, k := range und {
			keys = append(keys, k.String())
		}
		return nil, fmt.Errorf("unknown keys: %s", strings.Join(keys, ", "))
	}
	for _, k := range md.Keys() {
		if len(k) == 2 && k[0] == "params" {
			s.ParamOrder = append(s.ParamOrder, k[1])
		}
	}
	return &s, s.validate()
}

func (s *Snippet) validate() error {
	if !idRE.MatchString(s.ID) {
		return fmt.Errorf("id %q isn't namespace/name (lowercase letters, digits, dashes)", s.ID)
	}
	if s.Version == "" {
		return fmt.Errorf("%s: no version", s.ID)
	}
	if !slices.Contains(Categories, s.Category) {
		return fmt.Errorf("%s: category %q isn't one of %s", s.ID, s.Category, strings.Join(Categories, ", "))
	}
	if s.Title == "" || s.Summary == "" {
		return fmt.Errorf("%s: title and summary are required", s.ID)
	}
	if (s.Category == "base") != (s.Base != nil) {
		return fmt.Errorf("%s: a base snippet, and only a base snippet, has a [base] table", s.ID)
	}
	if s.Base != nil {
		if s.Base.Image == "" {
			return fmt.Errorf("%s: [base] needs an image", s.ID)
		}
		if !slices.Contains([]string{"apt", "apk", "dnf"}, s.Base.Family) {
			return fmt.Errorf("%s: [base] family must be apt, apk or dnf", s.ID)
		}
	}
	for _, name := range s.ParamOrder {
		if !paramRE.MatchString(name) {
			return fmt.Errorf("%s: param name %q: lowercase letters, digits and _", s.ID, name)
		}
		if err := s.Params[name].check(); err != nil {
			return fmt.Errorf("%s: param %s: %v", s.ID, name, err)
		}
	}
	for _, d := range s.Dockerfile {
		if !slices.Contains(Slots, d.Slot) && d.Stage == "" {
			return fmt.Errorf("%s: Dockerfile slot %q isn't one of %s", s.ID, d.Slot, strings.Join(Slots, ", "))
		}
	}
	for _, f := range s.Files {
		if f.Path == "" || (f.Text == "") == (f.Template == "") {
			return fmt.Errorf("%s: a [[file]] needs a path, and text or a template (not both)", s.ID)
		}
		if f.Own != "" && f.Own != Managed && f.Own != Seed {
			return fmt.Errorf("%s: file %s: own must be managed or seed", s.ID, f.Path)
		}
	}
	for _, n := range s.Next {
		if n.Kind != "" && !slices.Contains([]string{NextRun, NextRoot, NextPending, NextGap}, n.Kind) {
			return fmt.Errorf("%s: next kind %q isn't run, root, pending or gap", s.ID, n.Kind)
		}
	}
	for k, rule := range s.Merge {
		if rule != "max" {
			return fmt.Errorf("%s: merge rule %q for %s: only max is known", s.ID, rule, k)
		}
	}
	return nil
}

// Load reads the snippet in dir of fsys.
func Load(fsys fs.FS, dir string) (*Snippet, error) {
	data, err := fs.ReadFile(fsys, path.Join(dir, "snippet.toml"))
	if err != nil {
		return nil, err
	}
	s, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s/snippet.toml: %w", dir, err)
	}
	if want := path.Base(path.Dir(dir)) + "/" + path.Base(dir); s.ID != want {
		return nil, fmt.Errorf("%s/snippet.toml: id %q doesn't match its directory (want %q)", dir, s.ID, want)
	}
	s.Dir, s.FS = dir, fsys
	if b, err := fs.ReadFile(fsys, path.Join(dir, "README.md")); err == nil {
		s.Readme = string(b)
	}
	for _, f := range s.Files {
		if f.Template != "" {
			if _, err := fs.Stat(fsys, path.Join(dir, "files", f.Template)); err != nil {
				return nil, fmt.Errorf("%s: file template %s: %v", s.ID, f.Template, err)
			}
		}
	}
	return s, nil
}

// ReadFile reads one of the snippet's files/ templates.
func (s *Snippet) ReadFile(name string) (string, error) {
	b, err := fs.ReadFile(s.FS, path.Join(s.Dir, "files", name))
	return string(b), err
}
