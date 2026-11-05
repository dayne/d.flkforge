// Package forge does what the CLI (and later the wizard) asks of an app
// directory: render a recipe and write its files safely (SPEC 6.5), check
// it with flk (SPEC 7), and build the report (SPEC 8, step 13). It's the
// I/O around internal/render, which stays pure.
package forge

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/dayne/d.flkforge/internal/flk"
	"github.com/dayne/d.flkforge/internal/recipe"
	"github.com/dayne/d.flkforge/internal/render"
	"github.com/dayne/d.flkforge/internal/snippet"
	"github.com/dayne/d.flkforge/internal/source"
	"github.com/dayne/d.flkforge/snippets"
)

// Env is what a command works with.
type Env struct {
	Version string      // flkforge's version, recorded in recipes
	Flk     *flk.Client // nil: flk isn't available
	FlkErr  error       // why Flk is nil
	// UserSnippets is the user's snippet directory ("" for none).
	UserSnippets string
}

// Catalog loads the snippets an app directory sees: built in, the
// user's, then the app's own (SPEC 3).
func (e *Env) Catalog(dir string) (*source.Catalog, error) {
	srcs := []source.Source{source.FS{Label: "built in", FS: snippets.FS}}
	if e.UserSnippets != "" {
		srcs = append(srcs, source.Dir(e.UserSnippets))
	}
	if dir != "" {
		srcs = append(srcs, source.Dir(source.AppDir(dir)))
	}
	return source.Load(srcs...)
}

// FileStatus says what a render did with a file.
type FileStatus struct {
	Path   string   `json:"path"`
	Own    string   `json:"own"`      // managed, seed, recipe
	From   []string `json:"snippets"` // the uses behind it
	Status string   `json:"status"`   // written, unchanged, kept, drift, removed, left
	Note   string   `json:"note,omitempty"`
}

// Outcome is the result of a render.
type Outcome struct {
	Dir      string           `json:"dir"`
	App      string           `json:"app"`
	Files    []FileStatus     `json:"files"`
	Findings []render.Finding `json:"findings"`
	Steps    []render.Step    `json:"-"`
	// Flk: what flk config show said about the new flk.toml.
	FlkChecked  bool     `json:"flk_checked"`
	FlkWarnings []string `json:"flk_warnings"`
	FlkNote     string   `json:"flk_note,omitempty"`
	// Refused: nothing was written (drift, flk warnings, rule errors).
	Refused string `json:"refused,omitempty"`
}

// Drift lists the files a render refused to overwrite.
func (o *Outcome) Drift() []FileStatus {
	var out []FileStatus
	for _, f := range o.Files {
		if f.Status == "drift" {
			out = append(out, f)
		}
	}
	return out
}

// RenderOpts change how a render writes.
type RenderOpts struct {
	Force         bool // overwrite hand-edited files
	AllowWarnings bool // write even if flk config show warns
	DryRun        bool // compute everything, write nothing
}

