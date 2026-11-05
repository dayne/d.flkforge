// Package render turns a recipe and its snippets into files, rule
// findings and next steps (SPEC 6). It's a pure function: no I/O, no flk,
// no TUI. The same recipe and snippets give byte-identical files.
package render

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"slices"
	"sort"
	"strconv"
	"strings"
	"text/template"

	"github.com/BurntSushi/toml"

	"github.com/dayne/d.flkforge/internal/flktoml"
	"github.com/dayne/d.flkforge/internal/recipe"
	"github.com/dayne/d.flkforge/internal/snippet"
)

// Resolver finds snippets by id (a source.Catalog).
type Resolver interface {
	Get(id string) (*snippet.Snippet, error)
}

// File is one file to write.
type File struct {
	Path    string      `json:"path"`
	Content []byte      `json:"-"`
	Mode    fs.FileMode `json:"-"`
	Own     string      `json:"own"`      // managed or seed
	From    []string    `json:"snippets"` // the uses behind it
}

// Step is a line of the report (SPEC 8, step 13).
type Step struct {
	Kind  string `json:"kind"` // run, root, pending, gap
	Text  string `json:"text"`
	Why   string `json:"why,omitempty"`
	From  string `json:"from,omitempty"` // the use that asked for it
	Order int    `json:"-"`
}

// Finding is a rule's result (SPEC 6.4).
type Finding struct {
	Rule  string `json:"rule"`
	Level string `json:"level"` // error, warn, hint
	Msg   string `json:"message"`
	Fix   string `json:"fix,omitempty"`
}

// Result is what a render produces.
type Result struct {
	Files    []File    `json:"files"`
	Steps    []Step    `json:"steps"`
	Findings []Finding `json:"findings"`
}

// File returns the file at path, or nil.
func (r *Result) File(path string) *File {
	for i := range r.Files {
		if r.Files[i].Path == path {
			return &r.Files[i]
		}
	}
	return nil
}

// Errors are the findings at level error.
func (r *Result) Errors() []Finding {
	var out []Finding
	for _, f := range r.Findings {
		if f.Level == "error" {
			out = append(out, f)
		}
	}
	return out
}

// App is what templates see as .App.
type App struct {
	Name   string
	Image  string // the base image, full name
	Family string // apt, apk, dnf
	// BaseUser is the base image's uid-1000 user, if it has one.
	BaseUser string
	// User runs the app; empty: root.
	User string
	Uses []string // every use's key
}

// Data is what a template sees.
type Data struct {
	P   map[string]any // this use's params
	ID  string         // this use's key
	App App
}

type use struct {
	recipe.Use
	key string
	s   *snippet.Snippet
	p   map[string]any
}

