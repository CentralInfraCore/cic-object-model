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

## Where the trust boundary is

Three of the decisions below turn on this, and it was not written down
anywhere, so it is written down here first.

`Materialize` **is** the pre-entry step. It validates authoring input against
the schema descriptor and injects the declared defaults, marked — the mark is
`origin`: `[schema]` for a value the model supplied, `[yaml]` for one the author
wrote. What crosses the module boundary afterwards is the complete extracted
set, which is what INV-031's seven prohibitions describe.

Two consequences that read the wrong way without it:

- **`Materialize` and `ValidateCanonicalDocument` are the untrusted edge.** The
  schema validation is not something that happened before them; it is what they
  are. So the alias bomb and the quadratic scan were real threats at exactly
  that point, and resource budgets belong there — not at the module boundary,
  where the set is already complete.
- **A module never sees an incomplete object.** So "a declared position with no
  value" is not a state the boundary can be in. It is a pipeline failure, and
  D-2 follows from that rather than from a preference for strictness.

---

## D-1 — Does `scalar_type` constrain a value, or describe it?

> **DECIDED: constraining, with no coercion.** Part of D-5 rather than separate
> work, and a prerequisite for it.
>
> Not for symmetry. A contract cannot be evaluated without a type: `range: [1,
> 100]` against the string `"50"` has no answer. If `scalar_type` stays
> descriptive, the contract evaluator invents its own typing and the model has
> TWO type systems — one declared and unchecked, one implicit — and when they
> disagree the rejection arrives at the wrong stage naming the wrong thing.
>
> No coercion: `9000` is an integer, `"9000"` is a string, and
> `scalar_type: integer` refuses the second. Silent coercion in a provenance
> model is the same failure class as leaving `on`/`off` unquoted.
>
> Sequencing: `scalar_type` enforcement lands BEFORE contract evaluation. The
> other order does not build.

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

