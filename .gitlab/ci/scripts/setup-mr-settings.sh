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

# One-off: configure project-level MR settings for deckhouse/virtualization
# via GitLab REST API.
#
# Equivalent of running the script in CI is unnecessary; run this locally
# after creating the project (or once per repo). Wraps idempotent PUT
# requests so it's safe to re-run.
#
# Required environment:
#   GITLAB_API_TOKEN  - Personal Access Token with api scope (NOT a job token;
#                       job tokens cannot modify project settings).
#   CI_PROJECT_ID     - numeric project id (or pass --project-id on CLI).
#
# Optional CLI flags:
#   --project-id <id>     override CI_PROJECT_ID
#   --api-base <url>      default $CI_API_V4_URL or https://fox.flant.com/api/v4
#   --dry-run             print curl commands instead of executing them
#
# TODO_RUNNER_TAG: this script is intended to be run by a human from a
# workstation, not from CI. No runner tag applies.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/api.sh
source "${SCRIPT_DIR}/lib/api.sh"

PROJECT_ID="${CI_PROJECT_ID:-}"
API_BASE="${CI_API_V4_URL:-https://fox.flant.com/api/v4}"
DRY_RUN="false"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --project-id)
      PROJECT_ID="$2"
      shift 2
      ;;
    --api-base)
      API_BASE="$2"
      shift 2
      ;;
    --dry-run)
      DRY_RUN="true"
      shift
      ;;
    -h|--help)
      cat <<EOF
Usage: $0 [--project-id <id>] [--api-base <url>] [--dry-run]

Applies the following project MR settings (idempotent PUT requests):
  - merge_method                = "merge"
  - squash                      = true
  - remove_source_branch        = true
  - only_allow_merge_if_pipeline_succeeds = true
  - only_allow_merge_if_all_discussions_are_resolved = true
  - allow_merge_on_skipped_pipeline = true
  - resolve_outdated_diff_discussions = true
  - printing_merge_request_link_enabled = true
  - merge_requests_template     = ""  (configure later via UI if needed)

Required:
  GITLAB_API_TOKEN (api scope)
EOF
      exit 0
      ;;
    *)
      echo "ERROR: unknown flag '$1'" >&2
      exit 1
      ;;
  esac
done

if [[ -z "$PROJECT_ID" ]]; then
  echo "ERROR: project id not provided (set CI_PROJECT_ID or pass --project-id)." >&2
  exit 1
fi

if [[ -z "${GITLAB_API_TOKEN:-}" ]]; then
  echo "ERROR: GITLAB_API_TOKEN is not set." >&2
  exit 1
fi

# settings PUT body. Most fields accept the new value as-is.
SETTINGS_BODY=$(cat <<'EOF'
{
  "merge_method": "merge",
  "squash_option": "always",
  "remove_source_branch_after_merge": true,
  "only_allow_merge_if_pipeline_succeeds": true,
  "only_allow_merge_if_all_discussions_are_resolved": true,
  "allow_merge_on_skipped_pipeline": true,
  "resolve_outdated_diff_discussions": true,
  "printing_merge_request_link_enabled": true,
  "merge_requests_template": ""
}
EOF
)

SETTINGS_PATH="/projects/${PROJECT_ID}"

run() {
  if [[ "$DRY_RUN" == "true" ]]; then
    echo "DRY-RUN: $@"
  else
    eval "$@"
  fi
}

echo "Applying project MR settings to project_id=${PROJECT_ID}..."

# Push the settings payload via PUT.
run curl --silent --show-error --request PUT \
  --header "PRIVATE-TOKEN: ${GITLAB_API_TOKEN}" \
  --header "Content-Type: application/json" \
  --data "${SETTINGS_BODY}" \
  "${API_BASE}${SETTINGS_PATH}" \
  | jq '{
      id, name, path_with_namespace,
      merge_method, squash_option,
      remove_source_branch_after_merge,
      only_allow_merge_if_pipeline_succeeds,
      only_allow_merge_if_all_discussions_are_resolved,
      allow_merge_on_skipped_pipeline,
      resolve_outdated_diff_discussions,
      printing_merge_request_link_enabled
    }'

# Approvers: leave empty unless team decides on a default approver group.
# Push rules: file_size_limit and other settings are configured via UI
# (or push_rules PUT); intentionally not modified here to avoid surprise.
#   PUT /projects/:id/push_rule
#   body: { "deny_delete_tag": true, "member_check": true, ... }

echo "Done. Verify in the GitLab UI Settings -> Merge requests."
