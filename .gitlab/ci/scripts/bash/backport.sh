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

# Backport a merged MR to a release branch by opening a backport MR.
#
# Migration of .github/workflows/on-pull-request-backport.yml which used
# deckhouse/backport-action@v1.0.0 and direct cherry-pick to release branch.
#
# We DO NOT use the GitLab cherry-pick
# REST endpoint (POST /repository/commits/:sha/cherry_pick) because it
# bypasses code review. Instead we:
#   1. clone the repo (or reuse the runner workspace),
#   2. cherry-pick the merged commit (or head SHA),
#   3. push a backport branch,
#   4. open an MR to the target release branch via push options / API.
#
# Target branch resolution (priority):
#   1. TARGET_BRANCH env var (manual "Run pipeline" override).
#   2. Source MR milestone title: vX.Y.Z or X.Y.Z -> release-X.Y.
#
# After the backport attempt, the source MR receives feedback matching the
# GitHub flow:
#   - always remove `status/backport` (when source MR iid is known);
#   - success: add `status/backport/success` + comment with backport MR link;
#   - failure: add `status/backport/failed` + comment with error/job link.
# Feedback failures are logged but never mask the backport outcome on
# success; on the failure path they are best-effort and the script still
# exits non-zero.
#
# Required environment:
#   GITLAB_API_TOKEN, CI_API_V4_URL, CI_PROJECT_ID, CI_SERVER_HOST,
#   CI_PROJECT_PATH, CI_PROJECT_DIR
#   SOURCE_MR_IID (optional; defaults to CI_MERGE_REQUEST_IID)
#   TARGET_BRANCH (optional; otherwise derived from the source MR milestone)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=.gitlab/ci/scripts/bash/lib/api.sh
source "${SCRIPT_DIR}/lib/api.sh"

gl_required_env CI_API_V4_URL GITLAB_API_TOKEN CI_PROJECT_ID CI_SERVER_HOST CI_PROJECT_PATH CI_PROJECT_DIR

# Status labels (with slashes; used as-is in JSON payloads, comma is the only
# separator GitLab uses for add_labels/remove_labels).
BACKPORT_TRIGGER_LABEL="status/backport"
BACKPORT_SUCCESS_LABEL="status/backport/success"
BACKPORT_FAILED_LABEL="status/backport/failed"

# Resolved later; used by the failure-feedback trap.
SOURCE_MR_IID="${SOURCE_MR_IID:-}"
TARGET_BRANCH="${TARGET_BRANCH:-}"
BACKPORT_MR_IID=""
BACKPORT_MR_URL=""
FAILURE_CONTEXT=""
# Set to "success" only after the backport MR is created cleanly without conflicts.
BACKPORT_RESULT=""

# ---------------------------------------------------------------------------
# Source MR feedback helpers (best-effort: never abort the script).
# ---------------------------------------------------------------------------

# Remove a label from the source MR. GitLab PUT /merge_requests/:iid supports
# remove_labels with comma-separated names; a slash inside a label name is fine
# because the separator is the comma, not the slash.
mr_remove_label() {
  local iid="$1"
  local label="$2"
  local payload
  payload="$(jq -n --arg l "$label" '{remove_labels: $l}')"
  if ! api PUT "/projects/${CI_PROJECT_ID}/merge_requests/${iid}" --data "${payload}" >/dev/null; then
    echo "WARNING: failed to remove label '${label}' from MR !${iid}." >&2
    return 0
  fi
  echo "Removed label '${label}' from MR !${iid}."
}

# Add a label to the source MR.
mr_add_label() {
  local iid="$1"
  local label="$2"
  local payload
  payload="$(jq -n --arg l "$label" '{add_labels: $l}')"
  if ! api PUT "/projects/${CI_PROJECT_ID}/merge_requests/${iid}" --data "${payload}" >/dev/null; then
    echo "WARNING: failed to add label '${label}' to MR !${iid}." >&2
    return 0
  fi
  echo "Added label '${label}' to MR !${iid}."
}

# Create a note (comment) on the source MR.
mr_create_note() {
  local iid="$1"
  local body="$2"
  local payload
  payload="$(jq -n --arg b "$body" '{body: $b}')"
  if ! api POST "/projects/${CI_PROJECT_ID}/merge_requests/${iid}/notes" --data "${payload}" >/dev/null; then
    echo "WARNING: failed to create note on MR !${iid}." >&2
    return 0
  fi
  echo "Created note on MR !${iid}."
}

# Report failure feedback to the source MR (best-effort).
report_failure() {
  if [[ -z "${SOURCE_MR_IID}" ]]; then
    echo "No source MR iid known; cannot report failure feedback." >&2
    return 0
  fi
  local job_url="${CI_JOB_URL:-${CI_PIPELINE_URL:-}}"
  local body="Backport to \`${TARGET_BRANCH:-unknown}\` failed."
  if [[ -n "$job_url" ]]; then
    body="${body} See job: ${job_url}"
  fi
  if [[ -n "$FAILURE_CONTEXT" ]]; then
    body="${body}

${FAILURE_CONTEXT}"
  fi
  if [[ -n "$BACKPORT_MR_URL" ]]; then
    body="${body}

A backport MR was created for manual resolution: ${BACKPORT_MR_URL}"
  fi
  mr_remove_label "${SOURCE_MR_IID}" "${BACKPORT_TRIGGER_LABEL}"
  mr_add_label "${SOURCE_MR_IID}" "${BACKPORT_FAILED_LABEL}"
  mr_create_note "${SOURCE_MR_IID}" "${body}"
}

