# SPEC.md defects found by implementing it

Model version 0.1, measured against `SPEC.md` at `9ae04c1` and the 27-vector
corpus in `conformance/`.

This is the report the Go reference implementation produced by being the first
thing to execute the corpus. Sixteen defects were recorded by that job; a
seventeenth (**SD-017**) was derived during orchestrator review and is the root
cause of SD-004 — read it first. None of them was
worked around in code without being written down here first, and every one is
anchored by an `SD-nnn` comment at the place in `go/` where the choice was
forced.

**All 27 vectors pass.** That is not the same as "the specification is
correct": in six places the corpus and the normative text disagree, and the
corpus won, because `SPEC.md` §10 defines conformance as matching the vectors.
Those six are the ones to read first: **SD-003, SD-004, SD-005, SD-009,
SD-013, SD-014.**

## Status at model 0.2

Nine of these are fixed by model 0.2 — every one that made 0.1 unsatisfiable,
plus the three that made it merely wrong. Nine remain open. The fixed ones are
kept rather than deleted: a defect report that erases its own history stops
being evidence, and the reasoning is what makes the fix reviewable.

| | Count | Which |
|---|---|---|
| **Fixed in 0.2** | 9 | SD-003, SD-004, SD-005, SD-009, SD-012, SD-013, SD-014, SD-017, SD-018 |
| **Open** | 9 | SD-001, SD-002, SD-006, SD-007, SD-008, SD-011, SD-015, SD-016, **SD-019** |

SD-019 arrived after 0.2 shipped, from attacking INV-032 rather than asserting
it. The implementation compensates; the specification's claim is still false.

All six blocking defects are fixed. The nine that remain are five
*underspecified*, two *editorial*, one *divergent* (SD-002) and one *editorial*
that is really a known limit of Go (SD-016). None of them makes an invariant
unsatisfiable; each is a place where the document says less than an implementer
needs, and each is now a decision waiting rather than a surprise.

**A note for the second implementation.** The corpus and the text agree at 0.2
in the nine places they did not before, so a Rust implementation is no longer
being asked to reproduce a known contradiction. The six conflicts that made 0.1
dangerous to implement twice are gone.

---

## Severity

| | Meaning |
|---|---|
| **blocking** | Two normative statements cannot both hold. An implementer must violate one. |
| **divergent** | The corpus requires behaviour the normative text does not state, or contradicts. |
| **underspecified** | Two readings both satisfy the text and produce different output. |
| **editorial** | Wrong or incomplete, but not behaviour-changing. |

---

## SD-004 — INV-022 and INV-021 are both false of the root node

> **FIXED in model 0.2.** the root is an ordinary node — it carries its declared `shape` and no `cic` member.

| | |
|---|---|
| **Where** | `SPEC.md` §6.1 (INV-021, INV-022), §11 (INV-033) |
| **Severity** | **blocking** |
| **Anchor** | `go/objectmodel/primitives.go`, `canonical.go`, `validate.go` |

INV-022: *"A canonical node MUST carry every primitive its schema declares for
that position."* Every vector schema declares `root: shape: object`. So every
canonical object's root node MUST carry a `shape` primitive.

**No expected.yaml in the corpus has one.** All 13 materialization vectors show
a root of exactly `cic`, `values`, `origin`.

Simultaneously INV-021: *"A node MUST NOT carry a member that is neither
`values`, `origin`, nor a member of the primitive set."* The root carries
`cic` (required by INV-033). `cic` is not `values`, not `origin`, and not one
of the eight atoms.

So the root node violates INV-021 as specified, and satisfying INV-022 on it
would break the entire corpus.

**Why it could not be worked around.** These are not compatible readings —
they are opposite outputs. Measured: with the root emitting its declared
`shape`, **13 of 13 materialization vectors fail**; with it omitted, 13 pass.

**Suggestion.** State that the document root is a node *plus* the `cic`
envelope member, and that INV-021/INV-022 are scoped to nodes below the root —
or make `cic` a sibling of the root node rather than a member of it. The second
is cleaner: it keeps INV-021 total.

---

## SD-005 — INV-005 is unfalsifiable as worded, and `invalid/008` tests something else

> **FIXED in model 0.2.** INV-005 terminates on schema finiteness; a repeated primitive name is legitimate nesting.

| | |
|---|---|
| **Where** | `SPEC.md` §2.3 (INV-005); `conformance/invalid/008_cyclic_primitive_declaration` |
| **Severity** | **divergent** |
| **Anchor** | `go/objectmodel/schema.go` — `checkPrimitiveCycle` |

