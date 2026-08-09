package module_test

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CentralInfraCore/cic-object-model/go/module"
	"github.com/CentralInfraCore/cic-object-model/go/objectmodel"
)

// INV-032 is the only invariant in the specification that asserts an
// impossibility: a value of the module-input type must not be constructible
// outside the materializer. Everything else says what must be true of an
// object; this says a whole class of objects must not exist.
//
// An impossibility deserves to be attacked rather than asserted. go/inv032
// proves the direct construction does not compile. This file tries the ways
// around it that still compile, and records what each one achieves — including
// the ones that get further than they should.

// forgedByEmbedding is the interesting attack. Go promotes an embedded
// interface's method set, INCLUDING its unexported methods, so this type
// satisfies objectmodel.CanonicalObject in another package — which is exactly
// what the unexported marker method was supposed to prevent.
//
// The embedded value is nil, so calling any method panics. That makes it a
// forged *type*, not a forged *object*: it crosses the type system but not a
// single method call.
type forgedByEmbedding struct {
	objectmodel.CanonicalObject
}

// forgedWithPayload goes further: it embeds the interface to satisfy the type
// and overrides every method with attacker-chosen answers. Nothing panics, and
// nothing about it came from the materializer.
type forgedWithPayload struct {
	objectmodel.CanonicalObject
}

func (forgedWithPayload) ModelVersion() string { return "0.2" }
func (forgedWithPayload) CanonicalYAML() []byte {
	return []byte("values:\n  mtu:\n    values: 9000\n    origin: [yaml, schema]\norigin: [yaml]\n")
}
func (forgedWithPayload) Root() *objectmodel.Node { return nil }

// TestForgeryByEmbedding is the finding. It does not assert that the boundary
// holds — it asserts what actually happens, so the answer is on the record
// either way.
func TestForgeryByEmbedding(t *testing.T) {
	t.Run("empty embedding satisfies the type", func(t *testing.T) {
		// This compiling at all is the point: the unexported marker method does
		// not stop another package from satisfying the interface by embedding
		// it. INV-032 says such a value must not be constructible.
		// Not compared against nil: staticcheck rightly points out that a
		// concrete struct value never is. That it satisfies the interface at
		// all is the finding — the unexported marker method did not stop it.
		var obj objectmodel.CanonicalObject = forgedByEmbedding{}

		// It reaches the boundary. Whether the boundary survives it is the
		// question — a panic here would be a denial of service reachable from
		// a module author, which is worse than a rejection.
		err := mustNotPanic(t, func() error { return module.Execute(obj) })
		if err == nil {
			t.Error("INV-032: a forged object was accepted by the boundary")
		} else {
			t.Logf("boundary rejected the empty forgery: %v", err)
		}
	})

	t.Run("populated embedding answers every call", func(t *testing.T) {
		// Nothing here panics: every method is overridden. The object is
		// internally invalid — its origin holds both yaml and schema, which
		// INV-017 forbids — and no materializer ever saw it.
		var obj objectmodel.CanonicalObject = forgedWithPayload{}

		err := mustNotPanic(t, func() error { return module.Execute(obj) })
		if err == nil {
			t.Error("INV-032: a fully forged object with an invalid origin was " +
				"accepted by the boundary")
		} else {
			t.Logf("boundary rejected the populated forgery: %v", err)
		}
	})
}

// forgedMixed is the sharpest forgery available without unsafe: a REAL node
// tree, taken from a legitimate materialization, paired with canonical bytes
// the attacker chose. Every runtime defence the boundary has — a non-nil Root,
// a truthful-looking version — is satisfied, and the bytes a consumer reads
// are not the ones the materializer validated.
type forgedMixed struct {
	objectmodel.CanonicalObject
	realRoot *objectmodel.Node
}

func (forgedMixed) ModelVersion() string { return "0.2" }
func (forgedMixed) CanonicalYAML() []byte {
	// Origin holds both yaml and schema, which INV-017 forbids. No materializer
	// would ever emit this.
	return []byte("values:\n  mtu:\n    values: 66666\n    origin: [yaml, schema]\norigin: [yaml]\n")
}
func (f forgedMixed) Root() *objectmodel.Node { return f.realRoot }

// TestForgeryWithRealNodeTree is the attack that satisfies every runtime check.
func TestForgeryWithRealNodeTree(t *testing.T) {
	legit := mustMaterialize(t,
		[]byte("model: \"0.2\"\nroot:\n  shape: object\n  children:\n    mtu:\n      shape: scalar\n      scalar_type: integer\n      default: 1500\n"),
		[]byte("{}\n"))

	var obj objectmodel.CanonicalObject = forgedMixed{realRoot: legit.Root()}

	err := mustNotPanic(t, func() error { return module.Execute(obj) })
	if err != nil {
		t.Logf("boundary rejected the mixed forgery: %v", err)
		return
	}
	// It got through. Record precisely what a consumer would now believe.
	t.Errorf("INV-032: a forged object crossed the boundary.\n"+
		"  Root() is a real node tree from a real materialization\n"+
		"  CanonicalYAML() is attacker-chosen and violates INV-017:\n%s",
		obj.CanonicalYAML())
}

