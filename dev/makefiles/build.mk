SHELL=/usr/bin/env bash

.PHONY: build-validator
# @ name: Build Validator
# @ description: Compiles the makefile validator CLI binary to ./bin/make-mcp-validate.
# @ risk: low
# @ read-only: false
# @ destructive: false
# @ idempotent: true
# @ open-world: false
# @ param: none
# @ output: Binary at ./bin/make-mcp-validate
# @ output-type: application/octet-stream
build-validator:
	@mkdir -p "$(_LOG_DIR)"
	@{ go build -o ./bin/make-mcp-validate ./cmd/validate ; } \
		$(call _tee-log,build-validator) ; \
	wait

.PHONY: build-mcp-server
# @ name: Build MCP Server
# @ description: Compiles the MCP server CLI binary to ./bin/make-mcp. RUN AFTER CHANGING ./pkg or ./cmd
# @ risk: low
# @ read-only: false
# @ destructive: false
# @ idempotent: true
# @ open-world: false
# @ param: none
# @ output: Binary at ./bin/make-mcp
# @ output-type: application/octet-stream
build-mcp-server:
	@mkdir -p "$(_LOG_DIR)"
	@{ go build -o ./bin/make-mcp ./cmd/mcp-server ; } \
		$(call _tee-log,build-mcp-server) ; \
	wait

.PHONY: build-container
# @ name: Build Container
# @ description: Builds the make-mcp container image locally. Requires IMAGE_TAG (default: latest). All tests must pass their per-package coverage thresholds or the build fails.
# @ risk: low
# @ read-only: false
# @ destructive: false
# @ idempotent: true
# @ open-world: false
# @ param: IMAGE_TAG string | Container image tag to apply to the built image (default: latest)
# @ output: Docker build output as JSON progress records
# @ output-type: application/json
build-container:
	@mkdir -p "$(_LOG_DIR)"
	@{ \
		docker build --progress=rawjson \
			--add-host "host.docker.internal:host-gateway" \
			--build-arg "OTEL_EXPORTER_OTLP_ENDPOINT=$(OTEL_TEST_ENDPOINT)" \
			-t "$(IMAGE_NAME):$(IMAGE_TAG)" . ; \
	} $(call _tee-log,build-container) ; \
	wait

# @ name: Run
# @ description: Compiles and runs the MCP server locally using the default configuration.
# @ risk: low
# @ read-only: false
# @ destructive: false
# @ idempotent: false
# @ open-world: true
# @ param: none
# @ output: MCP server logs
# @ output-type: text/plain
run: build
	"./bin/make-mcp" -config "make-mcp.yml"
