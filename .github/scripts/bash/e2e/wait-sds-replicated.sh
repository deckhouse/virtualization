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
# shellcheck source=.github/scripts/bash/e2e/deckhouse.sh
source "${SCRIPT_DIR}/deckhouse.sh"

sds_replicated_ready() {
  local count=60
  local sds_replicated_volume_status

  for i in $(seq 1 "$count"); do
    sds_replicated_volume_status="$(kubectl get ns d8-sds-replicated-volume -o jsonpath='{.status.phase}' || echo "False")"

    if [[ "$sds_replicated_volume_status" = "Active" ]]; then
      echo "[SUCCESS] Namespaces sds-replicated-volume are Active"
      kubectl get ns d8-sds-replicated-volume
      return 0
    fi

    echo "[INFO] Waiting 10s for sds-replicated-volume namespace to be ready (attempt ${i}/${count})"
    if (( i % 5 == 0 )); then
      echo "[INFO] Show namespaces sds-replicated-volume"
      kubectl get ns | grep sds-replicated-volume || echo "Namespaces sds-replicated-volume are not ready"
      echo "[DEBUG] Show queue (first 25 lines)"
      d8 s queue list | head -n25 || echo "No queues"
    fi
    sleep 10
  done

  echo "[ERROR] Namespaces sds-replicated-volume are not ready after ${count} attempts"
  echo "[DEBUG] Show namespaces sds"
  kubectl get ns | grep sds || echo "Namespaces sds-replicated-volume are not ready"
  echo "[DEBUG] Show queue"
  echo "::group::Show queue"
  d8 s queue list || echo "No queues"
  echo "::endgroup::"
  echo "[DEBUG] Show deckhouse logs"
  echo "::group::deckhouse logs"
  d8 s logs | tail -n 100
  echo "::endgroup::"
  return 1
}

sds_pods_ready() {
  local count=100
  local linstor_node
  local csi_node
  local workers

  workers="$(kubectl get nodes -o name | grep -c worker || true)"
  workers=$((workers))

  echo "[INFO] Wait while linstor-node csi-node webhooks pods are ready"
  for i in $(seq 1 "$count"); do
    linstor_node="$(kubectl -n d8-sds-replicated-volume get pods | grep -c "linstor-node.*Running" || true)"
    csi_node="$(kubectl -n d8-sds-replicated-volume get pods | grep -c "csi-node.*Running" || true)"

    echo "[INFO] Check if sds-replicated pods are ready"
    if [[ "$linstor_node" -ge "$workers" && "$csi_node" -ge "$workers" ]]; then
      echo "[SUCCESS] sds-replicated-volume is ready"
      return 0
    fi

    echo "[WARNING] Not all pods are ready, linstor_node=${linstor_node}, csi_node=${csi_node}"
    echo "[INFO] Waiting 10s for pods to be ready (attempt ${i}/${count})"
    if (( i % 5 == 0 )); then
      echo "[DEBUG] Get pods"
      kubectl -n d8-sds-replicated-volume get pods || true
      echo "[DEBUG] Show queue (first 25 lines)"
      d8 s queue list | head -n 25 || echo "Failed to retrieve list queue"
      echo " "
    fi
    sleep 10
  done

  echo "[ERROR] sds-replicated-volume is not ready after ${count} attempts"
  echo "[DEBUG] Get pods"
  echo "::group::sds-replicated-volume pods"
  kubectl -n d8-sds-replicated-volume get pods || true
  echo "::endgroup::"
  echo "[DEBUG] Show queue"
  echo "::group::Show queue"
  d8 s queue list || echo "Failed to retrieve list queue"
  echo "::endgroup::"
  echo "[DEBUG] Show deckhouse logs"
  echo "::group::deckhouse logs"
  d8 s logs | tail -n 100
  echo "::endgroup::"
  return 1
}

blockdevices_ready() {
  local count=60
  local workers
  local blockdevices

  workers="$(kubectl get nodes -o name | grep -c worker || true)"
  workers=$((workers))

  if [[ "$workers" -eq 0 ]]; then
    echo "[ERROR] No worker nodes found"
    return 1
  fi

  for i in $(seq 1 "$count"); do
    blockdevices="$(kubectl get blockdevice -o name | wc -l | tr -d ' ' || true)"
    blockdevices=$((blockdevices))
    if [[ "$blockdevices" -ge "$workers" ]]; then
      echo "[SUCCESS] Blockdevices is greater or equal to $workers"
      kubectl get blockdevice
      return 0
    fi

    echo "[INFO] Wait 10 sec until blockdevices is greater or equal to $workers (attempt ${i}/${count})"
    if (( i % 5 == 0 )); then
      echo "[DEBUG] Show queue (first 25 lines)"
      d8 s queue list | head -n25 || echo "No queues"
    fi

    sleep 10
  done

  echo "[ERROR] Blockdevices is not 3"
  echo "[DEBUG] Show cluster nodes"
  kubectl get nodes || echo "[WARNING] Failed to get cluster nodes"
  echo "[DEBUG] Show blockdevices"
  kubectl get blockdevice || echo "[WARNING] Failed to get blockdevices"
  echo "[DEBUG] Show sds namespaces"
  kubectl get ns | grep sds || echo "[WARNING] Namespace sds is not found"
  echo "[DEBUG] Show pods in sds-replicated-volume"
  echo "::group::pods in sds-replicated-volume"
  kubectl -n d8-sds-replicated-volume get pods || echo "[WARNING] Failed to get pods in sds-replicated-volume"
  echo "::endgroup::"
  echo "[DEBUG] Show deckhouse logs"
  echo "::group::deckhouse logs"
  d8 s logs | tail -n 100 || echo "[WARNING] Failed to get deckhouse logs"
  echo "::endgroup::"
  return 1
}
