# GitHub CI and Release

The repository CI and release pipeline is defined in [`.github/workflows/ci.yml`](../.github/workflows/ci.yml).

## Pipeline overview

Every push triggers the full pipeline. Jobs run in the following order:

```
lint ─┐
      ├─► build-and-test (binary-amd64)  ─┐
unit-tests ─┤                              ├─► release (main only) ─► publish-release-archives (amd64, arm64)
      ├─► build-and-test (binary-arm64)  ─┤                        └─► publish-container
      ├─► build-and-test (container-amd64)─┤
      └─► build-and-test (container-arm64)─┘
```

The four `build-and-test` jobs are fully independent and run in parallel. Each has three phases, each notifying Discord on completion:

1. **Build** — compile binaries or build the container image tar.
2. **Smoke test** — start the server, poll `/ready`, confirm it responds.
3. **Integration** — run k6 MCP protocol tests against the running server.

Binary jobs start the `mcp-server` binary directly on the runner. Container jobs load the pre-built tar and run it via Docker.

## Required branch protection

Apply [`.github/branch-protection/main.json`](../.github/branch-protection/main.json) to the `main` branch with:

```sh
gh api \
  --method PUT \
  -H "Accept: application/vnd.github+json" \
  "/repos/<owner>/<repo>/branches/main/protection" \
  --input ".github/branch-protection/main.json"
```

The required status checks are:

- `lint`
- `unit-tests`
- `build-and-test (binary-amd64)`
- `build-and-test (binary-arm64)`
- `build-and-test (container-amd64)`
- `build-and-test (container-arm64)`

## Release flow

Releases are fully automated via [`semantic-release`](https://github.com/semantic-release/semantic-release) on every push to `main`. **No manual tagging is required or expected.**

### How it works

1. After all `build-and-test` jobs pass, the `release` job runs `npx semantic-release`.
2. `semantic-release` scans all commits since the last `vX.Y.Z` tag using the **Conventional Commits** format and determines whether a release is warranted:

   | Commit type | Version bump |
   |---|---|
   | `fix:` | patch — `1.0.0 → 1.0.1` |
   | `feat:` | minor — `1.0.0 → 1.1.0` |
   | `feat!:` or `BREAKING CHANGE:` footer | major — `1.0.0 → 2.0.0` |
   | `chore:`, `docs:`, `test:`, `ci:`, etc. | no release |

3. If a release is warranted, `semantic-release` runs its plugin chain in order:
   - Prepends the new release notes to `CHANGELOG.md` (creating it if absent).
   - Creates the `vX.Y.Z` git tag and GitHub release with the generated notes as the release body.
   - Commits the updated `CHANGELOG.md` back to `main` with `[skip ci]` so the push does not re-trigger the pipeline.
4. The `release` job outputs `released=true` and the new tag.
5. The `publish-release-archives` and `publish-container` jobs gate on `released=true` and reuse the artifacts already built in step 1 — no rebuilding occurs.

### Conventional Commits format

Commit messages must follow the format:

```
<type>[optional scope]: <description>

[optional body]

[optional footer(s)]
```

Examples:

```
feat: add stdio transport support
fix(parser): handle empty makefile gracefully
feat!: remove deprecated --legacy flag

BREAKING CHANGE: --legacy flag removed, update your config
```

### Squash merges and the changelog

This repo uses squash merges. When a PR is merged, GitHub squashes all branch commits into a single commit on `main` using the PR title as the commit message (e.g. `feat: add stdio transport (#42)`). The individual branch commits never appear in `main`'s history.

`semantic-release` only analyzes commits present in `main`, so each PR contributes exactly one changelog entry — the PR title. Branch commits are never analyzed and never appear in the changelog. **This means the PR title is the changelog entry — it must follow the Conventional Commits format.**

### Verifying a release locally before merging

To preview what version would be cut without publishing anything:

```sh
npx semantic-release --dry-run
```

This prints the computed next version and the full changelog that would be generated, based on commits since the last tag. Run it on the branch before opening a PR to confirm the commit messages will produce the intended bump.

### Preventing bad commit messages

Commit message linting is built into the pre-commit hook. After running `make setup`, every commit attempt runs `commitlint` against the message before it is accepted. Non-conformant messages are rejected immediately with an explanation of the rule that failed.

`make setup` installs two hooks:

- **`pre-commit`** — runs `make pre-commit` (lint + unit tests) before the commit is recorded.
- **`commit-msg`** — runs `commitlint` against the message file Git passes as `$1`, validating it against the Conventional Commits rules in `commitlint.config.cjs`. This hook fires after the message is written, so it always sees the actual new message.

## Artifacts

For each architecture (`amd64`, `arm64`) the pipeline produces:

- **Binary artifact** — `dist/mcp-server_linux_ARCH` and `dist/validate_linux_ARCH`, uploaded as `binaries-ARCH`.
- **Container tar** — `dist/make-mcp_ci-SHA_linux_ARCH.tar`, uploaded as `container-ARCH`.

On a successful release, these are repackaged and published as:

- **Release archive** — `make-mcp_vX.Y.Z_linux_ARCH.tar.gz` attached to the GitHub release, containing both binaries, `README.md`, and `make-mcp.yml`.
- **Container image** — pushed to `ghcr.io/aidanhall34/make-mcp` with tags `vX.Y.Z`, `SHORT_SHA`, and `latest` as a multi-arch manifest covering `linux/amd64` and `linux/arm64`.

## Discord notifications

Each job phase sends a Discord webhook notification when `DISCORD_WEBHOOK_URL` is configured as a repository secret. Notifications include job name, status, duration, commit, and linked pull request.

Store the webhook with:

```sh
make upload-discord-webhook WEBHOOK_URL="https://discord.com/api/webhooks/..."
```

## Local runs with `act`

Run the full workflow locally against the dev LGTM stack:

```sh
make act-run
```

This recipe:

- uses the `push` event to match the production workflow trigger
- injects `host.docker.internal` into job containers so they can reach the host LGTM stack
- forwards `ACT_OTEL_EXPORTER_OTLP_ENDPOINT` to the workflow, defaulting to `http://host.docker.internal:4317`
- limits concurrent jobs to 2 (configurable via `ACT_CONCURRENT_JOBS`)

Optional repository secrets for `act` can be stored in `./.act.secrets` and will be passed automatically when the file exists. Use `make upload-discord-webhook` to populate both GitHub and the local secrets file.