// TestNilInterfaceIsRejected covers the hole Go leaves and the specification
// already admits (docs/spec-defects.md SD-016): a nil interface value cannot be
// forbidden by the type system, so the boundary must reject it at runtime.
func TestNilInterfaceIsRejected(t *testing.T) {
	if err := module.Execute(nil); !errors.Is(err, module.ErrNilObject) {
		t.Errorf("nil crossed the boundary: %v", err)
	}
	// A typed nil is the sharper version: the interface is non-nil, the value
	// inside it is nil.
	var typed objectmodel.CanonicalObject = forgedByEmbedding{}
	if err := mustNotPanic(t, func() error { return module.Execute(typed) }); err == nil {
		t.Error("a typed nil crossed the boundary")
	}
}

// TestResourceExhaustion probes the shape the fuzzer hinted at: its execution
// rate fell to zero for twenty-second stretches, which means some inputs are
// very slow. For a library that parses YAML it did not write, that is a
// denial-of-service surface, not a performance curiosity.
func TestResourceExhaustion(t *testing.T) {
	t.Run("deeply nested authoring input", func(t *testing.T) {
		// 10k levels of nesting against a schema that declares none of it. The
		// input is undeclared, so the correct answer is a fast rejection.
		var b strings.Builder
		for i := 0; i < 10000; i++ {
			b.WriteString("a:\n")
			b.WriteString(strings.Repeat(" ", (i+1)*2))
		}
		schema := []byte("model: \"0.2\"\nroot:\n  shape: object\n  children:\n    a:\n      shape: scalar\n      scalar_type: string\n")

		done := make(chan struct{})
		go func() {
			defer close(done)
			_, _ = objectmodel.Materialize(schema, []byte(b.String()))
		}()
		// Budget: 30s. Measured 0.75s plain and ~11s under -race, which is the
		// race detector's usual order-of-magnitude tax rather than a finding.
		//
		// The finding is the 0.75s. This input is INVALID — the schema declares
		// none of that nesting — so the correct behaviour is a fast rejection,
		// and three quarters of a second to say no is a lot. Combined with the
		// fuzzer's 0/sec stretches it points at superlinear work on deep input,
		// which is a denial-of-service shape for a library that parses YAML it
		// did not write. Recorded, not yet chased.
		select {
		case <-done:
		case <-timeAfterSeconds(30):
			t.Fatal("materialization of deeply nested input did not finish in 30s — " +
				"a caller can hang the library with input it did not write")
		}
	})

	t.Run("yaml alias expansion", func(t *testing.T) {
		// The classic billion-laughs shape. gopkg.in/yaml.v3 has its own limits;
		// this records whether they hold at this boundary rather than assuming.
		bomb := "a: &a [x,x,x,x,x,x,x,x,x]\nb: &b [*a,*a,*a,*a,*a,*a,*a,*a,*a]\n" +
			"c: &c [*b,*b,*b,*b,*b,*b,*b,*b,*b]\nd: &d [*c,*c,*c,*c,*c,*c,*c,*c,*c]\n" +
			"e: [*d,*d,*d,*d,*d,*d,*d,*d,*d]\n"
		schema := []byte("model: \"0.2\"\nroot:\n  shape: object\n  children:\n    a:\n      shape: scalar\n      scalar_type: string\n")

		done := make(chan struct{})
		go func() {
			defer close(done)
			_, _ = objectmodel.Materialize(schema, []byte(bomb))
		}()
		select {
		case <-done:
		case <-timeAfterSeconds(30):
			t.Fatal("alias expansion did not finish in 30s")
		}
	})
}

// timeAfterSeconds keeps the select statements above readable.
func timeAfterSeconds(n int) <-chan time.Time {
	return time.After(time.Duration(n) * time.Second)
}

// mustNotPanic runs f and turns a panic into a test failure rather than a
// crashed test binary. A panic reachable from a module author is a denial of
// service, so it is reported as its own kind of finding.
func mustNotPanic(t *testing.T, f func() error) (err error) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("the boundary panicked instead of rejecting: %v", r)
			err = errors.New("panic")
		}
	}()
	return f()
}

