SHELL=/usr/bin/env bash
LGTM_VERSION:= 0.23.0
DEV_DIR=./dev
_LOG_DIR := $(DEV_DIR)/logs
_MCP_PID_FILE        := $(CURDIR)/dev/run/make-mcp.pid
_LGTM_LOGS_PID_FILE  := $(CURDIR)/dev/run/lgtm-logs.pid
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
K6_OTEL_GRPC_EXPORTER_ENDPOINT ?=
K6_OTEL_GRPC_EXPORTER_INSECURE ?= true
K6_OTEL_SERVICE_NAME ?= make-mcp-integration
OTEL_TRACES_EXPORTER ?= otlp
OTEL_METRICS_EXPORTER ?= otlp
OTEL_EXPORTER_OTLP_ENDPOINT ?= http://host.docker.internal:4317
OTEL_EXPORTER_OTLP_INSECURE ?= true
OTEL_SERVICE_NAME ?= make-mcp-server
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

include dev/makefiles/helper.mk
include dev/makefiles/ci.mk
include dev/makefiles/go.mk
include dev/makefiles/lint.mk
include dev/makefiles/scan.mk
include dev/makefiles/build.mk
include dev/makefiles/generate.mk
include dev/makefiles/dev.mk
include dev/makefiles/github.mk

.PHONY: all
all: build

.PHONY: clean

.PHONY: test
test: tests

.PHONY: tests
# @ name: All Go lang tests
# @ description: Runs all Go unit and benchmark tests with race detection and coverage enabled. Each package enforces its own coverage threshold via TestMain and emits per-file JSON coverage to stderr. RUN ON EVERY CHANGE TO .go files
# @ risk: low
# @ read-only: true
# @ destructive: false
# @ idempotent: true
# @ open-world: false
# @ param: none
# @ output: Test results, per-file coverage JSON, and pass/fail summary
# @ output-type: text/plain
tests: unit-tests bench

.PHONY: lint
# @ name: Lint
# @ description: Runs repository linting and validation checks required by CI. RUN AFTER ALL FILE CHANGES
# @ risk: low
# @ read-only: true
# @ destructive: false
# @ idempotent: true
# @ open-world: false
# @ param: none
# @ output: Lint and validation results
# @ output-type: text/plain
lint: lint-dockerfile lint-makefile lint-go lint-markdown lint-tidy validate

.PHONY: build
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
build: tests build-validator build-mcp-server integration-build-k6 build-container build-test-containers

.PHONY: build-test-containers
# @ name: Build Test Containers
# @ description: Builds a minimal testing container for CI that includes make and other utilities. This image is used for integration tests and serves as an example for users.
# @ risk: low
# @ read-only: false
# @ destructive: false
# @ idempotent: true
# @ open-world: false
# @ param: IMAGE_TAG string | Container image tag to apply (default: latest)
# @ output: Docker build output
# @ output-type: application/octet-stream
build-test-containers: build-container build-test-image

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
