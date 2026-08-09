# Independent claim audit — `cic-object-model`

## Audit identity and scope

- Repository examined: `/workspace/scratch/103a14c45aa7/cic-object-model-audit`
- Branch/ref observed: `devel`
- Pinned commit: `cbaf928be5362f67a7dbf6378637eba7716ebb5b`
- Commit metadata: `2026-08-09T17:32:47+02:00`, “Merge pull request #14 from CentralInfraCore/spec/release-provenance”
- Repository files were not modified. `git status --short` was empty before and after the audit.
- Examined: `README.md`, all of `SPEC.md`, the conformance corpus and both runners, `docs/` (including English/Hungarian architecture/workflow pages and the defect/vector maps), `tools/check_spec_vectors.py`, release/manifest/review tooling, Make/CI/Docker definitions, the Go object model/module/CLI and tests, the Rust implementation and tests, machine-readable schemas, and release descriptors.

Read-only commands/results used as evidence included:

- `git rev-parse HEAD` → `cbaf928be5362f67a7dbf6378637eba7716ebb5b`
- `python3 tools/check_spec_vectors.py` → `PASS`, reporting **46 invariants, 31 vectors, 39 vector-covered, 7 declared unvectorizable, 8/8 truth-table rows**.
- `python3 tools/release_subject.py verify` → pass; subject `12b70121bbbb44071297fd479548f88de669c87869c93702b6f714ad8f1f7269`, 307 subject files.
- `python3 tools/release_subject.py review` → fail; no matching review record.
- `make -n release-dependency VERSION=v1.0.0` → exit 2, “No rule to make target `release-dependency`.”
- `make -n release-schema VERSION=v1.0.0` → exit 2, “No rule to make target `release-schema`.”
- Host execution of Go/Rust/pytest suites was unavailable: the audit environment had no `go`, `cargo`, `rustc`, or Docker executable, and its Python environment lacked pytest/jsonschema. This is a limit on fresh execution, not evidence that the checked-in suites fail. Static control/data-flow findings below are labeled accordingly.

## Executive assessment

The pinned tree contains substantial, unusually candid self-critique, and two narrow facts do hold: the release-subject verifier matches the current tracked tree, and the mapping checker reports success on its own limited invariant-ID model. However, several broader claims are false or materially overstated.

The most serious defects are process-level: the external-review gate is self-invalidating for a tracked review record, and the inherited `make release` path neither preserves the whole-tree release subject nor can validate the descriptor it writes. At the model/implementation level, the supposedly exact address and serialization rules have no escaping/quoting for arbitrary string keys, and Go’s INV-032 construction guarantee is expressly impossible in Go. The two implementations also demonstrably disagree outside the 31-vector sample.

Passing the current corpus therefore proves the 31 recorded examples, not general conformance to all normative statements or cross-implementation equivalence over the admitted input space.

## Findings

### F-01 — The external-review gate cannot be satisfied by an ordinary tracked review record

**Claim.** A release is preceded by an independent review bound to the exact release subject; `make review.check` and CI enforce that binding.

**Evidence.**

- `SPEC.md:1114-1117` requires a review record against the subject digest.
- `reviews/README.md:3-10` says the record is named `<subject-digest>.md` and CI checks it.
- `tools/release_subject.py:48-52,66-82` excludes only `MANIFEST.sha256` and `project.yaml`; therefore a tracked file under `reviews/` is part of the subject.
- `tools/release_subject.py:190-205` recomputes the current subject, then requires `reviews/<current-subject>.md`.
- `.github/workflows/ci.yml:78-87` runs that check for every PR into `main`.
- The positive unit test creates the review at `tests/test_tools/test_release_subject.py:173-178` but never `git add`s it. Because subject enumeration is `git ls-files`, the test record is deliberately or accidentally invisible to the subject calculation.
- Independent no-write calculation against the pinned tree:

  ```text
  subject before record: 12b70121bbbb44071297fd479548f88de669c87869c93702b6f714ad8f1f7269
  subject after tracking reviews/<before>.md: 49e061b1721ba07a7f4952a77c45a65d653e7fadc1f988f7fde50ed7d2a199be
  record required after tracking: reviews/49e061...a199be.md
  record actually created:       reviews/12b701...7269.md
  matches: False
  ```

**Observed reality.** An untracked local record can satisfy `cmd_review`, which is why the test passes. A review record committed to the PR changes the digest, so its filename no longer matches. Adding the newly required record changes it again. A clean CI checkout cannot contain an untracked review file.

