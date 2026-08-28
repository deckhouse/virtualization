#!/usr/bin/env bash

# Copyright 2026 Flant JSC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# shellcheck disable=SC2154 # CI_* and GITLAB_API_TOKEN are injected by the GitLab Runner at job runtime.

# GitLab equivalent of GitHub's
#   concurrency: {group: "<workflow>-<ref>", cancel-in-progress: true}
#
# GitLab has no concurrency group: its built-in auto-cancel keys on the pipeline
# ref and nothing else. That is fine for MR and branch pipelines, where ref IS
# the group, but it breaks the web dispatch that rebuilds an existing tag:
# build_dev_manual runs on ref=main (see the comment above the job in
# .gitlab/ci/jobs/build-dev.yml for why it cannot be a tag pipeline), so every
# push to main marks it redundant and cancels it mid-build. GitHub is not
# exposed to this because its group starts with the workflow name and
# dev_module_build.yml has no push trigger, so a main push never shares its
# group.
#
# The group implemented here is "<pipeline source>-<BUILD_TAG>": rebuilding the
# same tag cancels the previous run, different tags build side by side, and
# pushes to main are not part of the group at all. The job pairs this with
# interruptible: false, which is what takes it out of the ref group.
#
# Cancelling is best-effort: a pipeline may finish between the listing and the
# cancel call, and losing that race must not fail a build that is otherwise fine.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=.gitlab/ci/scripts/bash/lib/api.sh
source "${SCRIPT_DIR}/lib/api.sh"

gl_required_env CI_PIPELINE_ID CI_PIPELINE_SOURCE BUILD_TAG

echo "Concurrency group: ${CI_PIPELINE_SOURCE}-${BUILD_TAG}"

# Only pipelines older than this one are candidates, so neither order_by/sort
# nor pagination beyond the first page matters here.
older_pipelines="$(api GET "/projects/${CI_PROJECT_ID}/pipelines?source=${CI_PIPELINE_SOURCE}&per_page=100" \
  | jq -r --argjson current "${CI_PIPELINE_ID}" '
      .[]
      | select(.id < $current)
      | select(.status == "created" or .status == "pending" or .status == "running")
      | .id')"

for pipeline_id in ${older_pipelines}; do
  pipeline_build_tag="$(api GET "/projects/${CI_PROJECT_ID}/pipelines/${pipeline_id}/variables" \
    | jq -r '.[] | select(.key == "BUILD_TAG") | .value')"

  if [[ "${pipeline_build_tag}" != "${BUILD_TAG}" ]]; then
    continue
  fi

  echo "Cancelling redundant pipeline ${pipeline_id} (BUILD_TAG=${pipeline_build_tag})"
  api POST "/projects/${CI_PROJECT_ID}/pipelines/${pipeline_id}/cancel" >/dev/null \
    || echo "WARNING: could not cancel pipeline ${pipeline_id}, skipping" >&2
done
