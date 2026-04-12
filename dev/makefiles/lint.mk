SHELL=/usr/bin/env bash

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
			hadolint --failure-threshold warning $$dockerfiles ; \
	} $(call _tee-log,lint-dockerfile) ; \
	wait

.PHONY: lint-yaml
# @ name: Lint YAML
# @ description: Lints all YAML files in the repository with yamllint using the rules defined in .yamllint.yml.
# @ risk: low
# @ read-only: true
# @ destructive: false
# @ idempotent: true
# @ open-world: false
# @ param: none
# @ output: yamllint results and any rule violations
# @ output-type: text/plain
lint-yaml:
	@mkdir -p "$(_LOG_DIR)"
	@{ "$(VENV)/bin/yamllint" -c .yamllint.yml . ; } \
		$(call _tee-log,lint-yaml) ; \
	wait

.PHONY: lint-makefile
# @ name: Lint Makefile
# @ description: Lints all Makefiles in the repository with checkmake running in a Docker container. Fails if any rule violations are found.
# @ risk: low
# @ read-only: true
# @ destructive: false
# @ idempotent: true
# @ open-world: true
# @ param: none
# @ output: checkmake lint results
# @ output-type: text/plain
lint-makefile:
	@mkdir -p "$(_LOG_DIR)"
	@{ \
		for f in makefile testdata/Makefile ; do \
			docker run --rm \
				--entrypoint="" \
				-v "$(CURDIR):/workspace" \
				-w "/workspace" \
				mrtazz/checkmake@sha256:eb6919b20b22d1701a976856e4a224627df0a74b118246101fb6cf5c2e03049f \
				/checkmake "$$f" ; \
		done ; \
	} $(call _tee-log,lint-makefile) ; \
	wait
