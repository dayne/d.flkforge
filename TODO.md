# TODO

Open questions. Decide the ones marked (M0) before writing code that
depends on them; ask the user when it's their call.

## When flk's r1-reliability reaches main

The flk session says when Dayne merges it. Then, and not before (the
real apps and the boot service run main):
- `core/check-http`: drop the "Gaps in flk" step (BUGS.flk.md #1, flk 4294558),
  then redeploy the hello slice with the check as flkt-forge-hello.
- Rule `log-check-dollar` and the check-log hint/gotcha about `$`
  (BUGS.flk.md #2, flk a8461a0); the primer's line about it.
- The primer's note on busybox setpriv crashing init (flk c887f14).

## Decisions

- ~~(M0) Final `snippet.toml` field names~~: settled 2026-11-04 (SPEC 4).
- ~~(M0) Forms~~: decided 2026-11-04: `charm.land/huh/v2` exists
  (v2.0.3); use it for the forms (M4). The preview and hint panes are
  built on bubbletea.
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
- (M2) How much of flk's own help to show inline versus pointing at
  `flk help ...`. Inline text goes stale; pointers don't.

## Hub (later, design only)

- Transport: a git repo of snippets (simple, reviewable) or an HTTP index.
- Trust: signing, review, pinning by hash; what `add` shows before using
  a non-local snippet.
- Publishing: `flkforge hub publish` from a recipe, and what gets
  stripped (site-specific values, secret names?).
- Whether lazyflk browses the hub too.

## From M3

- `core/sshd` (deferred): sshd is a second, root-owned process next to the
  app, with host keys made at first boot on a volume and a public key from
  the build context or a secret. It needs a design for running two
  processes (flk-run's prestart? a tiny supervisor?) and real SSH clients
  to test with. `flk ssh console` covers shells meanwhile
  (`flkforge explain shell`).
- The reference recipes show what the real apps' wrappers do that flk-run
  hooks would have to: worth offering to a game repo's owner as a port
  (Dayne's call), not doing here.

## From M2's agent test

- The report could show the port mapping (host 6390 -> VM 6390/tcp).
- App-specific snippets (redis, postgres, mosquitto) that tie the
  program's own config (its port, its data dir, foreground mode) to the
  service and volume params, so they can't drift apart (M3).

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
