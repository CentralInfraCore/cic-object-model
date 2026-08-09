package objectmodel

import (
	"strings"
	"testing"
)

// The canonical serializer, at the edges the corpus does not reach.
//
// §8.8.1 is a specification of bytes, and a second implementation has to
// reproduce it exactly — so the cases where "what does this scalar look like"
// has a non-obvious answer are the ones worth pinning. The corpus exercises the
// common path thirteen times and touches almost none of these.

// TestQuotingRemovesAmbiguityRatherThanAnsweringIt — §8.8.1's scalar rule.
//
// A plain scalar is quoted when a reader could take it for something other than
// text. The interesting set is not the syntactically dangerous characters but
// the words: `on`, `off`, `yes` and `no` are booleans in YAML 1.1 and strings in
// YAML 1.2, so an unquoted round trip means whichever reader opens the file
// decides what the author wrote. One already did — a script rewriting these
// expectations turned `[on, off]` into `[true, false]`.
func TestQuotingRemovesAmbiguityRatherThanAnsweringIt(t *testing.T) {
	quoted := []string{
		// Booleans and null under YAML 1.1, YAML 1.2, or a case variant of
		// either. All of them, because a check that knows three spellings is a
		// check the next writer routes around without meaning to.
		"true", "false", "yes", "no", "on", "off", "null", "~",
		"True", "False", "Yes", "No", "On", "Off", "Null",
		"TRUE", "FALSE", "YES", "NO", "ON", "OFF", "NULL",
		// Text that would read back as a number.
		"1500", "-3", "+7", "0", "1.5", "1e6", "0x10", "0o17", "017", "1_000",
		".inf", ".NaN", "-.inf", "1:30",
		// Empty, and text whose first or last character changes how the line
		// parses.
		"", " leading", "trailing ",
		"-dash", "?query", ":colon", ",comma", "[bracket", "]bracket",
		"{brace", "}brace", "#hash", "&anchor", "*alias", "!tag",
		"|literal", ">folded", "'quote", "\"double", "%directive", "@at", "`tick",
		// Text carrying a structural sequence inside it.
		"key: value", "text # comment", "two\nlines",
	}
	for _, s := range quoted {
		got := emitScalar(s)
		if !strings.HasPrefix(got, "'") {
			t.Errorf("emitScalar(%q) = %s, want it quoted", s, got)
		}
	}

	plain := []string{
		"scalar", "integer", "config", "allow", "10.0.0.1/24",
		"$network-object", "$.access.read", "not-a-primitive", "OU=operators,O=acme",
		"a:b", "a#b", "yes-ish", "ontology", "nulled",
	}
	for _, s := range plain {
		got := emitScalar(s)
		if strings.HasPrefix(got, "'") {
			t.Errorf("emitScalar(%q) = %s, want it plain", s, got)
		}
	}

	// A `'` inside the text does NOT by itself require quoting: a plain scalar
	// may contain one, and quoting on sight would make the rule harder to
	// reproduce rather than safer. When something else triggers quoting, the
	// embedded quote is doubled so the result reads back as itself.
	if got := emitScalar("it's"); got != "it's" {
		t.Errorf("emitScalar(it's) = %s, want it plain", got)
	}
	if got := emitScalar("'quoted'"); got != `'''quoted'''` {
		t.Errorf("emitScalar('quoted') = %s, want '''quoted'''", got)
	}
}

// TestNumbersKeepTheirType — §8.8.1: an integer has no decimal point, a float
// always carries a marker.
//
// A float rendered as `3` would read back as an integer, which changes the type
// of a value that passed validation as a float.
func TestNumbersKeepTheirType(t *testing.T) {
	for value, want := range map[any]string{
		9000:          "9000",
		int64(-1):     "-1",
		0:             "0",
		float64(3):    "3.0",
		float64(1.5):  "1.5",
		float64(-0.5): "-0.5",
		float64(1e21): "1e+21",
		true:          "true",
		false:         "false",
		nil:           "null",
	} {
		if got := emitScalar(value); got != want {
			t.Errorf("emitScalar(%#v) = %s, want %s", value, got, want)
		}
	}

	// Anything the projection did not expect still serializes rather than
	// panicking: a serializer that dies on a live object is worse than one that
	// is wrong about it.
	if got := emitScalar(struct{ A int }{1}); got == "" {
		t.Error("emitScalar produced nothing for an unexpected type")
	}
}

