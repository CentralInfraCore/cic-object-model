#!/usr/bin/env python3
"""Does the documentation tell the truth about the tree it ships with?

Nothing asked that until an external audit did. It found `SPEC.md` — the
declared authority — saying "one implementation" and `conformance/README.md`
saying "No implementation exists in this repository yet. Nothing here has
passed", in the same commit as a second implementation that passes every vector.
It found three different vector counts in three documents and an invariant count
two short.

None of that was carelessness in the ordinary sense. Every one of those
sentences was true when it was written, and nothing in the repository connected
it to the thing it described. A count copied into prose is a claim with no
owner: it does not fail when it stops being true, it just quietly stops being
true.

So this gate measures the tree and compares it to what the documents say. Two
kinds of check, because there are two ways a status claim goes wrong:

  NUMBERS   a document states a count; the count is measured and compared.
  PHRASES   a document states a situation — "one implementation", "never
            executed" — which the measurement contradicts. These cannot be
            derived, so they are listed and refused.

What this does NOT do is check prose nobody registered here. A sentence that
drifts in a file this tool does not know about drifts silently, exactly as
before. That is a real limit and it is the reason the lists below are explicit
rather than clever: every entry is a claim someone decided was worth binding.
"""

from __future__ import annotations

import re
import sys
from pathlib import Path


def repo_root() -> Path:
    return Path(__file__).resolve().parent.parent


# --------------------------------------------------------------------------
# What is actually true of the tree
# --------------------------------------------------------------------------


def measured(root: Path) -> dict[str, int]:
    """Facts, each derived from the artifact rather than from a document."""
    spec = (root / "SPEC.md").read_text(encoding="utf-8")

    # The invariant index of §12 is the register; a token appearing in prose is
    # a reference to an invariant, not a declaration of one.
    index = spec.split("## 12.", 1)[-1]
    invariants = len(set(re.findall(r"^\| (INV-\d+) \|", index, re.M)))

    vectors = len(list((root / "conformance").glob("*/*/meta.yaml")))

    # An implementation counts when it has BOTH sources and a conformance
    # runner. A directory with a .gitkeep is not an implementation, and one
    # that cannot run the corpus is not evidence about the model.
    implementations = 0
    for lang, runner in (
        ("go", "go/conformance"),
        ("rust", "rust/tests/conformance.rs"),
    ):
        has_sources = any((root / lang).rglob("*.go")) or any(
            (root / lang).rglob("*.rs")
        )
        has_runner = (root / runner).exists()
        if has_sources and has_runner:
            implementations += 1

    return {
        "invariants": invariants,
        "vectors": vectors,
        "implementations": implementations,
    }


# --------------------------------------------------------------------------
# Numbers stated in documents
# --------------------------------------------------------------------------

# (path, regex with one capturing group, which measured fact it must equal)
NUMBER_CLAIMS: list[tuple[str, str, str]] = [
    ("README.md", r"— (\d+) numbered invariants", "invariants"),
    ("README.md", r"— (\d+) vectors", "vectors"),
    ("README.md", r"pass all (\d+) vectors", "vectors"),
    ("docs/en/architecture.md", r"SPEC\.md — (\d+) numbered invariants", "invariants"),
    ("docs/en/architecture.md", r"conformance/ — (\d+) vectors", "vectors"),
    # docs/hu/architecture.md states no counts, deliberately: it describes the
    # layers and points at the register. A document that does not make a
    # numeric claim has nothing here to go stale, which is the cheapest fix
    # available for this whole class.
]

# --------------------------------------------------------------------------
# Situations stated in documents that a measurement can contradict
# --------------------------------------------------------------------------

# (path, phrase, the condition under which the phrase is false)
# The phrases are deliberately long. A short one — "one implementation" —
# also matched "without the corpus and the other implementation noticing" and
# a sentence about why 0.1's bootstrap rule deadlocked, both of which are
# correct prose. A check that fires on legitimate text gets switched off, and
# then it checks nothing.
PHRASE_CLAIMS: list[tuple[str, str, str]] = [
    ("SPEC.md", "normative, one implementation", "implementations > 1"),
    ("SPEC.md", "executed by one implementation", "implementations > 1"),
    ("conformance/README.md", "never executed", "implementations > 0"),
    ("conformance/README.md", "No implementation exists", "implementations > 0"),
    ("docs/spec-vector-map.md", "does not exist yet", "implementations > 1"),
    ("docs/spec-vector-map.md", "still an untested claim", "implementations > 1"),
    ("Makefile", "mk/rust.mk is NOT present", "implementations > 1"),
]


def phrase_is_false(condition: str, facts: dict[str, int]) -> bool:
    key, op, value = condition.split()
    return facts[key] > int(value) if op == ">" else False


def main() -> int:
    root = repo_root()
    facts = measured(root)
    failures: list[str] = []

    print("measured from the tree:")
    for name, value in facts.items():
        print(f"  {name:16} {value}")
    print()

    for path, pattern, fact in NUMBER_CLAIMS:
        target = root / path
        if not target.is_file():
            failures.append(f"{path}: registered for a claim but the file is missing")
            continue
        text = target.read_text(encoding="utf-8")
        found = re.findall(pattern, text)
        if not found:
            failures.append(
                f"{path}: no claim matched /{pattern}/ — the sentence moved or was "
                f"reworded, so it is no longer checked"
            )
            continue
        for stated in found:
            if int(stated) != facts[fact]:
                failures.append(
                    f"{path}: says {stated} {fact}, the tree has {facts[fact]}"
                )

    for path, phrase, condition in PHRASE_CLAIMS:
        target = root / path
        if not target.is_file():
            continue
        if phrase.lower() in target.read_text(encoding="utf-8").lower():
            if phrase_is_false(condition, facts):
                failures.append(
                    f'{path}: says "{phrase}", which is false when {condition} '
                    f"(it is {facts[condition.split()[0]]})"
                )

    if failures:
        print("FAIL  the documentation does not describe this tree:")
        for f in failures:
            print(f"  {f}")
        print()
        print("A count copied into prose is a claim with no owner. Update the")
        print(
            "document, or register the claim differently in tools/check_status_claims.py."
        )
        return 1

    print(f"ok    {len(NUMBER_CLAIMS)} counts and {len(PHRASE_CLAIMS)} status phrases")
    print()
    print("This checks the claims listed in this tool and nothing else. Prose in a")
    print("file it does not know about drifts silently, as all of this did.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
