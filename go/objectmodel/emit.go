package objectmodel

import (
	"fmt"
	"strconv"
	"strings"
)

// The canonical serializer of SPEC §8.8.1.
//
// Hand-written rather than delegated to gopkg.in/yaml.v3's encoder, and that is
// the point rather than an oversight. A library emits its own house style, and
// the house style is what made the two implementations of this document produce
// zero byte-identical objects across thirteen vectors while agreeing on every
// one of them semantically. §8.8.1 now says what the bytes are; a library that
// happens to agree today is not an implementation of that.
//
// The rules, in the order §8.8.1 states them: UTF-8, no BOM, `\n` endings
// including the last; a leading `---`; block style at two spaces per level;
// `origin` inline; sequence entries opening `- ` with their first member on the
// same line; `{}` and `[]` for empty collections; single quotes wherever plain
// would be ambiguous under YAML 1.1 OR 1.2; floats that keep a marker.

func emitCanonical(doc any) []byte {
	var b strings.Builder
	b.WriteString("---\n")
	emitNodeMembers(&b, doc, 0)
	return []byte(b.String())
}

// emitNodeMembers writes one node's members. `doc` is the projected form: an
// ordered mapping whose keys are `values`, `origin` and the primitives, already
// in the order INV-044 requires.
func emitNodeMembers(b *strings.Builder, doc any, depth int) {
	m, ok := doc.(*orderedMap)
	if !ok {
		// Not a node: a leaf the projection left as a value.
		emitValue(b, doc, depth)
		return
	}
	for _, k := range m.keys {
		v, _ := m.get(k)
		switch k {
		case "origin":
			writeIndent(b, depth)
			b.WriteString("origin: ")
			b.WriteString(emitOriginInline(v))
			b.WriteByte('\n')
		case "values":
			writeIndent(b, depth)
			b.WriteString("values:")
			emitValue(b, v, depth)
		default:
			writeIndent(b, depth)
			b.WriteString(emitKey(k))
			b.WriteString(":\n")
			emitNodeMembers(b, v, depth+1)
		}
	}
}

// emitValue writes a payload, which is either a nested structure of nodes or a
// value carried verbatim. Both are handled the same way here: a mapping whose
// entries carry `values` is a structure of nodes, anything else is data.
func emitValue(b *strings.Builder, v any, depth int) {
	switch t := v.(type) {
	case *orderedMap:
		if len(t.keys) == 0 {
			b.WriteString(" {}\n")
			return
		}
		b.WriteByte('\n')
		for _, k := range t.keys {
			child, _ := t.get(k)
			writeIndent(b, depth+1)
			b.WriteString(emitKey(k))
			if isNode(child) {
				b.WriteString(":\n")
				emitNodeMembers(b, child, depth+2)
			} else {
				b.WriteByte(':')
				emitValue(b, child, depth+1)
			}
		}
	case []any:
		if len(t) == 0 {
			b.WriteString(" []\n")
			return
		}
		b.WriteByte('\n')
		for _, e := range t {
			writeIndent(b, depth+1)
			// §8.8.1: the entry opens `- ` with its first member on that same
			// line, and the rest align under it.
			b.WriteString("- ")
			start := b.Len()
			if isNode(e) {
				emitNodeMembers(b, e, depth+2)
			} else {
				var inner strings.Builder
				emitValue(&inner, e, depth+1)
				b.WriteString(strings.TrimLeft(inner.String(), " "))
				continue
			}
			stripFirstIndent(b, start, depth+2)
		}
	default:
		b.WriteByte(' ')
		b.WriteString(emitScalar(v))
		b.WriteByte('\n')
	}
}

// isNode reports whether a projected value is a CIC node rather than data. A
// node always carries `values`; nothing below an opaque payload does, which is
// what keeps `values` as a domain key readable as data (INV-011).
func isNode(v any) bool {
	m, ok := v.(*orderedMap)
	if !ok {
		return false
	}
	return m.has("values") && m.has("origin")
}

// stripFirstIndent removes the indent from the first line written since start,
// so a sequence entry's first member sits on the dash line.
func stripFirstIndent(b *strings.Builder, start, depth int) {
	s := b.String()
	head, tail := s[:start], s[start:]
	pad := strings.Repeat("  ", depth)
	if strings.HasPrefix(tail, pad) {
		b.Reset()
		b.WriteString(head)
		b.WriteString(tail[len(pad):])
	}
}