# Report success feedback to the source MR (best-effort).
report_success() {
  if [[ -z "${SOURCE_MR_IID}" ]]; then
    echo "No source MR iid known; cannot report success feedback." >&2
    return 0
  fi
  local body="Backport to \`${TARGET_BRANCH}\` successful: ${BACKPORT_MR_URL:-!(unknown)}"
  mr_remove_label "${SOURCE_MR_IID}" "${BACKPORT_TRIGGER_LABEL}"
  mr_add_label "${SOURCE_MR_IID}" "${BACKPORT_SUCCESS_LABEL}"
  mr_create_note "${SOURCE_MR_IID}" "${body}"
}

# EXIT trap: route to success/failure feedback. Preserves the exit code on
# the failure path; feedback errors never override the backport outcome.
on_exit() {
  local rc=$?
  # Clean exit (success) — nothing more to do.
  if [[ $rc -eq 0 || "${BACKPORT_RESULT}" == "success" ]]; then
    exit 0
  fi
  set +e
  echo "Backport failed (exit code ${rc}); reporting failure feedback to source MR." >&2
  report_failure
  exit "$rc"
}
trap on_exit EXIT

# ---------------------------------------------------------------------------
# Resolve the source MR iid.
# ---------------------------------------------------------------------------
SOURCE_MR_IID="${SOURCE_MR_IID:-${CI_MERGE_REQUEST_IID:-}}"
if [[ -z "$SOURCE_MR_IID" ]]; then
  echo "ERROR: SOURCE_MR_IID is required (CI_MERGE_REQUEST_IID unset and no explicit var)." >&2
  exit 1
fi
echo "Source MR: !${SOURCE_MR_IID}"

# ---------------------------------------------------------------------------
# Read source MR (merged commit SHA + milestone).
# ---------------------------------------------------------------------------
mr_path="/projects/${CI_PROJECT_ID}/merge_requests/${SOURCE_MR_IID}"
mr_json="$(api GET "${mr_path}")"

sha="$(printf '%s' "$mr_json" | jq -r '.merge_commit_sha // .sha // empty')"
if [[ -z "$sha" || "$sha" == "null" ]]; then
  echo "ERROR: could not extract SHA from MR !${SOURCE_MR_IID} (not merged yet?)." >&2
  FAILURE_CONTEXT="could not extract merged commit SHA from MR !${SOURCE_MR_IID}"
  exit 1
fi
echo "Source commit SHA: ${sha}"

milestone_title="$(printf '%s' "$mr_json" | jq -r '.milestone.title // empty')"

# ---------------------------------------------------------------------------
# Resolve TARGET_BRANCH (priority: explicit var > milestone-based inference).
# ---------------------------------------------------------------------------
TARGET_BRANCH="${TARGET_BRANCH:-}"
if [[ -z "$TARGET_BRANCH" ]]; then
  if [[ -z "$milestone_title" || "$milestone_title" == "null" ]]; then
    echo "ERROR: source MR !${SOURCE_MR_IID} has no milestone; cannot derive target branch." >&2
    echo "       Set TARGET_BRANCH or assign a milestone to the source MR." >&2
    FAILURE_CONTEXT="source MR !${SOURCE_MR_IID} has no milestone"
    exit 1
  fi
  # Match vX.Y.Z or X.Y.Z and keep the X.Y minor (mirrors the GitHub workflow).
  if [[ "$milestone_title" =~ v?([0-9]+\.[0-9]+)\.[0-9]+ ]]; then
    TARGET_BRANCH="release-${BASH_REMATCH[1]}"
  else
    echo "ERROR: milestone '${milestone_title}' does not match v?X.Y.Z format." >&2
    FAILURE_CONTEXT="invalid milestone format: ${milestone_title}"
    exit 1
  fi
fi

if ! [[ "$TARGET_BRANCH" =~ ^release-[0-9]+\.[0-9]+$ ]]; then
  echo "ERROR: TARGET_BRANCH='${TARGET_BRANCH}' does not match ^release-[0-9]+\\.[0-9]+\$" >&2
  FAILURE_CONTEXT="invalid TARGET_BRANCH format: ${TARGET_BRANCH}"
  exit 1
fi

echo "Backport target: ${TARGET_BRANCH}"
if [[ -n "$milestone_title" && "$milestone_title" != "null" ]]; then
  echo "Source MR milestone: ${milestone_title}"
fi

# ---------------------------------------------------------------------------
# Verify target branch exists (clearer error than a git fetch failure).
# ---------------------------------------------------------------------------
if ! api GET "/projects/${CI_PROJECT_ID}/repository/branches/${TARGET_BRANCH}" >/dev/null; then
  echo "ERROR: target branch '${TARGET_BRANCH}' does not exist in this project." >&2
  FAILURE_CONTEXT="target branch '${TARGET_BRANCH}' does not exist"
  exit 1
