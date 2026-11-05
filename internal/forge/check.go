package forge

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/dayne/d.flkforge/internal/flk"
	"github.com/dayne/d.flkforge/internal/recipe"
	"github.com/dayne/d.flkforge/internal/render"
	"github.com/dayne/d.flkforge/internal/snippet"
)

// Line is one preflight result (~/m.gamebus/PREFLIGHT.md): ok, warn or
// FAIL, with the fix for a problem.
type Line struct {
	Group  string `json:"group"`
	Status string `json:"status"` // ok, warn, FAIL
	Msg    string `json:"message"`
	Fix    string `json:"fix,omitempty"`
}

// CheckResult is `flkforge check`.
type CheckResult struct {
	Dir   string `json:"dir"`
	App   string `json:"app,omitempty"`
	Lines []Line `json:"checks"`
	OK    int    `json:"ok"`
	Warn  int    `json:"warn"`
	Fail  int    `json:"fail"`
	Next  string `json:"next"`
}

func (c *CheckResult) add(group, status, msg, fix string) {
	c.Lines = append(c.Lines, Line{group, status, msg, fix})
	switch status {
	case "ok":
		c.OK++
	case "warn":
		c.Warn++
	default:
		c.Fail++
	}
}

// Check is read-only: tools, recipe, files, flk, host, in that order,
// stopping at the first broken link of each chain (SPEC 7).
func (e *Env) Check(dir string) *CheckResult {
	c := &CheckResult{Dir: dir}
	defer func() { c.Next = c.next() }()

	// 1. tools
	var help *flk.Help
	if e.Flk == nil {
		c.add("tools", "FAIL", "flk: "+e.FlkErr.Error(), "install flk (github.com/dayne/d.flk) and put it on PATH")
	} else if h, err := e.Flk.HelpAll(); err != nil {
		c.add("tools", "FAIL", "flk doesn't answer: "+err.Error(), "run flk doctor")
	} else {
		help = h
		c.add("tools", "ok", fmt.Sprintf("flk %s (help schema %d)", h.Version, h.Schema), "")
		if h.Schema > flk.Schema {
			c.add("tools", "warn", fmt.Sprintf("flk's help schema is %d; flkforge knows %d", h.Schema, flk.Schema), "update flkforge; meanwhile it ignores what it doesn't know")
		}
	}
	if p, err := exec.LookPath("podman"); err == nil {
		c.add("tools", "ok", "podman: "+p, "")
	} else if p, err := exec.LookPath("docker"); err == nil {
		c.add("tools", "ok", "docker: "+p, "")
	} else {
		c.add("tools", "warn", "neither podman nor docker is on PATH: flk deploy can't build the image", "install podman (flk help install)")
	}

	// 2. recipe
	r, err := recipe.Read(dir)
	if err != nil {
		fix := "flkforge new " + dir + " --recipe FILE --yes starts one"
		if !errors.Is(err, fs.ErrNotExist) {
			fix = "fix " + filepath.Join(dir, recipe.File)
		}
		c.add("recipe", "FAIL", err.Error(), fix)
		return c
	}
	c.App = r.App
	cat, err := e.Catalog(dir)
	if err != nil {
		c.add("recipe", "FAIL", "snippets: "+err.Error(), "fix the snippet it names")
		return c
	}
	res, err := render.Render(r, cat)
	if err != nil {
		c.add("recipe", "FAIL", err.Error(), "fix "+filepath.Join(dir, recipe.File)+", then flkforge render")
		return c
	}
	c.add("recipe", "ok", fmt.Sprintf("%s: %d snippets resolve, params are valid", r.App, len(r.Uses)), "")
	for _, f := range res.Findings {
		st := map[string]string{"error": "FAIL", "warn": "warn", "hint": "ok"}[f.Level]
		c.add("recipe", st, f.Rule+": "+f.Msg, f.Fix)
	}
	if v, _ := r.Lock["flkforge"].(string); v != e.Version {
		c.add("recipe", "warn", fmt.Sprintf("rendered by flkforge %q; this is %s: snippets may have changed", v, e.Version), "flkforge render (it shows what changes)")
	}

	// 3. files
	filesOK := true
	for _, f := range res.Files {
		disk, err := os.ReadFile(filepath.Join(dir, f.Path))
		switch {
		case err != nil && f.Own == snippet.Seed:
			c.add("files", "warn", f.Path+" is missing", "flkforge render writes a new one")
		case err != nil:
			filesOK = false
			c.add("files", "FAIL", f.Path+" is missing", "flkforge render")
		case f.Own == snippet.Seed:
			c.add("files", "ok", f.Path+" (yours)", "")
		case r.Render[f.Path] != "" && recipe.Hash(disk) != r.Render[f.Path]:
			filesOK = false
			c.add("files", "FAIL", f.Path+" was changed by hand since the last render", "put the change in the recipe and flkforge render --force, or keep the file and stop using flkforge for it")
		case !bytes.Equal(disk, f.Content):
			filesOK = false
			c.add("files", "FAIL", f.Path+" doesn't match the recipe (the recipe changed, or another flkforge version)", "flkforge render")
		default:
			c.add("files", "ok", f.Path+" matches the recipe", "")
		}
	}

	// 4. flk
	if e.Flk != nil && filesOK {
		cfg, err := e.Flk.ConfigShow(dir, "")
		switch {
		case err != nil:
			c.add("flk", "FAIL", "flk config show: "+err.Error(), "fix flk.toml (in the recipe), then flkforge render")
		case len(cfg.Warnings) > 0:
			for _, w := range cfg.Warnings {
				c.add("flk", "FAIL", "flk config show: "+w, "a snippet wrote a key flk doesn't know: report it (flkforge's bug)")
			}
		default:
			c.add("flk", "ok", "flk config show: no warnings", "")
		}
	}

	// 5. host hints (warn only)
	if help == nil {
		return c
	}
	reg, path, err := e.Registered(r.App)
	abs, _ := filepath.Abs(filepath.Join(dir, "flk.toml"))
	switch {
	case err != nil:
		c.add("host", "warn", "flk apps list: "+err.Error(), "")
	case !reg:
		c.add("host", "warn", r.App+" isn't registered with flk yet", "cd "+dir+" && flk launch --no-deploy")
	case path != abs:
		c.add("host", "FAIL", r.App+" is already registered, from "+path, "pick another app name (set app in the recipe, then flkforge render)")
	default:
		c.add("host", "ok", r.App+" is registered", "")
		vols, err := e.Flk.Volumes(r.App)
		if err != nil {
			c.add("host", "warn", "flk volumes list: "+err.Error(), "")
			break
		}
		for _, s := range res.Steps {
			if name, size, ok := volumeStep(s.Text); ok {
				if slices.ContainsFunc(vols, func(v flk.Volume) bool { return v.Name == name }) {
					c.add("host", "ok", "volume "+name+" exists", "")
				} else {
					c.add("host", "warn", "volume "+name+" doesn't exist yet: the deploy will fail", "cd "+dir+" && flk volumes create "+name+" --size "+size)
				}
			}
		}
	}
	return c
}

// volumeStep picks the volume name and size out of a volumes create step.
func volumeStep(text string) (name, size string, ok bool) {
	var n, s string
	if _, err := fmt.Sscanf(text, "flk volumes create %s --size %s", &n, &s); err != nil {
		return "", "", false
	}
	return n, s, true
}

func (c *CheckResult) next() string {
	for _, l := range c.Lines {
		if l.Status == "FAIL" && l.Fix != "" {
			return l.Fix
		}
	}
	for _, l := range c.Lines {
		if l.Status == "warn" && l.Fix != "" && l.Group == "host" {
			return l.Fix
		}
	}
	if c.Fail == 0 {
		for _, l := range c.Lines {
			if l.Group == "host" && l.Status == "ok" && strings.HasSuffix(l.Msg, "is registered") {
				return "nothing to fix: flk deploy applies changes; flk status shows the machines"
			}
		}
		return "cd " + c.Dir + " && flk deploy"
	}
	return ""
}