> **DECIDED: rejection.** Every declared position has a value or a default.
>
> This is not a preference for strictness; it follows from the trust boundary
> above. A complete extracted set crosses into a module, so "a declared position
> with no value" is not a state that can exist there. Either the pipeline
> produces a value or it cannot produce the set, and the second is an error.
>
> **And `required` keeps a meaning, a sharper one.** It stops being about
> whether a value exists on the output — one always does — and becomes about
> where it may come from:
>
>   `required: true` = the INSTANCE must supply it; a schema default does not
>   satisfy it.
>
> Which is expressible in the object itself: a required member may not carry
> `origin: [schema]`. The keyword is not removed.
>
> Refused: **omit** (Go's behaviour) leaves a reader unable to tell "not
> configured" from "not declared" without the schema. **Null** (Rust's) makes
> every optional position a node holding a value nobody wrote, and `origin` has
> no term that is true of it.

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

> **DECIDED: enforce, and enforce `contract` with it.** The document argued
> these are one question; they are answered as one.
>
> `access` gets a closed member set and typed values in §6.4. `contract` gets an
> evaluator — the primitive §8.7 already requires to be enforced and which
> nothing has ever read. This is the largest single piece of work in the
> repository: a grammar, an evaluator, both implementations, and a vector family
> for each.
>
> Refused: declaring both unenforced was cheap and honest, and would have left
> two primitives that describe guarantees the model does not provide. The
> current state — a MUST in the specification with no code behind it — is worse
> than either, because it looks like a guarantee.
>
> Depends on D-1, which is the type foundation a contract evaluates against.

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

> **DECIDED: restrict declared names; opaque payload keys are exempt and not
> addressable.**
>
> Schema-declared names take a declared character set, and a schema declaring
> `a.b` becomes a schema-load rejection. Because names arrive from a compiled,
> validated schema descriptor, the restriction is enforceable upstream too and
> the model only has to state it.
>
> An opaque payload's keys are chosen by a foreign system, not by the schema,
> so they stay unrestricted — and §2.5 will say that INV-040 covers the
> MODEL's nodes, not the keys of data carried verbatim. Nothing references into
> an opaque blob: not evidence, not policy. That is what makes the exemption
> free rather than a hole.
>
> Refused: **escape in the address grammar** keeps every name addressable and
> costs a second byte-level rule to pin and reconcile across two
> implementations — §8.8.1 took a day. **Restricting opaque keys too** would
> make §4's "carried verbatim" false for a config blob the model does not even
> interpret.

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

## D-13 — Is `make ci` the gate, or part of it?

**Measured:** `README.md` says a green badge and a green local run mean the same
thing. They do not. CI adds `review.check` for pull requests into `main`, which
`make ci` deliberately does not run. And the environment is not fixed:
`Dockerfile` builds `FROM python:3.11-slim` — a moving tag — installs apt
packages without versions, and downloads the Go toolchain over the network with
no checksum in the repository. `mk/rust.mk` installs `cargo-llvm-cov` and
`cargo-deny` unpinned at run time. The Rust base image is the one thing pinned
by digest.

**Accident on both halves.** The review step was added deliberately and the
README sentence was not revisited; the base image and toolchain were inherited.

| option | cost |
|---|---|
| **Say "shared core gate"** and add a local target that runs what a main-PR runs | free, and honest; the equivalence claim shrinks to what is true |
| **Pin everything** — digest the Python base, version the apt set, checksum the Go tarball, pin the cargo tools | a reproducible build; every pin becomes something to update |
| **Both** | the only combination under which "the same pipeline" is a statement about bytes rather than about command names |

**Blocks:** any argument that a green CI run is evidence about a specific tree
rather than about a tree built from whatever the network served that day.
**Related:** audit claim F-08.

---

## D-14 — What does `TestVersionIdentity` promise?

**Measured:** `README.md` says it holds *every other declaration* in the
repository to `SPEC.md`. It checks two Go constants, two fields of
`spec/index.yaml`, the major and minor of `project.yaml`, and every vector
schema. It contains **zero** references to `rust/`, which has its own
`MODEL_VERSION` and its own `Cargo.toml` version. Those agree today; nothing
makes them.

**Accident**, and a familiar one: the test was written when there was one
implementation, and its claim was true then.

| option | cost |
|---|---|
| **Enumerate the Rust declarations too** | one edit; the list grows by hand every time a declaration site appears, which is the failure mode the test exists to prevent, one level up |
| **Discover declaration sites** — scan for a registered pattern across the tree | the claim becomes true rather than maintained; needs a convention for what counts as a declaration |
| **Narrow the README** — name what the test checks | free, and leaves the Rust side unguarded by design rather than by oversight |

**Related:** audit claim F-10.

---

## Status

**Four decided, ten open.**

| decided | choice |
|---|---|
| D-1 | `scalar_type` constrains, no coercion — part of D-5 |
| D-2 | rejection; `required: true` means the instance must supply it |
| D-5 | enforce `access` AND `contract` |
| D-6 | restrict declared names; opaque keys exempt and not addressable |

D-1, D-2 and D-6 are cheap to implement and change both implementations. D-5 is
a sprint: a grammar, a contract evaluator, two implementations, two vector
families — and D-1 has to land first, because a contract is evaluated against
a type.

Recommendations exist for the ten still open and are not decisions. D-4
(`[schema]` for an injected default), D-8 (make the stages and codes normative),
D-12 (replace the release path) and D-13 (pin the build, narrow the `make ci`
claim) look to me like they have one defensible answer each; D-3 follows from
D-5 once the grammar exists; D-7, D-9, D-10, D-11 and D-14 need choosing.

## How these get decided

Not by me and not by an implementation. Each one is settled by an entry in
`SPEC.md`, a vector that holds both implementations to it, and a line here
saying which way it went and why the alternatives were refused.

Until then, every one of them is answered — by accident, in code, differently in
two places in at least three of the cases above.
