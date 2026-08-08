// Package module is the module boundary of SPEC §9.
//
// It contains no provisioning logic. It exists so that INV-031 and INV-032
// have a boundary to be true of: docs/spec-vector-map.md marks both
// unvectorizable and assigns them to the implementation as source-level
// checks, and a boundary that does not exist cannot be checked.
package module

import (
	"errors"
	"fmt"

	"github.com/CentralInfraCore/cic-object-model/go/objectmodel"
)

// ModelVersion is the object model version this module declares it consumes
// (SPEC INV-034). A host must not hand it an object of any other version.
const ModelVersion = "0.1"

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
	// A trust boundary must not be crashable by what crosses it. An embedded
	// forgery with a nil inner interface satisfies the type and panics on the
	// first method call; a panic reachable from a module author is a denial of
	// service, and worse than a rejection. Turn it into one.
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%w: the object's methods are not callable (%v)",
				ErrNilObject, r)
		}
	}()

	if obj == nil {
		return ErrNilObject
	}
	if obj.ModelVersion() != ModelVersion {
		return fmt.Errorf("%w: got %q, this module consumes %q",
			ErrModelVersion, obj.ModelVersion(), ModelVersion)
	}
	root := obj.Root()
	if root == nil {
		return ErrNilObject
	}
	// Defence in depth, added after adversarial_test.go got a forged object
	// across this boundary.
	//
	// The comment on ErrNilObject above claims the unexported method makes this
	// type unconstructible elsewhere. That claim is FALSE in Go: another
	// package can embed the interface in a struct, which promotes the
	// unexported method and satisfies the type. Pair that with a real node tree
	// taken from a legitimate materialization, and every check above passes
	// while CanonicalYAML returns whatever the forger chose. Recorded as
	// docs/spec-defects.md SD-019.
	//
	// The type-level guarantee cannot be restored — it is a property of the
	// language, not of this code. What can be restored is the guarantee that
	// actually matters: what a module READS has been validated. One parse per
	// delivery buys that back.
	if err := objectmodel.ValidateCanonicalDocument(obj.CanonicalYAML()); err != nil {
		return fmt.Errorf("%w: %v", ErrUnvalidatedObject, err)
	}
	// A real module would act here. What matters for the spec is what it can
	// no longer be handed: INV-031(a)-(g) are all eliminated upstream, and
	// boundary_test.go asserts that clause by clause.
	return nil
}
