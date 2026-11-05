// Package cli is flkforge's command line: one table of commands drives the
// parsing and every form of help (as flk's lib/flk/commands.rb does).
// Unknown options and extra arguments are errors.
package cli

// Opt is one option. Arg names its value; empty means a switch.
type Opt struct {
	Name string `json:"name"` // without the dashes
	Arg  string `json:"arg,omitempty"`
	Help string `json:"help"`
}

// Command is one entry of the table.
type Command struct {
	Name    string   `json:"name"`
	Args    string   `json:"args,omitempty"` // positionals: NAME required, [NAME] optional
	Summary string   `json:"summary"`
	Opts    []Opt    `json:"options"`
	Notes   []string `json:"notes,omitempty"`
	JSON    bool     `json:"json"`
	run     func(*ctx) int
}

var jsonOpt = Opt{Name: "json", Help: "Print JSON instead of text"}

// Exit codes (SPEC 9).
const (
	exitOK    = 0
	exitFail  = 1 // failed, or needs attention
	exitUsage = 2
)

var exitCodes = map[string]string{
	"0": "ok",
	"1": "failed, or needs attention (a FAIL in check, drift, a flk warning)",
	"2": "bad usage (unknown option, wrong arguments)",
}

var commands []*Command

func init() {
	commands = []*Command{
		{Name: "new", Args: "[DIR]", run: cmdNew, JSON: true,
			Summary: "Start a new app in DIR (default .) from a recipe; the wizard comes later",
			Opts: []Opt{
				{Name: "recipe", Arg: "FILE", Help: "Build the app from this recipe (a flkforge.toml)"},
				{Name: "app", Arg: "NAME", Help: "App name (default: the recipe's, else from DIR: foo.flk -> foo-flk)"},
				{Name: "yes", Help: "Write without asking (needed without a terminal)"},
				{Name: "force", Help: "DIR already has a Dockerfile or flk.toml: write next to them (they're never overwritten)"},
				{Name: "allow-warnings", Help: "Write even if flk config show warns about the new flk.toml"},
				jsonOpt,
			},
			Notes: []string{
				"The agent path: flkforge new DIR --recipe FILE --yes [--app NAME] builds a whole app with no terminal.",
				"Writes Dockerfile, flk.toml, any files the snippets add, and DIR/flkforge.toml (the recipe, with hashes of what it wrote).",
				"Never deploys and never runs root steps: it ends with the flk commands to run.",
			}},
		{Name: "render", Args: "[DIR]", run: cmdRender, JSON: true,
			Summary: "Rewrite the app's files from DIR/flkforge.toml",
			Opts: []Opt{
				{Name: "force", Help: "Overwrite files that were changed by hand"},
				{Name: "allow-warnings", Help: "Write even if flk config show warns about the new flk.toml"},
				{Name: "dry-run", Help: "Show what would change; write nothing"},
				jsonOpt,
			},
			Notes: []string{
				"A file changed by hand since the last render isn't overwritten: the new version goes to FILE.flkforge-new and render exits 1.",
				"Files a snippet seeds (a start script) are written only when missing; after that they're yours.",
				"The new flk.toml is checked with flk config show before anything is written.",
			}},
		{Name: "check", Args: "[DIR]", run: cmdCheck, JSON: true,
			Summary: "Read-only preflight: tools, recipe, files, flk config show, host",
			Opts:    []Opt{jsonOpt},
			Notes: []string{
				"Each line is ok, warn or FAIL, with a -> fix. Exit 1 on any FAIL.",
				"Never changes anything: safe for agents and cron.",
			}},
		{Name: "snippets", run: cmdSnippets, JSON: true,
			Summary: "List the snippets in the catalog",
			Opts: []Opt{
				{Name: "category", Arg: "C", Help: "Only this category"},
				{Name: "dir", Arg: "DIR", Help: "Include DIR's own snippets (DIR/.flkforge/snippets; default .)"},
				jsonOpt,
			},
			Notes: []string{"Built-in snippets, then ~/.config/flkforge/snippets, then the app's .flkforge/snippets; later ones override earlier ones by id."}},
		{Name: "help", Args: "[COMMAND]", run: cmdHelp, JSON: true,
			Summary: "Show help for flkforge or a command",
			Opts: []Opt{
				{Name: "all", Help: "The full reference: every command and option"},
				{Name: "json", Help: "With --all: the command table as JSON"},
			}},
		{Name: "version", run: cmdVersion, Summary: "Print flkforge's version", Opts: []Opt{}},
	}
}

func lookup(name string) *Command {
	for _, c := range commands {
		if c.Name == name {
			return c
		}
	}
	return nil
}
