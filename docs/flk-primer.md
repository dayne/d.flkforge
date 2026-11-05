---
updated: 2026-11-04
---

# The flk ecosystem: a primer for flkforge

What you need to know about flk and its neighbours to build flkforge. It's
a map, not a reference: flk documents itself, and its help is the truth
when the two differ. Written 2026-11-04 against flk 0.1.0 (`~/flk`
commit 6958961).

## 1. Ask flk

    flk context                 this directory's app, host health, next steps
    flk context --all           every app on the host
    flk help --all              the full reference: concepts, every command,
                                the flk.toml subset, gotchas, exit codes
    flk help --all --json       the command table, with "schema" and "version"
    flk help COMMAND            one command's options
    flk help cheatsheet         one screen, by task
    flk help first-app          the guide flkforge's wizard follows
    flk config show [--json]    what flk understood from ./flk.toml, with warnings

Every key flk reads, annotated: `~/flk/examples/reference/flk.toml`. The
smallest working app: `~/flk/examples/hello/`. The long-form reference
source is `~/flk/share/guide.md`, and the guides by stage are
`~/flk/share/help/*.md`.

## 2. How flk works (what flkforge's hints explain)

- **App**: a directory with a `flk.toml` (or an unchanged `fly.toml`). Its
  name is `app = "..."`: lowercase letters, digits and dashes.
- **Build**: `flk deploy` builds the Dockerfile with podman (or docker) and
  turns the image into an ext4 root filesystem, release `vN`. The root
  filesystem is thrown away on every deploy, so anything that must
  survive goes on a volume. `[build] args` become build ARGs, which is how
  apps pin versions (`STEAM_BUILDID`, `VS_VERSION`). `[build] image` skips
  the build and uses a prebuilt image.
- **Image names**: podman on Ubuntu has no search registries, so
  `FROM ubuntu:24.04` fails with "short-name did not resolve". Write
  `FROM docker.io/library/ubuntu:24.04`. flkforge should always emit full
  names. (`~/opencode.flk/Dockerfile` still uses the short form.)
- **Machine**: one Firecracker VM per machine, with a fixed IP on
  `172.30.0.0/24` that the host can reach. The image's ENTRYPOINT/CMD is
  the app. Its stdin is the serial console (`flk console`), unless
  `[flk] stdin = "null"`. When the app exits, the VM powers off.
- **Env inside**: `[env]`, secrets, `FLY_APP_NAME`, `FLY_MACHINE_ID`,
  `FLY_PRIVATE_IP`, `FLY_REGION=local`, the `FLK_*` equivalents, and
  `FLK_RELEASE`. A snippet should rely only on documented ones. Check them
  on a test machine with `flk exec env`.
- **The guest is a VM, not a container** (`~/flk/share/init.sh`, about
  140 lines: read it). `/flk/init` is PID 1: that shell script when the
  image has `/bin/sh`, otherwise the static flk agent (`~/flk/agent/init.go`),
  which flk only has if Go was on the host when it built the agent. It
  writes the
  hostname, `/etc/resolv.conf` and `/etc/hosts`, mounts the volumes, then
  runs the image's ENTRYPOINT/CMD as the one app. There's no systemd, so
  a daemon (sshd, cron) runs in the foreground, either as the app or
  started by the run wrapper. Images shouldn't manage networking.
  To run as a non-root `USER`, init needs `setpriv` (util-linux) or a
  `chroot` that takes `--userspec`. Without either, it logs a warning and
  **runs the app as root**. Busybox's `setpriv` applet lacks `--reuid`, so
  on Alpine a non-root user needs util-linux's: the `setpriv` package
  (checked 2026-11-04). Until flk c887f14 (branch r1-reliability), init
  even called busybox's and died at boot. The no-shell init switches users itself. A fix in flk is planned
  (`~/flk/TODO.md`, Machines); until it lands, keep the rule.
- **Getting a shell**: `flk ssh console` and `flk exec CMD` reach a
  running machine through flk's agent over vsock. No sshd or network is
  needed, and the image doesn't need anything for it (the
  opencode-sandbox's idle CMD exists so there's a machine to land in). An
  sshd in the image is only for clients that need real SSH, such as
  editors' remote modes or other hosts.
- **Stopping**: flk sends the app SIGTERM, then kills the VM after
  `kill_timeout` (default 10s, at most 300s). A server that has to save
  needs a longer `kill_timeout`, and a wrapper that turns SIGTERM into the
  server's own stop (a console `stop` line, or SIGINT).
- **Restarts**: `[[restart]] policy` is `on-failure` (default), `always`
  or `never`, with `retries` (default 10, backing off up to 60s). The game
  servers set `retries = 1`: their wrapper restarts the server itself, and
  flk's policy is only an outer net, so a crash loop doesn't get
  multiplied.
