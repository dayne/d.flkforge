# flk requests from flkforge

What flkforge needs from flk but flk doesn't have yet. flk's lead session
reads this; don't patch `~/flk` yourself.

Format: one `## N. Title (date)` per item, then what's missing, why
flkforge needs it, what you do meanwhile, and a proposed CLI/JSON shape
(additive: lazyflk depends on flk's CLI too). Add `Status:` lines as flk
answers.

Filed directly in flk's `TODO.md` on 2026-11-04 (flk session), under
"For flkforge" and planned as `~/flk/PLAN.md` M6: `flk build` (build and
inspect without deploying), image warnings, `type = "exec"` checks,
`[[mounts]] seed = true`, flk.toml keys as JSON, the exact guest env list.
Under Machines: a non-root USER runs as root on busybox images. When one
lands, update the snippets and rules that work around it.

## 1. `type = "http"` checks crash `flk deploy` (2026-11-04)

**What happens:** any `[checks.NAME] type = "http"` makes `flk deploy`
crash once the machine has booted, while it polls the check:

    lib/flk/cli/deploy.rb:313:in `check_passes?': uninitialized constant Flk::Net::HTTP (NameError)

`lib/flk/model.rb` defines `module Flk::Net`, so inside `module Flk` the
name `Net::HTTP` resolves to `Flk::Net::HTTP`. The fix looks like
`::Net::HTTP` at deploy.rb:313 (and anywhere else `Net::HTTP` is used
inside `Flk`). `net/http` itself is required (util.rb:13).

**Seen:** flkforge's M0 deploy test, app `flkt-forge-hello` (flk 0.1.0,
~/flk at f7f9b4a): the image built, the VM booted and served
`http://172.30.0.17:8080/` fine (200 in its log), then deploy died with
the trace above and exited 1. Reproduce: `examples/hello` plus

    [checks.http]
      type = "http"
      port = 8080
      path = "/"
      grace_period = "30s"

then `flk deploy`. No test covers http checks yet, it seems.

**Why flkforge needs it:** `core/check-http` writes exactly this; it's the
natural check for a web service (the `web-service` template, PLAN M3).

**Meanwhile:** `core/check-http` adds a "Gaps in flk" line to the report
pointing here. Without a check, a deploy waits for `internal_port`.

**Asked:** fix the constant, and a test case with an http check.

Status: fixed in flk 4294558 (branch r1-reliability, not on main yet): `::Net` in `check_passes?`, both the request and the rescue list (the rescue raised the same NameError). Test: `tests/cases/10-hello.sh` test_http_checks_gate_the_deploy (200 deploys, 500 fails the check and rolls back).

## 2. Log checks never match a regex that ends in `$` (2026-11-04)

**What happens:** `[checks.ready] type = "log", match = "^fake: Server
ready$"` fails its check although the line is in the console log.
`check_passes?` (deploy.rb ~315) matches the regex against the raw tail
of `console.log`, where every line ends in `\r\r\n` (the serial console;
`flk logs` shows them as `^M^M`). Ruby's `$` matches before `\n`, so after
`ready` comes `\r\r` and `$` never matches. `^` works.

**Seen:** flkforge M1, `flkt-forge-run` v1 (flk at f7f9b4a): the log has
`fake: Server ready`, the check timed out after 60s, deploy exit 1.
No real app ends a check with `$` (vs-run's comment notes the `\r\r`).

**Why flkforge needs it:** `core/check-log` takes the admin's regex, and
anchoring both ends is the natural way to stop chat from faking a ready
line. The same regex works in flk-run (`RUN_READY`), which strips `\r`.

**Meanwhile:** flkforge's rule `log-check-dollar` warns about a `$`, and
the check-log hint says not to use one.

**Asked:** match line by line with trailing `\r` removed (e.g.
`tail.each_line.any? { _1.chomp.delete_suffix("\r")... }`, or strip `\r`
before matching), or document it in `flk help --all` ([checks.NAME]).
