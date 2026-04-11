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

The official container image is a **minimal scratch image** containing only the
`make-mcp` and `make-mcp-validate` binaries.

### Design Choice: Scratch Image

Since this MCP server executes arbitrary `make` commands on your behalf, it is
impossible to provide a single container image that includes all possible
runtime dependencies (e.g., specific versions of compilers, linters, or deployment
tools).

Instead of providing a "fat" container, we provide a minimal one that serves
as a source for the binaries. **You are expected to copy the `make-mcp` binary
into your own custom dev container** where you manage your project's software
versions.

The only requirement for the runtime environment is that `make` (or your
configured runner) must be available on the `PATH`.

### Usage in Custom Containers

To use `make-mcp` in your project's dev container, use a multi-stage build to
copy the binary from our official image:

```dockerfile
# Your existing dev container or a base image with your dependencies
FROM alpine:3.21

# Install your project's dependencies (e.g. make, gcc, python, etc.)
RUN apk add --no-cache make

# Copy the make-mcp binary from the official scratch image
COPY --from=ghcr.io/aidanhall34/make-mcp:latest /make-mcp /usr/local/bin/make-mcp

# Start the server
ENTRYPOINT ["make-mcp"]
```

For a complete example of a container with `make` and other testing utilities,
see the [integration test container](https://github.com/aidanhall34/make-mcp/blob/main/dev/integration/Dockerfile.test-server).

### Running the Scratch Image Directly

You can still run the scratch image directly if your host provides the
necessary environment via volume mounts, but this is generally not
recommended for production use:

```sh
docker run --rm \
  -v "$(pwd)/Makefile:/Makefile" \
  -v "$(pwd)/make-mcp.yml:/make-mcp.yml" \
  ghcr.io/aidanhall34/make-mcp:latest \
  --makefile /Makefile --config /make-mcp.yml
```

> **Note:** The scratch image does not have `sh`, `ls`, or even `make`
> installed. It will fail to execute any tools if they rely on these being
> present inside the container.

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
