SHELL=/usr/bin/env bash

_PID_DIR             := $(CURDIR)/dev/run
_MCP_PID_FILE        := $(_PID_DIR)/make-mcp.pid
_LGTM_LOGS_PID_FILE  := $(_PID_DIR)/lgtm-logs.pid

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
	mkdir -p "$(_LOG_DIR)" "$(_PID_DIR)"
	chmod 777 "$(_LOG_DIR)" "$(_PID_DIR)"
	docker volume create make-mcp-lgtm-prometheus-data
	docker volume create make-mcp-lgtm-loki-data
	docker volume create make-mcp-lgtm-tempo-data

.PHONY: dev-up
# @ name: Start development dependencies
# @ description: Starts the full local development stack, including LGTM, the Grafana MCP sidecar, and the make-mcp HTTP server (idempotent)
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
	@mkdir -p "$(CURDIR)/dev/run"
	@touch "$(_LOG_DIR)/lgtm.log"
	@chmod 664 "$(_LOG_DIR)/lgtm.log"
	@if [ -f "$(_LGTM_LOGS_PID_FILE)" ]; then \
		kill $$(cat "$(_LGTM_LOGS_PID_FILE)") 2>/dev/null || true ; \
		rm -f "$(_LGTM_LOGS_PID_FILE)" ; \
	fi
	@$(_DEV_COMPOSE_ENV) docker compose \
		$(_DEV_FULL_COMPOSE) \
		--env-file="$(DEV_DIR)/compose_versions" \
		logs -f --no-color >> "$(_LOG_DIR)/lgtm.log" 2>&1 & echo $$! > "$(_LGTM_LOGS_PID_FILE)"
	$(MAKE) mcp-server-up

.PHONY: dev-down
# @ name: Stop development dependencies
# @ description: Stops the make-mcp HTTP server and the full local development stack, including LGTM and the Grafana MCP sidecar
# @ risk: medium
# @ read-only: false
# @ destructive: true
# @ idempotent: true
# @ open-world: true
# @ param: none
# @ output: The shutdown logs of the running containers in the stack.
# @ output-type: text/plain
dev-down: mcp-server-down
	@if [ -f "$(_LGTM_LOGS_PID_FILE)" ]; then \
		kill $$(cat "$(_LGTM_LOGS_PID_FILE)") 2>/dev/null || true ; \
		rm -f "$(_LGTM_LOGS_PID_FILE)" ; \
	fi
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
	$(_DEV_COMPOSE_ENV) docker compose $(_DEV_FULL_COMPOSE) logs --since 5m

.PHONY: mcp-server-up
# @ name: Start make-mcp server
# @ description: Starts the make-mcp HTTP server as a local background process. Stops any existing instance first so re-running this after a new binary is built picks up the latest version. Requires start-stop-daemon (available on Debian/Ubuntu).
# @ risk: low
# @ read-only: false
# @ destructive: false
# @ idempotent: true
# @ open-world: false
# @ param: none
# @ output: none
# @ output-type: text/plain
mcp-server-up:
	@mkdir -p "$(CURDIR)/dev/run" "$(_LOG_DIR)"
	@touch "$(_LOG_DIR)/make-mcp-server.log"
	@chmod 664 "$(_LOG_DIR)/make-mcp-server.log"
	@if [ -f "$(_MCP_PID_FILE)" ]; then \
		start-stop-daemon --stop --pidfile "$(_MCP_PID_FILE)" \
			--retry TERM/5/KILL/2 2>/dev/null || true ; \
		rm -f "$(_MCP_PID_FILE)" ; \
	fi
	@env \
		OTEL_TRACES_EXPORTER="otlp" \
		OTEL_METRICS_EXPORTER="otlp" \
		OTEL_EXPORTER_OTLP_ENDPOINT="http://localhost:4317" \
		OTEL_EXPORTER_OTLP_INSECURE="true" \
		OTEL_SERVICE_NAME="make-mcp-server" \
		OTEL_METRIC_EXPORT_INTERVAL="5000" \
		start-stop-daemon --start --background \
		--pidfile "$(_MCP_PID_FILE)" --make-pidfile \
		--chdir "$(CURDIR)" \
		--exec "$(CURDIR)/bin/make-mcp" \
		--output "$(CURDIR)/dev/logs/make-mcp-server.log" \
		-- --config "$(DEV_DIR)/make-mcp.yml"

.PHONY: mcp-server-down
# @ name: Stop make-mcp server
# @ description: Stops the locally running make-mcp background process.
# @ risk: medium
# @ read-only: false
# @ destructive: true
# @ idempotent: true
# @ open-world: false
# @ param: none
# @ output: none
# @ output-type: text/plain
mcp-server-down:
	@{ \
		if [ -f "$(_MCP_PID_FILE)" ]; then \
			start-stop-daemon --stop --pidfile "$(_MCP_PID_FILE)" \
				--retry TERM/5/KILL/2 2>/dev/null || true ; \
			rm -f "$(_MCP_PID_FILE)" ; \
		fi ; \
	}

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
integration-lgtm: integration-build-k6 build-test-containers dev-volumes
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
integration-otel: integration-build-k6 build-test-containers
	$(call _integration-run,$(_INTEGRATION_OTEL_COMPOSE))
