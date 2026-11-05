---
updated: 2026-11-04
---

# The starting catalog, and where each snippet comes from

Every v1 snippet comes from a pattern that already works on this host.
Read the source file before writing the snippet: the comments in it
explain *why*, and that's what the hints need. Ids are `core/...`; the
names here are suggestions.

## Bases (category `base`)

| Snippet | Image | Family / uid-1000 user | Learned from |
|---|---|---|---|
| `base-ubuntu` | `docker.io/library/ubuntu:24.04` | apt / ubuntu | valheim.flk, opencode.flk |
| `base-debian` | `docker.io/library/debian:12-slim` | apt / none: create one | (new) |
| `base-alpine` | `docker.io/library/alpine:3.20` | apk / none | flk examples/hello, mqtt.flk |
| `base-temurin` | `docker.io/library/eclipse-temurin:25-jre` | apt / ubuntu | minecraft.flk |
| `base-dotnet` | `mcr.microsoft.com/dotnet/runtime:10.0` | apt / ubuntu | vintagestory.flk |
| `base-python` | `docker.io/library/python:3.12-slim` | apt / none | ncm.flk |
| `base-wine` | `docker.io/steamcmd/steamcmd:ubuntu-24` + wine64, a prefix built at image time | apt / ubuntu | enshrouded.flk |
| `base-image` | any image: ask for the family and user | | `[build] image` in flk's reference |

Note: a param picks the version tag. Show the real image's size in the hint
if it's cheap to find out.

## Getting the software in (`fetch`)

| Snippet | What | Learned from |
|---|---|---|
| `packages` | extra packages, merged into the base's install line | every Dockerfile |
| `download-sha256` | a URL or a `dist/` file, checked against a sha256 build ARG, unpacked to /server | vintagestory.flk (VS_VERSION / VS_SHA256) |
| `checksum-manifest` | a folder of files (mods) checked against a `*.sha256` manifest | vintagestory.flk (mods) |
| `steamcmd` | a separate stage, retries, a build-id pin checked against appmanifest, BUILDID file; `platform = windows` for Wine | valheim.flk (two stages), enshrouded.flk (one stage to save disk: a param) |
| `git-pinned` | clone + checkout of a pinned ref, optional `git apply` patch | ncm.flk |
| `pip` / `apk` / `apt` extras | language packages (CPU torch wheel trick as a hint) | ncm.flk |
| `on-volume` | nothing in the image: the program lives on the volume (explain the trade-off: no rollback of the program) | minecraft.flk |

## Process (`wrapper`, `restart`)

| Snippet | What | Learned from |
|---|---|---|
| `run-wrapper` | the `flk-run` engine, copied as is (SPEC 6.6): graceful stop (console line / signal), STOP_TIMEOUT escalation, restart loop (RESTART_DELAY, MAX_RESTARTS, RESTART_WINDOW), give up → VM off, ready line, `server.*` events; plus stub `prestart` / `watch` / `poststop` hooks for what's specific to the app | mc-run, vs-run, vh-run, en-run (what they share is the engine; how they differ goes in hooks), mqtt-run (`--exec`) |
| `gamebus-events` | the `gamebus_event` function, copied verbatim with its "keep in step" marker | `~/m.gamebus/adapters/gamebus-event.sh`, `SPEC.md` |
| `restart-outer-net` | `[[restart]] on-failure, retries = 1` with the comment explaining why | the four game flk.tomls |
| `restart-always` | infrastructure that always comes back | mqtt.flk |
| `idle-cmd` | keep a sandbox VM up; exits cleanly on SIGTERM | opencode.flk |
| `entrypoint-clear` | `ENTRYPOINT []` for images with their own entrypoint | mqtt.flk, enshrouded.flk |
| `stdin-null` | `[flk] stdin = "null"` for apps with no console | valheim.flk |

## User and data (`user`, `data`)

