---
updated: 2026-11-04
status: draft, nothing built yet
---

# flkforge spec

## 1. Who it's for

An admin who can write a Dockerfile and a flk.toml by hand with the docs
open, and would rather not have to remember everything every time. They
want three things: a fast start from patterns that are known to work, to
see what each choice does under the hood, and files they fully own
afterwards. The second user is an AI agent, which drives the same
features without a terminal, through a recipe file and `--json`.

## 2. Principles

1. **Teach while building.** Each choice shows what it adds (the exact
   Dockerfile lines and flk.toml keys), what flk does with them (at build,
   boot, stop, backup or deploy time), the gotchas, and where to read more
   (`flk help ...`). Hints can be turned down, but never hidden away
   entirely.
2. **The output is plain and owned by the user.** Commented files that
   look hand-written (house style: `docs/flk-primer.md` section 5). Nothing
   at runtime depends on flkforge. Deleting `flkforge.toml` leaves a
   normal flk app.
3. **flk is the authority.** Validate with `flk config show --json`. Read
   flk's capabilities from `flk help --all --json`. Never read flk's state
   directory. Never deploy, and never run root steps: print them as next
   steps.
4. **One model, two front ends.** The wizard and the CLI both edit a
   recipe and call the same renderer. Anything the wizard can do, a flag
   or a recipe can do too.
5. **Snippets are data.** A snippet is a directory of TOML and templates,
   not Go code, so new ones (user ones, and later hub ones) need no
   rebuild.
6. **Deterministic.** The same recipe and the same snippet versions give
   byte-identical files.

## 3. Concepts

- **Snippet**: one reusable piece: a base image, a way to fetch software,
  a run wrapper, a volume, a service port, a check, a backup mode. It has
  typed parameters and adds fragments to the Dockerfile and flk.toml, and
  sometimes extra files. It carries hints, the next steps it implies, and
  its relations to other snippets (`requires`, `conflicts`, `suggests`).
