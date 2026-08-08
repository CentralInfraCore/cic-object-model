//! `origin` — authoring authority (SPEC §5).
//!
//! The grammar has exactly four productions and INV-013 says no other form is
//! valid:
//!
//! ```text
//!   [ yaml ]
//! | [ schema ]
//! | [ sealed(template, path) ]
//! | [ sealed(template, path), schema ]
//! ```
//!
//! It is represented here as a closed enum rather than a vector of terms,
//! because a vector invites exactly the mistake the Go implementation made: it
//! validated the terms individually, checked the truth-table exclusions, and
//! never checked the SEQUENCE — so `[yaml, yaml]`, `[schema, schema]` and the
//! fourth production reversed all passed. A type that cannot represent those
//! states does not need a check that rejects them.
//!
//! Rows 5–8 of the §5.3 truth table are the invalid combinations, and they are
//! simply absent from this enum.

/// A sealed term: the closed template and the path within it (INV-015 — both,
/// always; template identity alone is not provenance).
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Sealed {
    pub template: String,
    pub path: String,
}

/// The authoring authority of a node. Four variants, one per production.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum Origin {
    /// Row 1 — the instance YAML explicitly supplied this value.
    Yaml,
    /// Row 2 — the value materialized from this node's schema default.
    Schema,
    /// Row 3 — the node and its value came from a closed template.
    Sealed(Sealed),
    /// Row 4 — the template defined and closed the node; the value came from
    /// the template node's own schema default. These combine where `yaml` and
    /// `schema` do not, because `sealed` constrains structural authority while
    /// `schema` describes the value source: different dimensions (INV-017).
    SealedSchema(Sealed),
}

impl Origin {
    /// True when this origin carries a `sealed` term, i.e. authoring is closed
    /// at and below the node (INV-019).
    #[must_use]
    pub fn is_sealed(&self) -> bool {
        matches!(self, Origin::Sealed(_) | Origin::SealedSchema(_))
    }

    /// The sealed term, when there is one.
    #[must_use]
    pub fn sealed(&self) -> Option<&Sealed> {
        match self {
            Origin::Sealed(s) | Origin::SealedSchema(s) => Some(s),
            _ => None,
        }
    }
}
