package objectmodel

import (
	"fmt"
	"sort"
)

// orderedMap is a mapping with an explicit key order.
//
// SPEC INV-030 requires a byte-identical canonical object for the same schema
// and input, but SPEC §8.8 never defines the canonical key order (or indent,
// or scalar style). This implementation defines one — node members in the
// order values, origin, then primitives in SPEC §6.1 declaration order;
// payload mappings sorted — and records the gap as docs/spec-defects.md
// SD-010.
type orderedMap struct {
	keys []string
	vals map[string]any
}

func newOrderedMap() *orderedMap {
	return &orderedMap{vals: map[string]any{}}
}

func (m *orderedMap) set(k string, v any) {
	if _, ok := m.vals[k]; !ok {
		m.keys = append(m.keys, k)
	}
	m.vals[k] = v
}

func (m *orderedMap) get(k string) (any, bool) {
	v, ok := m.vals[k]
	return v, ok
}

func (m *orderedMap) has(k string) bool {
	_, ok := m.vals[k]
	return ok
}

// asMap accepts every shape a mapping arrives in: the two gopkg.in/yaml.v3 can
// produce, and the ordered form fromNode builds.
func asMap(v any) (map[string]any, bool) {
	switch t := v.(type) {
	case *orderedMap:
		return t.vals, true
	case map[string]any:
		return t, true
	case map[any]any:
		out := make(map[string]any, len(t))
		for k, vv := range t {
			out[fmt.Sprint(k)] = vv
		}
		return out, true
	}
	return nil, false
}

// keysInOrder returns the keys of a mapping in the order INV-044 requires: the
// order they were written, when that is still known.
//
// A plain map has lost it — Go map iteration is randomised, so sorting is the
// only way to be deterministic at all, and every caller that reaches this
// branch is working on a value that came from somewhere order was already gone.
// The ordered branch is the one that matters, and it exists because sorting
// used to be the ONLY branch.
func keysInOrder(v any) []string {
	if om, ok := v.(*orderedMap); ok {
		out := make([]string, len(om.keys))
		copy(out, om.keys)
		return out
	}
	if m, ok := asMap(v); ok {
		return sortedKeys(m)
	}
	return nil
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// normalize converts a decoded YAML value into the orderedMap form the
// canonicaliser and the final validator both work on, KEEPING the order it
// arrived in (INV-044). It used to sort, which is what reordered opaque
// payloads.
func normalize(v any) any {
	if m, ok := asMap(v); ok {
		om := newOrderedMap()
		for _, k := range keysInOrder(v) {
			om.set(k, normalize(m[k]))
		}
		return om
	}
	if l, ok := v.([]any); ok {
		out := make([]any, len(l))
		for i, e := range l {
			out[i] = normalize(e)
		}
		return out
	}
	return v
}

// encodeCanonical is the serializer half of SPEC §8.8.
// deepCopy protects an opaque payload (SPEC INV-028: preserved verbatim) from
// aliasing the caller's input tree.
func deepCopy(v any) any {
	if m, ok := asMap(v); ok {
		// Ordered, because "verbatim" includes the order the author wrote
		// (INV-044). Copying into a plain map preserved the values and lost
		// exactly the property this function exists to protect.
		out := newOrderedMap()
		for _, k := range keysInOrder(v) {
			out.set(k, deepCopy(m[k]))
		}
		return out
	}
	if l, ok := v.([]any); ok {
		out := make([]any, len(l))
		for i, e := range l {
			out[i] = deepCopy(e)
		}
		return out
	}
	return v
}
