package objectmodel

import (
	"fmt"
	"strings"
)

// validateDocument is SPEC §8.7.
//
//	INPUT:  fully materialized node tree
//	OUTPUT: validated node tree
//	MUST:   enforce the origin grammar and truth table (§5.2, §5.3); confirm
//	        INV-001 and INV-002 hold at every node
//
// It runs on the projected document rather than on *Node so that the same
// rules apply to an object the materializer just built and to a hand-forged
// one presented at the module boundary (conformance/validation/*). That is
// the only way row 7 and row 8 of the truth table are reachable at all.
//
// It has to identify nodes without a schema, by looking for a `values` key —
// exactly what INV-008 forbids ("MUST NOT decide this by inspecting keys
// alone"). There is no alternative: validation/* supplies no schema.
// docs/spec-defects.md SD-013.
func validateDocument(doc any) error {
	m, ok := doc.(*orderedMap)
	if !ok {
		return newError(CodeMalformedDocument, "INV-001", StageFinalValidation, "$",
			"a canonical object must be a mapping")
	}

	// INV-033 — the model version belongs to the frame that hands the object
	// over, not to the object. There is nothing to check here: an object that
	// carries it is rejected below, as a member the node grammar has no room
	// for. 0.1 required the opposite and could not be satisfied by any object
	// (docs/spec-defects.md SD-017).
	return validateNode(m, "$", true)
}

func validateNode(m *orderedMap, path string, isDocRoot bool) error {
	// INV-001 / INV-002.
	if !m.has("values") {
		return newError(CodeMissingValues, "INV-001", StageFinalValidation, path+".values",
			"every CIC node must have exactly one `values` member")
	}
	if !m.has("origin") {
		return newError(CodeMissingOrigin, "INV-002", StageFinalValidation, path+".origin",
			"every CIC node in canonical form must have exactly one `origin` member")
	}

	for _, k := range m.keys {
		switch {
		case k == "values" || k == "origin":
		case isDocRoot && k == "cic":
			// INV-033 — the version travels with the hand-off, not inside the
			// object. A `cic` member is a member the node grammar has no room
			// for, so it falls to INV-021 like any other.
			return newError(CodeUnknownMember, "INV-021", StageFinalValidation, path+"."+k,
				"a node must not carry a member that is neither `values`, `origin`, nor a primitive; "+
					"the model version belongs to the frame that hands the object over")
		case k == "description" || k == "descr":
			// INV-006 — documentation is a property of the schema.
			return newError(CodeDocumentationOnNode, "INV-006", StageFinalValidation, path+"."+k,
				"documentation must be obtained from the schema, not carried on the node")
		case k == "default":
			// INV-012 — superseded by origin.
			return newError(CodeDefaultMemberOnNode, "INV-012", StageFinalValidation, path+"."+k,
				"value provenance is expressed through origin; a `default` member is not part of the canonical node")
		case isPrimitiveName(k):
		default:
			return newError(CodeUnknownPrimitive, "INV-021", StageFinalValidation, path+"."+k,
				fmt.Sprintf("`%s` is not a member of the primitive set (%s)", k, primitiveSetList()))
		}
	}

	if err := validateOrigin(m.vals["origin"], path+".origin"); err != nil {
		return err
	}

	// INV-003 — each primitive member is itself a CIC node.
	for _, k := range m.keys {
		if !isPrimitiveName(k) {
			continue
		}
		pm, ok := m.vals[k].(*orderedMap)
		if !ok {
			return newError(CodeMalformedDocument, "INV-003", StageFinalValidation, path+"."+k,
				"a primitive member must itself be a CIC node")
		}
		if err := validateNode(pm, path+"."+k, false); err != nil {
			return err
		}
	}

	return validatePayload(m.vals["values"], path)
}

