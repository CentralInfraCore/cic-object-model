package objectmodel_test

import (
	"os"
	"path/filepath"
	"testing"

	om "github.com/CentralInfraCore/cic-object-model/go/objectmodel"
)

// The reader API is exercised against a real vector rather than a hand-built
// fixture. A fixture would only prove the accessors agree with themselves; the
// corpus is what the specification is written against.
const vectorRoot = "../../conformance"

func materializeVector(t *testing.T, dir string) om.CanonicalObject {
	t.Helper()
	schema, err := os.ReadFile(filepath.Join(vectorRoot, dir, "schema.yaml"))
	if err != nil {
		t.Fatalf("schema: %v", err)
	}
	input, err := os.ReadFile(filepath.Join(vectorRoot, dir, "input.yaml"))
	if err != nil {
		t.Fatalf("input: %v", err)
	}
	obj, err := om.Materialize(schema, input)
	if err != nil {
		t.Fatalf("materialize %s: %v", dir, err)
	}
	return obj
}

// TestGetReachesTheAddressSpecClaims is the API half of SPEC §2.2. The
// specification says network.values.mtu.access.read is an addressable object;
// this asserts a caller can actually address it.
func TestGetReachesTheAddressSpecClaims(t *testing.T) {
	obj := materializeVector(t, "materialization/013_access_inherit_injection")
	root := obj.Root()

	// The full depth the model promises: a named rule inside an access
	// operation, four levels below the primitive.
	n, ok := root.Get("$.values.mtu.access.values.read.values.rules.values.operator.values.effect")
	if !ok {
		t.Fatal("the §2.2 address did not resolve")
	}
	if v, _ := n.Scalar(); v != "allow" {
		t.Fatalf("effect = %v, want allow", v)
	}

	// Relative addressing from an interior node must agree with the absolute
	// form — otherwise Get is two different operations wearing one name.
	mtu, ok := root.Get("values.mtu")
	if !ok {
		t.Fatal("relative address values.mtu did not resolve")
	}
	rel, ok := mtu.Get("access.values.read.values.rules.values.operator.values.effect")
	if !ok {
		t.Fatal("relative address from mtu did not resolve")
	}
	if rel.Path() != n.Path() {
		t.Fatalf("relative and absolute disagree: %s vs %s", rel.Path(), n.Path())
	}
}

// TestErrorPathIsAddressable is the reason Get takes Error.Path's syntax: a
// rejection names a position, and the caller must be able to go look at it.
func TestErrorPathIsAddressable(t *testing.T) {
	obj := materializeVector(t, "materialization/013_access_inherit_injection")
	n, ok := obj.Root().Get("$.values.mtu.access")
	if !ok {
		t.Fatal("$.values.mtu.access did not resolve")
	}
	if got, ok := obj.Root().Get(n.Path()); !ok || got.Path() != n.Path() {
		t.Fatalf("a node's own Path() is not resolvable by Get: %q", n.Path())
	}
}

// TestGetReportsMisses rather than panicking or inventing nodes. An address
// that does not resolve is a question with the answer "no".
func TestGetReportsMisses(t *testing.T) {
	root := materializeVector(t, "materialization/001_origin_yaml").Root()
	for _, addr := range []string{
		"$.values.nonexistent",
		"$.values.mtu.access",                          // not declared on this vector
		"$.values.mtu.shape.values.type.values.deeper", // past a leaf
		"$.values.mtu.values[7]",                       // index into a non-list
		"$.values.values.values.mtu",                   // a values step that names nothing
		"$.values.mtu.values.access",                   // access is a primitive, not a payload child
	} {
		if _, ok := root.Get(addr); ok {
			t.Errorf("Get(%q) resolved, want miss", addr)
		}
	}
}

// TestListTraversal covers the hole the 0.2 API review found: before Len/At
// there was no way to reach the contents of a list-valued node at all.
func TestListTraversal(t *testing.T) {
	root := materializeVector(t, "materialization/008_normalize_list").Root()
	addrs, ok := root.Get("$.values.addresses")
	if !ok {
		t.Fatal("addresses did not resolve")
	}
	if addrs.Len() != 2 {
		t.Fatalf("Len = %d, want 2", addrs.Len())
	}
	first, ok := addrs.At(0)
	if !ok {
		t.Fatal("At(0) missed")
	}
	if v, _ := first.Scalar(); v != "10.0.0.1/24" {
		t.Fatalf("At(0) = %v, want 10.0.0.1/24", v)
	}
	if _, ok := addrs.At(2); ok {
		t.Error("At(2) resolved on a 2-entry list")
	}
	// The same entry by address, so index addressing and At agree.
	byAddr, ok := root.Get("$.values.addresses.values[0]")
	if !ok || byAddr.Path() != first.Path() {
		t.Fatal("index addressing disagrees with At(0)")
	}
}

// TestEnumerationIsOrderedAndCopied — callers get canonical order, and cannot
// reach into the node by mutating what they were handed.
func TestEnumerationIsOrderedAndCopied(t *testing.T) {
	root := materializeVector(t, "materialization/013_access_inherit_injection").Root()
	mtu, _ := root.Get("$.values.mtu")

	prims := mtu.Primitives()
	if len(prims) == 0 {
		t.Fatal("no primitives enumerated on mtu")
	}
	prims[0] = "clobbered"
	if again := mtu.Primitives(); again[0] == "clobbered" {
		t.Error("Primitives() hands out the node's own slice")
	}

	kids := root.Children()
	if len(kids) == 0 {
		t.Fatal("no children enumerated on root")
	}
	kids[0] = "clobbered"
	if again := root.Children(); again[0] == "clobbered" {
		t.Error("Children() hands out the node's own slice")
	}
}
