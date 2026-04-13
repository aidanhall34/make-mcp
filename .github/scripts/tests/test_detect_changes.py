"""Unit tests for detect_changes.py."""

from __future__ import annotations

import argparse
import os
import sys
from io import StringIO
from pathlib import Path

import git
import pytest

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from detect_changes import (  # noqa: E402
    _EMPTY_TREE_SHA,
    changed_files_from_repo,
    detect,
    load_ignore_file,
    main,
    parse_rule,
    resolve_base,
    write_outputs,
)

SCRIPTS_DIR = Path(__file__).resolve().parents[1]
IGNORE_FILE = SCRIPTS_DIR / "ignore-patterns.txt"

# ---------------------------------------------------------------------------
# Shared ignore lists loaded from the real ignore-patterns.txt
# ---------------------------------------------------------------------------

_SHARED_IGNORE = load_ignore_file(str(IGNORE_FILE))
# README.md is not in the shared ignore file; CI suppresses it via --ignore,
# while release treats it as a wiki-sync trigger via a dedicated rule.
CI_IGNORE = ["wiki/*", "README.md", *_SHARED_IGNORE]
RELEASE_IGNORE = _SHARED_IGNORE
RELEASE_RULES = [("run-wiki-sync", "README.md"), ("run-wiki-sync", "wiki/*")]


# ---------------------------------------------------------------------------
# Fixtures
# ---------------------------------------------------------------------------


def _make_repo(tmp_path: Path) -> git.Repo:
    """Initialise a bare-minimum git repo with one commit."""
    repo = git.Repo.init(tmp_path)
    with repo.config_writer() as cfg:
        cfg.set_value("user", "name", "Test")
        cfg.set_value("user", "email", "test@example.com")
    (tmp_path / "base.txt").write_text("base")
    repo.index.add(["base.txt"])
    repo.index.commit("initial commit")
    return repo


@pytest.fixture()
def single_commit_repo(tmp_path):
    """A repo with exactly one commit (no parent)."""
    return _make_repo(tmp_path)


@pytest.fixture()
def two_commit_repo(tmp_path):
    """A repo with two commits; HEAD has a parent."""
    repo = _make_repo(tmp_path)
    (tmp_path / "second.txt").write_text("second")
    repo.index.add(["second.txt"])
    repo.index.commit("second commit")
    return repo


# ---------------------------------------------------------------------------
# load_ignore_file
# ---------------------------------------------------------------------------


class TestLoadIgnoreFile:
    def test_loads_real_ignore_file(self):
        patterns = load_ignore_file(str(IGNORE_FILE))
        assert "AGENTS.md" in patterns
        assert "pyproject.toml" in patterns
        assert "uv.lock" in patterns

    def test_strips_comments(self, tmp_path):
        f = tmp_path / "patterns.txt"
        f.write_text("# a comment\nREADME.md\n")
        assert load_ignore_file(str(f)) == ["README.md"]

    def test_strips_blank_lines(self, tmp_path):
        f = tmp_path / "patterns.txt"
        f.write_text("\nREADME.md\n\n.gitignore\n")
        assert load_ignore_file(str(f)) == ["README.md", ".gitignore"]

    def test_empty_file_returns_empty_list(self, tmp_path):
        f = tmp_path / "patterns.txt"
        f.write_text("# only comments\n\n")
        assert load_ignore_file(str(f)) == []

    def test_ignore_file_patterns_match(self, tmp_path):
        f = tmp_path / "patterns.txt"
        f.write_text("wiki/*\nREADME.md\n")
        out = detect(["wiki/Home.md", "README.md", "cmd/main.go"],
                     [], load_ignore_file(str(f)), "run")
        assert out["run"] is True  # cmd/main.go not in ignore file


# ---------------------------------------------------------------------------
# parse_rule
# ---------------------------------------------------------------------------