- **Template**: a starting set of snippets with defaults ("Steam game
  server", "Java server", "web service", "dev sandbox", "blank"). A
  template is just a recipe stored in the catalog.
- **Recipe**: `flkforge.toml` in the app directory: the app name, the
  chosen snippets and their parameter values, and a lock of the snippet
  versions and content hashes. It's what `edit`, `render` and `check` read,
  and what the hub will share later.
- **Source**: where snippets come from. In v1: embedded (built in),
  `~/.config/flkforge/snippets/`, and `./.flkforge/snippets/` in the app
  directory, in that order, with later sources overriding earlier ones by
  id. The hub becomes another source (section 10).
- **Rule**: a check across snippets that turns into a hint, a warning or
  an error (section 6.4).

## 4. Snippet format

One directory per snippet: `snippets/<namespace>/<name>/`. Built-in ones
use namespace `core`, so ids look like `core/steamcmd`, and the namespace
can be left off when the name is unique.

    snippets/core/steamcmd/
      snippet.toml        metadata, params, fragments, hints
      files/              templates copied into the app (optional)
      README.md           the long explanation (optional; `flkforge explain`)

`snippet.toml`, sketched (finalize it in M0 and update this section):

```toml
id = "core/steamcmd"
version = "1.0.0"
title = "Steam dedicated server via steamcmd"
category = "fetch"            # base | fetch | wrapper | user | data | network | check | backup | restart | vm | env | misc
summary = "Download a Steam server at build time, pinned to a build id."
tags = ["game", "steam"]
requires = ["category:base"]  # needs some base snippet
conflicts = []
suggests = ["core/run-wrapper", "core/backup-pause"]
source = "~/valheim.flk/Dockerfile"   # where the pattern was learned (docs only)

[params.app_id]
type = "int"
prompt = "Steam app id of the dedicated server"
hint = "The server's own app id, not the game's: Valheim's is 896660. Search steamdb.info for 'dedicated server'."
required = true

[params.platform]
type = "enum"
values = ["linux", "windows"]
default = "linux"
hint = "windows needs the core/wine base: the server runs under Wine."

[params.build_id]
type = "string"
default = "latest"
hint = "Pins the build in [build] args. Steam serves only its current build, so a pin that Steam has moved past fails the build on purpose."

# Fragments. Dockerfile ones name a slot (section 6.1); flk.toml ones are
# TOML text merged by section 6.2. Both are Go text/template with .P (params),
# .App (name, user, uid, family...) and helper funcs.
[[dockerfile]]
stage = "steam"               # its own build stage; omit for the final stage
slot = "fetch"
text = '''
ARG STEAM_BUILDID=latest
RUN for try in 1 2 3; do ... done
'''

# flk.toml fragments are records, not raw TOML text (section 6.2): a
# table path, a key, a value (templated, then parsed as a TOML value) and
# the comment that goes above it.
[[flktoml]]
table = "build.args"
key = "STEAM_BUILDID"
value = '"{{.P.build_id}}"'
comment = "The Steam build this release runs. Bump it to upgrade."

[hints]
what = "Downloads the server with steamcmd in a build stage and copies it into the runtime image."
under_the_hood = """
flk deploy runs podman build; this stage runs steamcmd anonymously and
checks that the build it got matches STEAM_BUILDID. The server lives in the
image, so every release is one exact Steam build and `flk releases rollback`
is how you get an older one back (keep_releases).
"""
gotchas = ["Steam drops old builds: keep_releases is your only way back.", "..."]
docs = ["flk help --all  (Config: [build] args)", "flk help releases"]

[[next]]                      # next steps this snippet implies, after render
text = "Pin the current build: look it up on api.steamcmd.net and set build_id"
when = '{{eq .P.build_id "latest"}}'
```

Params types: `string`, `int`, `bool`, `enum`, `list`, `port`, `size`
(memory/disk such as `"6gb"`), `duration` (`"5m"`), `regex`, and `secret`
(only the name is recorded; the value never is). Each param has `prompt`,
`hint`, `default`, `required` and optional `validate` (a regex, or a range
for ints).

## 5. Recipe format (`flkforge.toml`)

```toml
# Written by flkforge. Edit with `flkforge edit`, or by hand, then `flkforge render`.
flkforge = "0.1.0"
app = "valheim-flk"
template = "core/steam-game"   # where it started (informational)

[[use]]
snippet = "core/base-ubuntu"
params = { version = "24.04" }

[[use]]
snippet = "core/steamcmd"
params = { app_id = 896660, platform = "linux", build_id = "25909431" }

[[use]]
snippet = "core/run-wrapper"
params = { name = "vh-run", stop = "sigint", ready = "Game server connected", gamebus = true }

[[use]]
snippet = "core/service"
id = "game"                    # a snippet used more than once needs an id
params = { port = 2456, protocol = "udp" }

# ...

[lock]                         # written by render; don't edit
flkforge = "0.1.0"             # v1: every snippet is built in, so the binary's version is the lock
# with non-embedded sources (user dirs, later the hub), per snippet:
# "core/steamcmd" = { version = "1.0.0", sha256 = "..." }

[render]                       # written by render: hashes of the files it wrote (drift detection)
Dockerfile = "sha256:..."
"flk.toml" = "sha256:..."
```

## 6. Rendering

`internal/render` is a pure function: (recipe, resolved snippets) →
(files, warnings, next steps). No I/O.

### 6.1 Dockerfile

Build stages that snippets declare come first, in recipe order. Then the
final stage, assembled from slots in a fixed order:

    header     comment block: what's in the image, what's on volumes (generated from the snippets used)
    from       exactly one base snippet
    packages   all packages merged into one install line for the base's family
               (apt: debian/ubuntu; apk: alpine; dnf: fedora), cleaned up after
    env        ENV lines
    fetch      downloads, steamcmd, git clone, tarball + sha256
    install    build and setup steps
    copy       COPY of generated files (run wrapper) and user files
    dirs       mount points created with the right owner
    user       USER / HOME / WORKDIR
    entry      ENTRYPOINT [] and CMD

The base snippet declares facts the others need: `family`, the name of
the uid-1000 user if the image has one (`ubuntu` in Ubuntu-based images,
which eclipse-temurin and the dotnet runtime images are), and whether its
ENTRYPOINT must be cleared (`eclipse-mosquitto` needs `ENTRYPOINT []`).
Base images are always full names (`docker.io/library/...`).

Put things that change often (the run wrapper) late, so editing them
doesn't invalidate the expensive layers. Say so in a hint.

### 6.2 flk.toml

Fragments are structured records, not TOML text: `table` (a dotted path,
or `[[name]]` with an entry key for arrays of tables), `key`, `value`
(a TOML value after templating), and an optional `comment`. A record
with no key and only a comment gives a table its leading comment. Merging
works on records: tables merge, entries of arrays of tables
(`[[services]]`, `[[mounts]]`) append, and a scalar set twice is a
conflict unless the snippet declares a merge rule (`max` for
`kill_timeout`, for example). Records were chosen (2026-11-04) over
merging TOML text because a TOML parser drops comments, and keeping the
comments is the point. Output comes from flkforge's own writer, not
a generic TOML encoder, so it keeps the comments from the fragments and
uses the house section order (primer section 5). Any keys outside the
known order go at the end. `[env]` keeps the order of the snippets that
set it, with a comment line naming the snippet that owns each group.

### 6.3 Extra files

`files/` templates render into the app directory: the run wrapper (mode
755), a `.dockerignore`, `site.env.example`. A file that already exists
and wasn't written by flkforge is never overwritten (section 6.5).

### 6.4 Rules

These run after the merge. Each has an id, a level (`error` stops the
render; `warn` and `hint` don't) and a fix. The first set comes from the
real apps' history:

- `kill_timeout` must be above the wrapper's `STOP_TIMEOUT` with some
  margin, and at most 300.
- `[flk.backup] mode = "live"` with console hooks needs `stdin = "console"`.
- A non-root app user plus a volume needs its mount point created in the
  image with that owner.
- A UDP-only app has no tcp check to wait on: suggest a log check.
- `before_deploy = true` is only useful with at least one check.
- Host ports below 1024 are refused by flk by default (the fix is a root
  step: say so, don't do it).
- JVM heap (or the app's own memory setting) must fit inside `[[vm]]
  memory`, with room to spare.
- `huge_pages` needs memory in whole 2MB units, plus a host sysctl.
- A wrapper with its own restart loop goes with `[[restart]] retries = 1`.
- No secret value ever lands in a written file.
- Short image names fail on podman without search registries.
- A base without `/bin/sh` (scratch, distroless) boots only on flk's
  static init, which needs flk's agent (Go on the host): warn, and say so.
- A non-root user on a base without `setpriv` or `chroot --userspec`
  (busybox/Alpine) silently runs as root: add the package.
- Nothing in the image may expect systemd, or manage the network,
  hostname, `/etc/hosts` or `/etc/resolv.conf` (init writes them).
- A volume mounted over a path the image fills (e.g. `/home`, or a
  `curl | bash` installer that writes to `~`) hides that content: install
  system-wide, or seed the volume at first boot.
- No private key ever goes into the build context (public keys only), and
  tokens are secrets, not `[env]`.

### 6.5 Drift and re-rendering

`render` records each file's hash in the recipe. On the next render, a file
whose hash no longer matches was edited by hand. flkforge then doesn't
overwrite it: it writes `FILE.flkforge-new`, shows a diff, and exits 1,
unless `--force` is given. `check` reports drift without writing anything.

### 6.6 The run engine (`flk-run`) and its hooks

Decided 2026-11-04, after reading mc-run, vs-run, vh-run, en-run and
mqtt-run side by side. The four game wrappers share, almost line for
line: flk's SIGTERM turned into the server's own stop (a console line
through a FIFO held open on fd 3, or a signal), escalation after
`STOP_TIMEOUT` (SIGTERM, or SIGKILL plus a cleanup command for Wine), the
restart loop (`RESTART_DELAY`, `MAX_RESTARTS`, `RESTART_WINDOW`, give up
and the VM powers off), `finish`, the `server.*` gamebus events and the
`gamebus_event` function. What they don't share is specific to each game:
log lines turned into player events, checks before start, and config
written from env (vh's access lists, en's JSON with bans kept).

So the wrapper isn't generated from a template. It's **one versioned
bash script, `flk-run`**, copied as is from the snippet's `files/` (it
carries a `# flk-run vN` line, like `gamebus-event.sh v1`), configured
by `[env]`:

    RUN_NAME        log prefix and event source (default: the app name)
    RUN_STOP        console:LINE | signal:INT|TERM|...   how to stop gracefully
    RUN_ESCALATE    TERM | KILL, plus an optional RUN_ESCALATE_CMD (wineserver -k)
    RUN_READY       regex; the first match emits server.online
    RUN_FILTER      regex; matching lines are dropped from the log
    STOP_TIMEOUT RESTART_DELAY MAX_RESTARTS RESTART_WINDOW   as today
    RUN_EVENTS      1 = print gamebus events (default 1)

and the command after `--` (`CMD ["flk-run", "--", "java", ...]`).
The app-specific parts are optional hook files next to it, which
flkforge writes as commented stubs and the admin owns:

- `prestart`: runs before each start, and a non-zero exit is fatal
  (checks, writing config from env). It runs before every restart, not
  just the first start, because en-run rewrites its config each time.
- `watch`: sourced by the engine, it defines `on_line LINE`, which is
  called for each output line after filtering, along with `event` and
  `console TEXT` (writes to the server console, for vs-style polling).
- `poststop`: runs after the server exits (en-run keeps its bans here).

A daemon that needs no supervision (mqtt-run: setup, then `exec`) uses
`flk-run --exec`, which runs `prestart` and then `exec`s the command, or
just a `prestart` and a plain CMD. The engine needs bash; on Alpine
bases the `run-wrapper` snippet adds the package, and the `--exec`
form works in POSIX sh.

Fixing the engine in its snippet reaches every app on its next render.
Drift detection covers the engine, not the hooks: the hooks belong to
the admin from the moment they're written.

## 7. Validation and `check`

`flkforge check [DIR]` is read-only and follows the house preflight
pattern (`~/m.gamebus/PREFLIGHT.md`): grouped `ok` / `warn` / `FAIL` lines,
a `->` fix line for each problem, exit 1 on any FAIL, and a final count
with the next step. It checks the following, in order, stopping at the
first broken link in each chain:

1. tools: `flk` on PATH and answering (`flk version`), and podman or docker;
   it reads `flk help --all --json` and warns if its `schema` is newer than
   the one flkforge knows.
2. recipe: it parses, every snippet resolves, the lock matches, and the
   params are valid.
3. files: they're present and match the recipe (no drift); a render from
   the recipe is identical.
4. flk: `flk config show --json` in DIR gives no `warnings`, and its
   `machine_config` agrees with the recipe (ports, mounts, checks).
5. host hints (warn only): the app name is already registered
   (`flk apps list --json`), a host port is already taken (`flk ports`),
   a volume for each mount exists or needs creating.

`render` runs steps 2 to 4 itself, and fails on a flk warning unless
`--allow-warnings` is given.

## 8. The wizard

`flkforge new [DIR]` with a terminal; `flkforge edit [DIR]` opens an
existing recipe. Layout: steps down the left, the current form in the
middle, and on the right a **live preview** of the Dockerfile / flk.toml /
wrapper (tab between them). Lines from the snippet being edited are
highlighted, and a **hint panel** below shows what / under the hood /
gotchas / docs for the focused field. The preview re-renders on every
change, and rule results show inline.

Steps (each one skippable when its template already decided it):

1. **Start**: a template, or open a hub recipe later. One line on what
   each template is for.
2. **Name**: the app name, checked against flk's rules and the registered
   apps. Suggest a `flkt-` prefix for experiments. The default comes from
   the directory, following the house convention: `foo.flk` becomes
   `foo-flk` (app names can't contain dots). The machine names are flk's
   to generate; don't ask for them.
3. **Base**: the runtime image (Ubuntu, Debian, Alpine, Java/Temurin,
   .NET, Python, steamcmd+Wine, or a custom image with its family and
   user). Hint: full names, image size, what's already in it.
4. **Software**: how the program gets in: packages, a pinned download
   with sha256, steamcmd with a pinned build, a pinned git clone (plus a
   patch), files from a `dist/` folder, or none (it lives on the volume:
   the Minecraft pattern).
5. **Process**: the command, and whether to use a run wrapper: how to stop
   gracefully (a console line, SIGINT, SIGTERM), the ready line, a restart
   loop, gamebus events. Hint: flk's SIGTERM → `kill_timeout` → kill, and
   why the wrapper loop goes with `retries = 1`.
6. **User and data**: which user runs it, volumes and their mount points,
   and what lives in the image versus on the volume (the most important
   decision; explain it). Ask it directly: *what must survive a deploy?*
   The root filesystem is rebuilt every time. For a server, that's the
   world and its config. For a dev box, it's home directories, tool logins
   (such as `~/.claude`) and sshd host keys. Host keys generated in the
   image's filesystem change on every deploy ("host key changed"), so they
   belong on the volume, created at first boot.
7. **Network**: services (tcp/udp ports), http_service, `bind`. Hints:
   below 1024, 127.0.0.1, machine IPs.
8. **Health**: log, tcp and http checks with grace periods. Suggest a log
   check from the wrapper's ready line.
9. **Backups**: stop / live / pause, with what each costs the app, verify
   hooks, and `before_deploy`.
10. **Resources**: cpus, memory, `rootfs_extra_mb`, `keep_releases`.
11. **Env and secrets**: `[env]` values, and the names of secrets (values
    are never stored; the next steps list `flk secrets set NAME=... --stage`).
12. **Review**: the full files, the rule results, `flk config show`'s
    verdict, then write.
13. **Report and next steps**: four short parts, the same in the wizard
    and in `new`/`render` output (and in `--json`):
    - **Written**: the files, and the snippets behind each one.
    - **Not in effect yet**: what the files ask for but won't happen
      until something else does, and why (a volume not created yet, a
      secret not set, a host port below 1024, a host-wide `bind`).
    - **For you to run**: the flk commands in order, ready to copy, then
      any root steps on their own, marked as root and never run by
      flkforge.
    - **Gaps in flk**: anything asked for that flk can't do yet, with
      the nearest workaround, and an entry for `BUGS.flk.md` if it's new.

### 8.1 Asks that aren't snippets

Some requests can't be met by image or config at all. flkforge says so
before it generates anything, rather than writing files that look right
and do nothing. Each one falls in one of four groups:
**flk supports it** (a snippet), **root on the host**, **needs a flk
change**, or **not possible on Firecracker**. Keep a small catalog of
these (`snippets/core/_limits.toml`, shown by `flkforge explain`), each
with its group, why, and the nearest alternative. Starting entries:

- Share a host folder: not possible (no virtiofs/9p). Alternatives:
  `flk volumes create --from-dir`, `flk files cp`, or sshfs over the
  network.
- A LAN address of its own, DHCP, a fixed MAC: needs a flk change (M5
  "managed extra addresses" in `~/flk/PLAN.md`) and host networking.
  Today: forwarded host ports, `[flk] bind` to a second host address.
- GPU: not possible (no passthrough; see `~/ncm.flk`).
- Host ports below 1024: root on the host (`FLK_MIN_PORT` in flk's
  `setup-host.sh`).
- A shell in the VM: flk already does it (`flk ssh console`,
  `flk exec`). sshd is a snippet only for real SSH clients.

Before adding an entry, check `flk help --all` and `~/flk/PLAN.md`,
because flk changes. The `version` and `schema` from
`flk help --all --json` say which flk the entry was checked against.

Keys follow lazyflk where they overlap (`?` help, `q`/`esc`, tab). `h`
cycles the hint detail level (summary → under the hood → off), and
`--hints=summary|full|off` sets the starting level. The mouse is optional.
Works at 80 columns: the preview drops below the form when narrow.

## 9. CLI

    flkforge new [DIR] [--template T] [--recipe FILE] [--yes] [--force]
    flkforge edit [DIR]
    flkforge render [DIR] [--force] [--allow-warnings] [--json]
    flkforge check [DIR] [--json]
    flkforge add SNIPPET [--id ID] [--set K=V ...] [DIR]      add to a recipe, then render
    flkforge rm ID [DIR]
    flkforge set ID K=V ... [DIR]
    flkforge snippets [list] [--category C] [--json]
    flkforge snippets show ID [--json]
    flkforge templates [--json]
    flkforge explain SNIPPET|RULE|KEY        hints in full; KEY = a flk.toml key such as kill_timeout
    flkforge help [COMMAND] | help --all [--json]
    flkforge version

`new --recipe FILE --yes` builds a whole app without a terminal (the
agent path). With no TTY, `new` and `edit` refuse and name this form.
Commands that change something end with a `Next:` line, as flk's do.
Exit codes: 0 ok, 1 failed or needs attention, 2 bad usage. Copy flk's
approach of keeping commands and options in one table that drives parsing
and help (`~/flk/lib/flk/commands.rb`).

## 10. Hub readiness (built later, designed for now)

- Snippet ids are `namespace/name`. `core` is reserved for built-ins, and
  hub namespaces will be owners (`dayne/valheim`).
- Versions are semver. Recipes lock version + content sha256 (per-snippet
  entries are written only for snippets outside the binary; until then the
  flkforge version is the lock), so a recipe
  from the hub renders the same anywhere.
- `internal/source` is an interface (`List`, `Get(id, version)`) from the
  start. Embedded and directory sources implement it in v1; the hub is a
  third implementation (an HTTP index or a git repo, to be decided).
- Templates and whole-app recipes are shareable units, alongside snippets.
- A snippet's `source` field and its hints are what a hub page shows, so
  write them well now.
- Trust: hub snippets run as build steps and wrappers in the user's VMs,
  so `add` from a non-local source shows the snippet's full text and asks
  first. The hub itself, and signing, are open (`TODO.md`).

## 11. Not in v1

The hub; running flk commands that change things (launch, deploy,
volumes, secrets); editing existing hand-written apps that have no
recipe (import is in `TODO.md`); multi-machine apps beyond what one
recipe's `[[vm]]` gives; fly.io deployment.

## 12. Changes

- 2026-11-04: first draft (design session in `~/flk`, before any code).
- 2026-11-04: from a review of another agent's notes on a dev-box request
  ("genma"), checked against flk: guest rules in 6.4, the persistence
  question in step 6, the four-part report in step 13, asks that aren't
  snippets (8.1), and default names from the directory.
- 2026-11-04: planning review with Dayne as flkforge's lead: flk.toml
  fragments are structured records (6.2); the run wrapper is one
  versioned engine, `flk-run`, plus hooks the admin owns (6.6); the lock
  holds only the flkforge version while every snippet is built in (5,
  10). The milestones were reordered in PLAN.md: CLI and agents first,
  a vertical slice first, the wizard last.
