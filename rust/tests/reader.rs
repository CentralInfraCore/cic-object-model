//! The reader API — the surface a caller navigates an object with.
//!
//! The corpus drives every one of these paths end to end and CALLS none of
//! them: it compares serialized output. That distinction has already cost the
//! Go implementation twice — a broken `Get` passed 26/26 vectors, and `Scalar()`
//! returned nothing for every leaf inside a primitive payload while those
//! leaves sat correctly in the YAML. Both were invisible to a suite that only
//! checks bytes.
//!
//! So the standard here is not a coverage number. It is: every reader is
//! called, on the values it is meant for AND on the values it is not, because
//! what an accessor does when asked the wrong question is part of its contract.

use cic_object_model::node::Payload;
use cic_object_model::value::Value;
use cic_object_model::{materialize, CanonicalObject, Node, Origin};

const CORPUS: &str = "../conformance";

fn object(vector: &str) -> CanonicalObject {
    let dir = std::path::Path::new(CORPUS).join(vector);
    let read = |n: &str| std::fs::read(dir.join(n)).expect("the vector file is readable");
    materialize(&read("schema.yaml"), &read("input.yaml"))
        .unwrap_or_else(|e| panic!("{vector}: {e}"))
}

/// Visit every node reachable through the reader API, so the assertions run
/// over the whole object rather than a hand-picked path.
fn walk(n: &Node, visit: &mut dyn FnMut(&Node)) {
    visit(n);
    for name in n.children() {
        walk(n.child(name).expect("a listed child resolves"), visit);
    }
    for i in 0..n.len() {
        walk(n.at(i).expect("an index below len resolves"), visit);
    }
    for name in n.primitive_names() {
        walk(
            n.primitive(name).expect("a listed primitive resolves"),
            visit,
        );
    }
}

/// INV-040 — every node has exactly one address.
///
/// The invariant's real content is a round trip: every address the object
/// prints resolves back to the node that printed it, and to nothing else. A
/// resolver that is too strict fails loudly; one that is too loose still
/// resolves, just possibly to something else, and that is the direction this
/// kind of bug goes.
#[test]
fn every_address_resolves_to_the_node_that_printed_it() {
    for vector in [
        "materialization/013_access_inherit_injection",
        "materialization/008_normalize_list",
        "materialization/009_normalize_map",
        "materialization/006_closure_opaque",
        "materialization/004_origin_sealed_schema",
    ] {
        let obj = object(vector);
        let root = obj.root();
        let mut count = 0;
        let mut failures: Vec<String> = Vec::new();
        walk(root, &mut |node| {
            count += 1;
            match root.get(node.path()) {
                None => failures.push(format!("{} does not resolve", node.path())),
                Some(found) if !std::ptr::eq(found, node) => {
                    failures.push(format!("{} resolved to {}", node.path(), found.path()));
                }
                Some(_) => {}
            }
        });
        assert!(failures.is_empty(), "{vector}: {failures:?}");
        assert!(
            count > 1,
            "{vector}: walked {count} nodes; traversal is broken"
        );
    }
}

#[test]
fn absolute_addresses_are_answered_only_by_the_root() {
    let obj = object("materialization/001_origin_yaml");
    let root = obj.root();
    let mtu = root.get("$.values.mtu").expect("$.values.mtu resolves");

    // `$` names the root. A Node holds no parent link, so from anywhere else
    // the honest result is no result — not a silent reinterpretation of the
    // address as relative, which is what the Go implementation did until the
    // audit (F-12).
    assert!(
        mtu.get("$.shape").is_none(),
        "an absolute address resolved from a non-root node"
    );
    assert!(mtu.get("$").is_none());

    // The relative route from the same node works, and reaches the same node
    // the absolute route reaches from the root.
    let via_node = mtu.get("shape").expect("the relative address resolves");
    let from_root = root
        .get("$.values.mtu.shape")
        .expect("the absolute address resolves");
    assert!(std::ptr::eq(via_node, from_root));
}

#[test]
fn malformed_addresses_are_rejected_rather_than_repaired() {
    let obj = object("materialization/001_origin_yaml");
    let root = obj.root();

    for alias in [
        ".values.mtu",         // leading separator with no root
        "$values.mtu",         // `$` not followed by a separator
        "$.",                  // root, then an empty segment
        ".",                   // an empty segment
        "$..values",           // a doubled separator
        "values.",             // a trailing separator
        "$.values",            // a trailing `values` names the payload, not a node
        "$.values.mtu.values", // same, one level down
        "$.values.nonexistent",
        "$.nonexistent",
    ] {
        assert!(root.get(alias).is_none(), "{alias:?} resolved");
    }

    // "" is the receiver and "$" is the root: two operations that coincide here
    // because the receiver IS the root.
    assert!(std::ptr::eq(
        root.get("").expect("\"\" names the receiver"),
        root
    ));
    assert!(std::ptr::eq(
        root.get("$").expect("\"$\" names the root"),
        root
    ));
}

#[test]
fn list_entries_are_addressable_by_position() {
    let obj = object("materialization/008_normalize_list");
    let root = obj.root();
    let list = root.get("$.values.addresses").expect("the list resolves");

    let n = list.len();
    assert!(n > 0, "the list reports no entries");
    assert!(!list.is_empty());
    assert!(
        list.at(n).is_none(),
        "at({n}) resolved on an {n}-entry list"
    );
    for i in 0..n {
        let by_index = list.at(i).expect("an entry below len resolves");
        let by_address = root
            .get(&format!("$.values.addresses.values[{i}]"))
            .expect("the positional address resolves");
        assert!(std::ptr::eq(by_index, by_address));
    }
    // A position beyond the end, reached by arithmetic rather than intent.
    assert!(root.get("$.values.addresses.values[99]").is_none());
    assert!(root.get("$.values.addresses.values[x]").is_none());
}

