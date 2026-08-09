# External review records

One file per release subject, named `<subject-digest>.md`, where the digest is
what `make release.subject` prints for the tree that was reviewed.

The naming is the whole mechanism. A review is of a *tree*, and a record not
bound to one lets "it was reviewed" survive every subsequent change — which is
how a review becomes a thing that was once done rather than a thing that is
true. `make review.check` looks for the record matching the tree in front of it,
and CI runs that on every pull request into `main` (SPEC INV-046).

What the check cannot establish is whether a person did the work, or did it
well. It establishes which tree they were looking at, and nothing more is
claimed for it.

The procedure and the three commissioning prompts are in
[`../docs/external-review.md`](../docs/external-review.md). The prompts are run
in three separate threads, from a cold start each — findings anchor, and an
angle that arrives already pointed somewhere is not an independent angle.

## Why this directory is not part of the release subject

A record here is named for the subject digest, so it is a claim *about* the
subject and cannot be part of it — the same rule that excludes `MANIFEST.sha256`
and `project.yaml`.

That was missed when the rule was written, and it made INV-046 unsatisfiable: a
record committed to a pull request changed the digest its own filename referred
to, and adding the newly required record changed it again. The test did not
catch it because it wrote the record without `git add`, so `git ls-files` never
saw it — a false positive in the test written to check this gate.

An external audit found it by computing both digests. Nothing in the repository
did.

## What is here

Records from the first commissioned review, run as three separate threads
against `cbaf928`: a claim audit, a semantic divergence audit and an adversarial
boundary audit. The reviewer had no Go, Rust, Python-test or container
toolchain, so the findings are source-proven control-flow arguments with minimal
reproducers rather than executed results. Nine of them were subsequently
executed here and are marked with what was measured.