| Snippet | What | Learned from |
|---|---|---|
| `user-1000` | run as uid 1000 (use the base's user or create `app`), HOME, WORKDIR; on busybox bases also add `setpriv` (or init runs the app as root) | all game Dockerfiles, `~/flk/share/init.sh` |
| `user-rename` | rename the uid-1000 user, optional passwordless sudo (sandboxes only, with a hint saying so) | opencode.flk |
| `volume` | `[[mounts]]` + mount point owned by the app user + the `flk volumes create` next step | valheim.flk BUGS #4, all apps |
| `volume-links` | symlinks from the program's folder to the volume | ncm.flk |
| `home-volume` | home directories on a volume, seeded from the image (`/etc/skel` or a copy) at first boot, because the mount hides what the image put there | (new; from the genma notes) |
| `sshd` | for real SSH clients only (`flk ssh console` covers shells): sshd in the foreground (no systemd), host keys made at first boot on the volume, the public key from the build context or a secret, never a private key | (new; from the genma notes) |
| `site-env` | host-specific settings kept out of git (`site.env.example`) | valheim.flk |

## Network and health (`network`, `check`)

| Snippet | What | Learned from |
|---|---|---|
| `service` | one `[[services]]` (port, tcp/udp, host port) | minecraft.flk, vintagestory.flk (same port on tcp and udp) |
| `http-service` | `[http_service]` + optional `[flk] http_port` | flk examples/hello, reference |
| `check-log` | regex for the ready line, grace period | all games, mqtt.flk |
| `check-tcp` / `check-http` | port or path checks | minecraft.flk, flk reference |
| `bind` | `[flk] bind` to one host address | flk reference, enshrouded-flk's port on 10.47.11.7 |

## Backups (`backup`)

| Snippet | What | Learned from |
|---|---|---|
| `backup-stop` | default mode, with the downtime explained | vintagestory.flk |
| `backup-live-console` | `live` + console `pre`/`post` hooks with wait regexes | minecraft.flk |
| `backup-pause` | freeze the VM for the copy (like a power cut) | valheim.flk, mqtt.flk |
| `backup-verify` | a `verify` script stub using `debugfs` to check that key files exist | mc/vs/vh `bin/*-verify-backup` |
| `before-deploy` | `before_deploy = true` + more `keep_releases` | the games, mqtt.flk |

## Resources and misc (`vm`, `env`, `misc`)

`vm` (cpus, memory, huge_pages), `rootfs-extra`, `keep-releases`,
`timezone` (TZ + tzdata), `internal-hosts`, `secret` (a name and a next
step; never a value), `kill-timeout`.

## Templates

| Template | Snippets | Check it reproduces |
|---|---|---|
| `steam-game` | base-ubuntu, steamcmd, run-wrapper, user-1000, volume, service(udp), check-log, backup-pause, before-deploy, restart-outer-net, stdin-null | valheim.flk |
| `steam-game-windows` | base-wine, steamcmd(windows), run-wrapper(sigint) ... | enshrouded.flk |
| `java-server` | base-temurin, on-volume, run-wrapper(console stop), backup-live-console, check-log+tcp, two services | minecraft.flk |
| `dotnet-server` | base-dotnet, download-sha256, checksum-manifest, run-wrapper, backup-stop | vintagestory.flk |
| `daemon` | base-alpine, packages, entrypoint-clear, restart-always, backup-pause | mqtt.flk |
| `web-service` | base-alpine, http-service, check-http, volume | flk examples/hello |
| `python-workbench` | base-python, git-pinned, pip, volume-links | ncm.flk |
| `dev-sandbox` | base-ubuntu, user-rename, idle-cmd, rootfs-extra; options: `home-volume`, `sshd`. Tools from system packages or pinned downloads, not `curl \| bash` into `~`, so `flk deploy` upgrades them and a home volume doesn't hide them | opencode.flk |
| `blank` | a base only | |

The "Check it reproduces" column is the acceptance test (PLAN M1): a
recipe for each real app must render a flk.toml whose
`flk config show --json` `parsed` and `machine_config` equal the real
one's (`flk config show --json -a APP`, read-only), and a Dockerfile that does
the same steps. Byte-for-byte equality isn't the goal, but explain every
difference in `testdata/`.
