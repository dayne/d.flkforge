package forge

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
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
		fix := "flkforge templates, then flkforge new " + dir + " --template T --yes starts one"
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
	for _, s := range editedSteps(dir, res, res.Steps) {
		if s.UntilEdited != "" {
			c.add("files", "warn", s.Text, "edit "+filepath.Join(dir, s.UntilEdited)+": "+s.Why)
		}
	}
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
	var cfg *flk.Config
	if e.Flk != nil && filesOK {
		var err error
		cfg, err = e.Flk.ConfigShow(dir, "")
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

	// 5. host: ports, then what's registered and deployed
	if help == nil {
		return c
	}
	if cfg != nil {
		e.checkPorts(c, r.App, cfg)
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
		e.checkDeployed(c, dir, r.App, cfg)
		e.checkSecrets(c, dir, r.App, res.Steps)
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
	for _, g := range []string{"files", "host"} {
		for _, l := range c.Lines {
			if l.Status == "warn" && l.Fix != "" && l.Group == g {
				return l.Fix
			}
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

// checkPorts: each host port the app forwards is allowed (flk's min_port)
// and not already forwarded by another app.
func (e *Env) checkPorts(c *CheckResult, app string, cfg *flk.Config) {
	ports, err := e.Flk.Ports()
	if err != nil {
		c.add("host", "warn", "flk ports: "+err.Error(), "")
		return
	}
	type hp struct {
		proto string
		port  int
	}
	var want []hp
	if services, ok := cfg.Parsed["services"].([]any); ok {
		for _, s := range services {
			sm, _ := s.(map[string]any)
			proto, _ := sm["protocol"].(string)
			if proto == "" {
				proto = "tcp"
			}
			pl, _ := sm["ports"].([]any)
			for _, p := range pl {
				pm, _ := p.(map[string]any)
				if n, ok := pm["port"].(float64); ok {
					want = append(want, hp{proto, int(n)})
				}
			}
		}
	}
	if f, ok := cfg.Parsed["flk"].(map[string]any); ok {
		if n, ok := f["http_port"].(float64); ok {
			want = append(want, hp{"tcp", int(n)})
		}
	}
	for _, w := range want {
		name := fmt.Sprintf("%s/%d", w.proto, w.port)
		if ports.MinPort > 0 && w.port < ports.MinPort {
			c.add("host", "FAIL", fmt.Sprintf("host port %s is below flk's minimum (%d): flk won't forward it", name, ports.MinPort),
				fmt.Sprintf("as root: sudo FLK_MIN_PORT=%d ~/flk/setup-host.sh, or pick a port of %d and up", w.port, ports.MinPort))
			continue
		}
		taken := ""
		for _, a := range ports.Active {
			if a.Proto == w.proto && a.Port == w.port && a.App != app {
				taken = a.App
			}
		}
		if taken != "" {
			c.add("host", "FAIL", "host port "+name+" is already forwarded to "+taken, "pick another host port (core/service host_port)")
		} else {
			c.add("host", "ok", "host port "+name+" is free", "")
		}
	}
}

// checkDeployed: the app's release and machines, and whether flk.toml
// changed since the release was deployed.
func (e *Env) checkDeployed(c *CheckResult, dir, app string, cfg *flk.Config) {
	st, err := e.Flk.Status(app)
	if err != nil {
		c.add("host", "warn", "flk status: "+err.Error(), "")
		return
	}
	rel := st.App.Release
	if rel == nil {
		c.add("host", "warn", app+" isn't deployed yet", "cd "+dir+" && flk deploy")
		return
	}
	running := 0
	for _, m := range st.Machines {
		if m.State == "started" {
			running++
		}
	}
	c.add("host", "ok", fmt.Sprintf("release v%d deployed; %d of %d machines started", rel.Version, running, len(st.Machines)), "")
	if cfg != nil && rel.Config != nil && !reflect.DeepEqual(normalize(rel.Config), normalize(cfg.MachineConfig)) {
		c.add("host", "warn", fmt.Sprintf("flk.toml has changed since v%d was deployed (the Dockerfile may have too)", rel.Version), "cd "+dir+" && flk deploy")
	}
}

// normalize drops keys whose value is null, so an older release's config
// (without a newer flk's fields) compares equal when nothing changed.
func normalize(m map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range m {
		if v != nil {
			out[k] = v
		}
	}
	return out
}

// checkSecrets: every secret the recipe names is set.
func (e *Env) checkSecrets(c *CheckResult, dir, app string, steps []render.Step) {
	var names []string
	for _, s := range steps {
		if rest, ok := strings.CutPrefix(s.Text, "flk secrets set "); ok {
			names = append(names, strings.SplitN(rest, "=", 2)[0])
		}
	}
	if len(names) == 0 {
		return
	}
	set, err := e.Flk.Secrets(app)
	if err != nil {
		c.add("host", "warn", "flk secrets list: "+err.Error(), "")
		return
	}
	for _, n := range names {
		if slices.Contains(set, n) {
			c.add("host", "ok", "secret "+n+" is set", "")
		} else {
			c.add("host", "warn", "secret "+n+" isn't set: the app won't have it", "cd "+dir+" && flk secrets set "+n+"=- --stage")
		}
	}
}
