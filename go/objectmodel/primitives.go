package objectmodel

import "fmt"

// evaluatePrimitives is SPEC §8.6.
//
//	INPUT:  node tree with values materialized
//	OUTPUT: node tree with every schema-declared primitive materialized as a node
//	MUST:   materialize every declared primitive (INV-022); resolve `inherit`
//	        chains for `access` (INV-025)
//	MUST NOT: admit an unknown primitive (INV-021)
//
// The `inherit` chain resolution this stage is told to perform is not
// implemented, and cannot be in 0.1: INV-025's tri-state resolves against the
// PolicySurface, which SPEC §1 puts out of scope. materialization/013 records
// `inherit` verbatim rather than resolved, so no vector requires resolution
// either. Recorded as docs/spec-defects.md SD-012 — a normative MUST with no
// reachable semantics, not an omission this implementation chose.
func evaluatePrimitives(n *Node) error {
	// INV-021 — a node must not carry a member that is neither values,
	// origin, nor a member of the primitive set.
	if len(n.extraMembers) > 0 {
		k := sortedKeys(n.extraMembers)[0]
		return newError(CodeUnknownPrimitive, "INV-021", StagePrimitiveEvaluation,
			n.path+"."+k,
			fmt.Sprintf("`%s` is not a member of the primitive set (%s)", k, primitiveSetList()))
	}

	// The root is an ordinary node and carries the primitives its schema
	// declares, like every other node (INV-022). 0.1 exempted it — because it
	// had to make room for the `cic` member that INV-033 demanded and INV-021
	// forbade — and every vector then showed a root without the `shape` its own
	// schema declared. docs/spec-defects.md SD-004, root cause SD-017.
	if err := attachPrimitives(n); err != nil {
		return err
	}

	switch n.kind {
	case kindMap:
		for _, k := range n.order {
			if err := evaluatePrimitives(n.entries[k]); err != nil {
				return err
			}
		}
	case kindList:
		for _, item := range n.list {
			if err := evaluatePrimitives(item); err != nil {
				return err
			}
		}
	}
	return nil
}

func attachPrimitives(n *Node) error {
	s := n.eff.sch
	for _, name := range primitiveSet {
		authored, authoredOK := n.authoredPrims[name]

		var decl any
		declared := false
		if name == "shape" {
			// The `shape` primitive is assembled from the schema keywords
			// `shape:` and `scalar_type:`, not declared as a payload.
			if s.hasShape {
				sv := map[string]any{"type": s.shape}
				if s.scalarType != "" {
					sv["scalar_type"] = s.scalarType
				}
				decl, declared = sv, true
			}
		} else {
			decl, declared = s.prims[name]
		}

		var payload any
		var org Origin
		switch {
		case authoredOK:
			payload, org = authored, originYAML()
		case declared:
			payload, org = decl, originSchema()
		default:
			continue
		}

		normalized, err := normalizePrimitive(name, payload, n.path+"."+name)
		if err != nil {
			return err
		}
		// INV-003 — the primitive member is itself a CIC node.
		// INV-035 — and so is every schema-declared position inside its
		// payload, all the way to the leaves.
		n.setPrimitive(name, primitiveNode(n.path+"."+name, normalized, org))
	}
	return nil
}

// primitiveNode builds the node tree for a primitive and its payload (INV-003,
// INV-035). Every mapping key inside the payload is a declared position, so it
// becomes a node carrying the primitive's own origin.
//
// Anonymous list entries are deliberately NOT wrapped. Nodehood follows
// declaration, not data shape (SPEC §2.2): a list becomes nodes when the schema
// declares an element position via `item:` — which happens in §8.4, not here —
// while a CertPattern list inside an access rule declares no element position.
// `subjects` is a node; the strings in it are its payload. Wrapping them would
// hand out `subjects[0]` as an address, and a positional address is exactly
// what §6.4 gives rules names to avoid.
//
// 0.1 stopped at the primitive and left the payload a raw mapping, which made
// SPEC §2.2's own example false of every object it produced
// (docs/spec-defects.md SD-003).
func primitiveNode(path string, payload any, org Origin) *Node {
	switch v := payload.(type) {
	case *orderedMap:
		n := &Node{path: path, kind: kindMap, origin: org,
			entries: map[string]*Node{}, order: nil}
		for _, k := range v.keys {
			child, _ := v.get(k)
			n.order = append(n.order, k)
			n.entries[k] = primitiveNode(path+".values."+k, child, org)
		}
		return n
	case map[string]any:
		om := newOrderedMap()
		for _, k := range sortedKeys(v) {
			om.set(k, v[k])
		}
		return primitiveNode(path, om, org)
	case []any:
		// A list inside a primitive payload declares no element position, so it
		// stays payload (INV-035). It is raw rather than scalar: Scalar() must
		// say no, and Len()/At() must not offer entries that are not nodes.
		return &Node{path: path, kind: kindRaw, raw: payload, origin: org}
	default:
		// A scalar leaf is a scalar, and Scalar() must return it. Building these
		// as kindRaw made every leaf inside a primitive payload unreadable
		// through the API while being perfectly present in the canonical YAML —
		// caught by objectmodel/api_test.go, not by any vector, because the
		// vectors compare serialized output and never call an accessor.
		return &Node{path: path, kind: kindScalar, scalar: payload, origin: org}
	}
}

// normalizePrimitive turns a declared primitive payload into its canonical
// form. Only `access` has a form SPEC fixes (§6.4); everything else is
// carried through as declared.
func normalizePrimitive(name string, payload any, path string) (any, error) {
	if name != "access" {
		return normalize(payload), nil
	}

	m, ok := asMap(payload)
	if !ok {
		return nil, newError(CodeTypeMismatch, "INV-024", StagePrimitiveEvaluation, path,
			"access must be a mapping of operations")
	}
	out := newOrderedMap()
	for _, op := range sortedKeys(m) {
		// INV-024 — the operations are `read` and `modify`. `write` is not a
		// valid operation name.
		if op != "read" && op != "modify" {
			return nil, newError(CodeInvalidAccessOperation, "INV-024", StagePrimitiveEvaluation,
				path+"."+op,
				fmt.Sprintf("`%s` is not a valid access operation; access declares `read` and `modify`", op))
		}
		opm, ok := asMap(m[op])
		if !ok {
			return nil, newError(CodeTypeMismatch, "INV-024", StagePrimitiveEvaluation,
				path+"."+op, "an access operation must be a mapping")
		}
		merged := make(map[string]any, len(opm)+1)
		for k, v := range opm {
			merged[k] = v
		}
		// INV-025 — inherit is per-operation and defaults to true.
		//
		// Injecting the default is what materialization/011 and /013 require,
		// but no invariant states that a primitive's sub-defaults are
		// materialized, and INV-026's `null` default for default_injection is
		// pointedly NOT injected by the same vectors. docs/spec-defects.md SD-001.
		if _, ok := merged["inherit"]; !ok {
			merged["inherit"] = true
		}
		// INV-026 — default_injection is invalid under modify: a denied write
		// has no value to inject.
		if op == "modify" {
			if _, ok := merged["default_injection"]; ok {
				return nil, newError(CodeDefaultInjectionOnModify, "INV-026", StagePrimitiveEvaluation,
					path+".modify.default_injection",
					"default_injection is valid under access.read only")
			}
		}
		out.set(op, normalize(merged))
	}
	return out, nil
}

func primitiveSetList() string {
	s := ""
	for i, p := range primitiveSet {
		if i > 0 {
			s += ", "
		}
		s += p
	}
	return s
}
