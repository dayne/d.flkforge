package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/dayne/d.flkforge/internal/forge"
	"github.com/dayne/d.flkforge/internal/recipe"
	"github.com/dayne/d.flkforge/internal/render"
	"github.com/dayne/d.flkforge/internal/snippet"
	"github.com/dayne/d.flkforge/internal/source"
)

const agentForm = "flkforge new DIR --recipe FILE --yes (or --template T)"

func cmdNew(c *ctx) int {
	dir := c.arg(0, ".")
	if c.has("recipe") && c.has("template") {
		return c.usageError("--recipe or --template, not both")
	}
	if !c.has("yes") {
		if !c.isTTY() {
			if !c.has("recipe") && !c.has("template") {
				return c.usageError("no terminal, and the wizard needs one. Without a terminal: " + agentForm)
			}
			return c.usageError("no terminal to confirm in: add --yes (" + agentForm + ")")
		}
		if len(c.multi["set"]) > 0 {
			return c.usageError("--set goes with --yes; in the wizard, set params in their step")
		}
		if err := c.checkNewDir(dir); err != nil {
			return c.errorf("%v", err)
		}
		var r *recipe.Recipe
		if c.has("recipe") {
			var err error
			if r, err = recipe.Read(c.opt("recipe")); err != nil {
				return c.errorf("%v", err)
			}
			r.Render, r.Seeded = nil, nil
			if c.has("app") {
				r.App = c.opt("app")
			}
		} else if c.has("app") {
			r = &recipe.Recipe{App: c.opt("app")}
		}
		return c.runWizard(dir, r, c.opt("template"), false)
	}
	if !c.has("recipe") && !c.has("template") {
		return c.usageError("--yes needs --template T or --recipe FILE (" + agentForm + ")")
	}
	cat, err := c.env().Catalog(dir)
	if err != nil {
		return c.errorf("%v", err)
	}
	var r *recipe.Recipe
	if c.has("template") {
		t, err := cat.Template(c.opt("template"))
		if err != nil {
			return c.errorf("%v", err)
		}
		r = cloneRecipe(t.Recipe)
		r.Template, r.Title, r.Summary = t.ID, "", ""
	} else if r, err = recipe.Read(c.opt("recipe")); err != nil {
		return c.errorf("%v", err)
	}
	for _, kv := range c.multi["set"] {
		key, val, ok := strings.Cut(kv, "=")
		use, param, ok2 := strings.Cut(key, ".")
		if !ok || !ok2 {
			return c.usageError(fmt.Sprintf("--set %s: write USE.PARAM=VALUE (the use's id: its snippet's name unless it has one)", kv))
		}
		if err := setParam(r, cat, use, param, val); err != nil {
			return c.errorf("--set %s: %v", kv, err)
		}
	}
	var untrusted []string
	for _, u := range r.Uses {
		if s, err := cat.Get(u.Snippet); err == nil && strings.HasPrefix(s.Origin, "hub ") && !forge.Locked(r, s.ID) {
			untrusted = append(untrusted, s.ID+" ("+s.Origin+")")
		}
	}
	if len(untrusted) > 0 && !c.has("trust") {
		return c.errorf("the recipe uses hub snippets: %s. They run as build steps and in your VMs: read them (flkforge hub show ID), then add --trust", strings.Join(untrusted, ", "))
	}
	out, err := c.env().New(dir, r, forge.NewOpts{App: c.opt("app"), Force: c.has("force"), RenderOpts: forge.RenderOpts{AllowWarnings: c.has("allow-warnings")}})
	if err != nil {
		return c.renderError(err, "new")
	}
	return c.report(out, false)
}

// renderError prints a render error with the command that fixes it.
func (c *ctx) renderError(err error, cmd string) int {
	var pes render.ParamErrors
	if errors.As(err, &pes) {
		fmt.Fprintln(c.errw, "Error: params that don't fit:")
		for _, pe := range pes {
			var probs snippet.ParamErrors
			if !errors.As(pe.Err, &probs) {
				fmt.Fprintf(c.errw, "  %s: %v\n", pe.Use, pe.Err)
				continue
			}
			for _, p := range probs {
				fix := "flkforge set " + pe.Use + " " + p.Param + "=VALUE"
				if cmd == "new" {
					fix = "--set " + pe.Use + "." + p.Param + "=VALUE"
				}
				fmt.Fprintf(c.errw, "  %s.%s: %s\n    -> %s\n", pe.Use, p.Param, p.Msg, fix)
			}
		}
		fmt.Fprintln(c.errw, "Nothing was written. (flkforge snippets show SNIPPET: a snippet's params)")
		return exitFail
	}
	fmt.Fprintf(c.errw, "Error: %v\nNothing was written.\n", err)
	return exitFail
}

func cloneRecipe(r *recipe.Recipe) *recipe.Recipe {
	n := *r
	n.Uses = nil
	for _, u := range r.Uses {
		p := map[string]any{}
		for k, v := range u.Params {
			p[k] = v
		}
		u.Params = p
		n.Uses = append(n.Uses, u)
	}
	return &n
}

// splitDir: a trailing positional without "=" is the app directory.
func splitDir(args []string) ([]string, string) {
	if n := len(args); n > 0 && !strings.Contains(args[n-1], "=") {
		return args[:n-1], args[n-1]
	}
	return args, "."
}

