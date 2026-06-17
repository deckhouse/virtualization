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

sds_local_volume_ready() {
  local count=90
  local local_volume_status
  local csi_node_desired
  local csi_node_ready
  local deploy_count
  local controller_ready

  for i in $(seq 1 "${count}"); do
    local_volume_status="$(kubectl get modules sds-local-volume -o jsonpath='{.status.phase}' 2>/dev/null || echo "False")"
    csi_node_desired="$(kubectl -n d8-sds-local-volume get ds csi-node -o jsonpath='{.status.desiredNumberScheduled}' 2>/dev/null || echo "0")"
    csi_node_ready="$(kubectl -n d8-sds-local-volume get ds csi-node -o jsonpath='{.status.numberReady}' 2>/dev/null || echo "0")"
    deploy_count="$(kubectl -n d8-sds-local-volume get deploy -o name 2>/dev/null | wc -l | tr -d ' ')"
    controller_ready=false

    if [[ "${deploy_count}" -gt 0 ]] && kubectl -n d8-sds-local-volume wait --for=condition=Available deploy --all --timeout=10s >/dev/null 2>&1; then
      controller_ready=true
    fi

    if [[ "${local_volume_status}" == "Ready" && "${csi_node_desired}" -gt 0 && "${csi_node_ready}" -eq "${csi_node_desired}" && "${controller_ready}" == "true" ]]; then
      echo "[SUCCESS] sds-local-volume is ready (module=${local_volume_status}, csi-node=${csi_node_ready}/${csi_node_desired}, deployments=${deploy_count})"
      kubectl get modules sds-local-volume
      kubectl -n d8-sds-local-volume get pods
      return 0
    fi

    echo "[INFO] Waiting for sds-local-volume to be ready (attempt ${i}/${count})"
    echo "[WARNING] Current state: module=${local_volume_status}, csi-node=${csi_node_ready}/${csi_node_desired}, deployments=${deploy_count}, controller_ready=${controller_ready}"
    if (( i % 5 == 0 )); then
      kubectl get ns d8-sds-local-volume || true
      kubectl get modules sds-local-volume -o wide || true
      kubectl -n d8-sds-local-volume get pods || true
      kubectl -n d8-sds-local-volume get ds || true
      kubectl -n d8-sds-local-volume get deploy || true
      d8 s queue list | head -n 25 || true
    fi
    sleep 10
  done

  echo "[ERROR] sds-local-volume did not become ready in time"
  kubectl get modules sds-local-volume -o wide || true
  kubectl -n d8-sds-local-volume get pods || true
  d8 s queue list || true
  echo "::group::deckhouse logs"
  d8 s logs | tail -n 100
  echo "::endgroup::"
  exit 1
}

echo "[INFO] Apply sds-local-volume ModuleConfig"
kubectl apply -f mc.yaml

echo "[INFO] Wait for sds-local-volume module queue"
d8_queue
kubectl wait --for=jsonpath='{.status.phase}'=Ready modules sds-local-volume --timeout=300s
sds_local_volume_ready

chmod +x ./lsc-gen.sh
./lsc-gen.sh

echo "[INFO] Show resulting local storage classes"
kubectl get localstorageclass || true
