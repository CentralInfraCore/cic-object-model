//! Tests for the layers that are implemented: the YAML bridge, the schema
//! language, and the version declaration.
//!
//! The materialization pipeline is not here because it is not written yet.
//! That is deliberate rather than an omission to be tidied later: a test file
//! that asserts nothing about a stage which does not exist would report
//! coverage for code that is not there, and this repository has spent a day
//! finding out what a green gate over an unasserted claim is worth.

use cic_object_model::error::{code, Stage};
use cic_object_model::{schema, value, MODEL_VERSION};

const V: &str = MODEL_VERSION;

fn schema_bytes(body: &str) -> Vec<u8> {
    format!("model: \"{V}\"\n{body}").into_bytes()
}

// ---------------------------------------------------------------------------
// The version declaration
// ---------------------------------------------------------------------------

/// The version this crate implements is the version SPEC.md declares.
///
/// The Go implementation has the same test for the same reason: 0.2 was once
/// written into SPEC.md and propagated nowhere, and every gate passed because
/// each checked its own file against itself. A second implementation is a
/// second place for that to happen.
#[test]
fn model_version_matches_the_specification() {
    let spec = std::fs::read_to_string("../SPEC.md").expect("SPEC.md is readable");
    let declared = spec
        .lines()
        .find_map(|l| {
            l.strip_prefix("**Model version: ")
                .and_then(|r| r.strip_suffix("**"))
        })
        .expect("SPEC.md declares a model version");
    assert_eq!(
        declared.trim(),
        MODEL_VERSION,
        "SPEC.md says {declared}, this crate says {MODEL_VERSION}"
    );
}

#[test]
fn a_schema_at_another_version_is_refused() {
    for header in [
        "model: \"0.1\"\n",
        "model: \"9.9\"\n",
        "model: banana\n",
        "",
    ] {
        let body = format!("{header}root:\n  shape: object\n");
        let err = schema::load(body.as_bytes()).expect_err("accepted an unusable model version");
        assert_eq!(err.code, code::UNSUPPORTED_MODEL_VERSION, "for {header:?}");
        assert_eq!(err.invariant, "INV-033");
        assert_eq!(err.stage, Stage::SchemaLoad);
    }
    // And the supported one loads, so the loop above is not passing because
    // everything is refused.
    schema::load(&schema_bytes("root:\n  shape: object\n")).expect("the supported version loads");
}

// ---------------------------------------------------------------------------
// The schema language
// ---------------------------------------------------------------------------

#[test]
fn reserved_child_names_are_rejected_with_inv_010() {
    for reserved in ["values", "origin"] {
        let body =
            format!("root:\n  shape: object\n  children:\n    {reserved}:\n      shape: scalar\n");
        let err = schema::load(&schema_bytes(&body)).expect_err("accepted a reserved child name");
        assert_eq!(err.code, code::SCHEMA_RESERVED_CHILD_VALUES);
        assert_eq!(err.invariant, "INV-010");
        assert!(err.path.ends_with(reserved), "path was {}", err.path);
    }
}

#[test]
fn sealed_from_needs_both_template_and_path() {
    // INV-015 — template identity alone says which template, not which node in
    // it, and that is not provenance.
    for sealed in [
        "        template: $t\n",
        "        path: $.a\n",
        "        template: null\n        path: null\n",
    ] {
        let body =
            format!("root:\n  shape: object\n  children:\n    a:\n      sealed_from:\n{sealed}");
        let err = schema::load(&schema_bytes(&body)).expect_err("accepted a partial sealed_from");
        assert_eq!(err.code, code::SEALED_MISSING_TEMPLATE_OR_PATH);
        assert_eq!(err.invariant, "INV-015");
    }

    // Both present loads.
    let ok = schema_bytes(
        "templates:\n  $t:\n    $.a:\n      shape: object\nroot:\n  shape: object\n  children:\n    a:\n      sealed_from:\n        template: $t\n        path: $.a\n",
    );
    let s = schema::load(&ok).expect("a complete sealed_from loads");
    assert!(s.template_entry("$t", "$.a").is_some());
}

#[test]
fn an_unknown_schema_key_is_refused() {
    let err = schema::load(&schema_bytes("root:\n  shape: object\n  surprise: 1\n"))
        .expect_err("accepted an unknown schema key");
    assert_eq!(err.code, code::UNKNOWN_SCHEMA_KEY);
}

#[test]
fn a_node_must_declare_a_shape() {
    let err = schema::load(&schema_bytes(
        "root:\n  shape: object\n  children:\n    a:\n      scalar_type: integer\n",
    ))
    .expect_err("accepted a node with no shape");
    assert_eq!(err.code, code::SCHEMA_SHAPE_MISSING);
}

#[test]
fn primitives_are_kept_as_declarations() {
    let s = schema::load(&schema_bytes(
        "root:\n  shape: object\n  children:\n    mtu:\n      shape: scalar\n      scalar_type: integer\n      role: config\n",
    ))
    .expect("a schema with a primitive loads");
    let mtu = s.root.child("mtu").expect("mtu is declared");
    assert_eq!(mtu.shape.as_deref(), Some("scalar"));
    assert_eq!(mtu.scalar_type.as_deref(), Some("integer"));
    assert_eq!(mtu.prims.len(), 1, "role should be kept as a primitive");
    assert_eq!(mtu.prims[0].0, "role");
}

