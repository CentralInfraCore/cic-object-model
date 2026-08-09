# Independent adversarial boundary audit

## Pin and environment

Repository: `/workspace/scratch/103a14c45aa7/cic-object-model-audit`

Pinned commit: `cbaf928be5362f67a7dbf6378637eba7716ebb5b`

The repository was not modified; `git status --short` was empty.

```text
$ git rev-parse HEAD
cbaf928be5362f67a7dbf6378637eba7716ebb5b
$ uname -a
Linux 75d8070603e0 6.18.35 #1 SMP Mon Jul 27 18:07:50 UTC 2026 x86_64 GNU/Linux
$ go version
/bin/bash: go: command not found
$ rustc --version
/bin/bash: rustc: command not found
$ python3 --version
Python 3.12.13
```

The image has no Go/Rust toolchain or container runtime. Findings below are source-proven control-flow counterexamples with minimal reproducers. Runtime magnitude that could not be measured is marked inconclusive. PyYAML 6.0.3 was available for secondary serialization checks.

## Executive result

Seven concrete counterexamples:

1. **High:** Rust parses `required: true` but never enforces it; a missing required value becomes a validated null node.
2. **High:** Rust silently converts an authored non-sequence at a list position to an empty list.
3. **High:** Rust accepts a collection at a scalar position; debug serialization panics and release serialization changes it to null.
4. **High:** Go `Materialize` can return a `CanonicalObject` whose bytes are invalid YAML because mapping keys are never quoted.
5. **Medium:** Rust final validation accepts scalar primitive members, violating INV-003.
6. **Medium:** canonical validation ignores trailing YAML documents.
7. **Structural/High:** Go INV-032 is still forgeable; stateful interface methods defeat the compensating two-snapshot check.

Resource review also found a source-proven quadratic Rust pre-scan and unbounded recursive surfaces whose actual crash threshold is inconclusive.

## F-01 — Rust ignores required fields

**Claimed invariant.** SPEC §8.5 requires rejection when a required value is neither authored nor defaultable (`SPEC.md:850-858`). This is also part of INV-031(e,g)'s materializer-to-module guarantee.

**Attack construction.**

```yaml
model: "0.2"
root:
  shape: object
  children:
    secret:
      shape: scalar
      scalar_type: string
      required: true
```

Input: `{}`.

**Observed result.** Rust stores the flag at `rust/src/schema.rs:44-50,194-203`. No production code reads `SchemaNode.required`. `rust/src/materialize.rs:324-370` builds the absent child; `rust/src/materialize.rs:296-302` converts absence to `Value::Null`; `rust/src/materialize.rs:389-392` labels it `Origin::Schema`. Final validation accepts the resulting node. Go has an explicit rejection at `go/objectmodel/defaults.go:19-37`.

```text
$ rg -n "required" rust/src
rust/src/bin/cic_materialize.rs:100:            "-schema and -input are required, or -validate".into(),
rust/src/schema.rs:31:    "required",
rust/src/schema.rs:49:    pub required: bool,
rust/src/schema.rs:200:            "required" => n.required = matches!(v, Value::Bool(true)),
```

**Minimal reproducer.** Save the schema and input and run:

```sh
cargo run --manifest-path rust/Cargo.toml --bin cic-materialize -- -schema /tmp/f01-schema.yaml -input /tmp/f01-input.yaml
```

The implemented branch emits `$.values.secret.values: null` with `origin: [schema]` rather than `E_REQUIRED_VALUE_MISSING`.

**Violated guarantee.** Requiredness and INV-031(e,g). The sole materializer manufactures an object that should not exist, so a downstream type credential cannot repair it.

**Severity: High.** Silent config/policy defaulting can change authorization semantics.

## F-02 — Rust silently changes wrong-typed list input to empty

**Claimed invariant.** A declared list has a sequence payload; type mismatch must reject and authored data must not disappear (INV-027, SPEC §4.3/§8.4).

**Attack construction.**

```yaml
# schema
model: "0.2"
root:
  shape: object
  children:
    groups:
      shape: list
      item:
        shape: scalar
        scalar_type: string
```

```yaml
# input
groups: administrators
```

**Observed result.** `discriminate` preserves the non-mapping short form (`rust/src/materialize.rs:197-235`). The list builder uses `source.and_then(Value::as_seq)`; on the scalar it skips the loop and returns the initially empty vector (`rust/src/materialize.rs:303-318`). Origin is still YAML (`rust/src/materialize.rs:389-390`). Go explicitly rejects at `go/objectmodel/entry.go:122-139` and `go/objectmodel/construct.go:69-85`.

**Minimal reproducer.** Run the Rust CLI with the two files. The implemented branch emits `groups.values: []`, `groups.origin: [yaml]`; `administrators` disappears.

**Violated guarantee.** Type correctness, INV-027, and truthfulness of provenance at the module handoff.

**Severity: High.** Silent allow/deny-list deletion is materially security relevant.

