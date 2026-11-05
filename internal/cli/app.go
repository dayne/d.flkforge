package cli

import (
	"fmt"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/dayne/d.flkforge/internal/forge"
	"github.com/dayne/d.flkforge/internal/recipe"
	"github.com/dayne/d.flkforge/internal/render"
)

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
	for _, s := range rep.NotInEffect {
		if s.UntilEdited != "" {
			return "edit " + filepath.Join(out.Dir, s.UntilEdited) + " (" + s.Why + ")"
		}
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
