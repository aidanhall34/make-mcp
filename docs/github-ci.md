# GitHub CI and Release

The pipeline is split across multiple workflow files under `.github/workflows/`. Reusable
workflows are prefixed with `_` and have no push trigger of their own — they are called
by other workflows via `workflow_call`.

## Workflow files

| File | Trigger | Purpose |
|---|---|---|
| `lint.yml` | push | Go formatting, vet, markdownlint, mod tidy |
| `scan.yml` | push | TruffleHog secret scan + Trivy vulnerability scan on source |
| `test.yml` | push | Unit tests with `-race` on amd64 |
| `bench.yml` | push | Benchmark suite on amd64 |
| `build-images.yml` | push | Build container image tars — matrix amd64/arm64, calls `_build-image.yml` |
| `build-releases.yml` | push | Build binary release archives — matrix amd64/arm64, calls `_build-release.yml` |
| `smoke-tests.yml` | push | Start server + validator in default config, verify they respond |
| `install-tests.yml` | push | `go install` both binaries, run them without error |
| `compatibility-tests.yml` | push | k6 MCP protocol tests against binary and container |
| `release.yml` | push to `main` | Full release orchestrator — semantic-release, build, SBOM, scan, publish, verify |
| `_build-image.yml` | workflow_call | Reusable: build one container image tar for a given arch |
| `_build-release.yml` | workflow_call | Reusable: build one binary release archive for a given arch |

---

## Pre-merge pipeline

Every push to any branch runs the following workflows in parallel. No workflow blocks
another. A failure in one does not stop others from completing.

```
push
 ├── lint.yml
 ├── scan.yml
 ├── test.yml
 ├── bench.yml
 ├── build-images.yml ──────────────────┐
 ├── build-releases.yml ────────────────┤
 │                                       ├── smoke-tests.yml
 ├── install-tests.yml                  └── compatibility-tests.yml
 └── (smoke-tests + compatibility-tests need build-images + build-releases)
```

### lint

Runs `make lint`, which covers:

- `lint-go` — `gofmt` check + `go vet`
- `lint-markdown` — markdownlint
- `lint-tidy` — verifies `go.mod` and `go.sum` are in sync
- `validate` — validates `make-mcp.yml` against the project Makefile

### scan

Runs on the current commit:

- **TruffleHog** — scans the git history for verified secrets; fails if any are found.
- **Trivy (filesystem)** — scans source code and dependencies for known CVEs; fails on
  HIGH or CRITICAL severity findings.

### test

Runs `make unit-tests`: `go test -race -cover ./...` on amd64. Coverage thresholds are
enforced per-package by `TestMain`. Output is teed to `dev/logs/unit-tests.log`.

### bench

Runs `make bench`: `go test -bench=. -benchmem` on amd64. Results are uploaded as a
workflow artifact for trend tracking. Output is teed to `dev/logs/bench.log`.

### build-images

Calls `_build-image.yml` as a matrix over `[amd64, arm64]`. Each arch runs in parallel.

Each call:
1. Builds the container image tar via `make build-container-tar ARCH=<arch>`.
2. Uploads the tar as artifact `container-<arch>`.
3. Notifies Discord on completion.

### build-releases

Calls `_build-release.yml` as a matrix over `[amd64, arm64]`. Each arch runs in parallel.

Each call:
1. Cross-compiles `mcp-server` and `validate` via `make build-binaries ARCH=<arch>`.
2. Uploads binaries as artifact `binaries-<arch>`.
3. Notifies Discord on completion.

### smoke-tests

Needs: `build-images`, `build-releases`.

Runs as a matrix over `[amd64, arm64]` for both binary and container targets (4 jobs in
parallel). For each combination:

**Binary smoke test:**
1. Download `binaries-<arch>` artifact.
2. Run `validate --config make-mcp.yml` — verify it exits 0.
3. Start `mcp-server` in stdio mode; send a JSON-RPC `initialize` request over stdin;
   verify a valid response is received on stdout.
4. Run `make test-binary ARCH=<arch>` (HTTP mode, `/ready` poll).

**Container smoke test:**
1. Download `container-<arch>` artifact and `docker load` it.
2. Run `make smoke-test-container ARCH=<arch>` (HTTP mode, `/ready` poll).

Each job notifies Discord on completion.

### install-tests

No artifact dependency — builds from source using `go install`:

1. `go install ./cmd/mcp-server`
2. `go install ./cmd/validate`
3. Run `mcp-server --help` — verify exit 0.
4. Run `validate --config make-mcp.yml` — verify exit 0.

Runs on amd64 only. Notifies Discord on completion.

### compatibility-tests

Needs: `build-images`, `build-releases`.

Runs the k6 MCP protocol test suite against both the binary and the container, as a
matrix over `[amd64, arm64]` × `[binary, container]` (4 jobs in parallel).

