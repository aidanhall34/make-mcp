SHELL=/usr/bin/env bash

.PHONY: clean
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
	go run ./cmd/validate --config ./dev/make-mcp.yml --strict

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
