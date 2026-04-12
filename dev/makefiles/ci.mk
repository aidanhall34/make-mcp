SHELL=/usr/bin/env bash

.PHONY: build-binaries
# @ name: Build Binaries
# @ description: Cross-compiles mcp-server and validate for the target Linux architecture and writes them to ./dist. Used by CI to produce per-arch artifacts before packaging.
# @ risk: low
# @ read-only: false
# @ destructive: false
# @ idempotent: true
# @ open-world: false
# @ param: ARCH string | Target architecture: amd64 or arm64 (default: amd64)
# @ output: Binaries written to ./dist/make-mcp_linux_ARCH and ./dist/make-mcp-validate_linux_ARCH
# @ output-type: application/octet-stream
build-binaries:
	@mkdir -p "$(_LOG_DIR)"
	@{ \
		set -e ; \
		mkdir -p "$(DIST_DIR)" ; \
		CGO_ENABLED="0" GOOS="linux" GOARCH="$(ARCH)" go build -o "$(DIST_DIR)/make-mcp_linux_$(ARCH)" ./cmd/mcp-server ; \
		CGO_ENABLED="0" GOOS="linux" GOARCH="$(ARCH)" go build -o "$(DIST_DIR)/make-mcp-validate_linux_$(ARCH)" ./cmd/validate ; \
		printf 'built %s %s\n' "$(ARCH)" "$$(ls -1 "$(DIST_DIR)"/*_linux_$(ARCH))" ; \
	} $(call _tee-log,build-binaries-$(ARCH)) ; \
	wait

.PHONY: build-container-tar
# @ name: Build Container Tar
# @ description: Builds the make-mcp container image for a single Linux architecture using docker buildx and saves it as a tar to ./dist. Requires a docker-container buildx builder (set up by docker/setup-buildx-action in CI). OTEL_TEST_ENDPOINT is passed as a build arg to capture build-time telemetry.
# @ risk: low
# @ read-only: false
# @ destructive: false
# @ idempotent: true
# @ open-world: true
# @ param: ARCH string | Target architecture: amd64 or arm64 (default: amd64)
# @ param: IMAGE_TAG string | Image tag to apply (default: git tag or short SHA)
# @ param: OTEL_TEST_ENDPOINT string | OTLP endpoint for build-time telemetry (default: http://localhost:4317)
# @ output: Container image tar at ./dist/make-mcp_IMAGE_TAG_linux_ARCH.tar
# @ output-type: application/octet-stream
build-container-tar:
	@mkdir -p "$(_LOG_DIR)"
	@{ \
		set -e ; \
		mkdir -p "$(DIST_DIR)" ; \
		docker buildx build \
			--platform "linux/$(ARCH)" \
			--provenance=false \
			--output "type=docker,dest=$(DIST_DIR)/make-mcp_$(IMAGE_TAG)_linux_$(ARCH).tar" \
			--build-arg "OTEL_EXPORTER_OTLP_ENDPOINT=$(OTEL_TEST_ENDPOINT)" \
			-t "$(IMAGE_NAME):$(IMAGE_TAG)" \
			. ; \
		printf 'saved %s\n' "$(DIST_DIR)/make-mcp_$(IMAGE_TAG)_linux_$(ARCH).tar" ; \
	} $(call _tee-log,build-container-tar-$(ARCH)) ; \
	wait

.PHONY: build-test-image
# @ name: Build Test Image
# @ description: Builds only the minimal testing container image. This target does not depend on build-container; it expects the base image (IMAGE_NAME:IMAGE_TAG) to be already available locally (e.g. loaded from a tar in CI).
# @ risk: low
# @ read-only: false
# @ destructive: false
# @ idempotent: true
# @ open-world: false
# @ param: IMAGE_TAG string | Container image tag to apply (default: latest)
# @ param: ARCH string | Target architecture: amd64 or arm64 (default: amd64)
# @ output: Docker build output
# @ output-type: application/octet-stream
build-test-image:
	@mkdir -p "$(_LOG_DIR)"
	@{ \
		docker build --progress=rawjson \
			--pull=false \
			--platform "linux/$(ARCH)" \
			--build-arg "MAKE_MCP_IMAGE=$(IMAGE_NAME):$(IMAGE_TAG)" \
			-t "$(TEST_IMAGE_NAME):$(IMAGE_TAG)" \
			-f "$(DEV_DIR)/integration/Dockerfile.test-server" \
			"$(DEV_DIR)/integration" ; \
	} $(call _tee-log,build-test-image) ; \
	wait

