# vintagestory.toml vs ~/vintagestory.flk

Checked 2026-11-04 against the app as registered (flk config show --json
-a APP, read-only). TestReferenceApps keeps the flk.toml half true.

## flk.toml

`parsed` and `machine_config` are equal, apart from flk-run's settings
(`RUN_STOP = "console:/stop"`, `RUN_READY`). The backup has no mode
(flk's default, stop), which is why the recipe uses core/backup-verify
alone rather than core/backup-stop (that one writes `mode = "stop"`).

## Dockerfile

- The same pinned tarball and mod manifest steps (core/download-sha256 with
  `args_in_flk_toml = false`, core/checksum-manifest), with generic temp
  names and the ARGs next to their steps instead of at the top.
- flk-run plus hooks instead of vs-run: deaths from the audit log, storm
  polling (`STORM_POLL` with `console /nexttempstorm` from `on_tick`) and
  join/leave events go in `flk-run.d/watch`; vs-run's check that
  /data/Saves exists goes in `flk-run.d/prestart`.
- `tzdata` in the install line (core/timezone): the .NET runtime image
  already has it (checked 2026-11-04), so it's a no-op here.
