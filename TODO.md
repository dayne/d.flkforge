# TODO

Open questions. Decide the ones marked (M0) before writing code that
depends on them; ask the user when it's their call.

## Decisions

- ~~(M0) Final `snippet.toml` field names~~: settled 2026-11-04 (SPEC 4).
- ~~(M0) Forms~~: decided 2026-11-04: `charm.land/huh/v2` exists
  (v2.0.3); use it for the forms (M4). The preview and hint panes are
  built on bubbletea.
- ~~(M1) Run wrapper shape~~: decided 2026-11-04: one versioned engine
  (`flk-run`) plus hooks the admin owns (SPEC 6.6).
- (M1) The engine's exact env names and hook interface: settle them while
  writing its test suite, then update SPEC 6.6.
- (M1) The fake server for the M1 deploy test: a few lines of bash, or
  a tiny Go binary built in a stage. Bash is enough unless it needs to
  behave like a real server in some way bash can't.
- ~~(M1) A "made by flkforge" header~~: done in M0, two comment lines in
  each managed file pointing at `flkforge.toml` (seed files get none:
  they're the admin's).
- (M1) Volume names: flk's backup file names only allow `[a-z0-9_]`, so
  `core/volume` refuses dashes. Confirm with the flk session whether
  `flk volumes create` itself refuses them, and match it exactly.
- (M2) `check` can't tell whether the app is deployed, only registered:
  read `flk status --json` for the release and machine state.
- (M2) Show the diff itself on drift (now: the `diff -u` command to run).
- (M2) How much of flk's own help to show inline versus pointing at
  `flk help ...`. Inline text goes stale; pointers don't.

## Hub (later, design only)

- Transport: a git repo of snippets (simple, reviewable) or an HTTP index.
- Trust: signing, review, pinning by hash; what `add` shows before using
  a non-local snippet.
- Publishing: `flkforge hub publish` from a recipe, and what gets
  stripped (site-specific values, secret names?).
- Whether lazyflk browses the hub too.

## Ideas

- Smoke checks: snippets declare `[[smoke]]` commands (the volume is
  mounted, the tool runs, the app runs as the expected user), and
  `flkforge verify -a APP` runs them over `flk exec`. That's read-only, so
  it's safe on real apps too, and it covers what flk's deploy checks don't
  (they only see ports and log lines). It also confirms which env vars a
  guest really sees. Make it part of PLAN M4 if it's cheap.

- `flkforge explain KEY` for any flk.toml key, built from flk's reference
  plus the snippets that set the key.
- Import an existing app into a recipe (PLAN, Later).
- A `preflight` snippet that writes a `bin/APP preflight` in the
  PREFLIGHT.md style.
