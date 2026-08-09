//! The CLI as a contract, driven as a subprocess.
//!
//! Running the binary is the only way to assert an exit code, and the exit code
//! is half of what a caller acts on. The golden data is the corpus itself —
//! there are deliberately no separate golden files, because a second set would
//! be a second thing to keep in step and the first time they drifted the CLI
//! would be checked against a stale copy of what the vectors already say.

use cic_object_model::error::Stage;
use cic_object_model::value::{self, Value};
use std::path::{Path, PathBuf};
use std::process::{Command, Output};

const CORPUS: &str = "../conformance";

fn binary() -> PathBuf {
    // CARGO_BIN_EXE_ is set by cargo for every [[bin]] of this package, so the
    // test uses the binary cargo just built rather than one found on PATH.
    PathBuf::from(env!("CARGO_BIN_EXE_cic-materialize"))
}

fn run(args: &[&str]) -> Output {
    Command::new(binary())
        .args(args)
        .output()
        .expect("the CLI runs")
}

fn vectors(group: &str) -> Vec<PathBuf> {
    let mut out: Vec<PathBuf> = std::fs::read_dir(Path::new(CORPUS).join(group))
        .expect("the group is readable")
        .filter_map(|e| {
            let e = e.ok()?;
            e.file_type().ok()?.is_dir().then(|| e.path())
        })
        .collect();
    out.sort();
    assert!(!out.is_empty(), "{group}: no vectors — the path is wrong");
    out
}

fn same_yaml(left: &[u8], right: &[u8]) -> bool {
    let parse = |doc: &[u8]| value::parse(doc, Stage::FinalValidation, "$", "doc").ok();
    match (parse(left), parse(right)) {
        (Some(one), Some(two)) => one == two,
        _ => false,
    }
}

#[test]
fn materialization_vectors_go_to_stdout_with_exit_zero() {
    for dir in vectors("materialization") {
        let name = dir
            .file_name()
            .expect("a name")
            .to_string_lossy()
            .to_string();
        let out = run(&[
            "-schema",
            dir.join("schema.yaml").to_str().expect("a path"),
            "-input",
            dir.join("input.yaml").to_str().expect("a path"),
        ]);
        assert!(out.status.success(), "{name}: exit {:?}", out.status.code());
        assert!(
            out.stderr.is_empty(),
            "{name}: stderr must be empty on success:\n{}",
            String::from_utf8_lossy(&out.stderr)
        );
        let expected = std::fs::read(dir.join("expected.yaml")).expect("expected.yaml");
        assert!(
            same_yaml(&out.stdout, &expected),
            "{name}: stdout is not the canonical object:\n{}",
            String::from_utf8_lossy(&out.stdout)
        );
    }
}

#[test]
fn rejections_go_to_stderr_as_a_parseable_envelope_with_exit_one() {
    let check = |name: &str, dir: &Path, out: &Output| {
        assert_eq!(out.status.code(), Some(1), "{name}: wrong exit code");
        assert!(
            out.stdout.is_empty(),
            "{name}: stdout must be empty when nothing was produced"
        );
        let expected = std::fs::read(dir.join("expected-error.yaml")).expect("expected-error.yaml");
        // The envelope must PARSE, and its four asserted fields must match.
        // `detail` is prose for a human and is not compared.
        let got =
            value::parse(&out.stderr, Stage::FinalValidation, "$", "stderr").unwrap_or_else(|e| {
                panic!(
                    "{name}: stderr is not a parseable envelope: {e}\n{}",
                    String::from_utf8_lossy(&out.stderr)
                )
            });
        let want = value::parse(&expected, Stage::FinalValidation, "$", "expected")
            .expect("expected-error.yaml parses");
        let field = |v: &Value, k: &str| {
            v.as_map()
                .and_then(|m| m.get("error"))
                .and_then(Value::as_map)
                .and_then(|m| m.get(k))
                .map_or_else(|| panic!("{name}: no `{k}`"), Value::to_plain_string)
        };
        for k in ["code", "invariant", "stage", "path"] {
            assert_eq!(field(&got, k), field(&want, k), "{name}: `{k}` differs");
        }
    };

    for dir in vectors("invalid") {
        let name = format!(
            "invalid/{}",
            dir.file_name().expect("a name").to_string_lossy()
        );
        let out = run(&[
            "-schema",
            dir.join("schema.yaml").to_str().expect("a path"),
            "-input",
            dir.join("input.yaml").to_str().expect("a path"),
        ]);
        check(&name, &dir, &out);
    }
    for dir in vectors("validation") {
        let name = format!(
            "validation/{}",
            dir.file_name().expect("a name").to_string_lossy()
        );
        let out = run(&[
            "-validate",
            dir.join("object.yaml").to_str().expect("a path"),
        ]);
        check(&name, &dir, &out);
    }
}

/// The shapes a caller meets when it holds the tool wrong. The corpus does not
/// reach any of these, and each one is a thing a second implementation has to
/// reproduce.
#[test]
fn the_contract_outside_the_corpus() {
    let dir = Path::new(CORPUS).join("materialization/001_origin_yaml");

    // No arguments is a usage error, not a crash.
    let out = run(&[]);
    assert_eq!(out.status.code(), Some(1));
    assert!(
        String::from_utf8_lossy(&out.stderr).contains("required"),
        "stderr does not say what is missing"
    );

    // An unknown flag is refused rather than ignored. A silently ignored flag
    // is how a caller ends up believing it asked for something it did not.
    let out = run(&["-nonsense"]);
    assert_eq!(out.status.code(), Some(1));
    assert!(String::from_utf8_lossy(&out.stderr).contains("unknown argument"));

    // A missing file is reported, not panicked on, and in the shared envelope.
    let out = run(&["-schema", "/nonexistent", "-input", "/nonexistent"]);
    assert_eq!(out.status.code(), Some(1));
    assert!(out.stdout.is_empty());
    assert!(String::from_utf8_lossy(&out.stderr).contains("E_CLI"));

    let out = run(&["-validate", "/nonexistent"]);
    assert_eq!(out.status.code(), Some(1));
    assert!(String::from_utf8_lossy(&out.stderr).contains("E_CLI"));

    // Validating a good object says so on stdout.
    let out = run(&[
        "-validate",
        dir.join("expected.yaml").to_str().expect("a path"),
    ]);
    assert!(
        out.status.success(),
        "{:?}",
        String::from_utf8_lossy(&out.stderr)
    );
    assert_eq!(String::from_utf8_lossy(&out.stdout).trim(), "valid");

    // -deliver keeps stdout clean: the note goes to stderr, so the tool can be
    // piped without the note contaminating the object.
    let out = run(&[
        "-schema",
        dir.join("schema.yaml").to_str().expect("a path"),
        "-input",
        dir.join("input.yaml").to_str().expect("a path"),
        "-deliver",
    ]);
    assert!(out.status.success());
    let expected = std::fs::read(dir.join("expected.yaml")).expect("expected.yaml");
    assert!(
        same_yaml(&out.stdout, &expected),
        "-deliver contaminated stdout"
    );
    assert!(String::from_utf8_lossy(&out.stderr).contains("delivered model"));

    // The long forms of every flag work too, since they are accepted.
    let out = run(&[
        "--schema",
        dir.join("schema.yaml").to_str().expect("a path"),
        "--input",
        dir.join("input.yaml").to_str().expect("a path"),
    ]);
    assert!(out.status.success());
}