// Render renders a recipe.
func Render(r *recipe.Recipe, cat Resolver) (*Result, error) {
	if r.App == "" {
		return nil, fmt.Errorf("the recipe has no app name")
	}
	if err := recipe.CheckAppName(r.App); err != nil {
		return nil, err
	}
	uses, err := resolve(r, cat)
	if err != nil {
		return nil, err
	}
	var base *use
	for _, u := range uses {
		if u.s.Category == "base" {
			if base != nil {
				return nil, fmt.Errorf("two base snippets, %s and %s: an image has one base", base.key, u.key)
			}
			base = u
		}
	}
	if base == nil {
		return nil, fmt.Errorf("no base snippet: add one (flkforge snippets --category base)")
	}
	app := App{Name: r.App, Family: base.s.Base.Family, BaseUser: base.s.Base.User}
	for _, u := range uses {
		app.Uses = append(app.Uses, u.key)
	}
	img, err := execute(base.s.ID+" [base] image", base.s.Base.Image, Data{P: base.p, ID: base.key, App: app})
	if err != nil {
		return nil, err
	}
	app.Image = strings.TrimSpace(img)

	res := &Result{}
	dockerfile, from, err := renderDockerfile(uses, base, app)
	if err != nil {
		return nil, err
	}
	res.Files = append(res.Files, File{Path: "Dockerfile", Content: []byte(dockerfile), Mode: 0o644, Own: snippet.Managed, From: from})

	flk, from, err := renderFlkToml(uses, app)
	if err != nil {
		return nil, err
	}
	res.Files = append(res.Files, File{Path: "flk.toml", Content: []byte(flk), Mode: 0o644, Own: snippet.Managed, From: from})

	for _, u := range uses {
		d := Data{P: u.p, ID: u.key, App: app}
		for _, f := range u.s.Files {
			ok, err := when(u, f.When, d)
			if err != nil || !ok {
				if err != nil {
					return nil, err
				}
				continue
			}
			p, err := execute(u.s.ID+" file path", f.Path, d)
			if err != nil {
				return nil, err
			}
			if p = strings.TrimSpace(p); p == "" || strings.HasPrefix(p, "/") || strings.Contains(p, "..") {
				return nil, fmt.Errorf("%s: file path %q must be inside the app directory", u.key, p)
			}
			if res.File(p) != nil {
				return nil, fmt.Errorf("%s: two snippets write %s", u.key, p)
			}
			text := f.Text
			if f.Template != "" {
				if text, err = u.s.ReadFile(f.Template); err != nil {
					return nil, err
				}
			}
			if !f.Verbatim {
				if text, err = execute(u.s.ID+" "+p, text, d); err != nil {
					return nil, err
				}
			}
			mode := fs.FileMode(0o644)
			if f.Mode != "" {
				m, err := strconv.ParseUint(f.Mode, 8, 32)
				if err != nil {
					return nil, fmt.Errorf("%s: file %s: mode %q isn't octal", u.key, p, f.Mode)
				}
				mode = fs.FileMode(m)
			}
			own := f.Own
			if own == "" {
				own = snippet.Managed
			}
			res.Files = append(res.Files, File{Path: p, Content: []byte(text), Mode: mode, Own: own, From: []string{u.key}})
		}
	}

	if res.Steps, err = steps(uses, app); err != nil {
		return nil, err
	}
	res.Findings = rules(uses, base, app, res)
	return res, nil
}

func resolve(r *recipe.Recipe, cat Resolver) ([]*use, error) {
	var uses []*use
	count := map[string]int{}
	for _, ru := range r.Uses {
		s, err := cat.Get(ru.Snippet)
		if err != nil {
			return nil, err
		}
		p, err := s.Resolve(ru.Params)
		if err != nil {
			return nil, err
		}
		count[s.ID]++
		uses = append(uses, &use{Use: ru, key: ru.Key(), s: s, p: p})
	}
	for _, u := range uses {
		if count[u.s.ID] > 1 && !u.s.Multi {
			return nil, fmt.Errorf("%s is used %d times, but it can only be used once", u.s.ID, count[u.s.ID])
		}
	}
	has := func(ref string, self *use) bool {
		for _, o := range uses {
			if o == self {
				continue
			}
			if cat, ok := strings.CutPrefix(ref, "category:"); ok {
				if o.s.Category == cat {
					return true
				}
			} else if o.s.ID == ref || o.s.Name() == ref {
				return true
			}
		}
		return false
	}
	for _, u := range uses {
		for _, req := range u.s.Requires {
			if !has(req, u) {
				return nil, fmt.Errorf("%s needs %s in the recipe", u.key, strings.Replace(req, "category:", "a snippet of category ", 1))
			}
		}
		for _, c := range u.s.Conflicts {
			if has(c, u) {
				return nil, fmt.Errorf("%s can't be used with %s", u.key, strings.Replace(c, "category:", "a snippet of category ", 1))
			}
		}
	}
	return uses, nil
}

// header is the first comment line of every managed file (TODO.md: one
// line pointing at the recipe; the drift hashes do the real work).
func header(app string) string {
	return "# " + app + ": made by flkforge from flkforge.toml. Change the recipe and run\n# `flkforge render`, or edit this file and stop using flkforge for it."
}

