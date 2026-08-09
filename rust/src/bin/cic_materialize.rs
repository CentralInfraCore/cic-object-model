//! `cic-materialize` — the CLI contract.
//!
//! Bytes in, bytes out, an exit code. This is the surface a second
//! implementation has to match, and it is pinned by the same corpus the library
//! is: the Go binary of the same name answers identically, and neither was
//! written from the other.
//!
//!   -schema S -input I   stdout is the canonical object, exit 0
//!   -validate O          stdout is `valid`, exit 0
//!   -deliver             the delivery note goes to STDERR, so stdout stays
//!                        pipeable
//!   any rejection        stderr is the error envelope, exit 1
//!
//! Argument parsing is by hand rather than by a crate. Four flags do not
//! justify a dependency in a library whose whole point is that a second
//! implementation can reproduce it.

use cic_object_model::{materialize, validate_canonical_document, Error, MODEL_VERSION};
use std::io::Write;
use std::process::ExitCode;

fn main() -> ExitCode {
    let args: Vec<String> = std::env::args().skip(1).collect();
    let mut schema = None;
    let mut input = None;
    let mut validate = None;
    let mut deliver = false;

    let mut i = 0;
    while i < args.len() {
        let take = |i: &mut usize| -> Option<String> {
            *i += 1;
            args.get(*i).cloned()
        };
        match args[i].as_str() {
            "-schema" | "--schema" => schema = take(&mut i),
            "-input" | "--input" => input = take(&mut i),
            "-validate" | "--validate" => validate = take(&mut i),
            "-deliver" | "--deliver" => deliver = true,
            other => {
                usage(&format!("unknown argument `{other}`"));
                return ExitCode::FAILURE;
            }
        }
        i += 1;
    }

    match run(
        schema.as_deref(),
        input.as_deref(),
        validate.as_deref(),
        deliver,
    ) {
        Ok(()) => ExitCode::SUCCESS,
        Err(Failure::Usage(msg)) => {
            usage(&msg);
            ExitCode::FAILURE
        }
        Err(Failure::Io(msg)) => {
            emit_envelope("E_CLI", "-", "-", "-", &msg);
            ExitCode::FAILURE
        }
        Err(Failure::Rejected(e)) => {
            emit_envelope(e.code, e.invariant, e.stage.as_str(), &e.path, &e.detail);
            ExitCode::FAILURE
        }
    }
}

enum Failure {
    Usage(String),
    Io(String),
    Rejected(Box<Error>),
}

impl From<Error> for Failure {
    fn from(e: Error) -> Self {
        Failure::Rejected(Box::new(e))
    }
}

fn read(path: &str) -> Result<Vec<u8>, Failure> {
    std::fs::read(path).map_err(|e| Failure::Io(format!("cannot read {path}: {e}")))
}

fn run(
    schema: Option<&str>,
    input: Option<&str>,
    validate: Option<&str>,
    deliver: bool,
) -> Result<(), Failure> {
    if let Some(object) = validate {
        validate_canonical_document(&read(object)?)?;
        println!("valid");
        return Ok(());
    }

    let (Some(s), Some(i)) = (schema, input) else {
        return Err(Failure::Usage(
            "-schema and -input are required, or -validate".into(),
        ));
    };

    let obj = materialize(&read(s)?, &read(i)?)?;
    // stdout is the object and nothing else, so the tool composes in a pipe.
    std::io::stdout()
        .write_all(obj.canonical_yaml())
        .map_err(|e| Failure::Io(format!("cannot write the object: {e}")))?;

    if deliver {
        // The hand-off frame of INV-033: the version travels with the object,
        // never inside it. It goes to stderr because stdout is the object.
        eprintln!("delivered model {}", obj.model_version());
    }
    Ok(())
}

fn usage(msg: &str) {
    eprintln!("cic-materialize (CIC object model {MODEL_VERSION})");
    eprintln!("  cic-materialize -schema S -input I [-deliver]");
    eprintln!("  cic-materialize -validate OBJECT");
    eprintln!();
    eprintln!("error: {msg}");
}

/// The machine-readable half of the contract: the same YAML shape the vectors'
/// `expected-error.yaml` carries.
///
/// Every field is quoted. The detail of at least one real rejection begins with
/// a backtick, which a plain YAML scalar may not — the Go implementation shipped
/// a format-string version of this that produced unparseable YAML for exactly
/// that case.
fn emit_envelope(code: &str, invariant: &str, stage: &str, path: &str, detail: &str) {
    let q = |s: &str| format!("'{}'", s.replace('\'', "''"));
    eprintln!("---");
    eprintln!("error:");
    eprintln!("  code: {}", q(code));
    eprintln!("  invariant: {}", q(invariant));
    eprintln!("  stage: {}", q(stage));
    eprintln!("  path: {}", q(path));
    eprintln!("  detail: {}", q(detail));
}
