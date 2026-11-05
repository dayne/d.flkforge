# flkforge

Forge a new [flk](https://github.com/dayne/d.flk) app from parts.

flk runs fly.io-style apps as Firecracker microVMs on a local host. Every
app needs a `Dockerfile` and a `flk.toml`, and the ones that run well (the
game servers, the MQTT broker) share the same hard-won patterns: a pinned
build, a run wrapper that turns flk's stop into a clean save, a volume
owned by the right user, health checks a deploy waits for, and a backup
mode that suits the app. flkforge turns those patterns into **snippets**
you pick in a wizard, and it explains each one as you go: which lines it
adds, what flk does with them at build, boot, stop and backup time, and
what to watch out for.

    flkforge templates             # starting points: daemon, steam-game, web-service, blank
    flkforge new myserver --template daemon --yes --set service.port=6390 ...
    flkforge add secret --set name=API_TOKEN     # remix: add, set, rm a snippet, then render
    flkforge render                # rewrite the files from flkforge.toml
    flkforge check                 # recipe, files and flk agree; ports, secrets, deploy state
    flkforge snippets [show ID]    # the catalog (--json)
    flkforge explain run-wrapper   # a snippet, a rule, a flk.toml key, or an ask that isn't a snippet (gpu)

What it writes in the app directory:

- `Dockerfile` and `flk.toml`: plain, commented files like the hand-written
  ones. They belong to you, and nothing at runtime depends on flkforge.
- `flk-run`, the run engine, when the app needs supervising (a clean stop,
  restarts, events), and stub hooks for what is specific to your app.
- `flkforge.toml`: the **recipe**, the snippets and settings you chose,
  so `flkforge edit` can pick up where you left off and a recipe can be
  shared.

flkforge never deploys and never needs root. It finishes with the flk
commands to run next (`flk launch --no-deploy`, `flk volumes create ...`,
`flk secrets set ...`, `flk deploy`).

Later: a **hub** of shared snippets and whole-app recipes
(`flkforge hub search valheim`).

Status: M3 (2026-11-04). The command line is complete for agents and
scripts, with the full catalog: 52 snippets and 9 templates (daemon,
web-service, steam-game, steam-game-windows, java-server, dotnet-server,
python-workbench, dev-sandbox, blank), all learned from the apps on this
host. The wizard (`new` and `edit` in a terminal) is next: `PLAN.md`.

    make install                   # ~/.local/bin/flkforge
    flkforge new ~/myapp.flk --recipe testdata/hello.toml --yes --app flkt-myapp
