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

# Auto-assign MR author as the MR assignee.
#
# Migration of .github/workflows/dev_auto-pr-author-assign.yml which used the
# third-party toshimaru/auto-author-assign@v2.1.0 action.
#
# Behaviour (per migration plan §0 / §11):
#   - Skip silently if MR already has at least one assignee.
#   - Otherwise assign the MR author (the user who opened the MR).
#   - Token: GITLAB_API_TOKEN (Project Access Token, scope api).
#
# Required environment:
#   GITLAB_API_TOKEN, CI_API_V4_URL, CI_PROJECT_ID, CI_MERGE_REQUEST_IID
#
# Exits non-zero only on unexpected API errors; "already assigned" is a no-op.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/api.sh
source "${SCRIPT_DIR}/lib/api.sh"

gl_required_env CI_API_V4_URL GITLAB_API_TOKEN CI_PROJECT_ID CI_MERGE_REQUEST_IID

MR_PATH="/projects/${CI_PROJECT_ID}/merge_requests/${CI_MERGE_REQUEST_IID}"

echo "Reading MR ${CI_MERGE_REQUEST_IID} to detect author and current assignees..."
mr_json="$(api GET "${MR_PATH}")"

# Author user id (author_id is the numeric ID of the user who created the MR).
author_id="$(printf '%s' "$mr_json" | jq -r '.author.id // empty')"
if [[ -z "$author_id" || "$author_id" == "null" ]]; then
  echo "ERROR: MR has no author_id (response had no .author.id)" >&2
  exit 1
fi
author_name="$(printf '%s' "$mr_json" | jq -r '.author.name // .author.username // "unknown"')"
echo "MR author: ${author_name} (id=${author_id})"

# Count current assignees (assignees[] is an array of user objects).
assignee_count="$(printf '%s' "$mr_json" | jq -r '.assignees | length')"
echo "Current assignee count: ${assignee_count}"

if [[ "${assignee_count}" -gt 0 ]]; then
  echo "MR already has ${assignee_count} assignee(s) — skipping auto-assign per plan §0(4)."
  exit 0
fi

# Assign author by user_id.
echo "Assigning user_id=${author_id} as MR assignee..."
assignee_payload="$(jq -n --argjson uid "$author_id" '{assignee_ids: [$uid]}')"
api PUT "${MR_PATH}" --data "$assignee_payload" >/dev/null

echo "Assigned author (${author_name}) to MR !${CI_MERGE_REQUEST_IID}."
