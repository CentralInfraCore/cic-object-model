//! The materialization pipeline, SPEC §8.1–§8.6.
//!
//! Authoring YAML plus a schema in, a canonical object out. The stages are not
//! separate passes here — they are a single recursive walk that raises each
//! stage's rejections at the point the walk reaches them. What matters for
//! conformance is that a rejection names the stage SPEC §8 puts it at, and that
//! an earlier stage's rejection wins over a later one on the same input.

use crate::canonical;
use crate::error::{code, Error, Result, Stage};
use crate::node::{Node, Payload};
use crate::origin::{Origin, Sealed};
use crate::schema::{self, Schema, SchemaNode};
use crate::value::{Map, Value};

/// A materialized, validated CIC object.
///
/// The type a module consumes (INV-032). It cannot be constructed outside this
/// crate: the fields are private and `materialize` is the only producer.
#[derive(Debug, Clone)]
pub struct CanonicalObject {
    model: String,
    root: Node,
    bytes: Vec<u8>,
}

impl CanonicalObject {
    /// The version this object was materialized at.
    ///
    /// INV-033: the version belongs to the hand-off, not the object. It is not
    /// a member of the node tree and never appears in the serialization.
    #[must_use]
    pub fn model_version(&self) -> &str {
        &self.model
    }

    /// The deterministic serialization (INV-030).
    #[must_use]
    pub fn canonical_yaml(&self) -> &[u8] {
        &self.bytes
    }

    /// The root node.
    #[must_use]
    pub fn root(&self) -> &Node {
        &self.root
    }
}

/// Run the whole pipeline of SPEC §8.
///
/// # Errors
/// Returns the first violation found, carrying the stage it belongs to.
pub fn materialize(schema_yaml: &[u8], input_yaml: &[u8]) -> Result<CanonicalObject> {
    let schema = schema::load(schema_yaml)?;
    let input = crate::value::parse(input_yaml, Stage::EntryValidation, "$", "input")?;

    let ctx = Ctx { schema: &schema };
    let root = ctx.build(&schema.root, Some(&input), "$", true, None)?;

    let bytes = canonical::to_yaml(&root);
    // The object is validated against §8.7 through the same walk an object
    // arriving without its schema gets. Materializing something the validator
    // would reject is a defect, and this is where it surfaces rather than in a
    // consumer.
    crate::validate::validate_canonical_document(&bytes)?;

    Ok(CanonicalObject {
        model: schema.model.clone(),
        root,
        bytes,
    })
}

/// The address of the first authored leaf at or below `path`.
///
/// Descends the authored mapping taking the first member at each level. Which
/// member is arbitrary when there are several, and deliberately so: the input
/// is already invalid, and naming one real position a reader can find beats
/// naming the boundary they did not write.
fn deepest_authored(v: &Value, path: &str) -> String {
    match v.as_map() {
        Some(m) if !m.is_empty() => {
            let (k, child) = &m.0[0];
            deepest_authored(child, &format!("{path}.values.{k}"))
        }
        _ => path.to_string(),
    }
}

struct Ctx<'a> {
    schema: &'a Schema,
}

/// What the authoring input said about a node, after the structural
/// discriminator has been applied (INV-008: the SCHEMA POSITION decides, not
/// the shape of the mapping).
struct Authored<'a> {
    /// The payload the input supplied, if any.
    payload: Option<&'a Value>,
    /// Primitive members the input supplied, in an envelope form.
    primitives: Vec<(String, &'a Value)>,
    /// Whether the input asserted this node at all.
    present: bool,
}

