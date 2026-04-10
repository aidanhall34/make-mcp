#!/usr/bin/env bash

set -euo pipefail

if [ -z "${DISCORD_WEBHOOK_URL:-}" ]; then
  echo "DISCORD_WEBHOOK_URL is not configured."
  exit 1
fi

if [ -z "${JOB_NAME:-}" ]; then
  echo "JOB_NAME is required."
  exit 1
fi

if [ -z "${STARTED_AT:-}" ]; then
  echo "STARTED_AT is required."
  exit 1
fi

echo "[notify-discord] job=${JOB_NAME} status=${JOB_STATUS:-unknown}"

ended_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
started_epoch="$(date -d "${STARTED_AT}" +%s)"
ended_epoch="$(date -d "${ended_at}" +%s)"
duration_seconds="$((ended_epoch - started_epoch))"
status="${JOB_STATUS:-unknown}"
repo_url="${GITHUB_SERVER_URL}/${GITHUB_REPOSITORY}"
run_url="${repo_url}/actions/runs/${GITHUB_RUN_ID}"
commit_url="${repo_url}/commit/${GITHUB_SHA}"
pr_number=""
pr_url=""

echo "[notify-discord] repo=${GITHUB_REPOSITORY} sha=${GITHUB_SHA} run_id=${GITHUB_RUN_ID}"

if [ -f "${GITHUB_EVENT_PATH:-}" ]; then
  echo "[notify-discord] reading PR info from event file: ${GITHUB_EVENT_PATH}"
  pr_number="$(jq -r '.pull_request.number // empty' "${GITHUB_EVENT_PATH}")"
  pr_url="$(jq -r '.pull_request.html_url // empty' "${GITHUB_EVENT_PATH}")"
  echo "[notify-discord] event file pr_number=${pr_number:-<empty>}"
else
  echo "[notify-discord] no event file at GITHUB_EVENT_PATH=${GITHUB_EVENT_PATH:-<unset>}, skipping"
fi

if [ -z "${pr_number}" ] && [ -n "${GH_TOKEN:-}" ]; then
  echo "[notify-discord] fetching PR via GitHub API for commit ${GITHUB_SHA}"
  pr_json="$(gh api -H "Accept: application/vnd.github+json" "/repos/${GITHUB_REPOSITORY}/commits/${GITHUB_SHA}/pulls" 2>/dev/null || true)"
  if [ -n "${pr_json}" ] && [ "${pr_json}" != "[]" ]; then
    pr_number="$(printf '%s' "${pr_json}" | jq -r '.[0].number // empty')"
    pr_url="$(printf '%s' "${pr_json}" | jq -r '.[0].html_url // empty')"
    echo "[notify-discord] API pr_number=${pr_number:-<empty>}"
  else
    echo "[notify-discord] API returned no associated PRs"
  fi
elif [ -z "${GH_TOKEN:-}" ]; then
  echo "[notify-discord] GH_TOKEN not set, skipping API PR lookup"
fi

if [ "${status}" = "success" ]; then
  color=3066993
elif [ "${status}" = "failure" ]; then
  color=15158332
else
  color=16776960
fi

echo "[notify-discord] building payload"
payload="$(jq -n \
  --arg job_name "${JOB_NAME}" \
  --arg workflow_name "${GITHUB_WORKFLOW}" \
  --arg status "${status}" \
  --arg started_at "${STARTED_AT}" \
  --arg ended_at "${ended_at}" \
  --arg duration_seconds "${duration_seconds}" \
  --arg repository "${GITHUB_REPOSITORY}" \
  --arg repo_url "${repo_url}" \
  --arg commit_sha "${GITHUB_SHA}" \
  --arg commit_url "${commit_url}" \
  --arg pr_number "${pr_number}" \
  --arg pr_url "${pr_url}" \
  --arg run_url "${run_url}" \
  --argjson color "${color}" \
  '{
    embeds: [
      {
        title: "\($workflow_name) / \($job_name)",
        url: $run_url,
        color: $color,
        fields: [
          {name: "Status", value: $status, inline: true},
          {name: "Started", value: $started_at, inline: true},
          {name: "Ended", value: $ended_at, inline: true},
          {name: "Duration (seconds)", value: $duration_seconds, inline: true},
          {name: "Repository", value: "[\($repository)](\($repo_url))", inline: false},
          {name: "Commit", value: "[\($commit_sha)](\($commit_url))", inline: false},
          {name: "Pull Request", value: (if $pr_number == "" then "n/a" else "[#\($pr_number)](\($pr_url))" end), inline: true}
        ]
      }
    ]
  }')"

echo "[notify-discord] sending to Discord (webhook URL length=${#DISCORD_WEBHOOK_URL})"
curl --fail --silent --show-error \
  -H "Content-Type: application/json" \
  -d "${payload}" \
  "${DISCORD_WEBHOOK_URL}"
echo "[notify-discord] done"