// editRecipe loads DIR's recipe, applies change, and renders. Nothing is
// written unless the new recipe renders (and its files can be written).
func (c *ctx) editRecipe(dir string, change func(*recipe.Recipe, *source.Catalog) error) int {
	r, err := recipe.Read(dir)
	if err != nil {
		return c.errorf("%v (flkforge new starts an app)", err)
	}
	cat, err := c.env().Catalog(dir)
	if err != nil {
		return c.errorf("%v", err)
	}
	if err := change(r, cat); err != nil {
		return c.errorf("%v", err)
	}
	out, err := c.env().Render(dir, r, forge.RenderOpts{
		Force:         c.has("force"),
		AllowWarnings: c.has("allow-warnings"),
		DryRun:        c.has("dry-run"),
		Relock:        c.has("relock"),
	})
	if err != nil {
		return c.renderError(err, "set")
	}
	if out.Refused != "" && !c.has("json") {
		fmt.Fprintln(c.errw, "The recipe wasn't changed either.")
	}
	return c.report(out, c.has("dry-run"))
}

func cmdAdd(c *ctx) int {
	id := c.pos[0]
	dir := c.arg(1, ".")
	return c.editRecipe(dir, func(r *recipe.Recipe, cat *source.Catalog) error {
		s, err := cat.Get(id)
		if err != nil {
			return err
		}
		if strings.HasPrefix(s.Origin, "hub ") && !forge.Locked(r, s.ID) && !c.has("trust") {
			showSnippetFiles(c, s)
			return fmt.Errorf("%s comes from %s, not flkforge: it runs as build steps and in your VMs. Read it above; add --trust to use it", s.ID, s.Origin)
		}
		u := recipe.Use{Snippet: s.ID, ID: c.opt("id"), Params: map[string]any{}}
		for _, existing := range r.Uses {
			if existing.Key() == u.Key() {
				if s.Multi {
					return fmt.Errorf("the recipe already has a use %q: give this one its own --id", u.Key())
				}
				return fmt.Errorf("the recipe already uses %s (as %q): change it with flkforge set %s PARAM=VALUE", s.ID, existing.Key(), existing.Key())
			}
		}
		r.Uses = append(r.Uses, u)
		for _, kv := range c.multi["set"] {
			k, v, ok := strings.Cut(kv, "=")
			if !ok {
				return fmt.Errorf("--set %s: write PARAM=VALUE", kv)
			}
			if err := setParam(r, cat, u.Key(), k, v); err != nil {
				return err
			}
		}
		return nil
	})
}

func cmdRm(c *ctx) int {
	key := c.pos[0]
	dir := c.arg(1, ".")
	return c.editRecipe(dir, func(r *recipe.Recipe, _ *source.Catalog) error {
		i := slices.IndexFunc(r.Uses, func(u recipe.Use) bool { return u.Key() == key })
		if i < 0 {
			return fmt.Errorf("the recipe has no use %q (%s)", key, useKeys(r))
		}
		r.Uses = slices.Delete(r.Uses, i, i+1)
		return nil
	})
}

func cmdSet(c *ctx) int {
	key := c.pos[0]
	kvs, dir := splitDir(c.pos[1:])
	if len(kvs) == 0 && len(c.multi["unset"]) == 0 {
		return c.usageError("nothing to set: give PARAM=VALUE or --unset PARAM")
	}
	return c.editRecipe(dir, func(r *recipe.Recipe, cat *source.Catalog) error {
		for _, kv := range kvs {
			k, v, _ := strings.Cut(kv, "=")
			if err := setParam(r, cat, key, k, v); err != nil {
				return err
			}
		}
		for _, k := range c.multi["unset"] {
			i := slices.IndexFunc(r.Uses, func(u recipe.Use) bool { return u.Key() == key })
			if i < 0 {
				return fmt.Errorf("the recipe has no use %q (%s)", key, useKeys(r))
			}
			delete(r.Uses[i].Params, k)
		}
		return nil
	})
}

func useKeys(r *recipe.Recipe) string {
	var keys []string
	for _, u := range r.Uses {
		keys = append(keys, u.Key())
	}
	return "it has: " + strings.Join(keys, ", ")
}

// setParam sets one param of a use, parsing the value by the param's type.
func setParam(r *recipe.Recipe, cat *source.Catalog, key, param, raw string) error {
	i := slices.IndexFunc(r.Uses, func(u recipe.Use) bool { return u.Key() == key })
	if i < 0 {
		return fmt.Errorf("the recipe has no use %q (%s)", key, useKeys(r))
	}
	s, err := cat.Get(r.Uses[i].Snippet)
	if err != nil {
		return err
	}
	p, ok := s.Params[param]
	if !ok {
		return fmt.Errorf("%s has no param %q (it has: %s)", s.ID, param, strings.Join(s.ParamOrder, ", "))
	}
	v, err := snippet.ParseValue(p, raw)
	if err != nil {
		return fmt.Errorf("%s.%s: %v", key, param, err)
	}
	if _, err := p.Coerce(v); err != nil {
		return fmt.Errorf("%s.%s: %v", key, param, err)
	}
	if r.Uses[i].Params == nil {
		r.Uses[i].Params = map[string]any{}
	}
	r.Uses[i].Params[param] = v
	return nil
}

// checkNewDir refuses a directory that already has an app, before the
// wizard starts (forge.New checks again when it writes).
func (c *ctx) checkNewDir(dir string) error {
	if _, err := os.Stat(filepath.Join(dir, recipe.File)); err == nil {
		return fmt.Errorf("%s already has a recipe: flkforge edit %s", dir, dir)
	}
	if !c.has("force") {
		for _, f := range []string{"Dockerfile", "flk.toml", "fly.toml"} {
			if _, err := os.Stat(filepath.Join(dir, f)); err == nil {
				return fmt.Errorf("%s already has a %s: pick an empty directory, or --force to write next to it", dir, f)
			}
		}
	}
	return nil
}
