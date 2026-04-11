SHELL=/usr/bin/env bash

# @ name: Upload Branch Protection Rules
# @ description: Applies branch protection rules to GitHub for each JSON file in .github/branch-protection/. Each file must be named after the branch it protects (e.g. main.json). Reads the current repo from the gh CLI.
# @ risk: high
# @ read-only: false
# @ destructive: false
# @ idempotent: true
# @ open-world: true
# @ param: none
# @ output: Confirmation that branch protection was applied for each branch
# @ output-type: text/plain
.PHONY: upload-branch-protection
upload-branch-protection:
	@{ \
		set -e ; \
		repo="$$(gh repo view --json nameWithOwner -q .nameWithOwner)" ; \
		for f in .github/branch-protection/*.json; do \
			branch="$$(basename "$$f" .json)" ; \
			printf 'applying branch protection for %s/%s...\n' "$$repo" "$$branch" ; \
			gh api \
				--method PUT \
				-H "Accept: application/vnd.github+json" \
				"/repos/$$repo/branches/$$branch/protection" \
				--input "$$f" ; \
			printf 'branch protection applied for %s\n' "$$branch" ; \
		done ; \
	}

# @ name: Upload Discord Webhook Secret
# @ description: Stores a Discord webhook URL as the DISCORD_WEBHOOK_URL GitHub Actions secret using the gh CLI, and also writes it to the local ACT_SECRET_FILE (.act.secrets) so act can read it when running workflows locally.
# @ risk: high
# @ read-only: false
# @ destructive: false
# @ idempotent: true
# @ open-world: true
# @ param: WEBHOOK_URL string | Discord webhook URL to store as the DISCORD_WEBHOOK_URL GitHub Actions secret
# @ output: Confirmation that the repository secret has been updated and the local secrets file has been written
# @ output-type: text/plain
.PHONY: upload-discord-webhook
upload-discord-webhook:
	@{ \
		set -e ; \
		webhook_url="$$(printf '%s' "$$WEBHOOK_URL" | tr -d '\r\n')" ; \
		if [ -z "$$webhook_url" ]; then \
			printf '%s\n' "WEBHOOK_URL is required" ; \
			exit 1 ; \
		fi ; \
		gh secret set "DISCORD_WEBHOOK_URL" --body "$$webhook_url" ; \
		printf '%s\n' "DISCORD_WEBHOOK_URL updated" ; \
		secret_file="$(ACT_SECRET_FILE)" ; \
		touch "$$secret_file" ; \
		if grep -q "^DISCORD_WEBHOOK_URL=" "$$secret_file" 2>/dev/null; then \
			sed -i "s|^DISCORD_WEBHOOK_URL=.*|DISCORD_WEBHOOK_URL=$$webhook_url|" "$$secret_file" ; \
		else \
			printf 'DISCORD_WEBHOOK_URL=%s\n' "$$webhook_url" >> "$$secret_file" ; \
		fi ; \
		printf '%s\n' "DISCORD_WEBHOOK_URL written to $$secret_file for act" ; \
	}

# @ name: Run GitHub Actions Locally With ACT Using the Current GH Token
# @ description: Fetches the current gh CLI token and passes it directly to act as GH_TOKEN and GITHUB_TOKEN secrets without writing the token to disk.
# @ risk: low
# @ read-only: false
# @ destructive: false
# @ idempotent: true
# @ open-world: true
# @ param: none
# @ output: act workflow execution logs
# @ output-type: text/plain
.PHONY: act-run
act-run:
	@mkdir -p "$(_LOG_DIR)"
	@{ \
		set -e ; \
		act_args="" ; \
		if [ -n "$(ACT_JOB)" ]; then \
			act_args="$$act_args --job $(ACT_JOB)" ; \
		fi ; \
		if [ -f "$(ACT_SECRET_FILE)" ]; then \
			act_args="$$act_args --secret-file $(ACT_SECRET_FILE)" ; \
		fi ; \
		token="$$(gh auth token)" ; \
		if [ -z "$$token" ]; then \
			printf '%s\n' "gh auth token returned an empty token" ; \
			exit 1 ; \
		fi ; \
		act \
			--json \
			--workflows "$(ACT_WORKFLOW)" \
			--platform "ubuntu-latest=$(ACT_IMAGE)" \
			--container-options "--add-host=host.docker.internal:host-gateway" \
			--env "ACT=true" \
			--env "ACT_OTEL_EXPORTER_OTLP_ENDPOINT=$(ACT_OTEL_ENDPOINT)" \
			--secret "GH_TOKEN=$$token" \
			--secret "GITHUB_TOKEN=$$token" \
			--concurrent-jobs "$(ACT_CONCURRENT_JOBS)" \
			$(ACT_EVENT) \
			$$act_args ; \
	} $(call _tee-log,act-run) ; \
	wait
