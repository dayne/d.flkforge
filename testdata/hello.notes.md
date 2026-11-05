# hello.toml vs ~/flk/examples/hello

The rendered flk.toml means the same to flk as the example's: equal
`parsed` and `machine_config` from `flk config show --json`, no
warnings (`TestHelloMatchesFlkExample`). It differs only in comments
and in an extra comment above `[http_service]`.

The Dockerfile does the same steps, with these differences:

- `FROM docker.io/library/alpine:3.20`, not `FROM alpine:3.20`: podman
  on this host has no search registries for short names (rule
  `image-full-name`).
- No `RUN chmod +x /start.sh`: flkforge writes start.sh with mode 755,
  and COPY keeps the mode. One layer fewer; the start-script hint says
  to keep the file executable.
- A header comment (made by flkforge; what's on the volume).
- start.sh is byte-identical (it's the recipe's `body` param).
