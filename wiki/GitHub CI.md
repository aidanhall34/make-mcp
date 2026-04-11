# GitHub CI and Release

The pipeline is managed by two main orchestrator workflows that call reusable workflows
stored in `.github/workflows/`. Reusable workflows are prefixed with `_` and have no
push trigger of their own — they are invoked via `workflow_call`.

## Workflow files

| File | Trigger | Purpose |
|---|---|---|
| `ci.yml` | push | **Orchestrator:** Runs the full pre-merge suite (lint, scan, test, build, smoke, integration) |
| `release.yml` | push to `main` | **Orchestrator:** Full release lifecycle — semantic-release, publish, SBOM, verify |
| `_lint.yml` | workflow_call | Go formatting, vet, markdownlint, mod tidy |
| `_scan.yml` | workflow_call | TruffleHog secret scan + Trivy vulnerability scan on source |
| `_test.yml` | workflow_call | Unit tests with `-race` on amd64 |
| `_bench.yml` | workflow_call | Benchmark suite on amd64 |
| `_build-image.yml` | workflow_call | Build container image tar + Trivy vuln scan |
| `_build-release.yml` | workflow_call | Build binary release archives |
| `_smoke-tests.yml` | workflow_call | Start server + validator, verify they respond |
| `_install-tests.yml` | workflow_call | `go install` both binaries, run them without error |
| `_compatibility-tests.yml` | workflow_call | k6 MCP protocol tests against binary and container |

---

## Pre-merge pipeline (ci.yml)

Every push to any branch triggers `ci.yml`. It coordinates parallel execution to ensure
that fast native builds (amd64) are not blocked by slower emulated builds (arm64).

### Execution Graph

```
push
 ├── lint (_lint.yml)
 ├── scan (_scan.yml)
 ├── test (_test.yml)
 ├── bench (_bench.yml)
 ├── install-tests (_install-tests.yml)
 │
 ├── build-images-amd64 ───┬── smoke-tests-amd64
 └── build-releases-amd64 ─┘── compatibility-tests-amd64
 │
 ├── build-images-arm64 ───┬── smoke-tests-arm64
 └── build-releases-arm64 ─┘── compatibility-tests-arm64
```

### build-images (_build-image.yml)

Builds the container image tar via `make build-container-tar`. Immediately after building,
it runs a **Trivy vulnerability scan** on the tarball. If any `HIGH` or `CRITICAL` 
vulnerabilities are found, the job fails.

### smoke-tests (_smoke-tests.yml)

**Binary smoke test:**
1. Download `binaries-<arch>` artifact.
2. Run `make test-binary ARCH=<arch>` (HTTP mode, `/ready` poll).

**Container smoke test:**
1. Download `container-<arch>` artifact and `docker load` it.
2. Run `make smoke-test-container ARCH=<arch>` (HTTP mode, `/ready` poll).

### compatibility-tests (_compatibility-tests.yml)

Runs the k6 MCP protocol test suite against both the binary and the container.

**Binary:** starts `mcp-server` on the host, runs k6 via Docker targeting 
`host.docker.internal:9378` using `make integration-binary`.

**Container:** loads the pre-built tar, runs `make integration-prebuilt`.

---

## Post-merge release pipeline (release.yml)

`release.yml` triggers on every push to `main`. It follows a sequential chain to ensure
only verified code is published.

### semantic-release

Runs `npx semantic-release`. If a new version is determined:
1. Creates a git tag (e.g., `v1.2.3`).
2. Generates a GitHub Release with a changelog.
3. Outputs `released=true` to trigger downstream jobs.

### sbom-and-scan

After release builds complete, this job:
1. **Generates SBOMs** in CycloneDX format for both binaries and containers.
2. **Performs a final Trivy scan** on the release artifacts.
3. **Uploads SBOMs** directly to the GitHub Release.

---

## Discord notifications

Notifications are optimized to reduce noise:
- **CI Jobs:** Only send a Discord notification on **failure**.
- **Release Jobs:** Only send on **failure**, except for `semantic-release`, which 
  notifies on failure OR when a new version is successfully published.

Embeds include job name, status, duration, and a link to the commit/PR.

---

## Local runs with `act`

```sh
make act-run
```

- Uses the `push` event to trigger the `ci.yml` coordinator.
- Secrets are read from `.act.secrets`.
- To run a specific reusable workflow directly (if needed):
  `make act-run ACT_WORKFLOW=.github/workflows/_lint.yml`
