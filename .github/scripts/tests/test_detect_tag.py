"""Unit tests for detect_tag.py."""

from __future__ import annotations

import os
import sys
from pathlib import Path

import git
import pytest

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from detect_tag import (  # noqa: E402
    SEMVER_RE,
    detect_tag,
    find_semver_tag,
    main,
    write_outputs,
)


# ---------------------------------------------------------------------------
# Fixtures
# ---------------------------------------------------------------------------


def _init_repo(tmp_path: Path) -> git.Repo:
    repo = git.Repo.init(tmp_path)
    with repo.config_writer() as cfg:
        cfg.set_value("user", "name", "Test")
        cfg.set_value("user", "email", "test@example.com")
    (tmp_path / "file.txt").write_text("init")
    repo.index.add(["file.txt"])
    repo.index.commit("initial commit")
    return repo


def _add_commit(repo: git.Repo, content: str = "change") -> git.Commit:
    path = Path(repo.working_dir) / "file.txt"
    path.write_text(content)
    repo.index.add(["file.txt"])
    return repo.index.commit(f"commit: {content}")


def _set_fetch_head(repo: git.Repo, sha: str) -> None:
    """Write a FETCH_HEAD file so rev_parse('FETCH_HEAD') resolves to sha."""
    fetch_head = Path(repo.git_dir) / "FETCH_HEAD"
    fetch_head.write_text(f"{sha}\t\tbranch 'main' of test\n")


# ---------------------------------------------------------------------------
# SEMVER_RE
# ---------------------------------------------------------------------------


class TestSemverRe:
    @pytest.mark.parametrize("tag", ["v1.0.0", "v0.0.1", "v12.34.56"])
    def test_valid_semver_tags_match(self, tag):
        assert SEMVER_RE.match(tag)

    @pytest.mark.parametrize("tag", ["1.0.0", "v1.0", "v1.0.0.0", "vfoo", ""])
    def test_invalid_tags_do_not_match(self, tag):
        assert not SEMVER_RE.match(tag)


# ---------------------------------------------------------------------------
# find_semver_tag
# ---------------------------------------------------------------------------


class TestFindSemverTag:
    @pytest.fixture()
    def repo(self, tmp_path):
        return _init_repo(tmp_path)

    def test_returns_tag_on_matching_commit(self, repo):
        sha = repo.head.commit.hexsha
        repo.create_tag("v1.2.3")
        assert find_semver_tag(repo, sha) == "v1.2.3"

    def test_returns_none_when_no_tags(self, repo):
        assert find_semver_tag(repo, repo.head.commit.hexsha) is None

    def test_returns_none_for_non_semver_tags(self, repo):
        repo.create_tag("release-1")
        repo.create_tag("latest")
        assert find_semver_tag(repo, repo.head.commit.hexsha) is None

    def test_ignores_tag_on_different_commit(self, repo):
        first_sha = repo.head.commit.hexsha
        _add_commit(repo)
        repo.create_tag("v1.0.0")  # tag is on HEAD, not first_sha
        assert find_semver_tag(repo, first_sha) is None

    def test_returns_latest_of_multiple_semver_tags(self, repo):
        sha = repo.head.commit.hexsha
        for tag in ("v1.0.0", "v1.9.0", "v1.10.0", "v2.0.0"):
            repo.create_tag(tag)
        assert find_semver_tag(repo, sha) == "v2.0.0"

    def test_numeric_sort_not_lexicographic(self, repo):
        # v1.10.0 > v1.9.0 numerically but < lexicographically
        sha = repo.head.commit.hexsha
        repo.create_tag("v1.9.0")
        repo.create_tag("v1.10.0")
        assert find_semver_tag(repo, sha) == "v1.10.0"

    def test_mixed_semver_and_non_semver_returns_semver(self, repo):
        sha = repo.head.commit.hexsha
        repo.create_tag("latest")
        repo.create_tag("v1.0.0")
        assert find_semver_tag(repo, sha) == "v1.0.0"

    def test_annotated_tag_resolved_to_commit(self, repo):
        sha = repo.head.commit.hexsha
        repo.create_tag("v3.0.0", message="release v3.0.0")  # annotated tag
        assert find_semver_tag(repo, sha) == "v3.0.0"


# ---------------------------------------------------------------------------
# detect_tag
# ---------------------------------------------------------------------------


