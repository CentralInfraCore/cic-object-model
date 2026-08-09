package objectmodel_test

import (
	"strings"
	"testing"

	om "github.com/CentralInfraCore/cic-object-model/go/objectmodel"
)

// The paths the corpus does not reach.
//
// The 26 vectors are chosen to pin the specification, not to exercise every
// branch of an implementation, and those are different jobs. What is left
// uncovered after them is mostly rejection: a required value that is absent, a
// list where a mapping was declared, a default of the wrong type. Those are the
// paths a caller meets when it holds the library wrong, which makes them the
// paths most worth being sure about.
//
// Each case below names the branch it exists for, so a future reader can tell
// whether deleting it loses anything.

// materialize is a helper for the many single-shot cases below.
func materialize(t *testing.T, schema, input string) (om.CanonicalObject, error) {
	t.Helper()
	return om.Materialize([]byte(schema), []byte(input))
}

func mustReject(t *testing.T, schema, input, wantStage string) *om.Error {
	t.Helper()
	obj, err := materialize(t, schema, input)
	if err == nil {
		t.Fatalf("accepted input it should reject; got %v", obj != nil)
	}
	e, ok := err.(*om.Error)
	if !ok {
		t.Fatalf("rejection is %T, want *objectmodel.Error: %v", err, err)
	}
	if wantStage != "" && string(e.Stage) != wantStage {
		t.Errorf("stage = %s, want %s (%s)", e.Stage, wantStage, e.Detail)
	}
	return e
}

// TestDefaultMaterializationBranches — SPEC §8.5. Absence is where the
// interesting decisions are: a value nobody authored either defaults, or is
// required and rejected, or the node does not exist at all.
func TestDefaultMaterializationBranches(t *testing.T) {
	t.Run("required and absent is rejected", func(t *testing.T) {
		e := mustReject(t,
			"model: \"0.2\"\nroot:\n  shape: object\n  children:\n    mtu:\n      shape: scalar\n      scalar_type: integer\n      required: true\n",
			"{}\n", "default-materialization")
		if e.Invariant != "INV-022" {
			t.Errorf("invariant = %s, want INV-022", e.Invariant)
		}
	})

	t.Run("absent and not defaultable simply does not exist", func(t *testing.T) {
		// Neither authored nor defaulted nor required: SPEC §8.5 says the node
		// does not exist rather than materializing as null.
		obj, err := materialize(t,
			"model: \"0.2\"\nroot:\n  shape: object\n  children:\n    mtu:\n      shape: scalar\n      scalar_type: integer\n",
			"{}\n")
		if err != nil {
			t.Fatalf("rejected an object that is legitimately empty: %v", err)
		}
		if _, ok := obj.Root().Child("mtu"); ok {
			t.Error("a node with no value, no default and no requirement was materialized anyway")
		}
	})

	t.Run("a default fills a nested object", func(t *testing.T) {
		obj, err := materialize(t,
			"model: \"0.2\"\nroot:\n  shape: object\n  children:\n    net:\n      shape: object\n      children:\n        mtu:\n          shape: scalar\n          scalar_type: integer\n          default: 1500\n",
			"net: {}\n")
		if err != nil {
			t.Fatalf("materialization failed: %v", err)
		}
		n, ok := obj.Root().Get("$.values.net.values.mtu")
		if !ok {
			t.Fatal("the nested default did not materialize")
		}
		if v, _ := n.Scalar(); v != 1500 {
			t.Errorf("value = %v, want 1500", v)
		}
		if org := n.Origin(); len(org) != 1 || org[0].Kind != om.OriginSchema {
			t.Errorf("origin = %v, want [schema] — the value came from the default", org)
		}
	})

	t.Run("a list default materializes its entries", func(t *testing.T) {
		obj, err := materialize(t,
			"model: \"0.2\"\nroot:\n  shape: object\n  children:\n    addrs:\n      shape: list\n      default: [a, b]\n      item:\n        shape: scalar\n        scalar_type: string\n",
			"{}\n")
		if err != nil {
			t.Fatalf("materialization failed: %v", err)
		}
		l, ok := obj.Root().Get("$.values.addrs")
		if !ok {
			t.Fatal("the list default did not materialize")
		}
		if l.Len() != 2 {
			t.Errorf("Len = %d, want 2", l.Len())
		}
	})
}