// TestEmptyCollectionsUseFlowForm — block style has no way to write "nothing
// here", so §8.8.1 fixes `{}` and `[]`.
func TestEmptyCollectionsUseFlowForm(t *testing.T) {
	node := newOrderedMap()
	node.set("values", newOrderedMap())
	node.set("origin", []any{"yaml"})
	if got := string(emitCanonical(node)); !strings.Contains(got, "values: {}") {
		t.Errorf("an empty mapping payload is not written as {}:\n%s", got)
	}

	node = newOrderedMap()
	node.set("values", []any{})
	node.set("origin", []any{"yaml"})
	if got := string(emitCanonical(node)); !strings.Contains(got, "values: []") {
		t.Errorf("an empty sequence payload is not written as []:\n%s", got)
	}
}

// TestOriginIsWrittenInline — all four productions of §5.2 on one line each.
func TestOriginIsWrittenInline(t *testing.T) {
	sealed := newOrderedMap()
	inner := newOrderedMap()
	inner.set("template", "$t")
	inner.set("path", "$.a")
	sealed.set("sealed", inner)

	for _, tc := range []struct {
		origin []any
		want   string
	}{
		{[]any{"yaml"}, "[yaml]"},
		{[]any{"schema"}, "[schema]"},
		{[]any{sealed}, "[{sealed: {template: $t, path: $.a}}]"},
		{[]any{sealed, "schema"}, "[{sealed: {template: $t, path: $.a}}, schema]"},
	} {
		if got := emitOriginInline(tc.origin); got != tc.want {
			t.Errorf("emitOriginInline = %s, want %s", got, tc.want)
		}
	}

	// A malformed origin still serializes: this runs after final validation
	// rejected anything the grammar does not produce, so reaching here means
	// something upstream is wrong and the serializer should not compound it by
	// panicking.
	if got := emitOriginInline("not a sequence"); got != "[]" {
		t.Errorf("emitOriginInline on a non-sequence = %s, want []", got)
	}
	if got := emitOriginInline([]any{newOrderedMap()}); got == "" {
		t.Error("emitOriginInline produced nothing for a constructor-shaped term")
	}
}

// TestSequenceEntriesOpenOnTheDashLine — §8.8.1: the entry's first member sits
// on the `- ` line and the rest align under it.
func TestSequenceEntriesOpenOnTheDashLine(t *testing.T) {
	entry := newOrderedMap()
	entry.set("values", 1)
	entry.set("origin", []any{"yaml"})

	root := newOrderedMap()
	root.set("values", []any{entry})
	root.set("origin", []any{"yaml"})

	got := string(emitCanonical(root))
	if !strings.Contains(got, "  - values: 1") {
		t.Errorf("a sequence entry did not open on the dash line:\n%s", got)
	}
	// And a sequence of plain values, which the projection produces for a list
	// inside a primitive payload (INV-035).
	root = newOrderedMap()
	root.set("values", []any{"a", "b"})
	root.set("origin", []any{"schema"})
	got = string(emitCanonical(root))
	if !strings.Contains(got, "- a") || !strings.Contains(got, "- b") {
		t.Errorf("a raw sequence lost its entries:\n%s", got)
	}
}

// TestDocumentShape — the frame every canonical object carries.
func TestDocumentShape(t *testing.T) {
	node := newOrderedMap()
	node.set("values", 1)
	node.set("origin", []any{"yaml"})

	got := string(emitCanonical(node))
	if !strings.HasPrefix(got, "---\n") {
		t.Errorf("the document does not begin with ---:\n%s", got)
	}
	if !strings.HasSuffix(got, "\n") {
		t.Errorf("the document does not end with a newline:\n%q", got)
	}
	if strings.Contains(got, "\r") {
		t.Error("the document carries a carriage return")
	}

	// A value the projection left as a leaf where a node was expected still
	// serializes.
	if got := string(emitCanonical("bare")); !strings.Contains(got, "bare") {
		t.Errorf("a non-node document lost its value: %q", got)
	}
}

// TestIsNodeDistinguishesDataFromNodes — the one decision the emitter makes
// while walking, and the one that keeps `values` readable as a domain key
// inside an opaque payload (INV-011).
func TestIsNodeDistinguishesDataFromNodes(t *testing.T) {
	node := newOrderedMap()
	node.set("values", 1)
	node.set("origin", []any{"yaml"})
	if !isNode(node) {
		t.Error("a node carrying values and origin was not recognised")
	}

	// A mapping with a `values` key and nothing else is domain data — exactly
	// the false positive materialization/012 exists to pin.
	data := newOrderedMap()
	data.set("values", []any{"on", "off"})
	data.set("default", false)
	if isNode(data) {
		t.Error("an opaque payload with a `values` key was mistaken for a node")
	}
	if isNode("scalar") || isNode([]any{1}) {
		t.Error("a non-mapping was mistaken for a node")
	}
}