**Binary:** starts `mcp-server` on the host, runs k6 via Docker targeting
`host.docker.internal:9378` using `make integration-binary`.

**Container:** loads the pre-built tar, runs `make integration-prebuilt`.

Scripts:

- `tools-list.js` — verifies `tools/list` returns the expected tool set.
- `tools-call-hello-world.js` — verifies `tools/call` executes and returns correct output.

Each job uses `continue-on-error: true` and a final fail-check step so all variants
complete before the workflow is marked failed. Each job notifies Discord after its
integration phase.

---

## Required branch protection

Apply [`.github/branch-protection/main.json`](../.github/branch-protection/main.json)
to the `main` branch:

```sh
gh api \
  --method PUT \
  -H "Accept: application/vnd.github+json" \
  "/repos/<owner>/<repo>/branches/main/protection" \
  --input ".github/branch-protection/main.json"
```

Required status checks (all must pass before merge):

- `lint`
- `scan`
- `test`
- `bench`
- `build-images (amd64)` / `build-images (arm64)`
- `build-releases (amd64)` / `build-releases (arm64)`
- `smoke-tests (binary-amd64)` / `smoke-tests (binary-arm64)` / `smoke-tests (container-amd64)` / `smoke-tests (container-arm64)`
- `install-tests`
- `compatibility-tests (binary-amd64)` / `compatibility-tests (binary-arm64)` / `compatibility-tests (container-amd64)` / `compatibility-tests (container-arm64)`

---

## Post-merge release pipeline

`release.yml` triggers on every push to `main`. It runs a fully sequential job chain: if
any job fails, all downstream jobs are skipped and the release is aborted. Each job
notifies Discord on completion.

```
push to main
  │
  ▼
semantic-release ──── (no release?) ──► end
  │ (released=true)
  ▼
 ┌─────────────────────────────┐
 │  build-image-amd64          │  (parallel)
 │  build-image-arm64          │
 │  build-release-amd64        │
 │  build-release-arm64        │
 └──────────────┬──────────────┘
                ▼
 ┌─────────────────────────────┐
 │  sbom-image-amd64           │  (parallel)
 │  sbom-image-arm64           │
 │  sbom-release-amd64         │
 │  sbom-release-arm64         │
 │  scan-images                │
 │  scan-releases              │
 └──────────────┬──────────────┘
                ▼
 ┌─────────────────────────────┐
 │  release-binaries           │  (parallel)
 │  release-containers         │
 └──────────────┬──────────────┘
                ▼
 ┌─────────────────────────────┐
 │  pull-test-containers       │  (parallel, matrix: amd64/arm64)
 │  install-test-releases      │  (parallel, matrix: amd64/arm64)
 └─────────────────────────────┘
```

### semantic-release

Runs `npx semantic-release` using `release.config.cjs`.

Outputs: `released` (bool), `git_tag` (e.g. `v1.2.3`).

If `released=false`, all downstream jobs are skipped via `if:
needs.semantic-release.outputs.released == 'true'`.