**Classification.** **False** (and the positive test is a false positive for the real tracked-file workflow).

**Consequence.** PRs into `main` are effectively ungateable by the documented mechanism, aside from finding a SHA-256 fixed point or injecting an untracked file during CI. INV-046 is not operationally enforceable as implemented. Review records must be excluded from the subject, stored out-of-band, or bound through a non-self-referential signed envelope.

### F-02 — The documented release path does not implement INV-045 and is internally unable to finalize this repository

**Claim.** A release signature covers the whole normative product; `project.yaml`’s `buildHash` is that subject, and `make release` fills/signs the release metadata.

**Evidence.**

- `SPEC.md:1072-1077` requires a digest over SPEC, machine schemas, every vector, and every shipped implementation.
- `project.yaml:21-25,39-52` says `make release` fills the placeholders and that `buildHash` is the whole-tree subject.
- `Makefile:78-89` routes `make release` to `tools.compiler release`.
- The actual preparation path loads one canonical source and hashes only `source_data["spec"]` (`tools/infra.py:142-175`), then explicitly writes `buildHash: ""` and also adds a top-level `spec` member to `project.yaml` (`tools/infra.py:181-205`). It asks the operator to run a binary build and refill the hash (`tools/infra.py:223-233`).
- Finalization validates that modified descriptor (`tools/infra.py:289-309`) and re-signs a five-field metadata subset including whatever `buildHash` was supplied (`tools/infra.py:254-287`). It never invokes `release_subject.py` or verifies that `buildHash` is the subject required by INV-045.
- The descriptor cannot validate against its own declared schema: current `project.yaml:56-64` has `compiler_settings.repo_type: spec`, while `project.schema.yaml:176-190` permits only `schema`, `workflow`, or `module`. In addition, `project.schema.yaml:5-9` sets `additionalProperties: false` and only declares `metadata`, `compiler_settings`, and `release`; preparation adds the forbidden top-level `spec` at `tools/infra.py:203-205`.
- `project.schema.yaml:114-120` still describes `buildHash` as `sha256(module/module.wasm)`, contradicting the whole-tree subject meaning in `project.yaml`.

**Observed reality.** `release_subject.py verify` correctly checks the currently committed descriptor claim, but it is a separate verifier, not integrated with the release state machine. The release state machine is inherited WASM/schema machinery: it clears the good subject, expects a build artifact, writes schema-invalid structure, and signs a different payload.

**Classification.** **False** for the end-to-end release claim; the standalone subject verifier itself is **true but partial**.

**Consequence.** The repository cannot produce a demonstrably INV-045-compliant release through its documented `make release` path. Even before cryptographic verification, final descriptor validation should fail. A compliant path needs to compute the subject after the normative tree is frozen, preserve it, sign exactly that digest, and use a descriptor schema for `repo_type: spec`.

### F-03 — Both implementations pass the sample corpus, but they do not agree over the admitted/error input space

**Claim.** The two independent reference implementations mutually check semantics, making it awkward to alter one without the corpus and the other noticing (`README.md:29-32,58-62`; `docs/en/architecture.md:24-34`; Hungarian equivalent `docs/hu/architecture.md:15-25`).

**Evidence.**

- The corpus contains 13 materialization, 9 invalid, and 9 validation directories; both runners hard-code those counts (`go/conformance/conformance_test.go:88-110`; `rust/tests/conformance.rs:26-32`). Both compare the 13 positive outputs byte-for-byte (`go/conformance/conformance_test.go:196-240`; `rust/tests/conformance.rs:88-116`). That supports only the stated corpus result.
- For a list position given a scalar short-form payload, Go rejects it as a type mismatch at entry validation (`go/objectmodel/entry.go:122-139`). Rust takes `source.and_then(Value::as_seq)`, silently executes no loop when it is not a sequence, and returns an empty list (`rust/src/materialize.rs:303-319`). The result is then structurally valid and can be returned.
- For an absent, optional, non-defaulted declaration, Go deliberately removes the node (`go/objectmodel/defaults.go:15-18,21-37,39-57`). Rust materializes absent scalar as `null`, absent list as empty, and walks all object children (`rust/src/materialize.rs:296-320,331-370`); its test explicitly requires an unauthored list to materialize empty (`rust/tests/rejections.rs:288-297`).
- `docs/spec-defects.md:486-506` admits that the absent optional case is not decided by the specification, listing omit/null/reject as three possible readings.
- Nested sealed template content is another uncovered divergence risk: Rust constructs a child and then overwrites its payload with `Payload::Scalar(cv.clone())` regardless of the child’s declared shape (`rust/src/materialize.rs:351-367`). Its scalar emitter treats a collection reaching that path as a routing bug, panics in debug, and emits `null` in release (`rust/src/canonical.rs:173-181`). Go recursively constructs content according to shape (`go/objectmodel/construct.go:47-110,112-151`). Vector 003 covers only a scalar content child.

