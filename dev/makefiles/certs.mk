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
