// Package module is the module boundary of SPEC §9.
//
// It contains no provisioning logic. It exists so that INV-031 and INV-032
// have a boundary to be true of: docs/spec-vector-map.md marks both
// unvectorizable and assigns them to the implementation as source-level
// checks, and a boundary that does not exist cannot be checked.
package module

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/CentralInfraCore/cic-object-model/go/objectmodel"
)

// ModelVersion is the object model version this module declares it consumes
// (SPEC INV-034). A host must not hand it an object of any other version.
const ModelVersion = "0.2"

// ErrNilObject is returned for a nil CanonicalObject, or for one whose methods
// cannot be called.
//
// INV-032 says the type is constructible only by the materializer. Two things
// defeat that in Go, and neither is closed by a type trick:
//
//   - the nil interface value, which the specification already admits (SD-016)
//   - interface EMBEDDING, which promotes the unexported marker method and lets
//     another package satisfy the type outright (SD-019, found by
//     adversarial_test.go)
//
// So the boundary does not trust the type. It rejects nil, it re-validates the
// bytes, and it survives a value whose methods panic.
var ErrNilObject = errors.New("module: nil object at the boundary; INV-032 requires Validated<Canonical<CICObject>>")

// ErrUnvalidatedObject is returned when the bytes a module would read do not
// pass final validation. It exists because INV-032's type-level guarantee is
// defeatable by interface embedding (docs/spec-defects.md SD-019), so the
// boundary re-checks rather than trusting the type alone.
var ErrUnvalidatedObject = errors.New("module: the object's canonical bytes are not valid")

// ErrObjectNotBound is returned when an object's node tree and its
// serialization do not describe the same object.
//
// A CanonicalObject that materialization produced always satisfies this: the
// bytes were made from the tree. Reaching this error means the value was
// assembled elsewhere, which INV-032 says cannot happen and Go cannot prevent
// (docs/spec-defects.md SD-019).
var ErrObjectNotBound = errors.New("module: the object's tree and bytes describe different objects")

// ErrModelVersion is returned when the host offers an object of a version
// this module has not declared (SPEC INV-034).
var ErrModelVersion = errors.New("module: undeclared object model version")

// Execute is the module entry point.
//
// Its parameter type is the whole of INV-032: it accepts
// objectmodel.CanonicalObject and nothing else. An Execute(map[string]any) —
// which SPEC §9 names explicitly as a violation — could not be given an
// object this package can trust, because the pipeline is the only thing that
// produces one.
func Execute(obj objectmodel.CanonicalObject) (err error) {
	delivered, err := Accept(obj)
	if err != nil {
		return err
	}
	// A real module would act here, and it acts on `delivered` — never on
	// `obj`. What matters for the spec is what it can no longer be handed:
	// INV-031(a)-(g) are all eliminated upstream, and boundary_test.go asserts
	// that clause by clause.
	_ = delivered
	return nil
}

// Delivered is what a module works from: an immutable snapshot taken at the
// boundary, with no way back to the value that crossed it.
//
// This type exists because of a time-of-check/time-of-use hole an external
// audit demonstrated. Execute used to call the interface five times — the
// version twice, the bytes twice — so a forgery that simply COUNTS its calls
// could return honest answers to the checks and something else to the module
// afterwards. Measured on the previous implementation: Execute returned nil
// after exactly two reads of CanonicalYAML, and the third read returned the
// attacker's bytes.
//
// Re-validating more often does not fix that; it only moves the count. The
// mitigation has to be that nothing downstream can ask again, which means the
// module never holds the interface. It holds this.
type Delivered struct {
	modelVersion string
	yaml         []byte
	root         *objectmodel.Node
}

// ModelVersion is the version this object was delivered at.
func (d *Delivered) ModelVersion() string { return d.modelVersion }

// CanonicalYAML is the validated serialization, copied.
func (d *Delivered) CanonicalYAML() []byte {
	out := make([]byte, len(d.yaml))
	copy(out, d.yaml)
	return out
}

// Root is the node tree the bytes above were checked against.
func (d *Delivered) Root() *objectmodel.Node { return d.root }

// accept is the boundary: it reads each method of the incoming value EXACTLY
// ONCE, checks the snapshot, and returns it.
//
// The call counts are the contract, and boundary_test.go asserts them. A second
// read of anything here would reopen the hole this function was written to
// close, however well-intentioned the reason for it.
// Accept is the boundary, exported so a host can take delivery without also
// running a module. Execute is Accept plus the module body.
func Accept(obj objectmodel.CanonicalObject) (d *Delivered, err error) {
	// A trust boundary must not be crashable by what crosses it. An embedded
	// forgery with a nil inner interface satisfies the type and panics on the
	// first method call; a panic reachable from a module author is a denial of
	// service, and worse than a rejection. Turn it into one.
	defer func() {
		if r := recover(); r != nil {
			d, err = nil, fmt.Errorf("%w: the object's methods are not callable (%v)",
				ErrNilObject, r)
		}
	}()

	if obj == nil {
		return nil, ErrNilObject
	}

	// One read each, and the checks interleaved rather than hoisted.
	//
	// Reading all three up front would also give the single-read property, and
	// it costs something: a value that implements only ModelVersion — which is
	// what a host holding a foreign-version object looks like — would panic on
	// the next method and be reported as uncallable instead of as the wrong
	// version. Reading in the order the checks need keeps both.
	version := obj.ModelVersion()
	if version != ModelVersion {
		return nil, fmt.Errorf("%w: got %q, this module consumes %q",
			ErrModelVersion, version, ModelVersion)
	}

	root := obj.Root()
	if root == nil {
		return nil, ErrNilObject
	}

	yaml := obj.CanonicalYAML()

	// The bytes must be a valid canonical object.
	//
	// The unexported marker method on the interface does NOT make the type
	// unconstructible elsewhere: another package can embed the interface in a
	// struct, which promotes the marker and satisfies the type. That is a
	// property of the language, not of this code (docs/spec-defects.md SD-019).
	// What can be restored is the guarantee that actually matters — that what a
	// module reads has been validated — and one parse per delivery buys it.
	//
	// The snapshot is copied first, so a slice the caller still holds cannot be
	// rewritten between this check and the module's use of it.
	snapshot := make([]byte, len(yaml))
	copy(snapshot, yaml)
	if err := objectmodel.ValidateCanonicalDocument(snapshot); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnvalidatedObject, err)
	}

	// And the two views must describe the SAME object.
	//
	// Validating the bytes says they are a well-formed canonical object; it does
	// not say they are THIS object's. A forgery pairing a real node tree with a
	// different but perfectly valid byte string passes everything above. That
	// was audit finding F-02, and it could not be closed while §8.8 defined no
	// serialization: re-serializing the tree and comparing would have failed on
	// formatting alone. §8.8.1 made the bytes a function of the tree.
	if !bytes.Equal(objectmodel.Canonicalize(root), snapshot) {
		return nil, ErrObjectNotBound
	}

	return &Delivered{modelVersion: version, yaml: snapshot, root: root}, nil
}