// forgedSplitView is audit finding F-02, built rather than argued about.
//
// It carries a REAL node tree, taken from a legitimate materialization, and a
// DIFFERENT byte string that is itself a perfectly valid canonical object. Every
// check the boundary had before this: the tree is real, so Root() is non-nil and
// its version is right; the bytes validate, because they are a valid object.
// Only the pairing is a lie.
//
// This is the forgery the earlier adversarial tests did not build. They paired a
// real tree with INVALID bytes, which the validation check catches, and stopping
// there made the boundary look stronger than it was: "the bytes validate" was
// being read as "the bytes are this object's".
type forgedSplitView struct {
	objectmodel.CanonicalObject
	realRoot  *objectmodel.Node
	otherBody []byte
}

func (f forgedSplitView) ModelVersion() string    { return module.ModelVersion }
func (f forgedSplitView) Root() *objectmodel.Node { return f.realRoot }
func (f forgedSplitView) CanonicalYAML() []byte   { return f.otherBody }

// TestSplitViewForgeryIsRefused — the two views must describe the same object.
func TestSplitViewForgeryIsRefused(t *testing.T) {
	schema := []byte("model: \"0.2\"\nroot:\n  shape: object\n  children:\n    mtu:\n      shape: scalar\n      scalar_type: integer\n      default: 1500\n")

	// Two legitimate objects. Everything about each of them is real.
	nine := mustMaterialize(t, schema, []byte("mtu: 9000\n"))
	fifteen := mustMaterialize(t, schema, []byte("{}\n"))

	// Sanity: they are genuinely different objects, so the test is not passing
	// because the two happen to serialize alike.
	if bytes.Equal(nine.CanonicalYAML(), fifteen.CanonicalYAML()) {
		t.Fatal("the two fixtures are the same object; the forgery would be a no-op")
	}
	// And the bytes we are about to smuggle are valid on their own — the
	// boundary's validation check has nothing to object to.
	if err := objectmodel.ValidateCanonicalDocument(fifteen.CanonicalYAML()); err != nil {
		t.Fatalf("the substituted bytes are not a valid object: %v", err)
	}

	forged := forgedSplitView{realRoot: nine.Root(), otherBody: fifteen.CanonicalYAML()}

	err := module.Execute(forged)
	if err == nil {
		t.Fatal("the boundary accepted an object whose tree and bytes describe different objects")
	}
	if !errors.Is(err, module.ErrObjectNotBound) {
		t.Errorf("rejected for the wrong reason: %v", err)
	}

	// The negative control: the same forgery TYPE, carrying the bytes that do
	// belong to the tree, gets through. The check is rejecting the mismatch,
	// not the shape of the value.
	honest := forgedSplitView{realRoot: nine.Root(), otherBody: nine.CanonicalYAML()}
	if err := module.Execute(honest); err != nil {
		t.Errorf("a value whose tree and bytes agree was refused: %v", err)
	}
}

// TestCanonicalizeIsTheInverseTheBindingNeeds — the property the check rests on.
//
// Re-serializing a tree must reproduce the object's own bytes exactly. Before
// §8.8.1 it could not: the serialization was undefined, so a re-serialization
// differing from the original was nobody's fault and the binding check above
// would have rejected every honest object.
func TestCanonicalizeIsTheInverseTheBindingNeeds(t *testing.T) {
	for _, vector := range []string{
		"materialization/001_origin_yaml",
		"materialization/006_closure_opaque",
		"materialization/008_normalize_list",
		"materialization/013_access_inherit_injection",
	} {
		dir := filepath.Join("../../conformance", vector)
		read := func(n string) []byte {
			b, err := os.ReadFile(filepath.Join(dir, n))
			if err != nil {
				t.Fatalf("%s: %v", vector, err)
			}
			return b
		}
		obj := mustMaterialize(t, read("schema.yaml"), read("input.yaml"))
		if !bytes.Equal(objectmodel.Canonicalize(obj.Root()), obj.CanonicalYAML()) {
			t.Errorf("%s: re-serializing the tree does not reproduce the object's bytes\n--- again ---\n%s\n--- object ---\n%s",
				vector, objectmodel.Canonicalize(obj.Root()), obj.CanonicalYAML())
		}
	}
	// A nil tree has no bytes, rather than a panic at a trust boundary.
	if objectmodel.Canonicalize(nil) != nil {
		t.Error("Canonicalize(nil) invented an object")
	}
}

// countingForgery answers honestly for a fixed number of calls and then lies.
//
// It is the audit's F-07 and it is the sharpest kind of finding: it does not
// break any single check, it breaks the ASSUMPTION that a check and the use of
// what it checked see the same value. The previous boundary read CanonicalYAML
// twice, so `honest = 2` passed validation, passed the tree/bytes binding, and
// left the third read — the one a real module would make — unprotected.
type countingForgery struct {
	objectmodel.CanonicalObject
	root       *objectmodel.Node
	good, evil []byte
	honest     int

	versionCalls, rootCalls, yamlCalls int
}

func (f *countingForgery) ModelVersion() string {
	f.versionCalls++
	return module.ModelVersion
}

