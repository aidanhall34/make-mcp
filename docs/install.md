# Installation

## GitHub Releases

Pre-built binaries for `linux/amd64` and `linux/arm64` are attached to each
[GitHub release](https://github.com/aidanhall34/make-mcp/releases).

```sh
# Replace vX.Y.Z and <arch> (amd64 or arm64) for your target
curl -Lo make-mcp.tar.gz \
  https://github.com/aidanhall34/make-mcp/releases/download/vX.Y.Z/make-mcp_vX.Y.Z_linux_<arch>.tar.gz

tar -xzf make-mcp.tar.gz
chmod +x make-mcp make-mcp-validate

# Verify the config validates before running
./make-mcp-validate --config make-mcp.yml

# Start the server
./make-mcp --config make-mcp.yml
```

SHA-256 checksums are provided in `make-mcp_vX.Y.Z_linux_<arch>.sha256` and
CycloneDX SBOMs are attached as `sbom-release-<arch>.cdx.json`.

---

## Docker

Container images are published to GHCR as multi-arch manifests (`linux/amd64`
and `linux/arm64`).

```sh
docker pull ghcr.io/aidanhall34/make-mcp:latest
```

Available tags:

| Tag | Meaning |
|---|---|
| `latest` | Most recent release |
| `vX.Y.Z` | Immutable release tag |
| `<short-sha>` | Commit reference |

Run with a volume-mounted Makefile:

```sh
docker run --rm \
  -v "$(pwd)/Makefile:/workspace/Makefile" \
  -v "$(pwd)/make-mcp.yml:/opt/make-mcp/make-mcp.yml" \
  -p 9378:9378 \
  ghcr.io/aidanhall34/make-mcp:latest \
  --transport http --listen 0.0.0.0:9378
```

The entrypoint is `/opt/make-mcp/make-mcp`. The default config path inside the
container is `/opt/make-mcp/make-mcp.yml`; override with `--config`.

---

## go install

```sh
go install github.com/aidanhall34/make-mcp/cmd/mcp-server@latest
go install github.com/aidanhall34/make-mcp/cmd/validate@latest
```

> **Note:** Go installs binaries using the `cmd/` directory name, not the
> release archive name. The installed binaries are named `mcp-server` and
> `validate`, not `make-mcp` and `make-mcp-validate`. To get the canonical
> names, build from source with `make build` or download a release archive.

---

## Build from source

Requirements: Go 1.25+, GNU Make, Node.js >=24.14.1.

```sh
git clone https://github.com/aidanhall34/make-mcp.git
cd make-mcp

# Install git hooks and Node.js dev dependencies
make setup

# Run tests
make tests

# Build both binaries into ./bin/
make build

# Validate the project Makefile
make validate
```

Binaries are written to `./bin/make-mcp` and `./bin/make-mcp-validate`.
