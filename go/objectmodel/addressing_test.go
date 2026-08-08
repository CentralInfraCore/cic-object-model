package objectmodel_test

import (
	"testing"

	om "github.com/CentralInfraCore/cic-object-model/go/objectmodel"
)

// INV-040 says every node has exactly one address. That was asserted nowhere:
// the resolver accepted eight spellings of it and the corpus never called an
// accessor, so nothing measured the property the invariant is named for.
//
// The strongest form of it is a round trip — every address the object hands out
// resolves back to the node that handed it out, and nothing else does. That is
// what this file checks.

// walk visits every node reachable through the reader API, so the assertions
// below run over the whole object rather than a hand-picked path.
func walk(n *om.Node, visit func(*om.Node)) {
	visit(n)
	for _, name := range n.Children() {
		if c, ok := n.Child(name); ok {
			walk(c, visit)
		}
	}
	for i := 0; i < n.Len(); i++ {
		if e, ok := n.At(i); ok {
			walk(e, visit)
		}
	}
	for _, name := range n.Primitives() {
		if p, ok := n.Primitive(name); ok {
			walk(p, visit)
		}
	}
}

// TestAddressRoundTrip — Path() and Get are inverse over the whole object.
//
// A resolver can be wrong in two directions and only one of them is obvious. If
// Get is too strict, an address the object printed stops resolving and the
// failure is loud. If it is too loose it still resolves, just possibly to
// something else — which is how `$` came to be decorative: an absolute address
// on a non-root node returned a different node and reported success.
func TestAddressRoundTrip(t *testing.T) {
	for _, vector := range []string{
		"materialization/013_access_inherit_injection",
		"materialization/008_normalize_list",
		"materialization/009_normalize_map",
		"materialization/006_closure_opaque",
	} {
		t.Run(vector, func(t *testing.T) {
			root := materializeVector(t, vector).Root()

			var n int
			walk(root, func(node *om.Node) {
				n++
				addr := node.Path()
				got, ok := root.Get(addr)
				if !ok {
					t.Errorf("the object printed %q and cannot resolve it", addr)
					return
				}
				if got != node {
					t.Errorf("%q resolved to %q, a different node", addr, got.Path())
				}
			})
			if n < 2 {
				t.Fatalf("walked %d nodes; the traversal is not reaching the object", n)
			}
			t.Logf("%d nodes, every address resolving to itself", n)
		})
	}
}

// TestAddressAliasesAreRejected — the spellings that used to name the same node.
func TestAddressAliasesAreRejected(t *testing.T) {
	root := materializeVector(t, "materialization/001_origin_yaml").Root()

	// The canonical form still works, and is the one Path() produces.
	canonical, ok := root.Get("$.values.mtu")
	if !ok {
		t.Fatal("the canonical address stopped resolving")
	}
	if canonical.Path() != "$.values.mtu" {
		t.Fatalf("Path() = %q, want $.values.mtu", canonical.Path())
	}

	// A relative address is a different operation, not a spelling of the
	// absolute one, and it is kept: nothing else navigates from a node.
	if rel, ok := root.Get("values.mtu"); !ok || rel != canonical {
		t.Error("relative resolution from the receiver stopped working")
	}

	for _, alias := range []string{
		".values.mtu", // leading separator with no root
		"$values.mtu", // `$` not followed by a separator
		"$.",          // root, then an empty segment
		".",           // an empty segment
		"$..values",   // a doubled separator
		"values.",     // a trailing separator
		"$.values.",   // a trailing separator, absolute
	} {
		if n, ok := root.Get(alias); ok {
			t.Errorf("%q resolved to %q; it is not an address", alias, n.Path())
		}
	}

	// "" is the receiver and "$" is the root. They are two operations that
	// coincide here because the receiver IS the root.
	if n, ok := root.Get(""); !ok || n != root {
		t.Error(`"" must name the receiver`)
	}
	if n, ok := root.Get("$"); !ok || n != root {
		t.Error(`"$" must name the root`)
	}
}

// TestAbsoluteAddressNeedsTheRoot — `$` names the root, so only the root can
// answer for it.
//
// This was the substantive defect rather than a cosmetic one. `$` was trimmed
// and discarded, so an address that named one node resolved to another and
// reported true. A caller taking Error.Path from a rejection and feeding it to
// the node it was holding got a wrong node, not a miss — and a wrong node with
// ok=true is not a failure anyone checks for.
func TestAbsoluteAddressNeedsTheRoot(t *testing.T) {
	root := materializeVector(t, "materialization/001_origin_yaml").Root()
	mtu, ok := root.Get("$.values.mtu")
	if !ok {
		t.Fatal("$.values.mtu did not resolve")
	}

	// Measured before the fix: this returned $.values.mtu.shape, ok=true.
	if n, ok := mtu.Get("$.shape"); ok {
		t.Errorf("an absolute address resolved from a non-root node, to %q", n.Path())
	}
	if n, ok := mtu.Get("$"); ok {
		t.Errorf(`"$" resolved from a non-root node, to %q`, n.Path())
	}

	// The relative form from the same node is how you reach it, and it works.
	if _, ok := mtu.Get("shape"); !ok {
		t.Error("the relative address of the shape primitive does not resolve")
	}
	// And the absolute form still works from the root, naming the same node.
	fromRoot, ok := root.Get("$.values.mtu.shape")
	if !ok {
		t.Fatal("$.values.mtu.shape did not resolve from the root")
	}
	viaNode, _ := mtu.Get("shape")
	if fromRoot != viaNode {
		t.Error("the absolute and relative routes reached different nodes")
	}
}
