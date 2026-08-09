//! The schema language (`conformance/README.md`), and the `schema-load` stage.
//!
//! Three checks live here because the corpus places them here: INV-010 (a
//! schema may not declare a child called `values`), INV-015 (a `sealed_from`
//! carries both template and path) and the model-version check of INV-033.
//! `schema-load` is not a stage SPEC §8 lists — `docs/spec-defects.md` SD-002.

use crate::error::{code, Error, Result, Stage};
use crate::value::Value;
use crate::MODEL_VERSION;

/// The eight irreducible atoms, in the order SPEC §6.1 states them. That order
/// is also the canonical member order of a node, so this array is both the
/// membership test and the sort key.
pub const PRIMITIVES: [&str; 8] = [
    "shape", "role", "behavior", "contract", "address", "identity", "event", "access",
];

#[must_use]
pub fn is_primitive(name: &str) -> bool {
    PRIMITIVES.contains(&name)
}

/// Keys of the schema language that are not primitive names. `shape` is both:
/// declared as a keyword (`shape: scalar` with a sibling `scalar_type:`) and
/// materialized as the `shape` primitive node.
const KEYWORDS: [&str; 8] = [
    "shape",
    "scalar_type",
    "default",
    "required",
    "children",
    "item",
    "sealed_from",
    "content",
];

#[derive(Debug, Clone)]
pub struct SealedRef {
    pub template: String,
    pub path: String,
}

#[derive(Debug, Clone, Default)]
pub struct SchemaNode {
    pub shape: Option<String>,
    pub scalar_type: Option<String>,
    pub default: Option<Value>,
    pub required: bool,
    pub children: Vec<(String, SchemaNode)>,
    pub item: Option<Box<SchemaNode>>,
    pub sealed_from: Option<SealedRef>,
    /// Primitive declarations, in declaration order. Re-ordered to §6.1 order
    /// only at serialization.
    pub prims: Vec<(String, Value)>,
    /// `content` of a template entry: the values a sealed instance carries.
    pub content: Option<Value>,
}

impl SchemaNode {
    #[must_use]
    pub fn child(&self, name: &str) -> Option<&SchemaNode> {
        self.children
            .iter()
            .find(|(n, _)| n == name)
            .map(|(_, c)| c)
    }
}

#[derive(Debug, Clone)]
pub struct Schema {
    pub model: String,
    pub root: SchemaNode,
    /// template name -> path within the template -> node
    pub templates: Vec<(String, Vec<(String, SchemaNode)>)>,
}

impl Schema {
    #[must_use]
    pub fn template_entry(&self, template: &str, path: &str) -> Option<&SchemaNode> {
        self.templates
            .iter()
            .find(|(n, _)| n == template)
            .and_then(|(_, entries)| entries.iter().find(|(p, _)| p == path))
            .map(|(_, n)| n)
    }
}

/// Parse and validate a schema on its own.
///
/// # Errors
/// Returns the first violation found, at stage `schema-load`.
pub fn load(data: &[u8]) -> Result<Schema> {
    let raw = crate::value::parse(data, Stage::SchemaLoad, "$", "schema")?;
    let Some(m) = raw.as_map() else {
        return Err(Error::new(
            code::MALFORMED_DOCUMENT,
            "INV-013",
            Stage::SchemaLoad,
            "$",
            "schema must be a mapping",
        ));
    };

    // INV-033 — an object is handed over at a KNOWN model version, so a schema
    // at an unknown one cannot produce such an object and is refused here
    // rather than at the boundary, where the fault is not.
    let model = match m.get("model") {
        Some(Value::Str(s)) => s.clone(),
        Some(other) => other.to_plain_string(),
        None => {
            return Err(Error::new(
                code::UNSUPPORTED_MODEL_VERSION,
                "INV-033",
                Stage::SchemaLoad,
                "$.model",
                "schema declares no model version",
            ))
        }
    };
    if model != MODEL_VERSION {
        return Err(Error::new(
            code::UNSUPPORTED_MODEL_VERSION,
            "INV-033",
            Stage::SchemaLoad,
            "$.model",
            format!(
                "model version `{model}` is not supported; this implementation is {MODEL_VERSION}"
            ),
        ));
    }

    let Some(root_raw) = m.get("root") else {
        return Err(Error::new(
            code::MALFORMED_DOCUMENT,
            "INV-013",
            Stage::SchemaLoad,
            "$.root",
            "schema must declare a root node",
        ));
    };
    let root = parse_node(root_raw, "$")?;

    let mut templates = Vec::new();
    if let Some(t) = m.get("templates") {
        let Some(tm) = t.as_map() else {
            return Err(Error::new(
                code::MALFORMED_DOCUMENT,
                "INV-015",
                Stage::SchemaLoad,
                "$templates",
                "templates must be a mapping",
            ));
        };
        for (name, paths) in &tm.0 {
            let Some(pm) = paths.as_map() else {
                return Err(Error::new(
                    code::MALFORMED_DOCUMENT,
                    "INV-015",
                    Stage::SchemaLoad,
                    format!("$templates.{name}"),
                    "a template must be a mapping of path to node",
                ));
            };
            let mut entries = Vec::new();
            for (path, node) in &pm.0 {
                entries.push((
                    path.clone(),
                    parse_node(node, &format!("$templates.{name}.{path}"))?,
                ));
            }
            templates.push((name.clone(), entries));
        }
    }

    Ok(Schema {
        model,
        root,
        templates,
    })
}

