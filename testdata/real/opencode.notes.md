# opencode.toml vs ~/opencode.flk

Checked 2026-11-04 against the app as registered (flk config show --json
-a APP, read-only). TestReferenceApps keeps the flk.toml half true.

## flk.toml

`parsed` and `machine_config` are equal. `[flk] stdin = "console"` (the
default, written out in the real file) comes from core/flk-toml, the
escape hatch; `[[restart]] policy = "on-failure"` with no retries from
core/restart.

## Dockerfile

- `FROM docker.io/library/ubuntu:24.04` instead of `FROM ubuntu:24.04`:
  the real file's short name only builds where podman has search
  registries (rule image-full-name).
- One apt line (the real file has two, the second without
  --no-install-recommends or a cleanup), and the Go PPA through
  core/dockerfile-lines.
- sudo through /etc/sudoers.d/developer (mode 440) instead of appending to
  /etc/sudoers.
- The OpenCode install is `curl | bash` into ~ as developer, also through
  core/dockerfile-lines. That's what the real app does, and it's what the
  dev-sandbox template avoids: with the home directory on a volume
  (core/home-volume) it would be hidden, and a deploy wouldn't upgrade it.
  For a new sandbox, install tools system-wide.
- The idle CMD uses /bin/sh and TERM (core/idle-cmd); same behaviour.
