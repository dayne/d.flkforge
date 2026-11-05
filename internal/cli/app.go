package cli

import (
	"fmt"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/dayne/d.flkforge/internal/forge"
	"github.com/dayne/d.flkforge/internal/recipe"
	"github.com/dayne/d.flkforge/internal/render"
	"github.com/dayne/d.flkforge/internal/snippet"
)

const agentForm = "flkforge new DIR --recipe FILE --yes"

func cmdNew(c *ctx) int {
	dir := c.arg(0, ".")
	if !c.has("recipe") {
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
	r, err := recipe.Read(c.opt("recipe"))
	if err != nil {
		return c.errorf("%v", err)
	}
	out, err := c.env().New(dir, r, forge.NewOpts{App: c.opt("app"), Force: c.has("force"), RenderOpts: forge.RenderOpts{AllowWarnings: c.has("allow-warnings")}})
	if err != nil {
		return c.errorf("%v", err)
	}
	return c.report(out, false)
}

func cmdRender(c *ctx) int {
	dir := c.arg(0, ".")
	r, err := recipe.Read(dir)
	if err != nil {
		return c.errorf("%v (flkforge new starts an app)", err)
	}
	out, err := c.env().Render(dir, r, forge.RenderOpts{
		Force:         c.has("force"),
		AllowWarnings: c.has("allow-warnings"),
		DryRun:        c.has("dry-run"),
	})
	if err != nil {
		return c.errorf("%v", err)
	}
	return c.report(out, c.has("dry-run"))
}

// next is the one command to run next.
func next(out *forge.Outcome, rep forge.Report) string {
	if out.Refused != "" {
		if d := out.Drift(); len(d) > 0 {
			return fmt.Sprintf("cd %s && diff -u %s %s.flkforge-new", out.Dir, d[0].Path, d[0].Path)
		}
		return ""
	}
	if len(rep.Run) > 0 {
		return "cd " + out.Dir + " && " + rep.Run[0].Text
	}
	return ""
}

// report prints a render's outcome: the four-part report (SPEC 8,
// step 13), or why nothing was written.
func (c *ctx) report(out *forge.Outcome, dry bool) int {
	rep := out.Report()
	code := exitOK
	if out.Refused != "" {
		code = exitFail
	}
	if c.has("json") {
		c.printJSON(struct {
			*forge.Outcome
			DryRun bool         `json:"dry_run"`
			Report forge.Report `json:"report"`
			Next   string       `json:"next"`
		}{out, dry, rep, next(out, rep)})
		return code
	}
	w := c.out
	fmt.Fprintf(w, "flkforge: %s in %s\n", out.App, out.Dir)

	if out.Refused != "" {
		fmt.Fprintf(w, "\nNothing written: %s.\n", out.Refused)
	} else if dry {
		fmt.Fprintln(w, "\nDry run: nothing written. What a render would do:")
	}
	if out.Refused != "" || dry {
		fmt.Fprintln(w, "\nFiles:")
	} else {
		fmt.Fprintln(w, "\nWritten:")
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, f := range out.Files {
		status := f.Status
		if dry && status == "written" {
			status = "would write"
		}
		if out.Refused != "" && status == "written" {
			status = "not written"
		}
		detail := strings.Join(f.From, ", ")
		if f.Note != "" {
			if detail != "" {
				detail += ": "
			}
			detail += f.Note
		}
		fmt.Fprintf(tw, "  %s\t%s\t%s\n", f.Path, status, detail)
	}
	tw.Flush()

	switch {
	case !out.FlkChecked:
		fmt.Fprintf(w, "\nflk config show: %s\n", out.FlkNote)
	case len(out.FlkWarnings) == 0:
		fmt.Fprintln(w, "\nflk config show: no warnings.")
	default:
		fmt.Fprintln(w, "\nflk config show warns:")
		for _, x := range out.FlkWarnings {
			fmt.Fprintln(w, "  - "+x)
		}
	}
	if len(out.Findings) > 0 {
		fmt.Fprintln(w, "\nRules:")
		for _, f := range out.Findings {
			fmt.Fprintf(w, "  %-5s %s: %s\n", f.Level, f.Rule, f.Msg)
			if f.Fix != "" {
				fmt.Fprintf(w, "        -> %s\n", f.Fix)
			}
		}
	}
	if out.Refused == "" {
		if len(rep.NotInEffect) > 0 {
			fmt.Fprintln(w, "\nNot in effect yet:")
			for _, s := range rep.NotInEffect {
				fmt.Fprintln(w, "  - "+withWhy(s, ": "))
			}
		}
		fmt.Fprintf(w, "\nFor you to run, in %s:\n", out.Dir)
		steps(c, rep.Run)
		if len(rep.Root) > 0 {
			fmt.Fprintln(w, "\nAs root (flkforge never runs these; read them first):")
			steps(c, rep.Root)
		}
		if len(rep.Gaps) > 0 {
			fmt.Fprintln(w, "\nGaps in flk (asked for, but flk can't do it yet):")
			for _, s := range rep.Gaps {
				fmt.Fprintln(w, "  - "+withWhy(s, ": "))
			}
		}
	}
	if n := next(out, rep); n != "" {
		fmt.Fprintln(w, "\nNext: "+n)
	}
	return code
}

func withWhy(s render.Step, sep string) string {
	if s.Why == "" {
		return s.Text
	}
	return s.Text + sep + s.Why
}

func steps(c *ctx, list []render.Step) {
	tw := tabwriter.NewWriter(c.out, 0, 0, 2, ' ', 0)
	for _, s := range list {
		if s.Why != "" {
			fmt.Fprintf(tw, "  %s\t# %s\n", s.Text, s.Why)
		} else {
			fmt.Fprintf(tw, "  %s\n", s.Text)
		}
	}
	tw.Flush()
}

func cmdCheck(c *ctx) int {
	res := c.env().Check(c.arg(0, "."))
	code := exitOK
	if res.Fail > 0 {
		code = exitFail
	}
	if c.has("json") {
		c.printJSON(res)
		return code
	}
	group := ""
	for _, l := range res.Lines {
		if l.Group != group {
			group = l.Group
			fmt.Fprintln(c.out, group)
		}
		fmt.Fprintf(c.out, "  %-4s  %s\n", l.Status, l.Msg)
		if l.Fix != "" && l.Status != "ok" {
			fmt.Fprintf(c.out, "        -> %s\n", l.Fix)
		}
	}
	fmt.Fprintf(c.out, "\n%d ok, %d warn, %d FAIL.", res.OK, res.Warn, res.Fail)
	if res.Next != "" {
		fmt.Fprintf(c.out, " Next: %s", res.Next)
	}
	fmt.Fprintln(c.out)
	return code
}

type paramJSON struct {
	Name     string   `json:"name"`
	Type     string   `json:"type"`
	Default  any      `json:"default,omitempty"`
	Required bool     `json:"required"`
	Values   []string `json:"values,omitempty"`
	Prompt   string   `json:"prompt"`
	Hint     string   `json:"hint,omitempty"`
}

type snippetJSON struct {
	ID       string      `json:"id"`
	Version  string      `json:"version"`
	Title    string      `json:"title"`
	Category string      `json:"category"`
	Summary  string      `json:"summary"`
	Multi    bool        `json:"multi"`
	Requires []string    `json:"requires,omitempty"`
	Origin   string      `json:"origin"`
	Params   []paramJSON `json:"params"`
}

func cmdSnippets(c *ctx) int {
	dir := c.opt("dir")
	if dir == "" {
		dir = "."
	}
	cat, err := c.env().Catalog(dir)
	if err != nil {
		return c.errorf("%v", err)
	}
	want := c.opt("category")
	if want != "" && !slices.Contains(snippet.Categories, want) {
		return c.usageError(fmt.Sprintf("category %q isn't one of %s", want, strings.Join(snippet.Categories, ", ")))
	}
	var list []*snippet.Snippet
	for _, s := range cat.All() {
		if want == "" || s.Category == want {
			list = append(list, s)
		}
	}
	slices.SortStableFunc(list, func(a, b *snippet.Snippet) int {
		return slices.Index(snippet.Categories, a.Category) - slices.Index(snippet.Categories, b.Category)
	})
	if c.has("json") {
		out := []snippetJSON{}
		for _, s := range list {
			j := snippetJSON{ID: s.ID, Version: s.Version, Title: s.Title, Category: s.Category, Summary: s.Summary, Multi: s.Multi, Requires: s.Requires, Origin: s.Origin, Params: []paramJSON{}}
			for _, n := range s.ParamOrder {
				p := s.Params[n]
				j.Params = append(j.Params, paramJSON{n, p.Type, p.Default, p.Required, p.Values, p.Prompt, p.Hint})
			}
			out = append(out, j)
		}
		c.printJSON(out)
		return exitOK
	}
	tw := tabwriter.NewWriter(c.out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tCATEGORY\tSUMMARY")
	for _, s := range list {
		fmt.Fprintf(tw, "%s\t%s\t%s\n", s.ID, s.Category, s.Summary)
	}
	tw.Flush()
	return exitOK
}
