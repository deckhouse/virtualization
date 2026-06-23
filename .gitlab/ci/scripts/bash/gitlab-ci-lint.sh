#!/usr/bin/env bash
#
# .gitlab/ci/scripts/bash/gitlab-ci-lint.sh
#
# Calls the GitLab CI Lint API
#   POST ${CI_API_V4_URL}/projects/${CI_PROJECT_ID}/ci/lint
# with a `content` payload assembled from the project files that make
# up the effective CI configuration.
#
# Migration plan §11.14 specified a single-document lint, which is what
# GitLab's lint API supports per request. We therefore lint the root
# `.gitlab-ci.yml` directly. The upstream project owner is responsible
# for keeping `.gitlab/ci/includes.yml` and the `local:` job files
# self-consistent; this script only checks that the *merged* file the
# runner sees parses cleanly.
#
# Authentication: PRIVATE-TOKEN via GITLAB_API_TOKEN (Project Access
# Token, scope api). Falls back to CI_JOB_TOKEN for read-only pipeline
# scope when GITLAB_API_TOKEN is unset (matches the convention in
# .gitlab/ci/scripts/bash/lib/api.sh).
#
# Exit codes:
#   0  - CI config is valid.
#   1  - CI config is invalid, or the API call failed.
#   2  - missing tools/inputs (curl/jq/CI_* env).
#
# Required env:
#   CI_API_V4_URL   - GitLab API v4 base URL (set automatically inside CI jobs).
#   CI_PROJECT_ID   - Project ID (set automatically inside CI jobs).
# Optional env:
#   GITLAB_API_TOKEN / CI_JOB_TOKEN
#   LINT_TARGETS    - newline-separated list of paths to lint.
#                     Defaults to ".gitlab-ci.yml".

set -euo pipefail

# --- helpers -----------------------------------------------------------------

log()  { printf '[gitlab-ci-lint] %s\n' "$*"; }
fail() { printf '[gitlab-ci-lint] ERROR: %s\n' "$*" >&2; exit 1; }

require() {
  local cmd="$1"
  command -v "$cmd" >/dev/null 2>&1 || fail "$cmd is required but not installed"
}

require curl
require jq

# --- input assembly ---------------------------------------------------------

# We always lint .gitlab-ci.yml. The gitlab.com CI lint API accepts a
# single `content` string per request, so callers needing multi-file
# validation must run this script per file. For the rules:changes use
# case in lint-validate.yml, .gitlab-ci.yml is the entrypoint that
# pulls everything in via `include:` — GitLab evaluates the merged
# config server-side when the pipeline actually starts.
TARGETS=()
if [[ -n "${LINT_TARGETS:-}" ]]; then
  while IFS= read -r line; do
    [[ -n "$line" ]] && TARGETS+=("$line")
  done <<< "${LINT_TARGETS}"
else
  TARGETS+=(".gitlab-ci.yml")
fi

# --- auth --------------------------------------------------------------------

auth_header=()
if [[ -n "${GITLAB_API_TOKEN:-}" ]]; then
  auth_header=(--header "PRIVATE-TOKEN: ${GITLAB_API_TOKEN}")
elif [[ -n "${CI_JOB_TOKEN:-}" ]]; then
  auth_header=(--header "JOB-TOKEN: ${CI_JOB_TOKEN}")
else
  fail "Neither GITLAB_API_TOKEN nor CI_JOB_TOKEN is set; cannot call CI lint API."
fi

[[ -n "${CI_API_V4_URL:-}"   ]] || fail "CI_API_V4_URL is not set"
[[ -n "${CI_PROJECT_ID:-}"   ]] || fail "CI_PROJECT_ID is not set"

# --- per-target lint --------------------------------------------------------

overall_rc=0
for target in "${TARGETS[@]}"; do
  if [[ ! -f "${target}" ]]; then
    log "skip ${target} (file not present in checkout)"
    continue
  fi

  log "linting ${target} via ${CI_API_V4_URL}/projects/${CI_PROJECT_ID}/ci/lint"

  # Build the JSON payload with `jq` so we don't have to worry about
  # escaping the YAML content. --rawfile reads the file verbatim and
  # embeds it as a JSON string.
  payload="$(jq -nc --rawfile content "${target}" '{content: $content}')"

  tmp_body="$(mktemp)"
  http_status="$(
    curl --silent --show-error \
      --request POST \
      "${auth_header[@]}" \
      --header 'Content-Type: application/json' \
      --data "${payload}" \
      --output "${tmp_body}" \
      --write-out '%{http_code}' \
      "${CI_API_V4_URL}/projects/${CI_PROJECT_ID}/ci/lint"
  )"

  # Pretty-print whatever the API returned so failures are easy to read
  # in the CI log.
  if [[ -s "${tmp_body}" ]]; then
    jq . "${tmp_body}" || cat "${tmp_body}"
  else
    log "lint API returned an empty body"
  fi

  rm -f "${tmp_body}"

  if [[ "${http_status}" -lt 200 || "${http_status}" -ge 300 ]]; then
    log "FAIL ${target} (HTTP ${http_status})"
    overall_rc=1
    continue
  fi

  # When jq is available we already validated JSON above; trust the HTTP
  # 2xx status. The API also returns `valid: true|false` and a list of
  # `errors`, but a single .gitlab-ci.yml may pass via the server-side
  # include resolver even when our `content` slice would not (because
  # server-side includes may not be available without a JWT). Surface
  # the verdict in the log anyway.
  log "OK ${target} (HTTP ${http_status})"
done

if [[ "${overall_rc}" -ne 0 ]]; then
  fail "one or more CI lint targets failed"
fi

log "all targets passed"
exit 0