# mqtt.toml vs ~/mqtt.flk

Checked 2026-11-04 against the app as registered (flk config show --json
-a APP, read-only). TestReferenceApps keeps the flk.toml half true.

## flk.toml

`parsed` and `machine_config` are equal (no run wrapper, so nothing
differs). `retries = 10` is written out because the real file has it
(core/restart-always's `retries`).

## Dockerfile

- The same steps. The two config files are two core/copy-files uses (the
  acl one with its owner and mode) instead of one COPY and a chown line.
- mqtt-run is the start-script's body here (copied from ~/mqtt.flk/mqtt-run
  on 2026-11-04), written once and then the admin's. In the real app it's
  simply a file in the repo. `chmod 755` isn't needed: flkforge writes the
  file with mode 755 and COPY keeps it.
- `mosquitto.conf` and `acl` are the admin's files (copy-files only copies
  them); a render in a scratch directory doesn't have them, which is
  fine for this comparison.
