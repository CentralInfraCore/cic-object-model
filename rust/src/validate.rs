//! Final validation, SPEC §8.7, and the schema-less walk of INV-039.
//!
//! This checks an already-canonical object presented **without** its schema. It
//! is weaker than materialization by design and INV-039 says so: without the
//! schema there is no way to know which primitives a node was supposed to
//! declare, so the closure rules cannot be enforced here. What can be checked
//! is the grammar every canonical object satisfies regardless of schema — the
//! two mandatory members, the origin productions, and the members no canonical
//! node may carry.

use crate::error::{code, Error, Result, Stage};
use crate::origin::{Origin, Sealed};
use crate::schema::is_primitive;
use crate::value::{Map, Value};

/// Validate an already-canonical object.
///
/// # Errors
/// Returns the first violation found, at stage `final-validation`.
pub fn validate_canonical_document(data: &[u8]) -> Result<()> {
    let doc = crate::value::parse(data, Stage::FinalValidation, "$", "object")?;
    let Some(m) = doc.as_map() else {
        return Err(Error::new(
            code::MALFORMED_DOCUMENT,
            "INV-001",
            Stage::FinalValidation,
            "$",
            "a canonical object must be a mapping",
        ));
    };
    walk(m, "$")
}

fn walk(node: &Map, path: &str) -> Result<()> {
    // INV-001 — exactly one `values` member.
    let Some(values) = node.get("values") else {
        return Err(Error::new(
            code::MISSING_VALUES,
            "INV-001",
            Stage::FinalValidation,
            path,
            "a canonical node must carry a `values` member",
        ));
    };
    // INV-002 — exactly one `origin` member.
    let Some(origin_raw) = node.get("origin") else {
        return Err(Error::new(
            code::MISSING_ORIGIN,
            "INV-002",
            Stage::FinalValidation,
            path,
            "a canonical node must carry an `origin` member",
        ));
    };

    validate_origin(origin_raw, &format!("{path}.origin"))?;

    for (name, member) in &node.0 {
        match name.as_str() {
            "values" | "origin" => {}
            // INV-006 — documentation is not a member of a node. It belongs to
            // the schema, which describes; the object records what was decided.
            "description" | "documentation" | "title" | "comment" => {
                return Err(Error::new(
                    code::DOCUMENTATION_ON_NODE,
                    "INV-006",
                    Stage::FinalValidation,
                    format!("{path}.{name}"),
                    "a canonical node carries no documentation member",
                ))
            }
            // INV-012 — a canonical node carries no `default`. The default was
            // a schema instruction; once materialized there is a value, and
            // keeping the instruction beside it invites the two to disagree.
            "default" => return Err(Error::new(
                code::DEFAULT_MEMBER_ON_NODE,
                "INV-012",
                Stage::FinalValidation,
                format!("{path}.default"),
                "a canonical node carries no `default` member; the value is already materialized",
            )),
            // INV-003 — a primitive member is itself a CIC node.
            //
            // This used to recurse when the member was a mapping and accept it
            // otherwise, so `shape: 7` passed: the validator said `valid` for an
            // object whose `shape` is a number. Go rejected the same object. An
            // `if let` with no `else` is how a check becomes a check of the
            // cases that happen to reach it.
            other if is_primitive(other) => {
                let Some(pm) = member.as_map() else {
                    return Err(Error::new(
                        code::MALFORMED_DOCUMENT,
                        "INV-003",
                        Stage::FinalValidation,
                        format!("{path}.{other}"),
                        "a primitive member must itself be a CIC node",
                    ));
                };
                walk(pm, &format!("{path}.{other}"))?;
            }
            // INV-021 — `values`, `origin` and the eight primitives. Anything
            // else has no interpretation, including `cic`: 0.2 moved the model
            // version to the hand-off frame (INV-033), so a version inside the
            // object reaches exactly this branch.
            other => {
                return Err(Error::new(
                    code::UNKNOWN_MEMBER,
                    "INV-021",
                    Stage::FinalValidation,
                    format!("{path}.{other}"),
                    format!("`{other}` is not a member a canonical node may carry"),
                ))
            }
        }
    }

    // Descend into the payload. A mapping whose entries are themselves nodes is
    // a structured payload; anything else is a leaf and has nothing below it to
    // check. Distinguishing them without a schema is exactly the weakness
    // INV-039 names.
    if let Some(pm) = values.as_map() {
        for (name, child) in &pm.0 {
            if let Some(cm) = child.as_map() {
                if cm.contains("values") || cm.contains("origin") {
                    walk(cm, &format!("{path}.values.{name}"))?;
                }
            }
        }
    }
    if let Some(items) = values.as_seq() {
        for (i, item) in items.iter().enumerate() {
            if let Some(im) = item.as_map() {
                if im.contains("values") || im.contains("origin") {
                    walk(im, &format!("{path}.values[{i}]"))?;
                }
            }
        }
    }
    Ok(())
}

