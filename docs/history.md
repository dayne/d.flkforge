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