fn parse_node(raw: &Value, path: &str) -> Result<SchemaNode> {
    let Some(m) = raw.as_map() else {
        return Err(Error::new(
            code::MALFORMED_DOCUMENT,
            "INV-013",
            Stage::SchemaLoad,
            path,
            "a schema node must be a mapping",
        ));
    };

    let mut n = SchemaNode::default();
    for (k, v) in &m.0 {
        match k.as_str() {
            "shape" => n.shape = Some(v.to_plain_string()),
            "scalar_type" => n.scalar_type = Some(v.to_plain_string()),
            "default" => n.default = Some(v.clone()),
            "required" => n.required = matches!(v, Value::Bool(true)),
            "content" => n.content = Some(v.clone()),
            "children" => n.children = parse_children(v, path)?,
            "item" => n.item = Some(Box::new(parse_node(v, &format!("{path}[]"))?)),
            "sealed_from" => {
                let sealed_path = format!("{path}.values.sealed_from");
                let Some(sm) = v.as_map() else {
                    return Err(Error::new(
                        code::SEALED_MISSING_TEMPLATE_OR_PATH,
                        "INV-015",
                        Stage::SchemaLoad,
                        sealed_path,
                        "sealed_from must be a mapping carrying template and path",
                    ));
                };
                // INV-015 — both, always. Template identity alone is not
                // provenance: it says which template, not which node in it.
                //
                // `null` counts as absent. A key present with a null value is
                // Some(Value::Null) and would otherwise pass a presence test,
                // producing a sealed origin whose template is the string
                // "null" — provenance pointing at nothing, which is worse than
                // a rejection because it looks like an answer.
                let present = |k: &str| match sm.get(k) {
                    None | Some(Value::Null) => None,
                    Some(v) => Some(v),
                };
                let (Some(t), Some(p)) = (present("template"), present("path")) else {
                    return Err(Error::new(
                        code::SEALED_MISSING_TEMPLATE_OR_PATH,
                        "INV-015",
                        Stage::SchemaLoad,
                        sealed_path,
                        "a sealed_from must carry both template and path",
                    ));
                };
                n.sealed_from = Some(SealedRef {
                    template: t.to_plain_string(),
                    path: p.to_plain_string(),
                });
            }
            other if is_primitive(other) => n.prims.push((other.to_string(), v.clone())),
            other => {
                return Err(Error::new(
                    code::UNKNOWN_SCHEMA_KEY,
                    "INV-021",
                    Stage::SchemaLoad,
                    path,
                    format!("`{other}` is neither a schema keyword nor a primitive name"),
                ))
            }
        }
    }

    // A node declares its shape, or is sealed from a template that declares it.
    if n.shape.is_none() && n.sealed_from.is_none() {
        return Err(Error::new(
            code::SCHEMA_SHAPE_MISSING,
            "INV-011",
            Stage::SchemaLoad,
            path,
            "a schema node must declare a shape",
        ));
    }
    let _ = KEYWORDS;
    Ok(n)
}

/// The `children` mapping of a schema node.
///
/// Split out of `parse_node` because two of the reservations below carry their
/// own explanation and the arm had grown past the point where the reader can
/// see the whole match.
fn parse_children(v: &Value, path: &str) -> Result<Vec<(String, SchemaNode)>> {
    let Some(cm) = v.as_map() else {
        return Err(Error::new(
            code::MALFORMED_DOCUMENT,
            "INV-013",
            Stage::SchemaLoad,
            format!("{path}.children"),
            "children must be a mapping",
        ));
    };
    let mut out = Vec::new();
    for (cname, cnode) in &cm.0 {
        // INV-010 — `values` and, since 0.2, `origin` are reserved members of
        // every node. A child of either name would be indistinguishable from
        // the member itself.
        if cname == "values" || cname == "origin" {
            return Err(Error::new(
                code::SCHEMA_RESERVED_CHILD_VALUES,
                "INV-010",
                Stage::SchemaLoad,
                format!("{path}.values.{cname}"),
                format!("a schema may not declare a child named `{cname}`; it is a reserved member of every node"),
            ));
        }
        out.push((
            cname.clone(),
            parse_node(cnode, &format!("{path}.values.{cname}"))?,
        ));
    }
    Ok(out)
}
