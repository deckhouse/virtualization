#!/usr/bin/env bash
# Copyright 2026 Flant JSC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# shellcheck disable=SC2154 # CI_* and GITLAB_API_TOKEN are injected by the GitLab Runner at job runtime.

# Shared GitLab API helpers for migration-era jobs.
#
# Source from a job's script:
#   source .gitlab/ci/scripts/bash/lib/api.sh
#
# Provides:
#   api METHOD PATH [curl-args...]   -- REST call with PRIVATE-TOKEN, prints body, returns exit code.
#   gl_required_env                  -- fails if required env vars are missing.
#   gl_log_call                      -- echoes request line for log readability.
#
# Conventions (see tmp/ai-summary/gitlab-ci-migration-plan.md §11.1):
#   - Always CI_API_V4_URL (never hardcode the host).
#   - Always GITLAB_API_TOKEN (Project Access Token, scope api).
#   - Always CI_PROJECT_ID (numeric) and CI_MERGE_REQUEST_IID (iid, not id).

# Guard against double-sourcing.
if [[ -n "${__GL_API_SH_SOURCED:-}" ]]; then
  return 0
fi
__GL_API_SH_SOURCED=1

set -euo pipefail

gl_required_env() {
  local missing=()
  local v
  for v in "$@"; do
    if [[ -z "${!v:-}" ]]; then
      missing+=("$v")
    fi
  done
  if [[ "${#missing[@]}" -gt 0 ]]; then
    echo "ERROR: required environment variables are not set: ${missing[*]}" >&2
    exit 1
  fi
}

gl_log_call() {
  local method="$1"
  local path="$2"
  echo ">>> ${method} ${CI_API_V4_URL}${path}" >&2
}

# api METHOD PATH [extra curl args]
#
# Examples:
#   api GET "/projects/${CI_PROJECT_ID}/merge_requests/${CI_MERGE_REQUEST_IID}"
#   api POST "/projects/${CI_PROJECT_ID}/merge_requests/${CI_MERGE_REQUEST_IID}/assignees" \
#           --data '{"user_id":42}'
#
# Behaviour:
#   - Uses PRIVATE-TOKEN with $GITLAB_API_TOKEN.
#   - Sets Content-Type: application/json by default (overridable via extra args).
#   - On non-2xx: prints status and body to stderr, returns non-zero.
api() {
  local method="$1"
  shift
  local path="$1"
  shift

  gl_required_env CI_API_V4_URL GITLAB_API_TOKEN CI_PROJECT_ID
  gl_log_call "$method" "$path"

  local response_file
  response_file="$(mktemp)"
  local http_code
  http_code="$(curl --silent --show-error --output "$response_file" \
    --request "$method" \
    --write-out '%{http_code}' \
    --header "PRIVATE-TOKEN: ${GITLAB_API_TOKEN}" \
    --header "Content-Type: application/json" \
    --header "Accept: application/json" \
    "${CI_API_V4_URL}${path}" "$@")"

  if [[ "$http_code" =~ ^2 ]]; then
    cat "$response_file"
    rm -f "$response_file"
    echo "<<< status=${http_code}" >&2
    return 0
  fi

  cat "$response_file" >&2
  rm -f "$response_file"
  echo "<<< status=${http_code} (FAILED)" >&2
  return 1
}
