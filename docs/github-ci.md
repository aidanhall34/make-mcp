# GitHub CI and Release

The repository CI and release pipeline is defined in [`.github/workflows/ci.yml`](../.github/workflows/ci.yml).

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
- `integration (tools-list)`
- `integration (tools-call-hello-world)`

## Release flow

- Pull requests and pushes to `main` run linting, unit tests, and parallel integration jobs.
- Pushes to `main` run `semantic-release` after all required checks pass.
- A published release tag uses the `vX.Y.Z` semantic version format.
- Release archives are uploaded to the GitHub release as Linux `amd64` and `arm64` zip files.
- The container image is published to GHCR with `latest` and `vX.Y.Z` tags for `linux/amd64` and `linux/arm64`.

## Discord notifications

Each job sends a Discord webhook notification when `DISCORD_WEBHOOK_URL` is configured as a repository secret.

Store the webhook with:

```sh
make upload-discord-webhook WEBHOOK_URL="https://discord.com/api/webhooks/..."
```

## Local runs with `act`

Use the local host LGTM collector with:

```sh
make act-run
make act-run-authenticated
```

This recipe:

- defaults to the `pull_request` event so local runs exercise CI checks without publishing
- injects `host.docker.internal` into the `act` job container
- forwards `ACT_OTEL_EXPORTER_OTLP_ENDPOINT` to the workflow, defaulting to `http://host.docker.internal:4317`
- leaves the host LGTM stack running; the workflow uses `make integration-act`, which ensures LGTM is up without starting or stopping the optional Grafana MCP sidecar

Optional repository secrets for `act` can be stored in `./.act.secrets` and will be passed automatically when the file exists.

`make act-run-authenticated` additionally reads the current `gh` CLI token and passes it to `act` as `GH_TOKEN` and `GITHUB_TOKEN` secrets without writing that token to disk.
