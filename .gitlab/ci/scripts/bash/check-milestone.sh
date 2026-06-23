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

# Check that the current MR has a milestone assigned.
#
# Migration of .github/workflows/check-pr-milestone.yml which used
# actions/github-script@v6.4.1 to GET the PR and assert data.milestone.
#
# Behaviour (per plan §0):
#   - On MR pipelines: GET MR via API, ensure milestone is present.
#   - On other pipelines: no-op (print "skipping").
#   - Skip-labels respected (see rules in job yml).
#
# Required environment:
#   GITLAB_API_TOKEN, CI_API_V4_URL, CI_PROJECT_ID, CI_MERGE_REQUEST_IID

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=.gitlab/ci/scripts/bash/lib/api.sh
source "${SCRIPT_DIR}/lib/api.sh"

if [[ "${CI_PIPELINE_SOURCE:-}" != "merge_request_event" ]]; then
  echo "Not a merge request pipeline (CI_PIPELINE_SOURCE=${CI_PIPELINE_SOURCE:-}). Skipping."
  exit 0
fi

gl_required_env CI_API_V4_URL GITLAB_API_TOKEN CI_PROJECT_ID CI_MERGE_REQUEST_IID

MR_PATH="/projects/${CI_PROJECT_ID}/merge_requests/${CI_MERGE_REQUEST_IID}"

echo "Reading MR ${CI_MERGE_REQUEST_IID} to check milestone..."
mr_json="$(api GET "${MR_PATH}")"

milestone_title="$(printf '%s' "$mr_json" | jq -r '.milestone.title // empty')"
milestone_id="$(printf '%s' "$mr_json" | jq -r '.milestone.id // empty')"

if [[ -n "$milestone_title" && "$milestone_title" != "null" ]]; then
  echo "OK: MR has milestone '${milestone_title}' (id=${milestone_id})."
  exit 0
fi

echo "ERROR: MR !${CI_MERGE_REQUEST_IID} has no milestone set. Set a milestone before merge." >&2
exit 1
