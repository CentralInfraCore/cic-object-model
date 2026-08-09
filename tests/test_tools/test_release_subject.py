"""The release subject digest, and the tamper property it exists for.

Built against a temporary git repository rather than this one, so the tests are
hermetic and can assert what happens when a file CHANGES without editing the
tree they are running in.

The property under test is the one the audit found missing: a signature over a
release must cover the normative product. Before this, the release signed a hash
of `project.yaml` while `canonical_source_file` pointed at `spec/index.yaml` —
one file, which names `../SPEC.md` by path and binds nothing about its content.
"""

import importlib.util
import subprocess
from pathlib import Path

import pytest

TOOL = Path(__file__).resolve().parents[2] / "tools" / "release_subject.py"


def load_tool():
    spec = importlib.util.spec_from_file_location("release_subject", TOOL)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


rs = load_tool()


@pytest.fixture
def repo(tmp_path):
    """A minimal repository standing in for the normative product."""
    subprocess.run(["git", "init", "-q"], cwd=tmp_path, check=True)
    (tmp_path / "SPEC.md").write_text("**Model version: 0.2**\n", encoding="utf-8")
    (tmp_path / "spec").mkdir()
    (tmp_path / "spec" / "index.yaml").write_text("version: '0.2'\n", encoding="utf-8")
    (tmp_path / "conformance").mkdir()
    (tmp_path / "conformance" / "expected.yaml").write_text(
        "values: 1\n", encoding="utf-8"
    )
    (tmp_path / "project.yaml").write_text(
        "metadata:\n  buildHash: 'TBD'\n", encoding="utf-8"
    )
    subprocess.run(["git", "add", "-A"], cwd=tmp_path, check=True)
    return tmp_path


def write_manifest(root: Path) -> None:
    (root / "MANIFEST.sha256").write_text(rs.build_manifest(root), encoding="utf-8")
    subprocess.run(["git", "add", "-A"], cwd=root, check=True)


def set_subject(root: Path, value: str) -> None:
    (root / "project.yaml").write_text(
        f"metadata:\n  buildHash: '{value}'\n", encoding="utf-8"
    )
    subprocess.run(["git", "add", "-A"], cwd=root, check=True)


def seal(root: Path) -> str:
    """Bring the repository to a state that verifies, and return the subject."""
    write_manifest(root)
    subject = rs.subject_digest(rs.build_manifest(root))
    set_subject(root, subject)
    write_manifest(root)
    return subject


def test_a_sealed_repository_verifies(repo, capsys):
    seal(repo)
    assert rs.cmd_verify(repo) == 0
    out = capsys.readouterr().out
    assert "does NOT establish who produced the tree" in out


def test_the_subject_is_stable_under_writing_it(repo):
    """Writing the answer must not change the question.

    `project.yaml` carries the digest, so covering it would mean every release
    changed the value it had just recorded. `MANIFEST.sha256` is excluded for
    the same reason: it cannot hash itself.
    """
    before = rs.subject_digest(rs.build_manifest(repo))
    set_subject(repo, before)
    after = rs.subject_digest(rs.build_manifest(repo))
    assert before == after


@pytest.mark.parametrize(
    "target",
    ["SPEC.md", "spec/index.yaml", "conformance/expected.yaml"],
)
def test_one_changed_byte_anywhere_breaks_the_subject(repo, target):
    """The tamper property, stated per part of the normative product."""
    subject = seal(repo)
    path = repo / target
    path.write_text(path.read_text(encoding="utf-8") + "\n", encoding="utf-8")

    assert (
        rs.subject_digest(rs.build_manifest(repo)) != subject
    ), f"changing {target} did not change the release subject"
    assert rs.cmd_verify(repo) == 1


def test_a_new_file_breaks_the_subject(repo):
    """Adding is tampering too.

    A digest over a fixed file list would miss this, which is the same shape as
    a manifest check that verifies only the files it lists.
    """
    subject = seal(repo)
    (repo / "conformance" / "extra.yaml").write_text("values: 2\n", encoding="utf-8")
    subprocess.run(["git", "add", "-A"], cwd=repo, check=True)
    assert rs.subject_digest(rs.build_manifest(repo)) != subject


