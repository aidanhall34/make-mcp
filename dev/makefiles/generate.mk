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

SHELL=/usr/bin/env bash

CERTS_DIR ?= $(DEV_DIR)/certs
CERT_DAYS ?= 3650

.PHONY: gen-certs
# @ name: Generate development TLS certificates
# @ description: Generates a self-signed CA and a server certificate (with localhost SAN) for local OAuth/TLS development. Writes ca.crt, server.crt, and server.key to $(CERTS_DIR). Safe to re-run; existing certificates are overwritten.
# @ risk: low
# @ read-only: false
# @ destructive: false
# @ idempotent: true
# @ open-world: false
# @ param: none
# @ output: Paths of the generated certificate files
# @ output-type: text/plain
gen-certs:
	mkdir -p "$(CERTS_DIR)"
	openssl req -x509 -newkey rsa:4096 -days "$(CERT_DAYS)" -nodes \
		-keyout "$(CERTS_DIR)/ca.key" -out "$(CERTS_DIR)/ca.crt" \
		-subj "/CN=make-mcp-dev-ca"
	openssl req -newkey rsa:4096 -nodes \
		-keyout "$(CERTS_DIR)/server.key" -out "$(CERTS_DIR)/server.csr" \
		-subj "/CN=localhost"
	printf '[SAN]\nsubjectAltName=DNS:localhost,IP:127.0.0.1,DNS:keycloak\n' \
		> "$(CERTS_DIR)/san.ext"
	openssl x509 -req -days "$(CERT_DAYS)" \
		-in "$(CERTS_DIR)/server.csr" \
		-CA "$(CERTS_DIR)/ca.crt" -CAkey "$(CERTS_DIR)/ca.key" -CAcreateserial \
		-out "$(CERTS_DIR)/server.crt" \
		-extfile "$(CERTS_DIR)/san.ext" -extensions SAN
	@printf 'Certificates written to %s/\n' "$(CERTS_DIR)"
	@printf '  CA:     %s/ca.crt\n' "$(CERTS_DIR)"
	@printf '  Cert:   %s/server.crt\n' "$(CERTS_DIR)"
	@printf '  Key:    %s/server.key\n' "$(CERTS_DIR)"
