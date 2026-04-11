SHELL=/usr/bin/env bash
LGTM_VERSION:= 0.23.0
DEV_DIR=./dev
_LOG_DIR := $(DEV_DIR)/logs
TRUFFLEHOG_VERSION=3.94.3
TRIVY_VERSION=0.69.3
# Tee stdout and stderr to both the terminal and a recipe log file while
# preserving the original streams. Requires bash (process substitution).
# Usage: append $(call _tee-log,<name>) to the end of a { ... } group,
# followed by ; wait to flush the tee coprocesses before the shell exits.
# $(1) = log file basename (written to _LOG_DIR/NAME.log)
_tee-log = > >(tee -a "$(_LOG_DIR)/$(1).log") 2> >(tee -a "$(_LOG_DIR)/$(1).log" >&2)
GITHUB_OWNER=aidanhall34
IMAGE_NAME=ghcr.io/$(GITHUB_OWNER)/make-mcp
TEST_IMAGE_NAME=$(IMAGE_NAME)-test
# OTEL_TEST_ENDPOINT controls where test spans and metrics are sent during a
# container build. Override with an empty string to disable test telemetry.
OTEL_TEST_ENDPOINT ?= http://localhost:4317
_GIT_TAG  := $(shell git tag --points-at HEAD | tr '\n' ' ' | xargs -r semver 2>/dev/null | head -1)
_GIT_SHA  := $(shell git rev-parse --short HEAD)
IMAGE_TAG ?= $(if $(_GIT_TAG),$(_GIT_TAG),$(_GIT_SHA))
DIST_DIR ?= ./dist
ARCH ?= amd64
ACT_IMAGE ?= ghcr.io/catthehacker/ubuntu:act-latest
ACT_WORKFLOW ?= ./.github/workflows/ci.yml
ACT_EVENT ?= push
ACT_JOB ?=
ACT_SECRET_FILE ?= ./.act.secrets
ACT_OTEL_ENDPOINT ?= http://host.docker.internal:4317
ACT_CONCURRENT_JOBS ?= 2
DEV_COMPOSE_PROJECT ?= make-mcp-dev
_K6_SCRIPT_SLUG := $(shell printf '%s' "$(notdir $(K6_SCRIPT))" | tr '[:upper:]' '[:lower:]' | tr -cs '[:alnum:]' '-')
INTEGRATION_COMPOSE_PROJECT ?= make-mcp-integration-$(_K6_SCRIPT_SLUG)-$(_GIT_SHA)
K6_OUT ?= experimental-opentelemetry
K6_OTEL_GRPC_EXPORTER_ENDPOINT ?= host.docker.internal:4317
K6_OTEL_GRPC_EXPORTER_INSECURE ?= true
K6_OTEL_SERVICE_NAME ?= make-mcp-integration
OTEL_TRACES_EXPORTER ?= otlp
OTEL_METRICS_EXPORTER ?= otlp
OTEL_EXPORTER_OTLP_INSECURE ?= true
OTEL_SERVICE_NAME ?= make-mcp-integration-server
OTEL_METRIC_EXPORT_INTERVAL ?= 2000
_DEV_LGTM_COMPOSE := -f "$(DEV_DIR)/docker-compose.lgtm.yml"
_DEV_GRAFANA_MCP_COMPOSE := -f "$(DEV_DIR)/docker-compose.grafana-mcp.yml"
_DEV_FULL_COMPOSE := $(_DEV_LGTM_COMPOSE) $(_DEV_GRAFANA_MCP_COMPOSE)
_INTEGRATION_SERVER_COMPOSE := -f "$(DEV_DIR)/docker-compose.integration.server.yml"
_INTEGRATION_RUNNER_COMPOSE := -f "$(DEV_DIR)/docker-compose.integration.runner.yml"
_INTEGRATION_STACK_COMPOSE := $(_INTEGRATION_SERVER_COMPOSE) $(_INTEGRATION_RUNNER_COMPOSE)
_INTEGRATION_BINARY_RUNNER_COMPOSE := -f "$(DEV_DIR)/docker-compose.integration.binary-runner.yml"
_INTEGRATION_TELEMETRY_COMPOSE := -f "$(DEV_DIR)/docker-compose.integration.telemetry-host.yml"
_DEV_COMPOSE_ENV := COMPOSE_PROJECT_NAME="$(DEV_COMPOSE_PROJECT)"
_INTEGRATION_COMPOSE_ENV := COMPOSE_PROJECT_NAME="$(INTEGRATION_COMPOSE_PROJECT)"

