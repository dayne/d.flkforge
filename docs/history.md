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