class TestParseRule:
    def test_valid(self):
        assert parse_rule("run-code-ci:src/*.go") == ("run-code-ci", "src/*.go")

    def test_colon_in_pattern_uses_first_split(self):
        assert parse_rule("flag:foo:bar") == ("flag", "foo:bar")

    def test_missing_colon_raises(self):
        with pytest.raises(argparse.ArgumentTypeError):
            parse_rule("no-colon")

    def test_empty_name_raises(self):
        with pytest.raises(argparse.ArgumentTypeError):
            parse_rule(":pattern")

    def test_empty_pattern_raises(self):
        with pytest.raises(argparse.ArgumentTypeError):
            parse_rule("name:")


# ---------------------------------------------------------------------------
# resolve_base
# ---------------------------------------------------------------------------


class TestResolveBase:
    def test_valid_sha_resolves_directly(self, two_commit_repo):
        first_sha = list(two_commit_repo.iter_commits())[-1].hexsha
        assert resolve_base(two_commit_repo, first_sha) == first_sha

    def test_all_zeros_falls_back_to_head_parent(self, two_commit_repo):
        result = resolve_base(two_commit_repo, "0" * 40)
        assert result == two_commit_repo.commit("HEAD^").hexsha

    def test_empty_string_falls_back_to_head_parent(self, two_commit_repo):
        result = resolve_base(two_commit_repo, "")
        assert result == two_commit_repo.commit("HEAD^").hexsha

    def test_single_commit_empty_string_falls_back_to_empty_tree(self, single_commit_repo):
        result = resolve_base(single_commit_repo, "")
        assert result == _EMPTY_TREE_SHA

    def test_single_commit_all_zeros_falls_back_to_empty_tree(self, single_commit_repo):
        result = resolve_base(single_commit_repo, "0" * 40)
        assert result == _EMPTY_TREE_SHA

    def test_invalid_ref_falls_back_to_head_parent(self, two_commit_repo):
        result = resolve_base(two_commit_repo, "nonexistent-branch")
        assert result == two_commit_repo.commit("HEAD^").hexsha

    def test_origin_prefix_unreachable_remote_falls_back(self, two_commit_repo):
        # No remote named "origin" — fetch silently fails, falls back to HEAD^.
        result = resolve_base(two_commit_repo, "origin/main")
        assert result == two_commit_repo.commit("HEAD^").hexsha

    def test_origin_prefix_single_commit_falls_back_to_empty_tree(self, single_commit_repo):
        result = resolve_base(single_commit_repo, "origin/main")
        assert result == _EMPTY_TREE_SHA


# ---------------------------------------------------------------------------
# changed_files_from_repo
# ---------------------------------------------------------------------------


class TestChangedFilesFromRepo:
    @pytest.fixture()
    def repo_with_new_file(self, tmp_path):
        repo = _make_repo(tmp_path)
        base_sha = repo.head.commit.hexsha
        (tmp_path / "added.txt").write_text("new content")
        repo.index.add(["added.txt"])
        repo.index.commit("add file")
        return repo, base_sha, tmp_path

    def test_returns_added_file(self, repo_with_new_file):
        repo, base_sha, path = repo_with_new_file
        files = changed_files_from_repo(base_sha, str(path))
        assert files == ["added.txt"]

    def test_empty_diff_returns_empty_list(self, repo_with_new_file):
        repo, _, path = repo_with_new_file
        head_sha = repo.head.commit.hexsha
        files = changed_files_from_repo(head_sha, str(path))
        assert files == []

    def test_fallback_base_used_when_sha_invalid(self, repo_with_new_file):
        # Passing an empty string triggers HEAD^ fallback, which still diffs.
        repo, _, path = repo_with_new_file
        files = changed_files_from_repo("", str(path))
        assert "added.txt" in files

    def test_diff_against_empty_tree_lists_all_files(self, single_commit_repo):
        # Empty string on a single-commit repo → empty-tree base → all files shown.
        files = changed_files_from_repo("", str(single_commit_repo.working_dir))
        assert "base.txt" in files


# ---------------------------------------------------------------------------
# detect — core behaviour
# ---------------------------------------------------------------------------