**Observed reality.** The exact statement “both pass all 31 vectors” is supported by runner structure and repository history, though it could not be freshly executed in this host. The broader mutual-equivalence/reference-implementation claim is not: simple uncovered cases diverge, including one where Rust silently converts wrong-shaped input into an empty list.

**Classification.** Corpus claim: **unproven in this audit but narrowly specified**. General mutual-check/reference-equivalence implication: **overstated/partially true**.

**Consequence.** Consumers cannot infer equal acceptance/rejection or output outside the corpus. Add cross-language differential cases for every shape mismatch, absence/default combination, and nested template-content shape; first specify SD-007’s missing rule.

### F-04 — Exact addressing and canonical serialization fail for legal string keys because neither grammar nor emitters define escaping

**Claim.** Every node has exactly one implementation-independent address (INV-040), and canonical serialization is defined byte-for-byte (INV-043/044) (`SPEC.md:181-224,902-976`; `README.md:34-41`).

**Evidence.**

- The node grammar permits `Map<String,CICNode>` (`SPEC.md:61-74`) and opaque payloads may be arbitrary. No lexical restriction or escape production is stated for `<payload child>` in the address grammar (`SPEC.md:189-198`).
- Go constructs paths by raw concatenation of child names (`go/objectmodel/construct.go:142-150`; primitive equivalent `go/objectmodel/primitives.go:120-129`) and later splits on every dot (`go/objectmodel/node.go:270-295`). Rust does the same (`rust/src/materialize.rs:351-357`; `rust/src/node.rs:181-201`). A declared child named `a.b` therefore prints a path that both resolvers parse as two children and cannot round-trip.
- Both canonical emitters write mapping keys verbatim, without applying their scalar-quoting rules: Go `go/objectmodel/emit.go:65-83`; Rust `rust/src/canonical.rs:63-74,100-112`. Their quote helpers are used only for values.
- A legal quoted opaque key such as `'a: b'` is consequently serialized as `a: b:`, which is not the same YAML mapping and is generally malformed. Empty keys, leading indicators, embedded newlines, and other scalar forms have analogous failures.
- Go validates the in-memory projection before serialization and never reparses the emitted bytes (`go/objectmodel/materialize.go:101-112`), so it can return malformed “canonical YAML.” Rust does revalidate emitted bytes (`rust/src/materialize.rs:61-67`), so the same case is likely to become another Go/Rust divergence rather than a shared result.

**Observed reality.** The 13 materialization vectors use safe identifiers and cannot establish the universal key/address claims. INV-040 is false for permitted names containing `.`; INV-043 is false for permitted keys requiring YAML quoting.

**Classification.** **False** as a universal claim; corpus coverage is **insufficient**.

**Consequence.** Evidence paths can be unresolvable or ambiguous, and canonical output can cease to be valid YAML. Define an identifier restriction or an escape/length-prefixed address grammar, and apply the canonical scalar renderer to every mapping key in both implementations.

### F-05 — Go does not and cannot satisfy the normative INV-032 type-system guarantee

**Claim.** A module input value is constructible only by the core materializer, and both Go and Rust enforce this in their type systems (`SPEC.md:994-1004`). The Go interface comment still says no outside package can implement or produce it (`go/objectmodel/materialize.go:3-13`).

**Evidence.**

- Go promotes an embedded interface’s methods, including the unexported marker. The repository’s own analysis and adversarial tests demonstrate an external struct embedding `objectmodel.CanonicalObject` satisfies the interface.
- `go/module/module.go:21-40,62-117` explicitly says interface embedding defeats the type claim and compensates by panic recovery, byte validation, and tree/byte binding.
- `docs/spec-defects.md:767-792,803-857` labels SD-019 “Still open,” shows the compiling forgery, and states the strong guarantee cannot be restored in Go.
- The vector map nevertheless describes a direct compile-fail check as if it established unconstructibility (`docs/spec-vector-map.md:82`); that artifact tests one attempted implementation, not the embedding construction that actually compiles.

