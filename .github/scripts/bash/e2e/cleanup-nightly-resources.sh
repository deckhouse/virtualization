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

collect_items_json() {
  local resource="$1"

  kubectl get "${resource}" -l "${LABEL_SELECTOR}" -o json \
    | jq -c '.items[] | {name: .metadata.name, created_at: .metadata.creationTimestamp, project: (.metadata.labels["projects.deckhouse.io/project"] // "")}'
}

cleanup_kind() {
  local kind="$1"
  local item
  local name
  local created_at
  local project

  # Every nightly run starts from a clean slate: all leftovers of previous runs
  # are deleted, no matter how old they are.
  echo "[INFO] Process ${kind} with label ${LABEL_SELECTOR}"
  collect_items_json "${kind}" | while read -r item; do
    name="$(echo "${item}" | jq -r '.name')"
    created_at="$(echo "${item}" | jq -r '.created_at')"
    project="$(echo "${item}" | jq -r '.project')"
    [ -z "${name}" ] && continue

    # Nested clusters are provisioned as Deckhouse Projects. The project's
    # controller owns the namespace and recreates it if it is deleted directly,
    # so deleting the namespace alone leaks the cluster forever. When the item is
    # backed by a project, delete the project instead: it cascades the namespace
    # and everything in it.
    if [ -n "${project}" ]; then
      printf "%-63s %22s\n" "[INFO] Delete project/${project} (owns ${kind}/${name}):" "created_at ${created_at}"
      # Deleting the project only triggers teardown and returns before the
      # namespace and its resources are actually gone — unlike a namespace
      # delete, which blocks until everything inside is finalized. Fire the
      # delete, then wait for the namespace to disappear so the resources are
      # really freed before moving on.
      kubectl delete projects.deckhouse.io "${project}" --wait=false || true
      kubectl wait --for=delete "namespace/${name}" --timeout=300s || true
      continue
    fi

    printf "%-63s %22s\n" "[INFO] Delete ${kind}/${name}:" "created_at ${created_at}"
    kubectl delete "${kind}" "${name}" --timeout=300s || true
  done || true
}

cleanup_kind "namespaces"
echo " "
cleanup_kind "vmclass"
