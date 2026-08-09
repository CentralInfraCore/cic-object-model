# CIC Object Model — semantic divergence audit

## Scope and method

- Repository: `/workspace/scratch/103a14c45aa7/cic-object-model-audit`
- Audited commit: `cbaf928be5362f67a7dbf6378637eba7716ebb5b` (`Merge pull request #14 from CentralInfraCore/spec/release-provenance`)
- Audit target: model 0.2, comparing `SPEC.md`, the 31-vector conformance corpus, `spec/*.schema.yaml`, the Go implementation/API, documentation, and conformance/release tooling.
- Repository files were not modified. This report is outside the repository.
- The checked-out tree happens to contain `rust/`, but the semantic findings below do not assume a Rust implementation exists or treat it as an authority. “Latent cross-implementation ambiguity” means a genuinely independent second implementation is likely to expose the difference.
- Read-only checks: `git rev-parse HEAD` returned the commit above; `python tools/check_spec_vectors.py` returned `PASS` with 46 invariants, 31 vectors, 39 invariants with vectors, and 7 declared unvectorizable. `go test ./...` could not be run because `go` is not installed in the audit environment (`/bin/bash: go: command not found`). Findings about Go behavior are therefore source- and fixture-derived unless a repository test is cited.

## Executive assessment

The corpus pins the common-path serialization tightly, but the model still has material semantic forks an independent implementation cannot resolve from `SPEC.md` alone. The most consequential are:

1. instance-authored `access` overrides are required by a vector but forbidden by INV-036's plain wording;
2. an injected `inherit: true` can be labelled `[yaml]` even though the author did not supply it;
3. the access tri-state is described but not type-validated;
4. schema typing, optional omission, template adoption, template recursion, and error stages are largely implementation/corpus semantics rather than specified semantics;
5. the address grammar cannot encode every legal string key and the Go resolver accepts multiple spellings of a list index;
6. the Go origin validator and machine-readable origin schema accept different languages;
7. byte canonicalization remains underdetermined for mapping keys and many numeric values;
8. schema-less final validation cannot satisfy the stated contract/closure obligations;
9. INV-032 promises a language-neutral type guarantee that Go cannot provide, so conforming implementations can have different trust boundaries.

## Findings

### F-01 — Instance-authored primitives are simultaneously allowed and forbidden

**Severity:** Critical. **Classification:** direct normative/corpus conflict; **latent cross-implementation ambiguity**.

**Semantic question.** May instance input create or replace a schema-declared primitive such as `access`, and if so is the instance value a replacement, a merge, or an override at individual leaves?

**Competing interpretations.**

1. No: INV-036 says a node materialized from a schema declaration has `[schema]` origin and a schema-origin node “MUST NOT be authorable from instance input,” including anything beneath it.
2. Yes: the discriminator example and vector 011 explicitly author an `access` override in the instance envelope.
3. If yes, an implementation can replace the entire declared primitive, recursively merge it, or reject conflicts; the specification chooses none.

**Evidence.** `SPEC.md:295-306` uses instance-authored `access`; `SPEC.md:660-663` forbids creating, replacing, or deleting schema-origin nodes and descendants. `conformance/materialization/011_discriminator_envelope/meta.yaml:3-8` calls the value an “instance-level access override”; its schema declares `access.modify` at `schema.yaml:9-11`, its input supplies a different value at `input.yaml:4-9`, and its expected result marks the entire access subtree `[yaml]` at `expected.yaml:15-36`. Go deliberately permits primitive members during entry validation (`go/objectmodel/entry.go:76-94`) and gives an authored primitive precedence over its schema declaration (`go/objectmodel/primitives.go:55-100`).

**Current implementation choice.** Go replaces the primitive declaration wholesale whenever the envelope supplies that primitive; it does not merge declared and authored payloads. The resulting primitive and every descendant are YAML-origin.

**Specified or accidental?** Authorability is corpus-specified but contradicts the normative invariant. Whole-primitive replacement is an implementation choice; neither the vector nor the text distinguishes replacement from merging outside the one overlapping example.