// Render renders the recipe in dir and writes the files (SPEC 6.5): a
// managed file that changed since the last render is not overwritten
// unless opts.Force; seed files are only written when missing.
func (e *Env) Render(dir string, r *recipe.Recipe, opts RenderOpts) (*Outcome, error) {
	cat, err := e.Catalog(dir)
	if err != nil {
		return nil, err
	}
	res, err := render.Render(r, cat)
	if err != nil {
		return nil, err
	}
	out := &Outcome{Dir: dir, App: r.App, Findings: res.Findings, Steps: res.Steps}
	e.hostSteps(dir, out)
	if errs := res.Errors(); len(errs) > 0 {
		out.Refused = "a rule failed: " + errs[0].Msg
		return out, nil
	}

	flkToml := res.File("flk.toml")
	e.validate(out, flkToml.Content)
	if len(out.FlkWarnings) > 0 && !opts.AllowWarnings {
		out.Refused = "flk config show warns about the new flk.toml (--allow-warnings writes it anyway)"
	}

	// Decide each file.
	type write struct {
		path    string
		content []byte
		mode    fs.FileMode
	}
	var writes []write
	hashes := map[string]string{}
	for _, f := range res.Files {
		st := FileStatus{Path: f.Path, Own: f.Own, From: f.From}
		full := filepath.Join(dir, f.Path)
		disk, err := os.ReadFile(full)
		exists := err == nil
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		switch {
		case f.Own == snippet.Seed && exists:
			st.Status, st.Note = "kept", "yours: flkforge only writes it when it's missing"
		case f.Own == snippet.Seed:
			st.Status, st.Note = "written", "yours from now on: flkforge won't overwrite it"
			writes = append(writes, write{full, f.Content, f.Mode})
		case exists && bytes.Equal(disk, f.Content):
			st.Status = "unchanged"
			hashes[f.Path] = recipe.Hash(f.Content)
		case exists && r.Render[f.Path] != recipe.Hash(disk) && !opts.Force:
			st.Status = "drift"
			if r.Render[f.Path] == "" {
				st.Note = "it's there already and flkforge didn't write it"
			} else {
				st.Note = "changed by hand since the last render"
			}
			st.Note += "; the new version is in " + f.Path + ".flkforge-new"
			writes = append(writes, write{full + ".flkforge-new", f.Content, f.Mode})
		default:
			st.Status = "written"
			writes = append(writes, write{full, f.Content, f.Mode})
			hashes[f.Path] = recipe.Hash(f.Content)
		}
		out.Files = append(out.Files, st)
	}
	// Files the last render wrote that this one doesn't (a snippet was removed).
	var removes []string
	for _, p := range sortedKeys(r.Render) {
		if res.File(p) != nil {
			continue
		}
		disk, err := os.ReadFile(filepath.Join(dir, p))
		switch {
		case errors.Is(err, fs.ErrNotExist):
		case err == nil && recipe.Hash(disk) == r.Render[p]:
			out.Files = append(out.Files, FileStatus{Path: p, Own: snippet.Managed, Status: "removed", Note: "no snippet writes it any more"})
			removes = append(removes, filepath.Join(dir, p))
		default:
			out.Files = append(out.Files, FileStatus{Path: p, Own: snippet.Managed, Status: "left", Note: "no snippet writes it any more, but it was changed by hand: delete it yourself if it's not needed"})
		}
	}
	if len(out.Drift()) > 0 && out.Refused == "" {
		out.Refused = "files were changed by hand (render --force overwrites them)"
	}

	if opts.DryRun {
		return out, nil
	}
	if out.Refused != "" {
		// Only the .flkforge-new files, so the admin can compare.
		for _, w := range writes {
			if strings.HasSuffix(w.path, ".flkforge-new") {
				if err := os.WriteFile(w.path, w.content, w.mode); err != nil {
					return nil, err
				}
			}
		}
		return out, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	for _, w := range writes {
		if err := writeFile(w.path, w.content, w.mode); err != nil {
			return nil, err
		}
		os.Remove(w.path + ".flkforge-new")
	}
	for _, p := range removes {
		if err := os.Remove(p); err != nil {
			return nil, err
		}
	}
	r.Flkforge = e.Version
	r.Lock = map[string]any{"flkforge": e.Version}
	r.Render = hashes
	if err := writeFile(filepath.Join(dir, recipe.File), []byte(r.Format()), 0o644); err != nil {
		return nil, err
	}
	out.Files = append(out.Files, FileStatus{Path: recipe.File, Own: "recipe", Status: "written", Note: "the recipe: what to render, and hashes of what was written"})
	return out, nil
}

// validate runs flk config show on the new flk.toml in a scratch
// directory, so nothing is written until flk is happy.
func (e *Env) validate(out *Outcome, flkToml []byte) {
	if e.Flk == nil {
		out.FlkNote = "not checked with flk: " + e.FlkErr.Error()
		return
	}
	tmp, err := os.MkdirTemp("", "flkforge-check-")
	if err != nil {
		out.FlkNote = "not checked with flk: " + err.Error()
		return
	}
	defer os.RemoveAll(tmp)
	if err := os.WriteFile(filepath.Join(tmp, "flk.toml"), flkToml, 0o644); err != nil {
		out.FlkNote = "not checked with flk: " + err.Error()
		return
	}
	cfg, err := e.Flk.ConfigShow(tmp, "")
	if err != nil {
		out.FlkChecked = true
		out.FlkWarnings = []string{"flk can't read it: " + err.Error()}
		return
	}
	out.FlkChecked = true
	out.FlkWarnings = cfg.Warnings
}

// writeFile writes atomically (a temp file, then rename) with mode.
func writeFile(path string, content []byte, mode fs.FileMode) error {
	tmp := path + ".flkforge-tmp"
	if err := os.WriteFile(tmp, content, mode); err != nil {
		return err
	}
	if err := os.Chmod(tmp, mode); err != nil { // umask
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

// NewOpts are `flkforge new`'s choices.
type NewOpts struct {
	App   string // overrides the recipe's app name
	Force bool   // the directory already has files
	RenderOpts
}

// New starts an app in dir from a recipe.
func (e *Env) New(dir string, r *recipe.Recipe, opts NewOpts) (*Outcome, error) {
	if _, err := os.Stat(filepath.Join(dir, recipe.File)); err == nil {
		return nil, fmt.Errorf("%s already has a recipe (%s): use flkforge render (or edit) there", dir, recipe.File)
	}
	if !opts.Force {
		for _, f := range []string{"Dockerfile", "flk.toml", "fly.toml"} {
			if _, err := os.Stat(filepath.Join(dir, f)); err == nil {
				return nil, fmt.Errorf("%s already has a %s: pick an empty directory, or --force to write next to it (existing files are never overwritten)", dir, f)
			}
		}
	}
	switch {
	case opts.App != "":
		r.App = opts.App
	case r.App == "":
		r.App = recipe.DefaultAppName(dir)
	}
	if err := recipe.CheckAppName(r.App); err != nil {
		return nil, err
	}
	r.Render = nil // a recipe from elsewhere: nothing here was written by it
	ro := opts.RenderOpts
	ro.Force = false // never overwrite files that are already there
	return e.Render(dir, r, ro)
}

// Report is the four-part report (SPEC 8, step 13).
type Report struct {
	Written     []FileStatus  `json:"written"`
	NotInEffect []render.Step `json:"not_in_effect"`
	Run         []render.Step `json:"run"`
	Root        []render.Step `json:"root"`
	Gaps        []render.Step `json:"gaps"`
}

// Report builds the report from an outcome.
func (o *Outcome) Report() Report {
	rep := Report{Written: o.Files, NotInEffect: []render.Step{}, Run: []render.Step{}, Root: []render.Step{}, Gaps: []render.Step{}}
	for _, s := range o.Steps {
		switch s.Kind {
		case snippet.NextPending:
			rep.NotInEffect = append(rep.NotInEffect, s)
		case snippet.NextRoot:
			rep.Root = append(rep.Root, s)
		case snippet.NextGap:
			rep.Gaps = append(rep.Gaps, s)
		default:
			rep.Run = append(rep.Run, s)
		}
	}
	return rep
}

// Registered says whether flk already has an app of this name, and where
// its flk.toml is.
func (e *Env) Registered(app string) (bool, string, error) {
	if e.Flk == nil {
		return false, "", e.FlkErr
	}
	apps, err := e.Flk.Apps()
	if err != nil {
		return false, "", err
	}
	i := slices.IndexFunc(apps, func(a flk.App) bool { return a.Name == app })
	if i < 0 {
		return false, "", nil
	}
	return true, apps[i].ConfigPath, nil
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// hostSteps drops next steps the host has already done (the app is
// registered from this directory, a volume exists), and warns when the
// app name is taken by another directory.
func (e *Env) hostSteps(dir string, out *Outcome) {
	reg, path, err := e.Registered(out.App)
	if err != nil || !reg {
		return
	}
	abs, _ := filepath.Abs(filepath.Join(dir, "flk.toml"))
	if path != abs {
		out.Findings = append(out.Findings, render.Finding{Rule: "app-name-taken", Level: "warn",
			Msg: out.App + " is already registered, from " + path + ": flk launch will refuse it, and flk -a " + out.App + " acts on that one",
			Fix: "pick another app name (new --app NAME, or app in the recipe)"})
		return
	}
	vols, _ := e.Flk.Volumes(out.App)
	var keep []render.Step
	for _, s := range out.Steps {
		if s.Text == "flk launch --no-deploy" {
			continue
		}
		if name, _, ok := volumeStep(s.Text); ok && slices.ContainsFunc(vols, func(v flk.Volume) bool { return v.Name == name }) {
			continue
		}
		keep = append(keep, s)
	}
	out.Steps = keep
}