INV-005 says the primitive-declaration graph must be acyclic: *"a primitive's
own primitives MUST NOT, directly or transitively, re-declare the node they
hang from."*

The vector schema language (`conformance/README.md`) is a **finite literal
tree**. It has no `$ref`, no named type reuse, no recursion mechanism. A schema
written in it cannot re-declare a node, and materialization against it cannot
fail to terminate. **INV-005 as stated cannot be violated by any schema the
corpus can express**, so it is not falsifiable.

What `invalid/008` actually contains is a finite, explicitly written nesting:

```
mtu.access.read.contract.rules.guard.access.read.contract.rules.guard: {}
```

That terminates. It is not a cycle in the node graph. The inner `access` hangs
from `guard`, not from `mtu`. The vector is testing a different and stronger
property: **acyclicity of the primitive-*name* graph** (`access` → `contract` →
`access`), which the expected error path confirms —
`$.mtu.access.read.contract.rules.guard.access` is the point where a primitive
name repeats on its own declaration path.

**Why it could not be worked around.** Implementing INV-005 literally makes the
check dead code and `invalid/008` fail. This implementation enforces the
name-graph property to pass the vector.

**Suggestion.** Either reword INV-005 to the property the vector tests ("a
primitive name MUST NOT reappear on its own declaration path"), or keep INV-005
and mark it unvectorizable-until-the-schema-language-has-references, and give
`invalid/008` its own invariant. Note the name-graph rule is strictly stronger
and forbids legitimate finite schemas — that trade-off should be a decision,
not a side effect.

---

## SD-013 — §8.7 must discriminate without a schema, which INV-008 forbids

> **FIXED in model 0.2.** INV-008 scoped to materialization; INV-039 states the schema-less walk is weaker, with its residual risk named.

| | |
|---|---|
| **Where** | `SPEC.md` §4.3 (INV-008), §8.7; `conformance/validation/*` |
| **Severity** | **blocking** |
| **Anchor** | `go/objectmodel/validate.go` — `validatePayload` |

INV-008: *"Whether a mapping is a node envelope or payload MUST be determined
by the schema-declared shape at that position. An implementation MUST NOT
decide this by inspecting keys alone."*

All six `validation/` vectors supply **`object.yaml` and nothing else** — no
`schema.yaml`. `conformance/README.md` explains why this is necessary: truth
table rows 7 and 8 are unreachable from authoring input, so they can only be
exercised against an already-canonical object.

But final validation has to walk that object, and to walk it it has to know
which mappings are nodes. With no schema, the only available signal is key
inspection — precisely what INV-008 prohibits.

**Why it could not be worked around.** There is no third option. This
implementation inspects keys (a mapping is a node iff it contains `values`) and
therefore violates INV-008 on the `validation/*` path. The failure mode is
real, not theoretical: an opaque payload that happens to contain a `values` key
mapping to a mapping will be mis-identified as a node and validated as one.

**Suggestion.** Scope INV-008 to materialization (§8.1–§8.6), and state
explicitly that §8.7 on a schema-less object is structural, key-directed, and
weaker — or require `validation/` vectors to carry the schema.

---

## SD-003 — primitive payloads are not materialized, contradicting §2.2 and INV-027

> **FIXED in model 0.2.** INV-035 carries INV-027 into primitive payloads; §2.2 is now true of the objects the model produces.

| | |
|---|---|
| **Where** | `SPEC.md` §2.2, §7 (INV-027); `conformance/materialization/011`, `013` |
| **Severity** | **divergent** |
| **Anchor** | `go/objectmodel/primitives.go` — `normalizePrimitive` |

§2.2 is the passage that motivates the whole recursive model:

> This is what makes `network.values.mtu.access.read` a first-class,
> addressable object rather than a path into a YAML blob. It can be hashed,
> diffed, referenced from evidence, and governed by its own `access` — because
> it is a node like any other.

In the corpus it is not. `materialization/013` expects:

```yaml
access:
  values:
    read:
      rules: {operator: {subjects: [...], effect: allow}}
      inherit: true
      default_injection: 0
  origin: [schema]
```

`access.read` is a plain mapping. It has no `values`, no `origin`, no address
of its own. It cannot be governed by its own `access`, and it cannot be
referenced from evidence as a node — exactly the properties §2.2 claims for it.
The same is true of `shape.values`, which is a raw `{type, scalar_type}` map.

INV-027 compounds this: *"A structured object known to the schema MUST be
recursively materialized: every schema-known child MUST become a CIC node."*
The `access` declaration in `schema.yaml` **is** schema-known and structured.

**Why it could not be worked around.** Materializing primitive payloads into
nodes fails `materialization/011` and `013`. This implementation keeps them
verbatim.

**Suggestion.** Decide whether primitive-internal structure is nodes or data.
If data (as the corpus says), §2.2's motivating example is wrong and should be
replaced — it is currently the strongest argument in the document for a
property the model does not have. If nodes, both vectors need rewriting and the
recursion needs a stated depth bound.

---

## SD-014 — INV-007 and INV-011 disagree about `origin` in a non-opaque payload

> **FIXED in model 0.2.** INV-010 reserves `origin` alongside `values`, making the conflict unreachable.

| | |
|---|---|
| **Where** | `SPEC.md` §3 (INV-007), §4.3 (INV-010, INV-011) |
| **Severity** | **blocking** |
| **Anchor** | `go/objectmodel/entry.go` — `entryWalk` |

INV-007: *"Authoring input MUST NOT contain an `origin` member at any depth
**outside an opaque payload**."*

INV-011: *"Below a `values` member, no CIC primitive interpretation applies.
Keys named `values`, `origin`, `access`, `shape` or any other primitive name
occurring in payload are domain data and MUST be preserved verbatim."*

A structured (non-opaque) payload containing a key named `origin` satisfies
INV-011's "preserve verbatim" and violates INV-007's "at any depth outside an
opaque payload". INV-010 reserves only `values` at schema-declaration time, so
a schema may legally declare a child named `origin` and make this reachable.

**Measured.** With a schema declaring `iface.origin: {shape: scalar}` and input
`iface: {origin: customer-supplied}`, this implementation (following INV-011)
produces a canonical object containing:

```yaml
values:
  iface:
    values:
      origin:                       # a node named origin...
        values: customer-supplied
        origin: [yaml]              # ...carrying its own origin
```

Following INV-007 instead, the same input is rejected at entry validation. Two
readings, both textually supported, opposite outputs. No vector covers it.

**Why it could not be worked around.** A choice was forced. This implementation
applies INV-007 to envelope-level `origin` members only, which is what passes
both `invalid/005` (envelope `origin` → rejected) and `materialization/012`
(opaque payload `origin` → preserved).

**Suggestion.** Either extend INV-010 to reserve `origin` as well as `values`
at schema-declaration time — which makes the conflict unreachable and is the
smaller change — or restate INV-007 as "MUST NOT contain an `origin` **envelope
member**".

---

## SD-009 — §8.2 has no syntax, no vector, and INV-031(a) is mis-mapped

> **FIXED in model 0.2.** §8.2 is out of scope for 0.2 and keeps its pipeline position.

| | |
|---|---|
| **Where** | `SPEC.md` §8.2, §9 (INV-031a); `docs/spec-vector-map.md` |
| **Severity** | **divergent** |
| **Anchor** | `go/objectmodel/refs.go` |

§8.2 is a normative pipeline stage with two MUSTs and a FAILURE clause. The
vector schema language defines **no external-reference syntax at all**, so no
input can carry a reference, no vector exercises the stage, and the stage is
necessarily a no-op in any 0.1-conformant implementation.

`docs/spec-vector-map.md` claims otherwise. Its INV-031 row maps clause (a)
*"unresolved references"* to **`invalid/003_closure_undeclared`** — a vector
whose `meta.yaml` declares `invariants: [INV-029]` and whose subject is an
undeclared arbitrary object. It has nothing to do with reference resolution.
The clause is listed as covered and is not.

This also punctures the claim in §10 that "every normative statement in this
document is mapped to a vector or is explicitly marked unvectorizable":
`check_spec_vectors.py` checks `INV-nnn` ↔ vector mappings, and the FAILURE
clauses of §8.1–§8.8 carry no INV numbers, so they escape the gate entirely.
§8.5's *"a required value neither authored nor defaultable → reject"* has no
code, no vector and no invariant.

**Suggestion.** Mark §8.2 out of scope for 0.1 alongside the template
resolution protocol (§1 already excludes that), correct the INV-031(a) mapping,
and give the §8 FAILURE clauses invariant numbers so the gate can see them.

---

## SD-002 — `schema-load` is a pipeline stage §8 does not have

| | |
|---|---|
| **Where** | `SPEC.md` §8; `conformance/README.md` error table |
| **Severity** | **divergent** |
| **Anchor** | `go/objectmodel/errors.go` — `StageSchemaLoad` |

§8 enumerates eight stages, beginning at entry validation. Three vectors
(`invalid/004`, `007`, `008`) assert `stage: schema-load`, and
`conformance/README.md` lists two error codes as "Raised at: schema load".

`conformance/README.md` also makes stage placement normative and binding:
*"Rejecting with the right code at the wrong stage is a failure."* So an
implementation is required to raise errors at a stage the normative pipeline
does not define. §8 also names none of its stages — the strings
`entry-validation`, `primitive-evaluation` and `final-validation` exist only in
the vectors.

**Suggestion.** Add §8.0 "Schema load" with its own INPUT/OUTPUT/MUST/FAILURE
block (INV-010, INV-015, INV-005 belong to it), and give every stage its
normative string name in §8.

---

## SD-001 — `access` sub-defaults are injected by the vectors, by no invariant

| | |
|---|---|
| **Where** | `SPEC.md` §6.4 (INV-025, INV-026); `materialization/011`, `013` |
| **Severity** | **underspecified** |
| **Anchor** | `go/objectmodel/primitives.go` — `normalizePrimitive` |

`materialization/013`'s schema declares `access.read` with no `inherit`. The
expected output has `inherit: true`. So a sub-default was injected.

The same vector's `access.modify` has no `default_injection`, and INV-026 says
its default is `null` — the expected output does **not** contain
`default_injection: null`. So that sub-default was not injected.

Both fields are described as having a default in identical language
("`true` (default)", "`null` (the default)"). One is materialized, the other is
not, and no invariant states that primitive-internal defaults are materialized
at all. INV-022 governs *primitives*, not fields inside a primitive's payload.

**Why it could not be worked around.** Injecting both fails `013`; injecting
neither fails `011` and `013`. This implementation injects `inherit` only,
matching the corpus.

**Suggestion.** State the rule: which primitive-internal fields are
materialized when absent, and which are absent-means-default. As written, an
independent implementer has no way to derive the corpus's behaviour.

---

## SD-006 — a sealed node's shape and children come from the template, unstated

| | |
|---|---|
| **Where** | `SPEC.md` §8.3; `materialization/003`, `004` |
| **Severity** | **underspecified** |
| **Anchor** | `go/objectmodel/template.go` — `buildEffective` |

In both vectors, `access_policy` is declared with `sealed_from` **and nothing
else** — no `shape`, no `children`, no `default`. The expected output gives it
`shape: {values: {type: object}, origin: [schema]}` and a fully materialized
child.

So the node's entire schema is adopted from the template entry at the
referenced path. §8.3's MUST covers only origin recording ("record template
identity and source path"); nothing says the template's *schema* becomes the
node's schema, which is the larger behaviour.

A second unstated rule sits inside the same vectors: the child `enabled` gets
`path: $.access.read` in its origin — the template **mount** path, not the
child's own path within the template (`$.access.read.enabled`). INV-015 says
the path exists to distinguish instantiations, which is consistent with the
mount path, but the choice is nowhere stated.

**Suggestion.** Add to §8.3: a `sealed_from` node adopts the referenced
template node's schema in full, and every node produced from that expansion
carries the mount path — not its own sub-path — in its origin's `sealed` term.

---

## SD-010 — INV-030 requires byte-identity; §8.8 defines no serialization

| | |
|---|---|
| **Where** | `SPEC.md` §8 (INV-030), §8.8, §10 |
| **Severity** | **underspecified** — **FIXED in 0.2 by §8.8.1 / §8.8.2** |
| **Anchor** | `go/objectmodel/emit.go`, `rust/src/canonical.rs` |

**Resolved.** §8.8.1 (INV-043) defines the serialization byte for byte and
§8.8.2 (INV-044) defines the member order. Both implementations emit it from a
hand-written serializer rather than a library's encoder, and both conformance
runners compare bytes.

The prediction below was exactly right and was measured before the fix: **zero
of thirteen** materialization vectors matched between Go and Rust, while all
thirteen agreed semantically. Two of the differences were not formatting — Go
sorted every mapping alphabetically, which reordered opaque payloads, data §4
promises to carry untouched, and no structural comparison could see it.

After: **13/13 byte-identical between the two implementations**, and each
matches `expected.yaml` byte for byte.

The original entry follows.

INV-030: *"the same schema and input MUST produce a byte-identical canonical
object."* §8.8: *"MUST produce a deterministic serialization."* §10: output must
match `expected.yaml` **"exactly"**.

Nothing defines the mapping key order, the indent, the sequence style, or the
scalar quoting. Two conformant implementations will produce different bytes for
the same object — Go and Rust certainly will, and the corpus itself is
inconsistent (`origin: [yaml]` flow style in the expected files, which no
stated rule requires).

Read strictly, INV-030 is only per-implementation determinism, and then §10's
"exactly" cannot mean bytes. Read as cross-implementation, INV-030 is
unsatisfiable without a canonical form.

**Why it could not be worked around.** This implementation defines its own
order (node members: `values`, `origin`, then primitives in §6.1 order; payload
mappings sorted) and the vector runner compares **parsed structure, not
bytes**. A byte comparison would fail all 13 materialization vectors purely on
formatting.

**Suggestion.** Specify a canonical serialization in §8.8, or state that
conformance is semantic equality of the parsed object and demote INV-030 to
per-implementation determinism. This matters more than it looks: `origin` is
supposed to be hashable evidence, and a hash over undefined bytes is not
evidence.

---

## SD-012 — §8.6 mandates `inherit` chain resolution that 0.1 cannot perform

> **FIXED in model 0.2.** INV-037 records `inherit` verbatim and forbids resolving it; §6.4 names the three undefined questions.

| | |
|---|---|
| **Where** | `SPEC.md` §1, §6.4 (INV-025), §8.6 |
| **Severity** | **blocking** |
| **Anchor** | `go/objectmodel/primitives.go` — `evaluatePrimitives` |

§8.6 MUST: *"materialize every declared primitive (INV-022); **resolve
`inherit` chains for `access`** (INV-025)."*

INV-025's tri-state cannot be resolved in 0.1:

- `true` → "sub-objects inherit this rule" — needs a parent rule to inherit.
- `0` → "full reset; the sub-object **recomputes from its PolicySurface**".

§1 puts the runtime policy-decision point out of scope, and
`docs/decision-delta.md` defers PolicySurface entirely, stating that
per-operation inheritance semantics *"will have to define what per-operation
inheritance means when the two operations disagree. This specification does not
define that."*

So §8.6 contains a MUST whose semantics the same document declines to define.
`materialization/013` records `read.inherit: true` and `modify.inherit: 0`
verbatim — divergent, unresolved — so no vector requires resolution either.

**Why it could not be worked around.** Resolution is not implementable without
inventing PolicySurface semantics, which would be spec invention. This
implementation records `inherit` and does not resolve it. **DoD-relevant: this
is one §8.6 MUST this implementation does not satisfy, and cannot.**

**Suggestion.** Strike "resolve `inherit` chains" from §8.6 for 0.1 and add it
to §1's out-of-scope list next to the policy-decision point, so the pipeline
does not carry an obligation the model defers.

---

## SD-007 — no rule for an absent value that is neither defaultable nor required

| | |
|---|---|
| **Where** | `SPEC.md` §8.5 |
| **Severity** | **underspecified** |
| **Anchor** | `go/objectmodel/defaults.go` — `materializeDefaults` |

§8.5's only FAILURE is *"a required value neither authored nor defaultable →
reject"*. A child that is declared, **not** required, has no default, and is not
authored, falls through every clause. INV-001 forbids a node without `values`,
so it cannot be emitted empty. No vector reaches the case.

Three possible behaviours — omit the node, emit `values: null`, reject — and
nothing selects one. This implementation omits it, the only option that neither
invents a value nor violates INV-001.

**Suggestion.** Add the rule to §8.5. "Omitted" is the natural choice but it
has a consequence worth stating: the canonical object's member set then depends
on what was authored, which weakens INV-022's "every primitive its schema
declares" into "every primitive of every node that exists".

---

## SD-008 — INV-029 covers objects; undeclared scalars are unruled

| | |
|---|---|
| **Where** | `SPEC.md` §7 (INV-029), §8.1 |
| **Severity** | **underspecified** |
| **Anchor** | `go/objectmodel/entry.go` — `entryPayload` |

INV-029: *"An **object** that is neither schema-known nor explicitly declared
opaque is invalid and MUST be rejected."* `invalid/003` exercises exactly that —
`surprise: {arbitrary: structure}`.

An undeclared **scalar** (`surprise: 5`) is not an object. No invariant rejects
it, and nothing says to keep it either — and it cannot be kept, since it has no
schema position and so no shape, no origin rule and no place in the canonical
form. Silently dropping authored data is the worst of the three outcomes.

This implementation rejects undeclared children of any type with
`E_UNDECLARED_OBJECT`/INV-029, which is stricter than the invariant says.

**Suggestion.** Reword INV-029 to "a value" rather than "an object", or add a
companion invariant for undeclared scalars.

---

## SD-011 — the schema language is normative in behaviour, informal in status

| | |
|---|---|
| **Where** | `SPEC.md` (absent); `conformance/README.md` "Schema language used by the vectors" |
| **Severity** | **editorial** |
| **Anchor** | `go/objectmodel/schema.go` — `parseSchemaNode` |

`SPEC.md` says the schema decides everything — the discriminator (INV-008), the
primitive set at a position (INV-022), opacity (INV-028), defaults (§8.5) — and
never defines what a schema is. The only definition is a fenced block in
`conformance/README.md` introduced as *"Deliberately minimal — just enough to
exercise the model"*.

Consequences an implementer hits immediately: whether an unrecognised schema key
is an error or inert (this implementation rejects, fail-closed); whether
`shape:`+`scalar_type:` siblings collapsing into one `shape` primitive node is
normative (the vectors require it, no text states it); whether a schema may
declare a primitive at a position the instance may then override (`011` relies
on it).

**Suggestion.** Either promote the schema language to a normative section of
`SPEC.md`, or state explicitly that the vector schema language is a test
fixture and that real schemas come from `CIC-Schemas` — in which case the
corpus is testing the model through a language that is not the real one, and
that should be said out loud.

---

## SD-015 — `conformance/README.md`'s error table lists 8 of 13 codes

| | |
|---|---|
| **Where** | `conformance/README.md` "Error codes" |
| **Severity** | **editorial** |

The table is presented as the code set and the stage mapping. Five codes used
by vectors are missing from it:

| Missing code | Vector | Stage |
|---|---|---|
| `E_CYCLIC_PRIMITIVE_DECLARATION` | `invalid/008` | schema-load |
| `E_ORIGIN_NOT_TERMINAL` | `validation/003` | final-validation |
| `E_DOCUMENTATION_ON_NODE` | `validation/004` | final-validation |
| `E_DEFAULT_MEMBER_ON_NODE` | `validation/005` | final-validation |
| `E_MISSING_MODEL_VERSION` | `validation/006` | final-validation |

Since the table is the only place stage placement is stated normatively
("An implementation MUST raise the stated code at the stated stage"), a code
absent from it has no specified stage at all.

**Suggestion.** Complete the table, and add the codes §8's FAILURE clauses
require but no vector exercises (see SD-009).

---

## SD-016 — INV-032 is not fully achievable in Go; the residue is `nil`

| | |
|---|---|
| **Where** | `SPEC.md` §9 (INV-032) |
| **Severity** | **editorial** |
| **Anchor** | `go/objectmodel/materialize.go`, `go/module/module.go` |

INV-032 requires that a value of `Validated<Canonical<CICObject>>` be
*"constructible only by the core materializer"*, and names the mechanism: *"an
unexported constructor in Go"*.

An unexported constructor is not sufficient on its own — an unexported
constructor still leaves the exported struct's zero value constructible
(`var o objectmodel.Object` compiles anywhere). This implementation uses the
stronger available construct: an interface with an unexported marker method,
which no external package can implement. Verified by a compile-fail artifact
(`go/inv032/testdata/forge/`).

One hole remains and no Go construct closes it: the **nil interface value**.
`var o objectmodel.CanonicalObject` is legal in any package. It cannot carry
forged *data* — there is no object behind it — so it is a liveness rather than
an integrity problem, and `module.Execute` rejects it at runtime.

**Suggestion.** Reword INV-032's Go clause from "an unexported constructor" to
"an interface with an unexported method, or an unexported concrete type", and
state that the nil case is a runtime check. Rust's `compile_fail` newtype has
no equivalent hole, so the two implementations are not equally strong here —
worth saying in the spec rather than discovering at review.

---

## What the defects do not include

Three things the job brief flagged as suspicious were examined and found sound:

- **§4.3's `list` row** ("a mapping at a list position is an envelope"). The
  table is per-position, and a list *element*'s position is typed by `item:`,
  so element mappings are discriminated by the `object` row. A list payload can
  never be a mapping, so the row is correct as written. No defect.
- **INV-022 and `role`** ("every node carries `shape`, but `role` only where
  declared"). Consistent: `shape` is declared at every position and `role` only
  in `materialization/013`. The real inconsistency is the root node — SD-004.
- **INV-025 with divergent per-operation `inherit`.** Materialization records
  both operations independently, so divergence is representable and
  `materialization/013` passes. The problem is not representation but
  resolution, recorded as SD-012.

---

## SD-017 — INV-033 is unsatisfiable inside the closed node grammar

> **FIXED in model 0.2.** INV-033 restated — the version belongs to the hand-off, matching what INV-034 already said.

| | |
|---|---|
| **Where** | `SPEC.md` §11 (INV-033, INV-034), §2.1 (INV-001…INV-004), §6.1 (INV-021) |
| **Severity** | **blocking** |
| **Found by** | orchestrator review, not the implementation job |
| **Relation** | root cause of SD-004; SD-004 describes the symptom |

INV-033: *"Every canonical CIC object MUST carry the model version it conforms
to."*

**Try to satisfy it.** The version has to go somewhere in the object:

```yaml
values:
  mtu: {…}
origin: [yaml]
cic:                    # added to satisfy INV-033
  model: "0.1"
```

Now look at what was built. `cic` is not `values`, not `origin`, and not one of
the eight atoms. By §2.1 and INV-021 this **is not a CIC node** and must be
rejected.

So: **to satisfy INV-033 you must produce something that is no longer a CIC
object — but INV-033 is a statement about CIC objects.** The set of objects
that satisfy it is empty. This is not "underspecified"; it is unsatisfiable.

### Why the corpus is a symptom, not the fault

Writing the vectors left exactly two options, both violations: omit the version
(violate INV-033), or include it (violate INV-021). The corpus chose the second
and put `cic` on the root node. That is what SD-004 measures. The corpus was not
careless — **the specification forced it.**

### The three assumptions that close the set

1. The node grammar is **closed** — §2.1 plus INV-021.
2. The primitive set is **eight, and this specification does not change it** —
   §6.1, D-003.
3. Every non-`values` member **describes the payload's semantics** (§2.1). A
   model version does not.

Relaxing any one makes INV-033 satisfiable, and each is expensive:

- Relaxing (1) forfeits the property the whole model is built on — that every
  path is addressable, hashable, and governable, because there are no special
  members.
- Relaxing (2) means a ninth atom. D-003 fixes eight, and §6.2 rejects a ninth
  for `origin` on structural grounds — the same argument applies here.
- Relaxing (3) asserts that the model version is part of the payload's
  semantics, which is false.

### INV-033 and INV-034 already disagree

INV-034: *"A module MUST declare the model version it consumes, and a host MUST
NOT hand a module an object of a version the module has not declared."*

Here the version is a property of the **hand-off** between host and module.
INV-033 makes it a property of the **object**. Two invariants, one fact, two
different homes.

### Suggestion

Restate INV-033 so the version belongs to the serialization/hand-off frame,
consistent with INV-034 — or drop it and let INV-034 be the only version
invariant. If a document-level version carrier is still wanted, it must be
**defined**: named, placed, and stated to be outside the node grammar. Not
introduced by an example, which is how `cic` entered the specification: it
occurs exactly once in `SPEC.md`, in the code block under INV-033, and is never
defined as a construct.

**Until this is decided, SD-004 cannot be fixed** — any implementation must put
the version somewhere, and the grammar has nowhere to put it. A second
implementation would hit the same gap and reproduce the same non-conformance,
which would waste the guarantee that having two implementations exists to
provide.

---

## SD-018 — §11's release rule made 0.1 unfixable during bootstrap

> **FIXED in model 0.2.** INV-038 requires every implementation that exists, not two.

| | |
|---|---|
| **Where** | `SPEC.md` §11 (0.1 text) |
| **Severity** | **blocking** (process, not object) |
| **Found by** | orchestrator review, while planning the 0.2 revision |

0.1 §11: *"any change to a normative statement in this document is a version
increment and MUST arrive in a single change together with its conformance
vectors and **both implementations**."*

Fixing any of the seventeen defects above is a normative change, so the rule
required both implementations to ship with the fix. Only the Go implementation
existed. Writing the Rust one first would have meant implementing the
known-defective 0.1 — and the reason to have a second implementation is
independent corroboration, which is worth little against a text already known
to contradict itself in six places.

So: the specification could not be fixed until a second implementation existed,
and the second implementation should not be written until the specification was
fixed.

**Why it could not be worked around.** Ignoring the rule to fix the rule is
exactly the move the rule exists to prevent, and doing it silently would set the
precedent that §11 is advisory.

**Root cause.** The rule was written for the steady state — its own stated
purpose is to make it *"physically awkward to change one implementation's
semantics without the other and the corpus noticing"*. That purpose is served by
requiring the implementations that exist. Requiring a fixed number of them
mistakes the count for the property.

**Suggestion (applied in 0.2).** State the requirement against the
implementations this repository ships. INV-038 does that, and adds the half the
0.1 sentence left implicit: a normative change must not be split from its
vectors across releases either.

---

## SD-019 — INV-032's type-level guarantee is defeatable by interface embedding

**Still open, and narrowed.** The type-level guarantee cannot be restored in Go;
it is a property of the language. What has changed is what an embedded forgery
can accomplish with it.

Before: a forged value pairing a REAL node tree with a different but perfectly
valid byte string crossed the boundary. The tree was real, the bytes validated,
and the only lie was the pairing — so a module reading the tree and one reading
the serialization were told different things by the same object. That was audit
finding F-02, and it was never built until now; the earlier adversarial tests
paired a real tree with INVALID bytes, which made the boundary look stronger
than it was.

After: the boundary re-serializes the tree and compares
(`module.ErrObjectNotBound`), so the two views must describe the same object.
This was not possible before §8.8.1: with the serialization undefined, a
re-serialization differing from the original was nobody's fault, and the check
would have rejected every honest object.

What remains is an object assembled elsewhere whose tree and bytes are mutually
consistent and which never went through materialization. Every check the
boundary can make passes, because there is nothing left to catch it with short
of the type system Go does not offer here. The Rust implementation has no such
hole: `CanonicalObject` is a struct with private fields and `materialize` is its
only constructor, so the forgery does not compile.

The original entry follows.


| | |
|---|---|
| **Where** | `SPEC.md` §9 (INV-032) |
| **Severity** | **blocking** (security) |
| **Found by** | `go/module/adversarial_test.go`, by attacking the claim rather than asserting it |

INV-032: *"the module input type is constructible only by the materializer."*
The Go implementation enforces this with an unexported marker method on
`CanonicalObject`, on the reasoning that no other package can implement an
interface it cannot name a method of.

**Go promotes an embedded interface's method set, including unexported
methods.** So this compiles, in any package:

```go
type forged struct {
	objectmodel.CanonicalObject   // embedded, nil
}
var obj objectmodel.CanonicalObject = forged{}   // satisfies the type
```

Three forgeries, each defeating one more defence:

| Forgery | Result before the fix |
|---|---|
| empty embedding | satisfies the type; **panics** the boundary on the first method call |
| every method overridden, `Root()` nil | rejected — by the nil-Root check, not by the type |
| **real node tree from a real materialization, attacker-chosen `CanonicalYAML()`** | **crossed the boundary** |

The third is the finding. Every runtime check passed: non-nil object, non-nil
root, truthful-looking version. The bytes a consumer would read violated INV-017
(an origin holding both `yaml` and `schema`) and no materializer ever produced
them.

**Why it cannot be fixed as stated.** Interface embedding is a language
property. No arrangement of unexported methods, sealed interfaces or build tags
closes it: any package that can name the type can embed it. An unexported
*struct* type returned as a concrete type would close it, but then the boundary
could not be an interface at all, and modules could not be written against it.

**What was done instead (go/module/module.go).** The boundary stops trusting the
type and re-establishes the property that matters — that what a module READS has
been validated:

- it re-runs `ValidateCanonicalDocument` on the bytes, one parse per delivery
- it recovers from panics, because a crash reachable from a module author is a
  denial of service and worse than a rejection

That closes all three forgeries. It does not make INV-032 true.

**Suggestion.** Restate INV-032 to say what is achievable and what the boundary
must therefore do. Something of the shape: the module input type MUST NOT be
constructible by ordinary means, AND a boundary MUST NOT rely on the type alone
— it MUST validate what it is handed and MUST NOT be crashable by it. An
invariant that a conforming implementation cannot satisfy is worse than a weaker
one it can, because the first teaches implementers that invariants are
aspirational.

Note for the Rust implementation: a private-field newtype in Rust genuinely is
unconstructible outside its module, so Rust can satisfy the strong form where Go
cannot. That asymmetry belongs in the specification rather than in a surprise
during review — the two implementations will not be equally strong here, and
`docs/spec-vector-map.md` already hints at it without saying so.