**Consequence.** Access policy can be discarded or replaced by instance data under one conformant reading and rejected under another. That changes both authorization semantics and provenance, so this is not a formatting divergence.

### F-02 — The corpus assigns YAML provenance to a default the YAML did not contain

**Severity:** Critical. **Classification:** corpus-only provenance semantics; **latent cross-implementation ambiguity**.

**Semantic question.** What origin does an injected primitive-internal default carry when the surrounding primitive was authored?

**Competing interpretations.**

1. Per-value provenance: absent `inherit` defaults to `true`, so the materializer supplied the value and its origin is `[schema]`.
2. Subtree provenance: every node under an authored primitive inherits `[yaml]`, including injected values.

**Evidence.** `[yaml]` means the instance “explicitly supplied this value” and `[schema]` means it came from the node's schema default (`SPEC.md:476-481`). `inherit` defaults to true (`SPEC.md:674-682`). Vector 011's input omits `inherit` (`conformance/materialization/011_discriminator_envelope/input.yaml:4-9`), yet the expected injected node is `values: true; origin: [yaml]` (`expected.yaml:32-34`). Go injects the value at `go/objectmodel/primitives.go:183-190`, then `primitiveNode` propagates one origin recursively to every payload child (`go/objectmodel/primitives.go:105-149`). Vector 013 shows the opposite surrounding case: its schema omits `read.inherit` (`schema.yaml:10-16`) and expected output assigns `[schema]` (`expected.yaml:35-37`).

**Current implementation choice.** Origin is inherited from the primitive as a whole, not computed for each materialized leaf.

**Specified or accidental?** Required by the exact bytes of vector 011, but semantically accidental: it conflicts with the definition of `[yaml]`, and no invariant states subtree-origin inheritance.

**Consequence.** Canonical evidence can falsely state that an author explicitly supplied an access-control value. Consumers cannot distinguish a deliberate `inherit: true` from an injected default.

### F-03 — `access` has described value domains but no enforced grammar

**Severity:** High. **Classification:** normative requirement the corpus cannot distinguish; **latent cross-implementation ambiguity**.

**Semantic question.** Must `inherit` be exactly boolean `true`, boolean `false`, or integer `0`; must access operation objects contain only `rules`, `inherit`, and (for read) `default_injection`?

**Competing interpretations.**

1. Closed grammar: reject `inherit: "true"`, `inherit: 1`, `inherit: null`, and unknown operation members.
2. Descriptive semantics only: preserve any payload except invalid operation names and `modify.default_injection`.

**Evidence.** INV-025 gives exactly three values and meanings (`SPEC.md:674-682`); INV-026 fixes placement (`SPEC.md:683-687`); §8.8.2 fixes the operation member order to `rules`, `inherit`, `default_injection` (`SPEC.md:972-976`). Go validates only that access and operations are mappings, operation names are `read|modify`, and `default_injection` is absent under modify (`go/objectmodel/primitives.go:160-200`). It appends every other operation key unchanged (`go/objectmodel/primitives.go:201-217`). The only access vector is positive and exercises `true` and `0` (`conformance/materialization/013_access_inherit_injection/meta.yaml:3-9`). `spec/node.schema.yaml:21-46` constrains only the outer primitive node, not access payload semantics.

**Current implementation choice.** Open operation payload plus limited special cases; `inherit` type/domain is not checked.

**Specified or accidental?** The tri-state and placement are specified; Go's open payload and lack of domain validation are accidental/under-tested.

**Consequence.** One implementation can reject invalid policy values while another emits them as canonical. Downstream policy code then has to invent coercion/error behavior the object model claims to remove.

### F-04 — The effective schema language and scalar typing are not normative

**Severity:** High. **Classification:** acknowledged underspecification (SD-011), still open; **latent cross-implementation ambiguity**.

**Semantic question.** What is a valid model schema, and does `scalar_type` validate/coerce the payload or merely annotate it?

