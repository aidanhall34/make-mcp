#!/usr/bin/env python3
"""
Classify changed files and emit GitHub Actions outputs.

Changed files are sourced either from the repository via --git-base (preferred)
or from stdin, one per line.  Rules are evaluated in the order given on the
command line:

1. Files matching any --ignore pattern are skipped entirely.
2. Files matching a --rule NAME:PATTERN set the output NAME=true.
   The first matching rule wins; later rules are not evaluated for that file.
3. Files that match no rule and no ignore pattern set --default NAME=true.

Results are appended to $GITHUB_OUTPUT when that variable is set,
otherwise they are written to stdout.  A summary line per output is always
printed to stderr so the values appear in the Actions log.

Git base resolution (--git-base)
---------------------------------
When --git-base is given the script resolves a base commit and diffs HEAD
against it using GitPython.  Resolution order:

  1. If the value starts with "origin/" the remote is fetched first.
  2. If the value is a valid commit ref it is used directly.
  3. Falls back to HEAD^ if a parent commit exists.
  4. Falls back to the empty-tree SHA (first commit with no parents).

This makes the script safe to run in act (local GitHub Actions runner) where
the remote is unreachable and github.event.before is not set.

Examples
--------
# CI: diff against origin/main, anything outside the ignore list triggers CI
python3 detect_changes.py \\
    --git-base origin/main \\
    --ignore-file .github/scripts/ignore-patterns.txt \\
    --ignore 'wiki/*' \\
    --default run-code-ci

# Release: diff against the push base SHA, wiki files sync the wiki
python3 detect_changes.py \\
    --git-base "$BEFORE_SHA" \\
    --ignore-file .github/scripts/ignore-patterns.txt \\
    --rule 'run-wiki-sync:wiki/*' \\
    --default run-code-release

# Stdin fallback (e.g. unit tests, manual piping)
git diff --name-only origin/main...HEAD | \\
  python3 detect_changes.py \\
    --ignore-file .github/scripts/ignore-patterns.txt \\
    --default run-code-ci
"""

from __future__ import annotations

import argparse
import fnmatch
import os
import sys
from pathlib import Path

# ---------------------------------------------------------------------------
# Empty-tree SHA — the canonical parent of the very first commit in any repo.
# Using it as a diff base lists every file in HEAD as "added".
# ---------------------------------------------------------------------------
_EMPTY_TREE_SHA = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"


def matches(path: str, pattern: str) -> bool:
    """Return True if *path* matches *pattern* (fnmatch glob semantics)."""
    return fnmatch.fnmatch(path, pattern)


def load_ignore_file(path: str) -> list[str]:
    """Return non-blank, non-comment lines from *path*."""
    lines = []
    for line in Path(path).read_text().splitlines():
        stripped = line.strip()
        if stripped and not stripped.startswith("#"):
            lines.append(stripped)
    return lines


def parse_rule(value: str) -> tuple[str, str]:
    """Parse ``NAME:PATTERN`` into ``(name, pattern)``."""
    name, sep, pattern = value.partition(":")
    if not sep or not name or not pattern:
        raise argparse.ArgumentTypeError(
            f"--rule must be NAME:PATTERN, got: {value!r}"
        )
    return (name, pattern)


def resolve_base(repo, base: str) -> str:  # type: ignore[no-untyped-def]
    """Resolve *base* to a commit SHA using GitPython with fallbacks.

    Resolution order:
      1. If base starts with "origin/", fetch the remote branch first then
         resolve.  A failed fetch is silently ignored so act runs still work.
      2. If base is a non-empty, non-zero SHA/ref, try resolving it directly.
      3. Fall back to HEAD^ when a parent commit exists.
      4. Fall back to the empty-tree SHA for repos with a single commit.
    """
    _ZERO_SHA = "0" * 40

    # Step 1 / 2 — try the supplied value.
    if base and base != _ZERO_SHA:
        if base.startswith("origin/"):
            branch = base.split("/", 1)[1]
            try:
                repo.remote("origin").fetch(branch)
            except Exception:
                pass  # unreachable remote (act, offline) — continue with fallbacks

        try:
            return repo.commit(base).hexsha
        except Exception:
            pass

    # Step 3 — fall back to HEAD^.
    try:
        return repo.commit("HEAD^").hexsha
    except Exception:
        pass

    # Step 4 — fall back to the empty tree (single-commit repo).
    return _EMPTY_TREE_SHA


