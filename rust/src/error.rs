//! Rejections.
//!
//! Every rejection carries the same four fields the conformance vectors assert
//! on — code, invariant, stage, path — plus a human-readable detail that is
//! deliberately NOT compared by the corpus. The stage matters as much as the
//! code: the pipeline stages of SPEC §8 run in a fixed order, and a later stage
//! cannot observe input an earlier one would have rejected, so a rejection
//! raised at the wrong stage is a defect even when the code is right.

use std::fmt;

/// The pipeline stage a rejection was raised at.
///
/// These strings are the corpus's, not this crate's invention. `schema-load`
/// is not among the stages SPEC §8 lists — see `docs/spec-defects.md` SD-002 —
/// but the vectors place three checks there, and the vectors are what a second
/// implementation has to match.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Stage {
    SchemaLoad,
    EntryValidation,
    PrimitiveEvaluation,
    FinalValidation,
}

impl Stage {
    #[must_use]
    pub fn as_str(self) -> &'static str {
        match self {
            Stage::SchemaLoad => "schema-load",
            Stage::EntryValidation => "entry-validation",
            Stage::PrimitiveEvaluation => "primitive-evaluation",
            Stage::FinalValidation => "final-validation",
        }
    }
}

impl fmt::Display for Stage {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.write_str(self.as_str())
    }
}

/// Error codes. Every one of these appears in an `expected-error.yaml`, or is
/// reachable from a caller holding the tool wrong.
pub mod code {
    pub const ORIGIN_DECLARED_IN_INPUT: &str = "E_ORIGIN_DECLARED_IN_INPUT";
    pub const AUTHORING_BELOW_SEALED: &str = "E_AUTHORING_BELOW_SEALED";
    pub const UNDECLARED_OBJECT: &str = "E_UNDECLARED_OBJECT";
    pub const SCHEMA_RESERVED_CHILD_VALUES: &str = "E_SCHEMA_RESERVED_CHILD_VALUES";
    pub const SEALED_MISSING_TEMPLATE_OR_PATH: &str = "E_SEALED_MISSING_TEMPLATE_OR_PATH";
    pub const UNKNOWN_PRIMITIVE: &str = "E_UNKNOWN_PRIMITIVE";
    pub const ORIGIN_YAML_SCHEMA_CONFLICT: &str = "E_ORIGIN_YAML_SCHEMA_CONFLICT";
    pub const ORIGIN_EMPTY: &str = "E_ORIGIN_EMPTY";
    pub const ORIGIN_NOT_TERMINAL: &str = "E_ORIGIN_NOT_TERMINAL";
    pub const DOCUMENTATION_ON_NODE: &str = "E_DOCUMENTATION_ON_NODE";
    pub const DEFAULT_MEMBER_ON_NODE: &str = "E_DEFAULT_MEMBER_ON_NODE";
    pub const UNKNOWN_MEMBER: &str = "E_UNKNOWN_MEMBER";
    pub const ORIGIN_GRAMMAR: &str = "E_ORIGIN_GRAMMAR";
    pub const ORIGIN_SEALED_YAML_CONFLICT: &str = "E_ORIGIN_SEALED_YAML_CONFLICT";
    pub const MALFORMED_DOCUMENT: &str = "E_MALFORMED_DOCUMENT";
    pub const UNKNOWN_SCHEMA_KEY: &str = "E_UNKNOWN_SCHEMA_KEY";
    pub const SCHEMA_SHAPE_MISSING: &str = "E_SCHEMA_SHAPE_MISSING";
    pub const TEMPLATE_NOT_FOUND: &str = "E_TEMPLATE_NOT_FOUND";
    pub const TYPE_MISMATCH: &str = "E_TYPE_MISMATCH";
    pub const MISSING_VALUES: &str = "E_MISSING_VALUES";
    pub const MISSING_ORIGIN: &str = "E_MISSING_ORIGIN";
    pub const UNSUPPORTED_MODEL_VERSION: &str = "E_UNSUPPORTED_MODEL_VERSION";
}

/// The single error type this crate raises.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Error {
    pub code: &'static str,
    pub invariant: &'static str,
    pub stage: Stage,
    pub path: String,
    pub detail: String,
}

impl Error {
    pub(crate) fn new(
        code: &'static str,
        invariant: &'static str,
        stage: Stage,
        path: impl Into<String>,
        detail: impl Into<String>,
    ) -> Self {
        Self {
            code,
            invariant,
            stage,
            path: path.into(),
            detail: detail.into(),
        }
    }
}

impl fmt::Display for Error {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(
            f,
            "{} ({}) at {} [stage {}]: {}",
            self.code, self.invariant, self.path, self.stage, self.detail
        )
    }
}

impl std::error::Error for Error {}

pub type Result<T> = std::result::Result<T, Error>;
