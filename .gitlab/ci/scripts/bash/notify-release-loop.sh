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

# Post a markdown release-status summary to the Loop chat webhook.
#
# Port of the GitHub Actions job `send-release-results-to-loop`
# (.github/workflows/release_module_release-channels.yml). GitLab does not
# inject needs.*.result into the job environment, so this script queries the
# pipeline jobs API once to collect each edition/check/release job status and
# builds the same status table the GH job produced.
#
# Inputs (GitLab predefined + dotenv artifact):
#   CI_API_V4_URL, CI_PROJECT_ID, CI_PIPELINE_ID, CI_PIPELINE_URL,
#   GITLAB_API_TOKEN, RELEASE_TAG, RELEASE_CHANNEL, EDITION_CE, EDITION_EE,
#   CHECK_ONLY, RELEASE_TO_GITLAB, SEND_RESULTS_TO_LOOP, LOOP_WEBHOOK_URL,
#   release.env (GH_RELEASE_STATUS, GH_RELEASE_URL from prod:create-gitlab-release).

# shellcheck disable=SC2154 # CI_* and GITLAB_API_TOKEN are injected by the GitLab Runner at job runtime.

set -euo pipefail

source .gitlab/ci/scripts/bash/lib/api.sh

gl_required_env CI_API_V4_URL GITLAB_API_TOKEN CI_PROJECT_ID CI_PIPELINE_ID \
  RELEASE_TAG RELEASE_CHANNEL LOOP_WEBHOOK_URL

# Fetch this pipeline's jobs once.
JOBS_JSON=$(api GET "/projects/${CI_PROJECT_ID}/pipelines/${CI_PIPELINE_ID}/jobs?per_page=100")

job_status() {
  # $1 = job name. Echo the GitLab status of the first matching job, or "".
  echo "$JOBS_JSON" | jq -r --arg n "$1" '[.[] | select(.name == $n)][0].status // ""'
}

# Map a GitLab job status to a GitHub-style result word for emoji selection.
map_result() {
  case "$1" in
    success)            echo "success" ;;
    failed)             echo "failure" ;;
    canceled|cancelled) echo "cancelled" ;;
    skipped|manual)     echo "skipped" ;;
    running|pending|created|waiting_for_resource|preparing) echo "running" ;;
    *)                  echo "unknown" ;;
  esac
}

status_emoji() {
  case "$1" in
    success)   echo ":white_check_mark:" ;;
    failure)   echo ":x:" ;;
    cancelled) echo ":warning:" ;;
    skipped)   echo ":fast_forward:" ;;
    running)   echo ":hourglass:" ;;
    *)         echo ":grey_question:" ;;
  esac
}

export TZ="Europe/Moscow"
DATE=$(date +"%Y-%m-%d %H:%M:%S UTC+03:00")
RUN_URL="${CI_PIPELINE_URL}"

CE_RESULT=$(map_result "$(job_status prod:deploy:ce)")
EE_RESULT=$(map_result "$(job_status prod:deploy:ee)")
SE_PLUS_RESULT=$(map_result "$(job_status prod:deploy:se-plus)")
FE_RESULT=$(map_result "$(job_status prod:deploy:fe)")
CHECK_RESULT=$(map_result "$(job_status prod:check-version)")

# Load the release creation dotenv artifact if present.
GH_RELEASE_STATUS=""
# shellcheck disable=SC1091
[ -f release.env ] && . release.env

HEADER_ROW="| Edition |"
STATUS_ROW="| Status |"
if [ "${EDITION_CE:-false}" = "true" ]; then
  HEADER_ROW+=" CE |"
  STATUS_ROW+=" $(status_emoji "${CE_RESULT}") |"
fi
if [ "${EDITION_EE:-false}" = "true" ]; then
  HEADER_ROW+=" EE | SE+ | FE |"
  STATUS_ROW+=" $(status_emoji "${EE_RESULT}") | $(status_emoji "${SE_PLUS_RESULT}") | $(status_emoji "${FE_RESULT}") |"
fi
HEADER_ROW+=" Check |"
STATUS_ROW+=" $(status_emoji "${CHECK_RESULT}") |"
if [ "${RELEASE_TO_GITLAB:-true}" = "true" ] && [ "${CHECK_ONLY:-false}" != "true" ]; then
  HEADER_ROW+=" GitLab Release |"
  case "${GH_RELEASE_STATUS}" in
    created) STATUS_ROW+=" :white_check_mark: |" ;;
    skipped) STATUS_ROW+=" :fast_forward: |" ;;
    *)       STATUS_ROW+=" :x: |" ;;
  esac
fi

# Build the markdown separator row matching the header column count.
COL_COUNT=$(echo "${HEADER_ROW}" | tr -cd '|' | wc -c)
COL_COUNT=$((COL_COUNT - 1))
SEP="|"
i=0
while [ "${i}" -lt "${COL_COUNT}" ]; do
  SEP+="---|"
  i=$((i + 1))
done

SUMMARY="## :dvp: **DVP | Release ${RELEASE_TAG} to ${RELEASE_CHANNEL}**\\n\\n"
SUMMARY+="Date: ${DATE}\\n"
SUMMARY+="[:link: GitLab CI Pipeline](${RUN_URL})\\n\\n"
SUMMARY+="${HEADER_ROW}\\n${SEP}\\n${STATUS_ROW}\\n"

echo -e "${SUMMARY}"

curl --silent --show-error --fail --request POST \
  --header "Content-Type: application/json" \
  --data "{\"text\": \"${SUMMARY}\"}" \
  "${LOOP_WEBHOOK_URL}"
