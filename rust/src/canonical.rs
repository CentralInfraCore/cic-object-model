//! Canonical serialization (SPEC §8.8).
//!
//! §8.8.1 defines this byte for byte and §8.8.2 defines the member order, so
//! this file implements a specification rather than making choices. It is
//! written by hand rather than delegated to a YAML library for that reason: a
//! library emits its own house style, and the house style is what made the two
//! implementations of this document produce zero byte-identical objects across
//! thirteen vectors while agreeing on every one of them semantically.
//!
//! The rules, in the order §8.8.1 states them: UTF-8, no BOM, `\n` line
//! endings including the last; a leading `---`; block style at two spaces per
//! level; `origin` inline; sequence entries opening `- ` with their first
//! member on the same line; `{}` and `[]` for empty collections; single quotes
//! wherever plain would be ambiguous under YAML 1.1 OR 1.2; floats that keep a
//! marker.

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

/// Write a mapping key under the same rule as a scalar value.
///
/// Keys used to be written verbatim while the quoting rule applied only to
/// values, and the two are the same problem: a key a reader would take for
/// something other than text changes what the document means. A child named
/// `a: b` produces `a: b:`, which is not the same mapping and is not YAML.
///
/// Go emitted exactly that and returned it, because it does not reparse what it
/// writes. This crate revalidates its own output, so the same case surfaced as
/// a rejection rather than as a malformed object handed to a caller — a better
/// failure, and still a failure.
fn emit_key(k: &str) -> String {
    quote_if_needed(k)
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
        // A primitive name is one of the eight identifiers of §6.1 and never
        // needs quoting; it goes through the same function anyway, so there is
        // one rule for keys rather than two places to keep in step.
        out.push_str(&emit_key(name));
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
                out.push_str(&emit_key(name));
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
                // §8.8.1: the entry opens `- ` with its FIRST member on the
                // same line, and the rest align under it. write_node_members
                // indents every line it writes, so the dash replaces the two
                // spaces the first line would have started with.
                out.push_str("- ");
                let start = out.len();
                write_node_members(out, item, depth + 2);
                let first_line_indent = "  ".repeat(depth + 2);
                if out[start..].starts_with(&first_line_indent) {
                    out.replace_range(start..start + first_line_indent.len(), "");
                }
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
                out.push_str(&emit_key(k));
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
        || looks_numeric(s)
        || s.starts_with([
            ' ', '-', '?', ':', ',', '[', ']', '{', '}', '#', '&', '*', '!', '|', '>', '\'', '"',
            '%', '@', '`',
        ])
        || s.ends_with(' ')
        || s.contains(": ")
        || s.contains(" #")
        || s.contains('\n')
        || s.contains('\r');

    if needs_quotes {
        format!("'{}'", s.replace('\'', "''"))
    } else {
        s.to_string()
    }
}

/// Whether a reader could take this text for a number under EITHER YAML
/// version, which is a wider set than Rust's own parsers accept.
///
/// `0x10` is the case that made this its own function: it is the string "0x10"
/// to `str::parse::<i64>` and the integer 16 to a YAML 1.1 reader, so a check
/// built from `parse` alone let it through. Octal, underscore digit groups and
/// sexagesimal are the same shape of mistake. The Go implementation carries the
/// same list, because §8.8.1 is one rule and two readings of it would put the
/// two emitters back where they started.
fn looks_numeric(s: &str) -> bool {
    let t = s.strip_prefix(['-', '+']).unwrap_or(s);
    if t.is_empty() {
        return false;
    }
    if matches!(t.to_ascii_lowercase().as_str(), ".inf" | ".nan") {
        return true;
    }

    let stripped = t.replace('_', "");
    let (digits, radix) = match stripped.get(..2).map(str::to_ascii_lowercase).as_deref() {
        Some("0x") => (&stripped[2..], 16),
        Some("0o") => (&stripped[2..], 8),
        Some("0b") => (&stripped[2..], 2),
        _ => (stripped.as_str(), 10),
    };
    if !digits.is_empty() && i64::from_str_radix(digits, radix).is_ok() {
        return true;
    }
    // A leading zero before digits is octal to a YAML 1.1 reader.
    if stripped.len() > 1
        && stripped.starts_with('0')
        && stripped[1..].bytes().all(|b| b.is_ascii_digit())
    {
        return true;
    }
    if stripped.parse::<f64>().is_ok() {
        return true;
    }
    // Sexagesimal: digit groups separated by colons, read as seconds.
    if stripped.contains(':')
        && stripped
            .split(':')
            .all(|p| !p.is_empty() && p.bytes().all(|b| b.is_ascii_digit()))
    {
        return true;
    }
    false
}
