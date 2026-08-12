> # ⛔ ARCHIVED — REJECTED DIRECTION
>
> **Do not build on this repository. Do not treat `SPEC.md` as a current CIC
> contract.** It is kept public and read-only as a record of an attempt.
>
> Abandoned on **2026-08-10** by the owner's decision, stated as: *"it turned
> out to be a wrong concept."* No further rationale was recorded, and none is
> invented here.
>
> ## Read this before judging the code
>
> This was **not** abandoned because it was broken. At the point it stopped, the
> corpus was green and the known defects were being closed in the open:
> [`docs/spec-defects.md`](docs/spec-defects.md) catalogues **19 specification
> defects, 9 of them marked fixed in model 0.2**, each with the reasoning. The
> hardest one — `INV-033`, which demanded that a canonical object carry its own
> model version while §2.1 left nowhere to put it — was resolved rather than
> patched: the version became a property of the hand-off frame, not a member of
> the object.
>
> What was rejected is the **shape of the model**, not the state of the work.
> The CIC primitive language continues as
> `atomic → aggregate → domain composition` in
> [`cic-primitives`](https://github.com/CentralInfraCore/cic-primitives).
>
> ## What must NOT be carried forward
>
> The ontology and its execution:
> `Node { values, origin, primitives }` · every primitive being itself a
> recursive CIC node · the schema-position `values` discriminator · the `origin`
> truth table · `sealed_from` template materialisation · this repository's
> restructuring of Access · its canonical YAML shape · the materializer
> pipeline's semantics.
>
> A structural constraint worth understanding before anyone proposes it again:
> `origin` cannot be a primitive. Every primitive is a node, every node has an
> `origin`, so an `origin` that were a primitive would need one of its own,
> without end. It is the fixed point that terminates the recursion, and a fixed
> point cannot be a member of the set it terminates (`SPEC.md` §6.2).
>
> ## What is worth extracting
>
> The **input-hardening layer**, which knows nothing about the ontology above it.
> Measured 2026-08-12, counting model-specific identifiers per file:
>
> | file | lines | model references |
> |---|---|---|
> | `rust/src/value.rs` | 377 | **0** |
> | `rust/src/error.rs` | 114 | 1 |
> | `rust/src/canonical.rs` | 282 | 13 |
> | `rust/src/node.rs` | 202 | 25 |
> | `rust/src/materialize.rs` | 645 | 65 |
>
> `value.rs` refuses duplicate mapping keys at the event level, refuses anchors
> and aliases *before* a tree exists (measured amplification: 393 bytes →
> 12,345,678 nodes in 2.7 s), enforces string keys and preserves insertion
> order. That layer is portable. The pipeline above it is not — `materialize.rs`,
> the largest file, is written in the rejected ontology throughout.
>
> Reusable as *method* rather than content: the error model (code + invariant +
> stage + object path), a conformance harness that cannot report success with
> zero vectors, byte-exact comparison, and the boundary type whose constructor is
> private so that only the validator can produce one.
>
> ## The result that outlived the model
>
> Two independent implementations, written from the specification rather than
> from each other, found **two real divergences the conformance corpus could not
> see**: on a duplicate mapping key Go rejected the document while Rust silently
> took the last value; on aliases they disagreed in the other direction. Neither
> case appeared in any vector. They became `INV-041` and `INV-042`.
>
> **A corpus proves what it contains. Only differential execution finds what
> nobody thought to write down.**
>
> ## Where the extracted kernel goes
>
> Into a **separate repository of its own** — not here, and not inside
> `cic-primitives`. Rebuilding it in place would recreate exactly the
> everything-in-one-model repository this one became.
>
> ## Branch note
>
> `main` is **28 commits behind** `devel`. `devel` (`4918240`) carries the
> furthest state: the second implementation, the canonical serialization work,
> and the later defect closures. Neither branch is maintained.

---

# cic-object-model

The normative specification of the CIC object model, its conformance vectors,
and two independent reference implementations, in Go and in Rust.

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
| `SPEC.md` — 46 numbered invariants | written, normative |
| `conformance/` — 38 vectors | executed against Go on every CI run |
| `docs/spec-vector-map.md` | 37 invariants vector-covered, 5 declared unvectorizable with a reason each |
| `tools/check_spec_vectors.py` | runs and passes; checks the invariant index, not RFC-2119 clauses |
| `go/` | implemented — corpus, fuzz, mutation, adversarial and CLI golden tests |
| `rust/` | implemented — corpus, reader, rejection and CLI golden tests |
| `mk/rust.mk` | present; digest-pinned toolchain, `make rust.quality` |
| Docker build / CI | runs; `make ci` is the same pipeline locally and in Actions |

Both implementations pass all 38 vectors, and neither was written from the
other: the Rust one was written from `SPEC.md` and the corpus, deliberately not
from `go/`. Two implementations that share an author's reading share that
reading's mistakes, and their agreement then proves nothing.

**§8.8.1 defines the canonical serialization byte for byte**, and §8.8.2 the
member order. Both runners compare bytes, and the two implementations produce
**byte-identical objects on all 13 materialization vectors**.

Before that was written they produced **zero** identical objects while agreeing
on every one semantically, and both were conformant — `INV-030` alone only
constrains an implementation to agree with itself. A digest taken over a
canonical object now has a defined input.

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
  pending-decisions.md        questions the spec does not answer and code does
  decision-delta.md           what this model changes in D-003 / D-011
  migration-surface.md        the measured file list this model would change
  branch-decision.md          why base-repo wasm/main
  rust-gate-extraction.md     line-referenced recipe for mk/rust.mk
go/                         reference implementation
rust/                       reference implementation
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
| `make rust.quality` | the Rust gate: pin, fmt, clippy, coverage, `cargo deny` |
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
