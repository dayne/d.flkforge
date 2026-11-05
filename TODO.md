# TODO

Open questions. Decide the ones marked (M0) before writing code that
depends on them; ask the user when it's their call.

## Decisions

- ~~(M0) Final `snippet.toml` field names~~: settled 2026-11-04 (SPEC 4).
- ~~(M0) Forms~~: decided 2026-11-04 for huh v2, then changed in M4
  (2026-11-05): hand-built widgets on bubbletea, as lazyflk does (SPEC
  8.2 has the reasons).
- ~~(M1) Run wrapper shape~~: decided 2026-11-04: one versioned engine
  (`flk-run`) plus hooks the admin owns (SPEC 6.6).
- ~~(M1) The engine's env names and hooks~~: settled (SPEC 6.6).
- ~~(M1) The fake server~~: bash, `testdata/fakeserver/fakeserver`.
- (M2) A check-tcp snippet, and `bind`, `site-env`, `backup-live-console`,
  `backup-verify`: the rest of the catalog's network/backup rows.
- (M2) Arguments built from env for flk-run (Valheim's `-name "$VH_NAME"`):
  today `bash -c` or a prestart hook. Maybe a RUN_ARGS_FILE that prestart
  writes. Decide when a second app needs it.
- Report to the game repos' owners (not ours to change): vh-run, mc-run,
  vs-run and en-run share the restart-delay race flk-run fixed (SPEC
  6.6). Small window; mention it to Dayne rather than filing in their repos.
- ~~(M1) A "made by flkforge" header~~: done in M0, two comment lines in
  each managed file pointing at `flkforge.toml` (seed files get none:
  they're the admin's).
- (M1) Volume names: flk's backup file names only allow `[a-z0-9_]`, so
  `core/volume` refuses dashes. Confirm with the flk session whether
  `flk volumes create` itself refuses them, and match it exactly.
- ~~(M2) check: deployed or only registered~~: done (flk status --json).
- (M2) Show the diff itself on drift (now: the `diff -u` command to run).
- ~~(M2) Inline flk help vs pointers~~: pointers in hints ("More: flk
  help ..."), and `flkforge explain KEY` quotes flk's own reference live.

## Hub

- ~~Transport, trust, publishing, lazyflk~~: decided and built
  2026-11-05 (SPEC 10).
- Signing (signed commits, or a hub's allowed keys) before hubs from
  other people are trusted.
- `hub publish` could strip more site-specific values (a TZ, a world
  name): today it removes the app name and core/bind, and flags IP-like
  values.

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
