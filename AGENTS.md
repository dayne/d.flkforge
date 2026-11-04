# Building flkforge (for AI agents)

You're building **flkforge**: a CLI and terminal wizard that helps an admin
put together a new flk app (a `Dockerfile`, a `flk.toml`, and a run wrapper
when one is needed) from **snippets**: small, reusable, documented pieces
that can be mixed and remixed. Along the way it tells the admin what each
choice does under the hood. A later version will browse a **hub** of
shared snippets and recipes; v1 doesn't build the hub, but it's designed
so the hub can be added without rework.

Nothing is implemented yet. This folder holds the design and the plan.
Read, in this order:

1. `README.md`: what flkforge is, in a page.
2. `docs/flk-primer.md`: the flk ecosystem. **Read it before writing any
   code.** It covers how flk works, how to ask flk questions, which apps
   are real, and who owns what.
3. `SPEC.md`: the design you're building to (concepts, snippet and recipe
   formats, rendering, the wizard, the CLI, hints, hub readiness). It's
   the source of truth; change it in the same commit as code that departs
   from it.
4. `docs/snippet-sources.md`: the starting snippet catalog, taken from
   the apps that already run on this host, with the file each one comes
   from.
5. `PLAN.md`: milestones in order, each with its acceptance test.
   `TODO.md`: open questions and decisions still to make.

## Ground rules

- **flk is a separate project you don't edit.** It's Ruby, at `~/flk`
  (github.com/dayne/d.flk), and another agent leads it. When flkforge
  needs something flk doesn't have (a validation command, a JSON field),
  write it up in `BUGS.flk.md` in this repo (format in its header) and tell
  the user. Never patch `~/flk`, and don't work around a gap by reading
  flk's state directory (`~/.local/share/flk`).
- **Learn flk from flk, not from memory.** `flk help --all` is the full
  reference, including the flk.toml subset. `flk help --all --json` has the
  command table and a `schema` number, and `flk context --all` shows the
  live host. Snippet hints must match what flk actually does: check them
  against `flk help --all` and `~/flk/examples/reference/flk.toml`.
- **Validate with flk.** `flk config show --json`, run in the output
  directory, parses a flk.toml without registering or deploying anything
  and lists unknown keys under `warnings`. Every render should pass it
  with zero warnings (see SPEC section 7).
- **Don't touch real apps.** Every app on this host not named `flkt-*` is
  real: game servers with players' worlds (`minecraft-flk`,
  `vintagestory-flk`, `enshrouded-flk`, `valheim-flk`), the house MQTT
  broker (`mqtt-flk`), `ncm`, `opencode-sandbox`, and any that show up
  later. Read-only commands against them are fine (`flk context`,
  `flk config show -a APP`, `flk status --json`). Anything that launches,
  deploys, starts, stops or destroys uses an app named `flkt-forge-*`,
  which you clean up with `flk apps destroy NAME --yes`.
- **Read the app repos, never write to them.** `~/minecraft.flk`,
  `~/vintagestory.flk`, `~/valheim.flk`, `~/enshrouded.flk`, `~/mqtt.flk`,
  `~/ncm.flk`, `~/opencode.flk` and `~/m.gamebus` are the source material
  for snippets. Each has its own owner. Copy what you learn from them into
  snippets here.
- **Never run sudo.** If something needs root (flk's `setup-host.sh`, a
  sysctl), show the admin the command and stop. flkforge never runs root
  steps either: it prints them as next steps.
- **No hanging without a terminal**, as in flk: with no TTY the wizard
  refuses to start and names the non-interactive form
  (`flkforge new --recipe FILE`). Unknown options and extra arguments are
  errors.
- **Local repo, no remote.** Commit as you go with clear messages, and
  never push. The user will add a remote when they're ready.

## Stack

Go, with Charm's libraries, the same stack as lazyflk (`~/d.lazyflk`,
see its `go.mod`): `charm.land/bubbletea/v2` and `charm.land/lipgloss/v2`.
For forms, check whether `huh` has a release that works with bubbletea v2
before you depend on it. If none does, build the few form widgets you need
on bubbletea, the way lazyflk did. Read lazyflk's `AGENTS.md`, `Makefile`
and `internal/` layout and copy its conventions (Makefile targets,
`make install` linking `~/.local/bin/flkforge`, `--demo`-style offline
mode, fixtures captured from real flk output). Go is at `~/.local/go/bin`.

Suggested layout (adjust as needed, and record changes in SPEC):

    cmd/flkforge/         main, argument parsing, help
    internal/snippet/     the snippet format: load, validate, params
    internal/source/      where snippets come from (embedded, dirs; hub later)
    internal/recipe/      flkforge.toml: read, write, lock
    internal/render/      Dockerfile and flk.toml assembly, rules, drift
    internal/flk/         calls to the flk CLI (config show, help --all, apps list)
    internal/wizard/      the TUI
    snippets/             the built-in catalog (embedded with go:embed)
    testdata/             recipes and the expected output for golden tests

Keep `internal/render` free of I/O and of the TUI so it can be tested on
its own. The wizard and the CLI are two front ends over the same recipe
and render code.

## When you change something

- Update `SPEC.md` if the design moved, and `README.md` if a command
  changed.
- Snippet hints are user documentation: keep them accurate and short, and
  say where in flk's help to read more.
- `go test ./...` passes before every commit, and so does the golden suite
  (`make test`).
- Record finished milestones and decisions in `docs/history.md`, with
  absolute dates.