// TestConstructionTypeMismatches — SPEC §8.4. The schema declares a shape and
// the input disagrees. Each of these is a caller mistake, and each must be a
// rejection naming the position rather than a panic or a silent coercion.
func TestConstructionTypeMismatches(t *testing.T) {
	for name, tc := range map[string]struct{ schema, input string }{
		"a list where an object is declared": {
			"model: \"0.2\"\nroot:\n  shape: object\n  children:\n    net:\n      shape: object\n      children:\n        a:\n          shape: scalar\n          scalar_type: string\n",
			"net:\n  - one\n  - two\n",
		},
		"an object where a list is declared": {
			"model: \"0.2\"\nroot:\n  shape: object\n  children:\n    addrs:\n      shape: list\n      item:\n        shape: scalar\n        scalar_type: string\n",
			"addrs:\n  a: 1\n",
		},
		"a scalar where an object is declared": {
			"model: \"0.2\"\nroot:\n  shape: object\n  children:\n    net:\n      shape: object\n      children:\n        a:\n          shape: scalar\n          scalar_type: string\n",
			"net: 5\n",
		},
		"a list with no item shape declared": {
			"model: \"0.2\"\nroot:\n  shape: object\n  children:\n    addrs:\n      shape: list\n",
			"addrs: [a]\n",
		},
	} {
		t.Run(name, func(t *testing.T) {
			e := mustReject(t, tc.schema, tc.input, "")
			if e.Path == "" || !strings.HasPrefix(e.Path, "$") {
				t.Errorf("the rejection does not name a position: %q", e.Path)
			}
		})
	}
}

// TestOpaqueAndListPayloads — the payload kinds the corpus touches once each,
// exercised through the accessors a caller reads them with.
func TestOpaqueAndListPayloads(t *testing.T) {
	t.Run("an opaque payload is preserved and is not a node", func(t *testing.T) {
		obj, err := materialize(t,
			"model: \"0.2\"\nroot:\n  shape: object\n  children:\n    blob:\n      shape: opaque\n",
			"blob:\n  anything: [1, 2]\n  nested:\n    deep: true\n")
		if err != nil {
			t.Fatalf("materialization failed: %v", err)
		}
		blob, ok := obj.Root().Get("$.values.blob")
		if !ok {
			t.Fatal("blob did not resolve")
		}
		// INV-028: no CIC semantics below an opaque value, so nothing inside it
		// is addressable.
		if _, ok := blob.Get("values.anything"); ok {
			t.Error("a child inside an opaque payload was addressable")
		}
		if !strings.Contains(string(obj.CanonicalYAML()), "deep") {
			t.Error("the opaque payload was not preserved verbatim")
		}
	})

	t.Run("a list of objects materializes each entry", func(t *testing.T) {
		obj, err := materialize(t,
			"model: \"0.2\"\nroot:\n  shape: object\n  children:\n    peers:\n      shape: list\n      item:\n        shape: object\n        children:\n          name:\n            shape: scalar\n            scalar_type: string\n",
			"peers:\n  - name: a\n  - name: b\n")
		if err != nil {
			t.Fatalf("materialization failed: %v", err)
		}
		n, ok := obj.Root().Get("$.values.peers.values[1].values.name")
		if !ok {
			t.Fatal("the second entry's name did not resolve")
		}
		if v, _ := n.Scalar(); v != "b" {
			t.Errorf("value = %v, want b", v)
		}
	})
}

