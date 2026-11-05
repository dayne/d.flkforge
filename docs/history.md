# History

Finished milestones and decisions, newest last, with absolute dates.

## 2026-11-04: design

Designed in a flk session with Dayne, before any code. Decided:
- Name **flkforge**; folder `~/d.flkforge` ("d." is the repo prefix, as
  with d.flk and d.lazyflk; the tool, binary and docs say plain flkforge).
- Go + Charm, lazyflk's stack.
- It writes Dockerfile + flk.toml + a run wrapper when needed, plus a
  recipe (`flkforge.toml`) so you can remix later and the hub can share
  recipes.
- Local git repo, no remote, for now.
- v1 is the wizard and the CLI over snippets; the hub comes later, but
  ids, versions, locks and the Source interface are built for it from
  the start.
- Go module path `github.com/dayne/d.flkforge`, matching lazyflk, even
  while the repo has no remote (Dayne, 2026-11-04).

## 2026-11-04: planning review (flkforge's lead, with Dayne)

Reviewed the design before any code. Decided:
- Agents and the CLI come first, the wizard last (PLAN M4).
- Vertical slices: M0 ends with `flkt-forge-hello` deployed, and M1
  with a supervised app deployed (a fake server, no Steam download),
  before the catalog grows.
- The real apps are reference material, not migration targets: compare
  `parsed`/`machine_config` where it's cheap and explain the differences
  in notes. Import stays low priority.
- Run wrapper: one versioned `flk-run` engine plus hooks the admin owns
  (SPEC 6.6), chosen after reading all five `*-run` scripts.
- flk.toml fragments are structured records (SPEC 6.2), so comments
  survive merging.
- The lock holds only the flkforge version while every snippet is built
  in.
- Forms use `charm.land/huh/v2` (v2.0.3 exists).
- flkforge may message the flk agent's session directly; `BUGS.flk.md`
  stays the record.

## 2026-11-04: M0 done (skeleton and the hello slice)

