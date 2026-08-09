# External review — the rule, and how to commission one

`devel` reaches `main` only after an **independent external review** of the
release subject. This document is the procedure and the evidence format.

The rule exists because of what the first one found. An external audit of
`e173149` produced nineteen findings, and the four checked first were all
confirmed by measurement within minutes: the model version had split five ways,
`Origin()` handed out the node's own slice, the origin grammar accepted forms it
declares invalid, and the manifest gate could not see a file missing from the
manifest. Every one of them had passed a full green CI, repeatedly, because the
gates were written by the same reading that wrote the code.

That is the property an external review has and no internal gate can acquire:
**it does not share the author's blind spot.**

---

## What "independent" requires

- **Not the author.** Not the person who wrote the change, and not a reviewer
  working from the author's description of it.
- **Working from the artifact.** The repository is public. A reviewer is given
  the URL and the release subject digest, not a summary.
- **Free to disagree with the specification.** A review that only checks the
  implementation against `SPEC.md` cannot find a defect *in* `SPEC.md`, and the
  most expensive findings so far have been there.

## What a review must produce

A file at `reviews/<subject-digest>.md`, where the digest is the value
`make release.subject` prints for the tree that was reviewed.

Naming the file after the subject is what makes the record checkable. A review
of a different tree is a review of a different thing, and without the binding
"we had it reviewed" survives every subsequent change. `make review.check`
verifies that a record exists for the current subject; on a pull request into
`main`, CI runs it.

The record carries, at minimum:

```markdown
# Review of <subject-digest>

- **Reviewer:** who, and what they are independent of
- **Date:**
- **Commissioned with:** which of the prompts below, or the text used
- **Findings:** numbered, each one falsifiable

## Disposition

One line per finding: confirmed by measurement / not reproduced / contested,
and where the fix landed.
```

**A review with no findings is a result, not a failure** — but it must say what
was examined, or it records only that someone was asked.

---

## Commissioning: three prompts, in three separate threads

A commission carries **three distinct starting prompts**, and each one is run in
its **own thread**, from a cold start.

### Why three

One prompt gets one angle. The first audit of this repository was a claim audit,
and it was extremely productive — and it found nothing about YAML alias
amplification, which a security-angled prompt would have gone straight to, and
nothing about the two implementations disagreeing on duplicate keys, which a
divergence-angled prompt would have found immediately. Both were real. Both were
found later, by accident.

The three angles are deliberately not refinements of each other.

### Why separate threads

Because findings anchor. A reviewer who has just confirmed a version-propagation
defect starts looking for version-propagation defects, and the second and third
angles arrive already pointed somewhere. Three threads keep the angles
independent, which is the entire reason there are three of them rather than one
longer prompt.

It also makes the prompts falsifiable as prompts. If two threads with different
angles return the same finding, that finding is robust; if an angle returns
nothing across releases, the angle is wrong and should be replaced rather than
kept for symmetry.

Each prompt below is therefore **self-contained**: it names the repository, the
subject digest and what to do, and assumes no earlier conversation. Do not
summarise the other two threads into a third.

### Prompt 1 — Claim audit

> The repository at `<URL>` is a normative specification with two reference
> implementations. Its release subject digest is `<digest>`.
>
> It makes claims about itself: in `SPEC.md`, in `README.md`, in doc comments,
> in commit messages, and in the names of its own tests. Find claims that are
> not true of the tree, or that are true only in a weaker sense than the wording
> implies.
>
> Prioritise claims a reader would rely on: that an invariant is enforced, that
> a gate checks something, that a test proves a property, that two things agree.
> For each, state how you checked it and what you found — measurement, not
> reading. Say which findings you could not verify.

### Prompt 2 — Divergence hunt

> The repository at `<URL>` ships two independent implementations of one
> specification, in `go/` and `rust/`, plus a conformance corpus in
> `conformance/`. Release subject digest: `<digest>`.
>
> Find inputs on which the two implementations behave differently, and
> behaviours neither the corpus nor the specification pins. Look especially at
> what each delegates to its language's libraries — YAML parsing, number
> formatting, string handling, map ordering — because that is where two
> conformant implementations diverge without either being wrong.
>
> For each divergence, say which implementation you would consider correct and
> what the specification would have to say to settle it.

### Prompt 3 — Adversarial

> The repository at `<URL>` defines an object model whose inputs are untrusted
> by construction, and a module boundary that is meant to be a trust boundary.
> Release subject digest: `<digest>`.
>
> Attack it. Construct inputs or values that cross a boundary carrying something
> the model says cannot be there: a forged object, an object whose parts
> disagree with each other, an input that costs more to process than it costs to
> write, a document whose meaning depends on which parser reads it.
>
> Prefer working demonstrations over arguments. Where a defence exists, check
> whether it fails safely and whether the failure is loud.

---

## What the reviewer is given

- the repository URL — it is public, so no access needs granting
- the release subject digest, and `make release.verify` to confirm the tree
- `docs/spec-defects.md`, which records where this document is known to be
  weak, so the review is not spent rediscovering what is already written down

They are deliberately **not** given a summary of the changes. A reviewer working
from the author's account of a change inherits the author's reading of it, which
is the thing being checked.
