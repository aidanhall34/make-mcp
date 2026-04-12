#!/usr/bin/env bash
# Uploads the GitHub App credentials needed by the release workflow.
#
# The app must already be installed on this repository AND added to the
# ruleset bypass list via the GitHub UI before the release workflow will
# be able to push directly to main:
#
#   Settings → Rules → main → Bypass list → Add bypass → GitHub Apps
#
# Usage:
#   .github/scripts/setup-release-app.sh <APP_ID> <PRIVATE_KEY_PATH>
#
# Prerequisites:
#   - gh CLI authenticated as a repo admin

set -euo pipefail

REPO="aidanhall34/make-mcp"

APP_ID="${1:?Usage: $0 <APP_ID> <PRIVATE_KEY_PATH>}"
PRIVATE_KEY_PATH="${2:?Usage: $0 <APP_ID> <PRIVATE_KEY_PATH>}"

[[ -f "${PRIVATE_KEY_PATH}" ]] || { echo "error: key file not found: ${PRIVATE_KEY_PATH}"; exit 1; }
command -v gh >/dev/null 2>&1 || { echo "error: gh CLI is required"; exit 1; }

echo "→ Setting APP_ID secret..."
gh secret set APP_ID --repo "${REPO}" --body "${APP_ID}"

echo "→ Setting APP_PRIVATE_KEY secret..."
gh secret set APP_PRIVATE_KEY --repo "${REPO}" < "${PRIVATE_KEY_PATH}"

echo ""
echo "✓ Done. Secrets APP_ID and APP_PRIVATE_KEY set on ${REPO}."
echo ""
echo "Next: add the app to the ruleset bypass list if you haven't already:"
echo "  https://github.com/${REPO}/settings/rules"
echo "  main → Bypass list → Add bypass → GitHub Apps → make-mcp-release → Always"
