// Package objectmodel implements the CIC object model, model version 0.2.
//
// The normative document is ../../SPEC.md. Where this package and that document
// disagree, the document is right and this package has a bug. The conformance
// corpus in ../../conformance is what decides which of the two is happening.
//
// # The surface, in one place
//
// Three entry points, and nothing else produces a CIC object:
//
//	Materialize(schemaYAML, inputYAML) (CanonicalObject, error)
//	    Runs the whole pipeline of SPEC §8 and is the ONLY producer of a
//	    CanonicalObject. Authoring YAML in, canonical object out.
//
//	ValidateCanonicalDocument(objectYAML) error
//	    Checks an already-canonical object presented without its schema. This
//	    walk is weaker than materialization by design and says so: INV-039.
//
//	LoadSchema(schemaYAML) (*Schema, error)
//	    Validates a schema on its own, for tooling that wants to reject a bad
//	    schema before there is any input to materialize. Limit worth stating:
//	    *Schema has no accessors yet and cannot be handed back to Materialize,
//	    so today the useful half of the result is the error.
//
// # Reading an object
//
// CanonicalObject is the type of a module's input (INV-032). It cannot be
// constructed outside this package — the interface holds an unexported method,
// so no other package can implement it, which is the enforcement rather than a
// convention that callers are asked to follow.
//
//	obj.ModelVersion()   the version this object was materialized at. This is
//	                     the hand-off frame of INV-033; the version is NOT a
//	                     member of the object and never appears in the YAML.
//	obj.CanonicalYAML()  the deterministic serialization (INV-030), copied.
//	obj.Root()           the root CIC node.
//
// A Node is read through:
//
//	Get(path)      address resolution — the operation SPEC §2.2 promises
//	Child(name)    a named child of a structured payload
//	Primitive(n)   a materialized primitive member
//	Children()     child names, canonical order
//	Primitives()   primitive names, canonical order
//	Scalar()       a scalar payload
//	Len(), At(i)   a list payload whose element positions the schema declared
//	Path()         this node's own address
//	Origin()       authoring authority (SPEC §5)
//
// Get takes the same address syntax that Error.Path carries, so a rejection can
// be followed straight back to the position it names:
//
//	n, ok := obj.Root().Get("$.values.mtu.access.read.rules.operator")
//
// That address is the whole point of the model. §2.2 argues that
// network.values.mtu.access.read is a first-class object rather than a path
// into a YAML blob — hashable, diffable, referenceable from evidence. Model 0.1
// left primitive payloads as raw mappings, which made the claim false of every
// object it produced (docs/spec-defects.md SD-003); 0.2 materializes them, and
// Get is how a caller uses the result.
//
// # Errors
//
// Every rejection is an *Error carrying four fields the conformance vectors
// assert on — Code, Invariant, Stage, Path — plus a human Detail. The Stage
// matters as much as the Code: the pipeline stages of SPEC §8 run in a fixed
// order, and a later stage cannot observe input an earlier one would have
// rejected. An error raised at the wrong stage is a defect even when the code
// is right.
//
// # What this package does not do
//
//   - It does not resolve inherit chains. INV-037: `inherit` is recorded
//     verbatim. Resolution needs the policy-decision point, which SPEC §1 puts
//     out of scope, and §6.4 names the three questions that must be answered
//     before it can be specified at all.
//   - It does not resolve external references. §8.2 is out of scope for 0.2;
//     the schema language has no reference syntax.
//   - It does not decide policy, match CertPatterns, or evaluate access. It
//     records what was declared, at addresses that can be reasoned about later.
package objectmodel
