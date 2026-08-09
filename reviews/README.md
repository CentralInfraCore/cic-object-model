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

This directory is empty until the first release under the rule. That is the
honest state: the rule was written after the review that motivated it, and
back-dating a record for a tree nobody examined would be exactly the kind of
claim this repository has spent its time removing.