- **Volumes**: `[[mounts]] source = "data"` needs a volume named `data`
  per machine, and it must exist before the deploy
  (`flk volumes create data --size N`; `flk help deploy`). A new volume is
  an empty filesystem whose root is owned by root, unless it's created with
  `--owner UID[:GID]` (or filled with `--from-dir DIR`). If the app runs as
  uid 1000, the next step should say `--owner 1000:1000`, and the image
  should also create the mount point owned by that user
  (`install -d -o ubuntu -g ubuntu /data`, valheim.flk BUGS #4) for
  volumes made without it. A volume mounted over a directory hides
  whatever the image put there (mounting one over `/home` hides the
  image's home directories), so anything the program needs there has to
  be seeded at first boot.
- **No sharing host folders**: Firecracker has no virtiofs or 9p, so a VM
  can't mount a directory from the host. Data goes in through a volume
  (`flk volumes create --from-dir`, `flk files cp`), the network, or the
  image.
- **Ports**: `[[services]] internal_port` + `protocol` (tcp/udp) +
  `[[services.ports]] port` forwards a host port. Host ports below 1024
  are refused, and so are ports a host process already uses. Forwarding
  skips 127.0.0.1: from the host, use its LAN IP or the machine IP.
  `[http_service] internal_port` is on the machine IP only, unless
  `[flk] http_port = N`. The first `internal_port` is what deploy waits
  for after boot.
- **Health checks**: `[checks.NAME] type = "log"` (a Ruby regex for a
  console line since boot; console lines end in `\r\r`, so don't end it
  with `$`: BUGS.flk.md #2), `"tcp"` (port) or `"http"` (port, path). A deploy
  waits for all of them within `grace_period`, and a machine that fails
  goes back to its previous release. UDP-only servers use a log check.
- **Backups**: `[flk.backup] mode` = `"stop"` (default), `"live"` (copy
  while running, after `pre` hooks that make the app flush and stop
  writing, often console lines like `save-off`), or `"pause"` (freeze the
  VM for the copy: like a power cut). `verify` is a host command that
  checks a backup. `before_deploy = true` backs up during each deploy's
  stop, so a failed release rolls back with its data.
- **Secrets**: `flk secrets set KEY=VALUE` (add `--stage` to skip the
  restart). Never in flk.toml, and never in a file flkforge writes.
- **Releases**: `[flk] keep_releases` (default 2) images are kept for
  `flk releases rollback`. For Steam games, old builds exist only as these
  images.
- **Strict CLI**: unknown options are errors, and `--json` exists on every
  listing. `flk context --json`, `doctor` and `help --all` carry a
  `schema` number: new fields can appear, and renames bump it. Ignore
  fields you don't know.

## 3. Real apps on this host (read-only for you)

| App | Repo | What it shows |
|---|---|---|
| `minecraft-flk` | `~/minecraft.flk` | Java base, `mc-run` wrapper (console `stop`), server files on the volume, `live` backup with console hooks, log + tcp checks, tcp + udp services |
| `vintagestory-flk` | `~/vintagestory.flk` | .NET base, tarball + sha256 pin, mod set with a checksum manifest, `stop` backup, `vs-run` |
| `valheim-flk` | `~/valheim.flk` | steamcmd two-stage build with a build-id pin, `pause` backup, UDP only (log check), `stdin = "null"`, `site.env` for host settings, `bin/vh preflight` |
| `enshrouded-flk` | `~/enshrouded.flk` | Windows server under Wine, a single stage to save disk, a Wine prefix built at image time, SIGINT stop, secrets per group |
| `mqtt-flk` | `~/mqtt.flk` | alpine image, `ENTRYPOINT []`, `restart always`, small VM, `pause` backup |
| `ncm` | `~/ncm.flk` | Python, a git clone pinned to a commit, a patch applied at build time, big data on the volume |
| `opencode-sandbox` | `~/opencode.flk` | dev sandbox: renamed uid 1000 user, sudo, an idle CMD that exits on SIGTERM, `rootfs_extra_mb` |

Every app not named `flkt-*` is real. Use `flkt-forge-*` for your own test
apps. On 2026-10-14 a stray test action stopped `mqtt-flk`; don't repeat it.

## 4. Neighbouring projects

- **flk** (`~/flk`, Ruby, github.com/dayne/d.flk). The tool. Another
  agent leads it. Requests from flkforge go in this repo's `BUGS.flk.md`.
- **lazyflk** (`~/d.lazyflk`, Go + Charm). The terminal UI for running
  apps, built only on flk's CLI and JSON. flkforge borrows its stack and
  conventions. Later the two could link up (lazyflk's "new app" action
  could start flkforge), but don't edit lazyflk; note ideas in `TODO.md`.
- **gamebus** (`~/m.gamebus`, Go). Reads `[gamebus] {json}` event lines
  from game consoles via `flk logs` and publishes them to MQTT and
  Discord. Its `SPEC.md` is the event contract.
  `adapters/gamebus-event.sh` is the shell function every game wrapper
  copies. flkforge's run-wrapper snippet can include it as an option.
  `PREFLIGHT.md` there is the house pattern for read-only check commands,
  which `flkforge check` should follow.
- **mud** (`~/mud`): the host's own repo (disks, backups, cron). Host work
  goes there, not into flkforge.

## 5. House style flkforge's output should match

Look at the real `flk.toml` files. They're commented for a human: every
non-obvious value says *why* ("Give the server time to save before flk
kills the VM."). Sections come in a fairly steady order: `app`,
`kill_timeout`, `[build]`, `[env]`, `[[restart]]`, `[flk]`,
`[flk.backup]`, `[checks.*]`, `[[vm]]`, `[[mounts]]`, `[[services]]`.
Dockerfiles open with a comment block saying what's in the image and what
lives on the volume instead. Generated files should look as if the admin
wrote them carefully.
