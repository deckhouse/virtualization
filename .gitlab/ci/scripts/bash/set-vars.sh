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
#   MODULE_EDITION       CE if MR carries label edition/ce, otherwise EE.
#   RELEASE_IN_DEV       true if $CI_COMMIT_BRANCH matches release-X.Y,
#                        false otherwise.
#   DEBUG_COMPONENT      first delve/* label (empty if none).
#
# MODULES_MODULE_TAG is intentionally NOT emitted here. The dev build jobs
# (build_dev / build_dev_tags / build_main) derive their own tag from the
# .dev / .dev_tags / .main templates, and a dotenv variable would override
# those per-template tags (see the `needs:` note in jobs/build-dev.yml).
#
# Required env (provided by the job context):
#   CI_API_V4_URL, CI_PROJECT_ID, CI_PIPELINE_SOURCE, CI_COMMIT_BRANCH,
#   CI_MERGE_REQUEST_IID, CI_MERGE_REQUEST_LABELS, GITLAB_API_TOKEN.
#
# GITLAB_API_TOKEN is only required for the manual PR_NUMBER fallback below;
# normal MR pipelines use $CI_MERGE_REQUEST_LABELS and need no token.
#
# Wired into the `set_vars` job in .gitlab/ci/jobs/info.yml, which runs in
# the info stage on MR, default-branch, and dev-tag pipelines and is
# consumed by the dev build jobs via
# `needs: [{job: set_vars, artifacts: true}]`.

# shellcheck disable=SC2154 # CI_* and GITLAB_API_TOKEN are injected by the GitLab Runner at job runtime.

set -euo pipefail

# Source the api() helper for the GitLab API call below.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=.gitlab/ci/scripts/bash/lib/api.sh
source "${SCRIPT_DIR}/lib/api.sh"

OUTPUT="${CI_PROJECT_DIR:-.}/set_vars.env"

# 1) RELEASE_IN_DEV ----------------------------------------------------------
if [[ "${CI_COMMIT_BRANCH:-}" =~ ^release-[0-9]+\.[0-9]+ ]]; then
  RELEASE_IN_DEV="true"
else
  RELEASE_IN_DEV="false"
fi

# 2) Labels via GitLab API ---------------------------------------------------
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

# 3) MODULE_EDITION ----------------------------------------------------------
if [[ ",${LABELS}," == *,edition/ce,* ]]; then
  MODULE_EDITION="CE"
else
  MODULE_EDITION="EE"
fi

# 4) DEBUG_COMPONENT ---------------------------------------------------------
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

# 5) Persist -----------------------------------------------------------------
cat > "${OUTPUT}" <<EOF
MODULE_EDITION=${MODULE_EDITION}
RELEASE_IN_DEV=${RELEASE_IN_DEV}
DEBUG_COMPONENT=${DEBUG_COMPONENT}
EOF

echo ">>> wrote ${OUTPUT}"
cat "${OUTPUT}"