/// Parse an `origin` member against the four productions of §5.2.
///
/// # Errors
/// Rejects anything the grammar does not produce, including sequences built
/// entirely from legal terms.
pub fn parse_origin(raw: &Value, path: &str) -> Result<Origin> {
    // INV-004 — origin is terminal. If it had been expanded into a node it
    // would need an origin of its own, without end.
    if raw.as_map().is_some() {
        return Err(Error::new(
            code::ORIGIN_NOT_TERMINAL,
            "INV-004",
            Stage::FinalValidation,
            path,
            "origin must be a value in the grammar of SPEC §5.2, not a CIC node",
        ));
    }
    let Some(terms) = raw.as_seq() else {
        return Err(Error::new(
            code::ORIGIN_NOT_TERMINAL,
            "INV-004",
            Stage::FinalValidation,
            path,
            "origin must be a sequence of origin terms",
        ));
    };
    // INV-018 — row 8. Every materialized node has an authority.
    if terms.is_empty() {
        return Err(Error::new(
            code::ORIGIN_EMPTY,
            "INV-018",
            Stage::FinalValidation,
            path,
            "origin must not be empty; the node's authority is unattributable",
        ));
    }

    let mut shape: Vec<Term> = Vec::new();
    for t in terms {
        shape.push(term(t, path)?);
    }

    // The exclusion rules first, so a reader gets the invariant they violated
    // rather than "not in the grammar". The vectors assert these codes.
    let has_yaml = shape.iter().any(|t| matches!(t, Term::Yaml));
    let has_schema = shape.iter().any(|t| matches!(t, Term::Schema));
    let has_sealed = shape.iter().any(|t| matches!(t, Term::Sealed(_)));

    // INV-017 — row 7. A single effective value is either explicitly supplied
    // or defaulted; it cannot be both.
    if has_yaml && has_schema {
        return Err(Error::new(
            code::ORIGIN_YAML_SCHEMA_CONFLICT,
            "INV-017",
            Stage::FinalValidation,
            path,
            "origin holds both `yaml` and `schema`; these are mutually exclusive value sources",
        ));
    }
    // INV-016 — rows 5 and 6. `sealed` means authoring is closed at and below.
    if has_sealed && has_yaml {
        return Err(Error::new(
            code::ORIGIN_SEALED_YAML_CONFLICT,
            "INV-016",
            Stage::FinalValidation,
            path,
            "origin holds both `sealed` and `yaml`; a yaml-sourced value below a sealed boundary is structurally illegal",
        ));
    }

    // INV-013 — and now the sequence must BE one of the four productions, not
    // merely be built from legal terms that break no exclusion rule. Presence
    // questions cannot express arity or order, which is how `[yaml, yaml]` and
    // the fourth form reversed passed the Go implementation for 26 commits.
    match shape.as_slice() {
        [Term::Yaml] => Ok(Origin::Yaml),
        [Term::Schema] => Ok(Origin::Schema),
        [Term::Sealed(s)] => Ok(Origin::Sealed(s.clone())),
        [Term::Sealed(s), Term::Schema] => Ok(Origin::SealedSchema(s.clone())),
        _ => Err(Error::new(
            code::ORIGIN_GRAMMAR,
            "INV-013",
            Stage::FinalValidation,
            path,
            "origin is not one of the four forms of SPEC §5.2: \
             [yaml] | [schema] | [sealed(t,p)] | [sealed(t,p), schema]",
        )),
    }
}

