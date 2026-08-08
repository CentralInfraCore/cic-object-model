package objectmodel_test

import (
	"strings"
	"testing"

	om "github.com/CentralInfraCore/cic-object-model/go/objectmodel"
)

// Every exported symbol, exercised on purpose.
//
// api_test.go covers the reader API a caller actually navigates with. This file
// covers the rest of the surface — the accessors and entry points that the
// corpus drives end to end but never CALLS, which is a different thing.
//
// The distinction has already cost something twice: a broken Get passed 26/26
// vectors because the corpus compares serialized output and never touches an
// accessor, and Scalar() returned nothing for every leaf inside a primitive
// payload while those leaves were perfectly present in the YAML. Both were
// invisible to a suite that only checks bytes.
//
// So the standard here is not a coverage number. It is: every exported symbol
// is called, on the values it is meant for AND on the values it is not, because
// what an accessor does when asked the wrong question is part of its contract.

func mustObj(t *testing.T, vector string) om.CanonicalObject {
	t.Helper()
	return materializeVector(t, vector)
}

// TestErrorSurface — *Error is what every rejection arrives as, so its fields
// and its message are API, not diagnostics.
func TestErrorSurface(t *testing.T) {
	_, err := om.Materialize(
		[]byte("model: \"0.2\"\nroot:\n  shape: object\n  children:\n    mtu:\n      shape: scalar\n      scalar_type: integer\n"),
		[]byte("mtu:\n  values: 9000\n  priority: high\n"))
	if err == nil {
		t.Fatal("expected a rejection")
	}

	e, ok := err.(*om.Error)
	if !ok {
		t.Fatalf("rejection is %T, want *objectmodel.Error", err)
	}

	// The four fields the vectors assert on must all be populated. An error
	// missing a stage or a path is one a caller cannot act on.
	if e.Code == "" || e.Invariant == "" || e.Stage == "" || e.Path == "" {
		t.Fatalf("incomplete: code=%q invariant=%q stage=%q path=%q",
			e.Code, e.Invariant, e.Stage, e.Path)
	}

	// Error() is the one method, and it had no test at all. A message that
	// omits the code or the path sends a reader to the wrong place.
	msg := e.Error()
	for _, want := range []string{e.Code, e.Path} {
		if !strings.Contains(msg, want) {
			t.Errorf("Error() = %q, missing %q", msg, want)
		}
	}
}

// TestCanonicalObjectSurface — the three methods a module has, one of which had
// never been called by a test.
func TestCanonicalObjectSurface(t *testing.T) {
	obj := mustObj(t, "materialization/001_origin_yaml")

	// ModelVersion is the hand-off frame of INV-033: the version travels on the
	// value, never inside the object. Both halves are asserted here, because
	// the second is the one 0.2 changed.
	if v := obj.ModelVersion(); v == "" {
		t.Error("ModelVersion is empty; the hand-off carries no version")
	}
	if strings.Contains(string(obj.CanonicalYAML()), "cic:") {
		t.Error("the version leaked into the object (INV-033)")
	}

	if obj.Root() == nil {
		t.Fatal("Root is nil")
	}
	if len(obj.CanonicalYAML()) == 0 {
		t.Error("CanonicalYAML is empty")
	}
}

// TestNodeAccessorsOnTheRightValues — each accessor on the kind it is for.
func TestNodeAccessorsOnTheRightValues(t *testing.T) {
	root := mustObj(t, "materialization/013_access_inherit_injection").Root()

	// Origin: uncovered until now, and it is how a caller reads provenance —
	// the thing the whole origin grammar of §5 exists to express.
	if len(root.Origin()) == 0 {
		t.Error("the root has no origin (INV-002)")
	}
	mtu, ok := root.Get("$.values.mtu")
	if !ok {
		t.Fatal("$.values.mtu did not resolve")
	}
	org := mtu.Origin()
	if len(org) != 1 || org[0].Kind != om.OriginYAML {
		t.Errorf("mtu origin = %v, want [yaml] — the value was authored", org)
	}

	if p := mtu.Path(); p == "" {
		t.Error("Path is empty")
	}
	if v, ok := mtu.Scalar(); !ok || v != 9000 {
		t.Errorf("Scalar = %v (%v), want 9000", v, ok)
	}
	if kids := root.Children(); len(kids) == 0 {
		t.Error("the root reports no children")
	}
	if prims := mtu.Primitives(); len(prims) == 0 {
		t.Error("mtu reports no primitives")
	}
	if _, ok := mtu.Primitive("shape"); !ok {
		t.Error("mtu has no shape primitive (INV-022)")
	}
	if _, ok := root.Child("mtu"); !ok {
		t.Error("Child(mtu) missed on the root's payload")
	}
}

