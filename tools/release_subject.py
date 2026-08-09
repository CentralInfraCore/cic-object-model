#!/usr/bin/env python3
"""The release subject: what a signature over this repository actually covers.

Before this existed, the release signed a hash of ``project.yaml``, and
``compiler_settings.canonical_source_file`` pointed at ``spec/index.yaml`` — one
file, which names ``../SPEC.md`` by path and binds nothing about its content. So
``SPEC.md``, all 31 conformance vectors and both reference implementations could
be changed and the signature stayed valid. A signature that does not cover the
normative product is worse than none: it says something was checked, and what it
checked is not what a reader is relying on.

The subject is a digest over the manifest lines, which carry a SHA-256 for
every tracked file. Digesting them covers the whole tree transitively — change
any byte of any file and its manifest entry changes, and the subject digest with
it.

Two files are excluded, and the reason is the same for both: a digest cannot
cover something that carries the digest. ``MANIFEST.sha256`` cannot hash itself,
and ``project.yaml`` holds the subject digest, so including it would mean
writing the answer changed the question. The descriptor is the CLAIM about the
subject; the subject is the normative product it claims to be about.

Deliberately stdlib-only and free of this repository's tooling: a third party
verifying a release has a clone and a Python, not a Docker daemon, a Vault token
or a copy of ``mk/``. A verifier that only its own author can run does not
verify anything.

    python3 tools/release_subject.py compute
    python3 tools/release_subject.py verify
"""

from __future__ import annotations

import argparse
import hashlib

# nosec B404 — subprocess is used for exactly one thing: asking git which files
# are tracked. There is no way to enumerate that without git, and reimplementing
# .gitignore semantics to avoid a subprocess would be a larger correctness risk
# than the one being avoided.
import subprocess  # nosec B404
import sys
from pathlib import Path

MANIFEST = "MANIFEST.sha256"
DESCRIPTOR = "project.yaml"

# Excluded from the subject: neither can be covered by a digest it carries.
NOT_SUBJECT = frozenset({MANIFEST, DESCRIPTOR})

# Where external review records live, one per subject digest.
REVIEWS = "reviews"

# The field in project.yaml that carries the subject digest. `buildHash` is
# where the release template puts a compiled artifact's digest; this repository
# compiles nothing, and the note beside the field already said the subject is
# "the spec + vector corpus". This makes that true rather than intended.
SUBJECT_FIELD = "buildHash"


def repo_root() -> Path:
    """The repository root, found from this file rather than the caller's cwd."""
    return Path(__file__).resolve().parent.parent


def tracked_files(root: Path) -> list[str]:
    # nosec B603 B607 — a fixed argument vector with no shell and no caller
    # input; `git` is resolved from PATH deliberately, because pinning an
    # absolute path would make the verifier fail for the third parties it
    # exists to serve.
    #
    # The IDs are separated by a SPACE. `# nosec B603,B607` suppresses only the
    # first and reports the second, silently — a suppression that half works
    # reads exactly like one that works.
    # The call is written on one line because bandit anchors a `# nosec` to the
    # line it reports, and it reports the line the call STARTS on — a
    # black-reformatted multi-line call puts the comment where the finding is
    # not.
    out = subprocess.run(
        ["git", "ls-files"], cwd=root, check=True, capture_output=True, text=True
    ).stdout  # noqa: E501  # nosec B603 B607
    return sorted(p for p in out.splitlines() if p and p not in NOT_SUBJECT)


def file_digest(path: Path) -> str:
    h = hashlib.sha256()
    with path.open("rb") as f:
        for chunk in iter(lambda: f.read(65536), b""):
            h.update(chunk)
    return h.hexdigest()


def build_manifest(root: Path) -> str:
    """Regenerate the manifest exactly as ``make manifest-update`` writes it.

    Sorted over the WHOLE LINE, which puts the digest first — that is what
    `LC_ALL=C sort` does to `sha256sum` output, and the two have to agree
    byte for byte or every verification fails on ordering alone. Sorting by
    path instead looks more sensible and produces a different file.
    """
    lines = sorted(f"{file_digest(root / p)}  {p}" for p in tracked_files(root))
    return "".join(line + "\n" for line in lines)


def subject_digest(manifest_text: str) -> str:
    return hashlib.sha256(manifest_text.encode("utf-8")).hexdigest()


