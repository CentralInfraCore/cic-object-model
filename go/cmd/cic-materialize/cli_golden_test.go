package main_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// The CLI is the harness contract: bytes in, bytes out, an exit code. It is
// what a second implementation has to match, and until it is pinned, "the two
// implementations agree" means "two runners, written separately, each believed
// its own reading of the corpus".
//
// The golden data is the corpus itself. There are no separate golden files for
// the CLI, deliberately: a second set would be a second thing to keep in step,
// and the first time they drifted the CLI would be checked against a stale copy
// of what the vectors already say.
//
//	materialization/*  stdout is the canonical object, exit 0
//	invalid/*          stderr is the error envelope, exit 1
//	validation/*       same, via -validate
//
// The stderr envelope parses as the same YAML shape as expected-error.yaml.
// That is the part a Rust implementation must reproduce, and it is asserted
// semantically rather than byte-for-byte: §8.8 defines no canonical
// serialization (docs/spec-defects.md SD-010), so requiring identical bytes
// would be inventing a rule the specification does not have.

const corpus = "../../../conformance"

// buildCLI compiles the binary once. Driving `go run` per vector would be
// measuring the Go toolchain, not the contract.
func buildCLI(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "cic-materialize")
	out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput()
	if err != nil {
		t.Fatalf("building the CLI failed: %v\n%s", err, out)
	}
	return bin
}

type result struct {
	stdout, stderr string
	code           int
}

func runCLI(t *testing.T, bin string, args ...string) result {
	t.Helper()
	cmd := exec.Command(bin, args...)
	var so, se bytes.Buffer
	cmd.Stdout, cmd.Stderr = &so, &se
	err := cmd.Run()
	code := 0
	var ee *exec.ExitError
	if err != nil {
		if !asExitError(err, &ee) {
			t.Fatalf("running the CLI failed: %v", err)
		}
		code = ee.ExitCode()
	}
	return result{stdout: so.String(), stderr: se.String(), code: code}
}

func asExitError(err error, target **exec.ExitError) bool {
	e, ok := err.(*exec.ExitError)
	if ok {
		*target = e
	}
	return ok
}

func vectorDirs(t *testing.T, group string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(corpus, group))
	if err != nil {
		t.Fatalf("reading %s: %v", group, err)
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	if len(out) == 0 {
		t.Fatalf("%s: no vectors discovered — the corpus path is wrong", group)
	}
	return out
}

func sameYAML(t *testing.T, a, b []byte) bool {
	t.Helper()
	var x, y any
	if err := yaml.Unmarshal(a, &x); err != nil {
		return false
	}
	if err := yaml.Unmarshal(b, &y); err != nil {
		return false
	}
	return reflect.DeepEqual(x, y)
}

// TestCLIMaterialization — the success half of the contract.
func TestCLIMaterialization(t *testing.T) {
	bin := buildCLI(t)
	for _, name := range vectorDirs(t, "materialization") {
		dir := filepath.Join(corpus, "materialization", name)
		t.Run(name, func(t *testing.T) {
			got := runCLI(t, bin,
				"-schema", filepath.Join(dir, "schema.yaml"),
				"-input", filepath.Join(dir, "input.yaml"))

			if got.code != 0 {
				t.Fatalf("exit %d, want 0\nstderr:\n%s", got.code, got.stderr)
			}
			if got.stderr != "" {
				t.Errorf("stderr must be empty on success, got:\n%s", got.stderr)
			}
			expected, err := os.ReadFile(filepath.Join(dir, "expected.yaml"))
			if err != nil {
				t.Fatalf("reading expected.yaml: %v", err)
			}
			if !sameYAML(t, []byte(got.stdout), expected) {
				t.Errorf("stdout is not the canonical object\n--- got ---\n%s\n--- want ---\n%s",
					got.stdout, expected)
			}
		})
	}
}

