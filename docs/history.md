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