See [Release flow](#release-flow) for commit message rules.

### build-image-{arch} / build-release-{arch}

Calls `_build-image.yml` / `_build-release.yml` with `image-tag: ${{ git_tag }}`. These
produce the same artifacts as the pre-merge builds but tagged with the release version.

All four run in parallel and are gated only on `semantic-release` succeeding.

### sbom-{image,release}-{arch} / scan-{images,releases}

Runs after all build jobs pass. Parallel.

**SBOM generation** (Trivy):

- Container: `trivy image --format cyclonedx --output sbom-image-<arch>.cdx.json <image>:<tag>`
- Release archive: `trivy fs --format cyclonedx --output sbom-release-<arch>.cdx.json dist/`

SBOM files are uploaded as workflow artifacts for attachment to the GitHub release.

**Scan:**

- Trivy re-scans each container image and release archive for CVEs. Fails on HIGH or
  CRITICAL findings — this gates the publish jobs.
- TruffleHog re-scans the source at the release tag.

### release-binaries

Needs: all sbom + scan jobs.

For each arch:
1. Downloads `binaries-<arch>` artifact.
2. Generates `sha256sum` checksums.
3. Packages `make-mcp_vX.Y.Z_linux_<arch>.tar.gz` (binaries + README + make-mcp.yml).
4. Uploads archive, checksums, and SBOM to the GitHub release via `gh release upload`.

### release-containers

Needs: all sbom + scan jobs.

1. Downloads `container-<arch>` tars.
2. Loads and tags each as `ghcr.io/aidanhall34/make-mcp:<tag>-<arch>` and `:<sha>-<arch>`.
3. Pushes arch-specific images.
4. Creates multi-arch manifest with tags `:vX.Y.Z`, `:<short-sha>`, and `:latest`.

### pull-test-containers

Needs: `release-containers`.

Matrix over `[amd64, arm64]`. For each arch:
1. Pull `ghcr.io/aidanhall34/make-mcp:vX.Y.Z` from GHCR.
2. Run as smoke test (HTTP mode, `/ready` poll, same as pre-merge smoke-tests).

arm64 uses QEMU via `docker/setup-qemu-action`.

### install-test-releases

Needs: `release-binaries`.

Matrix over `[amd64, arm64]`. For each arch:
1. Download `make-mcp_vX.Y.Z_linux_<arch>.tar.gz` from the GitHub release.
2. Extract and run `validate --config make-mcp.yml` — verify exit 0.
3. Start `mcp-server` in HTTP mode; poll `/ready`; verify it responds.

---

## Release flow

### Conventional Commits format

Releases are fully automated. **No manual tagging.** Commit messages drive the version:

| Commit type | Version bump |
|---|---|
| `fix:` | patch — `1.0.0 → 1.0.1` |
| `feat:` | minor — `1.0.0 → 1.1.0` |
| `feat!:` or `BREAKING CHANGE:` footer | major — `1.0.0 → 2.0.0` |
| `chore:`, `docs:`, `test:`, `ci:`, etc. | no release |

Format:

```
<type>[optional scope]: <description>

[optional body]

[optional footer(s)]
```

### Squash merges and the changelog

This repo uses squash merges. Each PR lands as one commit on `main` using the PR title
as the message (e.g. `feat: add stdio transport (#42)`). `semantic-release` only analyzes
commits on `main`, so each PR produces exactly one changelog entry — the PR title.

**The PR title is the changelog entry. It must follow Conventional Commits format.**

### Changelog

`@semantic-release/changelog` prepends the new entry to `CHANGELOG.md`.
`@semantic-release/git` commits the updated file back to `main` with `[skip ci]`.
The GitHub release body is generated from the same notes.

### Verifying a release locally before merging

```sh
npx semantic-release --dry-run
```

Shows the computed next version and changelog without publishing.

### Preventing bad commit messages

`make setup` installs two git hooks:

- **`pre-commit`** — runs `make pre-commit` (lint + unit tests).
- **`commit-msg`** — runs `commitlint` using `commitlint.config.cjs`. Fires after the
  message is written so it always sees the actual new message.

---

## Artifacts

### Pre-merge (workflow artifacts, 7-day retention)

| Name | Contents |
|---|---|
| `binaries-amd64` / `binaries-arm64` | `make-mcp_linux_<arch>`, `make-mcp-validate_linux_<arch>` |
| `container-amd64` / `container-arm64` | `make-mcp_ci-<sha>_linux_<arch>.tar` |

### Release (attached to GitHub release)

| File | Contents |
|---|---|
| `make-mcp_vX.Y.Z_linux_<arch>.tar.gz` | binaries + `README.md` + `make-mcp.yml` |
| `make-mcp_vX.Y.Z_linux_<arch>.sha256` | SHA-256 checksums |
| `sbom-image-<arch>.cdx.json` | Container image SBOM (CycloneDX) |
| `sbom-release-<arch>.cdx.json` | Release archive SBOM (CycloneDX) |

### Container registry

`ghcr.io/aidanhall34/make-mcp` is published with three tags on each release:

- `vX.Y.Z` — immutable release tag
- `<short-sha>` — commit reference
- `latest` — updated on every release

All tags are multi-arch manifests covering `linux/amd64` and `linux/arm64`.

---

## Secrets

| Secret | Required by | Purpose |
|---|---|---|
| `DISCORD_WEBHOOK_URL` | all workflows | Job status notifications |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | all workflows | Telemetry export (optional) |
| `GITHUB_TOKEN` | auto-provisioned | Release creation, artifact upload, GHCR push |

---

## Discord notifications

Every job sends a Discord embed on completion (success, failure, or cancelled) when
`DISCORD_WEBHOOK_URL` is set. Embeds include job name, status, duration, commit SHA,
and linked pull request.

Store the webhook:

```sh
make upload-discord-webhook WEBHOOK_URL="https://discord.com/api/webhooks/..."
```

This updates both the GitHub secret and the local `.act.secrets` file.

---

## Local runs with `act`

```sh
make act-run
```

- Uses the `push` event to match production triggers.
- Injects `host.docker.internal` into job containers.
- Forwards `ACT_OTEL_EXPORTER_OTLP_ENDPOINT` (default `http://host.docker.internal:4317`).
- Limits concurrent jobs to `ACT_CONCURRENT_JOBS` (default 2).
- Output is teed to `dev/logs/act-run.log`.

Secrets are read from `.act.secrets` when the file exists.

To run a single workflow:

```sh
make act-run ACT_WORKFLOW=.github/workflows/lint.yml
```
