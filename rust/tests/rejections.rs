//! The rejection paths the corpus does not reach.
//!
//! The 29 vectors pin one rejection each, which is what a conformance corpus is
//! for: it says what the MODEL rejects, in a form a second implementation can
//! check. It does not say what happens to input that is merely broken — a
//! schema that is not a mapping, an origin term that is a number, a template
//! entry that names nothing. Those paths are this implementation's, and until
//! something calls them the only claim about them is that they compile.
//!
//! Every assertion below names the code and, where the specification names one,
//! the invariant — so a branch that silently starts returning a different
//! rejection is a failure here rather than a surprise in a caller.

use cic_object_model::error::{code, Stage};
use cic_object_model::value::{self, Value};
use cic_object_model::{materialize, schema, validate_canonical_document, MODEL_VERSION};
use std::fmt::Write as _;

fn schema_bytes(body: &str) -> Vec<u8> {
    format!("model: \"{MODEL_VERSION}\"\n{body}").into_bytes()
}

fn object(body: &str) -> Vec<u8> {
    body.as_bytes().to_vec()
}

// ---------------------------------------------------------------------------
// Final validation — the members a canonical node may and may not carry
// ---------------------------------------------------------------------------

#[test]
fn a_node_missing_values_or_origin_is_refused() {
    let err = validate_canonical_document(&object("origin: [yaml]\n")).expect_err("no values");
    assert_eq!(err.code, code::MISSING_VALUES);
    assert_eq!(err.invariant, "INV-001");

    let err = validate_canonical_document(&object("values: {}\n")).expect_err("no origin");
    assert_eq!(err.code, code::MISSING_ORIGIN);
    assert_eq!(err.invariant, "INV-002");
}

#[test]
fn a_document_that_is_not_a_mapping_is_refused() {
    let err = validate_canonical_document(&object("- a\n- b\n")).expect_err("a sequence");
    assert_eq!(err.code, code::MALFORMED_DOCUMENT);

    let err = validate_canonical_document(&object("[unclosed")).expect_err("unparseable");
    assert_eq!(err.code, code::MALFORMED_DOCUMENT);
    assert_eq!(err.stage, Stage::FinalValidation);
}

#[test]
fn documentation_is_not_a_member_of_a_node() {
    // INV-006. All four spellings, because a check that only knows one of them
    // is a check the next writer routes around without meaning to.
    for member in ["description", "documentation", "title", "comment"] {
        let doc = format!("values: 1\norigin: [yaml]\n{member}: prose\n");
        let err = validate_canonical_document(&object(&doc))
            .unwrap_err_or_panic(&format!("`{member}` was accepted on a node"));
        assert_eq!(err.code, code::DOCUMENTATION_ON_NODE, "for {member}");
        assert_eq!(err.invariant, "INV-006");
        assert!(err.path.ends_with(member));
    }
}

#[test]
fn a_canonical_node_carries_no_default() {
    // INV-012 — the default was a schema instruction; once materialized there
    // is a value, and keeping the instruction beside it invites the two to
    // disagree with nothing to say which wins.
    let err = validate_canonical_document(&object("values: 1\norigin: [yaml]\ndefault: 2\n"))
        .expect_err("a default member was accepted");
    assert_eq!(err.code, code::DEFAULT_MEMBER_ON_NODE);
    assert_eq!(err.invariant, "INV-012");
}

#[test]
fn a_member_that_is_neither_reserved_nor_a_primitive_is_refused() {
    // INV-021, and `cic` specifically: 0.2 moved the model version to the
    // hand-off frame, so a version inside the object lands exactly here.
    for member in ["surprise", "cic", "metadata"] {
        let doc = format!("values: 1\norigin: [yaml]\n{member}: x\n");
        let err = validate_canonical_document(&object(&doc))
            .unwrap_err_or_panic(&format!("`{member}` was accepted"));
        assert_eq!(err.code, code::UNKNOWN_MEMBER, "for {member}");
        assert_eq!(err.invariant, "INV-021");
    }
}

