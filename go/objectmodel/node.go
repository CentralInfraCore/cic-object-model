package objectmodel

import (
	"strconv"
	"strings"
)

// valueKind is the arity of a node's payload (SPEC §2.1, `Value`).
type valueKind int

const (
	// kindAbsent is a staging state only: the node has no value yet. SPEC
	// §8.5 either fills it or the node does not exist.
	kindAbsent valueKind = iota
	kindScalar
	kindList
	kindMap
	kindOpaque
	// kindRaw is payload kept verbatim: an opaque value, or a list inside a
	// primitive declaration, which declares no element position and so is not
	// materialized into nodes (INV-035). Scalar leaves are kindScalar; the
	// mapping levels above them are kindMap.
	kindRaw
)

// Node is a CIC node (SPEC §2.1): exactly one `values` member (INV-001) and,
// in canonical form, exactly one `origin` member (INV-002), plus one member
// per primitive its schema declares (INV-022).
//
// Every field is unexported and there is no exported constructor, so a Node
// can only come out of Materialize. That is half of INV-032; CanonicalObject
// in materialize.go is the other half.
type Node struct {
	path   string
	isRoot bool
	eff    *effNode

	kind    valueKind
	scalar  any
	list    []*Node
	entries map[string]*Node
	order   []string
	opaque  any
	raw     any

	origin Origin

	// fromAbsence marks an object node that no author and no template
	// asserted. If nothing below it materializes, it does not exist.
	fromAbsence bool

	primitives map[string]*Node
	primOrder  []string

	// staging, populated by §8.4 and consumed by §8.6
	authoredPrims map[string]any
	extraMembers  map[string]any
}

// Path returns the node's address in the object ($.interface.mtu).
func (n *Node) Path() string { return n.path }

// Origin returns the node's authoring authority (SPEC §5).
func (n *Node) Origin() Origin { return n.origin }

// Child returns a child node of a structured payload.
func (n *Node) Child(name string) (*Node, bool) {
	if n.kind != kindMap {
		return nil, false
	}
	c, ok := n.entries[name]
	return c, ok
}

// Primitive returns a materialized primitive member of the node.
func (n *Node) Primitive(name string) (*Node, bool) {
	p, ok := n.primitives[name]
	return p, ok
}

// Scalar returns a scalar payload.
func (n *Node) Scalar() (any, bool) {
	if n.kind != kindScalar {
		return nil, false
	}
	return n.scalar, true
}

// Len reports the number of entries in a list payload, or 0 for any other kind.
func (n *Node) Len() int {
	if n.kind != kindList {
		return 0
	}
	return len(n.list)
}

// At returns the i-th entry of a list payload.
//
// A list entry is a node when the schema declared an element position via
// `item:` (INV-035). Where it did not — a CertPattern list inside an access
// rule, for instance — the list is the payload of its own node and has no
// entries to walk here; Len reports 0 for it.
func (n *Node) At(i int) (*Node, bool) {
	if n.kind != kindList || i < 0 || i >= len(n.list) {
		return nil, false
	}
	return n.list[i], true
}

// Children returns the child names of a structured payload, in canonical order.
func (n *Node) Children() []string {
	if n.kind != kindMap {
		return nil
	}
	out := make([]string, len(n.order))
	copy(out, n.order)
	return out
}

// Primitives returns the names of the node's materialized primitives, in the
// order SPEC §6.1 states the atom set.
func (n *Node) Primitives() []string {
	out := make([]string, len(n.primOrder))
	copy(out, n.primOrder)
	return out
}

// Get resolves an address against this node and is the operation SPEC §2.2
// promises: `network.values.mtu.access.read` is a first-class, addressable
// object. Without it the claim is true of the data and unavailable to a caller,
// who has to hand-walk Child and Primitive to reach a position the model says
// is addressable.
//
// The syntax is the one Error.Path already uses, so an address read out of a
// rejection can be fed straight back in:
//
//	$.values.mtu.access.read     absolute, from the root of the object
//	values.mtu.access            relative to this node
//	$.values.addresses.0         a list entry, by index
//
// A segment names a child of the payload, a primitive of the node, or — for a
// list payload — an entry index. `values` is a segment like any other: it steps
// into the payload, which is what makes an Error.Path resolvable.
//
// Get does not create anything and reports false for an address that does not
// resolve. It is a reader, not a cursor.
func (n *Node) Get(path string) (*Node, bool) {
	cur := n
	for _, seg := range splitPath(path) {
		next, ok := cur.step(seg)
		if !ok {
			return nil, false
		}
		cur = next
	}
	return cur, true
}

// step resolves one address segment: payload child, primitive, list index, or
// the `values` step into the payload itself.
func (n *Node) step(seg string) (*Node, bool) {
	if seg == "values" {
		// `values` addresses the payload. For a map or list node the payload is
		// reached through the following segment, so `values` is a no-op step
		// that keeps Error.Path addresses resolvable.
		return n, true
	}
	if c, ok := n.Child(seg); ok {
		return c, true
	}
	if p, ok := n.Primitive(seg); ok {
		return p, true
	}
	if n.kind == kindList {
		if i, err := strconv.Atoi(seg); err == nil {
			return n.At(i)
		}
	}
	return nil, false
}

// splitPath turns an address into segments, tolerating the leading `$.` that
// Error.Path carries.
func splitPath(path string) []string {
	path = strings.TrimPrefix(path, "$")
	path = strings.TrimPrefix(path, ".")
	if path == "" {
		return nil
	}
	return strings.Split(path, ".")
}

func (n *Node) setPrimitive(name string, p *Node) {
	if n.primitives == nil {
		n.primitives = map[string]*Node{}
	}
	if _, ok := n.primitives[name]; !ok {
		n.primOrder = append(n.primOrder, name)
	}
	n.primitives[name] = p
}