// emitKey writes a mapping key under the same rule as a scalar value.
//
// Keys used to be written verbatim while the quoting rule was applied only to
// values, and the two are the same problem: a key a reader would take for
// something other than text changes what the document means. A child named
// `a: b` produced
//
//	values:
//	  a: b:
//	    values: x
//
// which is not the same mapping and is not YAML at all — PyYAML rejects it with
// "mapping values are not allowed here". The only producer of canonical objects
// could return bytes that are not a canonical object.
//
// Primitive names never need this, since they come from a fixed set of
// identifiers. Payload child names and opaque keys are whatever the schema or
// the author wrote.
func emitKey(k string) string { return quoteIfNeeded(k) }

func writeIndent(b *strings.Builder, depth int) {
	for i := 0; i < depth; i++ {
		b.WriteString("  ")
	}
}

// emitOriginInline writes the four productions of §5.2 on one line.
func emitOriginInline(v any) string {
	terms, ok := v.([]any)
	if !ok {
		return "[]"
	}
	parts := make([]string, 0, len(terms))
	for _, t := range terms {
		if m, ok := t.(*orderedMap); ok {
			if sv, ok := m.get("sealed"); ok {
				if sm, ok := sv.(*orderedMap); ok {
					tpl, _ := sm.get("template")
					pth, _ := sm.get("path")
					parts = append(parts, fmt.Sprintf("{sealed: {template: %s, path: %s}}",
						emitScalar(tpl), emitScalar(pth)))
					continue
				}
			}
		}
		parts = append(parts, emitScalar(t))
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func emitScalar(v any) string {
	switch t := v.(type) {
	case nil:
		return "null"
	case bool:
		return strconv.FormatBool(t)
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case float64:
		s := strconv.FormatFloat(t, 'g', -1, 64)
		// A float that renders without a marker would read back as an integer,
		// changing the type of the value on a round trip.
		if !strings.ContainsAny(s, ".eE") && !strings.Contains(s, "Inf") && !strings.Contains(s, "NaN") {
			s += ".0"
		}
		return s
	case string:
		return quoteIfNeeded(t)
	default:
		return quoteIfNeeded(fmt.Sprint(v))
	}
}

// reservedScalars are the strings a plain scalar cannot be: every spelling YAML
// 1.1 or YAML 1.2 reads as a boolean or null.
//
// `on` and `off` are not hypothetical. An opaque payload in this corpus holds
// the list `[on, off]`, and a script rewriting expectations round-tripped it to
// `[true, false]` by trusting one parser's reading of it.
var reservedScalars = map[string]bool{
	"true": true, "false": true, "yes": true, "no": true, "on": true, "off": true,
	"null": true, "~": true,
	"True": true, "False": true, "Yes": true, "No": true, "On": true, "Off": true, "Null": true,
	"TRUE": true, "FALSE": true, "YES": true, "NO": true, "ON": true, "OFF": true, "NULL": true,
}

func quoteIfNeeded(s string) string {
	needs := s == "" || reservedScalars[s] ||
		strings.HasSuffix(s, " ") ||
		strings.Contains(s, ": ") || strings.Contains(s, " #") ||
		strings.ContainsAny(s, "\n\r") ||
		looksNumeric(s)
	if !needs && strings.IndexAny(s[:1], " -?:,[]{}#&*!|>'\"%@`") == 0 {
		needs = true
	}
	if needs {
		return "'" + strings.ReplaceAll(s, "'", "''") + "'"
	}
	return s
}

// looksNumeric reports whether a reader could take this text for a number under
// EITHER YAML version — which is a wider set than Go's own parsers accept.
//
// `0x10` is the case that made this its own function: it is the string "0x10"
// to Go's base-10 ParseInt and the integer 16 to a YAML 1.1 reader, so quoting
// decided by ParseInt alone let it through. Octal, underscore digit groups and
// sexagesimal are the same shape of mistake.
func looksNumeric(s string) bool {
	t := strings.TrimPrefix(strings.TrimPrefix(s, "-"), "+")
	if t == "" {
		return false
	}
	switch strings.ToLower(t) {
	case ".inf", ".nan":
		return true
	}
	// Base-prefixed and underscore-separated forms. ParseInt with base 0 reads
	// 0x, 0o, 0b and a leading-zero octal, and tolerates `_`.
	if _, err := strconv.ParseInt(strings.ReplaceAll(t, "_", ""), 0, 64); err == nil {
		return true
	}
	if _, err := strconv.ParseFloat(strings.ReplaceAll(t, "_", ""), 64); err == nil {
		return true
	}
	// Sexagesimal: digit groups separated by colons, which YAML 1.1 reads as a
	// number of seconds.
	if strings.Contains(t, ":") {
		sexagesimal := true
		for _, part := range strings.Split(t, ":") {
			if part == "" {
				sexagesimal = false
				break
			}
			for _, r := range part {
				if r < '0' || r > '9' {
					sexagesimal = false
					break
				}
			}
			if !sexagesimal {
				break
			}
		}
		if sexagesimal {
			return true
		}
	}
	return false
}
