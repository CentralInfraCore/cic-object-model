package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// cli_golden_test.go drives the compiled binary as a subprocess, which is the
// only way to assert an exit code and the only honest way to pin a contract a
// second implementation must match. The cost is that coverage instrumentation
// cannot see inside the child: main, run and emitError all measured 0%, and a
// caller reading the report would conclude they were untested.
//
// This file closes that gap by calling them in-process. It is not a second
// contract test — the subprocess one is the contract. It is here so the
// coverage report tells the truth about what has been exercised.

const corpusRoot = "../../../conformance"

func TestRunMaterializes(t *testing.T) {
	dir := filepath.Join(corpusRoot, "materialization", "001_origin_yaml")

	// run writes the canonical object to stdout. Capture it rather than let it
	// escape into the test log, and assert it is what came out.
	out := captureStdout(t, func() {
		if err := run(filepath.Join(dir, "schema.yaml"), filepath.Join(dir, "input.yaml"), "", false); err != nil {
			t.Fatalf("run failed: %v", err)
		}
	})
	if !strings.Contains(out, "values:") || !strings.Contains(out, "origin:") {
		t.Errorf("stdout is not a canonical object:\n%s", out)
	}
	if strings.Contains(out, "cic:") {
		t.Error("the model version leaked into the object (INV-033)")
	}
}

func TestRunValidates(t *testing.T) {
	dir := filepath.Join(corpusRoot, "materialization", "001_origin_yaml")
	out := captureStdout(t, func() {
		if err := run("", "", filepath.Join(dir, "expected.yaml"), false); err != nil {
			t.Fatalf("validating a canonical object failed: %v", err)
		}
	})
	if strings.TrimSpace(out) != "valid" {
		t.Errorf("stdout = %q, want \"valid\"", strings.TrimSpace(out))
	}
}

func TestRunDelivers(t *testing.T) {
	dir := filepath.Join(corpusRoot, "materialization", "001_origin_yaml")
	_ = captureStdout(t, func() {
		if err := run(filepath.Join(dir, "schema.yaml"), filepath.Join(dir, "input.yaml"), "", true); err != nil {
			t.Fatalf("delivery failed: %v", err)
		}
	})
}

func TestRunRejections(t *testing.T) {
	dir := filepath.Join(corpusRoot, "invalid", "006_unknown_primitive")

	for name, tc := range map[string]struct{ schema, input, validate string }{
		"missing both arguments": {"", "", ""},
		"missing schema file":    {"/nonexistent", "/nonexistent", ""},
		"missing object file":    {"", "", "/nonexistent"},
		"a rejected input": {
			filepath.Join(dir, "schema.yaml"), filepath.Join(dir, "input.yaml"), "",
		},
	} {
		t.Run(name, func(t *testing.T) {
			_ = captureStdout(t, func() {
				if err := run(tc.schema, tc.input, tc.validate, false); err == nil {
					t.Error("run returned no error where one was expected")
				}
			})
		})
	}
}

// TestEmitErrorEnvelope — emitError is the machine-readable half of the CLI
// contract. Both branches: a pipeline rejection, and everything else.
func TestEmitErrorEnvelope(t *testing.T) {
	dir := filepath.Join(corpusRoot, "invalid", "006_unknown_primitive")

	t.Run("a pipeline rejection", func(t *testing.T) {
		err := run(filepath.Join(dir, "schema.yaml"), filepath.Join(dir, "input.yaml"), "", false)
		if err == nil {
			t.Fatal("expected a rejection")
		}
		out := captureStderr(t, func() { emitError(err) })
		for _, want := range []string{"code:", "invariant:", "stage:", "path:"} {
			if !strings.Contains(out, want) {
				t.Errorf("envelope is missing %q:\n%s", want, out)
			}
		}
		// The detail of this particular rejection begins with a backtick, which
		// is what broke the format-string version: YAML cannot start a plain
		// scalar that way. The marshaller quotes it.
		if !strings.Contains(out, "'") && !strings.Contains(out, "\"") {
			t.Errorf("a detail beginning with a backtick was not quoted:\n%s", out)
		}
	})

	t.Run("anything that is not a pipeline rejection", func(t *testing.T) {
		err := run("/nonexistent", "/nonexistent", "", false)
		if err == nil {
			t.Fatal("expected an error")
		}
		out := captureStderr(t, func() { emitError(err) })
		if !strings.Contains(out, "E_CLI") {
			t.Errorf("a non-pipeline error did not arrive in the shared envelope:\n%s", out)
		}
	})
}

func captureStdout(t *testing.T, f func()) string { return capture(t, &os.Stdout, f) }
func captureStderr(t *testing.T, f func()) string { return capture(t, &os.Stderr, f) }

func capture(t *testing.T, target **os.File, f func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	orig := *target
	*target = w
	done := make(chan string)
	go func() {
		var b strings.Builder
		buf := make([]byte, 4096)
		for {
			n, err := r.Read(buf)
			b.Write(buf[:n])
			if err != nil {
				break
			}
		}
		done <- b.String()
	}()
	f()
	w.Close()
	*target = orig
	return <-done
}