#[test]
fn every_primitive_name_is_accepted_as_a_declaration() {
    // All eight of SPEC §6.1, so a typo in the table is caught here rather than
    // by a vector that happens to use one of them.
    for p in schema::PRIMITIVES {
        let body = format!(
            "root:\n  shape: object\n  children:\n    a:\n      shape: scalar\n      {p}: x\n"
        );
        // `shape` is both keyword and primitive; declaring it twice is not the
        // case under test.
        if p == "shape" {
            continue;
        }
        let s = schema::load(&schema_bytes(&body))
            .unwrap_or_else(|e| panic!("`{p}` was refused as a primitive: {e}"));
        assert_eq!(s.root.child("a").expect("a").prims[0].0, p);
    }
}

// ---------------------------------------------------------------------------
// The YAML bridge
// ---------------------------------------------------------------------------

#[test]
fn mapping_order_survives_parsing() {
    // Canonical output is member-ordered, so this is load-bearing rather than
    // incidental.
    let v = value::parse(b"c: 1\na: 2\nb: 3\n", Stage::SchemaLoad, "$", "input").expect("parses");
    assert_eq!(v.as_map().expect("a mapping").keys(), vec!["c", "a", "b"]);
}

#[test]
fn a_non_string_key_is_refused_rather_than_stringified() {
    // YAML allows `1:` and `true:`. Coercing them to names would let two
    // distinct keys collapse into one, in a structure whose whole purpose is
    // unique addressing.
    for doc in [b"1: a\n".as_slice(), b"true: a\n".as_slice()] {
        let err = value::parse(doc, Stage::SchemaLoad, "$", "input")
            .expect_err("accepted a non-string key");
        assert_eq!(err.code, code::MALFORMED_DOCUMENT);
    }
}

/// Duplicate keys are NOT refused, and this records that rather than hiding it.
///
/// The parser collapses `a: 1` / `a: 2` to `{a: 2}` before this crate sees
/// either, so there is no point in the pipeline where the duplicate exists to
/// be rejected. An authoring document that writes one address twice is accepted
/// and the second value wins, in a model whose point is unique addressing.
/// SPEC.md says nothing about it; see the note in src/value.rs.
#[test]
fn duplicate_keys_are_silently_collapsed_last_wins() {
    let v = value::parse(
        b"a: 1
a: 2
",
        Stage::SchemaLoad,
        "$",
        "input",
    )
    .expect("parses");
    let m = v.as_map().expect("a mapping");
    assert_eq!(
        m.len(),
        1,
        "the parser kept both keys; the note in value.rs is stale"
    );
    assert!(
        matches!(m.get("a"), Some(value::Value::Int(2))),
        "last did not win"
    );
}

#[test]
fn an_empty_document_is_the_empty_mapping() {
    for doc in [b"".as_slice(), b"---\n".as_slice(), b"{}\n".as_slice()] {
        let v = value::parse(doc, Stage::SchemaLoad, "$", "input").expect("parses");
        assert!(
            v.as_map()
                .is_some_and(cic_object_model::value::Map::is_empty),
            "{doc:?} did not parse as the empty mapping"
        );
    }
}

#[test]
fn malformed_yaml_names_the_stage_it_was_read_at() {
    // `\tnope` is NOT the example to use: this parser accepts a leading tab
    // and reads it as the scalar "nope". An unclosed flow sequence is a
    // genuine parse failure.
    let err = value::parse(b"[unclosed", Stage::EntryValidation, "$.x", "input")
        .expect_err("accepted malformed YAML");
    assert_eq!(err.stage, Stage::EntryValidation);
    assert_eq!(err.path, "$.x");
}

#[test]
fn scalars_keep_their_type() {
    let v = value::parse(
        b"i: 1\nf: 1.5\ns: text\nb: true\nn: null\nl: [1, 2]\n",
        Stage::SchemaLoad,
        "$",
        "input",
    )
    .expect("parses");
    let m = v.as_map().expect("a mapping");
    assert!(matches!(m.get("i"), Some(value::Value::Int(1))));
    assert!(matches!(m.get("f"), Some(value::Value::Float(_))));
    assert!(matches!(m.get("s"), Some(value::Value::Str(_))));
    assert!(matches!(m.get("b"), Some(value::Value::Bool(true))));
    assert!(matches!(m.get("n"), Some(value::Value::Null)));
    assert_eq!(
        m.get("l").and_then(value::Value::as_seq).map(<[_]>::len),
        Some(2)
    );
}

#[test]
fn error_display_names_code_path_and_stage() {
    let err = schema::load(b"model: \"9.9\"\nroot:\n  shape: object\n").expect_err("a rejection");
    let msg = err.to_string();
    for part in [
        err.code,
        err.invariant,
        err.path.as_str(),
        err.stage.as_str(),
    ] {
        assert!(msg.contains(part), "{msg:?} is missing {part:?}");
    }
}