**Competing interpretations.** A second implementation could (a) validate the fenced fixture grammar strictly, (b) treat it as non-normative test scaffolding, (c) coerce scalar values, or (d) carry mismatches while emitting a contradictory `shape` primitive.

**Evidence.** The specification says the schema decides the discriminator, defaults, closure, and primitives (`SPEC.md:329-369`, `850-872`) but never defines the schema grammar. The only grammar is introduced as “Deliberately minimal — just enough to exercise the model” (`conformance/README.md:47-75`). It lists scalar types at `conformance/README.md:56-60`. The repository's own open defect records the resulting choices (`docs/spec-defects.md:535-560`). Go converts `model`, `shape`, and `scalar_type` with `fmt.Sprint`, treats a non-boolean `required` as false, rejects unknown *node* keywords, but does not reject unknown top-level schema members (`go/objectmodel/schema.go:112-169`, `172-240`). Scalar construction accepts any decoded value without comparing it to `scalar_type` (`go/objectmodel/construct.go:47-56`); shape mismatch tests cover list/object arity but no scalar type mismatch (`go/objectmodel/branches_test.go:107-135`).

**Current implementation choice.** Strict on unrecognized schema-node keys, permissive at the schema top level, permissive on scalar value types, and non-coercing.

**Specified or accidental?** Go comments explicitly identify fail-closed unknown-key handling as a choice (`schema.go:235-239`). Scalar permissiveness is unstated and corpus-uncovered.

**Consequence.** A node can say `shape.scalar_type: integer` while carrying a string or boolean. Schema acceptance and canonical output vary by implementation before any domain contract is evaluated.

### F-05 — Optional absence and aggregate defaults have multiple conformant outcomes

**Severity:** High. **Classification:** acknowledged underspecification (SD-007) plus an additional implementation restriction; **latent cross-implementation ambiguity**.

**Semantic question.** What happens when a declared position is absent, not required, and has no default; and may object-shaped positions have literal defaults?

**Competing interpretations.** For the first case: omit the node, emit `values: null`, or reject. For an object default: materialize it recursively, treat it like authored `{}`, or reject the schema.

**Evidence.** §8.5 specifies filling defaults and rejecting only a required non-defaultable value (`SPEC.md:850-859`); INV-001 prohibits an emitted node without `values` (`SPEC.md:79-83`). The fixture grammar permits `default: <value>` without a shape restriction (`conformance/README.md:56-63`). Go explicitly calls omission an unspecified choice and returns no node (`go/objectmodel/defaults.go:15-18`, `21-37`); its test comment incorrectly upgrades that choice to “SPEC says” (`go/objectmodel/branches_test.go:44-69`). Go separately rejects an object-shaped default literal (`go/objectmodel/defaults.go:75-109`), although §7.1 describes child defaults and does not forbid aggregate literals (`SPEC.md:757-783`). `docs/spec-defects.md:486-506` confirms no vector reaches optional absence.

**Current implementation choice.** Omit optional absent nodes; reject literal defaults for structured objects.

**Specified or accidental?** Both are implementation choices. Omission is documented honestly in production code but overstated in the test comment; the object-default prohibition is invented by Go.

**Consequence.** Canonical member sets and schema validity differ. Defaults for lists of objects are also constrained by Go's recursive call into the object-default rejection (`defaults.go:88-109`).

### F-06 — Template expansion silently defines schema adoption and origin path scope

**Severity:** High. **Classification:** corpus-only semantics, acknowledged as SD-006; **latent cross-implementation ambiguity**.

**Semantic question.** Does `sealed_from` replace, merge with, or merely source values from the referenced template schema; and does each descendant origin carry the mount path or its own source subpath?

**Competing interpretations.** Replace vs merge vs value-only expansion; mount path (`$.access.read`) vs leaf path (`$.access.read.enabled`).

