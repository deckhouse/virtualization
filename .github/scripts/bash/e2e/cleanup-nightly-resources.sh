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
source "${SCRIPT_DIR}/common.sh"

LABEL_SELECTOR="${LABEL_SELECTOR:-test=nightly-e2e}"
KEEP_HOURS="${KEEP_HOURS:-47}"
FRIDAY_KEEP_HOURS="${FRIDAY_KEEP_HOURS:-71}"
# sds-elastic (Ceph) nested clusters are heavy, so they are torn down sooner (~1 day)
# and are not granted the Friday extension. Matched by storage type in the resource name.
ELASTIC_KEEP_HOURS="${ELASTIC_KEEP_HOURS:-23}"
ELASTIC_NAME_PATTERN="${ELASTIC_NAME_PATTERN:-sds-elastic}"

current_date_seconds="$(date -u +%s)"

collect_items_json() {
  local resource="$1"

  kubectl get "${resource}" -l "${LABEL_SELECTOR}" -o json \
    | jq -c '.items[] | {name: .metadata.name, created_at: .metadata.creationTimestamp}'
}

should_keep() {
  local created_at="$1"
  local name="$2"
  local resource_created_at_seconds
  local age_seconds
  local weekday_of_day
  local keep_hours="${KEEP_HOURS}"
  local friday_keep_hours="${FRIDAY_KEEP_HOURS}"

  if [[ "${name}" == *"${ELASTIC_NAME_PATTERN}"* ]]; then
    keep_hours="${ELASTIC_KEEP_HOURS}"
    friday_keep_hours="${ELASTIC_KEEP_HOURS}"
  fi

  resource_created_at_seconds="$(date -d "${created_at}" -u +%s)"
  age_seconds="$(( current_date_seconds - resource_created_at_seconds ))"
  weekday_of_day="$(date -d "${created_at}" -u +%u)"

  if [ "${age_seconds}" -lt "$(( keep_hours * 3600 ))" ]; then
    echo "keep"
    return 0
  fi

  if [ "${weekday_of_day}" -eq 5 ] && [ "${age_seconds}" -lt "$(( friday_keep_hours * 3600 ))" ]; then
    echo "keep"
    return 0
  fi

  echo "delete"
}

cleanup_kind() {
  local kind="$1"
  local item
  local name
  local created_at
  local decision

  echo "[INFO] Process ${kind} with label ${LABEL_SELECTOR}"
  collect_items_json "${kind}" | while read -r item; do
    name="$(echo "${item}" | jq -r '.name')"
    created_at="$(echo "${item}" | jq -r '.created_at')"
    [ -z "${name}" ] && continue

    decision="$(should_keep "${created_at}" "${name}")"
    if [ "${decision}" = "keep" ]; then
      printf "%-63s %22s\n" "[INFO] Keep ${kind}/${name}:" "created_at ${created_at}"
      continue
    fi

    printf "%-63s %22s\n" "[INFO] Delete ${kind}/${name}:" "created_at ${created_at}"
    kubectl delete "${kind}" "${name}" --timeout=300s || true
  done || true
}

cleanup_kind "namespaces"
echo " "
cleanup_kind "vmclass"