class TestDetect:
    def test_empty_file_list_all_false(self):
        out = detect([], [("run-ci", "*.go")], [], "run-default")
        assert out == {"run-ci": False, "run-default": False}

    def test_ignored_file_triggers_nothing(self):
        out = detect(["README.md"], [("run-ci", "*.go")], ["README.md"], "run-default")
        assert out == {"run-ci": False, "run-default": False}

    def test_rule_match_sets_output(self):
        out = detect(["pkg/foo.go"], [("run-ci", "*.go")], [], None)
        assert out["run-ci"] is True

    def test_unmatched_file_triggers_default(self):
        out = detect(["some/file.txt"], [], [], "run-default")
        assert out["run-default"] is True

    def test_first_rule_wins(self):
        out = detect(
            ["wiki/page.md"],
            [("run-wiki", "wiki/*"), ("run-all", "*.md")],
            [],
            None,
        )
        assert out["run-wiki"] is True
        assert out["run-all"] is False

    def test_ignore_takes_precedence_over_rule(self):
        out = detect(
            ["wiki/page.md"],
            [("run-wiki", "wiki/*")],
            ["wiki/*"],
            "run-default",
        )
        assert out == {"run-wiki": False, "run-default": False}

    def test_blank_and_whitespace_lines_skipped(self):
        out = detect(["", "  ", "\t"], [], [], "run-default")
        assert out["run-default"] is False

    def test_all_output_names_initialised_to_false(self):
        out = detect([], [("a", "*.go"), ("b", "*.py")], [], "c")
        assert out == {"a": False, "b": False, "c": False}

    def test_multiple_files_all_ignored(self):
        out = detect(["README.md", "LICENSE"], [], ["README.md", "LICENSE"], "run")
        assert out["run"] is False

    def test_multiple_files_some_trigger_default(self):
        out = detect(["README.md", "cmd/main.go"], [], ["README.md"], "run")
        assert out["run"] is True


# ---------------------------------------------------------------------------
# write_outputs
# ---------------------------------------------------------------------------


class TestWriteOutputs:
    def test_prints_to_stdout_when_no_github_output(self, capsys):
        write_outputs({"run-ci": True, "run-wiki": False})
        out = capsys.readouterr().out
        assert "run-ci=true" in out
        assert "run-wiki=false" in out

    def test_writes_to_github_output_file(self, tmp_path, monkeypatch):
        output_file = tmp_path / "github_output"
        output_file.write_text("")
        monkeypatch.setenv("GITHUB_OUTPUT", str(output_file))
        write_outputs({"run-ci": True})
        assert "run-ci=true" in output_file.read_text()

    def test_always_logs_to_stderr(self, capsys):
        write_outputs({"flag": False})
        err = capsys.readouterr().err
        assert "Detected flag=false" in err


# ---------------------------------------------------------------------------
# main — stdin path
# ---------------------------------------------------------------------------


class TestMainStdin:
    def test_stdin_triggers_default(self, monkeypatch, capsys):
        monkeypatch.setattr("sys.stdin", StringIO("cmd/main.go\n"))
        main(["--default", "run-code-ci"])
        out = capsys.readouterr().out
        assert "run-code-ci=true" in out

    def test_stdin_all_ignored_outputs_false(self, monkeypatch, capsys):
        monkeypatch.setattr("sys.stdin", StringIO("AGENTS.md\n"))
        main([
            "--ignore-file", str(IGNORE_FILE),
            "--default", "run-code-ci",
        ])
        out = capsys.readouterr().out
        assert "run-code-ci=false" in out

    def test_stdin_rule_match(self, monkeypatch, capsys):
        monkeypatch.setattr("sys.stdin", StringIO("wiki/Home.md\n"))
        main(["--rule", "run-wiki-sync:wiki/*", "--default", "run-code-release"])
        out = capsys.readouterr().out
        assert "run-wiki-sync=true" in out
        assert "run-code-release=false" in out


# ---------------------------------------------------------------------------
# main — --git-base path
# ---------------------------------------------------------------------------


