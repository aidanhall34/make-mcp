# Security Policy

## Reporting a vulnerability

Please **do not** open a public GitHub issue for security vulnerabilities.

Report them privately via [GitHub Security Advisories](https://github.com/aidanhall34/make-mcp/security/advisories/new).
Include a description of the issue, steps to reproduce, and any relevant configuration or log output.

You will receive an acknowledgement within 72 hours. Critical issues will be patched and released as a priority.

## Design considerations

`make-mcp` executes `make` recipes on behalf of an MCP client. By design, any tool
exposed through the server can run arbitrary shell commands. Before deploying:

- **Expose only the recipes you intend to expose.** Use the annotation schema to
  control which targets become MCP tools; unannotated targets are not served.
- **Run the server in an isolated environment.** The official container image is a
  minimal scratch image intended to be copied into your own dev container, where
  you control the runtime environment and its capabilities.
- **Do not expose the HTTP transport publicly.** The server has no built-in
  authentication. Bind it to localhost or a private network interface and place
  an authenticating proxy in front of it if remote access is required.
- **Use `make-mcp-validate` before deploying.** The validator catches
  configuration errors and malformed annotations before the server starts.

## Supported versions

Only the latest release receives security fixes.

## Release artifacts

Every release includes:

- SHA-256 checksums for all binaries (`make-mcp_vX.Y.Z_linux_<arch>.sha256`)
- CycloneDX SBOMs (`sbom-release-<arch>.cdx.json`)
- Trivy vulnerability scan results attached to the GitHub release
