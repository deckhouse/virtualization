#!/usr/bin/env bash
#
# set-vars.sh — derives per-pipeline variables for downstream jobs.
#
# Carries forward the responsibilities of the GH `set_vars` job from
# dev_module_build.yml (migration plan §11.3.4). Produces a dotenv
# artifact that downstream jobs consume via `needs: [set_vars]` +
# `artifacts.reports.dotenv`.
#
# Outputs (written to set_vars.env in $CI_PROJECT_DIR):
#   MODULES_MODULE_TAG   mrNNN for MR pipelines, main for default branch,
#                        release-X.Y for release branches, mrNNN for manual
#                        PR_NUMBER override, fail otherwise.
#   MODULE_EDITION       CE if MR carries label edition/ce, otherwise EE.
#   RELEASE_IN_DEV       true if $CI_COMMIT_BRANCH matches release-X.Y,
#                        false otherwise.
#   DEBUG_COMPONENT      first delve/* label (empty if none).
#
# Required env (provided by the job context):
#   CI_API_V4_URL, CI_PROJECT_ID, CI_PIPELINE_SOURCE, CI_COMMIT_BRANCH,
#   CI_MERGE_REQUEST_IID, CI_MERGE_REQUEST_LABELS, GITLAB_API_TOKEN.
#
# This script is not yet wired into a job by this issue — the
# info.yml / set-vars integration lands in a follow-up because the previous
# GitLab config did not have an equivalent job. Child issue can call it via:
#   set_vars:
#     stage: info
#     script:
#       - bash .gitlab/ci/scripts/bash/set-vars.sh
#     artifacts:
#       reports:
#         dotenv: set_vars.env

# shellcheck disable=SC2154 # CI_* and GITLAB_API_TOKEN are injected by the GitLab Runner at job runtime.

set -euo pipefail

# Source the api() helper for the GitLab API call below.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=.gitlab/ci/scripts/bash/lib/api.sh
source "${SCRIPT_DIR}/lib/api.sh"

OUTPUT="${CI_PROJECT_DIR:-.}/set_vars.env"

# 1) MODULES_MODULE_TAG ------------------------------------------------------
# Mirrors the GH set_vars job: prefer MR iid for MR pipelines, then main for
# pushes to the default branch, then release-X.Y for release branches, then
# PR_NUMBER for manual triggers, fail otherwise.
if [[ "${CI_PIPELINE_SOURCE:-}" == "merge_request_event" ]]; then
  if [[ -z "${CI_MERGE_REQUEST_IID:-}" ]]; then
    echo "ERROR: merge_request_event pipeline without CI_MERGE_REQUEST_IID" >&2
    exit 1
  fi
  MODULES_MODULE_TAG="mr${CI_MERGE_REQUEST_IID}"
elif [[ "${CI_COMMIT_BRANCH:-}" == "${CI_DEFAULT_BRANCH:-main}" ]]; then
  MODULES_MODULE_TAG="main"
elif [[ "${CI_COMMIT_BRANCH:-}" =~ ^release-([0-9]+\.[0-9]+) ]]; then
  MODULES_MODULE_TAG="${CI_COMMIT_BRANCH}"
elif [[ -n "${PR_NUMBER:-}" ]]; then
  MODULES_MODULE_TAG="mr${PR_NUMBER}"
else
  echo "ERROR: cannot derive MODULES_MODULE_TAG (source=${CI_PIPELINE_SOURCE:-?}, branch=${CI_COMMIT_BRANCH:-empty})" >&2
  exit 1
fi

# 2) RELEASE_IN_DEV ----------------------------------------------------------
if [[ "${CI_COMMIT_BRANCH:-}" =~ ^release-[0-9]+\.[0-9]+ ]]; then
  RELEASE_IN_DEV="true"
else
  RELEASE_IN_DEV="false"
fi

# 3) Labels via GitLab API ---------------------------------------------------
# GitLab exposes $CI_MERGE_REQUEST_LABELS automatically for MR pipelines, but
# we keep the explicit API fetch as a safety net for manual/web pipelines that
# target a specific MR via PR_NUMBER.
LABELS=""
if [[ -n "${CI_MERGE_REQUEST_LABELS:-}" ]]; then
  LABELS="${CI_MERGE_REQUEST_LABELS}"
elif [[ -n "${PR_NUMBER:-}" && -n "${GITLAB_API_TOKEN:-}" ]]; then
  LABELS="$(api GET "/projects/${CI_PROJECT_ID}/merge_requests/${PR_NUMBER}" \
    | jq -r '.labels | join(",")')"
fi

# 4) MODULE_EDITION ----------------------------------------------------------
if [[ ",${LABELS}," == *,edition/ce,* ]]; then
  MODULE_EDITION="CE"
else
  MODULE_EDITION="EE"
fi

# 5) DEBUG_COMPONENT ---------------------------------------------------------
DEBUG_COMPONENT=""
DELVE_COUNT=0
if [[ -n "${LABELS}" ]]; then
  DEBUG_COMPONENT="$(echo "${LABELS}" | tr ',' '\n' | grep '^delve' | head -n1 || true)"
  DELVE_COUNT="$(echo "${LABELS}" | tr ',' '\n' | grep -c '^delve' || true)"
fi
if [[ "${DELVE_COUNT}" -gt 1 ]]; then
  echo "ERROR: multiple delve labels: ${LABELS}" >&2
  exit 1
fi

# 6) Persist -----------------------------------------------------------------
cat > "${OUTPUT}" <<EOF
MODULES_MODULE_TAG=${MODULES_MODULE_TAG}
MODULE_EDITION=${MODULE_EDITION}
RELEASE_IN_DEV=${RELEASE_IN_DEV}
DEBUG_COMPONENT=${DEBUG_COMPONENT}
EOF

echo ">>> wrote ${OUTPUT}"
cat "${OUTPUT}"
