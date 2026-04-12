SHELL=/usr/bin/env bash

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
