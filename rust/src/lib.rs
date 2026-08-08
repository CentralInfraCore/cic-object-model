//! The CIC object model, model version 0.2.
//!
//! The normative document is `../../SPEC.md`. Where this crate and that
//! document disagree, the document is right and this crate has a bug. The
//! conformance corpus in `../../conformance` is what decides which of the two
//! is happening.
//!
//! # Not a translation of the Go implementation
//!
//! This crate was written from `SPEC.md` and the corpus, deliberately not from
//! `go/`. Two implementations that share an author's reading share that
//! reading's mistakes, and then their agreement proves nothing — which would
//! cost the repository the only reason it holds two. Where the two agree, they
//! agree because the vectors made them.

pub mod error;
pub mod node;
pub mod origin;
pub mod schema;
pub mod value;

pub use error::{Error, Result, Stage};
pub use node::{Node, Payload};
pub use origin::{Origin, Sealed};

/// The model version this crate implements.
///
/// Held to `SPEC.md` by the version identity test, as the Go constant is. The
/// 0.2 revision was once written into SPEC.md and propagated nowhere, so
/// nothing here is trusted to remember it.
pub const MODEL_VERSION: &str = "0.2";