## F-03 — Rust scalar/collection confusion: debug panic, release corruption

**Claimed invariant.** A scalar position must contain a scalar. Untrusted input must reject rather than crash or change type.

**Attack construction.**

```yaml
# schema
model: "0.2"
root:
  shape: scalar
  scalar_type: string
```

```yaml
# input
- x
```

**Observed result.** The sequence is accepted as short-form scalar payload (`rust/src/materialize.rs:219-235,296-302`). Serialization routes `Payload::Scalar` to `scalar()` (`rust/src/canonical.rs:55-60`). Its collection arm executes `debug_assert!(false, ...)` and otherwise returns `"null"` (`rust/src/canonical.rs:173-180`).

**Minimal reproducer.**

```sh
cargo run --manifest-path rust/Cargo.toml --bin cic-materialize -- -schema /tmp/f03-schema.yaml -input /tmp/f03-input.yaml
cargo run --release --manifest-path rust/Cargo.toml --bin cic-materialize -- -schema /tmp/f03-schema.yaml -input /tmp/f03-input.yaml
```

The first panics at `rust/src/canonical.rs:179`; the release branch serializes `values: null`.

**Violated guarantee.** Crash safety, scalar arity, deterministic cross-profile behavior, and value preservation.

**Severity: High.** Tiny-input DoS in debug/test deployments; silent corruption in release.

## F-04 — Go can return invalid YAML as a canonical object

**Claimed invariant.** `Materialize` is the only producer of validated canonical objects (`go/objectmodel/materialize.go:47-54`); INV-043 requires valid byte-exact YAML (`SPEC.md:902-944`).

**Attack construction.**

```yaml
model: "0.2"
root:
  shape: object
  children:
    "a: b":
      shape: scalar
      scalar_type: string
      default: x
```

Input: `{}`.

**Observed result.** Parsing preserves the string key (`go/objectmodel/document.go:57-71`). The emitter writes mapping keys using `b.WriteString(k)` without scalar quoting (`go/objectmodel/emit.go:65-83`). Go validates the projected tree before serialization and returns the bytes without reparsing (`go/objectmodel/materialize.go:101-112`). The output necessarily contains:

```yaml
values:
  a: b:
    values: x
```

Actual secondary-parser output:

```text
$ python3 <PyYAML 6.0.3 parser harness>
ScannerError: mapping values are not allowed here
```

Rust has the same unquoted-key emitter (`rust/src/canonical.rs:63-74,103-112`) but reparses before returning (`rust/src/materialize.rs:61-71`), so it rejects/panics rather than handing out the malformed object.

**Minimal reproducer.**

```sh
go run ./cmd/cic-materialize -schema /tmp/f04-schema.yaml -input /tmp/f04-input.yaml > /tmp/f04-object.yaml
python3 -c 'import yaml; yaml.safe_load(open("/tmp/f04-object.yaml"))'
```

**Violated guarantee.** The only Go constructor can return `Validated<Canonical<CICObject>>` whose canonical bytes are not YAML. `-deliver` subsequently rejects the materializer's own object.

**Severity: High.** Short schema-controlled names break interchange, hashing, and module delivery.

## F-05 — Rust validator accepts scalar primitive members

**Claimed invariant.** INV-003 says every primitive member is itself a CIC node (`SPEC.md:110-113`).

**Attack construction.**

```yaml
---
values: ok
origin: [yaml]
shape: 7
```

**Observed result.** The Rust primitive arm recurses only if the member is a map and accepts it otherwise (`rust/src/validate.rs:82-86`). There is no later shape check. Go rejects the same object at `go/objectmodel/validate.go:79-91`.

**Minimal reproducer.**

```sh
cargo run --manifest-path rust/Cargo.toml --bin cic-materialize -- -validate /tmp/f05-object.yaml
```

Control flow reaches `Ok(())`, so the CLI prints `valid`.

**Violated guarantee.** INV-003 and schema-less final validation.

**Severity: Medium.** Direct public validation bypass; high if validation is treated as a credential.

## F-06 — validators ignore trailing YAML documents

**Claimed invariant.** INV-043 requires one document per object (`SPEC.md:904-909`).

**Attack construction.**

```yaml
values: 1
origin: [yaml]
---
values: attacker-controlled
origin: []
```

**Observed result.** Rust loads all documents and explicitly selects only `docs.into_iter().next()` (`rust/src/value.rs:155-175`). Go calls single-document `yaml.Unmarshal` once and never checks a second document (`go/objectmodel/document.go:29-54`). A secondary parser confirms the byte string contains two documents:

```text
$ python3 <PyYAML safe_load_all harness>
documents= 2
```

**Minimal reproducer.** Run each CLI with `-validate /tmp/f06-object.yaml`. Rust necessarily validates only the first tree. Go runtime confirmation is pending toolchain availability, though the one-shot API path is direct.

**Violated guarantee.** Validation authenticates only a prefix of the supplied bytes. A multi-document downstream consumer can observe unvalidated data.

