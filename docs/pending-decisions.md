# Pending decisions

Questions this specification does not answer, where an implementation has
answered them anyway.

That is the distinction worth holding onto. A defect is a place where the
document says something wrong; these are places where it says **nothing**, and
something had to happen, so whatever the first implementation did became the
answer. Nobody chose it. It is not written down. It is load-bearing.

`docs/spec-defects.md` records the first kind. This file records the second.

Each entry states the question, what happens today and whether that is a
decision or an accident, the options with what each costs, and what it blocks.
None of them is fixed by writing more code: they are settled by choosing, and
then by a vector that holds both implementations to the choice.

Sources: the three external review threads against `cbaf928`, recorded in
[`../reviews/`](../reviews/). Findings closed since are in the commit history;
what remains is here.

---

## D-1 — Does `scalar_type` constrain a value, or describe it?

**Measured:** neither implementation reads `scalar_type` during validation. It
is parsed, carried, and emitted into the `shape` primitive, and nothing compares
it to the payload. `scalar_type: integer` with a string value materializes and
validates.

**Accident.** The arity check added after the audit refuses a *collection* at a
scalar position; it deliberately did not touch subtype, because answering that
question in an emitter is how a specification acquires rules nobody wrote.

| option | cost |
|---|---|
| **Constraining** — the payload must match | needs a type lattice (what is `integer` for a YAML `1.0`? for `0x10`?), coercion rules or their explicit absence, and a vector per type |
| **Descriptive** — an annotation the model carries and does not check | cheap and honest, but then `shape.scalar_type` in a canonical object is a claim no one verified, and a module reading it is trusting the author |

**Blocks:** any consumer that treats a canonical object as typed data.
**Related:** audit semantic F-04, claim F-11.

---

## D-2 — What happens to a declared position that is absent, optional, and has no default?

**Measured:** Go removes the node. Rust materializes a scalar as `null`, a list
as empty, and walks object children. `docs/spec-defects.md` SD-007 already
records three readings — omit, null, reject — and calls none of them decided.

**Accident, and a divergence the corpus cannot see:** no vector has an absent
optional position.

| option | cost |
|---|---|
| **Omit** — the node does not exist | matches Go; a reader cannot distinguish "not configured" from "not declared" without the schema |
| **Materialize as null** | matches Rust; every optional position becomes a node with a value nobody wrote, and `origin` has no term for that |
| **Reject** — every declared position needs a value or a default | strictest and simplest to state; makes optionality a schema error rather than a runtime state |

**Blocks:** cross-implementation agreement outside the corpus.
**Related:** audit semantic F-05, claim F-03.

---

## D-3 — May instance input author a primitive the schema declares, and if so, does it replace or merge?

**Measured:** vector `011_discriminator_envelope` supplies an `access` primitive
from the instance and it REPLACES the declaration. Nothing states whether that
is allowed in general, and nothing states what happens when both the schema and
the instance declare parts of the same primitive.

**Accident.** One vector fixes one case; the rule is unwritten.

| option | cost |
|---|---|
| **Forbidden** — primitives come from the schema only | INV-036 nearly says this already; would invalidate vector 011 as written |
| **Replacement** — the instance value wins whole | what happens today for the one case that exists; simple, and loses schema-declared members silently |
| **Merge** — leaf-wise override | most useful, most to specify: merge order, list handling, and what `origin` says about a merged node |

**Blocks:** D-4, because a merged primitive's origin is undefined.
**Related:** audit semantic F-01.

---

## D-4 — What origin does an injected primitive-internal default carry?

**Measured:** `inherit` is injected into every access operation that does not
declare it, and it takes the origin of the *enclosing* primitive — `[yaml]` in
vector 011, `[schema]` in 013. So a value the YAML did not contain is recorded
as yaml-authored.

**Accident, and it makes an origin false.** `origin` is a claim about who
authored a value; the author did not write this one.

| option | cost |
|---|---|
| **Enclosing origin** — today's behaviour | keeps the object simple; the provenance of an injected default is wrong, which is the one thing origin exists to be right about |
| **Always `[schema]`** | truthful — the model supplied it — and makes a node's origin differ from its parent's inside one primitive, which nothing else does |
| **Do not inject** — absence means the default | nothing to attribute; a policy decision point must then know the default, which §6.4 says it should not have to |

**Related:** audit semantic F-02.

---

## D-5 — Is `access` a grammar or a shape?

**Measured:** the operations are checked (`read`, `modify`, and `write` is
refused). Nothing else is: `inherit` may be any value, an operation block may
carry any member, and `rules` may be anything at all.

**Accident.** INV-024 through INV-026 describe value domains in prose that no
code reads.

| option | cost |
|---|---|
| **Enforce** — closed member set, typed values | the primitive becomes a real contract; needs a grammar in §6.4 and a vector family |
| **Leave open** — `access` is a shape the model carries | honest, cheap, and means a policy decision point cannot rely on anything about its content |

**Related:** audit semantic F-03.
**Note:** this is the same shape of question as **F-08 of the first audit** —
the `contract` primitive, which §8.7 requires to be enforced and which nothing
evaluates. Both are "a primitive the specification describes and no code
checks". They should be decided together or the answer will not be consistent.

---

## D-6 — What characters may a node name contain?

**Measured:** a child named `a.b` materializes. `Path()` returns
`$.values.a.b`, and `Get` splits it into two segments, so the address does not
round-trip. Key quoting was fixed for serialization; addressing was not, and the
round-trip test in `addressing_test.go` passes only because no vector uses such
a name.

**Accident, and INV-040 is false for it.** Every node has exactly one address —
except these, which have none that resolves.