func renderDockerfile(uses []*use, base *use, app App) (string, []string, error) {
	slots := map[string][]string{}
	var stageOrder []string
	stages := map[string][]string{}
	var pkgs []string
	var from []string
	addFrom := func(k string) {
		if !slices.Contains(from, k) {
			from = append(from, k)
		}
	}
	addFrom(base.key)
	for _, u := range uses {
		d := Data{P: u.p, ID: u.key, App: app}
		if u.s.Packages != "" {
			out, err := execute(u.s.ID+" packages", u.s.Packages, d)
			if err != nil {
				return "", nil, err
			}
			for _, p := range strings.Fields(out) {
				if !slices.Contains(pkgs, p) {
					pkgs = append(pkgs, p)
				}
				addFrom(u.key)
			}
		}
		for _, f := range u.s.Dockerfile {
			ok, err := when(u, f.When, d)
			if err != nil {
				return "", nil, err
			}
			if !ok {
				continue
			}
			text, err := execute(u.s.ID+" Dockerfile "+f.Slot, f.Text, d)
			if err != nil {
				return "", nil, err
			}
			text = strings.Trim(text, "\n")
			if text == "" {
				continue
			}
			addFrom(u.key)
			if f.Stage != "" {
				if _, ok := stages[f.Stage]; !ok {
					stageOrder = append(stageOrder, f.Stage)
				}
				stages[f.Stage] = append(stages[f.Stage], text)
				continue
			}
			slots[f.Slot] = append(slots[f.Slot], text)
		}
	}
	slots["from"] = append([]string{"FROM " + app.Image}, slots["from"]...)
	if len(pkgs) > 0 {
		slots["packages"] = append([]string{install(app.Family, pkgs)}, slots["packages"]...)
	}
	if base.s.Base.ClearEntrypoint {
		slots["entry"] = append([]string{"# The base image has its own ENTRYPOINT; clear it so CMD is the app.\nENTRYPOINT []"}, slots["entry"]...)
	}

	var b strings.Builder
	b.WriteString(header(app.Name) + "\n")
	if h := slots["header"]; len(h) > 0 {
		b.WriteString("#\n" + strings.Join(h, "\n") + "\n")
	}
	for _, st := range stageOrder {
		b.WriteString("\n" + strings.Join(stages[st], "\n") + "\n")
	}
	for _, slot := range snippet.Slots[1:] {
		if len(slots[slot]) == 0 {
			continue
		}
		b.WriteString("\n" + strings.Join(slots[slot], "\n") + "\n")
	}
	return b.String(), from, nil
}

// install is the one package install line for the base's family.
func install(family string, pkgs []string) string {
	list := strings.Join(pkgs, " ")
	switch family {
	case "apk":
		return "RUN apk add --no-cache " + list
	case "dnf":
		return "RUN dnf install -y " + list + " \\\n && dnf clean all"
	default:
		return "RUN apt-get update \\\n && apt-get install -y --no-install-recommends " + list + " \\\n && rm -rf /var/lib/apt/lists/*"
	}
}

func renderFlkToml(uses []*use, app App) (string, []string, error) {
	root := &flktoml.Table{}
	root.Items = append(root.Items, &flktoml.KV{Key: "app", Value: flktoml.Quote(app.Name), Comment: strings.Split(header(app.Name), "\n")})
	build, _ := root.Sub("build", true)
	build.Set("dockerfile", `"Dockerfile"`)
	rules := map[string]string{}
	for _, u := range uses {
		for k, v := range u.s.Merge {
			rules[k] = v
		}
	}
	var from []string
	for _, u := range uses {
		d := Data{P: u.p, ID: u.key, App: app}
		for i, f := range u.s.FlkToml {
			ok, err := when(u, f.When, d)
			if err != nil {
				return "", nil, err
			}
			if !ok {
				continue
			}
			text, err := execute(fmt.Sprintf("%s flk.toml fragment %d", u.s.ID, i+1), f.Text, d)
			if err != nil {
				return "", nil, err
			}
			frag, err := flktoml.Parse(text)
			if err != nil {
				return "", nil, fmt.Errorf("%s: flk.toml fragment %d: %v\n%s", u.key, i+1, err, text)
			}
			setOrigin(frag, u.key)
			if err := flktoml.Merge(root, frag, rules); err != nil {
				return "", nil, fmt.Errorf("%s: %v", u.key, err)
			}
			if !slices.Contains(from, u.key) {
				from = append(from, u.key)
			}
		}
	}
	out := flktoml.Format(root, flktoml.HouseOrder)
	var check map[string]any
	if _, err := toml.Decode(out, &check); err != nil {
		return "", nil, fmt.Errorf("the merged flk.toml isn't valid TOML (a flkforge bug): %v\n%s", err, out)
	}
	return out, from, nil
}

