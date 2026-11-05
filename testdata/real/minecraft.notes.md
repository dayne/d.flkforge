# minecraft.toml vs ~/minecraft.flk

Checked 2026-11-04 against the app as registered (flk config show --json
-a APP, read-only). TestReferenceApps keeps the flk.toml half true.

## flk.toml

`parsed` and `machine_config` are equal, apart from flk-run's own
settings (`RUN_STOP = "console:stop"`, `RUN_READY`). kill_timeout 120 is
STOP_TIMEOUT 100 plus run-wrapper's `kill_margin = 20`; the live backup's
pre/post hooks, verify and before_deploy come from core/backup-live-console
and core/before-deploy.

## Dockerfile

- flk-run plus a watch hook instead of mc-run: player join/leave/death
  events (mc-run's `watch_output`) belong in `flk-run.d/watch`; the
  server-console FIFO, the console stop and the restart loop are flk-run's.
  The JVM flags move from mc-run into a `bash -c` command that expands
  `$MC_HEAP` and `$MC_JAVA_OPTS` from [env] (mc-run also took `MC_JAR`).
- `tzdata` is in the install line (core/timezone always adds it). The
  Temurin image already has it (checked 2026-11-04), so it's a no-op here.
- `RUN install -d -o ubuntu -g ubuntu /server`: the mount point owned by
  uid 1000 (core/volume); harmless, the world volume's files are 1000's.