**Evidence.** §8.3 requires only expansion and recording template identity/source path (`SPEC.md:832-839`). Vector 003 declares a sealed node with no local shape or children (`conformance/materialization/003_origin_sealed/schema.yaml:14-20`) but expects the referenced object shape and child (`expected.yaml:2-23`), and gives the child the same mount path as its parent (`expected.yaml:5-17`). The Go implementation acknowledges the unstated rule and replaces the schema with the template entry (`go/objectmodel/template.go:21-55`); descendants retain one `sealedCtx` (`template.go:58-83`). The existing defect report states both choices are unstated (`docs/spec-defects.md:365-391`).

**Current implementation choice.** Full schema replacement, no merge; one mount `(template,path)` is propagated to every descendant.

**Specified or accidental?** Corpus-specified for the examples but absent from normative text.

**Consequence.** A second implementation can create a different schema tree and different provenance while reasonably following §8.3.

### F-07 — Template references make the “finite literal tree” termination argument false

**Severity:** High. **Classification:** internal model contradiction plus arbitrary Go limit; **latent cross-implementation ambiguity**.

**Semantic question.** How are recursive/nested `sealed_from` template references terminated, and what maximum finite expansion is valid?

**Competing interpretations.** Detect graph cycles with no depth limit; impose a resource depth limit; forbid nested `sealed_from`; or accept every finite chain.

**Evidence.** INV-005 says termination follows because the schema is a finite literal tree and “the schema language has no references” (`SPEC.md:148-165`); §8.2 repeats that 0.2 has no reference syntax (`SPEC.md:812-830`). Yet the corpus grammar includes `sealed_from` references and a `templates` map (`conformance/README.md:64-75`), and §8.3 requires template expansion. A template entry is parsed as an ordinary schema node and can itself contain `sealed_from` (`go/objectmodel/schema.go:142-162`, `172-240`). Go has no cycle check; it rejects any expansion deeper than 64 with `E_TEMPLATE_NOT_FOUND`/INV-005 (`go/objectmodel/template.go:5-5`, `37-55`).

**Current implementation choice.** Arbitrary maximum expansion depth of 64, conflating excessive depth/cycles with “template not found.”

**Specified or accidental?** Accidental implementation safety limit. The normative finiteness rationale overlooks internal template references (even if §8.2 intends only *external* references).

**Consequence.** A finite chain accepted elsewhere is rejected by Go, while cycle diagnostics and stage/error codes can differ. Resource-safety behavior is not portable.

### F-08 — The machine origin schema and Go validators recognize different origin languages

**Severity:** High. **Classification:** machine-readable schema ↔ implementation divergence; **latent cross-implementation ambiguity**.

**Semantic question.** Are template/path lexical patterns and closed constructor objects normative, and must final validation enforce them?

**Competing interpretations.** Enforce the JSON Schema exactly; enforce only the four term shapes and presence of two fields; or treat machine schemas as advisory companions.

**Evidence.** `spec/origin.schema.yaml:14-36` requires closed objects, string values, template matching `^\$.+`, and path matching `^\$\..*`; the four forms are closed by `oneOf` at `origin.schema.yaml:38-68`. Go schema loading merely stringifies `sealed_from.template/path` and checks presence (`go/objectmodel/schema.go:216-229`, `260-269`), so it can emit origins that violate both patterns. Schema-less final validation checks that a constructor has a `sealed` member and non-null `template/path`, but checks neither extra members, types, nor patterns (`go/objectmodel/validate.go:167-186`). It also does not reject extra keys alongside `sealed` or inside the sealed object. `spec/index.yaml:9-14` says the machine schemas encode the origin grammar, not a non-normative approximation.

**Current implementation choice.** Presence-only constructor validation plus exact term order/arity; permissive lexical/content validation.

**Specified or accidental?** The machine schema is explicit; Go's permissiveness is accidental. The prose grammar itself does not state the regexes, so the authority rule (`SPEC.md:15-18`) makes it unclear whether those regexes can add requirements.

