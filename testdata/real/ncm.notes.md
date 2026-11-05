# ncm.toml vs ~/ncm.flk

Checked 2026-11-04 against the app as registered (flk config show --json
-a APP, read-only). TestReferenceApps keeps the flk.toml half true.

## flk.toml

`parsed` and `machine_config` are equal.

## Dockerfile

- The same steps: core/git-pinned (with `cpu.patch`), core/pip
  (`torch_cpu`, requirements in /opt/ncm), core/volume-links,
  core/start-script with ncm's start.sh as its body.
- `ENV HF_HOME` comes early (the env slot) instead of after pip: it's
  then also set during the build, which changes nothing here.
- No `RUN chmod +x /start.sh`: flkforge writes start.sh with mode 755.
