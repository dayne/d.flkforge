# flkforge

Forge a new [flk](https://github.com/dayne/d.flk) app from parts.

flk runs fly.io-style apps as Firecracker microVMs on a local host. Every
app needs a `Dockerfile` and a `flk.toml`, and the ones that run well (the
game servers, the MQTT broker) share the same hard-won patterns: a pinned
build, a run wrapper that turns flk's stop into a clean save, a volume
owned by the right user, health checks a deploy waits for, and a backup
mode that suits the app. flkforge turns those patterns into **snippets**
you pick in a wizard (or from the command line), and it explains each one as you go: which lines it
adds, what flk does with them at build, boot, stop and backup time, and
what to watch out for.

    flkforge new mygame.flk        # the wizard: pick a template, fill in the steps, watch the files build up
    flkforge edit mygame.flk       # reopen the recipe in the wizard and remix it
    flkforge templates             # starting points: daemon, steam-game, web-service, blank...
    flkforge new myserver --template daemon --yes --set service.port=6390 ...
    flkforge add secret --set name=API_TOKEN     # remix: add, set, rm a snippet, then render
    flkforge render                # rewrite the files from flkforge.toml
    flkforge check                 # recipe, files and flk agree; ports, secrets, deploy state
    flkforge snippets [show ID]    # the catalog (--json)
    flkforge explain run-wrapper   # a snippet, a rule, a flk.toml key, or an ask that isn't a snippet (gpu)
    flkforge import ~/old.flk      # a recipe from a hand-written app (best effort; --to DIR, --in-place)

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

Status: M4 (2026-11-05). The wizard (`new` and `edit` in a terminal),
the command line for agents and scripts, flk-run, and a catalog of 52
snippets and 9 templates learned from the apps on this host. Screens:
`docs/screens/`. What's next: `PLAN.md` (Later) and `TODO.md`.

    make install                   # ~/.local/bin/flkforge
    flkforge new ~/myapp.flk --recipe testdata/hello.toml --yes --app flkt-myapp
