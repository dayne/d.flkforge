package cli

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/dayne/d.flkforge/internal/flktoml"
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
	if !c.has("recipe") && !c.has("template") {
		if !c.isTTY() {
			return c.usageError("no terminal, and the wizard needs one. Without a terminal: " + agentForm)
		}
		return c.usageError("the wizard isn't built yet (PLAN M4). For now: " + agentForm)
	}
	if !c.has("yes") {
		if !c.isTTY() {
			return c.usageError("no terminal to confirm in: add --yes (" + agentForm + ")")
		}
		return c.usageError("the wizard, which would show the recipe before writing, isn't built yet: add --yes")
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
	v, err := parseValue(p, raw)
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

// parseValue reads a command-line value for a param: strings as they are
// (or TOML-quoted), numbers and booleans by type, lists as TOML arrays or
// comma-separated, maps as TOML inline tables.
func parseValue(p *snippet.Param, raw string) (any, error) {
	switch p.Type {
	case "int", "port":
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%q isn't a whole number", raw)
		}
		return n, nil
	case "bool":
		b, err := strconv.ParseBool(raw)
		if err != nil {
			return nil, fmt.Errorf("%q isn't true or false", raw)
		}
		return b, nil
	case "duration":
		if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
			return n, nil
		}
		return raw, nil
	case "list":
		if strings.HasPrefix(strings.TrimSpace(raw), "[") {
			v, err := flktoml.Decode(raw)
			if err != nil {
				return nil, err
			}
			return v, nil
		}
		if raw == "" {
			return []any{}, nil
		}
		var out []any
		for _, s := range strings.Split(raw, ",") {
			out = append(out, strings.TrimSpace(s))
		}
		return out, nil
	case "map":
		v, err := flktoml.Decode(raw)
		if err != nil {
			return nil, fmt.Errorf("want a TOML inline table, such as '{ NAME = \"value\" }': %v", err)
		}
		return v, nil
	}
	// strings: a TOML-quoted one is unquoted, anything else is taken as is
	if len(raw) >= 2 && (raw[0] == '"' || raw[0] == '\'') {
		if v, err := flktoml.Decode(raw); err == nil {
			if s, ok := v.(string); ok {
				return s, nil
			}
		}
	}
	return raw, nil
}
