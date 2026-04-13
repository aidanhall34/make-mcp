#!/usr/bin/env python3
"""
Detect the semantic-release tag created for the current push.

@semantic-release/git pushes a changelog commit *after* the tag is created,
so the tag points at that child commit (FETCH_HEAD after fetching origin/main),
not at GITHUB_SHA.  This script:

  1. Fetches --tags and the tip of origin/main so FETCH_HEAD is the changelog
     commit (skipped when --no-fetch is given, e.g. in unit tests).
  2. Looks for a semver tag (vX.Y.Z) pointing at FETCH_HEAD.
  3. Falls back to --sha (GITHUB_SHA) if no tag is found on FETCH_HEAD.
  4. Writes released, git_tag, and sem_ver to $GITHUB_OUTPUT (or stdout).

Examples
--------
# Production — called from GitHub Actions after semantic-release runs.
python3 detect_tag.py --sha "$GITHUB_SHA"

# Local / unit-test — skip the fetch, point at a known repo.
python3 detect_tag.py --sha "$SHA" --no-fetch --repo /path/to/repo
"""

from __future__ import annotations

import argparse
import os
import re
import sys

import git

SEMVER_RE = re.compile(r"^v\d+\.\d+\.\d+$")


def find_semver_tag(repo: git.Repo, commit_sha: str) -> str | None:
    """Return the latest vX.Y.Z tag pointing at *commit_sha*, or None.

    Tags are sorted numerically (so v1.10.0 > v1.9.0) and the highest is
    returned.  Both lightweight and annotated tags are resolved to their
    target commit before comparison.
    """
    matched = [
        tag.name
        for tag in repo.tags
        if SEMVER_RE.match(tag.name) and tag.commit.hexsha == commit_sha
    ]
    if not matched:
        return None
    matched.sort(key=lambda n: tuple(int(x) for x in n[1:].split(".")))
    return matched[-1]


def detect_tag(
    repo_path: str = ".",
    sha: str | None = None,
    fetch: bool = True,
) -> dict[str, str]:
    """Detect the release tag and return GitHub Actions output key/value pairs.

    Returns a dict with keys ``released``, ``git_tag``, and ``sem_ver``.
    """
    repo = git.Repo(repo_path, search_parent_directories=True)

    if fetch:
        # Mirrors: git fetch --tags origin main
        # After this FETCH_HEAD points at the tip of origin/main, which is the
        # changelog commit pushed by @semantic-release/git.
        repo.git.fetch("--tags", "origin", "main")

    git_tag: str | None = None

    # Primary: look for the tag on FETCH_HEAD (the changelog commit).
    try:
        fetch_head_sha = repo.git.rev_parse("FETCH_HEAD")
        git_tag = find_semver_tag(repo, fetch_head_sha)
    except git.GitCommandError:
        pass

    # Fallback: look for the tag directly on GITHUB_SHA.
    if git_tag is None and sha:
        git_tag = find_semver_tag(repo, sha)

    if git_tag:
        return {
            "released": "true",
            "git_tag": git_tag,
            "sem_ver": git_tag[1:],  # strip leading 'v'
        }
    return {
        "released": "false",
        "git_tag": "",
        "sem_ver": "",
    }


def write_outputs(outputs: dict[str, str]) -> None:
    """Append key=value pairs to $GITHUB_OUTPUT or print to stdout."""
    lines = [f"{k}={v}" for k, v in outputs.items()]
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
        "--sha",
        metavar="SHA",
        help="Fallback commit SHA to search for a tag (GITHUB_SHA).",
    )
    parser.add_argument(
        "--no-fetch",
        action="store_true",
        dest="no_fetch",
        help="Skip 'git fetch --tags origin main' (useful in tests).",
    )
    parser.add_argument(
        "--repo",
        metavar="PATH",
        default=".",
        help="Path to the git repository (default: .).",
    )
    args = parser.parse_args(argv)

    outputs = detect_tag(
        repo_path=args.repo,
        sha=args.sha,
        fetch=not args.no_fetch,
    )
    write_outputs(outputs)


if __name__ == "__main__":
    main()
