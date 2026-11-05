// Package cli is flkforge's command line: one table of commands drives the
// parsing and every form of help (as flk's lib/flk/commands.rb does).
// Unknown options and extra arguments are errors.
package cli

// Opt is one option. Arg names its value; empty means a switch.
type Opt struct {
	Name string `json:"name"` // without the dashes
	Arg  string `json:"arg,omitempty"`
	Help string `json:"help"`
	// Multi: may be given more than once (--set K=V --set K2=V2).
	Multi bool `json:"multi,omitempty"`
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

// editOpts are the options of the commands that change a recipe and render.
var editOpts = []Opt{
	{Name: "force", Help: "Overwrite files that were changed by hand"},
	{Name: "relock", Help: "Accept hub snippets that changed since the recipe locked them"},
	{Name: "allow-warnings", Help: "Write even if flk config show warns about the new flk.toml"},
	{Name: "dry-run", Help: "Show what would change; write nothing"},
	jsonOpt,
}

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
			Summary: "Start a new app in DIR (default .): the wizard in a terminal, or --template/--recipe with --yes",
			Opts: []Opt{
				{Name: "template", Arg: "T", Help: "Start from this template (flkforge templates lists them)"},
				{Name: "recipe", Arg: "FILE", Help: "Start from this recipe (a flkforge.toml)"},
				{Name: "set", Arg: "USE.PARAM=VALUE", Multi: true, Help: "Set a param of one of the recipe's uses (a template leaves some open)"},
				{Name: "app", Arg: "NAME", Help: "App name (default: the recipe's, else from DIR: foo.flk -> foo-flk)"},
				{Name: "yes", Help: "Write without asking (needed without a terminal)"},
				{Name: "force", Help: "DIR already has a Dockerfile or flk.toml: write next to them (they're never overwritten)"},
				{Name: "trust", Help: "The recipe or template uses hub snippets: use them (read them first: flkforge hub show ID)"},
				{Name: "allow-warnings", Help: "Write even if flk config show warns about the new flk.toml"},
				{Name: "hints", Arg: "LEVEL", Help: "The wizard's starting hint level: full (default), summary or minimal"},
				jsonOpt,
			},
			Notes: []string{
				"In a terminal without --yes it's the wizard: pick a template (or start from --template / --recipe), go through the steps with a live preview, and write.",
				"The agent path: flkforge new DIR --template T --yes [--set USE.PARAM=VALUE ...] [--app NAME], or --recipe FILE instead of --template.",
				"A use's id is its snippet's name (service, volume) unless the recipe gives it one. A template's open params are errors that name the --set to add.",
				"Writes Dockerfile, flk.toml, any files the snippets add, and DIR/flkforge.toml (the recipe, with hashes of what it wrote).",
				"Never deploys and never runs root steps: it ends with the flk commands to run.",
			}},
		{Name: "edit", Args: "[DIR]", run: cmdEdit,
			Summary: "Open DIR's recipe in the wizard, remix it, and write",
			Opts:    []Opt{{Name: "hints", Arg: "LEVEL", Help: "Starting hint level: full (default), summary or minimal"}},
			Notes: []string{
				"Needs a terminal. Without one: flkforge add, set and rm.",
				"Writing re-renders like flkforge render: a file changed by hand isn't overwritten (FILE.flkforge-new instead).",
			}},
		{Name: "render", Args: "[DIR]", run: cmdRender, JSON: true,
			Summary: "Rewrite the app's files from DIR/flkforge.toml",
			Opts:    editOpts,
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
		{Name: "add", Args: "SNIPPET [DIR]", run: cmdAdd, JSON: true,
			Summary: "Add a snippet to DIR's recipe, then render",
			Opts: append([]Opt{
				{Name: "id", Arg: "ID", Help: "The use's id (needed when the snippet is used more than once)"},
				{Name: "trust", Help: "Use a hub snippet the recipe hasn't locked yet (read it first: flkforge hub show ID)"},
				{Name: "set", Arg: "PARAM=VALUE", Multi: true, Help: "Set one of its params"},
			}, editOpts...),
			Notes: []string{
				"Values are read by the param's type: numbers, true/false, lists as a,b or [\"a\", \"b\"], tables as { K = \"v\" }.",
				"Nothing changes (recipe or files) unless the result renders and its files can be written.",
			}},
		{Name: "rm", Args: "USE [DIR]", run: cmdRm, JSON: true,
			Summary: "Remove a use from DIR's recipe, then render",
			Opts:    editOpts,
			Notes:   []string{"A file only that use wrote is deleted if it's unchanged since the last render, and left with a note if you edited it."}},
		{Name: "set", Args: "USE PARAM=VALUE... [DIR]", run: cmdSet, JSON: true,
			Summary: "Set params of a use in DIR's recipe, then render",
			Opts: append([]Opt{
				{Name: "unset", Arg: "PARAM", Multi: true, Help: "Remove a param (back to its default)"},
			}, editOpts...),
			Notes: []string{"USE is the use's id: its snippet's name unless the recipe gives it one (flkforge.toml's [[use]] id)."}},
		{Name: "import", Args: "SRC", run: cmdImport, JSON: true,
			Summary: "Build a recipe from a hand-written app (best effort): prints it, or --to DIR / --in-place",
			Opts: []Opt{
				{Name: "to", Arg: "DIR", Help: "Render the recipe into a new DIR and say whether flk reads its flk.toml like SRC's"},
				{Name: "in-place", Help: "Write SRC/flkforge.toml only (no other file changes)"},
				{Name: "app", Arg: "NAME", Help: "App name (default: SRC's)"},
				jsonOpt,
			},
			Notes: []string{
				"Reads SRC's flk.toml, Dockerfile and the files they copy; never changes them (--in-place adds flkforge.toml only).",
				"What matches a snippet becomes it; the rest is kept as is in core/flk-toml and core/dockerfile-lines. Every decision is a note.",
				"Without --to or --in-place the recipe goes to stdout and the notes to stderr: flkforge import ~/app > recipe.toml.",
			}},
		{Name: "hub", Args: "[SUB] [ARGS...]", run: cmdHub, JSON: true,
			Summary: "Shared snippets and recipes in git repos: hub list|add|rm|pull|search|show|publish",
			Opts: []Opt{
				{Name: "path", Arg: "DIR", Help: "add: use a clone you already have, in place"},
				{Name: "hub", Arg: "NAME", Help: "publish: the hub to publish to"},
				{Name: "as", Arg: "NS/NAME", Help: "publish: the id in the hub (default: the snippet's, or HUB/APP for a recipe)"},
				{Name: "branch", Arg: "B", Help: "publish: commit on this branch (default: a new flkforge/publish-NAME, for review)"},
				{Name: "current", Help: "publish: commit on the branch that's checked out"},
				{Name: "title", Arg: "T", Help: "publish a recipe: its title"},
				{Name: "summary", Arg: "S", Help: "publish a recipe: what it's for (hub search shows it)"},
				jsonOpt,
			},
			Notes: []string{
				"A hub is a git repo laid out like the built-in catalog: NAMESPACE/NAME/snippet.toml, NAMESPACE/_templates/NAME.toml. Hubs are in ~/.config/flkforge/hubs.toml.",
				"hub add NAME URL clones into ~/.local/share/flkforge/hubs/NAME on hub pull; hub add NAME --path DIR uses your clone in place. pull is git clone / git pull --ff-only.",
				"Hub snippets join the catalog (snippets, explain, the wizard). flkforge add ID needs --trust for one the recipe hasn't locked yet: hub show ID prints every file first. The recipe locks its version and sha256; a render refuses if it changed (render --relock accepts it).",
				"hub publish SRC copies a snippet directory, or an app's recipe as a template (app name and host addresses stripped), into the hub's checkout and commits it on a new branch for review (--current: the checked-out branch). flkforge never pushes.",
			}},
		{Name: "snippets", Args: "[SUB] [ID]", run: cmdSnippets, JSON: true,
			Summary: "List the snippets (snippets [list]), or show one's params (snippets show ID)",
			Opts: []Opt{
				{Name: "category", Arg: "C", Help: "Only this category"},
				{Name: "dir", Arg: "DIR", Help: "Include DIR's own snippets (DIR/.flkforge/snippets; default .)"},
				jsonOpt,
			},
			Notes: []string{"Built-in snippets, then ~/.config/flkforge/snippets, then the app's .flkforge/snippets; later ones override earlier ones by id."}},
		{Name: "templates", run: cmdTemplates, JSON: true,
			Summary: "List the templates: starting recipes for flkforge new --template",
			Opts:    []Opt{{Name: "dir", Arg: "DIR", Help: "Include DIR's own (default .)"}, jsonOpt}},
		{Name: "explain", Args: "WHAT", run: cmdExplain, JSON: true,
			Summary: "Explain a snippet, a rule, an ask that isn't a snippet (host-folder, gpu...), or a flk.toml key",
			Opts:    []Opt{{Name: "dir", Arg: "DIR", Help: "Include DIR's own snippets (default .)"}, jsonOpt},
			Notes: []string{
				"A snippet: what it does, under the hood, gotchas, where to read more.",
				"A rule (kill-timeout-margin...): what it checks and why. A key (kill_timeout, flk.backup): flk's own reference for it, read from flk help --all, and the snippets that set it.",
				"Asks that aren't snippets: " + "host-folder, lan-address, gpu, low-port, shell, systemd, docker-in-vm.",
			}},
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