// TestSchemaRejectionBranches — SPEC's schema-load stage. A bad schema must
// fail before any input is read, so a caller is never told its data is wrong
// when the schema was.
func TestSchemaRejectionBranches(t *testing.T) {
	for name, schema := range map[string]string{
		"root is not a mapping":       "model: \"0.2\"\nroot: 5\n",
		"children is not a mapping":   "model: \"0.2\"\nroot:\n  shape: object\n  children: 5\n",
		"a child is not a mapping":    "model: \"0.2\"\nroot:\n  shape: object\n  children:\n    a: 5\n",
		"templates is not a mapping":  "model: \"0.2\"\nroot:\n  shape: object\ntemplates: 5\n",
		"a template is not a mapping": "model: \"0.2\"\nroot:\n  shape: object\ntemplates:\n  $t: 5\n",
		"origin declared as a child": "model: \"0.2\"\nroot:\n  shape: object\n  children:\n    origin:\n      shape: scalar\n" +
			"      scalar_type: string\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := om.LoadSchema([]byte(schema)); err == nil {
				t.Error("accepted a schema it should reject")
			}
		})
	}
}

// TestValidationBranches — SPEC §8.7 on objects assembled by hand, reaching the
// rejections the validation vectors do not cover.
func TestValidationBranches(t *testing.T) {
	for name, doc := range map[string]string{
		"a primitive that is not a node": "values:\n  mtu:\n    values: 1\n    origin: [yaml]\n    shape: scalar\norigin: [yaml]\n",
		"an unknown member on a node":    "values:\n  mtu:\n    values: 1\n    origin: [yaml]\n    surprise: 1\norigin: [yaml]\n",
		"an origin term that is unknown": "values:\n  mtu:\n    values: 1\n    origin: [invented]\norigin: [yaml]\n",
		"a sealed origin without a path": "values:\n  mtu:\n    values: 1\n    origin:\n      - sealed:\n          template: $t\norigin: [yaml]\n",
		"origin is not a list":           "values:\n  mtu:\n    values: 1\n    origin: yaml\norigin: [yaml]\n",
	} {
		t.Run(name, func(t *testing.T) {
			if err := om.ValidateCanonicalDocument([]byte(doc)); err == nil {
				t.Error("accepted a document it should reject")
			}
		})
	}
}

// TestPrimitiveEvaluationBranches — SPEC §8.6, the access structure §6.4 fixes.
func TestPrimitiveEvaluationBranches(t *testing.T) {
	base := "model: \"0.2\"\nroot:\n  shape: object\n  children:\n    mtu:\n      shape: scalar\n      scalar_type: integer\n"

	for name, tc := range map[string]struct{ schema, input string }{
		"access is not a mapping": {
			base + "      access: 5\n", "mtu: 1\n",
		},
		"an access operation is not a mapping": {
			base + "      access:\n        read: 5\n", "mtu: 1\n",
		},
		"write is not a valid operation": {
			base + "      access:\n        write:\n          rules: {}\n", "mtu: 1\n",
		},
		"default_injection under modify": {
			base + "      access:\n        modify:\n          rules: {}\n          default_injection: 0\n", "mtu: 1\n",
		},
	} {
		t.Run(name, func(t *testing.T) {
			e := mustReject(t, tc.schema, tc.input, "primitive-evaluation")
			if e.Code == "" {
				t.Error("the rejection carries no code")
			}
		})
	}
}

