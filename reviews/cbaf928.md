# Review of cbaf928 — commissioned external audit, three threads

The tree reviewed is commit `cbaf928be5362f67a7dbf6378637eba7716ebb5b`, which is
NOT the subject digest this directory is otherwise named by. That is deliberate
and worth explaining rather than tidying: the review was commissioned before
`reviews/` was excluded from the subject, so no digest existed that a record
could be filed under — the defect this review itself found. It is filed by
commit, and the digest-named record for the fixed tree follows it.

- **Reviewer:** independent, working from the public repository at the pinned
  commit. Not the author of any of the code or the specification.
- **Date:** 2026-08-09
- **Commissioned with:** all three prompts of `docs/external-review.md`, run in
  three separate threads from a cold start each.
- **Records:** [`cbaf928.claim.md`](cbaf928.claim.md),
  [`cbaf928.semantic.md`](cbaf928.semantic.md),
  [`cbaf928.adversarial.md`](cbaf928.adversarial.md)

## A limit the reviewer stated, and what was done about it

The audit environment had no Go, Rust, pytest or container toolchain. Every
finding is therefore a source-proven control-flow argument with a minimal
reproducer, and the reviewer said so rather than presenting them as executed.

Nine were subsequently executed here, on the same commit, with both toolchains.

| Finding | Executed result |
|---|---|
| adversarial F-01 — Rust ignores `required` | Go rejects `E_REQUIRED_VALUE_MISSING`; Rust emits `values: null`, exit 0 |
| adversarial F-02 — wrong-typed list input | `groups: administrators` becomes `values: []`; the authored value disappears |
| adversarial F-03 — collection at a scalar position | Rust debug build **panics** on a 5-byte input; release emits `null` |
| adversarial F-04 — Go emits invalid YAML | child named `a: b` produces output PyYAML rejects with `ScannerError` |
| adversarial F-05 — scalar primitive member | Rust prints `valid` for `shape: 7`; Go rejects it |
| adversarial F-06 — trailing YAML document | **both** print `valid`; the second document is never seen |
| adversarial F-07 — stateful boundary forgery | `module.Execute` returned `nil` after exactly **2** calls to `CanonicalYAML()`; the third call returns the attacker's bytes |
| adversarial F-08 — "linear" pre-scan | 10k keys 1.0s · 20k 2.9s · 40k **11.6s** (389 KB). Quadratic, against a comment claiming linear |
| claim F-01 — the review gate is unsatisfiable | adding a tracked record changed the subject from `12b70121…` to `d1d0ef0c…`; the record's own name no longer matched |

The remaining findings — the semantic audit's fifteen, and the claim audit's
release-process items — have not been independently verified here and are
neither accepted nor contested in this record.

## Disposition

`claim F-01` is fixed in the commit that files this record: `reviews/` is
excluded from the subject, the test that hid the defect now tracks its record
before checking, and three tests fail without the fix. Everything else is open.

The finding that matters most for what this repository claims about itself is
`adversarial F-07`. It defeats a boundary check added the same day: `Execute`
reads `CanonicalYAML()` twice, so the check binds two snapshots rather than the
object, and a module reading a third time is unprotected. The fix was aimed at
the right property and used the wrong mechanism.
