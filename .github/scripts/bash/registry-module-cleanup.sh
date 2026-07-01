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

set -Eeuo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=.github/scripts/bash/e2e/common.sh
source "${SCRIPT_DIR}/e2e/common.sh"

require_env MODULES_MODULE_SOURCE
require_env MODULES_MODULE_NAME

readonly PR_TAG_TTL_DAYS="${REGISTRY_CLEANUP_PR_TAG_TTL_DAYS:-14}"
readonly RC_TAG_TTL_DAYS="${REGISTRY_CLEANUP_RC_TAG_TTL_DAYS:-60}"
readonly DRY_RUN="${REGISTRY_CLEANUP_DRY_RUN:-false}"
# MODULES_MODULE_SOURCE and MODULES_MODULE_NAME come from the workflow env block
# and are validated by require_env above.
# shellcheck disable=SC2154
readonly RELEASE_REPO="${MODULES_MODULE_SOURCE}/${MODULES_MODULE_NAME}/release"
readonly SECONDS_PER_DAY=$((24 * 60 * 60))

now_epoch="$(date +%s)"
readonly pr_threshold_epoch=$((now_epoch - PR_TAG_TTL_DAYS * SECONDS_PER_DAY))
readonly rc_threshold_epoch=$((now_epoch - RC_TAG_TTL_DAYS * SECONDS_PER_DAY))

log() {
  echo "[INFO] $*"
}

tag_created_epoch() {
  local tag="$1"

  crane config "${RELEASE_REPO}:${tag}" \
    | jq -r '.created | sub("\\.[0-9]+";"") | sub("Z?$";"Z") | fromdateiso8601'
}

# Echoes the expiry threshold epoch for a cleanup-candidate tag, or returns 1
# for protected tags (release channels, stable vX.Y.Z, main). Name-only, no
# network — lets the caller skip the crane config call for protected tags.
tag_threshold_epoch() {
  local tag="$1"

  if [[ "${tag}" =~ ^pr[0-9]+$ || "${tag}" =~ ^release-[0-9]+\.[0-9]+ ]]; then
    echo "${pr_threshold_epoch}"
  elif [[ "${tag}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+-rc\.[0-9]+$ ]]; then
    echo "${rc_threshold_epoch}"
  else
    return 1
  fi
}

# crane delete removes the manifest by the digest the tag points to. Tags that
# share a digest with the deleted one are untagged too, so only expired and
# unprotected tags reach this function.
delete_tag() {
  local tag="$1"

  if [ "${DRY_RUN}" = "true" ]; then
    log "[dry-run] would delete ${RELEASE_REPO}:${tag}"
    return
  fi

  log "deleting ${RELEASE_REPO}:${tag}"
  crane delete "${RELEASE_REPO}:${tag}"
}

cleanup_repo() {
  local tag threshold_epoch created_epoch
  local deleted=0 kept=0 failed=0

  while read -r tag; do
    [ -z "${tag}" ] && continue

    # Filter by name first: protected tags never reach the crane config call.
    threshold_epoch="$(tag_threshold_epoch "${tag}")" || { kept=$((kept + 1)); continue; }

    created_epoch="$(tag_created_epoch "${tag}" 2>/dev/null)" || created_epoch=""
    if [ -z "${created_epoch}" ]; then
      log "skip ${tag}: cannot resolve creation time"
      kept=$((kept + 1))
      continue
    fi

    if [ "${created_epoch}" -lt "${threshold_epoch}" ]; then
      if delete_tag "${tag}"; then
        deleted=$((deleted + 1))
      else
        log "WARN: failed to delete ${RELEASE_REPO}:${tag}"
        failed=$((failed + 1))
      fi
    else
      kept=$((kept + 1))
    fi
  done < <(crane ls "${RELEASE_REPO}")

  log "done: ${deleted} deleted, ${kept} kept, ${failed} failed"

  if [ "${failed}" -gt 0 ]; then
    return 1
  fi
}

log "cleaning custom release tags in ${RELEASE_REPO}"
log "dry run: ${DRY_RUN}"
log "pr/release branch tag TTL: ${PR_TAG_TTL_DAYS} days"
log "release candidate tag TTL: ${RC_TAG_TTL_DAYS} days"

cleanup_repo
