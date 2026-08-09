# External review — the rule, and how to commission one

`devel` reaches `main` only after an **independent external review** of the
release subject. This document is the procedure and the evidence format.

The rule exists because of what these reviews find. The first, against
`e173149`, produced nineteen findings; the four checked first were confirmed by
measurement within minutes. The second, against `cbaf928`, produced
thirty-seven across three threads, and nine of them were reproduced here by
execution — including a five-byte input that panicked a debug build and a
schema three lines long that made the only producer of canonical objects return
bytes no YAML parser accepts.

Every one of them had passed a full green CI, repeatedly, because the gates were
written by the same reading that wrote the code.

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

One prompt gets one angle, and the first commissioned review proved it in both
directions.

The claim angle found that the review gate written the day before could not be
satisfied at all — a fixed-point defect nothing here would have found, because
the only test exercising it happened to avoid the condition that breaks it. The
divergence angle found three cases where the two implementations disagreed on
ordinary input, all of them invisible to a corpus containing no wrong-typed
input. The adversarial angle found that a boundary check added that same day
bound two snapshots rather than the object, and that the sole producer of
canonical objects could return bytes that are not YAML.

No one of those prompts would have found the other two's findings.

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

### A preamble every thread carries

Each prompt is prefixed with the same three lines and nothing else:

```
Repository: https://github.com/CentralInfraCore/cic-object-model  (branch: devel)
Release subject digest: <digest>
Do not fix the repository. Do not assume the documentation, the specification,
the corpus or an implementation is correct. Support every finding with concrete
evidence from the repository.
```

The last sentence is load-bearing. The first review's most expensive findings
were places where a document and the code disagreed, and a reviewer who starts
from "the specification says X, does the code do X" cannot find a defect in X.

### Prompt 1 — Claim audit

> Examine what the repository claims about itself, and identify which of those
> claims are:
>
> * factually false,
> * stale,
> * only partly true,
> * unproven,
> * stated more strongly than the implementation or the tests establish.
>
> Do not examine only the README. Compare the claims of the README, SPEC, docs,
> conformance corpus, tooling, CI and the actual implementations **against each
> other**.
>
> For each finding give:
>
> **claim → evidence → observed reality → classification → consequence**
>
> Do not treat a passing test as automatic proof of the general claim behind it.

### Prompt 2 — Semantic divergence audit

> Find places where the repository's different representations describe the same
> semantics differently.
>
> Compare primarily:
>
> **SPEC ↔ conformance corpus ↔ machine-readable schema ↔ Go implementation ↔
> Rust implementation ↔ documentation ↔ tooling**
>
> Look especially for:
>
> * the same rule interpreted differently;
> * unspecified implementation decisions;
> * behaviour the corpus fixes but the SPEC does not state;
> * a normative requirement the corpus cannot distinguish;
> * two different behaviours that could both currently be considered conformant;
> * stale or mutually contradictory documentation;
> * semantics that come from one language's libraries rather than from the
>   specification.
>
> **There are two independent implementations**, `go/` and `rust/`, and they
> currently produce byte-identical output on every vector. Find inputs on which
> they DIVERGE — especially where each delegates to its own language's libraries
> (YAML parsing, number formatting, string handling, map ordering), because that
> is where two conformant implementations differ without either being wrong.
>
> Where a third implementation would be needed to decide whether the
> specification is genuinely unambiguous, mark it **latent cross-implementation
> ambiguity**.
>
> For each finding:
>
> **semantic question → competing interpretations → repo evidence → current
> implementation choice → specified or accidental? → consequence**

### Prompt 3 — Adversarial boundary audit

> Do not check whether the happy path follows the SPEC. Try to REFUTE the
> repository's security and structural claims.
>
> Attack in particular:
>
> * the materializer → canonical object → module boundary;
> * construction guarantees of the INV-031 / INV-032 kind;
> * type-system and API boundary bypasses;
> * hand-forged or partly legitimate objects;
> * gaps between canonical serialization and validation;
> * weaknesses of schema-less validation;
> * recursive and deeply nested structures;
> * input-amplification cases;
> * inputs that are cheap to produce but disproportionately expensive to
>   validate, materialize, canonicalize or reject;
> * memory, CPU, recursion and output amplification;
> * a payload that crosses a boundary although the model says it should not be
>   representable.
>
> The goal is not to confirm the invariants but to find a **minimal
> counterexample**.
>
> For each finding:
>
> **claimed invariant → attack construction → observed result → minimal
> reproducer → violated guarantee → severity**
>
> If you cannot execute an attack, do not report it as a successful defence
> merely because you found no counterexample.

### On a second and later round

A repeat review is given the same three prompts and one extra sentence: that
this is round *n* against the same repository, and how many findings from the
previous round are recorded as closed — **without** describing the fixes.

The count without the descriptions is deliberate. It lets the reviewer skip
ground already covered, and it leaves them free to check whether those findings
really are closed, which is a question the people who closed them cannot answer
about themselves.

## What the reviewer is given

- the repository URL — it is public, so no access needs granting
- the release subject digest, and `make release.verify` to confirm the tree
- `docs/spec-defects.md`, which records where this document is known to be
  weak, so the review is not spent rediscovering what is already written down

They are deliberately **not** given a summary of the changes. A reviewer working
from the author's account of a change inherits the author's reading of it, which
is the thing being checked.