**Observed reality.** Runtime defenses substantially reduce the integrity consequence, but a value of the interface type remains externally constructible. The specification’s construction-level statement is false in Go.

**Classification.** **False**, explicitly acknowledged by the repository.

**Consequence.** Go cannot be both an implementation of the literal INV-032 and the implementation shipped here. The invariant should specify achievable boundary behavior (validate, bind representations, reject/panic-proof) and document the stronger Rust-only construction property separately.

### F-06 — “Every normative statement is mapped” is beyond what the mapping gate checks; the current coverage documents are numerically stale

**Claim.** Every normative statement is vector-backed or justified as unvectorizable, and `check_spec_vectors.py` fails on drift (`SPEC.md:1017-1022`; `docs/spec-vector-map.md:3-8`; `README.md:102-105`). README further calls the tool “negative-tested” (`README.md:23`).

**Evidence.**

- `tools/check_spec_vectors.py:44-50` obtains its universe solely by regexing `INV-nnn` tokens after the section-12 index heading. It does not parse RFC-2119 statements. Unnumbered MUST/FAILURE clauses cannot enter C1/C3 at all.
- The spec contains normative, unnumbered behavior, e.g. primitive structure “MUST” use nested groups (`SPEC.md:595-610`) and pipeline FAILURE/MUST clauses (`SPEC.md:797-888`). The repository itself previously recorded this blind spot at `docs/spec-defects.md:297-302`.
- Vector discovery is `conformance.glob("*/*/meta.yaml")` (`tools/check_spec_vectors.py:67-74`). A vector directory missing `meta.yaml` is invisible, so C4 cannot enforce its own “every vector directory has required files” promise (`tools/check_spec_vectors.py:129-139`).
- The checker accepts any nonempty final table cell as an unvectorizable justification (`tools/check_spec_vectors.py:77-92`); it cannot evaluate whether the stated alternate check exists or proves the claim.
- No test under `tests/` references `check_spec_vectors`; repository-wide search found only the tool, docs, Make target, and comments. `tools/mutate.py:51-108` mutates seven Go behaviors, not this mapper. Thus “negative-tested” is unsupported by checked-in tests.
- Actual run: 46 invariants, 31 vectors, 39 with vectors, 7 unvectorizable, PASS. Yet `README.md:20-23` says 44 invariants and 37+5; `docs/spec-vector-map.md:64-65` says 32 of 34 with two remaining; `docs/en/architecture.md:9-16` says 34 invariants and 27 vectors.

**Observed reality.** The gate accurately checks a limited ID-to-metadata bookkeeping model and currently passes it. It cannot substantiate “every normative statement,” vector semantics, alternate-check sufficiency, or even a vector directory whose metadata file vanished.

**Classification.** Gate-pass claim: **true**. General completeness and negative-testing claims: **overstated/unproven**. Counts: **stale/false**.

**Consequence.** A green mapper can coexist with untested normative behavior and misleading coverage totals. Parse/index every normative clause (or require every RFC keyword to belong to an invariant), test the checker negatively, and derive displayed totals rather than hand-copying them.

### F-07 — The authoritative/current-status documents contradict the actual pinned tree

**Claim.** Status statements tell readers how many implementations have executed the corpus.

**Evidence.**

- `README.md:3-4,24-30` says two implementations exist and both pass all 31 vectors.
- The authority itself says “one implementation” at `SPEC.md:3-8` and repeats that Rust does not exist at `SPEC.md:1024-1029`.
- `conformance/README.md:7-10` says no implementation exists and nothing has ever run.
- `docs/spec-vector-map.md:10-13` says Rust does not exist and agreement is untested.
- In the same commit, `rust/Cargo.toml`, a full `rust/src/`, `rust/tests/conformance.rs`, and `mk/rust.mk` exist. Git history shows the Rust implementation landed before this pinned commit.
- Source comments also remain time-inconsistent, e.g. `Makefile:4-7` says `mk/rust.mk` is not present and CI is Go-only, although it is included at `Makefile:13` and used by `mk/ci.mk:90-94`.

**Observed reality.** These are not nuanced differences in assurance; mutually exclusive existence/execution claims coexist in primary reading paths.

**Classification.** **Stale/false**.

