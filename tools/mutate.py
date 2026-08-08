#!/usr/bin/env python3
"""Named mutation testing: does the suite notice when the code is wrong?

A green test suite proves the code passes the tests. It does not prove the tests
would fail if the code were broken — and a gate that cannot go red is not a
gate. This tool answers that question by breaking the code on purpose, one
change at a time, and requiring the suite to fail.

Deliberately NOT a generative mutation tool (go-mutesting and friends). Those
produce thousands of mutants, most of them equivalent or uninteresting, and take
long enough that nobody runs them. This carries a curated table instead: each
entry is a mistake someone could plausibly make, at a place where being wrong
matters, and the table says which suite is supposed to catch it.

The table is the interesting artefact. Adding a row is a claim about what the
tests protect; a row that survives is a hole nobody knew was there.

Usage:
    python3 tools/mutate.py            # run every mutation
    python3 tools/mutate.py --list     # show the table
    python3 tools/mutate.py -k origin  # only mutations whose name matches
"""
from __future__ import annotations

import argparse
import dataclasses
import pathlib

# nosec B404 — this tool exists to run the test suite; a subprocess is the
# point of it, not an incidental risk.
import subprocess  # nosec B404
import sys

REPO = pathlib.Path(__file__).resolve().parent.parent


@dataclasses.dataclass
class Mutation:
    name: str
    file: str
    find: str
    replace: str
    # What the mutation breaks, in one line — read this to decide whether the
    # suite catching it is meaningful.
    breaks: str
    # Which suite is expected to catch it. If the wrong suite catches it, that
    # is still a pass, but the note tells a reader what was expected.
    expect: str


MUTATIONS: list[Mutation] = [
    Mutation(
        name="root-loses-its-primitives",
        file="go/objectmodel/primitives.go",
        find="\tif err := attachPrimitives(n); err != nil {",
        replace="\tif err := func(*Node) error { return nil }(n); err != nil {",
        breaks="the root stops carrying the shape its schema declares (INV-022)",
        expect="conformance — every materialization vector shows the root's shape",
    ),
    Mutation(
        name="primitive-payload-stays-flat",
        file="go/objectmodel/primitives.go",
        find='\t\t\tn.entries[k] = primitiveNode(path+".values."+k, child, org)',
        replace="\t\t\tn.entries[k] = &Node{path: path, kind: kindRaw, raw: child, origin: org}",
        breaks="primitive payloads stop materializing into nodes (INV-035, SD-003)",
        expect="conformance — 013 and 011 encode the node tree",
    ),
    Mutation(
        name="address-drops-the-values-step",
        file="go/objectmodel/node.go",
        find='\t\tcase seg == "values":',
        replace='\t\tcase seg == "__never__":',
        breaks="Get stops resolving payload children, so no address works (INV-040)",
        expect="api — the corpus never calls an accessor and cannot see this",
    ),
    Mutation(
        name="origin-truth-table-accepts-yaml-and-schema",
        file="go/objectmodel/validate.go",
        find="\tif hasYAML && hasSchema {",
        replace="\tif hasYAML \u0026\u0026 !hasYAML {",
        breaks="a node may claim both authored and defaulted provenance (INV-017)",
        expect="conformance — validation/001 is exactly this case",
    ),
    Mutation(
        name="error-stage-is-wrong",
        file="go/objectmodel/primitives.go",
        find='return newError(CodeUnknownPrimitive, "INV-021", StagePrimitiveEvaluation,',
        replace='return newError(CodeUnknownPrimitive, "INV-021", StageEntryValidation,',
        breaks="a correct error is raised at the wrong pipeline stage (SPEC §8)",
        expect="conformance — every expected-error.yaml asserts on stage:",
    ),
    Mutation(
        name="canonical-output-is-not-copied",
        file="go/objectmodel/materialize.go",
        find="\tout := make([]byte, len(o.bytes))\n\tcopy(out, o.bytes)\n\treturn out",
        replace="\treturn o.bytes",
        breaks="a caller can mutate the canonical bytes of a validated object",
        expect="module — boundary_test asserts CanonicalYAML hands out a copy",
    ),
    Mutation(
        name="version-member-is-tolerated",
        file="go/objectmodel/validate.go",
        find='\t\tcase isDocRoot && k == "cic":',
        replace='\t\tcase isDocRoot && k == "__never__":',
        breaks="a `cic` member on the root stops being rejected (INV-033, SD-017)",
        expect="conformance — validation/006 is exactly this case",
    ),
]


def run(cmd: list[str]) -> int:
    # nosec B603 — B603 is about executing untrusted input. Every command this
    # runs is a fixed list defined in this file; nothing from the mutation table
    # or the command line reaches argv. shell=True is deliberately not used, so
    # there is no shell to inject into either.
    return subprocess.run(  # nosec B603
        cmd, cwd=REPO, capture_output=True, text=True
    ).returncode


def suite_passes() -> bool:
    """The whole Go suite: conformance, api, module boundary, inv032."""
    return (
        run(
            [
                "docker",
                "compose",
                "exec",
                "-T",
                "builder",
                "sh",
                "-c",
                "cd /app/go && go test ./... -count=1",
            ]
        )
        == 0
    )


def apply(m: Mutation) -> str:
    p = REPO / m.file
    original = p.read_text()
    if m.find not in original:
        raise SystemExit(
            f"mutation {m.name!r}: anchor not found in {m.file}.\n"
            f"The code moved and the table did not. Fix the anchor — do not delete the row."
        )
    p.write_text(original.replace(m.find, m.replace, 1))
    return original


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--list", action="store_true")
    ap.add_argument("-k", metavar="SUBSTR", default="")
    args = ap.parse_args()

    selected = [m for m in MUTATIONS if args.k in m.name]

    if args.list:
        for m in selected:
            print(f"{m.name}\n    breaks:  {m.breaks}\n    expect:  {m.expect}\n")
        return 0

    print("Baseline: the suite must pass before anything is broken.")
    if not suite_passes():
        print(
            "FAIL — the suite is already red. Fix that first; mutation results "
            "would mean nothing."
        )
        return 1
    print("  baseline green\n")

    survived: list[Mutation] = []
    for i, m in enumerate(selected, 1):
        print(f"[{i}/{len(selected)}] {m.name}")
        print(f"        {m.breaks}")
        original = apply(m)
        try:
            caught = not suite_passes()
        finally:
            (REPO / m.file).write_text(original)
        if caught:
            print("        CAUGHT\n")
        else:
            print("        SURVIVED — no test noticed\n")
            survived.append(m)

    print("=" * 64)
    print(f"{len(selected) - len(survived)}/{len(selected)} mutations caught")
    if survived:
        print("\nSurvivors — each is a hole in the suite, not a passing result:")
        for m in survived:
            print(f"  {m.name}\n      {m.breaks}\n      expected: {m.expect}")
        return 1
    print("\nEvery mutation was caught. The suite can go red.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
