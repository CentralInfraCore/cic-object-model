//! An order-preserving YAML value.
//!
//! Why not use the parser's own value type directly: canonical output is
//! member-ordered (SPEC §8.8), so insertion order has to survive parsing, and
//! the mapping keys of this model are always strings. A small owned type makes
//! both properties structural instead of something every call site remembers.
//!
//! `Map` is a `Vec` of pairs rather than a hash map. The corpus never has
//! mappings large enough for lookup cost to matter, and a `Vec` keeps order
//! without a second dependency.
//!
//! # A limit worth stating: duplicate keys
//!
//! `a: 1` followed by `a: 2` in one mapping does not reach this code as two
//! entries. The parser resolves it to `{a: 2}` — last wins, silently — so a
//! duplicate-key check here would be unreachable code claiming a guarantee it
//! cannot make. Measured, not assumed: `Yaml::load_from_str("a: 1\na: 2\n")`
//! yields a single-entry mapping.
//!
//! This matters more than it looks. A model whose stated purpose is unique
//! addressing (INV-040) silently accepts an authoring document in which one
//! address was written twice with different values, and takes the second. The
//! Go implementation is in the same position for the same reason, so the two
//! agree — which is exactly the kind of agreement that proves nothing, because
//! it comes from the two YAML libraries behaving alike rather than from the
//! specification saying anything. SPEC.md does not address duplicate keys at
//! all; it should, and until it does neither implementation can be said to be
//! right here.
//!
//! # Anchors and aliases are refused, before a tree exists
//!
//! An alias is not part of the schema language or the authoring format, and
//! leaving it to the tree builder is not an option. Measured on this parser:
//!
//! ```text
//! 393 bytes of nested aliases -> 12,345,678 nodes, 2.7 seconds
//! ```
//!
//! That is a ~31,000x amplification on input this library treats as untrusted,
//! and it happens during COMPOSITION — by the time a value reaches the code
//! below, the memory is already spent, so no budget checked here can help.
//!
//! So the document is scanned as an event stream first. Scanning is linear in
//! the input's own size and never expands an alias, so an `Alias` event can be
//! refused for the price of reading the bytes once.

use crate::error::{code, Error, Result, Stage};
use saphyr::{LoadableYamlNode, Yaml};
use saphyr_parser::{Event, Parser};

#[derive(Debug, Clone, PartialEq)]
pub enum Value {
    Null,
    Bool(bool),
    Int(i64),
    Float(f64),
    Str(String),
    Seq(Vec<Value>),
    Map(Map),
}

#[derive(Debug, Clone, Default, PartialEq)]
pub struct Map(pub Vec<(String, Value)>);

impl Map {
    #[must_use]
    pub fn get(&self, k: &str) -> Option<&Value> {
        self.0.iter().find(|(key, _)| key == k).map(|(_, v)| v)
    }
    #[must_use]
    pub fn contains(&self, k: &str) -> bool {
        self.get(k).is_some()
    }
    pub fn insert(&mut self, k: impl Into<String>, v: Value) {
        let k = k.into();
        if let Some(slot) = self.0.iter_mut().find(|(key, _)| *key == k) {
            slot.1 = v;
        } else {
            self.0.push((k, v));
        }
    }
    #[must_use]
    pub fn keys(&self) -> Vec<&str> {
        self.0.iter().map(|(k, _)| k.as_str()).collect()
    }
    #[must_use]
    pub fn len(&self) -> usize {
        self.0.len()
    }
    #[must_use]
    pub fn is_empty(&self) -> bool {
        self.0.is_empty()
    }
}

impl Value {
    #[must_use]
    pub fn as_map(&self) -> Option<&Map> {
        match self {
            Value::Map(m) => Some(m),
            _ => None,
        }
    }
    #[must_use]
    pub fn as_seq(&self) -> Option<&[Value]> {
        match self {
            Value::Seq(s) => Some(s),
            _ => None,
        }
    }
    #[must_use]
    pub fn as_str(&self) -> Option<&str> {
        match self {
            Value::Str(s) => Some(s),
            _ => None,
        }
    }
    /// The scalar rendering used in messages and in `shape.type`.
    #[must_use]
    pub fn to_plain_string(&self) -> String {
        match self {
            Value::Null => "null".into(),
            Value::Bool(b) => b.to_string(),
            Value::Int(i) => i.to_string(),
            Value::Float(f) => f.to_string(),
            Value::Str(s) => s.clone(),
            Value::Seq(_) => "<sequence>".into(),
            Value::Map(_) => "<mapping>".into(),
        }
    }
}

