package objectmodel_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	om "github.com/CentralInfraCore/cic-object-model/go/objectmodel"
)

// Fuzzing here does not ask "is the output right" — the corpus answers that.
// It asks whether the library can be broken: made to panic, to hang, or to
// hand back something that is neither a canonical object nor a proper error.
//
// The sharpest property is the last one in FuzzMaterialize below: whatever
// comes OUT of the materializer must be accepted by the validator. Those two
// were written independently against the same specification, and until now
// nothing had ever checked that they agree.

// seedFromCorpus feeds the fuzzer every real vector first, so it starts from
// inputs that mean something rather than from noise.
func seedFromCorpus(f *testing.F) {
	f.Helper()
	dirs, _ := filepath.Glob(filepath.Join(vectorRoot, "*", "*"))
	for _, d := range dirs {
		schema, err1 := os.ReadFile(filepath.Join(d, "schema.yaml"))
		input, err2 := os.ReadFile(filepath.Join(d, "input.yaml"))
		if err1 == nil && err2 == nil {
			f.Add(string(schema), string(input))
		}
	}
	// A few shapes the corpus has no reason to contain.
	f.Add("", "")
	f.Add("root: {shape: object}", "{}")
	f.Add("root:\n  shape: object\n  children:\n    a: {shape: scalar}", "a: 1")
}

// FuzzMaterialize asserts the four properties that make the library safe to
// call with input you did not write.
func FuzzMaterialize(f *testing.F) {
	seedFromCorpus(f)

	f.Fuzz(func(t *testing.T, schemaYAML, inputYAML string) {
		// 1. It must not panic. A panic crossing this boundary would take the
		//    host down on input a module author does not control.
		obj, err := om.Materialize([]byte(schemaYAML), []byte(inputYAML))

		// 2. Exactly one of the two results. Never both, never neither.
		if (obj == nil) == (err == nil) {
			t.Fatalf("Materialize returned obj=%v err=%v — exactly one must be set", obj != nil, err)
		}

		if err != nil {
			// 3. A rejection is a *Error and names all four fields the vectors
			//    assert on. An error with an empty Stage or Path is a rejection
			//    nobody can act on.
			var e *om.Error
			if !asError(err, &e) {
				t.Fatalf("error is %T, want *objectmodel.Error: %v", err, err)
			}
			if e.Code == "" || e.Invariant == "" || e.Stage == "" || e.Path == "" {
				t.Fatalf("incomplete error: code=%q invariant=%q stage=%q path=%q",
					e.Code, e.Invariant, e.Stage, e.Path)
			}
			if !strings.HasPrefix(e.Path, "$") {
				t.Fatalf("path %q does not start at the root", e.Path)
			}
			return
		}

		// 4. The materializer's own output must satisfy the validator. These
		//    two were written independently from the same specification; if
		//    they disagree, one of them is wrong and the corpus never asked.
		if verr := om.ValidateCanonicalDocument(obj.CanonicalYAML()); verr != nil {
			t.Fatalf("materializer produced an object its own validator rejects: %v\n---\n%s",
				verr, obj.CanonicalYAML())
		}

		// 5. Determinism (INV-030): the same input twice, byte for byte.
		again, err2 := om.Materialize([]byte(schemaYAML), []byte(inputYAML))
		if err2 != nil {
			t.Fatalf("second materialization of the same input failed: %v", err2)
		}
		if string(again.CanonicalYAML()) != string(obj.CanonicalYAML()) {
			t.Fatal("materialization is not deterministic for this input")
		}
	})
}

// FuzzValidateCanonicalDocument covers the other entry point: an object handed
// over without its schema, which is what INV-039 says is the weaker walk.
func FuzzValidateCanonicalDocument(f *testing.F) {
	dirs, _ := filepath.Glob(filepath.Join(vectorRoot, "validation", "*"))
	for _, d := range dirs {
		if b, err := os.ReadFile(filepath.Join(d, "object.yaml")); err == nil {
			f.Add(string(b))
		}
	}
	f.Add("")
	f.Add("values: 1\norigin: [yaml]")

	f.Fuzz(func(t *testing.T, objectYAML string) {
		err := om.ValidateCanonicalDocument([]byte(objectYAML))
		if err == nil {
			return
		}
		var e *om.Error
		if !asError(err, &e) {
			t.Fatalf("error is %T, want *objectmodel.Error: %v", err, err)
		}
		if e.Code == "" || e.Stage == "" {
			t.Fatalf("incomplete error: code=%q stage=%q", e.Code, e.Stage)
		}
	})
}

// asError is errors.As without importing errors into every assertion.
func asError(err error, target **om.Error) bool {
	e, ok := err.(*om.Error)
	if ok {
		*target = e
	}
	return ok
}