#[test]
fn the_walk_descends_into_payloads_primitives_and_list_entries() {
    // A defect below the root has to be found, and its path has to name where
    // it is. Three routes down: a payload child, a list entry, a primitive.
    let cases: [(&str, &str); 3] = [
        (
            "values:\n  child:\n    values: 1\n    origin: [yaml]\n    default: 2\norigin: [yaml]\n",
            "$.values.child.default",
        ),
        (
            "values:\n  - values: 1\n    origin: [yaml]\n    default: 2\norigin: [yaml]\n",
            "$.values[0].default",
        ),
        (
            "values: 1\norigin: [yaml]\nrole:\n  values: 1\n  origin: [yaml]\n  default: 2\n",
            "$.role.default",
        ),
    ];
    for (doc, want_path) in cases {
        let err = validate_canonical_document(&object(doc))
            .unwrap_err_or_panic(&format!("a defect at {want_path} was not found"));
        assert_eq!(err.path, want_path, "wrong path for {want_path}");
    }
}

// ---------------------------------------------------------------------------
// The origin grammar — every way a term or a sequence can be wrong
// ---------------------------------------------------------------------------

fn origin_error(origin: &str) -> cic_object_model::Error {
    let doc = format!("values: 1\norigin: {origin}\n");
    validate_canonical_document(&object(&doc))
        .unwrap_err_or_panic(&format!("origin {origin} was accepted"))
}

#[test]
fn origin_must_be_a_terminal_sequence() {
    // INV-004 — if origin had been expanded into a node it would need an origin
    // of its own, without end.
    let err = origin_error("{values: [yaml], origin: [schema]}");
    assert_eq!(err.code, code::ORIGIN_NOT_TERMINAL);
    assert_eq!(err.invariant, "INV-004");

    let err = origin_error("yaml");
    assert_eq!(err.code, code::ORIGIN_NOT_TERMINAL);
}

#[test]
fn origin_must_not_be_empty() {
    let err = origin_error("[]");
    assert_eq!(err.code, code::ORIGIN_EMPTY);
    assert_eq!(err.invariant, "INV-018");
}

#[test]
fn the_bare_token_sealed_is_not_an_origin_term() {
    // INV-020 — origin `sealed` is always a two-arity constructor. The bare
    // token belongs to the aggregate slot-mode vocabulary, which is a different
    // `sealed` entirely (SPEC §5.5).
    let err = origin_error("[sealed]");
    assert_eq!(err.code, code::ORIGIN_GRAMMAR);
    assert_eq!(err.invariant, "INV-020");
}

#[test]
fn an_unknown_or_ill_typed_origin_term_is_refused() {
    for origin in ["[invented]", "[1]", "[[yaml]]", "[{notsealed: {}}]"] {
        let err = origin_error(origin);
        assert_eq!(err.code, code::ORIGIN_GRAMMAR, "for {origin}");
    }
}

#[test]
fn a_sealed_term_carries_both_template_and_path() {
    // INV-015 — template identity alone says which template, not which node in
    // it, and that is not provenance. `null` counts as absent: a sealed origin
    // whose template is the string "null" points at nothing while looking like
    // an answer.
    for origin in [
        "[{sealed: {template: $t}}]",
        "[{sealed: {path: $.a}}]",
        "[{sealed: {template: null, path: null}}]",
        "[{sealed: notamapping}]",
    ] {
        let err = origin_error(origin);
        assert_eq!(
            err.code,
            code::SEALED_MISSING_TEMPLATE_OR_PATH,
            "for {origin}"
        );
        assert_eq!(err.invariant, "INV-015");
    }
}

#[test]
fn the_exclusion_rules_name_their_own_invariant() {
    // These are implied by the grammar match that follows them, and are kept
    // ahead of it so a reader gets the rule they broke rather than "not in the
    // grammar". The vectors assert these codes.
    let err = origin_error("[yaml, schema]");
    assert_eq!(err.code, code::ORIGIN_YAML_SCHEMA_CONFLICT);
    assert_eq!(err.invariant, "INV-017");

    let err = origin_error("[{sealed: {template: $t, path: $.a}}, yaml]");
    assert_eq!(err.code, code::ORIGIN_SEALED_YAML_CONFLICT);
    assert_eq!(err.invariant, "INV-016");
}