// TestCLIRejection — the failure half. This is the shape a second
// implementation has to reproduce, and the reason the CLI is worth pinning: an
// error a caller cannot parse is an error a caller cannot act on.
func TestCLIRejection(t *testing.T) {
	bin := buildCLI(t)

	check := func(t *testing.T, dir string, got result) {
		t.Helper()
		if got.code != 1 {
			t.Fatalf("exit %d, want 1\nstdout:\n%s", got.code, got.stdout)
		}
		expected, err := os.ReadFile(filepath.Join(dir, "expected-error.yaml"))
		if err != nil {
			t.Fatalf("reading expected-error.yaml: %v", err)
		}
		// The envelope must parse, and its four asserted fields must match.
		// `detail` is prose for a human and is not compared.
		var gotEnv, wantEnv struct {
			Error struct {
				Code, Invariant, Stage, Path string
			}
		}
		if err := yaml.Unmarshal([]byte(got.stderr), &gotEnv); err != nil {
			t.Fatalf("stderr is not a parseable error envelope: %v\n%s", err, got.stderr)
		}
		if err := yaml.Unmarshal(expected, &wantEnv); err != nil {
			t.Fatalf("expected-error.yaml is not parseable: %v", err)
		}
		if gotEnv.Error != wantEnv.Error {
			t.Errorf("error envelope mismatch\n--- got ---\n%+v\n--- want ---\n%+v",
				gotEnv.Error, wantEnv.Error)
		}
	}

	for _, name := range vectorDirs(t, "invalid") {
		dir := filepath.Join(corpus, "invalid", name)
		t.Run("invalid/"+name, func(t *testing.T) {
			check(t, dir, runCLI(t, bin,
				"-schema", filepath.Join(dir, "schema.yaml"),
				"-input", filepath.Join(dir, "input.yaml")))
		})
	}

	for _, name := range vectorDirs(t, "validation") {
		dir := filepath.Join(corpus, "validation", name)
		t.Run("validation/"+name, func(t *testing.T) {
			check(t, dir, runCLI(t, bin, "-validate", filepath.Join(dir, "object.yaml")))
		})
	}
}

// TestCLIContract pins the parts the corpus does not reach: the shapes a caller
// meets when it holds the tool wrong.
func TestCLIContract(t *testing.T) {
	bin := buildCLI(t)
	dir := filepath.Join(corpus, "materialization", "001_origin_yaml")

	t.Run("no arguments is a usage error, not a crash", func(t *testing.T) {
		got := runCLI(t, bin)
		if got.code != 1 {
			t.Errorf("exit %d, want 1", got.code)
		}
		if !strings.Contains(got.stderr, "required") {
			t.Errorf("stderr does not say what is missing:\n%s", got.stderr)
		}
	})

	t.Run("a missing file is reported, not panicked on", func(t *testing.T) {
		got := runCLI(t, bin, "-schema", "/nonexistent", "-input", "/nonexistent")
		if got.code != 1 {
			t.Errorf("exit %d, want 1", got.code)
		}
		if got.stdout != "" {
			t.Errorf("stdout must be empty when nothing was produced, got:\n%s", got.stdout)
		}
	})

	t.Run("validating a good object says so on stdout", func(t *testing.T) {
		// The canonical object of a passing vector must validate.
		got := runCLI(t, bin, "-validate", filepath.Join(dir, "expected.yaml"))
		if got.code != 0 {
			t.Fatalf("exit %d, want 0\nstderr:\n%s", got.code, got.stderr)
		}
		if strings.TrimSpace(got.stdout) != "valid" {
			t.Errorf("stdout = %q, want \"valid\"", strings.TrimSpace(got.stdout))
		}
	})

	t.Run("delivery keeps stdout clean", func(t *testing.T) {
		// -deliver writes its note to stderr. stdout stays the canonical
		// object, so the tool can be piped without the note contaminating it.
		got := runCLI(t, bin,
			"-schema", filepath.Join(dir, "schema.yaml"),
			"-input", filepath.Join(dir, "input.yaml"),
			"-deliver")
		if got.code != 0 {
			t.Fatalf("exit %d, want 0\nstderr:\n%s", got.code, got.stderr)
		}
		expected, err := os.ReadFile(filepath.Join(dir, "expected.yaml"))
		if err != nil {
			t.Fatalf("reading expected.yaml: %v", err)
		}
		if !sameYAML(t, []byte(got.stdout), expected) {
			t.Errorf("-deliver contaminated stdout:\n%s", got.stdout)
		}
		if !strings.Contains(got.stderr, "delivered model") {
			t.Errorf("the delivery note is missing from stderr:\n%s", got.stderr)
		}
	})
}
