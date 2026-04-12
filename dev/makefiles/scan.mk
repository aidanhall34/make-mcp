SHELL=/usr/bin/env bash

# @ name: Scan Secrets
# @ description: Scans the repository for secrets using TruffleHog in a Docker container.
# @ risk: low
# @ read-only: true
# @ destructive: false
# @ idempotent: true
# @ open-world: true
# @ param: none
# @ output: TruffleHog scan results
# @ output-type: text/plain
scan-secrets:
	@mkdir -p "$(_LOG_DIR)"
	@{ docker run --rm -v "$(CURDIR):/pwd" trufflesecurity/trufflehog:$(TRUFFLEHOG_VERSION) git file:///pwd --only-verified --fail ; } \
		$(call _tee-log,scan-secrets) ; \
	wait

# @ name: Scan Vulnerabilities
# @ description: Scans the repository for vulnerabilities using Trivy in a Docker container.
# @ risk: low
# @ read-only: true
# @ destructive: false
# @ idempotent: true
# @ open-world: true
# @ param: none
# @ output: Trivy vulnerability scan results
# @ output-type: text/plain
scan-vulnerabilities:
	@mkdir -p "$(_LOG_DIR)"
	@{ docker run --rm -v "$(CURDIR):/root" aquasec/trivy:$(TRIVY_VERSION) fs --exit-code 1 --severity HIGH,CRITICAL /root ; } \
		$(call _tee-log,scan-vulnerabilities) ; \
	wait

# @ name: Scan Container
# @ description: Scans the locally built make-mcp container image for vulnerabilities using Trivy in a Docker container. Requires IMAGE_TAG (default: latest).
# @ risk: low
# @ read-only: true
# @ destructive: false
# @ idempotent: true
# @ open-world: true
# @ param: IMAGE_TAG string | Container image tag to scan (default: latest)
# @ output: Trivy container scan results
# @ output-type: text/plain
scan-container:
	@mkdir -p "$(_LOG_DIR)"
	@{ docker run --rm -v /var/run/docker.sock:/var/run/docker.sock aquasec/trivy:$(TRIVY_VERSION) image --exit-code 1 --severity HIGH,CRITICAL "$(IMAGE_NAME):$(IMAGE_TAG)" ; } \
		$(call _tee-log,scan-container) ; \
	wait

# @ name: Scan Test Container
# @ description: Scans the locally built make-mcp test container image for vulnerabilities using Trivy in a Docker container. Requires IMAGE_TAG (default: latest).
# @ risk: low
# @ read-only: true
# @ destructive: false
# @ idempotent: true
# @ open-world: true
# @ param: IMAGE_TAG string | Container image tag to scan (default: latest)
# @ output: Trivy container scan results
# @ output-type: text/plain
scan-test-container:
	@mkdir -p "$(_LOG_DIR)"
	@{ docker run --rm -v /var/run/docker.sock:/var/run/docker.sock aquasec/trivy:$(TRIVY_VERSION) image --exit-code 1 --severity HIGH,CRITICAL "$(TEST_IMAGE_NAME):$(IMAGE_TAG)" ; } \
		$(call _tee-log,scan-test-container) ; \
	wait