fi

# ---------------------------------------------------------------------------
# Cherry-pick the merged commit onto the target branch.
# ---------------------------------------------------------------------------
cd "${CI_PROJECT_DIR}"

git config user.email "ci-backport@flant.com"
git config user.name  "GitLab CI Backport Bot"

REPO_URL="https://oauth2:${GITLAB_API_TOKEN}@${CI_SERVER_HOST}/${CI_PROJECT_PATH}.git"
git remote set-url origin "${REPO_URL}"

# Fetch the target branch.
git fetch --no-tags --depth=200 origin "${TARGET_BRANCH}"

BACKPORT_BRANCH="backport/${SOURCE_MR_IID}/${TARGET_BRANCH}"
echo "Creating backport branch: ${BACKPORT_BRANCH}"
git checkout -B "${BACKPORT_BRANCH}" "origin/${TARGET_BRANCH}"

CONFLICT_MARKER=""
CONFLICTED=0
# GIT_SEQUENCE_EDITOR=true drops the default commit message editor so the
# script can run unattended.
GIT_SEQUENCE_EDITOR=true git cherry-pick -x "${sha}" || {
  CONFLICTED=1
  CONFLICT_MARKER="
**Conflicts detected** — please resolve manually, amend the commit, and force-push.
"
  echo "Cherry-pick reported conflicts; staging resolved tree as best-effort commit."
  # Best-effort: keep partial progress so reviewer can see what was applied.
  git add -A || true
  if ! git diff --cached --quiet; then
    git -c core.editor=true commit --no-edit || true
  fi
}

# ---------------------------------------------------------------------------
# Push the backport branch and ensure a backport MR exists.
# ---------------------------------------------------------------------------
DESCRIPTION="Backport !${SOURCE_MR_IID} (${sha}) to ${TARGET_BRANCH}.

Auto-generated by GitLab CI backport job.
${CONFLICT_MARKER}"

# Push with merge_request.* push options to create the MR in one step.
push_output="$(git push --force-with-lease \
  -o merge_request.create \
  -o merge_request.target="${TARGET_BRANCH}" \
  -o merge_request.source="${BACKPORT_BRANCH}" \
  -o merge_request.title="Backport !${SOURCE_MR_IID} to ${TARGET_BRANCH}" \
  -o merge_request.description="${DESCRIPTION}" \
  -o merge_request.label="backport" \
  -o merge_request.label="auto" \
  -o merge_request.remove_source_branch \
  origin "${BACKPORT_BRANCH}" 2>&1 || true)"
echo "${push_output}"

# Fallback (push options are ignored on some GitLab versions): search for an
# open MR from our branch; if absent, create it via API.
backport_mr_json=""
existing_mr_iid="$(api GET "/projects/${CI_PROJECT_ID}/merge_requests?source_branch=${BACKPORT_BRANCH}&state=opened" \
  | jq -r 'if type == "array" and length > 0 then .[0].iid else empty end')"

if [[ -n "$existing_mr_iid" ]]; then
  echo "Backport MR already exists: !${existing_mr_iid}"
  backport_mr_json="$(api GET "/projects/${CI_PROJECT_ID}/merge_requests/${existing_mr_iid}")"
else
  echo "Push options did not create MR; creating via API..."
  payload="$(jq -n \
    --arg src "${BACKPORT_BRANCH}" \
    --arg tgt "${TARGET_BRANCH}" \
    --arg title "Backport !${SOURCE_MR_IID} to ${TARGET_BRANCH}" \
    --arg desc "${DESCRIPTION}" \
    '{source_branch: $src, target_branch: $tgt, title: $title, description: $desc, remove_source_branch: true, labels: "backport,auto"}')"
  backport_mr_json="$(api POST "/projects/${CI_PROJECT_ID}/merge_requests" --data "${payload}")"
fi

BACKPORT_MR_IID="$(printf '%s' "$backport_mr_json" | jq -r '.iid // empty')"
BACKPORT_MR_URL="$(printf '%s' "$backport_mr_json" | jq -r '.web_url // empty')"
if [[ -z "$BACKPORT_MR_IID" || "$BACKPORT_MR_IID" == "null" ]]; then
  echo "ERROR: could not determine backport MR iid." >&2
  FAILURE_CONTEXT="backport MR creation did not return an iid"
  exit 1
fi
echo "Backport MR: !${BACKPORT_MR_IID} (${BACKPORT_MR_URL})"

# ---------------------------------------------------------------------------
# Report outcome.
# ---------------------------------------------------------------------------
if [[ "$CONFLICTED" == "1" ]]; then
  # A backport MR exists for manual resolution, but the cherry-pick had
  # conflicts, so this is treated as a failure (matches the GitHub flow).
  FAILURE_CONTEXT="cherry-pick had conflicts; resolve them in the backport MR."
  exit 1
fi

BACKPORT_RESULT="success"
# Feedback failures must not mask the successful backport.
set +e
report_success
set -e
echo "Backport completed successfully."