| option | cost |
|---|---|
| **Restrict names** — a declared character set | simplest; a schema with `a.b` becomes a schema-load error; opaque payload keys stay unrestricted and unaddressable, which needs saying |
| **Escape in the address grammar** | keeps names free; every address becomes harder to read and both resolvers need the escape |
| **Length-prefixed or quoted segments** | unambiguous, and no longer looks like a path |

**Blocks:** INV-040 being true as stated.
**Related:** audit claim F-04, semantic F-09.

---

## D-7 — Which YAML is the input dialect?

**Measured:** Go uses `gopkg.in/yaml.v3`, Rust uses `saphyr`. Anchors, aliases,
duplicate keys and non-string keys are now refused explicitly, in both, because
each was found to diverge. Tags, `!!binary`, and version directives are not
mentioned anywhere and are handled by whatever each library does.

**Accident.** Three divergences in this area have been found by review; the
remainder is untested rather than agreed.

| option | cost |
|---|---|
| **Name a version and a tag policy** — e.g. YAML 1.2 core schema, no tags | states the contract; both implementations need checking against it |
| **Define an input profile** — an explicit subset the model reads | strongest, and the most work: every construct outside it must be refused by name |

**Related:** audit semantic F-11.

---

## D-8 — Are the pipeline stage names and error codes normative?

**Measured:** every rejection carries a stage and a code, the vectors assert
them, and **§8 does not list them**. `schema-load` is not a stage the
specification names at all (`docs/spec-defects.md` SD-002). The corpus is the
only place the vocabulary exists.

**Accident.** A second implementation must match strings it can only learn by
reading fixtures.

| option | cost |
|---|---|
| **Make them normative** — §8 lists stages, an appendix lists codes | a table, and then the vectors check the specification rather than defining it |
| **Declare them non-normative** — diagnostics, not contract | then the vectors must stop asserting them, and a caller cannot branch on a code |

**Related:** audit semantic F-13.

---

## D-9 — What does INV-032 require of a language that cannot express it?

**Measured:** Go cannot satisfy the construction guarantee — an external struct
embedding the interface satisfies it, which the repository documents in SD-019
and defends against at runtime. Rust satisfies it structurally. The invariant is
stated as a property of the type system, so one shipped implementation does not
meet the normative text.

**A defect the repository already admits, and a decision it has not made.**

| option | cost |
|---|---|
| **Restate as boundary behaviour** — validate, bind representations, snapshot once | achievable in both; loses the "unforgeable by construction" claim that made INV-032 interesting |
| **Keep the strong form and mark Go non-conformant on it** | honest; means the reference implementation fails a normative requirement, in writing |
| **Two levels** — a required boundary contract, plus a stronger construction property where the language allows | more text, and describes what is actually true |

**Related:** audit claim F-05, semantic F-14; SD-019.

---

## D-10 — What does the corpus mapping gate actually promise?

**Measured:** `check_spec_vectors.py` builds its universe by matching `INV-\d+`
tokens after the §12 heading. It does not read RFC 2119 keywords — zero
occurrences of `MUST` in the tool — so an unnumbered `MUST` cannot enter
coverage at all, and §8's stage requirements are full of them. It also accepts
any non-empty justification cell without checking that the alternate check
exists. No test exercises it.

**Accident.** The README calls it "negative-tested"; nothing under `tests/`
references it.

| option | cost |
|---|---|
| **Require every RFC-2119 clause to belong to an invariant** | makes the gate's promise true; a large editing pass over §8 |
| **Parse clauses directly and track them** | no editing pass, a harder tool, and a coverage number that finally means what it says |
| **Narrow the claim** — the gate checks the invariant index and says so | free, and leaves unnumbered MUSTs untested by design rather than by accident |

**Related:** audit claim F-06.

---

## D-11 — What are the resource budgets?

**Measured:** none, at any public entry point. No limit on input bytes, node
count, nesting depth, output bytes, time or memory. The only bound is template
expansion depth (64, now in both). Two amplification findings have been closed —
alias expansion and a quadratic scan — and both were found by review rather than
by a budget refusing them.

**Accident.** The adversarial review marked recursion depth, memory and output
"inconclusive, not defended" because it could not execute; the absence of any
budget is not inconclusive.

| option | cost |
|---|---|
| **Per-entry-point budgets** — bytes in, nodes, depth, bytes out | bounded work, and every limit becomes a number someone has to defend |
| **Depth only** — the recursive walks are the crash risk | cheapest real improvement; says nothing about a wide-and-shallow input |
| **Document the absence** | free, and moves the risk to every caller |

**Related:** audit adversarial, resource section.

---

## D-12 — Does `make release` implement INV-045?

**Measured:** it does not. The release path hashes one canonical source file,
clears `buildHash`, expects a build artifact this repository does not produce,
and signs a five-field metadata subset. `release_subject.py` — which computes
the INV-045 subject — is not called by it. The descriptor schema was corrected
to accept `repo_type: spec`, but nothing runs that validation either.

**A gap, not an accident:** the release machinery is inherited from a template
for a different kind of repository.

| option | cost |
|---|---|
| **Replace the path** — one command that freezes, computes, verifies, review-binds and signs the subject | the honest fix; discards inherited machinery that does not fit |
| **Wrap it** — keep the Vault chain, feed it the subject digest | less to write, keeps a state machine nobody here needs |

**Blocks:** producing a release that demonstrably satisfies INV-045.
**Related:** audit claim F-02.

---

## How these get decided

Not by me and not by an implementation. Each one is settled by an entry in
`SPEC.md`, a vector that holds both implementations to it, and a line here
saying which way it went and why the alternatives were refused.

Until then, every one of them is answered — by accident, in code, differently in
two places in at least three of the cases above.
