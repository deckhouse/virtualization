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

require_env RELEASE_NAMESPACE

required_env_value() {
  local name="$1"

  require_env "${name}"
  printf '%s' "${!name}"
}

release_namespace="$(required_env_value RELEASE_NAMESPACE)"

sleep_interval="${SLEEP_INTERVAL:-10}"
timeout_seconds="${TIMEOUT_SECONDS:-600}"
deadline=$(( $(date +%s) + timeout_seconds ))

echo "[INFO] Waiting for all Evict VirtualMachineOperations in namespace ${release_namespace} to reach Completed or Failed phase (timeout: ${timeout_seconds}s)..."

while true; do
  if [ "$(date +%s)" -ge "${deadline}" ]; then
    echo "[ERROR] Timeout of ${timeout_seconds}s reached while waiting for Evict VMOPs to complete." >&2
    kubectl -n "${release_namespace}" get vmop || true
    exit 1
  fi

  # VMOPs may be created or deleted at any moment, so re-read the full
  # list on every iteration and only look at the current snapshot.
  # Only consider migration operations (spec.type == Evict); other VMOP
  # types are irrelevant for this wait.
  phases="$(kubectl -n "${release_namespace}" get vmop \
    -o jsonpath='{range .items[?(@.spec.type=="Evict")]}{.status.phase}{"\n"}{end}')"

  if [ -z "${phases}" ]; then
    echo "[INFO] No Evict VMOPs found at the moment, nothing to wait for."
    break
  fi

  # Count Evict VMOPs that are not yet in a terminal phase.
  pending="$(printf '%s\n' "${phases}" | grep -vE '^(Completed|Failed)$' | grep -c . || true)"

  if [ "${pending}" -eq 0 ]; then
    echo "[INFO] All Evict VMOPs are in a terminal phase (Completed or Failed)."
    kubectl -n "${release_namespace}" get vmop || true
    break
  fi

  echo "[INFO] ${pending} Evict VMOP(s) still in progress, re-checking in ${sleep_interval}s..."
  sleep "${sleep_interval}"
done
