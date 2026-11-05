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

    flkforge new myserver          # the wizard: pick snippets, see the files build up
    flkforge edit                  # reopen this directory's recipe and remix it
    flkforge render                # rewrite the files from flkforge.toml
    flkforge check                 # recipe, files and flk agree; flk config show is clean
    flkforge snippets              # the catalog (--json)
    flkforge explain steamcmd      # what a snippet does under the hood

What it writes in the app directory:

- `Dockerfile` and `flk.toml`: plain, commented files like the hand-written
  ones. They belong to you, and nothing at runtime depends on flkforge.
- a run wrapper (`app-run`) when a snippet needs one.
- `flkforge.toml`: the **recipe**, the snippets and settings you chose,
  so `flkforge edit` can pick up where you left off and a recipe can be
  shared.

flkforge never deploys and never needs root. It finishes with the flk
commands to run next (`flk launch --no-deploy`, `flk volumes create ...`,
`flk secrets set ...`, `flk deploy`).

Later: a **hub** of shared snippets and whole-app recipes
(`flkforge hub search valheim`).

Status: M0 (2026-11-04). `new --recipe FILE --yes`, `render`, `check`,
`snippets`, `help` and `version` work, with the snippets for a small web
app (`flkforge snippets`). The wizard, `edit`, `explain`, templates and the
rest of the catalog are still to come: `PLAN.md`.

    make install                   # ~/.local/bin/flkforge
    flkforge new ~/myapp.flk --recipe testdata/hello.toml --yes --app flkt-myapp
