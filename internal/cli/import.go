package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"

	"github.com/dayne/d.flkforge/internal/forge"
	"github.com/dayne/d.flkforge/internal/importer"
	"github.com/dayne/d.flkforge/internal/recipe"
)

// cmdImport builds a recipe from a hand-written app (best effort).
func cmdImport(c *ctx) int {
	src := c.pos[0]
	if c.has("to") && c.has("in-place") {
		return c.usageError("--to or --in-place, not both")
	}
	ft, err := os.ReadFile(filepath.Join(src, "flk.toml"))
	if err != nil {
		return c.errorf("%v (import reads an app directory with a flk.toml)", err)
	}
	df, _ := os.ReadFile(filepath.Join(src, "Dockerfile"))
	cat, err := c.env().Catalog(src)
	if err != nil {
		return c.errorf("%v", err)
	}
	res, err := importer.Import(importer.Input{FlkToml: string(ft), Dockerfile: string(df), ReadFile: func(n string) ([]byte, error) {
		p := filepath.Join(src, filepath.Clean("/"+n))
		return os.ReadFile(p)
	}}, cat)
	if err != nil {
		return c.errorf("%v", err)
	}
	r := res.Recipe
	if c.has("app") {
		r.App = c.opt("app")
	}
	abs, _ := filepath.Abs(src)
	r.Title = "imported from " + abs

	var result struct {
		Recipe   string         `json:"recipe"`
		Notes    []string       `json:"notes"`
		Kept     int            `json:"kept"`
		Written  string         `json:"written,omitempty"`
		Same     *bool          `json:"flk_reads_the_same,omitempty"`
		Differs  []string       `json:"differs,omitempty"`
		Next     string         `json:"next,omitempty"`
		Missing  []string       `json:"to_copy,omitempty"`
		Rendered *forge.Outcome `json:"render,omitempty"`
	}
	result.Notes = res.Notes
	result.Kept = res.KeptToml + res.KeptDocker

	switch {
	case c.has("in-place"):
		path := filepath.Join(src, recipe.File)
		if _, err := os.Stat(path); err == nil {
			return c.errorf("%s already has a recipe", src)
		}
		if err := os.WriteFile(path, []byte(r.Format()), 0o644); err != nil {
			return c.errorf("%v", err)
		}
		result.Written = path
		result.Next = "flkforge render --dry-run " + src + " shows how the files would change; flkforge render --force " + src + " adopts them"
	case c.has("to"):
		dir := c.opt("to")
		out, err := c.env().New(dir, r, forge.NewOpts{App: r.App, RenderOpts: forge.RenderOpts{AllowWarnings: true}})
		if err != nil {
			return c.renderError(err, "new")
		}
		result.Rendered = out
		result.Written = dir
		if e := c.env(); e.Flk != nil && out.Refused == "" {
			got, err1 := e.Flk.ConfigShow(dir, "")
			want, err2 := e.Flk.ConfigShow(src, "")
			if err1 == nil && err2 == nil {
				for _, k := range unionKeys(got.Parsed, want.Parsed) {
					if k == "app" && c.has("app") {
						continue // renamed on purpose
					}
					if !reflect.DeepEqual(got.Parsed[k], want.Parsed[k]) {
						result.Differs = append(result.Differs, k)
					}
				}
				same := len(result.Differs) == 0
				result.Same = &same
			}
		}
		// Files the Dockerfile copies that flkforge didn't write: the build
		// needs them in DIR too.
		if b, err := os.ReadFile(filepath.Join(dir, "Dockerfile")); err == nil {
			for _, f := range copySources(string(b)) {
				if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
					if _, err := os.Stat(filepath.Join(src, f)); err == nil || strings.Contains(f, "$") {
						result.Missing = append(result.Missing, f)
					}
				}
			}
		}
		result.Next = "diff -r " + src + " " + dir + " (the Dockerfile is rebuilt from snippets; flk.toml should mean the same)"
	}
	result.Recipe = r.Format()

	if c.has("json") {
		c.printJSON(result)
		return exitOK
	}
	if !c.has("to") && !c.has("in-place") {
		fmt.Fprint(c.out, result.Recipe)
	}
	w := c.errw
	if c.has("to") || c.has("in-place") {
		w = c.out
	}
	fmt.Fprintf(w, "\nImported %s (best effort):\n", src)
	for _, n := range res.Notes {
		fmt.Fprintln(w, "  - "+n)
	}
	if result.Written != "" {
		fmt.Fprintln(w, "\nWrote "+result.Written)
	}
	if result.Same != nil {
		if *result.Same {
			fmt.Fprintln(w, "flk reads the new flk.toml exactly as the original (flk config show).")
		} else {
			fmt.Fprintln(w, "flk reads these differently: "+strings.Join(result.Differs, ", "))
		}
	}
	if len(result.Missing) > 0 {
		fmt.Fprintf(w, "To build it, copy these from %s (the Dockerfile copies them; flkforge doesn't): %s\n", src, strings.Join(result.Missing, ", "))
	}
	if result.Next != "" {
		fmt.Fprintln(w, "Next: "+result.Next)
	}
	return exitOK
}

func unionKeys(a, b map[string]any) []string {
	s := map[string]bool{}
	for k := range a {
		s[k] = true
	}
	for k := range b {
		s[k] = true
	}
	var out []string
	for k := range s {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// copySources lists the build-context paths a Dockerfile COPYs.
func copySources(dockerfile string) []string {
	var out []string
	for _, l := range strings.Split(dockerfile, "\n") {
		f := strings.Fields(l)
		if len(f) < 3 || !strings.EqualFold(f[0], "COPY") {
			continue
		}
		var args []string
		from := false
		for _, w := range f[1:] {
			if strings.HasPrefix(w, "--from") {
				from = true
			}
			if !strings.HasPrefix(w, "--") {
				args = append(args, w)
			}
		}
		if from || len(args) < 2 {
			continue
		}
		for _, a := range args[:len(args)-1] {
			a = strings.TrimSuffix(a, "/")
			if !slices.Contains(out, a) {
				out = append(out, a)
			}
		}
	}
	return out
}
