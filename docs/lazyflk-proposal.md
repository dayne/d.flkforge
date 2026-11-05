---
date: 2026-11-05
from: flkforge's lead session
to: lazyflk's session (via Dayne: no lazyflk session was running)
status: proposal, nothing built in lazyflk
---

# Proposal: flkforge in lazyflk

PLAN (Later) has "a lazyflk hook: new app… runs flkforge; propose it to
the lazyflk session, don't build it there". This is that proposal. It
needs nothing new from flkforge: everything below uses commands that
exist today.

## What lazyflk already has (read, not changed)

- `tea.ExecProcess` to hand the terminal to another program
  (internal/ui/edit.go, console.go).
- App actions from a `.lazyflk.toml` next to an app's flk.toml (SPEC 10.4
  there), with `{dir}`, `{app}` and `{input}` placeholders, run with
  `sh -c` in the app's directory after a confirm.

## 1. "New app…" (the PLAN item)

A host-level action (palette or menu):

1. Ask for a directory (`{input}`), e.g. `~/mygame.flk`.
2. Hand the terminal to `flkforge new DIR` (`tea.ExecProcess`). That's
   flkforge's wizard: templates, steps, live preview, write. It needs a
   terminal (without one it refuses and says so); lazyflk gives it one.
3. When it exits, show what it printed last (the report ends with the flk
   commands to run: `flk launch --no-deploy`, `flk volumes create`,
   `flk deploy`) and refresh. flkforge's exit status: 0 written, 1 not
   written (quit, or refused), 2 usage.

If `flkforge` isn't on PATH, hide the action or say how to get it
(`make install` in ~/d.flkforge links ~/.local/bin/flkforge).

## 2. For apps made with flkforge (optional)

An app directory with a `flkforge.toml` was made (or imported) with
flkforge. lazyflk could offer, for those apps only:

- **Remix**: hand the terminal to `flkforge edit {dir}` (the wizard on
  the existing recipe; it re-renders and refuses to overwrite files
  changed by hand, writing FILE.flkforge-new and a diff instead).
- **Check**: `flkforge check {dir} --json`, read-only. Shape:
  `{"dir","app","checks":[{"group","status","message","fix"}],"ok","warn","fail","next"}`;
  status is ok, warn or FAIL; exit 1 on any FAIL. It covers the recipe,
  drift, `flk config show`, host ports, the deployed release and whether
  flk.toml changed since, and unset secrets. A good fit for a panel or
  the attention list.

The second one works today with no lazyflk code, as an app action:

    [actions.forge-check]
    run  = "flkforge check {dir}"
    help = "flkforge's preflight: recipe, files, flk, host"

`edit` can't be an app action today (actions capture output; the wizard
needs the terminal), hence the ExecProcess suggestion.

## Contract

flkforge keeps these stable: `help --all --json` carries `"schema": 1`
(new fields can appear; renames bump it), and every listing has `--json`
(`templates`, `snippets`, `check`, `hub search`). lazyflk would only need
`templates --json` if it wanted its own template picker; Dayne decided
(2026-11-05) that lazyflk doesn't browse flkforge's hubs.

## Not proposed

lazyflk writing recipes or flk.toml itself, or reading flkforge's files
directly: flkforge's CLI is the interface, the way flk's is lazyflk's.
