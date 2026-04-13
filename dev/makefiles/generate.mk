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

.PHONY: generate-wiki-home
# @ name: Generate Wiki Home
# @ description: Generates wiki/Home.md from README.md by rewriting image paths and converting GitHub wiki URLs to wiki-link syntax. Fails if the home page was outdated.
# @ risk: low
# @ read-only: false
# @ destructive: false
# @ idempotent: true
# @ open-world: false
# @ param: none
# @ output: Updated wiki/Home.md or a success message
# @ output-type: text/plain
generate-wiki-home:
	@./scripts/gen-wiki-home.sh

.PHONY: gen-ci-doc
# @ name: Generate CI Documentation
# @ description: Parses .github/workflows/*.yml and .github/rulesets/main.json and generates wiki/GitHub CI.md from wiki/tmpl/GitHub CI.md.tmpl. Fails if the committed file was out of date.
# @ risk: low
# @ read-only: false
# @ destructive: false
# @ idempotent: true
# @ open-world: false
# @ param: none
# @ output: Updated wiki/GitHub CI.md or a success message
# @ output-type: text/plain
gen-ci-doc:
	@go run ./cmd/gen-ci-doc

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
