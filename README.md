# cic-object-model

The normative specification of the CIC object model, its conformance vectors,
and the Go reference implementation. The Rust one is not written.

**[`SPEC.md`](SPEC.md) is the authority.** The implementations are subordinate
to it: where an implementation disagrees with the specification, the
implementation is wrong.

---

## Status

Model version **0.2**. SPEC.md declares it; `TestVersionIdentity` holds every
other declaration in the repository to that number, because 0.2 was once
written into SPEC.md and propagated nowhere else.

| Component | Status |
|---|---|
| `SPEC.md` — 40 numbered invariants | written, normative |
| `conformance/` — 26 vectors | executed against Go on every CI run |
| `docs/spec-vector-map.md` | 35 invariants vector-covered, 5 declared unvectorizable with a reason each |
| `tools/check_spec_vectors.py` | runs and passes; negative-tested |
| `go/` | implemented — corpus, fuzz, mutation, adversarial and CLI golden tests |
| `rust/` | **empty** — the second implementation does not exist |
| `mk/rust.mk` | absent — see [`docs/rust-gate-extraction.md`](docs/rust-gate-extraction.md) |
| Docker build / CI | runs; `make ci` is the same pipeline locally and in Actions |

Two limits worth stating before you rely on any of the above.

**One implementation is not two.** The mutual check described below is the
reason there are meant to be two, and it is not in force: every claim that "the
model behaves this way" currently rests on one reading of the corpus by one
implementation, plus the corpus itself.

**§8.8 defines no canonical byte encoding** (`docs/spec-defects.md` SD-010), so
`INV-030`'s determinism is checked structurally rather than byte for byte. Until
that is written, "the two implementations agree" cannot mean byte-identical
output, and a digest taken over a canonical object has no specified input.

---

## Why a specification repository at all

Two properties are being frozen, and both need to be frozen *before* code
exists rather than distilled from it afterwards:

1. **The recursive node model** — primitives are not metadata attached to a
   node; they are CIC nodes themselves, which is what makes
   `network.values.mtu.access.read` an addressable, hashable, governable object
   rather than a path into a YAML blob.
2. **Closed semantics** — notably `origin`'s four permitted forms, and the
   three-rule object closure that leaves no way to smuggle an uninterpreted
   object graph past the model.

Spec, vectors and both implementations live in one repository on purpose. A
semantic change then arrives as a single change — SPEC + vectors + Go + Rust —
and it becomes physically awkward to alter one implementation's behaviour
without the corpus and the other implementation noticing. That mutual check is
the reason there are two implementations rather than one.

---

## Layout

```
SPEC.md                     the normative specification
spec/                       machine-readable schemas for the model itself
conformance/                the falsifiable part — YAML in, YAML out
  materialization/            authoring input -> canonical object
  invalid/                    input that MUST be rejected
  validation/                 canonical object -> accept / reject
docs/
  spec-vector-map.md          every invariant -> its vectors, or why it has none
  decision-delta.md           what this model changes in D-003 / D-011
  migration-surface.md        the measured file list this model would change
  branch-decision.md          why base-repo wasm/main
  rust-gate-extraction.md     line-referenced recipe for mk/rust.mk
go/                         reference implementation
rust/                       reference implementation (not written)
```

## Make targets

`make ci` runs the whole pipeline and is what CI runs — there is no separate
command, so a green badge and a green local run mean the same thing.

| Target | What it does |
|---|---|
| `make ci` | the full pipeline: gates, spec checks, Go build/test/coverage |
| `make check` | Python/YAML quality gate |
| `make manifest-verify` | `MANIFEST.sha256` describes the tracked tree exactly |
| `make docs.link-check` | internal documentation links resolve |
| `make golang.quality` | Go gate over `go/` |
| `make golang.coverage-threshold` | fails below `COVERAGE_MIN` (90%) |
| `make conformance` | run the corpus against every present implementation |
| `make verify` | fuzz, mutation and adversarial suites |

`tools/check_spec_vectors.py` runs in CI and fails if `SPEC.md` and the vector
corpus drift apart — if an invariant claims a vector that does not exist, or a
vector claims an invariant that does not. It checks the *mapping*; it does not
run vectors.

## Reading order

1. [`SPEC.md`](SPEC.md) §2 (node model) and §4 (the discriminator) — the two
   sections everything else depends on
2. [`SPEC.md`](SPEC.md) §5 (origin) and its truth table
3. [`conformance/README.md`](conformance/README.md) — the vector format
4. [`docs/decision-delta.md`](docs/decision-delta.md) — what this changes and
   what it would otherwise have lost silently

## License

CC-BY-NC-SA-4.0 — see [`LICENSE.md`](LICENSE.md).
