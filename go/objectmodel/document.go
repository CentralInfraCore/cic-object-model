package objectmodel

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// parseDocument reads a YAML document and rejects the two things the
// serialization can do to an address before the model ever sees it (SPEC §2.6).
//
// Both checks are here rather than in §8.1 because neither needs the schema:
// they apply wherever a document is read, which is the schema, the authoring
// input and a canonical object presented on its own.
//
// # Why this is not left to the YAML library
//
// Because the library's answer is not the specification's, and the two
// implementations of this document found that out by disagreeing.
//
// gopkg.in/yaml.v3 rejects a duplicate key when decoding into `any` — but with
// a message and a position of its own, mapped here to INV-041 so the rejection
// names the rule rather than the parser. It also caps alias expansion
// ("document contains excessive aliasing"), which stops the amplification but
// accepts a document with one alias. The Rust implementation refuses every
// alias. That divergence was invisible to the corpus because no vector
// contained one, and INV-042 now settles it: no anchors, no aliases, and the
// check lives here rather than in whatever the library happens to allow.
func parseDocument(data []byte, invariant string, stage Stage, what string) (any, error) {
	fail := func(code, inv, detail string) error {
		return newError(code, inv, stage, "$", detail)
	}

	// Decode into a Node first: the node tree keeps every key the document
	// wrote, and it is the only place an alias is still distinguishable from
	// the value it expands to.
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fail(CodeMalformedDocument, invariant,
			fmt.Sprintf("%s is not valid YAML: %v", what, err))
	}
	if err := checkNode(&doc, what, stage); err != nil {
		return nil, err
	}

	var out any
	if doc.Kind != 0 {
		if err := doc.Decode(&out); err != nil {
			return nil, fail(CodeMalformedDocument, invariant,
				fmt.Sprintf("%s is not valid YAML: %v", what, err))
		}
	}
	return out, nil
}

// checkNode walks the node tree for INV-041 and INV-042.
func checkNode(n *yaml.Node, what string, stage Stage) error {
	if n == nil {
		return nil
	}

	// INV-042 — an alias makes one value reachable from two addresses, and
	// `origin` has no form that says "the same value as somewhere else". The
	// anchor is refused with it: an anchor nothing refers to is harmless, but
	// accepting it would leave the rule depending on how the document uses it.
	if n.Kind == yaml.AliasNode || n.Anchor != "" {
		return newError(CodeMalformedDocument, "INV-042", stage, "$",
			fmt.Sprintf("%s uses a YAML anchor or alias; neither is part of the format, "+
				"and expanding an alias can turn a few hundred bytes into millions of nodes",
				what))
	}

	// INV-041 — one address written twice has no single answer. The same name
	// at different addresses is not a duplicate, which is why this looks only
	// at the keys of ONE mapping.
	if n.Kind == yaml.MappingNode {
		seen := make(map[string]bool, len(n.Content)/2)
		for i := 0; i+1 < len(n.Content); i += 2 {
			k := n.Content[i].Value
			if seen[k] {
				return newError(CodeMalformedDocument, "INV-041", stage, "$",
					fmt.Sprintf("%s declares `%s` twice in one mapping; the same name at "+
						"different addresses is legal, one address written twice is not",
						what, k))
			}
			seen[k] = true
		}
	}

	for _, c := range n.Content {
		if err := checkNode(c, what, stage); err != nil {
			return err
		}
	}
	return nil
}