# @ name: Clean
# @ description: Removes all build artifacts, logs, temporary files, and node modules.
# @ risk: medium
# @ read-only: false
# @ destructive: true
# @ idempotent: true
# @ open-world: false
# @ param: none
# @ output: None
# @ output-type: text/plain
clean:
	rm -rf "./bin"
	rm -rf "$(DIST_DIR)"
	rm -rf "$(DEV_DIR)/logs"
	rm -rf "$(DEV_DIR)/tmp"
	go clean -testcache

# @ name: Test
# @ description: Runs all Go unit and benchmark tests with race detection and coverage enabled. Each package enforces its own coverage threshold via TestMain and emits per-file JSON coverage to stderr.
# @ risk: low
# @ read-only: true
# @ destructive: false
# @ idempotent: true
# @ open-world: false
# @ param: none
# @ output: Test results, per-file coverage JSON, and pass/fail summary
# @ output-type: text/plain
tests: unit-tests bench


# @ name: Test
# @ description: Runs all Go unit tests with race detection and coverage enabled. Each package enforces its own coverage threshold via TestMain and emits per-file JSON coverage to stderr.
# @ risk: low
# @ read-only: true
# @ destructive: false
# @ idempotent: true
# @ open-world: false
# @ param: none
# @ output: Test results, per-file coverage JSON, and pass/fail summary
# @ output-type: text/plain
unit-tests:
	@mkdir -p "$(_LOG_DIR)"
	@{ go test -race -cover -coverprofile=coverage.out ./... ; } \
		$(call _tee-log,unit-tests) ; \
	wait

# @ name: Benchmark
# @ description: Runs all Go benchmark tests across every package and reports memory allocations.
# @ risk: low
# @ read-only: true
# @ destructive: false
# @ idempotent: true
# @ open-world: false
# @ param: none
# @ output: Benchmark results with ns/op, B/op, and allocs/op per benchmark
# @ output-type: text/plain
bench:
	@mkdir -p "$(_LOG_DIR)"
	@{ go test -bench=. -benchmem -run='^$$' ./... ; } \
		$(call _tee-log,bench) ; \
	wait

# @ name: Format
# @ description: Formats all Go source and test files in the cmd and pkg directories.
# @ risk: low
# @ read-only: false
# @ destructive: false
# @ idempotent: true
# @ open-world: false
# @ param: none
# @ output: Formatted Go files
# @ output-type: text/plain
format:
	gofmt -w $$(find ./cmd ./pkg -name '*.go' -type f)

# @ name: Tidy
# @ description: Synchronizes the go.mod and go.sum files with the source code.
# @ risk: low
# @ read-only: false
# @ destructive: false
# @ idempotent: true
# @ open-world: true
# @ param: none
# @ output: None
# @ output-type: text/plain
tidy:
	go mod tidy
	go mod verify

# @ name: Lint Tidy
# @ description: Verifies that go.mod and go.sum are synchronized (fails if changes are needed).
# @ risk: low
# @ read-only: true
# @ destructive: false
# @ idempotent: true
# @ open-world: true
# @ param: none
# @ output: Git diff exit code and any required changes
# @ output-type: text/plain
lint-tidy:
	@mkdir -p "$(_LOG_DIR)"
	@{ \
		go mod tidy ; \
		git diff --exit-code "go.mod" "go.sum" ; \
	} $(call _tee-log,lint-tidy) ; \
	wait

# @ name: Lint Markdown
# @ description: Lints all markdown files in the repository with markdownlint.
# @ risk: low
# @ read-only: true
# @ destructive: false
# @ idempotent: true
# @ open-world: false
# @ param: none
# @ output: Markdown lint results and any rule violations
# @ output-type: text/plain
lint-markdown:
	@mkdir -p "$(_LOG_DIR)"
	@{ npm run lint:markdown ; } \
		$(call _tee-log,lint-markdown) ; \
	wait

# @ name: Lint Go
# @ description: Verifies that Go files are formatted and pass go vet.
# @ risk: low
# @ read-only: true
# @ destructive: false
# @ idempotent: true
# @ open-world: false
# @ param: none
# @ output: gofmt check results and go vet diagnostics
# @ output-type: text/plain
lint-go:
	@mkdir -p "$(_LOG_DIR)"
	@{ \
		out="$$(gofmt -l $$(find ./cmd ./pkg ./internal -name '*.go' -type f))" ; \
		if [ -n "$$out" ]; then \
			printf '%s\n' "$$out" ; \
			exit 1 ; \
		fi ; \
		go vet ./... ; \
	} $(call _tee-log,lint-go) ; \
	wait

