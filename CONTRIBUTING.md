# Contributing

## Requirements

- Go 1.25+
- GNU Make
- Node.js >= 24.14.1 (for the release pipeline and commit linting)
- Docker (for integration tests, container builds, and linters that run in containers)
- Python 3 with `uv` (for `yamllint` — installed automatically by `make setup`)

The following tools are invoked via Docker and do not need to be installed locally:

- `hadolint` — Dockerfile linter
- `checkmake` — Makefile linter
- `trufflehog` — secrets scanner
- `trivy` — vulnerability scanner

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

## Testing

### Unit tests and benchmarks

`make tests` runs both `unit-tests` and `bench`:

```sh
make unit-tests   # go test -race -cover -coverprofile=coverage.out ./...
make bench        # go test -bench=. -benchmem -run='^$' ./...
make tests        # unit-tests + bench
```

Every package under `./cmd`, `./pkg`, and `./internal` enforces a per-package
statement-coverage threshold via `TestMain`. The threshold is declared in each
package's `testmain_test.go`. Per-file JSON coverage is emitted to stderr on
every run:

```json
{"file":"github.com/aidanhall34/make-mcp/pkg/runner/runner.go","coverage":1,"pass":true}
{"coverage":1,"threshold":0.95,"pass":true}
```

When adding a new package, include a `testmain_test.go` that calls
`testtel.RunMain(m, threshold)` with a threshold starting at `0.95`. Adjust
downward only when dead code (e.g., `main()` wiring, OTel instrument-creation
errors, `json.Marshal` failures) is genuinely unreachable.

### Test telemetry

Unit tests emit OpenTelemetry traces and metrics when
`OTEL_EXPORTER_OTLP_ENDPOINT` is set. When unset, telemetry is a no-op.

```sh
# With the dev LGTM stack running (make dev-up):
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317 make tests
```

### Integration tests

Integration tests verify the MCP protocol interface (streamable HTTP) using
k6 with the `xk6-mcp` extension. They require Docker.

| Recipe | LGTM stack | OTel output |
|---|---|---|
| `make integration` | not used | none |
| `make integration-lgtm` | started and stopped by the recipe | yes |
| `make integration-otel` | must already be running | yes, left running |

```sh
make integration        # plain functional run — no telemetry required
make integration-lgtm   # start LGTM, run with OTel, stop LGTM on exit
make integration-otel   # requires make dev-up first; leaves collector running
```

Each integration recipe builds the server container (running unit tests inside
Docker), builds the k6+xk6-mcp image, starts the MCP server, runs the suite,
and tears down containers on exit. Exit code mirrors k6's exit code.

A focused integration test against a pre-built binary is also available:

```sh
make build-binaries     # cross-compile to ./dist/
make integration-binary # run k6 against the binary on the host
```

### Security scans

```sh
make scan-secrets          # TruffleHog: verified secrets in git history
make scan-vulnerabilities  # Trivy: HIGH/CRITICAL CVEs in the source tree
make scan-container        # Trivy: HIGH/CRITICAL CVEs in the server image
make scan-test-container   # Trivy: HIGH/CRITICAL CVEs in the test image
```

Both scanners run in Docker and require no local installation.

## Linting

`make lint` runs all of the following in sequence:

| Recipe | Tool | What it checks |
|---|---|---|
| `lint-dockerfile` | hadolint (Docker) | All `Dockerfile*` files |
| `lint-makefile` | checkmake (Docker) | `makefile` and `testdata/Makefile` |
| `lint-go` | gofmt + go vet | Formatting and static analysis |
| `lint-markdown` | markdownlint (npm) | All `.md` files |
| `lint-tidy` | go mod tidy | `go.mod` / `go.sum` sync |
| `lint-yaml` | yamllint (Python) | All `.yml` / `.yaml` files |
| `validate` | make-mcp-validate | Annotated recipes in `./makefile` |

Run individual linters when iterating:

```sh
make lint-go        # check formatting and vet
make lint-markdown  # check markdown
make lint-yaml      # check YAML
make lint-tidy      # check go.mod/go.sum are tidy
make validate       # validate Makefile annotations
```

Auto-fix formatting before committing:

```sh
make format   # gofmt -w on all .go files in ./cmd, ./pkg, ./internal
make tidy     # go mod tidy && go mod verify
```

## Building

```sh
make build-mcp-server   # ./bin/make-mcp
make build-validator    # ./bin/make-mcp-validate
make build-container    # Docker image ghcr.io/<owner>/make-mcp:<tag>
make build              # all of the above + integration k6 image + test containers
```

The container build runs unit tests inside Docker as a build stage. All
per-package coverage thresholds must pass or the build fails.

Cross-compile release binaries for CI:

```sh
make build-binaries ARCH=amd64    # ./dist/make-mcp_linux_amd64
make build-binaries ARCH=arm64    # ./dist/make-mcp_linux_arm64
```

### Smoke tests

```sh
make test-binary ARCH=amd64        # smoke-test binaries in ./dist/
make smoke-test-container ARCH=amd64  # smoke-test the container image
```

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
