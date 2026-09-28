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

# wait_storage_profile waits until the virtualization-controller has created the
# StorageProfile for the given StorageClass and populated its access modes. The
# StorageProfile is an embedded resource of the virtualization module, so this
# must run in the configure-virtualization stage (after the module is Ready and
# before maintenance mode stops its reconciliation), not in configure-storage.
# The e2e suite's rwo-immediate-sc-precheck resolves the access mode of the
# default StorageClass from this StorageProfile; without the wait the suite can
# start before the profile exists and aborts with
# 'storageprofiles.storage.virtualization.deckhouse.io "<name>" not found'.
wait_storage_profile() {
  local sc_name="$1"
  local count=60
  local access_modes

  for i in $(seq 1 "${count}"); do
    access_modes="$(kubectl get storageprofiles.storage.virtualization.deckhouse.io "${sc_name}" -o jsonpath='{.status.claimPropertySets[*].accessModes[*]}' 2>/dev/null || echo "")"
    if [[ -n "${access_modes}" ]]; then
      echo "[SUCCESS] StorageProfile ${sc_name} is ready (accessModes: ${access_modes})"
      return 0
    fi

    echo "[INFO] Wait 10s for StorageProfile ${sc_name} to be populated (attempt ${i}/${count})"
    if (( i % 5 == 0 )); then
      echo "[DEBUG] StorageProfiles:"
      kubectl get storageprofiles.storage.virtualization.deckhouse.io || echo "[WARNING] Failed to retrieve storageprofiles"
    fi
    sleep 10
  done

  echo "[ERROR] StorageProfile ${sc_name} did not become ready in time"
  kubectl get storageprofiles.storage.virtualization.deckhouse.io "${sc_name}" -o yaml || true
  exit 1
}

SC_NAME="${1:-}"
if [ -z "${SC_NAME}" ]; then
  echo "[ERROR] Usage: wait-storage-profile.sh <storageclass-name>" >&2
  exit 1
fi

echo "[INFO] Wait for the ${SC_NAME} StorageProfile to be ready"
wait_storage_profile "${SC_NAME}"