#[test]
fn accessors_asked_the_wrong_question_answer_nothing() {
    let obj = object("materialization/001_origin_yaml");
    let root = obj.root();
    let mtu = root.get("$.values.mtu").expect("mtu resolves");

    // The root's payload is a map: not a scalar, not a list.
    assert!(
        root.scalar().is_none(),
        "scalar() succeeded on a map payload"
    );
    assert_eq!(root.len(), 0, "len() is non-zero on a map payload");
    assert!(root.at(0).is_none(), "at(0) succeeded on a map payload");

    // mtu's payload is a scalar: no children, no entries.
    assert!(mtu.children().is_empty());
    assert!(mtu.child("anything").is_none());
    assert!(mtu.primitive("nonexistent").is_none());
    assert!(
        root.child("mtu").is_some(),
        "child() missed on the root's payload"
    );
}

#[test]
fn origin_and_payload_are_readable_and_say_what_the_object_says() {
    let obj = object("materialization/004_origin_sealed_schema");
    let root = obj.root();

    // Row 1 — the root is always yaml-authored.
    assert_eq!(root.origin(), &Origin::Yaml);

    // Row 4 — the template defined and closed the node; the value came from the
    // template node's own schema default.
    let enabled = root
        .get("$.values.access_policy.values.enabled")
        .expect("the sealed child resolves");
    let Origin::SealedSchema(sealed) = enabled.origin() else {
        panic!("expected [sealed(t,p), schema], got {:?}", enabled.origin());
    };
    assert_eq!(sealed.template, "$network-object");
    assert_eq!(sealed.path, "$.access.read");
    assert!(enabled.origin().is_sealed());
    assert!(enabled.origin().sealed().is_some());
    assert_eq!(enabled.scalar(), Some(&Value::Bool(true)));

    // Row 3 — the template defined, closed AND valued it.
    let policy = root
        .get("$.values.access_policy")
        .expect("the sealed node resolves");
    assert!(matches!(policy.origin(), Origin::Sealed(_)));
    assert!(matches!(policy.payload(), Payload::Map(_)));

    // Row 2 — a value that materialized from a schema default, with no sealed
    // boundary above it.
    let obj = object("materialization/002_origin_schema");
    let mtu = obj.root().get("$.values.mtu").expect("mtu resolves");
    assert_eq!(mtu.origin(), &Origin::Schema);
    assert!(!mtu.origin().is_sealed());
    assert!(mtu.origin().sealed().is_none());
}

#[test]
fn a_primitive_payload_is_a_node_tree_not_a_mapping() {
    // INV-035. Model 0.1 left primitive payloads as raw mappings, which made
    // `network.values.mtu.access.read` a path into a YAML blob rather than the
    // address of a node — false of every object it produced.
    let obj = object("materialization/013_access_inherit_injection");
    let root = obj.root();

    let effect = root
        .get("$.values.mtu.access.values.read.values.rules.values.operator.values.effect")
        .expect("a leaf deep inside a primitive payload is addressable");
    assert_eq!(effect.scalar(), Some(&Value::Str("allow".into())));
    assert_eq!(effect.origin(), &Origin::Schema);

    // `inherit` was not declared on `read`; it is injected so a reader does not
    // have to know the default to get the right answer.
    let inherit = root
        .get("$.values.mtu.access.values.read.values.inherit")
        .expect("the injected inherit is addressable");
    assert_eq!(inherit.scalar(), Some(&Value::Bool(true)));

    // A sequence inside a primitive stays raw: it declares no element position,
    // so its entries have no addresses to be nodes at.
    let subjects = root
        .get("$.values.mtu.access.values.read.values.rules.values.operator.values.subjects")
        .expect("subjects is addressable");
    assert!(matches!(subjects.payload(), Payload::Raw(Value::Seq(_))));
    assert_eq!(subjects.len(), 0, "a raw sequence exposes no node entries");

    // Every node carries a shape, and the primitives come back in §6.1 order.
    let mtu = root.get("$.values.mtu").expect("mtu resolves");
    assert_eq!(mtu.primitive_names(), vec!["shape", "role", "access"]);
    assert!(mtu.primitive("shape").is_some());
}

#[test]
fn an_opaque_payload_is_kept_verbatim_and_interprets_nothing_below_it() {
    let obj = object("materialization/006_closure_opaque");
    let payload = obj
        .root()
        .get("$.values.payload")
        .expect("payload resolves");

    // §4: below `values` no primitive interpretation applies. `access` inside
    // an opaque payload is a key, not the primitive.
    let Payload::Raw(Value::Map(m)) = payload.payload() else {
        panic!("an opaque payload was materialized into nodes");
    };
    assert!(m.contains("access"), "the opaque payload lost a key");
    assert!(
        payload.child("access").is_none(),
        "an opaque key became a node"
    );
    assert_eq!(payload.primitive_names(), vec!["shape"]);
}

#[test]
fn the_model_version_travels_with_the_hand_off_not_inside_the_object() {
    // INV-033, both halves.
    let obj = object("materialization/001_origin_yaml");
    assert_eq!(obj.model_version(), cic_object_model::MODEL_VERSION);
    let yaml = String::from_utf8(obj.canonical_yaml().to_vec()).expect("valid UTF-8");
    assert!(!yaml.contains("cic:"), "the version leaked into the object");
    assert!(!yaml.is_empty());
}
