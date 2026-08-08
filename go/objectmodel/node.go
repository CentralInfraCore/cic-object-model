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

// Origin returns the node's authoring authority (SPEC §5), copied.
//
// The copy is the whole point. This method used to return the node's own
// slice, so a caller could write through it and change the provenance of a
// validated object in place:
//
//	obj.Root().Origin()[0].Kind = "evil"
//
// After that the tree said `evil` while CanonicalYAML() still said `schema`,
// byte for byte unchanged. No forgery and no unsafe was needed — one
// assignment split the two views of one object apart, and which half a reader
// consults then decides what it believes about who authored the value.
//
// Children() and Primitives() already copied; this one was missed. OriginTerm
// holds only strings, so copying the slice copies everything reachable.
func (n *Node) Origin() Origin {
	out := make(Origin, len(n.origin))
	copy(out, n.origin)
	return out
}

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

// Get resolves an address against this node (SPEC §2.5) and is the operation
// §2.2 promises: every declared position is a first-class, addressable object. Without it the claim is true of the data and unavailable to a caller,
// who has to hand-walk Child and Primitive to reach a position the model says
// is addressable.
//
// The syntax is the one Error.Path already uses, so an address read out of a
// rejection can be fed straight back in:
//
//	$.values.mtu.access               absolute — `mtu`'s access primitive
//	$.values.mtu.access.values.read   the access payload's `read` child
//	values.mtu                        relative to this node
//	$.values.addresses.values[0]      a list entry
//
// `values` steps into the payload and is never optional; anything else is a
// member of the node itself. That is the whole grammar, and it is what makes
// an address unique (INV-040): without the mandatory step, `$.values.mtu` and
// `$.values.values.values.mtu` would name the same node, and a primitive would
// be indistinguishable from a payload child that shares its name.
//
// Get does not create anything and reports false for an address that does not
// resolve. It is a reader, not a cursor.
func (n *Node) Get(path string) (*Node, bool) {
	cur := n
	inPayload := false // did the previous segment step into the payload?

	for _, seg := range splitPath(path) {
		switch {
		case inPayload:
			// Only a payload child can follow a `values` step.
			c, ok := cur.Child(seg)
			if !ok {
				return nil, false
			}
			cur, inPayload = c, false

		case seg == "values":
			// The payload step itself resolves nothing; the segment after it
			// does. Keeping it as state rather than a node is what stops
			// `$.values.values.mtu` from meaning `$.values.mtu`.
			inPayload = true

		default:
			if i, ok := parseIndex(seg); ok {
				// `values[i]` is the payload step and the entry in one token.
				e, ok := cur.At(i)
				if !ok {
					return nil, false
				}
				cur = e
				continue
			}
			// Anything else on a node is a member of the node: a primitive.
			p, ok := cur.Primitive(seg)
			if !ok {
				return nil, false
			}
			cur = p
		}
	}
	if inPayload {
		// A trailing `values` names the payload, which is not a node.
		return nil, false
	}
	return cur, true
}

// parseIndex recognises the `values[i]` token of SPEC §2.5.
func parseIndex(seg string) (int, bool) {
	if !strings.HasPrefix(seg, "values[") || !strings.HasSuffix(seg, "]") {
		return 0, false
	}
	i, err := strconv.Atoi(seg[len("values[") : len(seg)-1])
	if err != nil {
		return 0, false
	}
	return i, true
}

// splitPath turns an address into segments, tolerating the leading `$` that
// Path() and Error.Path carry. `values[0]` stays one segment.
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
