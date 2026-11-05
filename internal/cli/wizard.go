package cli

import (
	"bytes"
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/dayne/d.flkforge/internal/forge"
	"github.com/dayne/d.flkforge/internal/recipe"
	"github.com/dayne/d.flkforge/internal/wizard"
)

// runWizard runs the terminal UI on a new app (r nil) or an existing
// recipe (edit), and prints the report when it wrote something.
func (c *ctx) runWizard(dir string, r *recipe.Recipe, template string, edit bool) int {
	switch c.opt("hints") {
	case "", "full", "summary", "minimal", "off":
	default:
		return c.usageError("--hints is full, summary or minimal")
	}
	env := c.env()
	cat, err := env.Catalog(dir)
	if err != nil {
		return c.errorf("%v", err)
	}
	var lastReport string
	o := wizard.Options{
		Catalog: cat, Dir: dir, Recipe: r, Template: template, Edit: edit, Hints: c.opt("hints"),
		Write: func(nr *recipe.Recipe) (string, bool, error) {
			var out *forge.Outcome
			var err error
			if edit {
				out, err = env.Render(dir, nr, forge.RenderOpts{})
			} else {
				out, err = env.New(dir, nr, forge.NewOpts{App: nr.App})
			}
			if err != nil {
				return "", true, err
			}
			var b bytes.Buffer
			sub := *c
			sub.out = &b
			sub.opts = map[string]string{}
			sub.report(out, false)
			lastReport = b.String()
			return lastReport, out.Refused != "", nil
		},
	}
	if env.Flk != nil {
		o.Validate = env.ValidateFlkToml
		o.Taken = func(app string) string {
			reg, path, err := env.Registered(app)
			if err != nil || !reg {
				return ""
			}
			return path
		}
	}
	m := wizard.New(o)
	if _, err := tea.NewProgram(m).Run(); err != nil {
		return c.errorf("%v", err)
	}
	if lastReport != "" {
		fmt.Fprint(c.out, lastReport)
	}
	if !m.Written() {
		if lastReport == "" {
			fmt.Fprintln(c.errw, "Nothing written.")
		}
		return exitFail
	}
	return exitOK
}

func cmdEdit(c *ctx) int {
	dir := c.arg(0, ".")
	if !c.isTTY() {
		return c.usageError("no terminal, and the wizard needs one. Without a terminal: flkforge add, set and rm change a recipe")
	}
	r, err := recipe.Read(dir)
	if err != nil {
		return c.errorf("%v (flkforge new starts an app)", err)
	}
	return c.runWizard(dir, r, "", true)
}
