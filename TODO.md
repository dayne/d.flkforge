# TODO

Open questions. Decide the ones marked (M0) before writing code that
depends on them; ask the user when it's their call.

## Decisions

- (M0) Final `snippet.toml` field names. The sketch in SPEC 4 is a
  starting point; settle it and update the SPEC.
- (M0) Forms: `huh` (if a bubbletea-v2 version exists) or hand-built
  widgets.
- (M1) One generic run wrapper with params, or a small family
  (console-stop, signal-stop)? Decide after reading all four `*-run`
  scripts side by side.
- (M1) Should generated files carry a "made by flkforge" header? Leaning
  towards one comment line in each file pointing at `flkforge.toml`,
  since the drift hashes do the real work.
- (M2) How much of flk's own help to show inline versus pointing at
  `flk help ...`. Inline text goes stale; pointers don't.

## Hub (later, design only)

- Transport: a git repo of snippets (simple, reviewable) or an HTTP index.
- Trust: signing, review, pinning by hash; what `add` shows before using
  a non-local snippet.
- Publishing: `flkforge hub publish` from a recipe, and what gets
  stripped (site-specific values, secret names?).
- Whether lazyflk browses the hub too.

## Ideas

- `flkforge explain KEY` for any flk.toml key, built from flk's reference
  plus the snippets that set the key.
- Import an existing app into a recipe (PLAN, Later).
- A `preflight` snippet that writes a `bin/APP preflight` in the
  PREFLIGHT.md style.