**Consequence.** Go may materialize or validate an object rejected by the repository's own machine-readable schema. Independent validators and implementations disagree on canonical object validity.

### F-09 — Legal names are not representable in the address grammar, and indexes have aliases

**Severity:** Critical. **Classification:** normative grammar gap and Go-specific permissiveness; **latent cross-implementation ambiguity**.

**Semantic question.** What character set/escaping applies to `<payload child>` and `<primitive name>`, and what is the lexical grammar of list index `i`?

**Competing interpretations.** Restrict names; introduce escaping; treat member text as an opaque token; accept or reject leading sign/zero index spellings.

**Evidence.** INV-040 requires every node to have exactly one address and all implementations to resolve it identically (`SPEC.md:181-224`), but `member` and `i` have no lexical productions (`SPEC.md:189-198`). The schema loader accepts any YAML mapping key as a child name and builds paths by raw concatenation (`go/objectmodel/schema.go:196-209`); no rule forbids `.`, `[`, `]`, or an empty string. Go's resolver splits every `.` with no escaping (`go/objectmodel/node.go:245-295`), so a declared child `a.b` gets a `Path()` that cannot round-trip to that node. For indexes it passes the substring directly to `strconv.Atoi` (`node.go:233-243`), so spellings such as `values[00]`, `values[+0]`, and `values[-0]` can resolve the same element as `values[0]`. Address tests cover separator aliases but not name escaping or index canonicality (`go/objectmodel/addressing_test.go:76-117`). Go additionally defines relative paths and the empty-string identity, neither present in the normative grammar (`node.go:245-270`).

**Current implementation choice.** Unescaped dot-separated member names; permissive Go integer parsing; non-normative relative lookup extension.

**Specified or accidental?** Accidental. INV-040 intends uniqueness, but neither corpus nor grammar closes the lexical space.

**Consequence.** Some valid schemas produce nodes that cannot be addressed, and evidence can contain noncanonical aliases that another implementation rejects or resolves differently.

### F-10 — Canonical bytes are underdefined for mapping keys and numbers

**Severity:** High. **Classification:** incomplete byte-level norm; **latent cross-implementation ambiguity**.

**Semantic question.** Do scalar quoting rules apply to mapping keys, and which exact decimal string represents each floating-point number?

**Competing interpretations.** Quote keys using the scalar rules vs emit them raw; use shortest-round-trip, normalized exponent, fixed decimal, or language-native float formatting.

**Evidence.** INV-043 says serialization is exact byte for byte and gives scalar quoting conditions plus only two numeric constraints: integers lack a decimal point and floats retain `.` or an exponent (`SPEC.md:902-944`). It does not define exponent sign/zero normalization, negative zero, infinities/NaN as numeric values, or a shortest-round-trip algorithm. Go emits payload keys and primitive member names as raw strings (`go/objectmodel/emit.go:41-58`, `65-83`) even if a key begins with `#` or contains `: `; schema children and opaque mapping keys have no lexical restriction. Values use the quoting rule, but floats use Go's `strconv.FormatFloat(..., 'g', -1, 64)` (`emit.go:168-190`), and Go tests pin language-specific `1e+21` (`go/objectmodel/emit_test.go:74-93`). The corpus contains no numeric float canonicalization cases and only safe mapping keys.

**Current implementation choice.** Raw mapping keys; Go shortest representation with Go exponent conventions.

**Specified or accidental?** Raw keys are an implementation omission relative to the general “Scalars” rule; exact float spelling remains genuinely unspecified.

**Consequence.** Go can emit invalid or semantically changed YAML for hostile-but-legal string keys, and two implementations can satisfy the prose numeric rules while producing different bytes/digests.

### F-11 — Input YAML dialect, tags, and `bytes` have no portable semantics

**Severity:** High. **Classification:** corpus-only parser semantics; **latent cross-implementation ambiguity**.

**Semantic question.** Which YAML version/tag resolution rules govern authoring and schema input, and how is the `bytes` scalar represented and canonicalized?

