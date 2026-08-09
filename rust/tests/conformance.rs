//! The conformance corpus, run against this implementation.
//!
//! The vectors are implementation-independent — YAML in, YAML out — and this
//! runner adds no Rust-specific fixture: it reads the same files the Go runner
//! reads. Where the two implementations disagree, the disagreement is the
//! specification's, not the language's.
//!
//! Expectations are compared BYTE for byte, because §8.8.1 (INV-043) now says
//! what the bytes are.
//!
//! This used to compare parsed structure, on the reasoning that §8.8 defined no
//! serialization so requiring identical bytes would be inventing a rule. The
//! cost of that reasoning was measured: across the thirteen materialization
//! vectors the Go and Rust implementations produced ZERO byte-identical objects
//! while agreeing on every one semantically. A structural comparison cannot see
//! member order at all, so one implementation reordering opaque payloads — data
//! §4 promises to carry untouched — passed without comment.

use cic_object_model::error::Stage;
use cic_object_model::value::{self, Value};
use cic_object_model::{materialize, validate_canonical_document};
use std::path::{Path, PathBuf};

const CORPUS: &str = "../conformance";

/// What "the corpus ran" means. A runner that discovers zero vectors and exits
/// 0 has not run the corpus, so the counts are asserted rather than reported.
// 9 invalid since SPEC §2.6: 008 and 009 are INV-041 and INV-042, the two
// things the serialization can do to an address before the model sees the
// document. Both exist because the two implementations DISAGREED on real input
// and the corpus could not see it.
const EXPECTED: [(&str, usize); 3] = [("materialization", 14), ("invalid", 12), ("validation", 9)];

fn vectors(group: &str) -> Vec<PathBuf> {
    let dir = Path::new(CORPUS).join(group);
    let mut out: Vec<PathBuf> = std::fs::read_dir(&dir)
        .unwrap_or_else(|e| panic!("cannot read {}: {e}", dir.display()))
        .filter_map(|e| {
            let e = e.ok()?;
            e.file_type().ok()?.is_dir().then(|| e.path())
        })
        .collect();
    out.sort();
    let want = EXPECTED
        .iter()
        .find(|(g, _)| *g == group)
        .map(|(_, n)| *n)
        .expect("a known group");
    assert_eq!(
        out.len(),
        want,
        "{group}: discovered {} vectors, expected {want}",
        out.len()
    );
    out
}

fn read(dir: &Path, name: &str) -> Vec<u8> {
    std::fs::read(dir.join(name))
        .unwrap_or_else(|e| panic!("cannot read {}: {e}", dir.join(name).display()))
}

fn parse(data: &[u8], what: &str) -> Value {
    value::parse(data, Stage::FinalValidation, "$", what)
        .unwrap_or_else(|e| panic!("{what} is not parseable: {e}"))
}

#[test]
fn corpus_size() {
    let total: usize = EXPECTED.iter().map(|(g, _)| vectors(g).len()).sum();
    assert_eq!(total, EXPECTED.iter().map(|(_, n)| n).sum::<usize>());

    // A group present on disk and absent from EXPECTED would never run at all,
    // and the sum above cannot see it — the same failure shape as a manifest
    // that verifies only the files it lists.
    for entry in std::fs::read_dir(CORPUS).expect("the corpus root is readable") {
        let entry = entry.expect("a readable entry");
        if entry.file_type().expect("a file type").is_dir() {
            let name = entry.file_name().to_string_lossy().to_string();
            assert!(
                EXPECTED.iter().any(|(g, _)| *g == name),
                "conformance/{name}/ exists but no runner claims it"
            );
        }
    }
}

/// materialization/* — authoring input to canonical object.
#[test]
fn materialization() {
    for dir in vectors("materialization") {
        let name = dir
            .file_name()
            .expect("a name")
            .to_string_lossy()
            .to_string();
        let obj = materialize(&read(&dir, "schema.yaml"), &read(&dir, "input.yaml"))
            .unwrap_or_else(|e| panic!("{name}: materialization failed: {e}"));

        let want = read(&dir, "expected.yaml");
        assert_eq!(
            obj.canonical_yaml(),
            want.as_slice(),
            "{name}: the canonical object does not match expected.yaml byte for byte\n--- got ---\n{}\n--- want ---\n{}",
            String::from_utf8_lossy(obj.canonical_yaml()),
            String::from_utf8_lossy(&want)
        );

        // INV-030 — the same schema and input produce a byte-identical object.
        let again = materialize(&read(&dir, "schema.yaml"), &read(&dir, "input.yaml"))
            .unwrap_or_else(|e| panic!("{name}: second materialization failed: {e}"));
        assert_eq!(
            obj.canonical_yaml(),
            again.canonical_yaml(),
            "{name}: materialization is not deterministic"
        );

        // INV-033 — the version travels with the hand-off, never inside the
        // object.
        assert!(
            !String::from_utf8_lossy(obj.canonical_yaml()).contains("cic:"),
            "{name}: the model version leaked into the object"
        );
    }
}

/// invalid/* — authoring input that MUST be rejected.
#[test]
fn invalid() {
    for dir in vectors("invalid") {
        let name = dir
            .file_name()
            .expect("a name")
            .to_string_lossy()
            .to_string();
        let err = materialize(&read(&dir, "schema.yaml"), &read(&dir, "input.yaml"))
            .expect_err(&format!("{name}: input that must be rejected was accepted"));
        check_error(&name, &dir, &err);
    }
}

/// validation/* — an already-canonical object, accepted or rejected without its
/// schema.
#[test]
fn validation() {
    for dir in vectors("validation") {
        let name = dir
            .file_name()
            .expect("a name")
            .to_string_lossy()
            .to_string();
        let err = validate_canonical_document(&read(&dir, "object.yaml"))
            .expect_err(&format!("{name}: an object that must be rejected passed"));
        check_error(&name, &dir, &err);
    }
}

/// The four fields the vectors assert on. `detail` is prose for a human and is
/// deliberately not compared — pinning it would make every reworded message a
/// conformance failure.
fn check_error(name: &str, dir: &Path, err: &cic_object_model::Error) {
    let want = parse(&read(dir, "expected-error.yaml"), "expected-error.yaml");
    let e = want
        .as_map()
        .and_then(|m| m.get("error"))
        .and_then(Value::as_map)
        .unwrap_or_else(|| panic!("{name}: expected-error.yaml has no error mapping"));

    let field = |k: &str| {
        e.get(k).map_or_else(
            || panic!("{name}: expected-error.yaml has no `{k}`"),
            Value::to_plain_string,
        )
    };

    assert_eq!(err.code, field("code"), "{name}: wrong code ({err})");
    assert_eq!(
        err.invariant,
        field("invariant"),
        "{name}: wrong invariant ({err})"
    );
    assert_eq!(
        err.stage.as_str(),
        field("stage"),
        "{name}: wrong stage ({err})"
    );
    assert_eq!(err.path, field("path"), "{name}: wrong path ({err})");
}
