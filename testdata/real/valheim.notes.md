# valheim.toml vs ~/valheim.flk

Checked 2026-11-04 against valheim-flk as registered (flk config show
--json -a valheim-flk, read-only).

## flk.toml

`parsed` and `machine_config` are equal (TestReferenceApps), except the
run engine's own settings, which the real app doesn't have:
`RUN_STOP = "signal:INT"`, `RUN_READY`, `RUN_FILTER`. `STOP_TIMEOUT` and
the `RESTART_*` settings have the same names and values as vh-run's.

## Dockerfile

The same two stages and the same steps, with these differences:

- The steamcmd stage is the same loop, check and BUILDID file; the test
  is `test -e` instead of `test -x`, and the error names "set the new
  build id" instead of `bin/vh update apply` (flkforge doesn't know the
  app's scripts).
- vh-run is replaced by flk-run plus hooks. What vh-run does that
  flk-run doesn't, and where it would go:
  - building the server's arguments from `VH_*` (here, a `bash -c`
    command with the common ones; `VH_PASSWORD`, `-crossplay`,
    `-modifier`, `-setkey` and `VH_EXTRA_ARGS` would go in a prestart
    hook that writes the command line, or a small start script);
  - the access lists (`write_list`): the prestart hook;
  - the checks before the first start (volume writable, password rules):
    the prestart hook;
  - player events (join, leave, death, join code) from the log: the watch
    hook;
  - the `(Filename: ...)` filter: `RUN_FILTER` (blank lines are kept).
- `COPY flk-run` instead of `COPY --chmod=755 vh-run`: flkforge writes
  flk-run with mode 755.
- The header comment is generated from the snippets.

So this recipe reproduces valheim-flk's config exactly and its image's
shape, but not vh-run's game-specific behaviour, which is what hooks are
for (SPEC 6.6). It's reference material, not a migration (PLAN).