/// Parse a YAML document.
///
/// `stage` and `path` are carried in because the same malformed-input failure
/// is raised from three different stages and each has to name its own.
///
/// # Errors
/// Returns `E_MALFORMED_DOCUMENT` when the bytes are not UTF-8, not YAML, use a
/// construct this model does not read, or carry a non-string mapping key.
pub fn parse(data: &[u8], stage: Stage, path: &str, what: &str) -> Result<Value> {
    let text = std::str::from_utf8(data).map_err(|e| {
        Error::new(
            code::MALFORMED_DOCUMENT,
            "INV-013",
            stage,
            path,
            format!("{what} is not valid UTF-8: {e}"),
        )
    })?;

    refuse_aliases(text, stage, path, what)?;

    let docs = Yaml::load_from_str(text).map_err(|e| {
        Error::new(
            code::MALFORMED_DOCUMENT,
            "INV-013",
            stage,
            path,
            format!("{what} is not valid YAML: {e}"),
        )
    })?;

    // An empty document is the empty mapping. `{}`, an empty file and a file
    // holding only a `---` marker are the same input as far as this model is
    // concerned, and the corpus contains more than one of them. A bare `---`
    // parses as null rather than as no document at all, so both are folded here.
    let Some(doc) = docs.into_iter().next() else {
        return Ok(Value::Map(Map::default()));
    };
    if matches!(doc, Yaml::Value(saphyr::Scalar::Null)) {
        return Ok(Value::Map(Map::default()));
    }
    convert(&doc, stage, path, what)
}

/// Refuse a document containing an alias, without composing it.
///
/// The event stream is what the parser produces before it builds anything, so
/// an alias is visible here at the cost of reading the input once — and an
/// alias bomb never gets the chance to expand. See the note at the top of this
/// module for the measurement that makes this necessary rather than tidy.
fn refuse_aliases(text: &str, stage: Stage, path: &str, what: &str) -> Result<()> {
    let refuse =
        |detail: String| Error::new(code::MALFORMED_DOCUMENT, "INV-013", stage, path, detail);
    for event in Parser::new_from_str(text) {
        match event {
            Ok((Event::Alias(_), _)) => {
                return Err(refuse(format!(
                    "{what} uses a YAML alias; anchors and aliases are not part of \
                     the schema language or the authoring format, and expanding one \
                     can turn a few hundred bytes into millions of nodes"
                )))
            }
            Ok(_) => {}
            // A scan error here is the same malformed document the tree builder
            // would reject a moment later; reporting it now keeps one message.
            Err(e) => return Err(refuse(format!("{what} is not valid YAML: {e}"))),
        }
    }
    Ok(())
}

fn convert(y: &Yaml, stage: Stage, path: &str, what: &str) -> Result<Value> {
    Ok(match y {
        Yaml::Value(scalar) => convert_scalar(scalar),
        Yaml::Sequence(items) => Value::Seq(
            items
                .iter()
                .map(|i| convert(i, stage, path, what))
                .collect::<Result<Vec<_>>>()?,
        ),
        Yaml::Mapping(m) => {
            let mut out = Map::default();
            for (k, v) in m {
                // Non-string keys are refused rather than stringified. YAML
                // allows `1:` and `true:`, and coercing them would let two
                // distinct keys collapse into one name — a silent collision in
                // a structure whose whole purpose is unique addressing.
                let Yaml::Value(saphyr::Scalar::String(key)) = k else {
                    return Err(Error::new(
                        code::MALFORMED_DOCUMENT,
                        "INV-013",
                        stage,
                        path,
                        format!("{what} has a non-string mapping key; node names are strings"),
                    ));
                };
                out.insert(key.to_string(), convert(v, stage, path, what)?);
            }
            Value::Map(out)
        }
        // Aliases and tagged nodes are not part of the schema language or the
        // authoring format. Refusing them keeps the input a tree.
        _ => {
            return Err(Error::new(
                code::MALFORMED_DOCUMENT,
                "INV-013",
                stage,
                path,
                format!(
                "{what} uses a YAML construct this model does not read (alias, tag or bad value)"
            ),
            ))
        }
    })
}

fn convert_scalar(s: &saphyr::Scalar) -> Value {
    match s {
        saphyr::Scalar::Null => Value::Null,
        saphyr::Scalar::Boolean(b) => Value::Bool(*b),
        saphyr::Scalar::Integer(i) => Value::Int(*i),
        saphyr::Scalar::FloatingPoint(f) => Value::Float(f.into_inner()),
        saphyr::Scalar::String(s) => Value::Str(s.to_string()),
    }
}