fn validate_origin(raw: &Value, path: &str) -> Result<()> {
    parse_origin(raw, path).map(|_| ())
}

enum Term {
    Yaml,
    Schema,
    Sealed(Sealed),
}

fn term(t: &Value, path: &str) -> Result<Term> {
    match t {
        Value::Str(s) if s == "yaml" => Ok(Term::Yaml),
        Value::Str(s) if s == "schema" => Ok(Term::Schema),
        // INV-020 — origin `sealed` is always a two-arity constructor. The bare
        // token belongs to the aggregate slot-mode vocabulary, which is a
        // different `sealed` entirely (SPEC §5.5).
        Value::Str(s) if s == "sealed" => Err(Error::new(
            code::ORIGIN_GRAMMAR,
            "INV-020",
            Stage::FinalValidation,
            path,
            "the bare token `sealed` is not an origin term; origin sealed always carries template and path",
        )),
        Value::Str(s) => Err(Error::new(
            code::ORIGIN_GRAMMAR,
            "INV-013",
            Stage::FinalValidation,
            path,
            format!("`{s}` is not an origin term"),
        )),
        Value::Map(m) => {
            let Some(sv) = m.get("sealed") else {
                return Err(Error::new(
                    code::ORIGIN_GRAMMAR,
                    "INV-013",
                    Stage::FinalValidation,
                    path,
                    "the only constructor term in the origin grammar is `sealed`",
                ));
            };
            let sm = sv.as_map().ok_or_else(|| {
                Error::new(
                    code::SEALED_MISSING_TEMPLATE_OR_PATH,
                    "INV-015",
                    Stage::FinalValidation,
                    path,
                    "a sealed term must carry both template and path",
                )
            })?;
            let present = |k: &str| match sm.get(k) {
                None | Some(Value::Null) => None,
                Some(v) => Some(v.to_plain_string()),
            };
            let (Some(template), Some(p)) = (present("template"), present("path")) else {
                return Err(Error::new(
                    code::SEALED_MISSING_TEMPLATE_OR_PATH,
                    "INV-015",
                    Stage::FinalValidation,
                    path,
                    "a sealed term must carry both template and path",
                ));
            };
            // And nothing else, and both scalars.
            //
            // INV-013 says exactly four productions. Checking that the two
            // members are PRESENT leaves the term open: a constructor with a
            // third member, or a `template` that is a sequence, passed both
            // implementations while being no production the grammar contains.
            // The same presence-versus-shape mistake the sequence match fixed
            // one level up, one level down.
            if sm.len() != 2 {
                return Err(Error::new(
                    code::ORIGIN_GRAMMAR,
                    "INV-013",
                    Stage::FinalValidation,
                    path,
                    format!(
                        "a sealed term carries template and path and nothing else; found {:?}",
                        sm.keys()
                    ),
                ));
            }
            for name in ["template", "path"] {
                if matches!(sm.get(name), Some(Value::Map(_) | Value::Seq(_))) {
                    return Err(Error::new(
                        code::ORIGIN_GRAMMAR,
                        "INV-013",
                        Stage::FinalValidation,
                        path,
                        format!("a sealed term's {name} must be a scalar"),
                    ));
                }
            }
            Ok(Term::Sealed(Sealed { template, path: p }))
        }
        _ => Err(Error::new(
            code::ORIGIN_GRAMMAR,
            "INV-013",
            Stage::FinalValidation,
            path,
            "an origin term must be `yaml`, `schema`, or a sealed constructor",
        )),
    }
}
