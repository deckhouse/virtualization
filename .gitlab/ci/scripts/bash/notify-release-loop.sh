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
#   GH_RELEASE_STATUS (dotenv report of prod:create-gitlab-release, injected as
#   an environment variable by GitLab).

# shellcheck disable=SC2154 # CI_* and GITLAB_API_TOKEN are injected by the GitLab Runner at job runtime.

set -euo pipefail

source .gitlab/ci/scripts/bash/lib/api.sh

gl_required_env CI_API_V4_URL GITLAB_API_TOKEN CI_PROJECT_ID CI_PIPELINE_ID \
  RELEASE_TAG RELEASE_CHANNEL LOOP_WEBHOOK_URL

# Fetch this pipeline's jobs once.
JOBS_JSON=$(api GET "/projects/${CI_PROJECT_ID}/pipelines/${CI_PIPELINE_ID}/jobs?per_page=100")

job_status() {
  # $1 = job name. Echo the GitLab status of the matching job, or "" if the
  # pipeline has no such job.
  #
  # A `parallel: matrix` job is reported by the API as one job per leg, named
  # "<name>: [<value>]" (e.g. "prod:check-version: [registry]"), so a plain
  # equality match never finds it. Match the exact name plus all of its matrix
  # legs and collapse them into a single worst-case status: one failed or
  # canceled leg means the whole thing did not pass.
  echo "$JOBS_JSON" | jq -r --arg n "$1" '
    [.[] | select(.name == $n or (.name | startswith($n + ": ["))) | .status] as $s
    | if   ($s | length) == 0                     then ""
      elif ($s | any(. == "failed"))              then "failed"
      elif ($s | any(. == "canceled" or . == "cancelled")) then "canceled"
      elif ($s | any(. == "running" or . == "pending" or . == "created"
                     or . == "preparing" or . == "waiting_for_resource")) then "running"
      elif ($s | any(. == "success"))             then "success"
      else $s[0] end'
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
RELEASE_RESULT=$(map_result "$(job_status prod:create-gitlab-release)")

# The release job's own API status above is the source of truth for the release
# column. Its release.env dotenv only refines a successful run: "created" for a
# freshly created release vs "skipped" when the release already existed.
#
# GitLab hands that dotenv to this job as environment variables, NOT as a file:
# the runner wipes release.env from the shared build directory before this job
# starts ("Removing release.env" in the job log). So never reset the variable to
# "" here — that discarded the value GitLab had already injected — and only read
# the file when it happens to be there.
GH_RELEASE_STATUS="${GH_RELEASE_STATUS:-}"
if [ -z "${GH_RELEASE_STATUS}" ] && [ -f release.env ]; then
  # shellcheck disable=SC1091 # generated at pipeline runtime by prod:create-gitlab-release.
  . release.env
fi

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
  if [ "${RELEASE_RESULT}" = "success" ] && [ "${GH_RELEASE_STATUS}" = "skipped" ]; then
    STATUS_ROW+=" :fast_forward: |"
  else
    STATUS_ROW+=" $(status_emoji "${RELEASE_RESULT}") |"
  fi
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

SUMMARY="## :dvp: **DVP | Release ${RELEASE_TAG} to ${RELEASE_CHANNEL}**

Date: ${DATE}
[:link: GitLab CI Pipeline](${RUN_URL})

${HEADER_ROW}
${SEP}
${STATUS_ROW}"

echo "${SUMMARY}"

# Build the JSON body with jq so RELEASE_TAG / RELEASE_CHANNEL (manual UI input)
# and the newlines are escaped correctly; raw interpolation would break the
# payload on any ", \ or newline and allow field injection.
curl --silent --show-error --fail --request POST \
  --header "Content-Type: application/json" \
  --data "$(jq -nc --arg t "${SUMMARY}" '{text: $t}')" \
  "${LOOP_WEBHOOK_URL}"