func (f *countingForgery) Root() *objectmodel.Node {
	f.rootCalls++
	return f.root
}

func (f *countingForgery) CanonicalYAML() []byte {
	f.yamlCalls++
	if f.yamlCalls <= f.honest {
		return f.good
	}
	return f.evil
}

func twoObjects(t *testing.T) (objectmodel.CanonicalObject, objectmodel.CanonicalObject) {
	t.Helper()
	schema := []byte("model: \"0.2\"\nroot:\n  shape: object\n  children:\n    mtu:\n      shape: scalar\n      scalar_type: integer\n      default: 1500\n")
	return mustMaterialize(t, schema, []byte("mtu: 9000\n")),
		mustMaterialize(t, schema, []byte("{}\n"))
}

// TestBoundaryReadsEachMethodExactlyOnce is the fix, stated as the property
// rather than as the absence of one attack.
//
// A count of one cannot be beaten by a stateful value: there is no later call
// to answer differently. Any future check added to the boundary that reads
// again reopens the hole, and this test fails when it does — which is the point
// of asserting the count instead of asserting that this particular forgery
// fails.
func TestBoundaryReadsEachMethodExactlyOnce(t *testing.T) {
	honest, other := twoObjects(t)
	f := &countingForgery{
		root:   honest.Root(),
		good:   objectmodel.Canonicalize(honest.Root()),
		evil:   other.CanonicalYAML(),
		honest: 1000, // always honest: this test is about counts, not lying
	}
	if err := module.Execute(f); err != nil {
		t.Fatalf("an honest value was refused: %v", err)
	}
	for name, got := range map[string]int{
		"ModelVersion":  f.versionCalls,
		"Root":          f.rootCalls,
		"CanonicalYAML": f.yamlCalls,
	} {
		if got != 1 {
			t.Errorf("%s was read %d times, want exactly 1 — a second read is a "+
				"second chance for a stateful value to answer differently", name, got)
		}
	}
}

// TestStatefulForgeryCannotOutlastTheChecks — the attack itself, at every count
// of honest answers a forger might choose.
func TestStatefulForgeryCannotOutlastTheChecks(t *testing.T) {
	for _, honest := range []int{0, 1, 2, 3, 5} {
		honest := honest
		t.Run(fmt.Sprintf("honest_for_%d_calls", honest), func(t *testing.T) {
			hon, other := twoObjects(t)
			f := &countingForgery{
				root:   hon.Root(),
				good:   objectmodel.Canonicalize(hon.Root()),
				evil:   other.CanonicalYAML(),
				honest: honest,
			}
			err := module.Execute(f)

			// With one read, the only question is what that read returned.
			// honest == 0 means the boundary saw the evil bytes and must
			// refuse; anything else means it saw the good ones and accepts.
			if honest == 0 {
				if err == nil {
					t.Fatal("the boundary accepted an object whose bytes are not its tree's")
				}
				if !errors.Is(err, module.ErrObjectNotBound) {
					t.Errorf("refused for the wrong reason: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("an object that answered honestly was refused: %v", err)
			}
			// And the crucial part: whatever the forgery would say next, the
			// module is not holding it.
			if f.yamlCalls != 1 {
				t.Errorf("the boundary read the bytes %d times", f.yamlCalls)
			}
		})
	}
}

// TestDeliveredIsIndependentOfTheValueThatCrossed — a module works from the
// snapshot, and rewriting the caller's buffer afterwards cannot reach it.
//
// Without the copy, the boundary would validate bytes and then hand the module
// the same backing array, so the validation would be a statement about bytes
// that no longer exist by the time anything reads them.
func TestDeliveredIsIndependentOfTheValueThatCrossed(t *testing.T) {
	honest, _ := twoObjects(t)

	original := objectmodel.Canonicalize(honest.Root())
	shared := make([]byte, len(original))
	copy(shared, original)

	f := &countingForgery{root: honest.Root(), good: shared, evil: shared, honest: 1000}
	delivered, err := module.Accept(f)
	if err != nil {
		t.Fatalf("an honest value was refused: %v", err)
	}

	// The caller rewrites the slice it still holds.
	for i := range shared {
		shared[i] = 'x'
	}

	if !bytes.Equal(delivered.CanonicalYAML(), original) {
		t.Error("rewriting the caller's buffer changed what was delivered")
	}
	if err := objectmodel.ValidateCanonicalDocument(delivered.CanonicalYAML()); err != nil {
		t.Errorf("the delivered bytes stopped being a valid object: %v", err)
	}

	// And the snapshot handed out is itself a copy, so a module cannot corrupt
	// it for the next reader either.
	first := delivered.CanonicalYAML()
	for i := range first {
		first[i] = 'y'
	}
	if !bytes.Equal(delivered.CanonicalYAML(), original) {
		t.Error("a module writing to the bytes it was given changed the delivery")
	}
}