**Severity: Medium.** Parser-differential/validation-prefix vulnerability.

## F-07 — Go INV-032 forge plus stateful TOCTOU

**Claimed invariant.** INV-032 says the module-input value is constructible only by the materializer (`SPEC.md:994-1004`). `Execute` claims one parse ensures what the module reads was validated and checks tree/bytes equality (`go/module/module.go:96-117`).

**Attack construction.**

```go
type flip struct {
    objectmodel.CanonicalObject
    root *objectmodel.Node
    good, evil []byte
    calls int
}
func (f *flip) ModelVersion() string { return module.ModelVersion }
func (f *flip) Root() *objectmodel.Node { return f.root }
func (f *flip) CanonicalYAML() []byte {
    f.calls++
    if f.calls <= 2 { return f.good }
    return f.evil
}
```

Use a real materialized root and `good = objectmodel.Canonicalize(root)`. Embedding promotes the private marker, as the repository already acknowledges in SD-019 and `go/module/module.go:88-99`.

**Observed result.** `Execute` calls `CanonicalYAML` exactly twice (`go/module/module.go:100,115`): the valid first result passes validation and the valid second result passes equality. It returns nil. A third read by real module code returns `evil` without validation.

**Minimal reproducer.** In an external Go module, instantiate the type above from any fixture, call `module.Execute(f)`, then print `err`, `f.calls`, and a third `CanonicalYAML()`. Direct call count gives nil, 2, then evil.

**Violated guarantee.** INV-032 is false, and compensating validation binds two snapshots rather than a stable object. A module must obey a convention (“never call/retain the interface again”) that INV-032 was designed to eliminate.

**Severity: Structural/High.** The sample Execute has no real sink after validation, but any module logic added after line 117 inherits the hazard.

## F-08 — Rust's claimed-linear pre-scan is quadratic

**Claimed property.** Comments say the alias/duplicate scan is linear in input size (`rust/src/value.rs:178-188`).

**Attack construction.** A flat mapping with many unique keys; no aliases or duplicates.

```sh
python3 - <<'PY' > /tmp/f08.yaml
for i in range(100000): print(f'k{i}: 0')
PY
```

**Observed result.** Every new key executes `frame.keys.contains(&k)` on a `Vec<String>` before push (`rust/src/value.rs:232-257`): exactly `n(n-1)/2` prior-key equality tests. `Map::insert` adds a second linear search per key (`rust/src/value.rs:64-81,293-310`).

```text
1000 499500
10000 49995000
100000 4999950000
```

**Violated guarantee.** The amplification preconditioner is not linear.

**Severity: Medium DoS risk.** Exact wall-time threshold is unmeasured.

## Additional concrete gaps

**Origin grammar is permissive.** Both validators accept extra constructor members and non-scalar `template`/`path`. Go only checks presence/non-nil (`go/objectmodel/validate.go:167-186`); Rust converts collections to placeholder strings (`rust/src/validate.rs:249-282`, `rust/src/value.rs:119-130`). This falsifies “exactly four productions” for standalone validation, but Go's stable module comparison blocks it.

**Go stringifies non-string mapping keys.** `fromNode` uses a key node's textual `.Value` without checking its tag (`go/objectmodel/document.go:66-70`); Rust explicitly rejects non-string keys (`rust/src/value.rs:293-307`). A schema declaring string child `'1'` makes authoring key `1:` partly legitimate in Go, causing cross-implementation divergence.

## Recursive/deep structures and budgets — inconclusive, not defended

No general input-byte, node-count, nesting-depth, output-byte, time, or memory budget exists at the Go or Rust public entry points. Recursive walks include Go `checkNode`, `entryWalk`, `construct`, `materializeDefaults`, `evaluatePrimitives`, `validateNode`, `emitValue`; and Rust `convert`, `Ctx::build`, `primitive_node`, `walk`, `write_payload`, `write_raw`.

The only explicit bound found is Go template expansion (`go/objectmodel/template.go:37-40`), which does not protect ordinary YAML, schema-child, opaque, validation, or serialization depth. Toolchain absence prevented measuring stack-overflow/OOM/output thresholds. These surfaces are **inconclusive**, not defended.

## Fix order

1. Enforce Rust `required` and arity/type checks before node construction; add cross-language negative vectors for missing-required, scalar-at-list, and collection-at-scalar.
2. Make Rust primitive validation reject every non-map primitive.
3. Quote mapping keys with scalar rules in both emitters; make Go parse/validate emitted bytes before constructing `validatedCanonical`.
4. Reject a second YAML document/event in schema, authoring, and canonical-object parsing.
5. Replace Rust per-frame `Vec<String>` duplicate tracking with a hash set and wide-map linear lookup structures.
6. Replace Go's forgeable behavior-bearing credential interface with an unexported concrete dispatch token, or snapshot once and make all module work consume only that immutable snapshot.
7. Define byte, nesting, node, time, memory, and output budgets.
