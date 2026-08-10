# Disposition — review of `cbaf928`

One row per finding of the three review threads. `tools/check_review_ledger.py`
requires that every finding raised in `cbaf928.{claim,semantic,adversarial}.md`
appears here exactly once, and that every row not marked `closed` names a
decision that exists in [`../docs/pending-decisions.md`](../docs/pending-decisions.md).

That gate exists because this ledger's first version lost two findings. The
decisions document listed twelve; `claim/F-08` and `claim/F-10` were in neither
it nor any commit, and the omission was invisible because nothing compared the
list to its source. Which is the failure this repository has now found in the
manifest, in the status claims, in the vector map, and here — a list that looks
complete because nothing checks it against what it is a list of.

**Status vocabulary**

- `closed` — the finding no longer reproduces; the commit is named.
- `partial` — the executable half is fixed and the specification half is a
  decision. Both halves are named. A `partial` row is NOT closed, and the gate
  treats it as open.
- `open` — untouched, or a decision only.

| finding | status | where |
|---|---|---|
| adversarial/F-01 | closed | Rust `required` enforced — PR #17 |
| adversarial/F-02 | closed | list position rejects a non-sequence — PR #17 |
| adversarial/F-03 | closed | scalar position rejects a collection, both — PR #17 |
| adversarial/F-04 | closed | keys quoted; Go reparses its output — PR #18 |
| adversarial/F-05 | closed | a primitive member must be a node — PR #20 |
| adversarial/F-06 | closed | more than one document is refused, both — PR #20 |
| adversarial/F-07 | closed | boundary reads each method once, `Delivered` — PR #16 |
| adversarial/F-08 | closed | scan and map insert de-quadratised — PR #22 |
| adversarial/extra-origin-term | closed | sealed term is exactly two scalars — PR #22 |
| adversarial/extra-nonstring-keys | closed | Go rejects non-string keys — PR #22 |
| adversarial/budgets | open | D-11 |
| claim/F-01 | closed | `reviews/` excluded from the subject — PR #15 |
| claim/F-02 | partial | schema accepts `repo_type: spec` (PR #22); the release path still does not compute the subject — D-12 |
| claim/F-03 | partial | four divergences closed with vectors (PR #17, #20, #22, #23); the general equivalence claim remains bounded by the corpus — D-2 |
| claim/F-04 | partial | serialization quoting closed (PR #18); addressing for names containing `.` — D-6 |
| claim/F-05 | open | D-9 |
| claim/F-06 | partial | counts corrected and gated (PR #21, #24); the gate reads `INV-nnn` and not RFC-2119 clauses — D-10 |
| claim/F-07 | closed | status-claims gate — PR #21, #24 |
| claim/F-08 | open | D-13 |
| claim/F-09 | closed | release docs replaced — PR #22 |
| claim/F-10 | open | D-14 |
| claim/F-11 | partial | arity enforced (PR #17); `scalar_type` — D-1 |
| semantic/F-01 | open | D-3 |
| semantic/F-02 | open | D-4 |
| semantic/F-03 | open | D-5 |
| semantic/F-04 | open | D-1 |
| semantic/F-05 | open | D-2 |
| semantic/F-06 | partial | nested expansion and innermost origin pinned by `materialization/015` (PR #23); adoption semantics unwritten — D-3 |
| semantic/F-07 | partial | 64-level bound in both (PR #23); §2.3's finiteness argument still assumes no references — D-7 |
| semantic/F-08 | partial | term shape tightened (PR #22); the machine schema and the validators have not been compared — D-8 |
| semantic/F-09 | open | D-6 |
| semantic/F-10 | partial | keys closed (PR #18); the exact decimal form of a float is unspecified — D-1 |
| semantic/F-11 | partial | anchors, aliases, duplicate and non-string keys refused (PR #20, #22); tags, `!!binary`, version directives — D-7 |
| semantic/F-12 | open | D-8 |
| semantic/F-13 | open | D-8 |
| semantic/F-14 | open | D-9 |
| semantic/F-15 | closed | status-claims gate — PR #21, #24 |

**Totals: 14 closed, 10 partial, 13 open.** A partial is not a closed one; the
count that matters for "is this review discharged" is 14 of 37.
