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