.PHONY: package-release-archive
# @ name: Package Release Archive
# @ description: Packages pre-built mcp-server and validate binaries from ./dist together with README.md and make-mcp.yml into a .tar.gz release archive for the given version and architecture. Run build-binaries first to produce the binaries.
# @ risk: low
# @ read-only: false
# @ destructive: false
# @ idempotent: true
# @ open-world: false
# @ param: VERSION string | Semantic version tag, for example v1.2.3
# @ param: ARCH string | Target architecture: amd64 or arm64 (default: amd64)
# @ output: Release archive at ./dist/make-mcp_VERSION_linux_ARCH.tar.gz
# @ output-type: application/octet-stream
package-release-archive:
	@{ \
		set -e ; \
		if [ -z "$(VERSION)" ]; then \
			printf '%s\n' "VERSION is required" ; \
			exit 1 ; \
		fi ; \
		mkdir -p "$(DIST_DIR)" ; \
		tmpdir="$$(mktemp -d)" ; \
		trap 'rm -rf "$$tmpdir"' EXIT ; \
		stage="$$tmpdir/make-mcp_$(VERSION)_linux_$(ARCH)" ; \
		mkdir -p "$$stage" ; \
		cp "$(DIST_DIR)/make-mcp_linux_$(ARCH)" "$$stage/make-mcp" ; \
		cp "$(DIST_DIR)/make-mcp-validate_linux_$(ARCH)" "$$stage/make-mcp-validate" ; \
		cp README.md "$$stage/README.md" ; \
		cp make-mcp.yml "$$stage/make-mcp.yml" ; \
		tar -czf "$(DIST_DIR)/make-mcp_$(VERSION)_linux_$(ARCH).tar.gz" \
			-C "$$tmpdir" "make-mcp_$(VERSION)_linux_$(ARCH)" ; \
		printf '%s\n' "$(DIST_DIR)/make-mcp_$(VERSION)_linux_$(ARCH).tar.gz" ; \
	}

.PHONY: build-release-archives
# @ name: Build Release Archives
# @ description: Builds Linux amd64 and arm64 release tar.gz archives in ./dist for the provided semantic version tag. Convenience wrapper around build-binaries and package-release-archive for both arches.
# @ risk: low
# @ read-only: false
# @ destructive: false
# @ idempotent: true
# @ open-world: false
# @ param: VERSION string | Semantic version tag to package, for example v1.2.3
# @ output: Release tar.gz archives written to ./dist
# @ output-type: application/octet-stream
build-release-archives:
	$(MAKE) build-binaries ARCH=amd64
	$(MAKE) build-binaries ARCH=arm64
	$(MAKE) package-release-archive ARCH=amd64 VERSION=$(VERSION)
	$(MAKE) package-release-archive ARCH=arm64 VERSION=$(VERSION)

.PHONY: integration-prebuilt
# @ name: Integration Tests (prebuilt image)
# @ description: Runs MCP protocol integration tests using a container image that is already loaded in the local Docker daemon. Does not build the image. Set IMAGE_NAME and IMAGE_TAG to match the loaded image, and K6_SCRIPT to select the test script.
# @ risk: medium
# @ read-only: true
# @ destructive: false
# @ idempotent: true
# @ open-world: true
# @ param: IMAGE_TAG string | Tag of the pre-loaded container image (default: git tag or short SHA)
# @ param: K6_SCRIPT string | Path inside the k6 container to the test script (default: /scripts/integration.js)
# @ output: k6 integration test results and pass/fail summary
# @ output-type: text/plain
integration-prebuilt: integration-build-k6
	$(call _integration-run,$(_INTEGRATION_STACK_COMPOSE))

.PHONY: integration-binary
# @ name: Binary Integration Tests
# @ description: Starts the pre-built mcp-server binary on the host in HTTP mode and runs k6 MCP protocol integration tests against it via host.docker.internal. Does not use a container for the server. Run build-binaries first to produce the binaries.
# @ risk: medium
# @ read-only: true
# @ destructive: false
# @ idempotent: true
# @ open-world: true
# @ param: ARCH string | Target architecture: amd64 or arm64 (default: amd64)
# @ param: K6_SCRIPT string | Path inside the k6 container to the test script (default: /scripts/integration.js)
# @ output: k6 integration test results and pass/fail summary
# @ output-type: text/plain
integration-binary: integration-build-k6
	$(call _integration-run-binary,$(_INTEGRATION_BINARY_RUNNER_COMPOSE))

