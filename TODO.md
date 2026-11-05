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

## Hub (later, design only)

- Transport: a git repo of snippets (simple, reviewable) or an HTTP index.
- Trust: signing, review, pinning by hash; what `add` shows before using
  a non-local snippet.
- Publishing: `flkforge hub publish` from a recipe, and what gets
  stripped (site-specific values, secret names?).
- Whether lazyflk browses the hub too.

## Catalog additions (Dayne, 2026-11-04: noted, not scheduled)

Asked for: an Arch base, a Go service, a Rails app, a Lua app (OpenResty),
and Node-RED. Decide when to build them (a catalog milestone before or
after M4); each gets a template golden test, and at least one a deploy.
What's known so far:

- **base-arch**: `docker.io/library/archlinux:base` has pacman 7 and
  util-linux's setpriv (/usr/sbin/setpriv, so flk can switch users), and
  no uid-1000 user (core/user-1000 makes one with useradd). Needs a new
  family, `pacman`, in the renderer and the snippet validator: one line
  like `pacman -Syu --noconfirm --needed PKGS && pacman -Scc --noconfirm`.
  Arch is rolling: pin the image by its date tag (base-YYYYMMDD.0.N), and
  say in the hint that packages float with each build.
- **go-service**: a build stage `docker.io/library/golang:1.N` running
  `go build` with CGO_ENABLED=0 and -trimpath, then the binary on a small
  runtime: Debian slim or Alpine (a shell: flk's shell init), or
  distroless/static or scratch (no shell: rule no-shell, flk's static
  agent init, check flk doctor). Probably a `go-build` snippet (source in
  the app dir or core/git-pinned, module cache as a build cache) plus a
  template with http-service and check-http.
- **rails-app**: a Ruby base (`docker.io/library/ruby:3.N-slim`,
  Debian), bundler with `BUNDLE_DEPLOYMENT=1` and `BUNDLE_WITHOUT=development:test`,
  assets precompiled at build time (SECRET_KEY_BASE_DUMMY=1), Puma as the
  app (flk's SIGTERM is Puma's graceful stop: no wrapper needed), SQLite
  in /rails/storage on a volume, `bin/rails db:prepare` before the server
  (a start script, or flk-run --exec with a prestart hook),
  RAILS_MASTER_KEY as a core/secret, check-http on /up (Rails 7.1+). A
  Postgres variant would be a second app (its own flk app plus
  internal-hosts) — decide later.
- **openresty-app**: `docker.io/openresty/openresty:N-alpine` (or the
  Debian flavour) via core/base-image or its own base; nginx in the
  foreground (`daemon off;`), Lua code and nginx.conf copied in
  (core/copy-files), opm/luarocks for modules, logs to stdout/stderr so
  flk logs has them, check-http. nginx stops gracefully on SIGQUIT, not
  SIGTERM (SIGTERM is a fast stop): a stop setting worth explaining.
- **node-red**: Node-RED (`docker.io/nodered/node-red:N`, Node.js on
  Alpine or Debian, running as node-red uid 1000) with its flows and
  settings in /data on a volume, port 1880 over http, check-http.
  Relevant here: it pairs with mqtt-flk (internal-hosts, or the host's
  1883), so an MQTT-connected template could point at the house broker.
  Its admin UI needs credentials (adminAuth in settings.js with a hashed
  password: a secret, never in flk.toml). Check its image's entrypoint
  and how it handles SIGTERM before writing the stop hints.

## From M4

- Wizard: the mouse (click a step, a field, a preview tab).
- Wizard: a step's snippets could show their own short summary under the
  header at the summary hint level; at 80x24 the form has room for ~8 rows.
- lazyflk's "new app…" action could run `flkforge new` (propose it to the
  lazyflk session; PLAN, Later).

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