func validatePayload(v any, path string) error {
	switch t := v.(type) {
	case *orderedMap:
		for _, k := range t.keys {
			child, ok := t.vals[k].(*orderedMap)
			if ok && child.has("values") {
				if err := validateNode(child, path+".values."+k, false); err != nil {
					return err
				}
			}
		}
	case []any:
		for i, e := range t {
			child, ok := e.(*orderedMap)
			if ok && child.has("values") {
				if err := validateNode(child, fmt.Sprintf("%s.values[%d]", path, i), false); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// validateOrigin enforces SPEC §5.2 (the grammar, INV-013) and §5.3 (the
// truth table, INV-016 / INV-017 / INV-018), plus INV-004 and INV-020.
func validateOrigin(v any, path string) error {
	// INV-004 — origin is terminal. If it has been expanded into a node it
	// would need an origin of its own, without end.
	if _, isMap := v.(*orderedMap); isMap {
		return newError(CodeOriginNotTerminal, "INV-004", StageFinalValidation, path,
			"origin must be a value in the grammar of SPEC §5.2, not a CIC node; it must not carry values, origin or primitives")
	}
	terms, ok := v.([]any)
	if !ok {
		return newError(CodeOriginNotTerminal, "INV-004", StageFinalValidation, path,
			"origin must be a sequence of origin terms")
	}
	// INV-018 — row 8. Every materialized node has an authority.
	if len(terms) == 0 {
		return newError(CodeOriginEmpty, "INV-018", StageFinalValidation, path,
			"origin must not be empty; the node's authority is unattributable")
	}

	// shape records the term kinds in the order they appear, so the sequence
	// itself can be matched against the grammar once every term is known to be
	// well formed. The boolean flags alongside it answer only "does this term
	// appear anywhere", which is a strictly weaker question — see the grammar
	// check at the end.
	var shape []string
	var hasYAML, hasSchema, hasSealed bool
	for _, t := range terms {
		switch tv := t.(type) {
		case string:
			switch tv {
			case string(OriginYAML):
				hasYAML = true
				shape = append(shape, "yaml")
			case string(OriginSchema):
				hasSchema = true
				shape = append(shape, "schema")
			case string(OriginSealed):
				// INV-020 — origin `sealed` is always a 2-arity constructor.
				// The bare token belongs to the aggregate slot-mode vocabulary.
				return newError(CodeOriginGrammar, "INV-020", StageFinalValidation, path,
					"the bare token `sealed` is not a valid origin term; origin sealed is always a constructor carrying template and path")
			default:
				return newError(CodeOriginGrammar, "INV-013", StageFinalValidation, path,
					fmt.Sprintf("`%s` is not an origin term", tv))
			}
		case *orderedMap:
			sv, ok := tv.get("sealed")
			if !ok {
				return newError(CodeOriginGrammar, "INV-013", StageFinalValidation, path,
					"the only constructor term in the origin grammar is `sealed`")
			}
			sm, ok := sv.(*orderedMap)
			if !ok {
				return newError(CodeSealedMissingTemplateOrPath, "INV-015", StageFinalValidation, path,
					"a sealed term must carry both template and path")
			}
			tpl, tok := sm.get("template")
			pth, pok := sm.get("path")
			if !tok || !pok || tpl == nil || pth == nil {
				// INV-015 — template identity alone is not provenance.
				return newError(CodeSealedMissingTemplateOrPath, "INV-015", StageFinalValidation, path,
					"a sealed term must carry both template and path")
			}
			// And nothing else, and both scalars.
			//
			// INV-013 says the grammar has exactly four productions. Checking
			// that the two members are PRESENT leaves the term open: a
			// constructor carrying a third member, or a `template` that is a
			// sequence, passed both implementations while being no production
			// the grammar contains. That is the same presence-versus-shape
			// mistake the sequence check already fixed one level up.
			if len(sm.keys) != 2 {
				return newError(CodeOriginGrammar, "INV-013", StageFinalValidation, path,
					fmt.Sprintf("a sealed term carries template and path and nothing else; found %v", sm.keys))
			}
			for _, member := range []struct {
				name  string
				value any
			}{{"template", tpl}, {"path", pth}} {
				switch member.value.(type) {
				case *orderedMap, []any:
					return newError(CodeOriginGrammar, "INV-013", StageFinalValidation, path,
						fmt.Sprintf("a sealed term's %s must be a scalar", member.name))
				}
			}
			hasSealed = true
			shape = append(shape, "sealed")
		default:
			return newError(CodeOriginGrammar, "INV-013", StageFinalValidation, path,
				"an origin term must be `yaml`, `schema`, or a sealed constructor")
		}
	}

	// INV-017 — row 7. A single effective value is either explicitly
	// supplied or defaulted; it cannot be both.
	if hasYAML && hasSchema {
		return newError(CodeOriginYAMLSchemaConflict, "INV-017", StageFinalValidation, path,
			"origin holds both `yaml` and `schema`; these are mutually exclusive value sources")
	}
	// INV-016 — rows 5 and 6. sealed means authoring is closed at and below
	// this node.
	if hasSealed && hasYAML {
		return newError(CodeOriginSealedYAMLConflict, "INV-016", StageFinalValidation, path,
			"origin holds both `sealed` and `yaml`; a yaml-sourced value below a sealed boundary is structurally illegal")
	}

	// INV-013 — the sequence must BE one of the four productions of §5.2, not
	// merely be built from legal terms that break none of the exclusion rules.
	//
	// Everything above this point asks presence questions: is `yaml` here, is
	// `schema` here. That vocabulary cannot express arity or order, so it
	// accepted forms the grammar does not contain — `[yaml, yaml]`,
	// `[schema, schema]`, `[sealed(t,p), sealed(t,p)]`, `[schema, sealed(t,p)]`
	// with the terms reversed. Each one is a distinct origin no rule forbade
	// and the grammar never produced, in the member that says who authored a
	// value.
	//
	// The corpus could not see it either. INV-013 says "exactly four forms" and
	// was covered by four vectors, one per form — every one of them positive.
	// "These four are accepted" was tested; "nothing else is" was not, and
	// check_spec_vectors.py cannot tell the difference because it checks that a
	// mapping exists, not what the mapping proves.
	//
	// The exclusion checks above are now implied by this match and are kept
	// deliberately: they name the specific invariant a reader violated
	// (INV-016, INV-017) instead of reporting that the sequence was not in the
	// grammar, and the vectors assert those codes.
	switch {
	case matches(shape, "yaml"),
		matches(shape, "schema"),
		matches(shape, "sealed"),
		matches(shape, "sealed", "schema"):
		return nil
	}
	return newError(CodeOriginGrammar, "INV-013", StageFinalValidation, path,
		fmt.Sprintf("origin [%s] is not one of the four forms of SPEC §5.2: "+
			"[yaml] | [schema] | [sealed(t,p)] | [sealed(t,p), schema]",
			strings.Join(shape, ", ")))
}

func matches(shape []string, want ...string) bool {
	if len(shape) != len(want) {
		return false
	}
	for i := range want {
		if shape[i] != want[i] {
			return false
		}
	}
	return true
}