// TestNodeAccessorsOnTheWrongValues — what an accessor does when asked the
// wrong question is part of its contract, and it is where a caller's bug
// becomes a panic if the answer is careless.
func TestNodeAccessorsOnTheWrongValues(t *testing.T) {
	root := mustObj(t, "materialization/001_origin_yaml").Root()
	mtu, _ := root.Get("$.values.mtu")

	// The root's payload is a map, not a scalar or a list.
	if _, ok := root.Scalar(); ok {
		t.Error("Scalar succeeded on a map payload")
	}
	if n := root.Len(); n != 0 {
		t.Errorf("Len = %d on a map payload, want 0", n)
	}
	if _, ok := root.At(0); ok {
		t.Error("At succeeded on a map payload")
	}

	// mtu's payload is a scalar: no children, no entries.
	if kids := mtu.Children(); kids != nil {
		t.Errorf("Children = %v on a scalar payload, want nil", kids)
	}
	if _, ok := mtu.Child("anything"); ok {
		t.Error("Child succeeded on a scalar payload")
	}
	if _, ok := mtu.Primitive("nonexistent"); ok {
		t.Error("Primitive succeeded for a name that is not declared")
	}
	if _, ok := root.Get(""); !ok {
		t.Error("the empty address should resolve to the node itself")
	}
}

// TestListAccessorsAtTheEdges — Len and At around the boundaries, including the
// negative index a caller reaches by arithmetic rather than intent.
func TestListAccessorsAtTheEdges(t *testing.T) {
	root := mustObj(t, "materialization/008_normalize_list").Root()
	list, ok := root.Get("$.values.addresses")
	if !ok {
		t.Fatal("addresses did not resolve")
	}
	n := list.Len()
	if n == 0 {
		t.Fatal("the list reports no entries")
	}
	if _, ok := list.At(-1); ok {
		t.Error("At(-1) resolved")
	}
	if _, ok := list.At(n); ok {
		t.Errorf("At(%d) resolved on a %d-entry list", n, n)
	}
	for i := 0; i < n; i++ {
		if _, ok := list.At(i); !ok {
			t.Errorf("At(%d) missed inside the list", i)
		}
	}
}

// TestLoadSchemaSurface — the entry point whose stated limit is that the useful
// half of its result is the error.
func TestLoadSchemaSurface(t *testing.T) {
	good := []byte("model: \"0.2\"\nroot:\n  shape: object\n  children:\n    a:\n      shape: scalar\n      scalar_type: string\n")
	if s, err := om.LoadSchema(good); err != nil || s == nil {
		t.Fatalf("a valid schema was rejected: %v", err)
	}

	for name, bad := range map[string][]byte{
		"not YAML":            []byte("\tthis: is: not: yaml\n"),
		"not a mapping":       []byte("- a\n- b\n"),
		"no root":             []byte("model: \"0.2\"\n"),
		"values as a child":   []byte("model: \"0.2\"\nroot:\n  shape: object\n  children:\n    values:\n      shape: scalar\n"),
		"unknown schema key":  []byte("model: \"0.2\"\nroot:\n  shape: object\n  surprise: 1\n"),
		"sealed without path": []byte("model: \"0.2\"\nroot:\n  shape: object\n  children:\n    a:\n      sealed_from:\n        template: $t\n"),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := om.LoadSchema(bad); err == nil {
				t.Error("accepted a schema it should reject")
			}
		})
	}
}

// TestValidateCanonicalDocumentSurface — the schema-less walk of INV-039, which
// the corpus drives only through rejections.
func TestValidateCanonicalDocumentSurface(t *testing.T) {
	// A canonical object produced by the materializer must validate. This is
	// the same agreement the fuzzer checks on random input; here it is pinned
	// on a known one so a failure is readable rather than a fuzz artefact.
	obj := mustObj(t, "materialization/001_origin_yaml")
	if err := om.ValidateCanonicalDocument(obj.CanonicalYAML()); err != nil {
		t.Errorf("the materializer's own output was rejected: %v", err)
	}

	for name, bad := range map[string][]byte{
		"not YAML":       []byte("\tnope\n"),
		"not a mapping":  []byte("- a\n"),
		"missing values": []byte("origin: [yaml]\n"),
		"missing origin": []byte("values: {}\n"),
	} {
		t.Run(name, func(t *testing.T) {
			if err := om.ValidateCanonicalDocument(bad); err == nil {
				t.Error("accepted a document it should reject")
			}
		})
	}
}

// TestMaterializeRejections — the error branches of the entry point, one per
// stage, so a stage that stops rejecting is visible here and not only in the
// corpus.
func TestMaterializeRejections(t *testing.T) {
	schema := []byte("model: \"0.2\"\nroot:\n  shape: object\n  children:\n    mtu:\n      shape: scalar\n      scalar_type: integer\n      default: 1500\n")

	for name, tc := range map[string]struct{ schema, input []byte }{
		"unparseable schema": {[]byte("\tbad\n"), []byte("{}\n")},
		"unparseable input":  {schema, []byte("\tbad\n")},
		"input not a map":    {schema, []byte("- a\n")},
		"undeclared child":   {schema, []byte("surprise: 1\n")},
		"origin in input":    {schema, []byte("mtu:\n  values: 1\n  origin: [yaml]\n")},
	} {
		t.Run(name, func(t *testing.T) {
			obj, err := om.Materialize(tc.schema, tc.input)
			if err == nil {
				t.Fatal("accepted input it should reject")
			}
			if obj != nil {
				t.Error("an object was returned alongside an error")
			}
			if e, ok := err.(*om.Error); ok && e.Stage == "" {
				t.Error("the rejection names no stage")
			}
		})
	}
}
