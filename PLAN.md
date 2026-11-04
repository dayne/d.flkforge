---
updated: 2026-11-04
---

# Plan

Milestones in order. Each ends with an acceptance test you run and record
in `docs/history.md` before starting the next one. Commit at least once
per milestone; never push.

## M0: Skeleton and the format (no TUI)

- `go mod init github.com/dayne/d.flkforge` (the repo stays local for now), Makefile
  in lazyflk's style (`make`, `make build`, `make test`, `make install`).
- A command table that drives parsing and help; `flkforge help`,
  `help --all [--json]`, `version`. Strict arguments.
- The snippet format (SPEC 4): loader, param validation, the embedded
  source plus the directory sources, behind the `Source` interface.
- The recipe format (SPEC 5): read, write, lock.
- The renderer (SPEC 6), enough for three snippets: `base-alpine`,
  `http-service`, `volume`.
- `internal/flk`: run `flk config show --json` in a directory and return
  its warnings and machine_config.

**Accept:** `flkforge new /tmp/x --recipe testdata/hello.toml --yes`
writes files equivalent to `~/flk/examples/hello` (golden test), and
`flk config show --json` there has no warnings.

## M1: The real catalog

- Every snippet and template in `docs/snippet-sources.md`, with full hints.
- The rules in SPEC 6.4.
- `snippets`, `snippets show`, `templates`, `explain`.
- Golden tests: one recipe per real app (`testdata/real/*.toml`).

**Accept:** for minecraft-flk, vintagestory-flk, valheim-flk,
enshrouded-flk, mqtt-flk, ncm and opencode-sandbox, the rendered
flk.toml's `parsed` and `machine_config` (from `flk config show --json`)
equal the real app's (`flk config show --json -a APP`, read-only;
`machine_config` leaves out `[flk.backup]` and `keep_releases`, so compare
both), and every
Dockerfile difference is explained in a note next to the recipe.

## M2: The wizard

- The steps, live preview and hint panel from SPEC 8, at 80 columns and
  wider.
- `new` with a terminal, `edit`, and the no-TTY refusal.
- A `--demo` mode (or a test harness) that drives the wizard from a
  script, so it can be tested without a person.

**Accept:** pick the `steam-game` template, change the name, ports and
backup mode, and write. The result passes `flkforge check`. Then
`flkforge edit`, swap `backup-pause` for `backup-stop`, write again, and
only the expected lines change. Capture screens (as lazyflk does in
tmux) for the user to look over.

## M3: Remix and check

- `add`, `rm`, `set`; drift detection and `.flkforge-new` (SPEC 6.5).
- `check` with the full preflight output and `--json` (SPEC 7).
- `Next:` lines.

**Accept:** hand-edit a rendered flk.toml, then `render`: refused, with
a diff, and nothing overwritten. `check` names the drift and the fix.

## M4: End to end on a real VM

- Use the `web-service` and `steam-game` templates with app names
  `flkt-forge-web` and `flkt-forge-steam`, then follow the printed next
  steps exactly: `flk launch --no-deploy`, `flk volumes create`,
  `flk deploy`. Checks pass, `flk volumes backup` works in the chosen
  mode, and `flk machine stop` stops it cleanly through the wrapper (log
  shows the graceful path).
- Clean up: `flk apps destroy flkt-forge-... --yes`.

**Accept:** both deploy and pass their checks on the first try, with
nothing done by hand beyond the printed next steps. Anything that went
wrong becomes a snippet fix or a rule.

## Later

- Import: build a recipe from an existing hand-written app (best effort,
  matching known snippets).
- The hub: a third `Source`, plus `hub search|show|pull|publish`. Design it
  first (`TODO.md`).
- A lazyflk hook ("new app…" runs flkforge). Propose it to the lazyflk
  session, don't build it there.
