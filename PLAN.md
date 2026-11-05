---
updated: 2026-11-04
---

# Plan

Milestones in order. Each ends with an acceptance test you run and record
in `docs/history.md` before starting the next one. Commit at least once
per milestone; never push.

Order (decided with Dayne, 2026-11-04): **agents and the CLI first, the
wizard last**, and **a vertical slice before breadth**. Every milestone
up to M1 ends with a real VM deployed from flkforge's output, so a flaw
in the snippet model shows up before most of the catalog is written. The
real apps are **reference material, not migration targets**: compare
with them where it's cheap, and explain the differences in notes.

## M0: Skeleton and the hello slice

- `go mod init github.com/dayne/d.flkforge`, Makefile in lazyflk's style
  (`make`, `make build`, `make test`, `make install`).
- A command table that drives parsing and help; `flkforge help`,
  `help --all [--json]`, `version`. Strict arguments.
- The snippet format (SPEC 4), with flk.toml fragments as structured
  records (SPEC 6.2); loader, param validation, the embedded source and
  the directory sources behind the `Source` interface.
- The recipe format (SPEC 5): read, write, lock (flkforge version only
  while every snippet is built in).
- The renderer (SPEC 6), enough for `base-alpine`, `http-service`,
  `volume`, `check-http`.
- `new --recipe FILE --yes`, `render`, a basic `check` (recipe parses,
  no drift, `flk config show --json` has no warnings), `--json` on all
  three, and the four-part report (SPEC 8, step 13) as plain text and JSON.
- `internal/flk`: run `flk config show --json` in a directory and return
  its warnings, `parsed` and `machine_config`.

**Accept:** `flkforge new /tmp/x --recipe testdata/hello.toml --yes`
writes files equivalent to `~/flk/examples/hello` (golden test), with no
warnings from `flk config show --json`. Then, with app name
`flkt-forge-hello`, run the printed next steps exactly
(`flk launch --no-deploy`, `flk volumes create`, `flk deploy`). It's
healthy, and `flk apps destroy flkt-forge-hello --yes` cleans up.

## M1: The run engine and one supervised app, end to end

- `flk-run` (SPEC 6.6): one versioned bash script, copied as is. It
  handles the graceful stop (a console line or a signal), the
  `STOP_TIMEOUT` escalation, the restart loop, the ready line and the
  `server.*` gamebus events. Optional `prestart` and `watch` hooks hold
  the app-specific parts, and flkforge writes stub hooks for the admin
  to fill in.
- Its own test suite runs on the host against a fake server process:
  console stop, signal stop, escalation, restart, give-up, stop during
  the restart delay, events, and that the hooks are called.
- The snippets behind a supervised server: `base-ubuntu`, `run-wrapper`
  (the engine plus hooks), `gamebus-events`, `user-1000`, `volume`,
  `service` (udp and tcp), `check-log`, `backup-pause`, `backup-stop`,
  `before-deploy`, `restart-outer-net`, `stdin-null`, `kill-timeout`;
  `steamcmd` is written and render-tested, but not deployed.
- The rules SPEC 6.4 lists for these (kill_timeout vs STOP_TIMEOUT,
  restart retries, volume ownership, UDP-only checks, before_deploy).
- Reference: a `testdata/real/valheim.toml` recipe whose rendered
  `parsed` and `machine_config` match `flk config show --json -a
  valheim-flk` (read-only), with the differences explained in a note.

**Accept:** a recipe for `flkt-forge-run` using a **fake server** (a
small script in the image that logs a ready line, writes a file to the
volume, and saves on stop; no Steam download) deploys by following the
printed next steps. Its checks pass. `flk machine stop` goes through the
engine's graceful path (the log shows it), killing the fake server
triggers a restart, `flk volumes backup` works in `pause` and in `stop`
mode, and the saved file is in the backup. Then clean up.

## M2: The agent path, complete

- `snippets [--category] [--json]`, `snippets show`, `templates`,
  `explain SNIPPET|RULE|KEY` (`--json` everywhere).
- `add`, `rm`, `set`; drift detection and `.flkforge-new` (SPEC 6.5).
- `check` with the full preflight output and `--json` (SPEC 7).
- All the rules in SPEC 6.4; the limits catalog (SPEC 8.1).
- `Next:` lines.

**Accept:** an agent with no prior knowledge builds the `daemon` app
with only `flkforge help --all --json`, `snippets --json`, `explain` and
`add`/`set`, and it passes `check`. Hand-edit a rendered flk.toml, then
`render`: refused, with a diff, and nothing overwritten. `check` names
the drift and the fix.

## M3: The rest of the catalog

- Every snippet and template in `docs/snippet-sources.md`, with full
  hints.
- Golden tests for every template. For each real app, a reference
  recipe in `testdata/real/`: compare `parsed` and `machine_config` where
  it's cheap, and explain the remaining differences in a note. They're
  reference material, so exact reproduction isn't required.

**Accept:** every template renders, passes `check`, and has a golden
test. `web-service` deploys as `flkt-forge-web`, then gets cleaned up.

## M4: The wizard

- The steps, live preview and hint panel from SPEC 8, at 80 columns and
  wider. Forms use `charm.land/huh/v2`; the preview and hint panes are
  built on bubbletea.
- `new` with a terminal, `edit`, and the no-TTY refusal.
- A script harness that drives the wizard, so it can be tested without a
  person.

**Accept:** pick a template, change the name, ports and backup mode, and
write. The result passes `flkforge check`. Then `flkforge edit`, swap
`backup-pause` for `backup-stop`, write again, and only the expected
lines change. Capture screens (as lazyflk does in tmux) for the user to
look over.

## Alongside: flk's M6

As flk ships each item in its M6 (`flk build`, image warnings, `exec`
checks, `[[mounts]] seed = true`, keys as JSON, the guest env list),
adopt it: a `check` step, a snippet that replaces a workaround, a rule
that's no longer needed. Record each one in `BUGS.flk.md` (Status) and
`docs/history.md`. Nothing above waits for these.

## Later

- ~~Import~~: done 2026-11-05 (`flkforge import`, SPEC 9.1).
- ~~The hub~~: done 2026-11-05 (SPEC 10): git hubs, trust and the lock,
  publish on a branch.
- ~~A lazyflk hook~~: proposed 2026-11-05 in docs/lazyflk-proposal.md
  ("new app…" hands the terminal to flkforge new; remix and check for
  apps with a recipe). No lazyflk session was running: Dayne passes it on.
