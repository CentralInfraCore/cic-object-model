# CIC Object Model — Normative Specification

**Model version: 0.2**
**Status: normative, two implementations.** The reference implementations in
`go/` and `rust/` both execute the conformance corpus and produce byte-identical
canonical objects. 0.2 is the revision that follows from running it: eighteen
defects were found by implementing 0.1, and the ones that made 0.1
unsatisfiable are fixed here. The second implementation, and two external
reviews, found the rest. See `docs/spec-defects.md` for the full
list and [Conformance](#10-conformance) for what corpus status means for the
reader.

The key words MUST, MUST NOT, REQUIRED, SHALL, SHALL NOT, SHOULD, SHOULD NOT,
MAY and OPTIONAL in this document are to be interpreted as described in
RFC 2119.

This document is the authority. The `go/` and `rust/` implementations are
subordinate to it: where an implementation disagrees with this document, the
implementation is wrong. Where this document disagrees with itself, that is a
defect to be reported, not resolved by an implementer's judgement.

---

## 1. Scope and purpose

The CIC object model defines the single semantic form in which infrastructure
objects are represented between the point where a human (or an agent) authors
YAML and the point where a provisioner module receives an object to act on.

It exists to remove interpretation from every layer below the schema. A module
does not decide what an input means, whether a default applies, whether a
reference resolves, or who may write a field. All of that is decided before the
module is reachable, by the pipeline in §8, and the result is the only thing a
module can be handed (§9).

**Out of scope for 0.2:** the CertPattern matching algorithm, the template
repository/resolution protocol, the external reference syntax and its
resolution (§8.2), the runtime policy-decision point, and **the resolution of
`inherit` chains** (§6.4). This document specifies the object model and its
materialization, not the systems that consume it.

Two of those exclusions are load-bearing enough to state plainly, because 0.1
carried obligations it could not meet:

- **`inherit` is recorded, not resolved.** §6.4 defines what the tri-state
  *means*; resolving a chain requires the policy-decision point, which is out of
  scope. A canonical object therefore carries `inherit` verbatim. An
  implementation that resolves it is not more conformant — it is inventing
  semantics this document declines to define.
- **The model version is not part of the object.** It belongs to the frame that
  hands an object over (§11). 0.1 required the object to carry it, which the
  node grammar of §2.1 has no room for; see `docs/spec-defects.md` SD-017.

---

## 2. The node model

### 2.1 Definition

A **CIC node** is the single structural unit of the model. Every node has the
same grammar, at every depth, regardless of what it describes.

```
CICNode :=
{
    values : Value          # REQUIRED — the payload
    origin : Origin         # REQUIRED in canonical form — authoring authority
    <primitive> : CICNode   # zero or more, one per declared primitive
}

Value :=
      Scalar                # string | integer | number | boolean | bytes | null
    | List<CICNode>
    | Map<String, CICNode>
    | Opaque                # terminal; see §7
```

`values` is the payload. Everything else on the node describes the payload's
semantics. This is the whole model.

**INV-001** — Every CIC node MUST have exactly one `values` member.

**INV-002** — Every CIC node in canonical form MUST have exactly one `origin`
member.

### 2.2 Why it is recursive

The members that describe the payload — `shape`, `role`, `contract`, `access`,
and the rest of §6 — are **not metadata fields attached to the node**. Each is
itself a CIC node, with its own `values`, its own `origin`, and potentially its
own primitives.

This is what makes `$.values.network.values.mtu.access.values.read` a
first-class, addressable object rather than a path into a YAML blob. It can be
hashed, diffed, and referenced from evidence — because it is a node like any
other.

The address is written out in full because §2.5 makes it exact. 0.1 wrote this
example as `network.values.mtu.access.read`, which is not an address any
implementation emits or resolves — the form was prose, and prose is where the
`cic` construct also lived (SD-017).

**The recursion does not stop at the primitive.** `access` being a node is not
enough for the claim above: the `read` inside its payload must be a node too,
and so must everything the schema declares beneath it. Otherwise that address is
exactly the path into a YAML blob that this section says it is not. 0.1 stopped one level early and the corpus recorded
primitive payloads as raw mappings; see `docs/spec-defects.md` SD-003. INV-027
(§7) is what carries this all the way down, and it applies inside primitive
payloads exactly as it applies inside `values`.

**INV-003** — Each primitive member of a node MUST itself be a CIC node
satisfying INV-001 and INV-002.

**INV-035** — Every schema-declared child of a primitive's payload MUST itself
be a CIC node. A primitive's payload is not exempt from INV-027.

**Nodehood follows declaration, not data shape.** A position becomes a node
because something declares it as a position — a schema child, an `item:` shape,
or a key of a primitive's fixed structure (§6.4). It does not become a node
merely by being a mapping or a list element.

Two cases that look alike and are not:

| | Declared? | Result |
|---|---|---|
| `addresses` with `item: {shape: scalar}` | the element position is declared | each element is a node (`materialization/008`) |
| `access.read.rules.operator.subjects: [...]` | the list is declaration content; no element position is declared | `subjects` is a node, its entries are payload values |

The second is deliberate. `subjects` is a CertPattern list whose matching
algorithm is out of scope (§1), and its entries have no stable identity to
address: `subjects[0]` is a position, not a name. §6.4 gives rules names for
exactly this reason — `rules.operator` survives a reordering, `rules[0]` does
not. Making anonymous list entries into nodes would hand out addresses that
evidence must not rely on.

*(Numbered 035 rather than inserted here: conformance vectors reference
invariants by number, so the existing numbering is load-bearing and is never
renumbered. New invariants are appended.)*

### 2.3 Why the recursion terminates

INV-003 as stated is non-terminating: if `access` is a node, it has an `origin`;
if `origin` were a node, it would have an `origin`, and so on without end. The
model is only implementable because two things bound it.

**INV-004** — `origin` is terminal. An `origin` member MUST NOT itself carry
`values`, `origin`, or any primitive. It is a value in the grammar of §5, not a
node.

**INV-005** — Primitive materialization MUST terminate, and in 0.2 it
terminates **because the schema is finite**. A schema is a finite literal tree:
it declares a finite set of primitives per node, each of whose declarations is
itself finite. An implementation MUST reject a schema it cannot walk to a leaf
in finitely many steps.

A **repeated primitive name** on a declaration path is not a cycle and MUST NOT
be rejected. Under INV-035 a primitive's payload is materialized like any other
structure, so a path such as
`mtu.access.read.contract.rules.guard.access` is a legitimate finite schema: the
inner `access` hangs from `guard`, not from `mtu`. 0.1 relied on name-acyclicity
instead, which was both unfalsifiable in the schema language of the day and
strictly stronger than termination requires (`docs/spec-defects.md` SD-005).

*Forward note, not a requirement of 0.2:* the schema language has no references
(§8.2 is out of scope). If it gains them, finiteness stops being structural and
a genuine cycle check over the declaration graph becomes necessary. That check
belongs with the feature that creates the need for it.

INV-004 is the reason `origin` is not a ninth atom (§6.2): it is the fixed
point of the recursion, and a fixed point cannot be a member of the set it
terminates.

### 2.4 What is not on the node

`descr` / `description` is **not** part of the instance node. Documentation is a
property of the schema, not of a value instance; carrying it on every
materialized node would duplicate schema knowledge into every object.

**INV-006** — A canonical CIC node MUST NOT carry a documentation member.
Documentation MUST be obtained from the schema.


### 2.5 Addressing

The model's central claim is that every declared position is addressable. That
claim needs an address grammar, and 0.1 had none: §2.2 wrote addresses one way in
prose, implementations emitted another, and nothing said which was right.

An address names one node, and one node has exactly one address.

```
address := "$" step*
step    := "." member          a member of the node
         | ".values" "[" i "]" the i-th entry of a list payload

member  := "values"            step into the payload
         | "origin"            the node's origin (terminal, INV-004)
         | <primitive name>    a materialized primitive (§6.1)
         | <payload child>     only directly after a `values` step
```

**INV-040** — Every node in a canonical object MUST have exactly one address
under this grammar, and an implementation that reports an address MUST report
that one. An address that resolves to a node MUST resolve to the same node in
every implementation.

Reading it: `values` is the step into the payload, and it is never optional.
Anything else on a node is a member of the node itself.

```
$.values.mtu                          the root payload's `mtu` child
$.values.mtu.access                   `mtu`'s access PRIMITIVE
$.values.mtu.access.values.read       the access payload's `read` child
$.values.addresses.values[0]          the first entry of a list payload
$.values.config.values.values         a payload child literally named `values`
```

The last line is why the step is mandatory. A schema may not declare a child
named `values` (INV-010), but an *opaque* payload may contain one, and a
grammar where the step is optional cannot tell the two apart. It is also what
separates a primitive from a payload child that shares its name: `…mtu.access`
is the primitive, `…mtu.values.access` is a child called `access`.

The verbosity is the price of `$.values.values.values.mtu` and `$.values.mtu`
not being two names for one node. In a model whose purpose is that evidence can
reference a position, an address that is not unique is worse than a long one.

### 2.6 What the serialization may not do to an address

INV-040 makes an address unique in the *model*. Two things in the serialization
can defeat that before the model ever sees the document, and both are left to
the YAML library unless this specification says otherwise — which, until 0.2,
it did not.

**INV-041** — A mapping MUST NOT declare the same key twice. A document that
does MUST be rejected, at the stage that read it.

The same name at different addresses is not a duplicate: `a.a`, two sibling
mappings that each declare `x`, and two sequence entries that each declare `x`
are four distinct addresses and all are legal. What is forbidden is one address
written twice.

YAML itself does not settle this. Of the two implementations of this document,
one rejected such a mapping and the other silently kept the last value — for a
commit, undetected, because no vector contained a duplicate and the two were
assumed to agree. One address with two values has no single answer, and a model
that resolves it by position in the file is deciding provenance by luck.

**INV-042** — A document MUST NOT contain a YAML anchor or alias. A document
that does MUST be rejected, at the stage that read it.

Two reasons, and the second is the one that matters:

1. An alias makes one value reachable from two addresses. `origin` has four
   forms (§5.2) and none of them says "the same value as somewhere else", so an
   aliased node's provenance is not expressible in this model.
2. Alias expansion is an amplification vector. Measured on one of this
   document's implementations: **393 bytes of nested aliases composed to
   12,345,678 nodes in 2.7 seconds**, a factor of about 31,000, on input that
   is untrusted by construction. The expansion happens while the tree is being
   built, so a size limit applied afterwards is applied after the cost.

Neither invariant needs the schema, so both are enforceable wherever a document
is read — which is why they are stated here rather than in §8.

---

## 3. The two planes

The model has two planes, and most confusion about it comes from conflating
them.

| | Authoring plane | Canonical plane |
|---|---|---|
| Written by | human / agent, in YAML | the materializer, never by hand |
| Shape | partial subtree, short forms allowed | every node fully expanded |
| `origin` | MUST NOT appear | REQUIRED on every node |
| Below `values` | domain data, uninterpreted | CIC nodes (unless opaque) |
| Consumed by | the materializer | modules (§9) |

The author does not write a CIC object. The author writes a **value subtree**;
the schema plus the materializer produce the CIC object from it.

**INV-007** — Authoring input MUST NOT contain an `origin` member at any depth
outside an opaque payload. `origin` is computed, never declared. An input
containing one MUST be rejected at entry validation (§8.1).

INV-007 is what makes `origin` trustworthy: a node cannot claim its own
provenance.

---

## 4. The structural discriminator

### 4.1 The problem

In the authoring plane, `mtu: 9000` and

```yaml
mtu:
  values: 9000
  access:
    modify:
      rules:
        net-admin:
          subjects: ["OU=network-admins,O=acme"]
          effect: allow
```

must both be accepted, and something must decide whether a given mapping is a
**node envelope** or **payload**.

### 4.2 What does not work

A purely syntactic rule — "a mapping is an envelope iff it directly contains
`values` and `default`" — is **unsound in both directions**, and this
specification does not adopt it.

*False positive.* Domain payload legitimately carrying both keys is misread as
an envelope:

```yaml
checkbox:
  values: [on, off]     # a domain enumeration
  default: false        # the domain's own default
```

*False negative.* It rejects canonical nodes, which carry `values` and `origin`
but no `default` (§4.5).

### 4.3 The rule

The schema is always available during materialization. It — not key inspection
— decides the plane.

**INV-008** — **During materialization (§8.1–§8.6)**, whether a mapping is a
node envelope or payload MUST be determined by the schema-declared shape at that
position. An implementation MUST NOT decide this by inspecting keys alone.

**INV-039** — Final validation (§8.7) of an object presented **without** its
schema is structural and key-directed, and is explicitly weaker: a mapping is
treated as a node iff it carries a `values` member. An implementation MUST NOT
use this weaker rule while a schema is available.

The scoping is not a loophole, it is an admission. Truth-table rows 7 and 8
(§5.3) are unreachable from authoring input, so they can only be exercised
against an already-canonical object — and validating an object nobody handed you
a schema for leaves key inspection as the only available signal. 0.1 stated
INV-008 globally while requiring exactly that walk, so every conforming
implementation violated it on the validation path (`docs/spec-defects.md`
SD-013).

The residual risk is named rather than hidden: an **opaque** payload containing
a `values` key whose value is a mapping will be mis-identified as a node by the
weaker rule. Schema-less validation cannot distinguish the two, which is one
more reason a schema should accompany an object wherever it can.

Given the schema-declared shape at a position, for an authoring value `V`:

| Declared shape | `V` is a mapping | `V` is not a mapping |
|---|---|---|
| `scalar` | envelope (a scalar position cannot hold a map payload) | payload (short form) |
| `list` | envelope | payload (short form) |
| `object` (structured) | envelope **iff** `V` contains key `values`; else payload | payload (short form) |
| `opaque` | payload, always (§7) | payload |

**INV-009** — At a structured-object position, a mapping MUST be treated as a
node envelope if and only if it directly contains the key `values`.

**INV-010** — A schema MUST NOT declare a child property named `values` **or
`origin`** at a structured-object position. A schema that does MUST be rejected.

`origin` joins the rule in 0.2 to close a contradiction rather than to add a
restriction. INV-007 forbids an `origin` member in authoring input at any depth
outside an opaque payload; INV-011 requires a payload key named `origin` to be
preserved verbatim as domain data. With `origin` declarable, a schema could
reach the position where both applied and they disagreed — measured in 0.1,
producing a node named `origin` that carried its own `origin`
(`docs/spec-defects.md` SD-014). Reserving the name at declaration time makes
that position unreachable, which is the smaller fix: no existing invariant is
weakened, and INV-011 keeps its meaning everywhere it can still apply.

INV-010 is what makes INV-009 total: with the name unavailable to schema
authors, no structured payload can accidentally present as an envelope. This is
a constraint on *schema declaration*, not a global reserved word — `values` is
free as a key inside any payload, at any depth below a `values` member, and
inside any opaque subtree.

**INV-011** — Below a `values` member, no CIC primitive interpretation applies.
Keys named `values`, `origin`, `access`, `shape` or any other primitive name
occurring in payload are domain data and MUST be preserved verbatim.

So this input is unambiguous and entirely legal:

```yaml
interface:
  values:
    name: eth0
    access: customer      # domain data — INV-011
    values:
      foo: bar            # domain data — INV-011
  access:                 # CIC primitive — envelope level
    read:
      rules:
        ops:
          subjects: ["OU=operators,O=acme"]
          effect: allow
```

`interface.access` is a primitive; `interface.values.access` is a string.

### 4.4 `values` is plural because the payload is uniform

`values` holds the payload whether it is a scalar, a list, or a map. There is no
separate `value` / `values` distinction by arity — one member, one name, always.

### 4.5 `default` is not part of the model

The authoring-plane marker `default: true` — "this value came from a default"
— is **superseded by `origin`**. `origin: [schema]` states exactly that, and
states it more precisely (it distinguishes a schema default from a template's
schema default, which a boolean cannot).

**INV-012** — A canonical CIC node MUST NOT carry a `default` member. Value
provenance MUST be expressed through `origin`.

A schema still declares defaults; `default` is a schema-side declaration, not a
node member. The consequence is recorded in `docs/decision-delta.md`.

---

## 5. Origin

### 5.1 What origin is

`origin` classifies **authoring authority**: not merely where a value came
from, but who was entitled to put it there. It answers a question `access`
cannot:

- `origin` — may this node exist / be written at all, and by which authority?
- `access` — given that it may be written, which identity may write it?

These are separate concerns and MUST NOT be conflated.

### 5.2 Grammar

**INV-013** — `origin` MUST match exactly this grammar. No other form is valid.

```
Origin :=
      [ yaml ]
    | [ schema ]
    | [ sealed(template, path) ]
    | [ sealed(template, path), schema ]
```

Concrete YAML encoding:

```yaml
origin: [yaml]

origin: [schema]

origin:
  - sealed:
      template: $network-object
      path: $.access.read

origin:
  - sealed:
      template: $network-object
      path: $.access.read
  - schema
```

Meaning:

| Origin | Meaning |
|---|---|
| `[yaml]` | The instance YAML explicitly supplied this value. |
| `[schema]` | The value materialized from this node's schema default. |
| `[sealed(t,p)]` | The node and its value came from the closed template `t` at path `p`. |
| `[sealed(t,p), schema]` | The template defined and closed the node; the value came from the template node's schema default. |

**INV-014** — `origin` is a classification, not a history. It MUST NOT
accumulate lifecycle events (`transformed`, `migrated`, `normalized`, …).
Audit history belongs to ProofTrace, not to the object model.

**INV-015** — A `sealed` term MUST carry both `template` and `path`. A
`sealed` term missing either MUST be rejected. The template identity alone is
insufficient: one template may be instantiated at many paths, and provenance
that cannot distinguish them is not provenance.

The `template` reference SHOULD be content-addressed (`$name@sha256:…`) so that
origin is reproducible rather than merely descriptive. 0.2 does not require it.

### 5.3 The truth table

Read as: which source facts hold for this node's current value.

| # | sealed | yaml | schema | Result | Origin |
|---|---|---|---|---|---|
| 1 | no | yes | no | **valid** | `[yaml]` |
| 2 | no | no | yes | **valid** | `[schema]` |
| 3 | yes | no | no | **valid** | `[sealed(t,p)]` |
| 4 | yes | no | yes | **valid** | `[sealed(t,p), schema]` |
| 5 | yes | yes | no | **INVALID** | — |
| 6 | yes | yes | yes | **INVALID** | — |
| 7 | no | yes | yes | **INVALID** | — |
| 8 | no | no | no | **INVALID** | — |

**INV-016** — `sealed` and `yaml` MUST NOT both hold (rows 5, 6). `sealed`
means authoring is closed at and below this node; a YAML-sourced value there is
a structurally illegal authoring attempt, not merely a bad value.

**INV-017** — `yaml` and `schema` MUST NOT both hold (row 7). A single
effective value is either explicitly supplied or defaulted; it cannot be both.

**INV-018** — `origin` MUST NOT be empty (row 8). Every materialized node has
an authority; a node with no origin is unattributable and MUST be rejected.

Rows 3 and 4 are why `sealed` combines with `schema` but not with `yaml`:
`sealed` constrains *structural* authority while `schema` describes *value*
source. They are different dimensions. `yaml` is a value source too, which is
why it collides with both.

> Row 8 is not present in the source material this model was derived from; the
> table there enumerated six rows and left the all-negative case undefined.
> INV-018 closes it. Recorded in `docs/decision-delta.md`.

### 5.4 Sealed as an authoring boundary

**INV-019** — Authoring input MUST NOT supply any value at or below a node
whose origin contains `sealed`. Such an attempt MUST be rejected at entry
validation (§8.1) with a structural error, not silently ignored and not
deferred to a later validation stage.

This is a fail-closed property: the traversal stops at the boundary rather than
allowing invalid input to participate in reference resolution or template
expansion first.

### 5.5 `sealed` is an overloaded word — disambiguation

`sealed` already exists in the CIC schema layer with a **different** meaning:
it is one of the three aggregate slot modes (`sealed | defaulted | required`),
governing whether a *derived schema* may override a slot of its base aggregate.

| | Slot mode `sealed` | Origin `sealed(t,p)` |
|---|---|---|
| Plane | schema → schema (type derivation) | schema → instance (authoring) |
| Constrains | a derived type overriding a base slot | instance YAML writing a node |
| Encoding | bare token, as `mode: sealed` | constructor with arity 2 |

**INV-020** — The two MUST NOT be unified. Origin `sealed` MUST always appear
as a constructor carrying `template` and `path` (INV-015); the bare token
`sealed` in an `origin` is invalid. This makes the two syntactically
distinguishable at every occurrence.

---

## 6. Primitives

### 6.1 The set

The primitive set is the eight irreducible atoms:

`shape`, `role`, `behavior`, `contract`, `address`, `identity`, `event`,
`access`.

This specification does **not** change that set.

**INV-021** — A node MUST NOT carry a member that is neither `values`,
`origin`, nor a member of the primitive set. Unknown primitives MUST be
rejected.

**INV-022** — A canonical node MUST carry every primitive its schema declares
for that position, materialized per §8. Primitive semantics MUST NOT be left
undefined for a valid node.

### 6.2 `origin` is not a ninth atom

`origin` is a member of the node envelope, alongside `values` — not a
primitive. Three properties separate it from every atom:

1. **Authorability.** Every atom is declared by a schema author. `origin` is
   never authored (INV-007); it is computed.
2. **Subject.** The atoms describe the *managed object* — its structure, its
   role, its constraints, who may reach it. `origin` describes the *node as an
   artifact of materialization*.
3. **Recursion.** Every atom is itself a CIC node (INV-003). `origin` is
   terminal (INV-004); it is the fixed point that makes the recursion
   well-founded.

Consequently D-003 (the irreducible atom set) is **unchanged** by this model.
See `docs/decision-delta.md`.

### 6.3 Primitives are grouped, not prefixed

A primitive's internal structure MUST be expressed as nested groups, never as
name prefixes.

```yaml
# normative
shape:
  values:
    type: scalar
    scalar_type: integer

# NOT normative — prefix encoding
shape_type: scalar
scalar_type: integer
```

**INV-023** — Within a primitive, named addressable entries MUST be used in
place of ordered anonymous lists wherever the entry has identity. `contract[0]`
is not a stable address; `contract.rules.mtu-range` is.

The reason is not aesthetic. A deterministic addressable path is what allows an
entry to be hashed, diffed, referenced from evidence, and overridden by policy.
An index changes when a neighbour is inserted; a name does not.

### 6.4 `access`

`access` is the one primitive whose structure this document fixes, because it
carries semantics that already exist in the schema layer and must not be lost.

```yaml
access:
  read:
    rules:
      operator:
        subjects: ["OU=operators,O=acme"]
        effect: allow
      auditor:
        subjects: ["OU=auditors,O=acme"]
        effect: allow
    inherit: true
    default_injection: null
  modify:
    rules:
      network-admin:
        subjects: ["OU=network-admins,O=acme"]
        effect: allow
    inherit: true
```

That is the **schema-side declaration**. What materializes from it is a node
tree, not this mapping: `access` is a node, `access.values.read` is a node,
`access.values.read.values.rules` is a node, down to the leaves (INV-027,
INV-035). The shape above is what an author writes; §8 says what it becomes.

The two operations are named **`read`** and **`modify`**.

**INV-024** — `access` MUST declare operations under the names `read` and
`modify`. `write` is not a valid operation name.

#### Who guards the guard

Once `access.read` is a node, the obvious next question is what governs *it* —
and the answer is **`origin`, not a nested `access`**.

**INV-036** — A node materialized from a schema declaration MUST carry
`origin: [schema]`, and a node whose origin is `[schema]` MUST NOT be
authorable from instance input: it cannot be created, replaced or deleted there,
and neither can anything beneath it.

That is the whole protection, and it is stronger than an `access` rule would be:
`access` governs who may act on a value that exists, while `origin: [schema]`
decides whether instance input may reach the position at all. A denied write is
a decision; an unreachable position is not a decision anyone has to get right.

`access.read.access` is therefore **not** part of 0.2. Admitting it would
require the schema language to declare access on access declarations, which
starts the regress this section exists to stop.

**INV-025** — `inherit` is retained with its established tri-state semantics
and MUST be placed per-operation, at `access.<operation>.inherit`:

| Value | Meaning |
|---|---|
| `true` (default) | Sub-objects inherit this rule for this field. |
| `false` | Not inherited; the sub-object takes its own default rules. |
| `0` | Full reset; the sub-object recomputes from its PolicySurface. |

**INV-026** — `default_injection` is retained and MUST be placed at
`access.read.default_injection`. It declares what a requester without read
permission receives: `null` (default) hides the field entirely; any other value
is injected in its place so that the field's existence does not leak. It is
invalid under `access.modify` — a denied write has no value to inject.

INV-025 and INV-026 relocate two fields that a purely additive reading of the
new structure would have dropped. Both mappings are lossless and are recorded
in `docs/decision-delta.md`.

#### `inherit` is recorded, not resolved

**INV-037** — A canonical object MUST carry `inherit` verbatim, as declared. An
implementation MUST NOT resolve inheritance chains, and MUST NOT substitute a
computed effective rule for a declared one.

This is a deliberate boundary, and 0.1 got it wrong in the other direction: §8.6
ordered implementations to *"resolve `inherit` chains"* while §1 put the
policy-decision point out of scope and `docs/decision-delta.md` declined to
define what per-operation inheritance means when the two operations disagree. It
was a MUST whose semantics the same document refused to supply
(`docs/spec-defects.md` SD-012).

Three questions have to be answered before resolution can be specified at all,
and none of them is answered here:

1. **What does a node inherit from?** Under INV-027 the tree now has many more
   positions than 0.1 assumed. Does `mtu.access.read` inherit from `mtu.access`,
   from `mtu`, from the enclosing object, or from nothing?
2. **What happens when operations disagree?** `read.inherit: true` with
   `modify.inherit: 0` is representable and appears in the corpus. It has no
   defined meaning.
3. **What is `0` reset *to*?** The tri-state's full-reset value recomputes from
   the PolicySurface, which does not exist yet.

Recording without resolving is what lets an object be complete and honest at the
same time: everything declared is present and addressable, and nothing is
asserted about an evaluation the model cannot yet perform. Resolution is a 0.3
concern, and it arrives with the policy-decision point or not at all.

---

## 7. Object closure — the three rules

The one remaining way out of the model would be to hide an unprocessed object
graph behind a bare `{}`. These three rules close it.

**INV-027** — A structured object known to the schema MUST be recursively
materialized: every schema-known child MUST become a CIC node. It MUST NOT
survive as a raw `map<string, any>`.

This holds **wherever the structure sits**, not only under `values`. A
primitive's payload is a structured object known to the schema, so it
materializes the same way: `access.values.read` is a node, `…read.values.rules`
is a node, and so on to the leaves the schema declares (INV-035). 0.1 left these
as raw mappings, which is why §2.2's claim about
`network.values.mtu.access.read` was not true of any object 0.1 produced
(`docs/spec-defects.md` SD-003).

The cost is real and accepted: a canonical object is substantially larger than
the authoring input that produced it. That is the trade the model makes —
addressability, hashing, diffing and evidence references reach every declared
position, or they reach none.

**INV-028** — An object the schema explicitly declares **opaque** is a terminal
value. No CIC semantics apply below it; its content MUST be preserved verbatim
and MUST NOT be materialized into nodes.

**INV-029** — An object that is neither schema-known nor explicitly declared
opaque is **invalid** and MUST be rejected.

Opacity MUST be declared, never inferred. An object does not become opaque by
being empty, by being unrecognized, or by being awkward to materialize.

### 7.1 The empty object

`foo: {}` at a structured position does not mean "an arbitrary empty map". It
means the node exists and its children materialize from schema defaults:

```yaml
# schema declares foo.bar default 42, foo.baz default true
# input:
foo: {}

# canonical:
foo:
  values:
    bar:
      values: 42
      origin: [schema]
    baz:
      values: true
      origin: [schema]
  origin: [yaml]
```

`foo`'s origin is `[yaml]` — the author asserted the node's presence. Its
children's origins are `[schema]` — the author supplied no values. If the
schema declares no required children and no defaults, the effective value is
legitimately an empty object, and it is still a valid CIC node with a known
shape — not an unprocessed subtree.

---

## 8. The materialization pipeline

Each stage below is normative. An implementation MAY fuse stages internally but
MUST produce results indistinguishable from performing them in this order — in
particular, a later stage MUST NOT observe input that an earlier stage would
have rejected.

**INV-030** — Materialization MUST be deterministic: the same schema and input
MUST produce a byte-identical canonical object.

### 8.1 Entry validation

- **INPUT:** raw authoring tree
- **OUTPUT:** structurally legal authoring tree
- **MUST:** reject a document with a duplicate mapping key (INV-041) or a YAML
  anchor or alias (INV-042); reject input containing an `origin` member
  (INV-007); reject authoring at or below a sealed boundary (INV-019); reject
  undeclared arbitrary objects (INV-029)
- **MUST NOT:** perform reference resolution, template expansion, or default
  application
- **FAILURE:** reject, before the input can participate in any later stage

Validating only at the end of the pipeline would let invalid input take part in
expansion and resolution first, producing states that are hard to attribute.

### 8.2 External reference resolution — **out of scope for 0.2**

The schema language of 0.2 has no reference syntax, so this stage has nothing to
resolve and no way to fail. 0.1 specified it as a stage with a MUST and a
FAILURE clause anyway, which left an obligation no implementation could exercise
and a stage identifier with no reachable use (`docs/spec-defects.md` SD-009).

The stage keeps its position in the pipeline so that the ordering argument in §8
stays intact and so that adding references later does not renumber the stages.

- **INPUT:** structurally legal authoring tree
- **OUTPUT:** the same tree, unchanged
- **MUST:** nothing in 0.2
- **MUST NOT:** invent a reference syntax; leave a reference for a module to
  resolve (§9) once one exists
- **FAILURE:** none reachable in 0.2

When the reference syntax lands, this stage regains a MUST and a FAILURE, and
INV-005's forward note (§2.3) becomes a requirement rather than a note.

### 8.3 Sealed / template expansion

- **INPUT:** resolved authoring tree
- **OUTPUT:** tree with no unexpanded template references
- **MUST:** record template identity and source path in the origin of every
  node produced (INV-015)
- **MUST NOT:** permit any authoring value below a sealed boundary (INV-019)
- **FAILURE:** authoring attempt below a sealed subtree → reject

### 8.4 Recursive node construction

- **INPUT:** expanded tree
- **OUTPUT:** every schema-known structured child represented as a CIC node
- **MUST:** apply the discriminator of §4 at every position; apply the closure
  rules of §7
- **MUST NOT:** leave a schema-known child as a raw value (INV-027)
- **FAILURE:** undeclared arbitrary object → reject (INV-029)

### 8.5 Schema value and default materialization

- **INPUT:** node tree with authored values
- **OUTPUT:** node tree with every absent defaultable value filled
- **MUST:** set `origin: [schema]` on every node filled from a default, and
  `[sealed(t,p), schema]` where the default came from a sealed template's schema
- **MUST NOT:** produce a node whose origin holds both `yaml` and `schema`
  (INV-017)
- **FAILURE:** a required value neither authored nor defaultable → reject

### 8.6 Primitive evaluation

- **INPUT:** node tree with values materialized
- **OUTPUT:** node tree with every schema-declared primitive materialized as a
  node, **and every schema-declared child of a primitive's payload materialized
  as a node** (INV-027, INV-035)
- **MUST:** materialize every declared primitive (INV-022); recurse into
  primitive payloads to the leaves the schema declares (INV-035); record
  `inherit` verbatim (INV-037)
- **MUST NOT:** admit an unknown primitive (INV-021); **resolve `inherit`
  chains** (INV-037) — 0.1 required this and the semantics are undefined
- **FAILURE:** unknown primitive, or a primitive whose semantics cannot be
  resolved → reject

### 8.7 Final validation

- **INPUT:** fully materialized node tree
- **OUTPUT:** validated node tree
- **MUST:** enforce every contract; enforce the origin grammar and truth table
  (§5.2, §5.3); confirm INV-001, INV-002 hold at every node
- **FAILURE:** any violation → reject

### 8.8 Canonicalization

- **INPUT:** validated node tree
- **OUTPUT:** canonical CIC object
- **MUST:** produce the serialization of §8.8.1 (INV-043) in the member order of
  §8.8.2 (INV-044); hand the object over at a known model version (§11)
- **FAILURE:** output differing from §8.8.1 → implementation defect

Until 0.2 this stage said only "deterministic" (INV-030), which constrains an
implementation to agree **with itself**. It does not make two implementations
agree with each other, and they did not: measured across the thirteen
materialization vectors, the Go and Rust implementations produced **zero
byte-identical objects** while producing semantically identical ones every time.
Both were conformant. The specification could not say which was right because it
had not said anything.

That is not a tidiness problem. `INV-030` promises a byte-identical object;
evidence that references an object references its bytes; and a digest taken over
a canonical object has no defined input until this section exists.

#### 8.8.1 The serialization

**INV-043** — A canonical CIC object MUST be serialized exactly as follows.

- **Encoding** UTF-8, no byte-order mark. Lines end with a single `\n`,
  including the last.
- **Document** Begins with `---\n`. One document per object; no `...`
  terminator.
- **Structure** Block style throughout, except where stated below. Each nesting
  level indents by exactly two spaces.
- **Node members** `values:` on its own line when the payload is a collection,
  or `values: <scalar>` when it is a scalar. `origin:` always inline (below).
  Primitive members follow, each `<name>:` on its own line.
- **`origin`** Flow style, on one line: `[yaml]`, `[schema]`,
  `[{sealed: {template: T, path: P}}]`, `[{sealed: {template: T, path: P}}, schema]`.
  The grammar is four short productions and a block form spreads them over up to
  six lines each, which makes the one thing a reader most often checks the
  hardest thing to see.
- **Sequences** Each entry begins `- ` with its first member on that same line;
  the entry's remaining members align under it. An empty sequence is `[]`, an
  empty mapping is `{}` — block style has no way to write "nothing here".
- **Scalars** Plain where plain is unambiguous, single-quoted otherwise, with an
  embedded `'` doubled. Quoting is REQUIRED when the text:
  - is empty;
  - would be read as a **number** under either YAML version — decimal, a
    leading `+`/`-`, a decimal point or exponent, hexadecimal (`0x…`), octal
    (`0o…` or a leading `0` before digits), digit groups separated by `_`,
    `.inf`/`.nan` in any case, or sexagesimal (`1:30`);
  - would be read as a **boolean or null** under either YAML version, which
    includes `yes`, `no`, `on`, `off`, `~` and every case variant;
  - begins with a space or an indicator character, or ends with a space;
  - contains `: `, ` #`, or a line break.

  Everything else is written plain, including a `'` inside the text — a plain
  scalar may contain one, and quoting on sight would make the rule harder to
  reproduce rather than safer.
- **Numbers** An integer is written without a decimal point. A float always
  carries a `.` or an exponent, so that it does not read back as an integer.

The `on`/`off` rule is not hypothetical: an opaque payload in this corpus holds
the list `[on, off]`, and a script rewriting expectations round-tripped it to
`[true, false]` by trusting one parser's reading. Quoting removes the question
rather than answering it per reader.

#### 8.8.2 Member order

**INV-044** — Members are serialized in this order, and no other:

1. `values`
2. `origin`
3. the primitives the node carries, in the order §6.1 states them —
   `shape`, `role`, `behavior`, `contract`, `address`, `identity`, `event`,
   `access`

Within a payload, children are serialized **in the order they were established**:
for a schema-derived payload the order the schema declares them, and for an
opaque payload the order the author wrote them.

The second half is a consequence of §4 rather than a preference. An opaque
payload is carried verbatim and nothing below it is interpreted, so reordering
it is a transformation of data this model promised not to touch. One
implementation sorted every mapping alphabetically, including opaque payloads,
and the corpus could not see it because a conformance runner comparing parsed
structure cannot compare order at all.

Having settled that, sorting the interpreted payloads alphabetically while
carrying opaque ones verbatim would leave the model with two ordering rules and
a reader with a question at every level about which one applies. Declaration
order is one rule.

One position is fixed rather than declared: an `access` operation block is
serialized `rules`, `inherit`, `default_injection`. `inherit` is injected when
the schema does not state it (§6.4), and the injected member sits between a
declared `rules` and a declared `default_injection` — a position no rule about
declaration order can produce, because the member was never declared.

---

## 9. The module boundary contract

**INV-031** — A module MUST NOT receive any of the following:

| # | MUST NOT receive | Because |
|---|---|---|
| a | unresolved references | the module would become a resolver |
| b | authoring short forms | the module would become a normalizer |
| c | templates | the module would become an expander |
| d | sealed source fragments | the module would see a pre-boundary artifact |
| e | unapplied schema defaults | the module would become a default engine |
| f | unknown primitives | the module would define semantics |
| g | unvalidated objects | the module would become the validator |

**INV-032** — The type of a module's input MUST be
`Validated<Canonical<CICObject>>`, and a value of that type MUST be
constructible only by the core materializer. A module API accepting an
unvalidated map (`Execute(map[string]any)`, or equivalent) violates this
specification.

This is a construction-level guarantee, not a convention. "Validate before you
call" is a rule that can be forgotten; "an invalid object has no representation
that crosses the boundary" cannot be. The Go and Rust implementations MUST both
enforce INV-032 through their type systems — an unexported constructor in Go, a
private-field newtype in Rust.

---

## 10. Conformance

An implementation is conformant if and only if it produces, for every vector in
`conformance/`, output matching `expected.yaml` (or the error class in
`expected-error.yaml`) exactly.

Vectors are implementation-independent: YAML in, YAML out. Go, Rust, and any
later implementation run the same corpus.

**Every normative statement in this document is mapped to a vector or is
explicitly marked unvectorizable, in [`docs/spec-vector-map.md`](docs/spec-vector-map.md).**
That mapping is machine-checked by `tools/check_spec_vectors.py`, which fails
if an invariant claims a vector that does not exist or a vector claims an
invariant that does not exist. The check verifies the *mapping*, not
conformance results.

**Status of the corpus as of model 0.2: executed by both implementations.**
`go/` and `rust/` each run the whole corpus and produce byte-identical
canonical objects. What that establishes is that they agree on THESE
vectors — a bound worth stating, because for thirty-one of them the corpus
contained no wrong-typed input at all, and the two disagreed on every such
case an external review constructed.

---

## 11. Versioning

**INV-033** — Every canonical CIC object is handed over **at** a known model
version. The version is a property of the hand-off, not a member of the object:
it MUST be carried by whatever frame transfers the object — the serialization
envelope, the call, the artifact record — and it MUST NOT appear inside the node
tree.

**INV-034** — A module MUST declare the model version it consumes, and a host
MUST NOT hand a module an object of a version the module has not declared.

The two are one rule stated from both ends: INV-033 says the version travels
with the object, INV-034 says the receiver checks it.

0.1 said something different and unsatisfiable: that the *object* must carry the
version. §2.1 closes the node grammar to `values`, `origin` and the primitive
set, so there was nowhere in an object to put it, and the corpus invented a
`cic:` member on the root — producing something that, by this document's own
definition, was not a CIC node. The set of objects satisfying 0.1's INV-033 was
empty (`docs/spec-defects.md` SD-017). Note also that `cic` appeared exactly once
in 0.1, inside a code block, and was never defined as a construct: a
specification must not introduce one by example.

Modules do not claim to support a YAML dialect; they consume a numbered object
model. This is the semantic equivalent of an ABI, and it is versioned from the
first day rather than being called `latest` and pinned retroactively.

### 11.1 How a version increment arrives

Within 0.x, any change to a normative statement in this document is a version
increment and MUST arrive in a single change together with its conformance
vectors and **every implementation that exists at that time**. Keeping the spec,
the vectors, and the implementations in one repository is deliberate: it makes
it physically awkward to change one implementation's semantics without the
others and the corpus noticing.

**INV-038** — A normative change MUST NOT be split across releases from its
vectors, and MUST NOT land ahead of any implementation this repository ships.

### 11.2 What a release covers

**INV-045** — A release MUST identify its subject by a digest over the whole
normative product: this document, the machine-readable schemas, every
conformance vector, and every implementation shipped with it. A release whose
signature covers less than that MUST NOT be presented as covering the model.

The failure this closes was measured rather than imagined. The release signed a
hash of the project descriptor, and the descriptor named one file — the schema
index — as its canonical source. That file refers to this document **by path**
and binds nothing about its content, so `SPEC.md`, all conformance vectors and
both implementations could be changed while the signature stayed valid.

A signature that does not cover the normative product is worse than no
signature: it reports that something was checked, and what it checked is not
what a reader is relying on.

Two properties the subject must have, and both are consequences rather than
choices:

- **It cannot cover what is about it.** The descriptor holds the digest, and a
  review record is named for it, so including either would mean writing the
  answer changed the question. The descriptor and the review are *claims about*
  the subject; the subject is the normative product they are claims about.

  This was stated for the descriptor and missed for the review, which made
  INV-046 unsatisfiable: a record committed to a pull request changed the digest
  its own filename referred to, and the newly required record changed it again.
  No tree could carry a review of itself short of a SHA-256 fixed point.
- **It must be verifiable by someone who is not the publisher.** A check that
  needs the publisher's toolchain establishes nothing for anyone else.

A matching subject establishes that the tree is the tree the release describes.
It does not establish who produced it — that is the signature over the digest,
which is a separate artifact and a separate check.

0.1 wrote "**both** implementations", which during bootstrap made the rule
unsatisfiable in the other direction: only one implementation existed, so no
defect in 0.1 could be fixed without first writing a second implementation
against the known-defective specification. That is a deadlock created by a rule
intended for the steady state (`docs/spec-defects.md` SD-018). The requirement is
now stated against the implementations that exist, which is the property the
rule was actually protecting.

---

### 11.3 What a release requires

**INV-046** — A release MUST be preceded by an **independent external review**
of its subject (§11.2). The review MUST be recorded against the subject digest
it examined, and a subject with no such record MUST NOT be released.

Independent means: not the author, working from the artifact rather than from a
description of it, and free to disagree with this document. The last clause is
the one that does the work — a review that only checks the implementations
against `SPEC.md` cannot find a defect *in* `SPEC.md`, and that is where the
most expensive ones have been.

The rule exists because of what the first such review found. Nineteen findings
against one release, and the four checked first were confirmed by measurement
within minutes: the model version had split five ways, a reader handed out the
node's own slice so a validated object could be rewritten through it, the origin
grammar accepted forms it declares invalid, and the manifest gate could not see
a file missing from the manifest. Every one had passed a full green CI,
repeatedly.

They passed because **the gates were written by the same reading that wrote the
code**. No internal check acquires an outside view by being made stricter; that
is a property of who is looking, not of how hard.

What can be enforced mechanically is narrow and worth stating exactly: that a
review record exists **for this tree**. Whether a person did the work, or did it
well, is not checkable and is not claimed. Binding the record to the subject
digest is what stops "it was reviewed" from surviving every later change to the
tree — a review of a different subject is a review of a different thing.

The procedure, and the three commissioning prompts a request carries, are in
[`docs/external-review.md`](docs/external-review.md).

## 12. Invariant index

| ID | Statement | §|
|---|---|---|
| INV-001 | Exactly one `values` per node | 2.1 |
| INV-002 | Exactly one `origin` per canonical node | 2.1 |
| INV-003 | Every primitive is itself a CIC node | 2.2 |
| INV-004 | `origin` is terminal | 2.3 |
| INV-005 | Primitive materialization terminates; declaration graph acyclic | 2.3 |
| INV-006 | No documentation member on a canonical node | 2.4 |
| INV-007 | Authoring input MUST NOT contain `origin` | 3 |
| INV-008 | Envelope/payload decided by schema position, not keys | 4.3 |
| INV-009 | At structured positions: envelope iff `values` present | 4.3 |
| INV-010 | Schema MUST NOT declare a child named `values` | 4.3 |
| INV-011 | No primitive interpretation below `values` | 4.3 |
| INV-012 | No `default` member on a canonical node | 4.5 |
| INV-013 | Origin grammar — exactly four forms | 5.2 |
| INV-014 | Origin is classification, not history | 5.2 |
| INV-015 | `sealed` MUST carry `template` and `path` | 5.2 |
| INV-016 | `sealed` + `yaml` → invalid | 5.3 |
| INV-017 | `yaml` + `schema` → invalid | 5.3 |
| INV-018 | Empty origin → invalid | 5.3 |
| INV-019 | No authoring at or below a sealed boundary | 5.4 |
| INV-020 | Origin `sealed` always a constructor; distinct from slot mode | 5.5 |
| INV-021 | Unknown primitives rejected | 6.1 |
| INV-022 | Every schema-declared primitive materialized | 6.1 |
| INV-023 | Named addressable entries, not anonymous lists | 6.3 |
| INV-024 | `access` operations are `read` and `modify` | 6.4 |
| INV-025 | `inherit` retained, per-operation, tri-state | 6.4 |
| INV-026 | `default_injection` retained, `access.read` only | 6.4 |
| INV-027 | Structured object → recursively materialized | 7 |
| INV-028 | Explicit opaque → terminal value | 7 |
| INV-029 | Undeclared arbitrary object → invalid | 7 |
| INV-030 | Materialization is deterministic | 8 |
| INV-031 | Seven things a module MUST NOT receive | 9 |
| INV-032 | Module input type constructible only by the materializer | 9 |
| INV-033 | The model version belongs to the hand-off, not the object | 11 |
| INV-034 | Modules declare the model version they consume | 11 |
| INV-035 | Schema-declared children of a primitive payload are nodes | 2.2 |
| INV-036 | Schema-origin nodes are not authorable from instance input | 6.4 |
| INV-037 | `inherit` is recorded verbatim, never resolved | 6.4 |
| INV-038 | A normative change ships with its vectors and every implementation | 11.1 |
| INV-039 | Schema-less final validation is key-directed and weaker | 4.3 |
| INV-040 | Every node has exactly one address | 2.5 |
| INV-041 | A mapping declares no key twice | 2.6 |
| INV-042 | No YAML anchors or aliases | 2.6 |
| INV-043 | The canonical serialization, byte for byte | 8.8.1 |
| INV-044 | Canonical member order | 8.8.2 |
| INV-045 | A release's subject is the whole normative product | 11.2 |
| INV-046 | A release is preceded by an independent external review | 11.3 |

---

## 13. Related documents

- [`docs/spec-defects.md`](docs/spec-defects.md) — where this document is not
  executable, measured by implementing it. **Read SD-017 before relying on
  INV-033.**
- [`docs/spec-vector-map.md`](docs/spec-vector-map.md) — every invariant to its
  vectors, or its unvectorizable justification
- [`docs/decision-delta.md`](docs/decision-delta.md) — what this model changes
  in D-003 and D-011, and what would otherwise have been lost silently
- [`docs/migration-surface.md`](docs/migration-surface.md) — the measured file
  list this model would change
- [`docs/branch-decision.md`](docs/branch-decision.md) — why this repository
  was bootstrapped from `base-repo` `wasm/main`
- [`docs/rust-gate-extraction.md`](docs/rust-gate-extraction.md) — the
  line-referenced recipe for `mk/rust.mk`
- [`conformance/README.md`](conformance/README.md) — vector format
