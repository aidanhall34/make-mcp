SHELL=/usr/bin/env bash

LGTM_VERSION := 0.23.0
KEYCLOAK_VERSION := 26.6.0
VENV ?= .venv
DEV_DIR = ./dev
_LOG_DIR := $(DEV_DIR)/logs
_GIT_TAG  := $(shell git tag --points-at HEAD | tr '\n' ' ' | xargs -r semver 2>/dev/null | head -1)
_GIT_SHA  := $(shell git rev-parse --short HEAD)
K6_IMAGE ?= make-mcp-k6:local
K6_SCRIPT ?= /scripts/integration.js
_K6_SCRIPT_SLUG := $(shell printf '%s' "$(notdir $(K6_SCRIPT))" | tr '[:upper:]' '[:lower:]' | tr -cs '[:alnum:]' '-')
DEV_COMPOSE_PROJECT ?= make-mcp-dev
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
OAUTH ?= false
CERTS_DIR ?= $(DEV_DIR)/certs

_DEV_LGTM_COMPOSE := -f "$(DEV_DIR)/docker-compose.lgtm.yml"
_DEV_GRAFANA_MCP_COMPOSE := -f "$(DEV_DIR)/docker-compose.grafana-mcp.yml"
_DEV_KEYCLOAK_COMPOSE := $(if $(filter true,$(OAUTH)),-f "$(DEV_DIR)/docker-compose.keycloak.yml",)
_DEV_FULL_COMPOSE := $(_DEV_LGTM_COMPOSE) $(_DEV_GRAFANA_MCP_COMPOSE) $(_DEV_KEYCLOAK_COMPOSE)
_INTEGRATION_SERVER_COMPOSE := -f "$(DEV_DIR)/docker-compose.integration.server.yml"
_INTEGRATION_RUNNER_COMPOSE := -f "$(DEV_DIR)/docker-compose.integration.runner.yml"
_INTEGRATION_STACK_COMPOSE := $(_INTEGRATION_SERVER_COMPOSE) $(_INTEGRATION_RUNNER_COMPOSE)
_INTEGRATION_BINARY_RUNNER_COMPOSE := -f "$(DEV_DIR)/docker-compose.integration.binary-runner.yml"
_INTEGRATION_TELEMETRY_COMPOSE := -f "$(DEV_DIR)/docker-compose.integration.telemetry-host.yml"
_DEV_COMPOSE_ENV := COMPOSE_PROJECT_NAME="$(DEV_COMPOSE_PROJECT)"
_INTEGRATION_COMPOSE_ENV := COMPOSE_PROJECT_NAME="$(INTEGRATION_COMPOSE_PROJECT)"
_INTEGRATION_OTEL_COMPOSE := $(_INTEGRATION_STACK_COMPOSE) $(_INTEGRATION_TELEMETRY_COMPOSE)

# Tee stdout and stderr to both the terminal and a recipe log file while
# preserving the original streams. Requires bash (process substitution).
# Usage: append $(call _tee-log,<name>) to the end of a { ... } group,
# followed by ; wait to flush the tee coprocesses before the shell exits.
# $(1) = log file basename (written to _LOG_DIR/NAME.log)
_tee-log = > >(tee -a "$(_LOG_DIR)/$(1).log") 2> >(tee -a "$(_LOG_DIR)/$(1).log" >&2)

# OTel env vars forwarded to the integration containers.
_K6_OTEL_ENV := K6_OTEL_GRPC_EXPORTER_ENDPOINT=$(K6_OTEL_GRPC_EXPORTER_ENDPOINT) \
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

# ---------------------------------------------------------------------------
# Dev setup
# ---------------------------------------------------------------------------

# Creates a Python 3.13 venv with uv and installs project Python dependencies.
.PHONY: setup-python
setup-python:
	uv venv --python 3.13 "$(VENV)"
	uv sync

# Writes compose_versions, creates GEMINI.md symlink, installs Node and Python deps.
.PHONY: setup-env
setup-env: setup-python
	printf "LGTM_VERSION=$(LGTM_VERSION)\nKEYCLOAK_VERSION=$(KEYCLOAK_VERSION)" > "$(DEV_DIR)/compose_versions"
	ln -sf AGENTS.md GEMINI.md
	npm install

# Installs git pre-commit and commit-msg hooks.
.PHONY: setup-hooks
setup-hooks:
	printf '#!/usr/bin/env sh\nmake pre-commit\n' > .git/hooks/pre-commit
	chmod +x .git/hooks/pre-commit
	printf '#!/usr/bin/env sh\nnpx --no -- commitlint --edit "$$1"\n' > .git/hooks/commit-msg
	chmod +x .git/hooks/commit-msg

# Sets up Grafana LGTM versions, configures a symlink so GEMINI.md can read
# the AGENTS.md file, and installs git hooks for pre-commit and commit-msg.
.PHONY: setup
setup: setup-env setup-hooks

# Starts only the LGTM stack (without the Grafana MCP sidecar).
.PHONY: dev-up-lgtm
dev-up-lgtm: dev-volumes
	@{ $(_DEV_COMPOSE_ENV) docker compose \
		$(_DEV_LGTM_COMPOSE) \
		--env-file="$(DEV_DIR)/compose_versions" \
		up \
		-d \
		--wait ; }

# Tails logs from the LGTM dev stack.
.PHONY: dev-logs-tail
dev-logs-tail:
	$(_DEV_COMPOSE_ENV) docker compose $(_DEV_FULL_COMPOSE) logs -f

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
# Local debug tools
# ---------------------------------------------------------------------------

# Runs a local instance of the mcp inspector for debugging.
.PHONY: dev-mcp-inspector
dev-mcp-inspector: build-mcp-server
	npx @modelcontextprotocol/inspector \
		-e "OTEL_TRACES_EXPORTER=otlp" \
		-e "OTEL_METRICS_EXPORTER=otlp" \
		-e "OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317" \
		-e "OTEL_EXPORTER_OTLP_INSECURE=true" \
		-e "OTEL_METRIC_EXPORT_INTERVAL=5000" \
		-- ./bin/make-mcp \
		--config make-mcp.yml ;

# Connects the MCP inspector to the locally running make-mcp HTTP server.
# Run 'make mcp-server-up' first to ensure the server is running.
.PHONY: dev-mcp-inspector-http
dev-mcp-inspector-http:
	npx @modelcontextprotocol/inspector http://localhost:9378/mcp ;