**Competing interpretations.** YAML 1.1 vs 1.2 booleans/numbers; preserve explicit tags vs decode to host types; represent bytes as `!!binary`, base64 text, or an implementation byte array.

**Evidence.** The model includes `bytes` in `Value` (`SPEC.md:69-74`) and the fixture grammar lists it (`conformance/README.md:56-60`) but provides no syntax or vector. §8.8 quotes strings ambiguous under either YAML version (`SPEC.md:923-944`), but never chooses a version for *input*. Vector 012 implicitly requires unquoted `on`/`off` in authoring input to remain strings (`conformance/materialization/012_discriminator_payload_keywords/input.yaml:2-6`, `expected.yaml:4-10`), which is a YAML-1.2-like corpus rule rather than a prose rule. Go delegates scalar resolution to `yaml.Node.Decode(&any)` and falls back to raw text on decode error (`go/objectmodel/document.go:57-84`); unexpected host values are serialized with `fmt.Sprint` (`go/objectmodel/emit.go:168-190`).

**Current implementation choice.** Whatever `gopkg.in/yaml.v3` resolves into Go `any`, followed by Go-specific fallback serialization.

**Specified or accidental?** Accidental library semantics, except the one `on/off` corpus example.

**Consequence.** Timestamps, explicit tags, binary values, legacy booleans, and non-decimal numeric input can materialize with different types or bytes across parsers.

### F-12 — Schema-less final validation cannot enforce the final-validation contract

**Severity:** High. **Classification:** conflicting normative requirements; **latent cross-implementation ambiguity**.

**Semantic question.** Is §8.7 only an envelope/origin sanity check, or must it prove contract enforcement, schema closure, and primitive completeness?

**Competing interpretations.** Weak key-directed validation under INV-039; schema-aware validation of shape/contract/declared primitives; or a self-describing canonical object whose `shape`/`contract` primitives are interpreted.

**Evidence.** §8.7 says final validation MUST “enforce every contract” and confirm node invariants (`SPEC.md:874-880`). INV-022 requires all schema-declared primitives (`SPEC.md:570-576`) and closure requires all schema-known children to be nodes (`SPEC.md:725-752`). But INV-039 expressly makes schema-less validation key-directed and weaker (`SPEC.md:338-354`). Go's public validation entry accepts object bytes only (`go/objectmodel/materialize.go:115-128`); `validatePayload` validates a mapping/list entry only when it already has a `values` key and otherwise silently skips it (`go/objectmodel/validate.go:97-119`). It never interprets `shape` or `contract`. The machine node schema leaves `values` wholly unconstrained (`spec/node.schema.yaml:21-29`), while `spec/index.yaml:9-14` admits closure and discriminator rules are not expressible there.

**Current implementation choice.** Weak envelope/origin recursive validation; no contract evaluation, no schema closure proof, and no declared-primitive completeness proof.

**Specified or accidental?** Weakness is partly specified by INV-039, but it directly limits the unqualified MUSTs of §8.7 and INV-022/027. The boundary between acceptable weakness and nonconformance is not stated.

**Consequence.** A hand-forged object can be called valid despite raw children or unenforced contracts. A stricter second implementation would reject it and still have strong textual support.

### F-13 — Error stage and code semantics are defined by fixtures, not the normative pipeline

**Severity:** Medium/High. **Classification:** corpus-only semantics, acknowledged as SD-002/SD-015; **latent cross-implementation ambiguity**.

**Semantic question.** What are the normative stage identifiers and error-code mapping for every failure?

**Competing interpretations.** Use numbered §8 stages; add a preceding `schema-load` stage; choose implementation-defined names for unvectorized failures; or require one global code vocabulary.