**Consequence.** A reader following the declared authority or conformance guide gets the wrong implementation status, while the README says the opposite. Status should be generated from runner discovery/CI artifacts or updated atomically with implementation changes.

### F-08 — “`make ci` is the same pipeline locally and in Actions” is only partly true

**Claim.** `make ci` is the whole pipeline; a green badge and green local run mean the same thing (`README.md:27,85-88`; `mk/ci.mk:1-29`).

**Evidence.**

- The central CI gate does call `make ci` (`.github/workflows/ci.yml:74-76`), and core pass/fail commands are defined in `mk/ci.mk:26-95`. This supports command centralization.
- Actions adds a separate pass/fail step, `make review.check`, for PRs into main (`.github/workflows/ci.yml:78-87`). `review.check` is deliberately not in `ci.gates` (`Makefile:153-160`). Therefore a local `make ci` does not run the full main-PR gate.
- Reproducibility is also weaker than “same” implies: the Python builder base is the moving tag `python:3.11-slim`, apt packages are unpinned, the Go tarball is downloaded without an in-repo checksum, and `pip-tools` itself is unversioned (`Dockerfile:1-18,25-38`). Rust’s base image is digest-pinned, but `cargo-llvm-cov` and `cargo-deny` are installed without pinned versions at runtime (`mk/rust.mk:74-83`).
- The host here could not execute `make ci` because Docker was absent; this does not classify the repository, but means no fresh end-to-end result is claimed by this audit.

**Observed reality.** The main command graph is shared, which is useful. The full Action decision is not equal to local `make ci`, and the environment/tools can move over time.

**Classification.** **Partially true / overstated**.

**Consequence.** A green local run can still fail the main PR Action (currently it will, due F-01), and a later run can use different tool versions. Describe the equivalence as “shared core gate,” provide a local main-PR target, and pin/verify build inputs.

### F-09 — The English and Hungarian release workflow documents prescribe nonexistent targets

**Claim.** The workflow and “comprehensive” Make cheatsheets tell users how to create signed releases (`docs/en/workflow.md:83-110`; `docs/en/makefile-cheatsheet.md:1-24`; Hungarian mirrors).

**Evidence.**

- English workflow directs `make release-dependency` at `docs/en/workflow.md:90-95`; Hungarian does the same at `docs/hu/workflow.md:91-94`.
- English cheatsheet advertises `release-dependency` and `release-schema` at `docs/en/makefile-cheatsheet.md:21-24`; Hungarian at `docs/hu/makefile-cheatsheet.md:21-24`.
- The actual Makefile defines `release`, not either target (`Makefile:78-90`).
- Read-only dry runs at the pinned commit returned exit 2 for both advertised targets: “No rule to make target …”.
- The cheatsheets omit the repository’s central `ci`, conformance, Go, Rust, release-subject, and review targets, despite calling themselves comprehensive.

**Observed reality.** The documented release commands cannot start. The only extant `make release` path has the separate defects in F-02.

**Classification.** **False/stale**.

**Consequence.** Users following either language’s documentation immediately fail or may improvise around release controls. Replace inherited base-repo workflow text with spec-repository-specific commands and tested examples.

### F-10 — “TestVersionIdentity holds every other declaration” exceeds what the test enumerates

**Claim.** `TestVersionIdentity` holds every other version declaration in the repository to `SPEC.md` (`README.md:14-16`).

**Evidence.**

- `go/module/version_test.go:47-136` checks two Go constants, two fields in `spec/index.yaml`, the major/minor portion of `project.yaml`, and vector schemas.
- It does not inspect `rust/src/lib.rs:34-36` (`MODEL_VERSION`), `rust/Cargo.toml:1-6`, or arbitrary other declarations. Rust has its own narrower test, but that does not make the named Go test exhaustive.
- Current inspected declarations happen to agree (`0.2` / package `0.2.0`); the issue is the strength of the guard claim, not a current value mismatch.

**Observed reality.** The test guards a useful hand-maintained list, not “every declaration.” New declaration sites can drift unless separately added.

**Classification.** **Overstated** (current values observed consistent).

**Consequence.** The exact propagation failure the test was created to prevent can recur in unenumerated sites. Derive declarations from one generated source or scan/parse all registered declaration locations in both implementations.

### F-11 — “Implemented/reference implementation” overstates value-shape validation, especially in Go