def claimed_subject(root: Path) -> str | None:
    """Read the subject digest out of project.yaml.

    Parsed with a line scan rather than a YAML library, because the verifier
    must run for someone who has a clone and a Python and nothing else. The
    field is a plain scalar in a known place; a dependency to read it would cost
    more than it buys.
    """
    prefix = f"  {SUBJECT_FIELD}:"
    for line in (root / DESCRIPTOR).read_text(encoding="utf-8").splitlines():
        if line.startswith(prefix):
            value = line[len(prefix) :].strip().strip("'\"")
            return value or None
    return None


def cmd_compute(root: Path) -> int:
    print(subject_digest(build_manifest(root)))
    return 0


def cmd_verify(root: Path) -> int:
    """Three questions, in the order that makes a failure legible."""
    ok = True

    # 1. Does the manifest describe the tree that is here?
    #
    # Compared over the subject's file set, so this answers the same question
    # `make manifest-verify` does for the files both cover, and says nothing
    # about the descriptor — which the next check is entirely about.
    rebuilt = build_manifest(root)
    committed = "".join(
        line + "\n"
        for line in (root / MANIFEST).read_text(encoding="utf-8").splitlines()
        if line.split("  ", 1)[-1] not in NOT_SUBJECT
    )
    if rebuilt != committed:
        ok = False
        print(f"FAIL  {MANIFEST} does not describe this tree")
        rebuilt_set = set(rebuilt.splitlines())
        committed_set = set(committed.splitlines())
        for line in sorted(committed_set - rebuilt_set)[:10]:
            print(f"        in manifest, not in tree (or changed): {line}")
        for line in sorted(rebuilt_set - committed_set)[:10]:
            print(f"        in tree, not in manifest (or changed): {line}")
    else:
        print(
            f"ok    {MANIFEST} describes all {len(rebuilt.splitlines())} tracked files"
        )

    # 2. Does the descriptor's subject digest match the tree that is here?
    computed = subject_digest(rebuilt)
    claimed = claimed_subject(root)
    if claimed is None or claimed.startswith("TBD"):
        ok = False
        print(
            f"FAIL  {DESCRIPTOR} claims no release subject "
            f"({SUBJECT_FIELD} is {claimed!r})"
        )
        print(f"        the tree's subject digest is {computed}")
    elif claimed != computed:
        ok = False
        print("FAIL  the release subject does not match this tree")
        print(f"        {DESCRIPTOR} claims  {claimed}")
        print(f"        this tree computes  {computed}")
    else:
        print(f"ok    release subject {computed}")

    # 3. Say plainly what a matching subject does and does not establish.
    print()
    if ok:
        print("The subject digest covers every tracked file except MANIFEST.sha256")
        print("and project.yaml: SPEC.md, the schema index, all conformance vectors,")
        print("and both implementations. A change to any byte of any of them changes")
        print("this digest.")
        print()
        print("It does NOT establish who produced the tree. That is the signature")
        print("over this digest, which is a separate artifact and a separate check.")
    return 0 if ok else 1


def cmd_review(root: Path) -> int:
    """Is there an external review record for the tree that is here?

    `devel` reaches `main` only after an independent external review
    (SPEC INV-046, docs/external-review.md). No check can establish that a
    person did the work, or that they did it well. What it CAN establish is
    WHICH TREE they were looking at — a record naming a different subject is a
    review of a different thing, and without that binding "we had it reviewed"
    survives every subsequent change to the tree.
    """
    subject = subject_digest(build_manifest(root))
    reviews_dir = root / REVIEWS
    record = reviews_dir / f"{subject}.md"
    if record.is_file():
        print(f"ok    external review recorded: {REVIEWS}/{subject}.md")
        return 0

    print("FAIL  no external review record for this tree")
    print(f"        expected  {REVIEWS}/{subject}.md")
    existing = (
        sorted(p.name for p in reviews_dir.glob("*.md")) if reviews_dir.is_dir() else []
    )
    if existing:
        print(f"        present   {', '.join(existing)}")
        print("        those review a different tree")
    print("        see docs/external-review.md for the procedure and the three")
    print("        commissioning prompts")
    return 1


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("command", choices=["compute", "verify", "review"])
    args = parser.parse_args()
    root = repo_root()
    if args.command == "compute":
        return cmd_compute(root)
    if args.command == "review":
        return cmd_review(root)
    return cmd_verify(root)


if __name__ == "__main__":
    sys.exit(main())
