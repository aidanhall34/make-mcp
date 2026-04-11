SHELL=/usr/bin/env bash
LGTM_VERSION:= 0.23.0
DEV_DIR=./dev
_LOG_DIR := $(DEV_DIR)/logs
TRUFFLEHOG_VERSION=3.94.3
TRIVY_VERSION=0.69.3
HADOLINT_VERSION=2.12.0
GITHUB_OWNER=aidanhall34
IMAGE_NAME=ghcr.io/$(GITHUB_OWNER)/make-mcp
TEST_IMAGE_NAME=$(IMAGE_NAME)-test
# OTEL_TEST_ENDPOINT controls where test spans and metrics are sent during a
# container build. Override with an empty string to disable test telemetry.
OTEL_TEST_ENDPOINT ?= http://localhost:4317
K6_IMAGE ?= make-mcp-k6:local
K6_SCRIPT ?= /scripts/integration.js
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
_INTEGRATION_OTEL_COMPOSE := $(_INTEGRATION_STACK_COMPOSE) $(_INTEGRATION_TELEMETRY_COMPOSE)

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

# @ name: All Go lang tests
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


# @ name: Go lang Unit-tests
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

# @ name: Go lang Benchmarks
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

# @ name: Lint Dockerfile
# @ description: Lints all Dockerfiles in the repository with hadolint running in a Docker container. Fails if any exceptions are found.
# @ risk: low
# @ read-only: true
# @ destructive: false
# @ idempotent: true
# @ open-world: true
# @ param: none
# @ output: hadolint lint results
# @ output-type: text/plain
lint-dockerfile:
	@mkdir -p "$(_LOG_DIR)"
	@{ \
		dockerfiles="$$(find . -name "Dockerfile*" -not -path "./node_modules/*" | sort | tr '\n' ' ')" ; \
		docker run --rm \
			-v "$(CURDIR):/workspace" \
			-w "/workspace" \
			hadolint/hadolint:v$(HADOLINT_VERSION) \
			hadolint $$dockerfiles ; \
	} $(call _tee-log,lint-dockerfile) ; \
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
lint: lint-dockerfile lint-go lint-markdown lint-tidy validate

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

.PHONY: gen-metrics-doc
# @ name: Generate Metrics Documentation
# @ description: Parses pkg/telemetry/metrics.go via the Go AST and generates wiki/Metrics.md from docs/metrics.md.tmpl. Fails if the committed file was out of date.
# @ risk: low
# @ read-only: false
# @ destructive: false
# @ idempotent: true
# @ open-world: false
# @ param: none
# @ output: Updated wiki/Metrics.md or a success message
# @ output-type: text/plain
gen-metrics-doc:
	@go run ./cmd/gen-metrics-doc

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
pre-commit: generate-wiki-sidebar gen-metrics-doc lint unit-tests scan-secrets scan-vulnerabilities

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
build-test-container: build-container build-test-image

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

# @ name: Upload Branch Protection Rules
# @ description: Applies branch protection rules to GitHub for each JSON file in .github/branch-protection/. Each file must be named after the branch it protects (e.g. main.json). Reads the current repo from the gh CLI.
# @ risk: high
# @ read-only: false
# @ destructive: false
# @ idempotent: true
# @ open-world: true
# @ param: none
# @ output: Confirmation that branch protection was applied for each branch
# @ output-type: text/plain
.PHONY: upload-branch-protection
upload-branch-protection:
	@{ \
		set -e ; \
		repo="$$(gh repo view --json nameWithOwner -q .nameWithOwner)" ; \
		for f in .github/branch-protection/*.json; do \
			branch="$$(basename "$$f" .json)" ; \
			printf 'applying branch protection for %s/%s...\n' "$$repo" "$$branch" ; \
			gh api \
				--method PUT \
				-H "Accept: application/vnd.github+json" \
				"/repos/$$repo/branches/$$branch/protection" \
				--input "$$f" ; \
			printf 'branch protection applied for %s\n' "$$branch" ; \
		done ; \
	}

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

include ./dev/makefiles/helper.mk
include ./dev/makefiles/ci.mk