**Claim.** A canonical node’s `Value` is scalar, list-of-nodes, map-of-nodes, or opaque, and `shape`/`scalar_type` type positions (`SPEC.md:61-74`; conformance schema language `conformance/README.md:52-64`). Final validation “enforce[s] every contract” (`SPEC.md:874-880`).

**Evidence.**

- Go accepts every non-mapping value at a scalar position without checking that it is a scalar or matches `scalar_type` (`go/objectmodel/entry.go:122-127`; `go/objectmodel/construct.go:47-56`). A YAML sequence can therefore be stored in `kindScalar` while the emitted `shape` primitive says scalar/integer.
- Defaults at scalar positions are also copied without subtype checking (`go/objectmodel/defaults.go:75-87`).
- `scalar_type` is parsed and emitted but never used for validation: its implementation references are declaration parsing and shape projection (`go/objectmodel/schema.go:63-75,185-189`; `go/objectmodel/primitives.go:62-76`). Rust similarly projects it without enforcing it (`rust/src/schema.rs:47,198`; `rust/src/materialize.rs:462-472`).
- Schema-less final validation checks envelope membership, origin grammar, and recursively key-identified nodes, but has no schema or contract with which to confirm declared shape/subtype (`go/objectmodel/validate.go:24-36,39-119`). The spec admits key-directed validation is weaker (`SPEC.md:338-354`) while still requiring “every contract.”

**Observed reality.** “Scalar” can contain a collection in both implementations’ internal scalar variants; declared scalar subtypes are descriptive output rather than enforced semantics. Schema-less validation cannot enforce every contract by construction.

**Classification.** **Partially true / underspecified**, with a **false implementation implication** if `scalar_type` is intended as a constraint.

**Consequence.** Modules may receive canonical objects whose declared shape/type disagrees with their payload. The normative schema language must say whether and how scalar subtypes constrain values, and implementations need schema-aware final validation or an explicit narrower validation claim.

## Confirmed positives and bounded claims

- The pinned commit and working tree were clean and matched the requested hash.
- The current `MANIFEST.sha256` and the subject claim are internally consistent according to `tools/release_subject.py verify`. The tool correctly states that this does **not** establish who produced/signed the tree (`tools/release_subject.py:177-187`).
- Both conformance runners use the same corpus paths and assert nonzero fixed group counts. Positive materialization comparisons are byte-for-byte, not semantic-only.
- Duplicate-key and YAML-anchor/alias rejection are implemented before generic tree conversion in Go (`go/objectmodel/document.go:29-54,87-126`) and in Rust’s custom YAML bridge; the corpus includes one negative vector for each.
- Go’s module boundary adds meaningful runtime defenses against the known interface-forgery limitation: panic recovery, version check, byte validation, and tree/byte rebinding (`go/module/module.go:62-117`). Those mitigations should be preserved even after INV-032 is rewritten.

## Overall classification of the repository’s main public claims

| Public claim | Audit classification |
|---|---|
| `SPEC.md` is normative authority | Stated, but internally/openly contains at least one impossible Go requirement and stale status |
| 46 invariants / 31 vectors | Actual counts are 46/31; README/map/docs counts are stale |
| Both implementations pass the corpus | Plausibly supported by checked-in runners/history; not freshly executable in this host |
| Two reference implementations generally agree/check each other | Partially true only over the corpus; false as a general behavioral implication |
| Canonical bytes and addresses are universally defined | False for unrestricted string keys/names |
| Mapping gate covers every normative statement | Overstated; it covers indexed invariant IDs and metadata bookkeeping |
| `make ci` equals the complete Actions gate | Partially true for the shared core command; false for main-PR review and hermeticity |
| Whole-product release provenance is enforced | Standalone digest check works; actual release and review workflows do not |
| English/Hungarian release docs are actionable | False/stale |

## Recommended remediation order

1. Break the review-record hash cycle and add a test that commits (`git add`s) the record before checking.
2. Replace or isolate the inherited release state machine; make one command freeze, compute, verify, review-bind, and sign the INV-045 subject.
3. Define key/name lexical rules or escaping, then fix both path resolvers and both canonical key emitters; add adversarial cross-language vectors.
4. Decide absent optional and scalar subtype semantics; add shape-mismatch/default/template-content differential vectors.
5. Rewrite INV-032 to an achievable cross-language boundary contract while retaining Go’s runtime defenses.
6. Make normative-clause coverage machine-visible beyond `INV-nnn` presence; negative-test the checker.
7. Regenerate all status/count documentation and replace the inherited release workflow/cheatsheets.