class TestDetectTag:
    @pytest.fixture()
    def repo(self, tmp_path):
        return _init_repo(tmp_path)

    def test_finds_tag_on_fetch_head(self, repo):
        sha = repo.head.commit.hexsha
        repo.create_tag("v1.0.0")
        _set_fetch_head(repo, sha)
        result = detect_tag(repo_path=str(repo.working_dir), fetch=False)
        assert result == {"released": "true", "git_tag": "v1.0.0", "sem_ver": "1.0.0"}

    def test_falls_back_to_sha_when_fetch_head_has_no_tag(self, repo):
        first_commit = repo.head.commit
        second_commit = _add_commit(repo)
        repo.create_tag("v2.0.0", ref=first_commit)
        # FETCH_HEAD points at second commit (no tag), SHA falls back to first
        _set_fetch_head(repo, second_commit.hexsha)
        result = detect_tag(
            repo_path=str(repo.working_dir),
            sha=first_commit.hexsha,
            fetch=False,
        )
        assert result == {"released": "true", "git_tag": "v2.0.0", "sem_ver": "2.0.0"}

    def test_returns_false_when_no_tag_found(self, repo):
        _set_fetch_head(repo, repo.head.commit.hexsha)
        result = detect_tag(repo_path=str(repo.working_dir), fetch=False)
        assert result == {"released": "false", "git_tag": "", "sem_ver": ""}

    def test_returns_false_with_mismatched_sha_and_no_fetch_head_tag(self, repo):
        other_commit = _add_commit(repo)
        _set_fetch_head(repo, other_commit.hexsha)
        result = detect_tag(
            repo_path=str(repo.working_dir),
            sha="0" * 40,  # SHA that doesn't exist
            fetch=False,
        )
        assert result == {"released": "false", "git_tag": "", "sem_ver": ""}

    def test_falls_back_to_sha_when_no_fetch_head_file(self, repo):
        sha = repo.head.commit.hexsha
        repo.create_tag("v1.1.0")
        # No FETCH_HEAD file — rev_parse raises, falls through to sha
        fetch_head_file = Path(repo.git_dir) / "FETCH_HEAD"
        fetch_head_file.unlink(missing_ok=True)
        result = detect_tag(
            repo_path=str(repo.working_dir),
            sha=sha,
            fetch=False,
        )
        assert result == {"released": "true", "git_tag": "v1.1.0", "sem_ver": "1.1.0"}

    def test_sem_ver_strips_v_prefix(self, repo):
        repo.create_tag("v10.20.30")
        _set_fetch_head(repo, repo.head.commit.hexsha)
        result = detect_tag(repo_path=str(repo.working_dir), fetch=False)
        assert result["sem_ver"] == "10.20.30"

    def test_sha_none_does_not_error_when_fetch_head_empty(self, repo):
        _set_fetch_head(repo, repo.head.commit.hexsha)
        result = detect_tag(repo_path=str(repo.working_dir), sha=None, fetch=False)
        assert result["released"] == "false"


# ---------------------------------------------------------------------------
# write_outputs
# ---------------------------------------------------------------------------


class TestWriteOutputs:
    def test_prints_to_stdout_when_no_github_output(self, capsys):
        write_outputs({"released": "true", "git_tag": "v1.0.0", "sem_ver": "1.0.0"})
        out = capsys.readouterr().out
        assert "released=true" in out
        assert "git_tag=v1.0.0" in out
        assert "sem_ver=1.0.0" in out

    def test_writes_to_github_output_file(self, tmp_path, monkeypatch):
        output_file = tmp_path / "github_output"
        output_file.write_text("")
        monkeypatch.setenv("GITHUB_OUTPUT", str(output_file))
        write_outputs({"released": "false", "git_tag": "", "sem_ver": ""})
        content = output_file.read_text()
        assert "released=false" in content
        assert "git_tag=" in content

    def test_always_logs_to_stderr(self, capsys):
        write_outputs({"released": "true", "git_tag": "v2.0.0", "sem_ver": "2.0.0"})
        err = capsys.readouterr().err
        assert "Detected released=true" in err
        assert "Detected git_tag=v2.0.0" in err


# ---------------------------------------------------------------------------
# main
# ---------------------------------------------------------------------------


class TestMain:
    @pytest.fixture()
    def repo(self, tmp_path):
        return _init_repo(tmp_path)

    def test_finds_tag_via_sha_arg(self, repo, capsys):
        sha = repo.head.commit.hexsha
        repo.create_tag("v5.0.0")
        main(["--no-fetch", "--repo", str(repo.working_dir), "--sha", sha])
        out = capsys.readouterr().out
        assert "released=true" in out
        assert "git_tag=v5.0.0" in out
        assert "sem_ver=5.0.0" in out

    def test_no_tag_outputs_false(self, repo, capsys):
        main(["--no-fetch", "--repo", str(repo.working_dir)])
        out = capsys.readouterr().out
        assert "released=false" in out

    def test_writes_to_github_output(self, repo, tmp_path, monkeypatch, capsys):
        output_file = tmp_path / "github_output"
        output_file.write_text("")
        monkeypatch.setenv("GITHUB_OUTPUT", str(output_file))
        sha = repo.head.commit.hexsha
        repo.create_tag("v0.1.0")
        main(["--no-fetch", "--repo", str(repo.working_dir), "--sha", sha])
        assert "released=true" in output_file.read_text()
