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

# Check that the current MR title follows the commit header convention
# documented in CONTRIBUTING.md#commit-message:
#
#   <type>(<scope>): <subject>
#
# Only the shape is checked. The scope contents and the subject text are not
# validated, so this job never blocks on a scope that CONTRIBUTING.md has not
# caught up with yet.
#
# Behaviour:
#   - On MR pipelines: read the title, ignore automation MRs, match the regex.
#   - On other pipelines: no-op (print "skipping").
#   - Skip-label respected (see rules in job yml).
#
# Required environment:
#   GITLAB_API_TOKEN, CI_API_V4_URL, CI_PROJECT_ID, CI_MERGE_REQUEST_IID
#
# Set TITLE to check an arbitrary string instead of a real merge request, which
# is how the convention can be tried out locally:
#
#   TITLE='feat(vm): add live migration' .gitlab/ci/scripts/bash/check-mr-title.sh

set -euo pipefail

ALLOWED_TYPES='feat|fix|refactor|docs|test|chore'

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=.gitlab/ci/scripts/bash/lib/api.sh
source "${SCRIPT_DIR}/lib/api.sh"

read_title() {
  if [[ -n "${TITLE:-}" ]]; then
    printf '%s' "$TITLE"
    return 0
  fi

  # Read the live title over the API rather than trusting
  # $CI_MERGE_REQUEST_TITLE: GitLab starts no new pipeline when only the title
  # changes, so the variable baked into this pipeline goes stale as soon as the
  # author fixes the title. Going to the API makes a plain job retry enough.
  local mr_json=""
  if [[ -n "${GITLAB_API_TOKEN:-}" && -n "${CI_MERGE_REQUEST_IID:-}" ]]; then
    mr_json="$(api GET "/projects/${CI_PROJECT_ID}/merge_requests/${CI_MERGE_REQUEST_IID}")" || mr_json=""
  fi

  local title=""
  if [[ -n "$mr_json" ]]; then
    title="$(printf '%s' "$mr_json" | jq -r '.title // empty')"
  fi

  printf '%s' "${title:-${CI_MERGE_REQUEST_TITLE:-}}"
}

# Automation opens merge requests whose titles are fixed strings of its own and
# are not commit headers: tools/changelog ("Changelog v1.11.0"), backport.sh
# ("Backport !123 to release-1.11") and the GitLab revert button ('Revert "..."').
is_automation_title() {
  case "$1" in
    "Changelog v"* | "Backport !"* | 'Revert "'*) return 0 ;;
    *) return 1 ;;
  esac
}

check_title() {
  local title="$1"

  if [[ -z "$title" ]]; then
    echo "ERROR: cannot read the merge request title." >&2
    return 1
  fi

  if is_automation_title "$title"; then
    echo "OK: '${title}' is opened by automation, not checked."
    return 0
  fi

  # GitLab prefixes work-in-progress merge requests itself; check what is under it.
  title="$(printf '%s' "$title" | sed -E 's/^([[:space:]]*(Draft|WIP):[[:space:]]*)+//I')"

  local re="^(${ALLOWED_TYPES})(\([a-z0-9, -]+\))?: .+\$"
  if [[ ! "$title" =~ $re ]]; then
    cat >&2 <<EOF
ERROR: the merge request title must be '<type>(<scope>): <subject>'.
  got:      ${title}
  type:     ${ALLOWED_TYPES//|/ | }
  scope:    optional, lower-case; separate several scopes with a comma
  examples: feat(vm): add live migration capability
            fix(disks, vd): fix PVC size calculation
            chore: update go dependencies
  See CONTRIBUTING.md#commit-message for the full convention.
  Fix the title and retry this job -- no new push is needed.
EOF
    return 1
  fi

  echo "OK: '${title}'"
}

if [[ -z "${TITLE:-}" && "${CI_PIPELINE_SOURCE:-}" != "merge_request_event" ]]; then
  echo "Not a merge request pipeline (CI_PIPELINE_SOURCE=${CI_PIPELINE_SOURCE:-}). Skipping."
  exit 0
fi

check_title "$(read_title)"