Built: the command table (`new --recipe`, `render`, `check`, `snippets`,
`help [--all] [--json]`, `version`), the snippet format and its loader,
sources (built in, user dir, app dir), the recipe, the pure renderer,
`internal/flktoml` (a comment-keeping parser and writer for flk.toml's
TOML subset; the real apps' files round-trip with every comment), drift
detection with `.flkforge-new`, the preflight `check`, the four-part
report in text and JSON. Snippets: base-alpine, packages, start-script,
env, http-service, vm, volume, check-http.

Acceptance (PLAN M0):
- `flkforge new DIR --recipe testdata/hello.toml --yes` matches the
  golden files (`TestGoldenHello`), and flk reads the rendered flk.toml
  with the same `parsed` and `machine_config` as `~/flk/examples/hello`
  and no warnings (`TestHelloMatchesFlkExample`). Dockerfile differences
  are in `testdata/hello.notes.md`.
- Deployed as `flkt-forge-hello` by running the printed steps exactly
  (`flk launch --no-deploy`, `flk volumes create data --size 1`,
  `flk deploy`). With `core/check-http`, the VM booted and served, but
  **flk deploy crashed** on the http check: a flk bug
  (`Flk::Net::HTTP`), filed as BUGS.flk.md #1 and sent to the flk
  session; `core/check-http` now reports it under "Gaps in flk". Without
  the check (the recipe edited by hand, then `flkforge render`), the
  deploy went through: v4 healthy, the page served, the boot counter on
  the volume kept across four deploys, and `flkforge check` 9 ok, 0 warn,
  0 FAIL. Destroyed with `flk apps destroy flkt-forge-hello --yes`.

Decided along the way: flk.toml fragments stay readable TOML text,
parsed into records (SPEC 6.2, correcting the planning review's
"records written by hand"); managed and seed files (SPEC 6.3); a render
is all or nothing (6.5); `new --app` for test names.

## 2026-11-04: M1 done (the run engine and one supervised app)

Built: `flk-run` (SPEC 6.6) with prestart/watch/poststop hooks and
`--exec`, its test suite against `testdata/fakeserver` (15 tests, run 15
times in a row without a flake), and the snippets base-ubuntu,
run-wrapper, user-1000, service, check-log, backup-pause, backup-stop,
before-deploy, restart-outer-net, stdin-null, kill-timeout and steamcmd
(render-tested, not deployed). Rules: kill-timeout-max/-margin,
wrapper-restart, before-deploy-check, udp-only-check, log-check-dollar,
busybox-user, stdin-null-console. `until_edited` steps, seed files in
subdirectories.

Found while building: bash starts background jobs with SIGINT ignored
(fixed in flk-run before exec); two stop races the old wrappers share
(fixed and tested); `flkforge new` failed on files in subdirectories
(fixed).

Acceptance (PLAN M1), app `flkt-forge-run` from testdata/fake-game.toml,
following the printed steps:
- First deploy failed its log check: `^fake: Server ready$` never
  matches, because flk matches the raw console where lines end in
  `\r\r`. A flk bug, BUGS.flk.md #2 (sent to the flk session); rule
  `log-check-dollar` now warns. Recipe fixed by hand, `flkforge render`,
  deploy v2 healthy in 4s (before_deploy took a backup).
- The server ran as ubuntu under flk-run; `prestart` (filled in) checked
  /data.
- `flk exec kill -KILL` of the server: flk-run logged status 137,
  restarted it after 5s (prestart again), online.
- `flk machine stop`: SIGTERM became `stop` on the console, the server
  saved, flk-run exited 0, in 1.3s.
- `flk volumes backup` (pause, 0.0s) and `--mode stop` (down 1.2s): both
  clean, and `debugfs` showed world.txt in each, from the latest clean stop.
- `flkforge check`: 11 ok, 0 warn, 0 FAIL. Destroyed with
  `flk apps destroy flkt-forge-run --yes --with-backups`. The host's
  off-machine copies in /data/backup/flk/flkt-forge-run (2.3 MB) are never
  deleted by flk: left for Dayne.
- Reference: testdata/real/valheim.toml renders a flk.toml that flk reads
  exactly like valheim-flk's (parsed and machine_config, apart from the
  engine's RUN_* env); differences in valheim.notes.md.

## 2026-11-04: M2 done (the agent path)

Built: `templates` and `new --template T --set USE.PARAM=VALUE` (blank,
web-service, daemon, steam-game; templates leave app-specific params
open, and every open one is named with its fix at once); `add`, `rm`,
`set [--unset]` (values read by the param's type; a failed edit changes
nothing); `snippets show`; `explain` for snippets, rules, limits
(`_limits.toml`: host-folder, lan-address, gpu, low-port, shell, systemd,
docker-in-vm) and flk.toml keys (flk's own reference, read live from
`flk help --all`), with a fallback search of the hints; rules as a
catalog plus no-shell, live-backup-console, mount-owner, mount-hides,
secret-in-env, private-key, guest-init, huge-pages; `check` adds host
ports (flk's min_port, ports other apps forward), the deployed release
and whether flk.toml changed since, and unset secrets; snippets
base-image, entrypoint-clear, restart-always, check-tcp, secret, and
vm huge_pages; `[seeded]` hashes, so a kept seed file whose recipe
version moved on gets a note, and an untouched stub is a check warning
and the Next step.

Acceptance (PLAN M2):
- A fresh agent, told only to build a Redis daemon as `flkt-forge-agent`
  and to learn flkforge from its own help, `snippets`, `explain` and
  `templates` (no repo files), did it with `templates`, `help new`,
  `snippets show`, one `new --template daemon --set ...`, and an edit of
  start.sh: `check` 0 FAIL on the first try. Its critique led to: the
  stub warning and Next step (check had passed a `sleep infinity`
  start.sh), the explain search, templates showing their params, and two
  wording fixes. Not done: linking a program's own config (Redis's port,
  dir) to the snippets' params; app-specific snippets are for M3.
- Deployed by its printed steps: Redis answered PONG and SET/GET over
  the forwarded port; `check` read release v1 with its machine started,
  and after `flkforge set vm memory=512mb` warned that flk.toml changed
  since v1. Destroyed.
- Drift (hand edit, refused render, `.flkforge-new`, check naming it):
  tested since M0 (`TestDrift`).
- `check` also caught a real conflict during development: a trial broker
  on host port 1883, which mqtt-flk forwards.
