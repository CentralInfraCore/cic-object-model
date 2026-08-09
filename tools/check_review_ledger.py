#!/usr/bin/env python3
"""Every review finding is accounted for, and every open one names a decision.

A review produces findings; a repository closes some and defers others. The
deferral is the dangerous half: a finding that is neither fixed nor written down
has not been decided against, it has been forgotten, and nothing about the tree
looks different afterwards.

This reconciles three lists that are supposed to describe the same set:

  the findings raised in reviews/<subject>.{thread}.md
  the rows of  reviews/<subject>.disposition.md
  the decisions in docs/pending-decisions.md

and fails when they disagree. Specifically:

  * a finding with no ledger row — raised and then lost
  * a ledger row for a finding nobody raised — a row that outlived its finding
  * an open or partial row naming a decision that does not exist
  * a decision nothing refers to — either the finding it came from was closed
    without updating the row, or the decision was invented

The last two are what happened here. The decisions document listed twelve;
`claim/F-08` and `claim/F-10` had been raised, measured, confirmed, and then
appeared in no decision and no commit. Nothing compared the list to its source,
so nothing could say so — the same shape as the manifest that verified only the
files it listed, and the status claims that were true when written.
"""

from __future__ import annotations

import re
import sys
from pathlib import Path

THREADS = ("claim", "semantic", "adversarial")


def repo_root() -> Path:
    return Path(__file__).resolve().parent.parent


def subjects(reviews: Path) -> list[str]:
    """Every subject that has a disposition ledger."""
    return sorted(
        p.name[: -len(".disposition.md")] for p in reviews.glob("*.disposition.md")
    )


def raised(reviews: Path, subject: str) -> set[str]:
    """Findings the review threads raise, namespaced by thread.

    Namespaced because the threads number independently: `claim/F-01` and
    `adversarial/F-01` are different findings, and a ledger keyed on `F-01`
    alone would silently hold one row for two.
    """
    out: set[str] = set()
    for thread in THREADS:
        path = reviews / f"{subject}.{thread}.md"
        if not path.is_file():
            continue
        for fid in re.findall(
            r"^#{2,3} (F-\d+)", path.read_text(encoding="utf-8"), re.M
        ):
            out.add(f"{thread}/{fid}")
    return out


def ledger(path: Path) -> dict[str, tuple[str, str]]:
    """finding -> (status, where), read from the disposition table."""
    rows: dict[str, tuple[str, str]] = {}
    for line in path.read_text(encoding="utf-8").splitlines():
        m = re.match(
            r"^\| ([a-z]+/[A-Za-z0-9-]+) \| (closed|partial|open) \| (.*?) \|$", line
        )
        if m:
            rows[m.group(1)] = (m.group(2), m.group(3))
    return rows


def decisions(path: Path) -> set[str]:
    return set(re.findall(r"^## (D-\d+) —", path.read_text(encoding="utf-8"), re.M))


def main() -> int:
    root = repo_root()
    reviews = root / "reviews"
    pending = root / "docs" / "pending-decisions.md"
    known = decisions(pending)
    failures: list[str] = []
    referenced: set[str] = set()
    totals = {"closed": 0, "partial": 0, "open": 0}

    found_any = False
    for subject in subjects(reviews):
        found_any = True
        rows = ledger(reviews / f"{subject}.disposition.md")
        raised_here = raised(reviews, subject)

        # Findings raised outside the numbered headings — a section rather than
        # an F-nn — are carried in the ledger under a descriptive id. They
        # cannot be discovered automatically, so they are allowed to exist in
        # the ledger without a heading, and that indulgence is stated rather
        # than silent.
        extra_ok = {k for k in rows if not re.match(r"^[a-z]+/F-\d+$", k)}

        for fid in sorted(raised_here - set(rows)):
            failures.append(f"{subject}: {fid} was raised and has no ledger row")
        for fid in sorted(set(rows) - raised_here - extra_ok):
            failures.append(
                f"{subject}: ledger has a row for {fid}, which no thread raises"
            )

        for fid, (status, where) in sorted(rows.items()):
            totals[status] += 1
            if status == "closed":
                continue
            named = set(re.findall(r"\bD-\d+\b", where))
            if not named:
                failures.append(f"{subject}: {fid} is {status} and names no decision")
                continue
            for d in named:
                referenced.add(d)
                if d not in known:
                    failures.append(
                        f"{subject}: {fid} names {d}, which docs/pending-decisions.md does not define"
                    )

    if not found_any:
        print("no disposition ledger found; nothing to reconcile")
        return 0

    for d in sorted(known - referenced):
        failures.append(
            f"docs/pending-decisions.md defines {d}, which no open finding refers to"
        )

    print(
        f"findings: {totals['closed']} closed, {totals['partial']} partial, "
        f"{totals['open']} open, over {len(known)} decisions"
    )
    print()

    if failures:
        print("FAIL  the review ledger, the findings and the decisions disagree:")
        for f in failures:
            print(f"  {f}")
        return 1

    print(
        "ok    every finding has a row, and every open row names a decision that exists"
    )
    print()
    print("A partial is not a closed one. This checks that nothing was lost, not")
    print("that anything was solved.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
