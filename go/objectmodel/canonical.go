package objectmodel

// project is the projection half of SPEC §8.8: the node tree becomes the
// document the canonical serializer writes and the final validator reads.
//
// Member order is values, origin, then primitives in the order SPEC §6.1
// states the atom set. SPEC §8.8 defines no order; see docs/spec-defects.md SD-010.
func (n *Node) project() any {
	m := newOrderedMap()
	m.set("values", n.projectValue())
	m.set("origin", n.origin.project())
	for _, name := range primitiveSet {
		if p, ok := n.primitives[name]; ok {
			m.set(name, p.project())
		}
	}
	return m
}

func (n *Node) projectValue() any {
	switch n.kind {
	case kindScalar:
		return n.scalar
	case kindOpaque:
		// INV-028 / INV-011 — verbatim. Keys named like primitives below
		// here are domain data.
		return normalize(n.opaque)
	case kindRaw:
		return n.raw
	case kindList:
		out := make([]any, 0, len(n.list))
		for _, c := range n.list {
			out = append(out, c.project())
		}
		return out
	case kindMap:
		m := newOrderedMap()
		for _, k := range n.order {
			m.set(k, n.entries[k].project())
		}
		return m
	}
	return nil
}

// document projects the root. Model 0.2: the root is an ordinary node.
//
// 0.1 stamped a `cic` member carrying the model version here and emitted no
// primitives on the root, which broke INV-021 and INV-022 at the one node every
// consumer touches first (docs/spec-defects.md SD-004). The cause was INV-033
// requiring the object to carry its version while §2.1 closes the node grammar
// to values, origin and primitives — a requirement no object could satisfy
// (SD-017). In 0.2 the version belongs to the frame that hands the object over,
// so nothing about it appears here, and the root projects like any other node.
func (n *Node) document(_ string) any {
	return n.project()
}
