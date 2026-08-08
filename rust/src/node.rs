//! The canonical node and the reader API.
//!
//! A `Node` cannot be built outside this crate: every field is private and
//! there is no public constructor, so the only way to obtain one is
//! `materialize`. That is INV-032, enforced by the type system rather than by
//! asking callers to behave.
//!
//! Note what Rust gives here for free that Go did not. `Origin()` in the Go
//! implementation handed out the node's own slice, and a caller could rewrite
//! the provenance of a validated object through it while the serialization
//! stayed byte-identical (audit finding F-03). Here `origin()` returns `&Origin`
//! — a shared borrow of an immutable value — so the same mistake does not
//! compile. This is not the Rust implementation being more careful; it is the
//! language making one class of the bug unrepresentable.

use crate::origin::Origin;
use crate::value::Value;

/// A node's payload — the `values` member (INV-001).
#[derive(Debug, Clone, PartialEq)]
pub enum Payload {
    /// A single value.
    Scalar(Value),
    /// A list whose element positions the schema declared, so each entry is a
    /// node in its own right.
    Seq(Vec<Node>),
    /// A structured payload: named children, in canonical order.
    Map(Vec<(String, Node)>),
    /// Kept verbatim. An opaque payload (SPEC §4), or a list inside a primitive
    /// declaration, which declares no element position and so is not
    /// materialized into nodes (INV-035).
    Raw(Value),
}

/// A CIC node: exactly one `values` member (INV-001), exactly one `origin`
/// member (INV-002), and one member per primitive its schema declares
/// (INV-022).
#[derive(Debug, Clone, PartialEq)]
pub struct Node {
    pub(crate) path: String,
    pub(crate) is_root: bool,
    pub(crate) payload: Payload,
    pub(crate) origin: Origin,
    /// Primitive members, already in the §6.1 canonical order.
    pub(crate) primitives: Vec<(String, Node)>,
}

impl Node {
    /// This node's address in the object (`$.values.mtu`).
    #[must_use]
    pub fn path(&self) -> &str {
        &self.path
    }

    /// The node's authoring authority (SPEC §5).
    #[must_use]
    pub fn origin(&self) -> &Origin {
        &self.origin
    }

    /// The payload, for callers that need to match on its arity.
    #[must_use]
    pub fn payload(&self) -> &Payload {
        &self.payload
    }

    /// A scalar payload, if that is what this node has.
    #[must_use]
    pub fn scalar(&self) -> Option<&Value> {
        match &self.payload {
            Payload::Scalar(v) => Some(v),
            _ => None,
        }
    }

    /// A named child of a structured payload.
    #[must_use]
    pub fn child(&self, name: &str) -> Option<&Node> {
        match &self.payload {
            Payload::Map(entries) => entries.iter().find(|(n, _)| n == name).map(|(_, c)| c),
            _ => None,
        }
    }

    /// Child names, canonical order.
    #[must_use]
    pub fn children(&self) -> Vec<&str> {
        match &self.payload {
            Payload::Map(entries) => entries.iter().map(|(n, _)| n.as_str()).collect(),
            _ => Vec::new(),
        }
    }

    /// Entry count of a list payload; zero for anything else.
    #[must_use]
    pub fn len(&self) -> usize {
        match &self.payload {
            Payload::Seq(items) => items.len(),
            _ => 0,
        }
    }

    #[must_use]
    pub fn is_empty(&self) -> bool {
        self.len() == 0
    }

    /// A list entry by position.
    #[must_use]
    pub fn at(&self, i: usize) -> Option<&Node> {
        match &self.payload {
            Payload::Seq(items) => items.get(i),
            _ => None,
        }
    }

    /// A materialized primitive member.
    #[must_use]
    pub fn primitive(&self, name: &str) -> Option<&Node> {
        self.primitives
            .iter()
            .find(|(n, _)| n == name)
            .map(|(_, p)| p)
    }

    /// Primitive names, canonical order.
    #[must_use]
    pub fn primitive_names(&self) -> Vec<&str> {
        self.primitives.iter().map(|(n, _)| n.as_str()).collect()
    }

    /// Resolve an address (SPEC §2.5).
    ///
    /// `values` steps into the payload and is never optional; anything else is
    /// a member of the node itself. That mandatory step is what makes an
    /// address unique (INV-040): without it `$.values.mtu` and
    /// `$.values.values.values.mtu` would name the same node, and a primitive
    /// would be indistinguishable from a payload child sharing its name.
    ///
    /// A leading `$` means the root and is not decoration — an absolute address
    /// is answered by the root and by nothing else. Without it the address is
    /// relative to the receiver. Malformed spellings are rejected rather than
    /// repaired, so a caller can tell whether the string it holds is canonical.
    #[must_use]
    pub fn get(&self, path: &str) -> Option<&Node> {
        let (segments, absolute) = parse_path(path)?;
        if absolute && !self.is_root {
            return None;
        }

        let mut cur = self;
        let mut in_payload = false;
        for seg in segments {
            if in_payload {
                cur = cur.child(seg)?;
                in_payload = false;
            } else if seg == "values" {
                in_payload = true;
            } else if let Some(i) = parse_index(seg) {
                cur = cur.at(i)?;
            } else {
                cur = cur.primitive(seg)?;
            }
        }
        // A trailing `values` names the payload, which is not a node.
        if in_payload {
            return None;
        }
        Some(cur)
    }
}

/// `values[i]` — the payload step and the entry in one token.
fn parse_index(seg: &str) -> Option<usize> {
    seg.strip_prefix("values[")?
        .strip_suffix(']')?
        .parse::<usize>()
        .ok()
}

/// Split an address, reporting whether it is absolute. Returns `None` for a
/// malformed address.
fn parse_path(path: &str) -> Option<(Vec<&str>, bool)> {
    match path {
        "" => return Some((Vec::new(), false)), // the receiver
        "$" => return Some((Vec::new(), true)), // the root
        _ => {}
    }
    let (rest, absolute) = match path.strip_prefix('$') {
        // `$values.mtu` — `$` not followed by a separator.
        Some(r) => (r.strip_prefix('.')?, true),
        None => (path, false),
    };
    if rest.is_empty() || rest.starts_with('.') || rest.ends_with('.') {
        return None;
    }
    let segments: Vec<&str> = rest.split('.').collect();
    if segments.iter().any(|s| s.is_empty()) {
        return None;
    }
    Some((segments, absolute))
}