**Evidence.** The conformance README says the stated code and stage are mandatory (`conformance/README.md:107-123`). Vectors require `schema-load` (`conformance/invalid/004_schema_declares_values_child/expected-error.yaml:2-7`, `invalid/007_sealed_missing_path/expected-error.yaml:2-7`), but §8 starts at entry validation and supplies no stage strings (`SPEC.md:787-888`). Go explicitly invents names for all non-fixture stages and a pre-§8 schema stage (`go/objectmodel/errors.go:5-36`), plus many codes with no vector or name in the specification (`errors.go:38-83`). The open defect records the contradiction (`docs/spec-defects.md:310-331`) and notes the README table is incomplete (`docs/spec-defects.md:564-587`).

**Current implementation choice.** Corpus strings where present; Go-defined stage/code vocabulary elsewhere.

**Specified or accidental?** Exact fixture cases are corpus-specified; the general API/error contract is accidental.

**Consequence.** Independent implementations can agree on accept/reject and canonical bytes but still fail conformance or expose incompatible machine-readable errors.

### F-14 — INV-032 is a language-dependent guarantee that Go cannot satisfy

**Severity:** Critical (boundary integrity). **Classification:** known unsatisfied invariant; **latent cross-implementation asymmetry**.

**Semantic question.** Is conformity defined by unforgeability in the language type system, or by runtime validation/binding at the module boundary?

**Competing interpretations.** Strong type-only guarantee; best available type seal plus runtime checks; or language-specific guarantees with explicitly different strengths.

**Evidence.** INV-032 requires the module input to be constructible only by the materializer and mandates enforcement through each language's type system (`SPEC.md:994-1004`). Go exposes an interface with an unexported marker (`go/objectmodel/materialize.go:3-28`), but interface embedding promotes that method. The repository's own measured defect shows an external forged implementation compiles and explains that a private-field Rust-style newtype would not share the hole (`docs/spec-defects.md:767-792`, `803-858`). The Go boundary therefore distrusts the type, recovers panics, revalidates bytes, and compares canonicalization of the tree to the bytes (`go/module/module.go:21-49`, `62-117`). Even those checks cannot prove the value passed through materialization when a forgery's tree and bytes are mutually consistent (`docs/spec-defects.md:787-792`).

**Current implementation choice.** Unsatisfied type-level invariant plus runtime defense-in-depth.

**Specified or accidental?** The strong requirement is specified but unachievable in Go; the effective runtime contract is implementation compensation not stated in INV-032.

**Consequence.** Two implementations can truthfully offer different integrity guarantees while both are shipped as references. Reviewers may mistake Go's type for an unforgeable capability.

### F-15 — Documentation and tooling present mutually incompatible product states

**Severity:** Medium, with high onboarding/conformance risk. **Classification:** stale/conflicting docs and broken orchestration.

**Semantic question.** Which object shape, implementation count, coverage count, and conformance command should an independent implementer trust?

**Competing interpretations/evidence.**

- `SPEC.md:3-8` and `SPEC.md:1024-1029` say one implementation and no Rust; `README.md:3-4`, `18-36` say two implementations pass. `conformance/README.md:7-10` says no implementation exists and nothing has run.
- The conformance README's canonical example contains the now-invalid root `cic` member and omits the root shape (`conformance/README.md:88-105`), while INV-033 says the version MUST NOT appear in the node tree (`SPEC.md:1033-1054`) and actual expected files start with root `values`, `origin`, `shape` (for example `conformance/materialization/001_origin_yaml/expected.yaml:1-21`).
- The invariant index says INV-005 is “declaration graph acyclic” and INV-010 reserves only `values` (`SPEC.md:1147-1159`), contradicting the body, which permits repeated primitive names and reserves both `values` and `origin` (`SPEC.md:148-160`, `368-379`).
- README claims 44 invariants and a 37/5 coverage split (`README.md:18-25`); the vector map claims “32 of 34” (`docs/spec-vector-map.md:64-65`); the read-only checker reports 46, 39, and 7 while still printing PASS.
- `tools/check_spec_vectors.py` only checks that an invariant is present somewhere in the map and independently tallies `meta.yaml`; it does not validate the map table's vector cell or what a vector proves (`tools/check_spec_vectors.py:95-165`). Thus the visibly blank INV-039 map row (`docs/spec-vector-map.md:57`) passes because validation metas mention it.
- The Makefile header says `mk/rust.mk` is absent (`Makefile:1-7`) although it is present. `make conformance` dispatches Rust to nonexistent target `test-rust` (`Makefile:179-193`), while the actual target is `rust.test` (`mk/rust.mk:31-32`, `70-72`).

