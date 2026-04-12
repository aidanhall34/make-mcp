# Contributing

## Requirements

- Go 1.25+
- GNU Make
- Node.js >= 24.14.1 (for the release pipeline and commit linting)
- Docker (for integration tests and container builds)

## Setup

```sh
git clone https://github.com/aidanhall34/make-mcp.git
cd make-mcp
make setup   # installs git hooks and Node.js and Python dev dependencies
```

`make setup` installs a pre-commit hook that runs linting, unit tests, and
security scans before every commit.

## Development workflow

```sh
make tests       # unit tests + benchmarks (run after every .go change)
make lint        # all linters and validators
make build       # binaries + containers
make integration # full MCP protocol integration test against a live container
```

Tests are written before implementation. Run `make tests` after every change to
a Go source file.

## Commit messages

This project uses [Conventional Commits](https://www.conventionalcommits.org/).
Commit messages are enforced by commitlint via the pre-commit hook and
determine the next semver version at release time via semantic-release.

Common prefixes:

| Prefix | Effect |
|---|---|
| `feat:` | minor version bump |
| `fix:` | patch version bump |
| `feat!:` / `fix!:` / `BREAKING CHANGE:` | major version bump |
| `chore:`, `docs:`, `test:`, `refactor:` | no version bump |

Keep the subject line under 72 characters. Use the body for context.

## Pull requests

- One logical change per PR.
- All CI checks must pass: lint, unit tests, security scans, build, smoke tests,
  and integration tests.
- PRs are squash-merged. The PR title becomes the commit message on `main`, so
  it must follow the Conventional Commits format.

## Running the full CI suite locally

```sh
# Requires act (https://github.com/nektos/act)
make act
```

Or run the individual stages directly:

```sh
make lint
make tests
make scan-secrets scan-vulnerabilities
make build
make integration
```