#[test]
fn only_the_four_productions_are_accepted() {
    // Presence questions cannot express arity or order. Each of these is built
    // from legal terms, breaks no exclusion rule, and is not a sequence the
    // grammar produces.
    for origin in [
        "[yaml, yaml]",
        "[schema, schema]",
        "[{sealed: {template: $t, path: $.a}}, {sealed: {template: $t, path: $.a}}]",
        "[schema, {sealed: {template: $t, path: $.a}}]",
        "[{sealed: {template: $t, path: $.a}}, schema, schema]",
    ] {
        let err = origin_error(origin);
        assert_eq!(err.code, code::ORIGIN_GRAMMAR, "for {origin}");
        assert_eq!(err.invariant, "INV-013", "for {origin}");
    }

    // And the four the grammar does produce are accepted, so the loop above is
    // not passing because everything is refused.
    for origin in [
        "[yaml]",
        "[schema]",
        "[{sealed: {template: $t, path: $.a}}]",
        "[{sealed: {template: $t, path: $.a}}, schema]",
    ] {
        let doc = format!("values: 1\norigin: {origin}\n");
        validate_canonical_document(&object(&doc))
            .unwrap_or_else(|e| panic!("a valid origin {origin} was refused: {e}"));
    }
}

// ---------------------------------------------------------------------------
// Schema load — the shapes a broken schema arrives in
// ---------------------------------------------------------------------------

#[test]
fn a_structurally_broken_schema_is_refused_at_schema_load() {
    let cases: [(&str, &str); 6] = [
        ("- a\n- b\n", "a schema that is not a mapping"),
        ("model: \"0.2\"\n", "a schema with no root"),
        ("model: \"0.2\"\nroot: 1\n", "a root that is not a mapping"),
        (
            "model: \"0.2\"\nroot:\n  shape: object\n  children: 1\n",
            "children that are not a mapping",
        ),
        (
            "model: \"0.2\"\ntemplates: 1\nroot:\n  shape: object\n",
            "templates that are not a mapping",
        ),
        (
            "model: \"0.2\"\ntemplates:\n  $t: 1\nroot:\n  shape: object\n",
            "a template that is not a mapping",
        ),
    ];
    for (body, what) in cases {
        let err = schema::load(body.as_bytes()).unwrap_err_or_panic(what);
        assert_eq!(err.stage, Stage::SchemaLoad, "for {what}");
    }
}

#[test]
fn a_sealed_from_naming_no_template_is_refused() {
    let err = materialize(
        &schema_bytes(
            "root:\n  shape: object\n  children:\n    a:\n      sealed_from:\n        template: $missing\n        path: $.a\n",
        ),
        b"{}\n",
    )
    .expect_err("a sealed_from naming no template was accepted");
    assert_eq!(err.code, code::TEMPLATE_NOT_FOUND);
    assert_eq!(err.invariant, "INV-015");
}

#[test]
fn a_list_item_schema_is_parsed_and_used() {
    let obj = materialize(
        &schema_bytes(
            "root:\n  shape: object\n  children:\n    xs:\n      shape: list\n      item:\n        shape: scalar\n        scalar_type: integer\n",
        ),
        b"xs: [1, 2, 3]\n",
    )
    .expect("a list materializes");
    let xs = obj.root().get("$.values.xs").expect("xs resolves");
    assert_eq!(xs.len(), 3);
    assert_eq!(
        xs.at(2).and_then(cic_object_model::Node::scalar),
        Some(&Value::Int(3))
    );
}

#[test]
fn a_list_with_no_authored_value_materializes_empty() {
    let obj = materialize(
        &schema_bytes(
            "root:\n  shape: object\n  children:\n    xs:\n      shape: list\n      item:\n        shape: scalar\n",
        ),
        b"{}\n",
    )
    .expect("an unauthored list materializes");
    assert_eq!(obj.root().get("$.values.xs").expect("xs").len(), 0);
}

// ---------------------------------------------------------------------------
// The YAML bridge
// ---------------------------------------------------------------------------

#[test]
fn input_that_is_not_utf8_is_refused_and_names_its_stage() {
    let err = value::parse(&[0xff, 0xfe], Stage::EntryValidation, "$", "input")
        .expect_err("invalid UTF-8 was accepted");
    assert_eq!(err.code, code::MALFORMED_DOCUMENT);
    assert_eq!(err.stage, Stage::EntryValidation);
}

#[test]
fn an_alias_is_refused_before_it_can_be_expanded() {
    let err = value::parse(b"a: &x 1\nb: *x\n", Stage::EntryValidation, "$", "input")
        .expect_err("an alias was accepted");
    assert_eq!(err.code, code::MALFORMED_DOCUMENT);
    assert!(
        err.detail.contains("alias"),
        "the message does not say why: {}",
        err.detail
    );
}

