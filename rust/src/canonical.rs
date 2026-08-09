//! Canonical serialization (SPEC §8.8).
//!
//! The emitter is written here rather than delegated to the YAML library, for
//! one reason worth stating plainly: **§8.8 does not define a canonical byte
//! encoding** (`docs/spec-defects.md` SD-010, audit finding F-13). INV-030
//! promises determinism, but nothing says what indentation, quoting or
//! sequence style a canonical object has.
//!
//! So this emitter is deterministic — the same object always produces the same
//! bytes — without being canonical in any sense the specification licenses.
//! Whether it agrees byte-for-byte with the Go implementation is not something
//! either of us can be right or wrong about yet, which is why the conformance
//! runner compares parsed structure. Writing the emitter by hand rather than
//! taking a library's defaults at least means the choices are visible and in
//! one place, ready to be pinned when §8.8 says something.
//!
//! Member order IS specified and is enforced here: `values`, `origin`, then
//! the primitives in the §6.1 order.

use crate::node::{Node, Payload};
use crate::origin::Origin;
use crate::value::Value;

/// Serialize a node tree as the canonical document.
#[must_use]
pub fn to_yaml(root: &Node) -> Vec<u8> {
    let mut out = String::from("---\n");
    write_node_members(&mut out, root, 0);
    out.into_bytes()
}

fn indent(out: &mut String, depth: usize) {
    for _ in 0..depth {
        out.push_str("  ");
    }
}

/// Write a node's members — `values`, `origin`, primitives — at `depth`.
fn write_node_members(out: &mut String, n: &Node, depth: usize) {
    indent(out, depth);
    out.push_str("values:");
    write_payload(out, n.payload(), depth);

    indent(out, depth);
    out.push_str("origin: ");
    out.push_str(&origin_inline(n.origin()));
    out.push('\n');

    for name in n.primitive_names() {
        let p = n.primitive(name).expect("a listed primitive resolves");
        indent(out, depth);
        out.push_str(name);
        out.push_str(":\n");
        write_node_members(out, p, depth + 1);
    }
}

fn write_payload(out: &mut String, payload: &Payload, depth: usize) {
    match payload {
        Payload::Scalar(v) => {
            out.push(' ');
            out.push_str(&scalar(v));
            out.push('\n');
        }
        Payload::Raw(v) => write_raw(out, v, depth),
        Payload::Map(entries) => {
            if entries.is_empty() {
                out.push_str(" {}\n");
                return;
            }
            out.push('\n');
            for (name, child) in entries {
                indent(out, depth + 1);
                out.push_str(name);
                out.push_str(":\n");
                write_node_members(out, child, depth + 2);
            }
        }
        Payload::Seq(items) => {
            if items.is_empty() {
                out.push_str(" []\n");
                return;
            }
            out.push('\n');
            for item in items {
                indent(out, depth + 1);
                out.push_str("-\n");
                write_node_members(out, item, depth + 2);
            }
        }
    }
}

/// A payload kept verbatim: an opaque value, or a list inside a primitive
/// declaration, which declares no element position and so is not materialized
/// into nodes (INV-035).
fn write_raw(out: &mut String, v: &Value, depth: usize) {
    match v {
        Value::Map(m) if !m.is_empty() => {
            out.push('\n');
            for (k, val) in &m.0 {
                indent(out, depth + 1);
                out.push_str(k);
                out.push(':');
                write_raw(out, val, depth + 1);
            }
        }
        Value::Seq(items) if !items.is_empty() => {
            out.push('\n');
            for item in items {
                indent(out, depth + 1);
                out.push('-');
                write_raw(out, item, depth + 1);
            }
        }
        // An empty mapping and an empty sequence are written in their flow
        // forms; block form has no way to say "nothing here". The two arms are
        // separate on purpose even though the text differs by one character —
        // merging them would hide which collection is being emptied.
        Value::Map(_) => out.push_str(" {}\n"),
        Value::Seq(_) => out.push_str(" []\n"),
        scalar_value => {
            out.push(' ');
            out.push_str(&scalar(scalar_value));
            out.push('\n');
        }
    }
}

/// `origin` is always written inline, because the grammar is a short closed set
/// and a block form would make the four productions harder to see side by side.
fn origin_inline(o: &Origin) -> String {
    match o {
        Origin::Yaml => "[yaml]".into(),
        Origin::Schema => "[schema]".into(),
        Origin::Sealed(s) => format!(
            "[{{sealed: {{template: {}, path: {}}}}}]",
            scalar(&Value::Str(s.template.clone())),
            scalar(&Value::Str(s.path.clone()))
        ),
        Origin::SealedSchema(s) => format!(
            "[{{sealed: {{template: {}, path: {}}}}}, schema]",
            scalar(&Value::Str(s.template.clone())),
            scalar(&Value::Str(s.path.clone()))
        ),
    }
}

/// Render a scalar, quoting when a plain scalar would read back as something
/// else.
fn scalar(v: &Value) -> String {
    match v {
        Value::Null => "null".into(),
        Value::Bool(b) => b.to_string(),
        Value::Int(i) => i.to_string(),
        Value::Float(f) => {
            // A float that renders without a marker would read back as an
            // integer, changing the type of the value on a round trip.
            let s = f.to_string();
            if s.contains(['.', 'e', 'E']) || s.contains("inf") || s.contains("NaN") {
                s
            } else {
                format!("{s}.0")
            }
        }
        Value::Str(s) => quote_if_needed(s),
        Value::Seq(_) | Value::Map(_) => {
            // write_raw handles collections before anything can ask one for its
            // scalar rendering, so reaching here is a routing bug rather than a
            // value the emitter has to represent. Loud in debug, and a value
            // that parses back as "nothing" in release, because a serializer
            // that panics on a live object is worse than one that is wrong.
            debug_assert!(false, "a collection reached the scalar emitter");
            "null".into()
        }
    }
}

/// Quote a string when leaving it plain would change what it means.
///
/// The interesting case is the one the corpus already contains: `on` and `off`
/// are YAML 1.1 booleans, and a vector holding the list `[on, off]` in an
/// opaque payload round-tripped to `[true, false]` once already — during the
/// 0.2 expectation rewrite, by a script that took a parser's word for it.
fn quote_if_needed(s: &str) -> String {
    const RESERVED: [&str; 22] = [
        "true", "false", "yes", "no", "on", "off", "null", "~", "True", "False", "Yes", "No", "On",
        "Off", "Null", "TRUE", "FALSE", "YES", "NO", "ON", "OFF", "NULL",
    ];

    let needs_quotes = s.is_empty()
        || RESERVED.contains(&s)
        || s.parse::<i64>().is_ok()
        || s.parse::<f64>().is_ok()
        || s.starts_with([
            ' ', '-', '?', ':', ',', '[', ']', '{', '}', '#', '&', '*', '!', '|', '>', '\'', '"',
            '%', '@', '`',
        ])
        || s.ends_with(' ')
        || s.contains(": ")
        || s.contains(" #")
        || s.contains('\n');

    if needs_quotes {
        format!("'{}'", s.replace('\'', "''"))
    } else {
        s.to_string()
    }
}
