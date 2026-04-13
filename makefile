SHELL=/usr/bin/env bash

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
# @ name: All actions
# @ description: Runs all unit tests and builds all packages and containers. RUN ON EVERY CHANGE TO .go AND .py files
# @ risk: low
# @ read-only: true
# @ destructive: false
# @ idempotent: true
# @ open-world: false
# @ param: none
# @ output: Lint and test results, container and binary build logs
# @ output-type: text/plain
all: lint test build build-test-containers

.PHONY: clean

.PHONY: test
# @ name: All tests
# @ description: Runs all Go unit and benchmark tests and Python CI helper tests. Each Go package enforces its own coverage threshold via TestMain and emits per-file JSON coverage to stderr.
# @ risk: low
# @ read-only: true
# @ destructive: false
# @ idempotent: true
# @ open-world: false
# @ param: none
# @ output: Test results, per-file coverage JSON, and pass/fail summary
# @ output-type: text/plain
test: unit-tests pytest

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
lint: lint-dockerfile lint-makefile lint-go lint-markdown lint-tidy lint-yaml validate

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
build: test build-validator build-mcp-server integration-build-k6 build-container build-test-containers

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
pre-commit: generate-wiki-sidebar generate-wiki-home gen-metrics-doc gen-ci-doc lint unit-tests scan-secrets scan-vulnerabilities