def changed_files_from_repo(base_ref: str, repo_path: str = ".") -> list[str]:
    """Return changed file paths between *base_ref* and HEAD via GitPython."""
    import git  # imported here so the rest of the module works without gitpython

    repo = git.Repo(repo_path, search_parent_directories=True)
    base_sha = resolve_base(repo, base_ref)
    output = repo.git.diff(base_sha, "HEAD", name_only=True)
    return output.splitlines() if output else []


def detect(
    changed_files: list[str],
    rules: list[tuple[str, str]],
    ignore_patterns: list[str],
    default_name: str | None,
) -> dict[str, bool]:
    """Return a mapping of output names to their triggered state.

    All output names are initialised to False so callers always receive a
    complete set of outputs regardless of which files changed.
    """
    outputs: dict[str, bool] = {}
    for name, _ in rules:
        outputs.setdefault(name, False)
    if default_name:
        outputs.setdefault(default_name, False)

    for raw in changed_files:
        path = raw.strip()
        if not path:
            continue

        if any(matches(path, pat) for pat in ignore_patterns):
            continue

        matched = False
        for name, pattern in rules:
            if matches(path, pattern):
                outputs[name] = True
                matched = True
                break

        if not matched and default_name:
            outputs[default_name] = True

    return outputs


def write_outputs(outputs: dict[str, bool]) -> None:
    """Append key=value pairs to $GITHUB_OUTPUT or print to stdout."""
    lines = [f"{name}={str(value).lower()}" for name, value in outputs.items()]
    github_output = os.environ.get("GITHUB_OUTPUT")
    if github_output:
        with open(github_output, "a") as fh:
            fh.write("\n".join(lines) + "\n")
    else:
        for line in lines:
            print(line)
    for line in lines:
        print(f"Detected {line}", file=sys.stderr)


def main(argv: list[str] | None = None) -> None:
    parser = argparse.ArgumentParser(
        description=__doc__,
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    parser.add_argument(
        "--git-base",
        metavar="REF",
        dest="git_base",
        help=(
            "Base commit ref or SHA to diff HEAD against. "
            "Supports 'origin/<branch>' (fetches first). "
            "Mutually exclusive with stdin input."
        ),
    )
    parser.add_argument(
        "--rule",
        metavar="NAME:PATTERN",
        action="append",
        type=parse_rule,
        default=[],
        dest="rules",
        help=(
            "Files matching PATTERN set output NAME=true. "
            "First matching rule wins. Repeatable."
        ),
    )
    parser.add_argument(
        "--ignore-file",
        metavar="FILE",
        dest="ignore_file",
        help="Path to a file of ignore patterns, one per line (# comments allowed).",
    )
    parser.add_argument(
        "--ignore",
        metavar="PATTERN",
        action="append",
        default=[],
        dest="ignore_patterns",
        help="Files matching this pattern are ignored entirely. Repeatable.",
    )
    parser.add_argument(
        "--default",
        metavar="NAME",
        dest="default_name",
        help=(
            "Output to set true when a file matches no rule "
            "and no ignore pattern."
        ),
    )
    args = parser.parse_args(argv)

    ignore_patterns = load_ignore_file(args.ignore_file) if args.ignore_file else []
    ignore_patterns += args.ignore_patterns

    if args.git_base is not None:
        changed_files = changed_files_from_repo(args.git_base)
    else:
        changed_files = sys.stdin.read().splitlines()

    outputs = detect(changed_files, args.rules, ignore_patterns, args.default_name)
    write_outputs(outputs)


if __name__ == "__main__":
    main()