func setOrigin(t *flktoml.Table, origin string) {
	for _, it := range t.Items {
		switch x := it.(type) {
		case *flktoml.KV:
			x.Origin = origin
		case *flktoml.Sub:
			setOrigin(x.T, origin)
		case *flktoml.Arr:
			for _, e := range x.Ts {
				setOrigin(e, origin)
			}
		}
	}
}

func steps(uses []*use, app App) ([]Step, error) {
	out := []Step{{Kind: snippet.NextRun, Text: "flk launch --no-deploy", Why: "registers " + app.Name + " with flk, without building or starting anything", Order: 0}}
	for _, u := range uses {
		d := Data{P: u.p, ID: u.key, App: app}
		for _, n := range u.s.Next {
			ok, err := when(u, n.When, d)
			if err != nil {
				return nil, err
			}
			if !ok {
				continue
			}
			text, err := execute(u.s.ID+" next", n.Text, d)
			if err != nil {
				return nil, err
			}
			why, err := execute(u.s.ID+" next why", n.Why, d)
			if err != nil {
				return nil, err
			}
			kind, order := n.Kind, n.Order
			if kind == "" {
				kind = snippet.NextRun
			}
			if order == 0 {
				order = 50
			}
			st := Step{Kind: kind, Text: strings.TrimSpace(text), Why: strings.TrimSpace(why), From: u.key, Order: order}
			if !slices.ContainsFunc(out, func(o Step) bool { return o.Kind == st.Kind && o.Text == st.Text }) {
				out = append(out, st)
			}
		}
	}
	out = append(out, Step{Kind: snippet.NextRun, Text: "flk deploy", Why: "builds the image and starts the app; it waits for the health checks", Order: 100})
	sort.SliceStable(out, func(i, j int) bool { return out[i].Order < out[j].Order })
	return out, nil
}

// execute runs a template with flkforge's helpers.
func execute(name, text string, d Data) (string, error) {
	if !strings.Contains(text, "{{") {
		return text, nil
	}
	t, err := template.New(name).Option("missingkey=error").Funcs(funcs).Parse(text)
	if err != nil {
		return "", fmt.Errorf("%s: %v", name, err)
	}
	var b bytes.Buffer
	if err := t.Execute(&b, d); err != nil {
		return "", fmt.Errorf("%s: %v", name, err)
	}
	return b.String(), nil
}

// when evaluates a fragment's condition: empty is true; otherwise the
// template's output must be true or false.
func when(u *use, cond string, d Data) (bool, error) {
	if strings.TrimSpace(cond) == "" {
		return true, nil
	}
	out, err := execute(u.s.ID+" when", cond, d)
	if err != nil {
		return false, err
	}
	switch strings.TrimSpace(out) {
	case "true":
		return true, nil
	case "false", "":
		return false, nil
	}
	return false, fmt.Errorf("%s: when %q gave %q, not true or false", u.s.ID, cond, out)
}

var funcs = template.FuncMap{
	// toml writes a value as TOML ({{toml .P.name}} -> "x")
	"toml": func(v any) string { return flktoml.Encode(v, false) },
	// json writes a value as JSON (exec-form CMD lines)
	"json": func(v any) (string, error) {
		b, err := json.Marshal(v)
		return string(b), err
	},
	// sh quotes a string for a POSIX shell
	"sh": func(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" },
	"join": func(l any, sep string) string {
		switch x := l.(type) {
		case []string:
			return strings.Join(x, sep)
		case nil:
			return ""
		}
		return fmt.Sprint(l)
	},
	"lower": strings.ToLower,
	"upper": strings.ToUpper,
	"has":   func(l []string, s string) bool { return slices.Contains(l, s) },
	"default": func(def, v any) any {
		if v == nil || v == "" {
			return def
		}
		return v
	},
	"trim": strings.TrimSpace,
	// key writes a TOML key, quoted if it needs to be
	"key": flktoml.FormatKey,
	"indent": func(n int, s string) string {
		pad := strings.Repeat(" ", n)
		return pad + strings.ReplaceAll(s, "\n", "\n"+pad)
	},
}
