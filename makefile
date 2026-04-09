SHELL=/usr/bin/env sh
LGTM_VERSION:= 0.23.0
DEV_DIR=./dev
# @ name: Test
# @ description: Runs all Go unit tests with race detection enabled.
# @ risk: low
# @ read-only: true
# @ destructive: false
# @ idempotent: true
# @ open-world: false
# @ param: none
# @ output: Test results and pass/fail summary
# @ output-type: text/plain
tests:
	go test -race ./...

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

# @ name: Build
# @ description: Compiles all binaries to ./bin/
# @ risk: low
# @ read-only: false
# @ destructive: false
# @ idempotent: true
# @ open-world: false
# @ param: none
# @ output: Binaries at ./bin/
# @ output-type: application/octet-stream
build: tests build-validator build-mcp-server

# Sets up Grafana LGTM versions and configured a symlink so GEMINI.md can read the AGENTS.md file.
setup:
	printf "LGTM_VERSION=$(LGTM_VERSION)" > ./$(DEV_DIR)/compose_versions
	ln -s AGENTS.md GEMINI.md

# @ name: Build Validator
# @ description: Compiles the makefile validator CLI binary to ./bin/validate.
# @ risk: low
# @ read-only: false
# @ destructive: false
# @ idempotent: true
# @ open-world: false
# @ param: none
# @ output: Binary at ./bin/validate
# @ output-type: application/octet-stream
build-validator:
	go build -o ./bin/validate ./cmd/validate

# @ name: Build MCP Server
# @ description: Compiles the MCP server CLI binary to ./bin/mcp-server.
# @ risk: low
# @ read-only: false
# @ destructive: false
# @ idempotent: true
# @ open-world: false
# @ param: none
# @ output: Binary at ./bin/mcp-server
# @ output-type: application/octet-stream
build-mcp-server:
	go build -o ./bin/mcp-server ./cmd/mcp-server

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
	go run ./cmd/validate --config make-mcp.yml

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
.PHONY: dev-volumes
dev-volumes:
	docker volume create make-mcp-lgtm-prometheus-data
	docker volume create make-mcp-lgtm-loki-data
	docker volume create make-mcp-lgtm-tempo-data

.PHONY: dev-up
# @ name: Start development dependencies
# @ description: Starts development dependencies defined in the dev docker-compose.yml file (idempotent)
# @ risk: low
# @ read-only: false
# @ destructive: false
# @ idempotent: true
# @ open-world: true
# @ param: none
# @ output: none
# @ output-type: text/plain
dev-up: dev-volumes
	@{ docker compose \
		-f $(DEV_DIR)/docker-compose.yml \
		--env-file="$(DEV_DIR)/compose_versions" \
		up \
		-d \
		--wait ; }


.PHONY: dev-down
# @ name: Stop development dependencies
# @ description: Stops development dependencies defined in the dev docker-compose.yml file
# @ risk: low
# @ read-only: false
# @ destructive: true
# @ idempotent: true
# @ open-world: true
# @ param: none
# @ output: The shutdown logs of the running containers in the stack.
# @ output-type: text/plain
dev-down: ## Stop and remove the local LGTM development stack
	@{ docker compose \
		--env-file="$(DEV_DIR)/compose_versions" \
		-f $(DEV_DIR)/docker-compose.yml \
		down ; }

.PHONY: dev-logs
# @ name: Fetch Docker Compose Logs
# @ description: Fetches docker compose service logs for containers managed by the ./dev/docker-compose.yml file
# @ risk: low
# @ read-only: true
# @ destructive: false
# @ idempotent: true
# @ open-world: true
# @ param: none
# @ output: Docker container logs
# @ output-type: text/plain
dev-logs:
	docker compose logs -f $(DEV_DIR)/docker-compose.yml

# Tails logs from the LGTM dev stack
.PHONY: dev-logs-tail
dev-logs-tail:
	docker compose logs -f $(DEV_DIR)/docker-compose.yml

# Runs a local instance of the mcp inspector for debugging
dev-mcp-inspector: build
	npx @modelcontextprotocol/inspector \
		-e "OTEL_TRACES_EXPORTER=otlp" \
		-e "OTEL_METRICS_EXPORTER=otlp" \
		-e "OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317" \
		-e "OTEL_EXPORTER_OTLP_INSECURE=true" \
		-e "OTEL_METRIC_EXPORT_INTERVAL=5000" \
		-- ./bin/mcp-server \
		--config make-mcp.yml ;
