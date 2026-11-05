package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"strings"

	"github.com/dayne/d.flkforge/internal/flk"
	"github.com/dayne/d.flkforge/internal/forge"
	"github.com/dayne/d.flkforge/internal/source"
)

// Version is flkforge's release version, recorded in every recipe.
const Version = "0.1.0"

// HelpSchema is the version of `help --all --json`'s shape: new fields can
// appear without a bump; renames and removals bump it.
const HelpSchema = 1

// Options for Main; tests replace them.
type Options struct {
	Stdout, Stderr io.Writer
	// IsTTY says whether stdin and stdout are a terminal.
	IsTTY func() bool
	// Env builds the environment (flk, snippet dirs); nil: the real one.
	Env func() *forge.Env
}

type ctx struct {
	cmd    *Command
	opts   map[string]string // option name -> value ("" for a switch that's set)
	pos    []string
	out    io.Writer
	errw   io.Writer
	isTTY  func() bool
	envFn  func() *forge.Env
	envVal *forge.Env
}

func (c *ctx) has(name string) bool   { _, ok := c.opts[name]; return ok }
func (c *ctx) opt(name string) string { return c.opts[name] }

func (c *ctx) env() *forge.Env {
	if c.envVal == nil {
		c.envVal = c.envFn()
	}
	return c.envVal
}

// arg returns positional i, or def.
func (c *ctx) arg(i int, def string) string {
	if i < len(c.pos) {
		return c.pos[i]
	}
	return def
}

func (c *ctx) errorf(format string, a ...any) int {
	fmt.Fprintf(c.errw, "Error: "+format+"\n", a...)
	return exitFail
}

func (c *ctx) printJSON(v any) {
	b, _ := json.MarshalIndent(v, "", "  ")
	fmt.Fprintln(c.out, string(b))
}

// RealEnv finds flk and the user's snippet directory.
func RealEnv() *forge.Env {
	e := &forge.Env{Version: Version, UserSnippets: source.UserDir()}
	if x, err := flk.Find(); err != nil {
		e.FlkErr = err
	} else {
		e.Flk = flk.New(x)
	}
	return e
}

// Main runs flkforge with args (without the program name) and returns the
// exit code.
func Main(args []string, o Options) int {
	if o.Stdout == nil {
		o.Stdout = os.Stdout
	}
	if o.Stderr == nil {
		o.Stderr = os.Stderr
	}
	if o.IsTTY == nil {
		o.IsTTY = isTerminal
	}
	if o.Env == nil {
		o.Env = RealEnv
	}
	if len(args) == 0 {
		writeHelp(o.Stdout)
		return exitOK
	}
	switch args[0] {
	case "-h", "--help":
		writeHelp(o.Stdout)
		return exitOK
	case "--version":
		args[0] = "version"
	}
	cmd := lookup(args[0])
	if cmd == nil {
		fmt.Fprintf(o.Stderr, "Error: unknown command %q (flkforge help lists them)\n", args[0])
		return exitUsage
	}
	c := &ctx{cmd: cmd, opts: map[string]string{}, out: o.Stdout, errw: o.Stderr, isTTY: o.IsTTY, envFn: o.Env}
	if err := parse(c, args[1:]); err != nil {
		if err == errHelp {
			writeCommandHelp(o.Stdout, cmd)
			return exitOK
		}
		fmt.Fprintf(o.Stderr, "Error: %v\nUsage: %s\n", err, usage(cmd))
		return exitUsage
	}
	return cmd.run(c)
}

var errHelp = fmt.Errorf("help")

// parse fills c.opts and c.pos from args, strictly.
func parse(c *ctx, args []string) error {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "-h" || a == "--help" {
			return errHelp
		}
		if a == "--" {
			c.pos = append(c.pos, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(a, "-") || a == "-" {
			c.pos = append(c.pos, a)
			continue
		}
		name, val, hasVal := strings.Cut(strings.TrimLeft(a, "-"), "=")
		var o *Opt
		for j := range c.cmd.Opts {
			if c.cmd.Opts[j].Name == name {
				o = &c.cmd.Opts[j]
			}
		}
		if o == nil || !strings.HasPrefix(a, "--") {
			return fmt.Errorf("unknown option %s for %s", a, c.cmd.Name)
		}
		if _, dup := c.opts[name]; dup {
			return fmt.Errorf("--%s given twice", name)
		}
		switch {
		case o.Arg == "" && hasVal:
			return fmt.Errorf("--%s takes no value", name)
		case o.Arg != "" && !hasVal:
			if i+1 >= len(args) {
				return fmt.Errorf("--%s needs a value (%s)", name, o.Arg)
			}
			i++
			val = args[i]
		}
		c.opts[name] = val
	}
	min, max := arity(c.cmd.Args)
	if len(c.pos) < min {
		return fmt.Errorf("missing argument: %s", c.cmd.Args)
	}
	if len(c.pos) > max {
		return fmt.Errorf("unexpected argument %q", c.pos[max])
	}
	return nil
}

// arity counts a usage string's positionals: NAME is required, [NAME] optional.
func arity(args string) (min, max int) {
	for _, f := range strings.Fields(args) {
		max++
		if !strings.HasPrefix(f, "[") {
			min++
		}
	}
	return
}

func isTerminal() bool {
	for _, f := range []*os.File{os.Stdin, os.Stdout} {
		st, err := f.Stat()
		if err != nil || st.Mode()&os.ModeCharDevice == 0 {
			return false
		}
	}
	return true
}

func cmdVersion(c *ctx) int {
	info, _ := debug.ReadBuildInfo()
	fmt.Fprintln(c.out, versionString(info))
	return exitOK
}

// versionString: the release version, and the commit it was built from.
func versionString(info *debug.BuildInfo) string {
	s := "flkforge " + Version
	if info == nil {
		return s
	}
	var rev, modified string
	for _, kv := range info.Settings {
		switch kv.Key {
		case "vcs.revision":
			rev = kv.Value
		case "vcs.modified":
			if kv.Value == "true" {
				modified = ", modified"
			}
		}
	}
	if len(rev) > 7 {
		rev = rev[:7]
	}
	if rev != "" {
		s += " (commit " + rev + modified + ")"
	}
	return s
}