# @ name: Scan Secrets
# @ description: Scans the repository for secrets using TruffleHog in a Docker container.
# @ risk: low
# @ read-only: true
# @ destructive: false
# @ idempotent: true
# @ open-world: true
# @ param: none
# @ output: TruffleHog scan results
# @ output-type: text/plain
scan-secrets:
	@mkdir -p "$(_LOG_DIR)"
	@{ docker run --rm -v "$(CURDIR):/pwd" trufflesecurity/trufflehog:$(TRUFFLEHOG_VERSION) git file:///pwd --only-verified --fail ; } \
		$(call _tee-log,scan-secrets) ; \
	wait

# @ name: Scan Vulnerabilities
# @ description: Scans the repository for vulnerabilities using Trivy in a Docker container.
# @ risk: low
# @ read-only: true
# @ destructive: false
# @ idempotent: true
# @ open-world: true
# @ param: none
# @ output: Trivy vulnerability scan results
# @ output-type: text/plain
scan-vulnerabilities:
	@mkdir -p "$(_LOG_DIR)"
	@{ docker run --rm -v "$(CURDIR):/root" aquasec/trivy:$(TRIVY_VERSION) fs --exit-code 1 --severity HIGH,CRITICAL /root ; } \
		$(call _tee-log,scan-vulnerabilities) ; \
	wait

# @ name: Scan Container
# @ description: Scans the locally built make-mcp container image for vulnerabilities using Trivy in a Docker container. Requires IMAGE_TAG (default: latest).
# @ risk: low
# @ read-only: true
# @ destructive: false
# @ idempotent: true
# @ open-world: true
# @ param: IMAGE_TAG string | Container image tag to scan (default: latest)
# @ output: Trivy container scan results
# @ output-type: text/plain
scan-container:
	@mkdir -p "$(_LOG_DIR)"
	@{ docker run --rm -v /var/run/docker.sock:/var/run/docker.sock aquasec/trivy:$(TRIVY_VERSION) image --exit-code 1 --severity HIGH,CRITICAL "$(IMAGE_NAME):$(IMAGE_TAG)" ; } \
		$(call _tee-log,scan-container) ; \
	wait

# @ name: Scan Test Container
# @ description: Scans the locally built make-mcp test container image for vulnerabilities using Trivy in a Docker container. Requires IMAGE_TAG (default: latest).
# @ risk: low
# @ read-only: true
# @ destructive: false
# @ idempotent: true
# @ open-world: true
# @ param: IMAGE_TAG string | Container image tag to scan (default: latest)
# @ output: Trivy container scan results
# @ output-type: text/plain
scan-test-container:
	@mkdir -p "$(_LOG_DIR)"
	@{ docker run --rm -v /var/run/docker.sock:/var/run/docker.sock aquasec/trivy:$(TRIVY_VERSION) image --exit-code 1 --severity HIGH,CRITICAL "$(TEST_IMAGE_NAME):$(IMAGE_TAG)" ; } \
		$(call _tee-log,scan-test-container) ; \
	wait

# @ name: Lint
# @ description: Runs repository linting and validation checks required by CI.
# @ risk: low
# @ read-only: true
# @ destructive: false
# @ idempotent: true
# @ open-world: false
# @ param: none
# @ output: Lint and validation results
# @ output-type: text/plain
lint: lint-go lint-markdown lint-tidy validate

# @ name: Build
# @ description: Compiles all binaries and builds all containers
# @ risk: low
# @ read-only: false
# @ destructive: false
# @ idempotent: true
# @ open-world: false
# @ param: none
# @ output: Binaries at ./bin/
# @ output-type: application/octet-stream
build: tests build-validator build-mcp-server integration-build-k6 build-container build-test-container

# Sets up Grafana LGTM versions, configures a symlink so GEMINI.md can read the AGENTS.md file,
# and installs a git pre-commit hook that runs lint and unit tests before each commit.
setup:
	printf "LGTM_VERSION=$(LGTM_VERSION)" > "$(DEV_DIR)/compose_versions"
	ln -sf AGENTS.md GEMINI.md
	npm install
	printf '#!/usr/bin/env sh\nmake pre-commit\n' > .git/hooks/pre-commit
	chmod +x .git/hooks/pre-commit
	printf '#!/usr/bin/env sh\nnpx --no -- commitlint --edit "$$1"\n' > .git/hooks/commit-msg
	chmod +x .git/hooks/commit-msg


.PHONY: generate-wiki-sidebar
# @ name: Generate Wiki Sidebar
# @ description: Generates the wiki sidebar from all .md files in the wiki directory. Fails if the sidebar was outdated.
# @ risk: low
# @ read-only: false
# @ destructive: false
# @ idempotent: true
# @ open-world: false
# @ param: none
# @ output: Updated wiki/_Sidebar.md or a success message
# @ output-type: text/plain
generate-wiki-sidebar:
	@./scripts/gen-wiki-sidebar.sh

.PHONY: pre-commit
# @ name: Pre-commit
# @ description: Runs linting, unit tests, and security scans. Installed as a git pre-commit hook by the setup recipe.
# @ risk: low
# @ read-only: true
# @ destructive: false
# @ idempotent: true
# @ open-world: false
# @ param: none
# @ output: Lint, test, and scan results
# @ output-type: text/plain
pre-commit: generate-wiki-sidebar lint unit-tests scan-secrets scan-vulnerabilities

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

# @ name: Build MCP Server
# @ description: Compiles the MCP server CLI binary to ./bin/make-mcp.
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

# @ name: Install
# @ description: Installs the mcp-server and validator binaries to the user's Go bin directory.
# @ risk: low
# @ read-only: false
# @ destructive: false
# @ idempotent: true
# @ open-world: true
# @ param: none
# @ output: Installation logs
# @ output-type: text/plain
install:
	go install "./cmd/mcp-server"
	go install "./cmd/validate"

# @ name: Validate Makefile
# @ description: Runs the validator against the project Makefile using the make-mcp.yml config.
# @ risk: low
# @ read-only: true
# @ destructive: false
# @ idempotent: true
# @ open-world: false
# @ param: none
# @ output: Validation report listing all annotated recipes and any errors
# @ output-type: text/plain
validate:
	go run ./cmd/validate --config make-mcp.yml --strict

# @ name: Hello World
# @ description: Prints a friendly greeting. Used to verify the MCP server toolchain is operational.
# @ risk: low
# @ read-only: true
# @ destructive: false
# @ idempotent: true
# @ open-world: false
# @ param: none
# @ output: A greeting message string
# @ output-type: text/plain
hello-world:
	@echo "Hello, World!"

## Development

.PHONY: otelcol-validate
# @ name: Validate OpenTelemetry collector configuration
# @ description: Validate changes to the Dev Opentelemetry collector configurations
# @ risk: low
# @ read-only: true
# @ destructive: false
# @ idempotent: true
# @ open-world: true
# @ param: none
# @ output: Blank output indicates that the validation was successful.
# @ output-type: text/plain
otelcol-validate:
	@{ docker run --rm \
		-v "$(CURDIR)/$(DEV_DIR)/otelcol-config.yaml:/otel-lgtm/otelcol-config.yaml:ro" \
		--entrypoint="" \
		grafana/otel-lgtm:$(LGTM_VERSION) \
		/otel-lgtm/otelcol-contrib/otelcol-contrib validate \
			--feature-gates service.profilesSupport \
			--config=file:/otel-lgtm/otelcol-config.yaml ; }

.PHONY: prometheus-validate
# @ name: Validate Prometheus configuration
# @ description: Validate changes to the Dev Prometheus configurations
# @ risk: low
# @ read-only: true
# @ destructive: false
# @ idempotent: true
# @ open-world: true
# @ param: none
# @ output: The results of validating the Prometheus configuration use in the dev folder
# @ output-type: text/plain
prometheus-validate:
	@{ docker run --rm \
		-v "$(CURDIR)/$(DEV_DIR)/prometheus.yaml:/etc/prometheus/prometheus.yaml:ro" \
		--entrypoint="" \
		grafana/otel-lgtm:$(LGTM_VERSION) \
		/otel-lgtm/prometheus/promtool check config \
			/etc/prometheus/prometheus.yaml; }

.PHONY: dev-volumes
# @ name: Development docker volumes
# @ description: Creates docker volumes for the docker compose lgtm stack (idempotent)
# @ risk: low
# @ read-only: false
# @ destructive: false
# @ idempotent: true
# @ open-world: true
# @ param: none
# @ output: Confirmation that the volume has been created
# @ output-type: text/plain
dev-volumes:
	docker volume create make-mcp-lgtm-prometheus-data
	docker volume create make-mcp-lgtm-loki-data
	docker volume create make-mcp-lgtm-tempo-data

.PHONY: dev-up
# @ name: Start development dependencies
# @ description: Starts the full local development stack, including LGTM and the Grafana MCP sidecar (idempotent)
# @ risk: low
# @ read-only: false
# @ destructive: false
# @ idempotent: true
# @ open-world: true
# @ param: none
# @ output: none
# @ output-type: text/plain
dev-up: dev-volumes
	@{ $(_DEV_COMPOSE_ENV) docker compose \
		$(_DEV_FULL_COMPOSE) \
		--env-file="$(DEV_DIR)/compose_versions" \
		up \
		-d \
		--wait ; }

.PHONY: dev-up-lgtm
dev-up-lgtm: dev-volumes
	@{ $(_DEV_COMPOSE_ENV) docker compose \
		$(_DEV_LGTM_COMPOSE) \
		--env-file="$(DEV_DIR)/compose_versions" \
		up \
		-d \
		--wait ; }

.PHONY: dev-down
# @ name: Stop development dependencies
# @ description: Stops the full local development stack, including LGTM and the Grafana MCP sidecar
# @ risk: medium
# @ read-only: false
# @ destructive: true
# @ idempotent: true
# @ open-world: true
# @ param: none
# @ output: The shutdown logs of the running containers in the stack.
# @ output-type: text/plain
dev-down: ## Stop and remove the local LGTM development stack
	@{ $(_DEV_COMPOSE_ENV) docker compose \
		--env-file="$(DEV_DIR)/compose_versions" \
		$(_DEV_FULL_COMPOSE) \
		down ; }

.PHONY: dev-logs
# @ name: Fetch Docker Compose Logs
# @ description: Fetches docker compose service logs for the full local development stack
# @ risk: low
# @ read-only: true
# @ destructive: false
# @ idempotent: true
# @ open-world: true
# @ param: none
# @ output: Docker container logs
# @ output-type: text/plain
dev-logs:
	$(_DEV_COMPOSE_ENV) docker compose $(_DEV_FULL_COMPOSE) logs

# Tails logs from the LGTM dev stack
.PHONY: dev-logs-tail
dev-logs-tail:
	$(_DEV_COMPOSE_ENV) docker compose $(_DEV_FULL_COMPOSE) logs -f

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

.PHONY: build-test-container
# @ name: Build Test Container
# @ description: Builds a minimal testing container for CI that includes make and other utilities. This image is used for integration tests and serves as an example for users.
# @ risk: low
# @ read-only: false
# @ destructive: false
# @ idempotent: true
# @ open-world: false
# @ param: IMAGE_TAG string | Container image tag to apply (default: latest)
# @ output: Docker build output
# @ output-type: application/octet-stream
build-test-container: build-container
	@mkdir -p "$(_LOG_DIR)"
	@{ \
		docker build --progress=rawjson \
			--build-arg "MAKE_MCP_IMAGE=$(IMAGE_NAME):$(IMAGE_TAG)" \
			-t "$(TEST_IMAGE_NAME):$(IMAGE_TAG)" \
			-f "$(DEV_DIR)/integration/Dockerfile.test-server" \
			"$(DEV_DIR)/integration" ; \
	} $(call _tee-log,build-test-container) ; \
	wait

# @ name: Publish Container
# @ description: Builds and pushes the make-mcp container image to the GitHub Container Registry. Requires IMAGE_TAG (default: latest). Authenticates via the gh CLI.
# @ risk: high
# @ read-only: false
# @ destructive: false
# @ idempotent: true
# @ open-world: true
# @ param: IMAGE_TAG - Container image tag to build and push (default: latest)
# @ output: Docker build and push output as JSON progress records
# @ output-type: application/json
.PHONY: publish
publish: build-container
	gh auth token | docker login ghcr.io -u "$(GITHUB_OWNER)" --password-stdin
	docker push --progress=rawjson "$(IMAGE_NAME):$(IMAGE_TAG)"

## Integration testing

.PHONY: integration-debug
# @ name: Integration Debug (local LGTM)
# @ description: Ensures the dev LGTM stack and MCP server are running, then attaches k6 in detached mode. LGTM is never stopped by this target — use dev-down to tear it down manually.
# @ risk: low
# @ read-only: true
# @ destructive: false
# @ idempotent: true
# @ open-world: true
# @ param: none
# @ output: k6 integration test results and pass/fail summary
# @ output-type: text/plain
integration-debug: integration-build-k6
	@{ \
		set -e ; \
		mkdir -p "$(DEV_DIR)/tmp" ; \
		$(_DEV_COMPOSE_ENV) docker compose $(_DEV_FULL_COMPOSE) \
			--env-file="$(DEV_DIR)/compose_versions" up -d --wait ; \
		$(MAKE) integration-server-up INTEGRATION_COMPOSE_FILES="$(_INTEGRATION_OTEL_COMPOSE)" ; \
		$(MAKE) integration-k6-attach INTEGRATION_COMPOSE_FILES="$(_INTEGRATION_OTEL_COMPOSE)" ; \
	}

K6_IMAGE ?= make-mcp-k6:local
K6_SCRIPT ?= /scripts/integration.js

# OTel env vars forwarded to the integration containers.
_K6_OTEL_ENV := K6_OUT=$(K6_OUT) \
	K6_OTEL_GRPC_EXPORTER_ENDPOINT=$(K6_OTEL_GRPC_EXPORTER_ENDPOINT) \
	K6_OTEL_GRPC_EXPORTER_INSECURE=$(K6_OTEL_GRPC_EXPORTER_INSECURE) \
	K6_OTEL_SERVICE_NAME=$(K6_OTEL_SERVICE_NAME)

_SERVER_OTEL_ENV := OTEL_TRACES_EXPORTER=$(OTEL_TRACES_EXPORTER) \
	OTEL_METRICS_EXPORTER=$(OTEL_METRICS_EXPORTER) \
	OTEL_EXPORTER_OTLP_ENDPOINT=$(OTEL_EXPORTER_OTLP_ENDPOINT) \
	OTEL_EXPORTER_OTLP_INSECURE=$(OTEL_EXPORTER_OTLP_INSECURE) \
	OTEL_SERVICE_NAME=$(OTEL_SERVICE_NAME) \
	OTEL_METRIC_EXPORT_INTERVAL=$(OTEL_METRIC_EXPORT_INTERVAL)

# All env vars forwarded to every integration container.
_INTEGRATION_RUN_ENV = \
	MAKE_MCP_IMAGE="$(TEST_IMAGE_NAME):$(IMAGE_TAG)" \
	K6_IMAGE="$(K6_IMAGE)" \
	K6_SCRIPT="$(K6_SCRIPT)" \
	$(_SERVER_OTEL_ENV) \
	$(_K6_OTEL_ENV)

# Compose files for integration runs that include a telemetry host sidecar.
_INTEGRATION_OTEL_COMPOSE := $(_INTEGRATION_STACK_COMPOSE) $(_INTEGRATION_TELEMETRY_COMPOSE)

# ---------------------------------------------------------------------------
# Composable integration helpers
# ---------------------------------------------------------------------------

# Builds the custom k6+xk6-mcp image used by the integration test stack.
.PHONY: integration-build-k6
integration-build-k6:
	docker build --progress=rawjson \
		-f "$(DEV_DIR)/integration/k6/Dockerfile" \
		-t "$(K6_IMAGE)" \
		"$(DEV_DIR)/integration/k6"

# Tears down the integration stack (idempotent, never fails).
.PHONY: integration-stack-down
integration-stack-down:
	-$(_INTEGRATION_COMPOSE_ENV) docker compose \
		$(INTEGRATION_COMPOSE_FILES) \
		down --remove-orphans >/dev/null 2>&1

# Starts the MCP server container and waits until it is healthy.
.PHONY: integration-server-up
integration-server-up:
	$(_INTEGRATION_RUN_ENV) \
		$(_INTEGRATION_COMPOSE_ENV) docker compose \
		$(INTEGRATION_COMPOSE_FILES) \
		up -d --wait make-mcp-server

# Runs k6, blocking until it exits. Returns k6's exit code.
.PHONY: integration-k6-run
integration-k6-run:
	$(_INTEGRATION_RUN_ENV) \
		$(_INTEGRATION_COMPOSE_ENV) docker compose \
		$(INTEGRATION_COMPOSE_FILES) \
		up --abort-on-container-exit --exit-code-from k6 k6

# Starts k6 in detached mode (for interactive/debug use).
.PHONY: integration-k6-attach
integration-k6-attach:
	$(_INTEGRATION_RUN_ENV) \
		$(_INTEGRATION_COMPOSE_ENV) docker compose \
		$(INTEGRATION_COMPOSE_FILES) \
		up -d --no-deps k6

# ---------------------------------------------------------------------------
# Integration run macros
# $(1) = compose file flags (e.g. $(_INTEGRATION_STACK_COMPOSE) or $(_INTEGRATION_OTEL_COMPOSE))
# ---------------------------------------------------------------------------

# Standard run: set up tmp, register cleanup trap, start server, run k6.
define _integration-run
	@{ \
		set -e ; \
		mkdir -p "$(DEV_DIR)/tmp" ; \
		trap '$(MAKE) integration-stack-down INTEGRATION_COMPOSE_FILES="$(1)" ; rm -rf "$(DEV_DIR)/tmp"' EXIT INT TERM ; \
		$(MAKE) integration-stack-down INTEGRATION_COMPOSE_FILES="$(1)" ; \
		$(MAKE) integration-server-up INTEGRATION_COMPOSE_FILES="$(1)" ; \
		$(MAKE) integration-k6-run INTEGRATION_COMPOSE_FILES="$(1)" ; \
	}
endef

# LGTM run: like standard but also starts and tears down the dev LGTM stack.
define _integration-run-with-lgtm
	@{ \
		set -e ; \
		mkdir -p "$(DEV_DIR)/tmp" ; \
		$(_DEV_COMPOSE_ENV) docker compose $(_DEV_FULL_COMPOSE) \
			--env-file="$(DEV_DIR)/compose_versions" up -d --wait ; \
		trap '$(MAKE) integration-stack-down INTEGRATION_COMPOSE_FILES="$(1)" ; \
			rm -rf "$(DEV_DIR)/tmp" ; \
			$(_DEV_COMPOSE_ENV) docker compose $(_DEV_FULL_COMPOSE) \
				--env-file="$(DEV_DIR)/compose_versions" down' EXIT INT TERM ; \
		$(MAKE) integration-stack-down INTEGRATION_COMPOSE_FILES="$(1)" ; \
		$(MAKE) integration-server-up INTEGRATION_COMPOSE_FILES="$(1)" ; \
		$(MAKE) integration-k6-run INTEGRATION_COMPOSE_FILES="$(1)" ; \
	}
endef

# Binary run: starts the mcp-server binary on the host in HTTP mode, waits
# until /ready responds, then runs k6 via the binary runner compose (which
# reaches the host via host.docker.internal). Tears down k6 and the binary
# server on exit.
# $(1) = compose file flags for the k6 binary runner
define _integration-run-binary
	@{ \
		set -e ; \
		mkdir -p "$(DEV_DIR)/tmp" ; \
		mcp_server="$(DIST_DIR)/make-mcp_linux_$(ARCH)" ; \
		chmod +x "$$mcp_server" ; \
		printf 'starting make-mcp for binary integration (%s)...\n' "$(ARCH)" ; \
		"$$mcp_server" \
			--config "$(DEV_DIR)/integration/make-mcp.yml" \
			--makefile "testdata/Makefile" & \
		server_pid=$$! ; \
		trap '$(MAKE) integration-stack-down INTEGRATION_COMPOSE_FILES="$(1)" ; \
			kill "$$server_pid" 2>/dev/null || true ; \
			rm -rf "$(DEV_DIR)/tmp"' EXIT INT TERM ; \
		retries=20 ; \
		until curl -sf "http://localhost:9378/ready" >/dev/null 2>&1 ; do \
			retries=$$((retries - 1)) ; \
			[ "$$retries" -gt 0 ] || { printf 'mcp-server did not become ready\n' ; exit 1 ; } ; \
			sleep 1 ; \
		done ; \
		printf 'mcp-server ready\n' ; \
		$(MAKE) integration-stack-down INTEGRATION_COMPOSE_FILES="$(1)" ; \
		$(MAKE) integration-k6-run INTEGRATION_COMPOSE_FILES="$(1)" ; \
	}
endef

# ---------------------------------------------------------------------------
# Integration test targets
# ---------------------------------------------------------------------------

.PHONY: integration
# @ name: Integration Tests
# @ description: Builds everything, starts the MCP server container, runs MCP protocol integration tests via k6, then tears everything down.
# @ risk: medium
# @ read-only: true
# @ destructive: false
# @ idempotent: true
# @ open-world: true
# @ param: none
# @ output: k6 integration test results and pass/fail summary
# @ output-type: text/plain
integration: build
	$(call _integration-run,$(_INTEGRATION_STACK_COMPOSE))

.PHONY: integration-lgtm
# @ name: Integration Tests (with LGTM)
# @ description: Starts the full local LGTM development stack, runs integration tests with OTel telemetry, then tears down both the integration stack and the LGTM stack on exit.
# @ risk: medium
# @ read-only: true
# @ destructive: false
# @ idempotent: true
# @ open-world: true
# @ param: none
# @ output: k6 integration test results and pass/fail summary
# @ output-type: text/plain
integration-lgtm: integration-build-k6 build-container dev-volumes
	$(call _integration-run-with-lgtm,$(_INTEGRATION_OTEL_COMPOSE))

.PHONY: integration-otel
# @ name: Integration Tests (OTel, LGTM already running)
# @ description: Runs integration tests with OTel output to an existing OTLP collector. Does not start or stop the collector; tears down only the integration containers on exit.
# @ risk: medium
# @ read-only: true
# @ destructive: false
# @ idempotent: true
# @ open-world: true
# @ param: none
# @ output: k6 integration test results and pass/fail summary
# @ output-type: text/plain
integration-otel: integration-build-k6 build-container
	$(call _integration-run,$(_INTEGRATION_OTEL_COMPOSE))

.PHONY: integration-act
integration-act: integration-build-k6 build-container build-test-container dev-up-lgtm
	$(call _integration-run,$(_INTEGRATION_OTEL_COMPOSE))

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
smoke-test-container: build-test-container
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
			--transport http --listen 0.0.0.0:9378 ; \
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

# @ name: Upload Discord Webhook Secret
# @ description: Stores a Discord webhook URL as the DISCORD_WEBHOOK_URL GitHub Actions secret using the gh CLI, and also writes it to the local ACT_SECRET_FILE (.act.secrets) so act can read it when running workflows locally.
# @ risk: high
# @ read-only: false
# @ destructive: false
# @ idempotent: true
# @ open-world: true
# @ param: WEBHOOK_URL string | Discord webhook URL to store as the DISCORD_WEBHOOK_URL GitHub Actions secret
# @ output: Confirmation that the repository secret has been updated and the local secrets file has been written
# @ output-type: text/plain
.PHONY: upload-discord-webhook
upload-discord-webhook:
	@{ \
		set -e ; \
		webhook_url="$$(printf '%s' "$$WEBHOOK_URL" | tr -d '\r\n')" ; \
		if [ -z "$$webhook_url" ]; then \
			printf '%s\n' "WEBHOOK_URL is required" ; \
			exit 1 ; \
		fi ; \
		gh secret set "DISCORD_WEBHOOK_URL" --body "$$webhook_url" ; \
		printf '%s\n' "DISCORD_WEBHOOK_URL updated" ; \
		secret_file="$(ACT_SECRET_FILE)" ; \
		touch "$$secret_file" ; \
		if grep -q "^DISCORD_WEBHOOK_URL=" "$$secret_file" 2>/dev/null; then \
			sed -i "s|^DISCORD_WEBHOOK_URL=.*|DISCORD_WEBHOOK_URL=$$webhook_url|" "$$secret_file" ; \
		else \
			printf 'DISCORD_WEBHOOK_URL=%s\n' "$$webhook_url" >> "$$secret_file" ; \
		fi ; \
		printf '%s\n' "DISCORD_WEBHOOK_URL written to $$secret_file for act" ; \
	}

# @ name: Run GitHub Actions Locally With ACT Using the Current GH Token
# @ description: Fetches the current gh CLI token and passes it directly to act as GH_TOKEN and GITHUB_TOKEN secrets without writing the token to disk.
# @ risk: low
# @ read-only: false
# @ destructive: false
# @ idempotent: true
# @ open-world: true
# @ param: none
# @ output: act workflow execution logs
# @ output-type: text/plain
.PHONY: act-run
act-run:
	@mkdir -p "$(_LOG_DIR)"
	@{ \
		set -e ; \
		act_args="" ; \
		if [ -n "$(ACT_JOB)" ]; then \
			act_args="$$act_args --job $(ACT_JOB)" ; \
		fi ; \
		if [ -f "$(ACT_SECRET_FILE)" ]; then \
			act_args="$$act_args --secret-file $(ACT_SECRET_FILE)" ; \
		fi ; \
		token="$$(gh auth token)" ; \
		if [ -z "$$token" ]; then \
			printf '%s\n' "gh auth token returned an empty token" ; \
			exit 1 ; \
		fi ; \
		act \
			--json \
			--workflows "$(ACT_WORKFLOW)" \
			--platform "ubuntu-latest=$(ACT_IMAGE)" \
			--container-options "--add-host=host.docker.internal:host-gateway" \
			--env "ACT=true" \
			--env "ACT_OTEL_EXPORTER_OTLP_ENDPOINT=$(ACT_OTEL_ENDPOINT)" \
			--secret "GH_TOKEN=$$token" \
			--secret "GITHUB_TOKEN=$$token" \
			--concurrent-jobs "$(ACT_CONCURRENT_JOBS)" \
			$(ACT_EVENT) \
			$$act_args ; \
	} $(call _tee-log,act-run) ; \
	wait

# Runs a local instance of the mcp inspector for debugging
dev-mcp-inspector: build
	npx @modelcontextprotocol/inspector \
		-e "OTEL_TRACES_EXPORTER=otlp" \
		-e "OTEL_METRICS_EXPORTER=otlp" \
		-e "OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317" \
		-e "OTEL_EXPORTER_OTLP_INSECURE=true" \
		-e "OTEL_METRIC_EXPORT_INTERVAL=5000" \
		-- ./bin/make-mcp \
		--config make-mcp.yml ;
