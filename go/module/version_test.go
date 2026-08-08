package module_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/CentralInfraCore/cic-object-model/go/module"
	"github.com/CentralInfraCore/cic-object-model/go/objectmodel"
)

// The model version is declared in seven places and was, until this file
// existed, believed in seven places independently.
//
// The 0.2 revision was written into SPEC.md and propagated nowhere else. The
// schema index, the project descriptor, both Go constants and all twenty vector
// schemas stayed at 0.1, so 0.2 semantics were implemented, materialized and
// tested under a 0.1 label — for twenty-six commits, through a full CI suite,
// past a release. Nothing caught it because nothing compared the declarations
// to each other: every gate checked its own file against itself.
//
// A propagation failure is not prevented by propagating carefully. It is
// prevented by a check that fails when the declarations disagree, which is what
// this is. SPEC.md is the authority; everything else must match it.

const repoRoot = "../.."

// specVersion reads the version SPEC.md declares. That document is normative
// (doc.go: "where this package and that document disagree, the document is
// right"), so it is the source and not one more opinion.
func specVersion(t *testing.T) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(repoRoot, "SPEC.md"))
	if err != nil {
		t.Fatalf("reading SPEC.md: %v", err)
	}
	m := regexp.MustCompile(`(?m)^\*\*Model version: ([^*]+)\*\*`).FindSubmatch(body)
	if m == nil {
		t.Fatal("SPEC.md declares no model version — the authority is missing, " +
			"so nothing below can be checked against it")
	}
	return strings.TrimSpace(string(m[1]))
}

// TestVersionIdentity — every declaration in the repository names the version
// SPEC.md names.
func TestVersionIdentity(t *testing.T) {
	want := specVersion(t)

	t.Run("the Go constants", func(t *testing.T) {
		// Two separate constants on purpose. INV-034 says a MODULE declares the
		// version it consumes, which is a claim independent of what the library
		// implements — a third-party module pins its own literal and a host
		// refuses the mismatch. Independent claims that must agree today are
		// exactly what needs an assertion rather than a shared symbol, because
		// collapsing them into one would make INV-034 unfalsifiable.
		if objectmodel.ModelVersion != want {
			t.Errorf("objectmodel.ModelVersion = %q, SPEC.md says %q",
				objectmodel.ModelVersion, want)
		}
		if module.ModelVersion != want {
			t.Errorf("module.ModelVersion = %q, SPEC.md says %q",
				module.ModelVersion, want)
		}
	})

	t.Run("the descriptors", func(t *testing.T) {
		// Matched textually rather than by unmarshalling into a struct: a typed
		// reader silently tolerates a renamed or missing key, and a version
		// declaration that has quietly disappeared is the same failure as one
		// that disagrees.
		for _, tc := range []struct{ file, pattern string }{
			{"spec/index.yaml", `(?m)^  version: "([^"]+)"`},
			{"spec/index.yaml", `(?m)^  model_version: "([^"]+)"`},
			// project.yaml carries a semantic version; only its first two
			// components track the model.
			{"project.yaml", `(?m)^  version: ([0-9]+\.[0-9]+)\.[0-9]+`},
		} {
			body, err := os.ReadFile(filepath.Join(repoRoot, tc.file))
			if err != nil {
				t.Fatalf("reading %s: %v", tc.file, err)
			}
			m := regexp.MustCompile(tc.pattern).FindSubmatch(body)
			if m == nil {
				t.Errorf("%s: no version matched %s — the declaration moved or "+
					"was removed", tc.file, tc.pattern)
				continue
			}
			if got := string(m[1]); got != want {
				t.Errorf("%s: version %q, SPEC.md says %q", tc.file, got, want)
			}
		}
	})

	t.Run("every conformance vector", func(t *testing.T) {
		// The vectors are the implementation-independent statement of what the
		// model does. A vector at the wrong version tests 0.2 behaviour and
		// claims it for 0.1, which is precisely the state this file was written
		// after finding.
		var checked int
		for _, group := range []string{"materialization", "invalid", "validation"} {
			dir := filepath.Join(repoRoot, "conformance", group)
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatalf("reading %s: %v", group, err)
			}
			for _, e := range entries {
				if !e.IsDir() {
					continue
				}
				path := filepath.Join(dir, e.Name(), "schema.yaml")
				body, err := os.ReadFile(path)
				if os.IsNotExist(err) {
					continue // validation vectors carry an object, not a schema
				}
				if err != nil {
					t.Fatalf("reading %s: %v", path, err)
				}
				m := regexp.MustCompile(`(?m)^model: "([^"]+)"`).FindSubmatch(body)
				if m == nil {
					t.Errorf("%s/%s: schema declares no model version", group, e.Name())
					continue
				}
				checked++
				if got := string(m[1]); got != want {
					t.Errorf("%s/%s: model %q, SPEC.md says %q", group, e.Name(), got, want)
				}
			}
		}
		if checked == 0 {
			t.Fatal("no vector schema was checked — the corpus path is wrong, and " +
				"a check that inspects nothing passes for the wrong reason")
		}
	})
}

// TestUnsupportedModelVersionIsRefused — the enforcement half. Before this,
// LoadSchema ran fmt.Sprint over whatever the key held and accepted it, so a
// schema could declare any version at all, including none.
func TestUnsupportedModelVersionIsRefused(t *testing.T) {
	body := "root:\n  shape: object\n  children:\n    mtu:\n      shape: scalar\n      scalar_type: integer\n"

	for name, header := range map[string]string{
		"a version from the future": "model: \"9.9\"\n",
		"the superseded 0.1":        "model: \"0.1\"\n",
		"not a version at all":      "model: \"banana\"\n",
		"no declaration":            "",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := objectmodel.LoadSchema([]byte(header + body))
			if err == nil {
				t.Fatal("accepted a schema at an unusable model version")
			}
			e, ok := err.(*objectmodel.Error)
			if !ok {
				t.Fatalf("rejection is %T, want *objectmodel.Error", err)
			}
			if e.Code != objectmodel.CodeUnsupportedModelVersion {
				t.Errorf("code = %q, want %q", e.Code, objectmodel.CodeUnsupportedModelVersion)
			}
			if e.Invariant != "INV-033" {
				t.Errorf("invariant = %q, want INV-033", e.Invariant)
			}
		})
	}

	// And the supported one still loads, so the check above is not passing
	// because everything is refused.
	if _, err := objectmodel.LoadSchema([]byte("model: \"" + objectmodel.ModelVersion + "\"\n" + body)); err != nil {
		t.Fatalf("the supported version was rejected: %v", err)
	}
}