def test_a_missing_claim_fails_rather_than_passing_quietly(repo, capsys):
    """An unsealed repository must not verify.

    `TBD` is what the descriptor shipped with for the whole of 0.1 and most of
    0.2, and a verifier that treats a placeholder as "nothing to check here"
    reports success for a release that claims nothing at all.
    """
    write_manifest(repo)
    assert rs.cmd_verify(repo) == 1
    assert "claims no release subject" in capsys.readouterr().out


def test_a_wrong_claim_names_both_values(repo, capsys):
    seal(repo)
    set_subject(repo, "0" * 64)
    write_manifest(repo)
    assert rs.cmd_verify(repo) == 1
    out = capsys.readouterr().out
    assert "claims" in out and "this tree computes" in out


def test_the_manifest_matches_the_shell_pipeline_byte_for_byte(repo):
    """The Python and the `make manifest-update` pipeline must agree exactly.

    They are sorted over the WHOLE LINE, digest first, because that is what
    `LC_ALL=C sort` does to `sha256sum` output. Sorting by path instead looks
    more sensible, produces a different file, and made every verification fail
    on ordering alone — which is how this test came to exist.
    """
    shell = subprocess.run(
        "git ls-files -z | xargs -0 sha256sum "
        '| grep -v "MANIFEST.sha256" | grep -v "project.yaml" | LC_ALL=C sort',
        cwd=repo,
        shell=True,
        check=True,
        capture_output=True,
        text=True,
    ).stdout
    assert rs.build_manifest(repo) == shell


# ---------------------------------------------------------------------------
# INV-046 — the external review record
# ---------------------------------------------------------------------------


def test_a_tree_with_no_review_record_fails(repo, capsys):
    seal(repo)
    assert rs.cmd_review(repo) == 1
    out = capsys.readouterr().out
    assert "no external review record" in out
    assert "docs/external-review.md" in out


def write_review(root: Path, subject: str) -> None:
    """Write a review record AND track it.

    The `git add` is the whole point. Without it the record is invisible to
    `git ls-files`, so it is invisible to the subject calculation — and the test
    passed while the real workflow, where the record arrives committed in a pull
    request, could not work at all.

    That was a false positive in a test written specifically to check this gate,
    on a day spent removing false positives. An external audit found it by
    computing the two digests; nothing here did.
    """
    reviews = root / rs.REVIEWS
    reviews.mkdir(exist_ok=True)
    (reviews / f"{subject}.md").write_text("# Review\n", encoding="utf-8")
    subprocess.run(["git", "add", "-A"], cwd=root, check=True)


def test_a_tracked_record_for_this_tree_passes(repo, capsys):
    subject = seal(repo)
    write_review(repo, subject)
    assert rs.cmd_review(repo) == 0
    assert subject in capsys.readouterr().out


def test_recording_a_review_does_not_change_the_subject(repo):
    """The fixed point the gate depends on.

    `reviews/` is excluded from the subject because a record is a claim ABOUT
    the subject, named for it. While it was included, adding the record changed
    the digest the record's own filename referred to, and adding the newly
    required record changed it again: no tree could carry a review of itself
    short of a SHA-256 fixed point.
    """
    subject = seal(repo)
    write_review(repo, subject)
    assert rs.subject_digest(rs.build_manifest(repo)) == subject

    # And a second record does not move it either, so a tree can accumulate
    # reviews from several angles — which is what three prompts in three
    # threads produces.
    (repo / rs.REVIEWS / f"{subject}.adversarial.md").write_text(
        "# 2\n", encoding="utf-8"
    )
    subprocess.run(["git", "add", "-A"], cwd=repo, check=True)
    assert rs.subject_digest(rs.build_manifest(repo)) == subject


def test_a_record_for_a_different_tree_does_not_count(repo, capsys):
    """The binding is the point.

    Without it, a review record keeps asserting something about a tree that has
    since changed — "it was reviewed" surviving every commit after the review,
    which is how a review becomes a thing that was once done rather than a thing
    that is true.
    """
    subject = seal(repo)
    write_review(repo, subject)
    assert rs.cmd_review(repo) == 0

    # One byte of the specification later, the record is about a different tree.
    spec = repo / "SPEC.md"
    spec.write_text(spec.read_text(encoding="utf-8") + "\n", encoding="utf-8")
    assert rs.cmd_review(repo) == 1
    out = capsys.readouterr().out
    assert "review a different tree" in out
