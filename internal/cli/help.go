package cli

import (
	"fmt"
	"io"
	"strings"
)

func usage(c *Command) string {
	s := "flkforge " + c.Name
	if c.Args != "" {
		s += " " + c.Args
	}
	for _, o := range c.Opts {
		if o.Arg != "" {
			s += " [--" + o.Name + " " + o.Arg + "]"
		} else {
			s += " [--" + o.Name + "]"
		}
	}
	return s
}

func writeHelp(w io.Writer) {
	fmt.Fprintln(w, "flkforge: forge a new flk app (Dockerfile, flk.toml, run wrapper) from snippets.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Commands:")
	for _, c := range commands {
		name := c.Name
		if c.Args != "" {
			name += " " + c.Args
		}
		fmt.Fprintf(w, "  %-18s %s\n", name, c.Summary)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Without a terminal: flkforge new DIR --recipe FILE --yes")
	fmt.Fprintln(w, "More: flkforge help COMMAND, flkforge help --all [--json]")
}

func writeCommandHelp(w io.Writer, c *Command) {
	fmt.Fprintln(w, "Usage: "+usage(c))
	fmt.Fprintln(w)
	fmt.Fprintln(w, c.Summary)
	if len(c.Opts) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Options:")
		for _, o := range c.Opts {
			sw := "--" + o.Name
			if o.Arg != "" {
				sw += " " + o.Arg
			}
			fmt.Fprintf(w, "  %-22s %s\n", sw, o.Help)
		}
	}
	if len(c.Notes) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Notes:")
		for _, n := range c.Notes {
			fmt.Fprintln(w, "  - "+n)
		}
	}
}

func cmdHelp(c *ctx) int {
	if c.has("json") && !c.has("all") {
		return c.usageError("--json goes with --all")
	}
	if c.has("all") {
		if len(c.pos) > 0 {
			return c.usageError("help --all takes no command")
		}
		if c.has("json") {
			c.printJSON(map[string]any{
				"schema":     HelpSchema,
				"version":    Version,
				"commands":   commands,
				"exit_codes": exitCodes,
			})
			return exitOK
		}
		fmt.Fprintln(c.out, "# flkforge reference")
		fmt.Fprintln(c.out)
		writeHelp(c.out)
		for _, cmd := range commands {
			fmt.Fprintf(c.out, "\n## flkforge %s\n\n", cmd.Name)
			writeCommandHelp(c.out, cmd)
		}
		fmt.Fprintln(c.out, "\n## Exit codes")
		fmt.Fprintln(c.out)
		for _, k := range []string{"0", "1", "2"} {
			fmt.Fprintf(c.out, "  %s  %s\n", k, exitCodes[k])
		}
		return exitOK
	}
	if len(c.pos) == 0 {
		writeHelp(c.out)
		return exitOK
	}
	cmd := lookup(c.pos[0])
	if cmd == nil {
		return c.usageError(fmt.Sprintf("unknown command %q", c.pos[0]))
	}
	writeCommandHelp(c.out, cmd)
	return exitOK
}

func (c *ctx) usageError(msg string) int {
	fmt.Fprintf(c.errw, "Error: %s\nUsage: %s\n", strings.TrimSpace(msg), usage(c.cmd))
	return exitUsage
}
