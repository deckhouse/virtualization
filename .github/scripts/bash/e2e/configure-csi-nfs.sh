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

nfs_ready() {
  local count=90
  local controller
  local csi_controller
  local csi_node_desired
  local csi_node_ready

  for i in $(seq 1 "${count}"); do
    echo "[INFO] Check d8-csi-nfs pods (attempt ${i}/${count})"
    controller="$(kubectl -n d8-csi-nfs get deploy controller -o jsonpath='{.status.readyReplicas}' 2>/dev/null || echo "0")"
    csi_controller="$(kubectl -n d8-csi-nfs get deploy csi-controller -o jsonpath='{.status.readyReplicas}' 2>/dev/null || echo "0")"
    csi_node_desired="$(kubectl -n d8-csi-nfs get ds csi-node -o jsonpath='{.status.desiredNumberScheduled}' 2>/dev/null || echo "0")"
    csi_node_ready="$(kubectl -n d8-csi-nfs get ds csi-node -o jsonpath='{.status.numberReady}' 2>/dev/null || echo "0")"

    if [[ "${controller}" -ge 1 && "${csi_controller}" -ge 1 && "${csi_node_desired}" -gt 0 && "${csi_node_ready}" -eq "${csi_node_desired}" ]]; then
      echo "[SUCCESS] NFS CSI is ready (controller=${controller}, csi-controller=${csi_controller}, csi-node=${csi_node_ready}/${csi_node_desired})"
      return 0
    fi

    echo "[WARNING] NFS CSI not ready: controller=${controller}, csi-controller=${csi_controller}, csi-node=${csi_node_ready}/${csi_node_desired}"
    if (( i % 5 == 0 )); then
      echo "[DEBUG] Pods in d8-csi-nfs:"
      kubectl -n d8-csi-nfs get pods || echo "[WARNING] Failed to retrieve pods"
      echo "[DEBUG] Deployments in d8-csi-nfs:"
      kubectl -n d8-csi-nfs get deploy || echo "[WARNING] Failed to retrieve deployments"
      echo "[DEBUG] DaemonSets in d8-csi-nfs:"
      kubectl -n d8-csi-nfs get ds || echo "[WARNING] Failed to retrieve daemonsets"
      echo "[DEBUG] csi-nfs module status:"
      kubectl get modules csi-nfs -o wide || echo "[WARNING] Failed to retrieve module"
    fi
    sleep 10
  done

  echo "[ERROR] NFS CSI did not become ready in time"
  kubectl -n d8-csi-nfs get pods || true
  exit 1
}

echo "[INFO] Apply csi-nfs ModuleConfig, ModulePullOverride, snapshot-controller"
kubectl apply -f mc.yaml

echo "[INFO] Wait for csi-nfs module to be ready"
kubectl wait --for=jsonpath='{.status.phase}'=Ready modules csi-nfs --timeout=300s

echo "[INFO] Wait for csi-nfs pods to be ready"
nfs_ready

echo "[INFO] Apply NFSStorageClass"
envsubst < storageclass.yaml | kubectl apply -f -

configure_default_sc="${CONFIGURE_DEFAULT_SC:-true}"
if [[ "${configure_default_sc}" == "true" ]]; then
  echo "[INFO] Configure default storage class"
  # The workflow runs this script from test/dvp-static-cluster/storage/nfs.
  ./default-sc-configure.sh
else
  echo "[INFO] Skip default storage class configuration"
fi

echo "[INFO] Show existing storageclasses"
kubectl get storageclass