.PHONY: test-binary
# @ name: Binary Smoke Tests
# @ description: Runs smoke tests against the pre-built mcp-server and validate binaries in ./dist for the target architecture. Verifies that validate accepts the project makefile, starts the MCP server in HTTP mode, polls /ready, and confirms it responds. On an amd64 host, arm64 binaries require QEMU binfmt_misc registration (provided automatically by docker/setup-qemu-action in CI).
# @ risk: low
# @ read-only: true
# @ destructive: false
# @ idempotent: true
# @ open-world: false
# @ param: ARCH string | Target architecture: amd64 or arm64 (default: amd64)
# @ output: Pass/fail summary for each binary
# @ output-type: text/plain
test-binary:
	@mkdir -p "$(_LOG_DIR)"
	@{ \
		set -e ; \
		mcp_server="$(DIST_DIR)/make-mcp_linux_$(ARCH)" ; \
		validate_bin="$(DIST_DIR)/make-mcp-validate_linux_$(ARCH)" ; \
		for f in "$$mcp_server" "$$validate_bin"; do \
			[ -f "$$f" ] || { printf 'binary not found: %s\n' "$$f" ; exit 1 ; } ; \
			chmod +x "$$f" ; \
		done ; \
		printf 'smoke testing make-mcp-validate (%s)...\n' "$(ARCH)" ; \
		"$$validate_bin" --config make-mcp.yml ; \
		printf 'starting make-mcp in HTTP mode (%s)...\n' "$(ARCH)" ; \
		"$$mcp_server" \
			--config "$(DEV_DIR)/integration/make-mcp.yml" \
			--makefile "testdata/Makefile" & \
		server_pid=$$! ; \
		trap 'kill "$$server_pid" 2>/dev/null || true' EXIT INT TERM ; \
		retries=30 ; \
		until curl -sf "http://localhost:9378/ready" >/dev/null 2>&1 ; do \
			retries=$$((retries - 1)) ; \
			[ "$$retries" -gt 0 ] || { printf 'mcp-server did not become ready\n' ; exit 1 ; } ; \
			sleep 2 ; \
		done ; \
		printf 'mcp-server ready (%s)\n' "$(ARCH)" ; \
		curl -sf "http://localhost:9378/ready" ; \
		printf '\nbinary smoke tests passed (%s)\n' "$(ARCH)" ; \
	} $(call _tee-log,test-binary-$(ARCH)) ; \
	kill "$$server_pid" 2>/dev/null || true ; \
	wait

.PHONY: smoke-test-container
# @ name: Container Smoke Test
# @ description: Runs a container smoke test against the pre-loaded image for the target architecture. Starts the container in HTTP mode with testdata/Makefile mounted, polls /ready, and confirms it responds. On an amd64 host, arm64 images require QEMU (provided by docker/setup-qemu-action in CI).
# @ risk: low
# @ read-only: true
# @ destructive: false
# @ idempotent: true
# @ open-world: true
# @ param: ARCH string | Target architecture: amd64 or arm64 (default: amd64)
# @ param: IMAGE_TAG string | Tag of the pre-loaded container image (default: git tag or short SHA)
# @ output: Pass/fail summary for the container smoke test
# @ output-type: text/plain
smoke-test-container: build-test-image
	@mkdir -p "$(_LOG_DIR)"
	@{ \
		set -e ; \
		image="$(TEST_IMAGE_NAME):$(IMAGE_TAG)" ; \
		container_name="make-mcp-smoke-$(ARCH)" ; \
		printf 'smoke testing container %s (%s)...\n' "$$image" "$(ARCH)" ; \
		docker rm -f "$$container_name" 2>/dev/null || true ; \
		docker run -d \
			--name "$$container_name" \
			-p 9378:9378 \
			-v "$(CURDIR)/testdata/Makefile:/opt/make-mcp/makefile:ro" \
			"$$image" \
			--transport http --listen 0.0.0.0:9378 --makefile /opt/make-mcp/makefile ; \
		trap 'docker rm -f "$$container_name" 2>/dev/null || true' EXIT INT TERM ; \
		retries=30 ; \
		until curl -sf "http://localhost:9378/ready" >/dev/null 2>&1 ; do \
			retries=$$((retries - 1)) ; \
			[ "$$retries" -gt 0 ] || { printf 'container did not become ready\n' ; exit 1 ; } ; \
			sleep 2 ; \
		done ; \
		printf 'container ready\n' ; \
		curl -sf "http://localhost:9378/ready" ; \
		printf '\ncontainer smoke test passed (%s)\n' "$(ARCH)" ; \
	} $(call _tee-log,smoke-test-container-$(ARCH)) ; \
	wait