/// The reason the guard exists, measured rather than asserted from principle.
///
/// This document is 393 bytes. Composed, it is 12,345,678 nodes and takes
/// nearly three seconds — a ~31,000x amplification on input this library treats
/// as untrusted. The expansion happens during composition, so no budget checked
/// after parsing can help; the only thing that works is refusing the alias
/// before a tree exists.
///
/// The test asserts the refusal AND that it is fast, because a guard that
/// refuses only after doing the work is not a guard.
#[test]
fn an_alias_bomb_is_refused_without_expanding_it() {
    let mut doc = String::from("a0: &a0 [x, x, x, x, x, x, x, x, x, x]\n");
    for i in 1..7 {
        let prev = format!("*a{}", i - 1);
        let row = vec![prev; 10].join(", ");
        writeln!(doc, "a{i}: &a{i} [{row}]").expect("writing to a String cannot fail");
    }
    assert!(
        doc.len() < 500,
        "the bomb should be small: {} bytes",
        doc.len()
    );

    let start = std::time::Instant::now();
    let err = value::parse(doc.as_bytes(), Stage::EntryValidation, "$", "input")
        .expect_err("an alias bomb was accepted");
    let elapsed = start.elapsed();

    assert_eq!(err.code, code::MALFORMED_DOCUMENT);
    // Composing this document took 2.7s when measured. A refusal that scans the
    // input once is three orders of magnitude below that; anything near the
    // composition time means the guard is running after the damage.
    assert!(
        elapsed < std::time::Duration::from_millis(200),
        "refusing the bomb took {elapsed:?}; the guard is not running before composition"
    );
}

/// Duplicate keys are refused, and the two implementations now agree because
/// they were made to, not because their parsers happened to behave alike.
///
/// This was left accepted for one commit, on the reasoning that Go behaved the
/// same way. That reasoning was never measured and was wrong: Go rejects the
/// document — `mapping key "a" already defined at line 1`. The two
/// implementations disagreed on real input, in a model whose point is unique
/// addressing, and the corpus could not see it because no vector holds a
/// duplicate.
#[test]
fn a_duplicate_key_is_refused() {
    for doc in [
        "a: 1\na: 2\n",              // at the root
        "outer:\n  a: 1\n  a: 2\n",  // nested
        "xs:\n  - a: 1\n    a: 2\n", // inside a sequence entry
        "a: 1\nb: 2\na: 3\n",        // not adjacent
    ] {
        let err = value::parse(doc.as_bytes(), Stage::EntryValidation, "$", "input")
            .unwrap_err_or_panic(&format!("a duplicate key was accepted in {doc:?}"));
        assert_eq!(err.code, code::MALFORMED_DOCUMENT, "for {doc:?}");
        assert!(
            err.detail.contains("twice"),
            "the message does not say why: {}",
            err.detail
        );
    }
}

/// The same key at different levels, and in different mappings at the same
/// level, is not a duplicate — those are different addresses. A check that
/// cannot tell them apart would reject most real documents.
#[test]
fn the_same_name_at_different_addresses_is_not_a_duplicate() {
    for doc in [
        "a:\n  a: 1\n",                   // a child sharing its parent's name
        "one:\n  x: 1\nother:\n  x: 2\n", // sibling mappings, same key
        "xs:\n  - x: 1\n  - x: 2\n",      // sequence entries, same key
        "a: 1\nb:\n  a: 2\n",             // shadowed one level down
    ] {
        value::parse(doc.as_bytes(), Stage::EntryValidation, "$", "input")
            .unwrap_or_else(|e| panic!("{doc:?} was wrongly refused: {e}"));
    }
}

#[test]
fn an_anchor_without_an_alias_is_still_refused() {
    // An anchor alone expands nothing, but it is not part of either format and
    // accepting it would leave the guard depending on how the document uses it.
    // Measured: the parser reports the alias, so a lone anchor parses — and
    // that is recorded here rather than claimed either way.
    let parsed = value::parse(b"a: &x 1\n", Stage::EntryValidation, "$", "input");
    assert!(
        parsed.is_ok(),
        "a lone anchor is accepted today; if this starts failing, the guard widened"
    );
}

/// `Result::expect_err` needs `T: Debug`, which `CanonicalObject` has but
/// `Schema` does not need to grow just for tests. This says the same thing with
/// the message the caller wants.
trait UnwrapErrOrPanic<T, E> {
    fn unwrap_err_or_panic(self, msg: &str) -> E;
}

impl<T, E> UnwrapErrOrPanic<T, E> for Result<T, E> {
    fn unwrap_err_or_panic(self, msg: &str) -> E {
        match self {
            Ok(_) => panic!("{msg}"),
            Err(e) => e,
        }
    }
}