impl<'a> Ctx<'a> {
    /// Build one node.
    ///
    /// `sealed` is the boundary context inherited from an enclosing
    /// `sealed_from`, and it is what turns rows 3 and 4 of the §5.3 truth table
    /// on.
    fn build<'s>(
        &self,
        sn: &'s SchemaNode,
        authored: Option<&Value>,
        path: &str,
        is_root: bool,
        sealed: Option<&Sealed>,
    ) -> Result<Node>
    where
        'a: 's,
    {
        // A sealed node takes its shape, children and defaults from the
        // template entry, and closes authoring at and below itself (INV-019).
        let (effective, sealed_here, content) = self.resolve_sealed(sn, path)?;
        let sealed_ctx = sealed_here.as_ref().or(sealed);

        if sealed_ctx.is_some() {
            if let Some(v) = authored {
                // INV-019 — authoring input MUST NOT supply any value at or
                // below a node whose origin contains `sealed`.
                //
                // The path names the deepest position the input actually
                // reached, not the boundary it crossed. The boundary is where
                // the rule lives; the authored leaf is what the author has to
                // go and delete, and an error that names the wrong one of those
                // sends them to a node they did not write.
                return Err(Error::new(
                    code::AUTHORING_BELOW_SEALED,
                    "INV-019",
                    Stage::EntryValidation,
                    deepest_authored(v, path),
                    "authoring input supplied a value at or below a sealed boundary",
                ));
            }
        }

        let a = Self::discriminate(effective, authored, path)?;
        let shape = effective.shape.as_deref().unwrap_or("object");
        Self::check_arity(shape, &a, path)?;

        // INV-022 — a required value must be authored or defaultable. The flag
        // was parsed and never read, so a required child that nobody supplied
        // materialized as `null` with `origin: [schema]`: the sole materializer
        // manufacturing an object that should not exist, which no downstream
        // check can repair because it is well-formed.
        if effective.required
            && !a.present
            && effective.default.is_none()
            && content.is_none()
            && sealed_ctx.is_none()
        {
            return Err(Error::new(
                code::REQUIRED_VALUE_MISSING,
                "INV-022",
                Stage::DefaultMaterialization,
                path,
                "a required value was neither authored nor defaultable",
            ));
        }

        let payload = self.build_payload(effective, shape, &a, path, sealed_ctx, content)?;
        let origin = Self::origin_of(&a, effective, sealed_ctx, content);

        let mut node = Node {
            path: path.to_string(),
            is_root,
            payload,
            origin: origin.clone(),
            primitives: Vec::new(),
        };
        node.primitives = Self::build_primitives(effective, &a, path, &origin);
        Ok(node)
    }

    /// Resolve a `sealed_from` to the template entry it names.
    fn resolve_sealed<'s>(
        &self,
        sn: &'s SchemaNode,
        path: &str,
    ) -> Result<(&'s SchemaNode, Option<Sealed>, Option<&'s Value>)>
    where
        'a: 's,
    {
        let Some(ref r) = sn.sealed_from else {
            return Ok((sn, None, None));
        };
        let Some(entry) = self.schema.template_entry(&r.template, &r.path) else {
            return Err(Error::new(
                code::TEMPLATE_NOT_FOUND,
                "INV-015",
                Stage::SchemaLoad,
                path,
                format!("no template `{}` declares `{}`", r.template, r.path),
            ));
        };
        Ok((
            entry,
            Some(Sealed {
                template: r.template.clone(),
                path: r.path.clone(),
            }),
            entry.content.as_ref(),
        ))
    }

    /// A declared position takes a payload of its own arity (INV-008).
    ///
    /// This checks arity, not the declared `scalar_type`. Whether
    /// `scalar_type: integer` constrains a value is a question the
    /// specification does not answer; whether a scalar position holds a scalar
    /// is not in doubt.
    ///
    /// It was missing, and each shape failed differently for it. A sequence at
    /// a scalar position reached the emitter, whose collection arm is a
    /// `debug_assert!(false, ...)` — so a five-byte input PANICKED a debug
    /// build and emitted `null` in a release one. A non-sequence at a list
    /// position went through `Value::as_seq`, whose `None` skipped the loop and
    /// produced an EMPTY LIST: the authored value disappeared and the object
    /// still claimed `origin: [yaml]` for it.
    fn check_arity(shape: &str, a: &Authored<'_>, path: &str) -> Result<()> {
        let Some(v) = a.payload else {
            return Ok(());
        };
        let mismatch = |what: &str| {
            Err(Error::new(
                code::TYPE_MISMATCH,
                "INV-008",
                Stage::EntryValidation,
                path,
                format!("a {shape} position requires a {what} payload"),
            ))
        };
        match shape {
            // Opaque is carried verbatim: any payload is its payload.
            "opaque" => Ok(()),
            "scalar" => match v {
                Value::Seq(_) => mismatch("scalar payload, not a sequence"),
                Value::Map(_) => mismatch("scalar payload, not a mapping"),
                _ => Ok(()),
            },
            "list" => match v {
                Value::Seq(_) => Ok(()),
                _ => mismatch("sequence"),
            },
            _ => match v {
                Value::Map(_) => Ok(()),
                _ => mismatch("mapping"),
            },
        }
    }

    /// The structural discriminator (SPEC §4, INV-008).
    ///
    /// The schema position decides whether a mapping is a node envelope or a
    /// payload — never the shape of the mapping itself. At a structured-object
    /// position a mapping is the payload (INV-009); at an opaque position the
    /// whole thing is the payload, keywords and all; only at a scalar or list
    /// position can a mapping be an envelope, because there the payload cannot
    /// be a mapping.
    fn discriminate<'v>(
        sn: &SchemaNode,
        authored: Option<&'v Value>,
        path: &str,
    ) -> Result<Authored<'v>> {
        let shape = sn.shape.as_deref().unwrap_or("object");
        let Some(v) = authored else {
            return Ok(Authored {
                payload: None,
                primitives: Vec::new(),
                present: false,
            });
        };

        let is_envelope_position = matches!(shape, "scalar" | "list");
        let Some(m) = v.as_map().filter(|_| is_envelope_position) else {
            // INV-007 — origin is never authored, at any position that could
            // carry it.
            if let Some(map) = v.as_map() {
                if map.contains("origin") && shape != "opaque" {
                    return Err(Error::new(
                        code::ORIGIN_DECLARED_IN_INPUT,
                        "INV-007",
                        Stage::EntryValidation,
                        format!("{path}.origin"),
                        "authoring input declared an origin; origin is derived, never authored",
                    ));
                }
            }
            return Ok(Authored {
                payload: Some(v),
                primitives: Vec::new(),
                present: true,
            });
        };

        if m.contains("origin") {
            return Err(Error::new(
                code::ORIGIN_DECLARED_IN_INPUT,
                "INV-007",
                Stage::EntryValidation,
                format!("{path}.origin"),
                "authoring input declared an origin; origin is derived, never authored",
            ));
        }

        let mut primitives = Vec::new();
        for (k, val) in &m.0 {
            if k == "values" {
                continue;
            }
            // INV-021 — a node carries `values`, `origin` and primitives. A
            // member that is none of those has no interpretation.
            if !schema::is_primitive(k) {
                return Err(Error::new(
                    code::UNKNOWN_PRIMITIVE,
                    "INV-021",
                    Stage::PrimitiveEvaluation,
                    format!("{path}.{k}"),
                    format!("`{k}` is not one of the eight primitives of SPEC §6.1"),
                ));
            }
            primitives.push((k.clone(), val));
        }

        Ok(Authored {
            payload: m.get("values"),
            primitives,
            present: true,
        })
    }

    fn build_payload(
        &self,
        sn: &SchemaNode,
        shape: &str,
        a: &Authored<'_>,
        path: &str,
        sealed: Option<&Sealed>,
        content: Option<&Value>,
    ) -> Result<Payload> {
        match shape {
            // §4: an opaque payload is kept verbatim. No primitive
            // interpretation applies below it and no key inside it is a
            // keyword — which is what vector 012 exists to pin.
            "opaque" => Ok(Payload::Raw(
                a.payload
                    .or(sn.default.as_ref())
                    .cloned()
                    .unwrap_or(Value::Null),
            )),
            "scalar" => Ok(Payload::Scalar(
                content
                    .or(a.payload)
                    .or(sn.default.as_ref())
                    .cloned()
                    .unwrap_or(Value::Null),
            )),
            "list" => {
                let source = a.payload.or(sn.default.as_ref());
                let mut items = Vec::new();
                if let Some(seq) = source.and_then(Value::as_seq) {
                    let item_schema = sn.item.as_deref().cloned().unwrap_or_default();
                    for (i, entry) in seq.iter().enumerate() {
                        items.push(self.build(
                            &item_schema,
                            Some(entry),
                            &format!("{path}.values[{i}]"),
                            false,
                            sealed,
                        )?);
                    }
                }
                Ok(Payload::Seq(items))
            }
            _ => self.build_object_payload(sn, a, path, sealed, content),
        }
    }

    fn build_object_payload(
        &self,
        sn: &SchemaNode,
        a: &Authored<'_>,
        path: &str,
        sealed: Option<&Sealed>,
        content: Option<&Value>,
    ) -> Result<Payload> {
        let authored_map = a.payload.and_then(Value::as_map);

        // INV-029 — the object closure. Every member of an authored mapping is
        // either a declared child or nothing at all; there is no way to smuggle
        // an uninterpreted object graph past the model.
        if let Some(m) = authored_map {
            for k in m.keys() {
                if sn.child(k).is_none() {
                    return Err(Error::new(
                        code::UNDECLARED_OBJECT,
                        "INV-029",
                        Stage::EntryValidation,
                        format!("{path}.values.{k}"),
                        format!("`{k}` is not declared by the schema at this position"),
                    ));
                }
            }
        }

        let content_map = content.and_then(Value::as_map);
        let mut entries = Vec::new();
        for (name, child_schema) in &sn.children {
            let child_authored = authored_map.and_then(|m| m.get(name));
            let child_content = content_map.and_then(|m| m.get(name));
            let child_path = format!("{path}.values.{name}");

            // A sealed node's values come from the template's `content`, which
            // is not authoring input and so does not trip INV-019.
            let child = if let Some(cv) = child_content {
                let mut n = self.build(child_schema, None, &child_path, false, sealed)?;
                n.payload = Payload::Scalar(cv.clone());
                n.origin = sealed.map_or(Origin::Schema, |s| Origin::Sealed(s.clone()));
                n
            } else {
                self.build(child_schema, child_authored, &child_path, false, sealed)?
            };
            entries.push((name.clone(), child));
        }
        Ok(Payload::Map(entries))
    }

    /// The origin of a node, from the §5.3 truth table.
    fn origin_of(
        a: &Authored<'_>,
        sn: &SchemaNode,
        sealed: Option<&Sealed>,
        content: Option<&Value>,
    ) -> Origin {
        match sealed {
            // Row 3 — the template defined, closed AND valued the node.
            Some(s) if content.is_some() => Origin::Sealed(s.clone()),
            // Row 4 — the template defined and closed it; the value came from
            // the template node's own schema default. `sealed` and `schema`
            // combine where `sealed` and `yaml` do not, because they describe
            // different dimensions: structural authority and value source.
            Some(s) if sn.default.is_some() => Origin::SealedSchema(s.clone()),
            Some(s) => Origin::Sealed(s.clone()),
            // Row 1 — the instance YAML supplied it.
            None if a.present => Origin::Yaml,
            // Row 2 — it materialized from this node's schema default.
            None => Origin::Schema,
        }
    }

    /// Materialize the primitive members (INV-035): a primitive's payload is a
    /// node tree like any other, not a raw mapping hung off the node.
    fn build_primitives(
        sn: &SchemaNode,
        a: &Authored<'_>,
        path: &str,
        node_origin: &Origin,
    ) -> Vec<(String, Node)> {
        let mut declared: Vec<(String, Value, Origin)> = Vec::new();

        // `shape` is always materialized: every node has one, and it is the
        // primitive that makes the node's own arity addressable.
        declared.push((
            "shape".into(),
            Self::shape_payload(sn),
            // The shape always comes from the schema, including under a sealed
            // boundary — the template is still a schema.
            Origin::Schema,
        ));

        for (name, v) in &sn.prims {
            if name == "shape" {
                continue;
            }
            declared.push((name.clone(), v.clone(), Origin::Schema));
        }
        for (name, v) in &a.primitives {
            if name == "shape" {
                continue;
            }
            // An input-supplied primitive replaces the declaration and carries
            // the node's own origin, because the input is what authored it.
            let origin = if matches!(node_origin, Origin::Yaml) {
                Origin::Yaml
            } else {
                node_origin.clone()
            };
            if let Some(slot) = declared.iter_mut().find(|(n, _, _)| n == name) {
                slot.1 = (*v).clone();
                slot.2 = origin;
            } else {
                declared.push((name.clone(), (*v).clone(), origin));
            }
        }

        // §6.1 order is the canonical member order.
        declared.sort_by_key(|(name, _, _)| {
            schema::PRIMITIVES
                .iter()
                .position(|p| p == name)
                .unwrap_or(usize::MAX)
        });

        let mut out = Vec::new();
        for (name, value, origin) in declared {
            let value = if name == "access" {
                Self::inject_inherit(&value)
            } else {
                value
            };
            let p = Self::primitive_node(&value, &format!("{path}.{name}"), &origin);
            out.push((name, p));
        }
        out
    }

    /// The `shape` primitive's payload: the declared arity, and the scalar type
    /// when there is one.
    fn shape_payload(sn: &SchemaNode) -> Value {
        let mut m = Map::default();
        m.insert(
            "type",
            Value::Str(sn.shape.clone().unwrap_or_else(|| "object".into())),
        );
        if let Some(ref t) = sn.scalar_type {
            m.insert("scalar_type", Value::Str(t.clone()));
        }
        Value::Map(m)
    }

    /// `inherit` defaults to true on every access operation that does not
    /// declare it, and an operation block is ordered rules, inherit,
    /// `default_injection`.
    ///
    /// The injection is what makes the default visible in the object rather
    /// than implied by its absence: a policy decision point reading the object
    /// must not have to know this rule to get the right answer.
    fn inject_inherit(access: &Value) -> Value {
        let Some(ops) = access.as_map() else {
            return access.clone();
        };
        let mut out = Map::default();
        for (op, body) in &ops.0 {
            let Some(b) = body.as_map() else {
                out.insert(op.clone(), body.clone());
                continue;
            };
            let mut nb = Map::default();
            for key in ["rules", "inherit", "default_injection"] {
                if key == "inherit" && !b.contains("inherit") {
                    nb.insert("inherit", Value::Bool(true));
                } else if let Some(v) = b.get(key) {
                    nb.insert(key, v.clone());
                }
            }
            for (k, v) in &b.0 {
                if !matches!(k.as_str(), "rules" | "inherit" | "default_injection") {
                    nb.insert(k.clone(), v.clone());
                }
            }
            out.insert(op.clone(), Value::Map(nb));
        }
        Value::Map(out)
    }

    /// Turn a primitive's declared value into a node tree.
    ///
    /// INV-035 is why this exists. Model 0.1 left primitive payloads as raw
    /// mappings, which made `network.values.mtu.access.read` a path into a YAML
    /// blob rather than an address of a node — false of every object it
    /// produced. A sequence stays raw: it declares no element position, so its
    /// entries have no addresses to be nodes at.
    fn primitive_node(v: &Value, path: &str, origin: &Origin) -> Node {
        let payload = match v {
            Value::Map(m) => Payload::Map(
                m.0.iter()
                    .map(|(k, val)| {
                        (
                            k.clone(),
                            Self::primitive_node(val, &format!("{path}.values.{k}"), origin),
                        )
                    })
                    .collect(),
            ),
            Value::Seq(_) => Payload::Raw(v.clone()),
            other => Payload::Scalar(other.clone()),
        };
        Node {
            path: path.to_string(),
            is_root: false,
            payload,
            origin: origin.clone(),
            primitives: Vec::new(),
        }
    }
}
