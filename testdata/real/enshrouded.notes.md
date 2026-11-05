# enshrouded.toml vs ~/enshrouded.flk

Checked 2026-11-04 against the app as registered (flk config show --json
-a APP, read-only). TestReferenceApps keeps the flk.toml half true.

## flk.toml

`parsed` and `machine_config` are equal, apart from flk-run's settings
(`RUN_STOP = "signal:INT"`, `RUN_ESCALATE = "KILL"`,
`RUN_ESCALATE_CMD = "wineserver -k"`, `RUN_READY`). The group passwords
are core/secret uses (EN_PW_ADMIN, EN_PW_FRIEND): names only.

## Dockerfile

- One stage instead of two (`wine`, then `server` FROM it): the real file
  names the first stage so `--target wine` can stop before the 7.6 GB
  download (its M1 spike). core/base-wine plus core/steamcmd
  `single_stage = true` give the same layers in the same order.
- The steamcmd step also creates /server owned by ubuntu (the real file
  does that in the wine stage). It doesn't remove
  `~ubuntu/.local/share/Steam/logs` as the real one does: a small
  difference in image size.
- flk-run plus hooks instead of en-run: writing enshrouded_server.json from
  [env] and secrets, and the group checks, go in `flk-run.d/prestart`
  (it runs before every start, as en-run's write_config did); keeping the
  bans on exit goes in `flk-run.d/poststop`; the player events and the
  log filter go in `flk-run.d/watch` and `RUN_FILTER`. en-run's SIGINT
  stop, SIGKILL escalation and `wineserver -k` are flk-run settings.