class TestMainGitBase:
    def test_git_base_detects_added_file(self, two_commit_repo, monkeypatch, capsys):
        monkeypatch.chdir(two_commit_repo.working_dir)
        first_sha = list(two_commit_repo.iter_commits())[-1].hexsha
        main(["--git-base", first_sha, "--default", "run-code-ci"])
        out = capsys.readouterr().out
        assert "run-code-ci=true" in out

    def test_git_base_empty_string_falls_back(self, two_commit_repo, monkeypatch, capsys):
        monkeypatch.chdir(two_commit_repo.working_dir)
        main(["--git-base", "", "--default", "run-code-ci"])
        # HEAD^ diff always has at least one file in two_commit_repo
        out = capsys.readouterr().out
        assert "run-code-ci=true" in out

    def test_git_base_no_changes_outputs_false(self, two_commit_repo, monkeypatch, capsys):
        monkeypatch.chdir(two_commit_repo.working_dir)
        head_sha = two_commit_repo.head.commit.hexsha
        main(["--git-base", head_sha, "--default", "run-code-ci"])
        out = capsys.readouterr().out
        assert "run-code-ci=false" in out


# ---------------------------------------------------------------------------
# detect — CI-style configuration
# ---------------------------------------------------------------------------


class TestCIConfig:
    def _run(self, files: list[str]) -> dict[str, bool]:
        return detect(files, [], CI_IGNORE, "run-code-ci")

    @pytest.mark.parametrize(
        "path",
        [
            "cmd/main.go",
            "makefile",
            "dev/makefiles/build.mk",
            ".github/workflows/ci.yml",
            "Dockerfile",
            "go.mod",
        ],
    )
    def test_code_files_trigger_ci(self, path):
        assert self._run([path])["run-code-ci"] is True

    @pytest.mark.parametrize(
        "path",
        [
            "wiki/Home.md",
            "wiki/Metrics.md",
            "README.md",
            "AGENTS.md",
            "CLAUDE.md",
            "SECURITY.md",
            "CONTRIBUTING.md",
            "LICENSE",
            ".mcp.json",
            ".gemini/config",
            ".vscode/settings.json",
            ".claude/settings.json",
            ".codex/config",
            ".markdownlint.json",
            ".markdownlint.yaml",
            ".yamllint.yml",
            "checkmake.ini",
            ".gitignore",
            ".dockerignore",
            ".nvmrc",
            "pyproject.toml",
            "uv.lock",
        ],
    )
    def test_ignored_files_do_not_trigger_ci(self, path):
        assert self._run([path])["run-code-ci"] is False

    def test_mixed_ignored_and_code_files(self):
        assert self._run(["README.md", "pkg/server.go"])["run-code-ci"] is True

    def test_all_ignored_no_ci(self):
        assert self._run(["README.md", "wiki/Home.md", ".gitignore"])["run-code-ci"] is False


# ---------------------------------------------------------------------------
# detect — Release-style configuration
# ---------------------------------------------------------------------------


class TestReleaseConfig:
    def _run(self, files: list[str]) -> dict[str, bool]:
        return detect(files, RELEASE_RULES, RELEASE_IGNORE, "run-code-release")

    def test_wiki_file_triggers_wiki_sync_only(self):
        out = self._run(["wiki/Home.md"])
        assert out["run-wiki-sync"] is True
        assert out["run-code-release"] is False

    def test_code_file_triggers_release_only(self):
        out = self._run(["cmd/main.go"])
        assert out["run-wiki-sync"] is False
        assert out["run-code-release"] is True

    def test_readme_triggers_wiki_sync_only(self):
        out = self._run(["README.md"])
        assert out["run-wiki-sync"] is True
        assert out["run-code-release"] is False

    def test_wiki_and_code_both_trigger(self):
        out = self._run(["wiki/Home.md", "cmd/main.go"])
        assert out["run-wiki-sync"] is True
        assert out["run-code-release"] is True

    def test_wiki_and_ignored_only_wiki_sync(self):
        out = self._run(["wiki/Home.md", "README.md"])
        assert out["run-wiki-sync"] is True
        assert out["run-code-release"] is False

    @pytest.mark.parametrize(
        "path",
        [
            "AGENTS.md",
            "CLAUDE.md",
            "pyproject.toml",
            "uv.lock",
            ".markdownlint.json",
        ],
    )
    def test_shared_ignored_files_trigger_neither(self, path):
        out = self._run([path])
        assert out == {"run-wiki-sync": False, "run-code-release": False}
