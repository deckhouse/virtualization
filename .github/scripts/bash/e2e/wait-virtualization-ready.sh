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

debug_output() {
  local nodes

  echo "[ERROR] Virtualization module deploy failed"
  echo "[DEBUG] Show describe virtualization module"
  echo "::group::describe virtualization module"
  kubectl describe modules virtualization || true
  echo "::endgroup::"
  echo "[DEBUG] Show namespace d8-virtualization"
  kubectl get ns d8-virtualization || true
  echo "[DEBUG] Show pods in namespace d8-virtualization"
  kubectl -n d8-virtualization get pods || true
  echo "[DEBUG] Show dvcr info"
  echo "::group::dvcr pod describe"
  kubectl -n d8-virtualization describe pod -l app=dvcr || true
  echo "::endgroup::"
  echo " "
  echo "::group::dvcr pod yaml"
  kubectl -n d8-virtualization get pods -l app=dvcr -o yaml || true
  echo "::endgroup::"
  echo " "
  echo "::group::dvcr deployment yaml"
  kubectl -n d8-virtualization get deployment -l app=dvcr -o yaml || true
  echo "::endgroup::"
  echo " "
  echo "::group::dvcr deployment describe"
  kubectl -n d8-virtualization describe deployment -l app=dvcr || true
  echo "::endgroup::"
  echo " "
  echo "::group::dvcr service yaml"
  kubectl -n d8-virtualization get service -l app=dvcr -o yaml || true
  echo "::endgroup::"
  echo " "
  echo "[DEBUG] Show pvc in namespace d8-virtualization"
  kubectl get pvc -n d8-virtualization || true
  echo "[DEBUG] Show cluster StorageClasses"
  kubectl get storageclasses || true
  echo "[DEBUG] Show cluster nodes"
  kubectl get node || true

  echo "[DEBUG] Show cluster node yaml and describe"
  nodes="$(kubectl get no -o jsonpath='{range .items[?(@.metadata.name)]}{.metadata.name}{"\n"}{end}')"
  for node in $nodes; do
    echo "::group::show cluster node ${node} yaml"
    kubectl get node "$node" -o yaml
    echo "::endgroup::"
    echo "::group::show cluster node ${node} describe"
    kubectl describe node "$node"
    echo "::endgroup::"
  done

  echo "[DEBUG] Show queue (first 25 lines)"
  d8 s queue list | head -n 25 || echo "[WARNING] Failed to retrieve list queue"
  echo "[DEBUG] Show deckhouse logs"
  echo "::group::deckhouse logs"
  d8 s logs | tail -n 100
  echo "::endgroup::"
}

virtualization_ready() {
  local count=90
  local virtualization_status

  for i in $(seq 1 "$count"); do
    virtualization_status="$(kubectl get modules virtualization -o jsonpath='{.status.phase}')"
    if [ "$virtualization_status" = "Ready" ]; then
      echo "[SUCCESS] Virtualization module is ready"
      kubectl get modules virtualization
      kubectl -n d8-virtualization get pods
      kubectl get vmclass || echo "[WARNING] no vmclasses found"
      return 0
    fi

    echo "[INFO] Waiting 10s for Virtualization module to be ready (attempt ${i}/${count})"

    if (( i % 5 == 0 )); then
      echo " "
      echo "[DEBUG] Show additional info"
      kubectl get ns d8-virtualization || echo "[WARNING] Namespace virtualization is not ready"
      echo " "
      kubectl -n d8-virtualization get pods || echo "[WARNING] Pods in namespace virtualization is not ready"
      kubectl get pvc -n d8-virtualization || echo "[WARNING] PVC in namespace virtualization is not ready"
      echo " "
      echo "d8-virtualization module status: ${virtualization_status}"
      echo " "
    fi
    sleep 10
  done

  debug_output
  return 1
}

virt_handler_ready() {
  local count=180
  local virt_handler_ready
  local nodes_total
  local time_wait=10

  for i in $(seq 1 "$count"); do
    # Count all nodes, not just workers: virt-handler is a per-node DaemonSet, and
    # the master gets it later (slower to converge). "|| true" so a transient
    # kubectl error doesn't abort under "set -eo pipefail" (falls back to 0).
    nodes_total="$(kubectl get nodes -o name | wc -l || true)"
    nodes_total=$((nodes_total))
    if [[ "$nodes_total" -eq 0 ]]; then
      echo "[WARNING] No nodes found (or kubectl failed), keep waiting"
      echo "[INFO] Wait ${time_wait}s virt-handler pods are ready (attempt ${i}/${count})"
      sleep "$time_wait"
      continue
    fi

    virt_handler_ready="$(kubectl -n d8-virtualization get pods | grep -c "virt-handler.*Running" || true)"

    if [[ "$virt_handler_ready" -ge "$nodes_total" ]]; then
      echo "[SUCCESS] virt-handler pods are ready ${virt_handler_ready}/${nodes_total}"
      return 0
    fi

    echo "[INFO] virt-handler pods ${virt_handler_ready}/${nodes_total}"
    echo "[INFO] Wait ${time_wait}s virt-handler pods are ready (attempt ${i}/${count})"
    if (( i % 5 == 0 )); then
      echo "[DEBUG] Show pods in namespace d8-virtualization"
      echo "::group::virtualization pods"
      kubectl -n d8-virtualization get pods || echo "[WARNING] No pods in virtualization namespace found"
      echo "::endgroup::"
      echo "[DEBUG] Show cluster nodes"
      echo "::group::cluster nodes"
      kubectl get node || echo "[WARNING] Failed to get cluster nodes"
      echo "::endgroup::"
    fi
    sleep "$time_wait"
  done

  debug_output
  return 1
}

enable_maintenance_mode() {
  if [ "$#" -ne 1 ]; then
    echo "[ERROR] Usage: enable_maintenance_mode <storage-type>" >&2
    return 1
  fi

  local storage_type="$1"

  echo "[INFO] Switch virtualization module to maintenance mode"
  kubectl patch mc virtualization --type merge --patch '{"spec":{"maintenance":"NoResourceReconciliation"}}'

  case "${storage_type}" in
  replicated)
    echo "[INFO] Switch sds-replicated-volume module to maintenance mode"
    kubectl patch mc sds-replicated-volume --type merge --patch '{"spec":{"maintenance":"NoResourceReconciliation"}}'
    ;;
  nfs)
    echo "[INFO] Switch csi-nfs module to maintenance mode"
    kubectl patch mc csi-nfs --type merge --patch '{"spec":{"maintenance":"NoResourceReconciliation"}}'
    ;;
  sds-elastic)
    echo "[INFO] Switch sds-elastic and csi-ceph modules to maintenance mode"
    kubectl patch mc sds-elastic --type merge --patch '{"spec":{"maintenance":"NoResourceReconciliation"}}'
    kubectl patch mc csi-ceph --type merge --patch '{"spec":{"maintenance":"NoResourceReconciliation"}}'
    ;;
  local)
    echo "[INFO] Switch sds-local-volume module to maintenance mode"
    kubectl patch mc sds-local-volume --type merge --patch '{"spec":{"maintenance":"NoResourceReconciliation"}}'
    ;;
  *)
    echo "[INFO] No storage module maintenance mode patch for storage type: ${storage_type}"
    ;;
  esac
}
