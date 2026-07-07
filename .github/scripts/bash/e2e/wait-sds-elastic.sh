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

# Waits until at least one Consumable BlockDevice per worker node is discovered by
# sds-node-configurator. These are the raw additional disks that back the Ceph OSDs.
elastic_blockdevices_ready() {
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
    blockdevices="$(kubectl get blockdevices.storage.deckhouse.io -o json | jq '[.items[] | select(.status.consumable == true)] | length' || echo 0)"
    blockdevices=$((blockdevices))
    if [[ "$blockdevices" -ge "$workers" ]]; then
      echo "[SUCCESS] Consumable blockdevices (${blockdevices}) is greater or equal to workers (${workers})"
      kubectl get blockdevices.storage.deckhouse.io -o wide
      return 0
    fi

    echo "[INFO] Wait 10s until consumable blockdevices >= ${workers} (attempt ${i}/${count})"
    if (( i % 5 == 0 )); then
      echo "[DEBUG] Show blockdevices"
      kubectl get blockdevices.storage.deckhouse.io -o wide || true
      echo "[DEBUG] Show queue (first 25 lines)"
      d8 s queue list | head -n25 || echo "No queues"
    fi
    sleep 10
  done

  echo "[ERROR] Consumable blockdevices did not reach ${workers} in time"
  echo "[DEBUG] Show cluster nodes"
  kubectl get nodes || true
  echo "[DEBUG] Show blockdevices"
  kubectl get blockdevices.storage.deckhouse.io -o wide || true
  echo "[DEBUG] Show deckhouse logs"
  echo "::group::deckhouse logs"
  d8 s logs | tail -n 100 || true
  echo "::endgroup::"
  return 1
}

# Waits until the ElasticCluster reaches phase Ready and Ceph reports HEALTH_OK.
# Rook cluster bring-up (mon/mgr/osd) can take many minutes, so the timeout is generous.
elastic_cluster_ready() {
  local ec_name="$1"
  local count=120
  local phase
  local health

  for i in $(seq 1 "$count"); do
    phase="$(kubectl get ec "$ec_name" -o jsonpath='{.status.phase}' 2>/dev/null || echo "")"
    health="$(kubectl get ec "$ec_name" -o jsonpath='{.status.health.status}' 2>/dev/null || echo "")"

    if [[ "$phase" == "Ready" && "$health" == "HEALTH_OK" ]]; then
      echo "[SUCCESS] ElasticCluster ${ec_name} is Ready (${health})"
      kubectl get ec "$ec_name" -o wide
      return 0
    fi

    echo "[INFO] Wait 15s for ElasticCluster ${ec_name} (phase=${phase:-<none>}, health=${health:-<none>}) (attempt ${i}/${count})"
    if (( i % 5 == 0 )); then
      echo "[DEBUG] ElasticCluster status"
      kubectl get ec "$ec_name" -o wide || true
      echo "[DEBUG] CephCluster status"
      kubectl -n d8-csi-ceph get cephcluster -o wide 2>/dev/null || true
      echo "[DEBUG] Show queue (first 25 lines)"
      d8 s queue list | head -n25 || echo "No queues"
    fi
    sleep 15
  done

  echo "[ERROR] ElasticCluster ${ec_name} did not become Ready/HEALTH_OK in time"
  echo "::group::ElasticCluster"
  kubectl get ec "$ec_name" -o yaml || true
  echo "::endgroup::"
  echo "::group::LVMVolumeGroups"
  kubectl get lvmvolumegroup -o wide || true
  echo "::endgroup::"
  echo "::group::CephCluster"
  kubectl -n d8-csi-ceph get cephcluster -o yaml 2>/dev/null || true
  echo "::endgroup::"
  echo "::group::deckhouse logs"
  d8 s logs | tail -n 100 || true
  echo "::endgroup::"
  return 1
}
