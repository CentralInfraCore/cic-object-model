package module_test

import (
	"errors"
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
