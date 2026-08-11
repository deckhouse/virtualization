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
timeout_seconds="${TIMEOUT_SECONDS:-1200}"
# Virtual machines are only moved when the workload images change: a new
# virt-handler drains the VMs off its node, a new virt-launcher makes the
# workload updater migrate the running ones. Releases that leave both untouched
# never trigger a migration, so there would be nothing to wait for.
migration_expected() {
  local module_source="${DEV_MODULE_SOURCE:-}"
  local current="${CURRENT_RELEASE:-}"
  local new="${NEW_RELEASE:-}"
  local current_digests new_digests image

  if [ -z "${module_source}" ] || [ -z "${current}" ] || [ -z "${new}" ]; then
    echo "[WARN] DEV_MODULE_SOURCE, CURRENT_RELEASE or NEW_RELEASE is not set, cannot tell whether the upgrade migrates VMs; waiting anyway"
    return 0
  fi

  if ! current_digests="$(module_images_digests "${module_source}" "${current}")" ||
     ! new_digests="$(module_images_digests "${module_source}" "${new}")"; then
    echo "[WARN] Failed to read the image digests of ${current} or ${new}, cannot tell whether the upgrade migrates VMs; waiting anyway"
    return 0
  fi

  for image in virtHandler virtLauncher; do
    if [ "$(jq -r --arg i "${image}" '.[$i] // ""' <<< "${current_digests}")" \
      != "$(jq -r --arg i "${image}" '.[$i] // ""' <<< "${new_digests}")" ]; then
      echo "[INFO] The ${image} image differs between ${current} and ${new}, virtual machines will be migrated"
      return 0
    fi
  done

  return 1
}

# The verdict is published so the new-release tests can tell a missing migration
# from one that was never going to happen.
publish_verdict() {
  [ -n "${GITHUB_OUTPUT:-}" ] || return 0
  echo "migrates_vms=$1" >> "${GITHUB_OUTPUT}"
}

if ! migration_expected; then
  publish_verdict false
  echo "[INFO] ${CURRENT_RELEASE} and ${NEW_RELEASE} ship the same virt-handler and virt-launcher: the upgrade does not migrate virtual machines, nothing to wait for"
  exit 0
fi

publish_verdict true

deadline=$(( $(date +%s) + timeout_seconds ))

# The number of Running VMs at the start is the number of Evict VMOPs we
# expect the workload updater to eventually create (one per migrated VM).
# Reading it once, before the loop, avoids racing a VM that flips out of
# Running while the wait is in progress.
vm_count="$(kubectl -n "${release_namespace}" get vm \
  -o jsonpath='{range .items[?(@.status.phase=="Running")]}{.metadata.name}{"\n"}{end}' \
  | grep -c . || true)"

echo "[INFO] Found ${vm_count} VirtualMachine(s) in Running phase in namespace ${release_namespace}."
echo "[INFO] Waiting for ${vm_count} Evict VirtualMachineOperation(s) to appear and reach Completed or Failed phase (timeout: ${timeout_seconds}s)..."

while true; do
  if [ "$(date +%s)" -ge "${deadline}" ]; then
    echo "[ERROR] Timeout of ${timeout_seconds}s reached before ${vm_count} Evict VMOP(s) settled." >&2
    kubectl -n "${release_namespace}" get vmop || true
    exit 1
  fi

  # VMOPs may be created at any moment, so re-read the full list on every
  # iteration and only look at the current snapshot. Only consider
  # migration operations (spec.type == Evict); other VMOP types are
  # irrelevant for this wait.
  phases="$(kubectl -n "${release_namespace}" get vmop \
    -o jsonpath='{range .items[?(@.spec.type=="Evict")]}{.status.phase}{"\n"}{end}')"

  vmop_count="$(printf '%s\n' "${phases}" | grep -c . || true)"
  terminal_count="$(printf '%s\n' "${phases}" | grep -cE '^(Completed|Failed)$' || true)"

  # Only stop once exactly as many Evict VMOPs exist as there were Running
  # VMs, and every one of them has reached a terminal phase. Fewer VMOPs
  # than VMs means migrations are still being created (or were never
  # created at all), which must not be mistaken for "nothing to wait for".
  if [ "${vmop_count}" -eq "${vm_count}" ] && [ "${terminal_count}" -eq "${vm_count}" ]; then
    echo "[INFO] All ${vm_count} Evict VMOP(s) are in a terminal phase (Completed or Failed)."
    kubectl -n "${release_namespace}" get vmop || true
    break
  fi

  echo "[INFO] ${terminal_count}/${vm_count} Evict VMOP(s) terminal (${vmop_count}/${vm_count} created); re-checking in ${sleep_interval}s..."
  sleep "${sleep_interval}"
done
