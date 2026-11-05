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

## 2026-11-04: M3 done (the rest of the catalog)

Built: bases debian, temurin, dotnet, python, wine; download-sha256,
checksum-manifest, git-pinned, pip, on-volume, steamcmd `single_stage`;
idle-cmd, user-rename, volume-links, home-volume (flk-seed-home),
site-env; bind, internal-hosts; backup-live-console, backup-verify;
rootfs-extra, keep-releases, timezone; restart, copy-files, image-env;
the escape hatches flk-toml and dockerfile-lines. run-wrapper
`kill_margin`, restart-always `retries`. Rule jvm-heap. Templates
java-server, dotnet-server, steam-game-windows, python-workbench,
dev-sandbox. Next steps can be `unless_found`. 52 snippets, 9 templates.
`sshd` deferred (TODO.md).

Acceptance (PLAN M3):
- Every template renders with no rule findings, passes flk config show
  with no warnings, and has golden files (TestTemplatesGolden).
- Reference recipes for all seven real apps: flk reads each rendered
  flk.toml exactly like the real app's (parsed and machine_config, apart
  from flk-run's RUN_* env), TestReferenceApps; the Dockerfile
  differences are explained in testdata/real/*.notes.md. Checking them
  found that the Temurin and .NET images already ship tzdata (so
  core/timezone's package is a no-op there), and that opencode's
  Dockerfile uses a short image name (only builds where podman has
  search registries).
- web-service deployed as flkt-forge-web from the template (a package
  added, start.sh filled in): v1 healthy, served the page from the
  volume, check 10 ok, 0 warn, 0 FAIL. Destroyed.
- Beyond the plan, dev-sandbox as flkt-forge-dev, to test home-volume's
  claim: first boot seeded /home/dev from the image, the user was dev
  with working sudo, and after a second deploy ~/.marker and the
  workspace file were still there, not seeded again. Destroyed. It
  turned up a false "run flk-seed-home" step (the template already
  does), fixed with unless_found.

## 2026-11-04: flk's r1-reliability on main

Dayne merged it; checked in ~/flk that 4294558, a8461a0 and c887f14 are
on main and that the installed flk runs that checkout. Removed the
workarounds: check-http's "Gaps in flk" line, the log-check-dollar rule
and its hints, the primer's notes. web-service now uses check-http.
Confirmed: testdata/hello-checked.toml (hello plus an http check)
deployed as flkt-forge-hello by the printed steps, and the deploy waited
on the http check and passed (the machine logged 200s for it), where in
M0 flk crashed. Destroyed.

## 2026-11-05: M4 done (the wizard)

Built `internal/wizard` (SPEC 8.2): steps, forms with live re-rendering,
a preview that marks the focused snippet's lines and follows it to its
file, placeholders while params are missing, a picker where a
conflicting snippet replaces the old one, review with flk's verdict and
the app-name check, writing through forge with the four-part report,
help, three hint levels, and layouts from 60x16 up. `flkforge new` in a
terminal and `flkforge edit`. Hand-built widgets instead of huh (decided
here; SPEC 8.2). Tests: golden snapshots of each screen at 80x24 and
160x48, behaviour tests (quitting, focus, bad values, enums, base change,
removal, help, multi-line params, taken names, every screen fits), and
the acceptance flow driven by key presses.

Acceptance (PLAN M4):
- In the tests and again in a real terminal (tmux, the real binary):
  pick steam-game, change the name (flkt-forge-steam / flkt-forge-tmux),
  fill in the app id, the server command, the ports (2466, host 2476) and
  the ready line, set a backup setting (keep = 5), write. flkforge check:
  0 FAIL (warnings: the two hook stubs, not registered yet). Then
  `flkforge edit`, swap backup-pause for backup-stop on Backups, write:
  the diff of flk.toml is the backup comment and `mode = "pause"` ->
  `"stop"`, keep = 5 carried over; Dockerfile and flk-run unchanged.
- Screens captured from tmux at 80x24 and 160x48: docs/screens/.
- Found by driving it in tmux, and fixed: the Write button fell below the
  form at 80x24 on Review (it's first now); entering a step now focuses
  the first missing param (counting rows by hand was error-prone, so it
  would be for people too); long directories in the header and the
  button; the preview column was too narrow at 160.