**Current implementation/tool choice.** Production code and actual fixture bytes embody the post-0.2 shape; prose spans pre-Go, one-Go, and two-implementation states. The mapping checker validates set membership, not the displayed map. CI runs implementation-specific gates rather than relying on the broken aggregate target (`mk/ci.mk:80-95`).

**Specified or accidental?** Accidental drift. The canonical example is behavior-changing, not merely a status typo.

**Consequence.** A new implementation following the conformance README will emit an object final validation must reject. Reviewers receive a false PASS for a stale coverage document, and the advertised cross-implementation conformance command cannot run both present trees.

## Cross-layer summary

| Semantic surface | SPEC | Corpus | Machine schema | Go | Risk for independent implementation |
|---|---|---|---|---|---|
| Authored primitive override | Contradictory (§4 example vs INV-036) | Requires override | Not represented | Replaces whole primitive | Authorization/provenance divergence |
| Access defaults/domain | Tri-state described | Positive cases only; pins odd origin | Not represented | Injects `inherit`; no domain check | Invalid policy may become canonical |
| Scalar typing/schema grammar | Relies on schema, no grammar | Minimal fixture grammar | Canonical node only | Permissive scalar typing | Different valid schemas/objects |
| Templates | Expansion required, merge/path semantics absent | Pins replacement + mount path | Not represented | Replacement; depth 64 | Different schema/provenance and termination |
| Origin constructor | Four forms in prose | Common valid/invalid sequences | Closed, typed, regex-constrained | Presence-only fields/patterns | Validator disagreement |
| Addresses | Uniqueness required, lexing absent | Common safe names only | Not represented | Dot split + permissive `Atoi` | Unaddressable nodes/aliases |
| Canonical bytes | Detailed but incomplete at keys/floats | Common bytes exact | Not represented | Raw keys + Go float format | Cross-language digest mismatch |
| Final validation | Contracts/closure MUST; schema-less weakness admitted | Negative envelope/origin cases | Values unconstrained | Weak key-directed walk | Forged object acceptance differs |
| Module boundary | Strong type unforgeability | Unvectorizable | Not represented | Runtime compensation | Unequal language guarantees |

## Recommended specification/vector decisions

1. Decide primitive authorability first: forbid it and replace vector 011, or define override/merge and narrow INV-036. Define origin per materialized leaf; an injected value should not claim explicit YAML authorship.
2. Publish a normative schema grammar (or explicitly declare the fixture grammar non-normative) with scalar type checking, optional omission, aggregate default, unknown-key, and template-reference rules.
3. Define access as a closed grammar, including exact types for `inherit` and any openness intended under `rules`.
4. Define template schema composition, descendant source paths, cycle detection, and resource limits independently of implementation language.
5. Reconcile `origin.schema.yaml` with prose and Go: either promote its regex/closed-object constraints to normative text and vectors or remove claims that it encodes the same grammar.
6. Give address members an escaping/identifier grammar and indexes a canonical decimal grammar; add punctuation and leading-zero vectors/API tests.
7. Extend canonical serialization to mapping keys and a language-neutral float algorithm; add floats, negative zero, non-finite policy, and hostile string keys to byte vectors.
8. Split weak schema-less sanity validation from schema-aware “final validation,” or narrow §8.7's contract/closure claims.
9. Restate INV-032 as a boundary property all shipped languages can satisfy, with mandatory runtime validation/binding where type unforgeability is unavailable.
10. Repair the canonical example, invariant index, status/count prose, vector map, and `make conformance`; make the mapping checker validate table cells against `meta.yaml`, not only set membership.