// TestSealedTemplateContent — SPEC §8.3. A sealed node takes its value from the
// template's `content:`, and the shapes that content can have are the branches
// the corpus reaches only for objects.
func TestSealedTemplateContent(t *testing.T) {
	tmpl := func(shape, content string) string {
		return "model: \"0.2\"\ntemplates:\n  $t:\n    $.a:\n      shape: " + shape + "\n" + content +
			"root:\n  shape: object\n  children:\n    a:\n      sealed_from:\n        template: $t\n        path: $.a\n"
	}

	t.Run("opaque content is preserved whole", func(t *testing.T) {
		obj, err := materialize(t, tmpl("opaque", "      content:\n        any: [1, 2]\n"), "{}\n")
		if err != nil {
			t.Fatalf("materialization failed: %v", err)
		}
		if !strings.Contains(string(obj.CanonicalYAML()), "any") {
			t.Error("the sealed opaque content was not carried through")
		}
		n, ok := obj.Root().Get("$.values.a")
		if !ok {
			t.Fatal("the sealed node did not materialize")
		}
		if org := n.Origin(); len(org) == 0 || org[0].Kind != om.OriginSealed {
			t.Errorf("origin = %v, want a sealed term — the value came from a template", org)
		}
	})

	t.Run("list content materializes its entries", func(t *testing.T) {
		obj, err := materialize(t,
			tmpl("list", "      item:\n        shape: scalar\n        scalar_type: string\n      content: [x, y]\n"),
			"{}\n")
		if err != nil {
			t.Fatalf("materialization failed: %v", err)
		}
		l, ok := obj.Root().Get("$.values.a")
		if !ok {
			t.Fatal("the sealed list did not materialize")
		}
		if l.Len() != 2 {
			t.Errorf("Len = %d, want 2", l.Len())
		}
	})

	t.Run("list content that is not a sequence is rejected", func(t *testing.T) {
		mustReject(t,
			tmpl("list", "      item:\n        shape: scalar\n        scalar_type: string\n      content:\n        not: a list\n"),
			"{}\n", "")
	})

	t.Run("authoring below a sealed node is rejected", func(t *testing.T) {
		e := mustReject(t, tmpl("opaque", "      content:\n        any: 1\n"), "a:\n  reaching: in\n", "entry-validation")
		if e.Invariant != "INV-019" {
			t.Errorf("invariant = %s, want INV-019", e.Invariant)
		}
	})

	t.Run("a sealed reference to a template that does not exist", func(t *testing.T) {
		mustReject(t,
			"model: \"0.2\"\nroot:\n  shape: object\n  children:\n    a:\n      sealed_from:\n        template: $missing\n        path: $.a\n",
			"{}\n", "")
	})
}

// TestNonStringMappingKeys — a mapping key is a node name, and node names are
// strings.
//
// This used to assert INV-029: the key was read through the YAML node's textual
// value, so `1:` arrived as the name "1" and was then rejected for not being
// declared. A rejection for the wrong reason, and it hid two things.
//
// It made `1: x` and `'1': x` — different keys in YAML — indistinguishable, in
// a model whose purpose is unique addressing. And it disagreed with the Rust
// implementation, which refused non-string keys outright, so the two did not
// agree on which names a document contains. Neither was visible from any
// vector.
//
// The last case here was previously asserted as "does not crash and does not
// pass", with a comment saying pinning the invariant would pin behaviour nobody
// designed. It is designed now.
func TestNonStringMappingKeys(t *testing.T) {
	schema := "model: \"0.2\"\nroot:\n  shape: object\n  children:\n    a:\n      shape: scalar\n      scalar_type: string\n"

	for name, input := range map[string]string{
		"an integer key": "1: one\n",
		"a boolean key":  "true: yes\n",
		"a null key":     "~: x\n",
		"a sequence key": "? [a, b]\n: value\n",
	} {
		t.Run(name, func(t *testing.T) {
			e := mustReject(t, schema, input, "entry-validation")
			if e.Code != om.CodeMalformedDocument {
				t.Errorf("code = %s, want %s", e.Code, om.CodeMalformedDocument)
			}
		})
	}

	// The quoted form of the same text IS a string key, and is refused for the
	// ordinary reason: nothing declares it. The check must reject the KEY FORM,
	// not every key that happens to look numeric.
	t.Run("a quoted numeric key is a string, merely undeclared", func(t *testing.T) {
		e := mustReject(t, schema, "'1': one\n", "entry-validation")
		if e.Invariant != "INV-029" {
			t.Errorf("invariant = %s, want INV-029", e.Invariant)
		}
	})
}
