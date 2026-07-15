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

# Regex matching the LVMVolumeGroup names that sds-elastic creates for its OSDs
# (e.g. sds-elastic-elastic-osd-80a5ec17). Used to recognise BlockDevices that a
# previous pipeline run already consumed into OSDs: such devices are no longer
# .status.consumable == true but must still count as OSD candidates so a rerun
# (including a partial one, where only some devices were consumed) can proceed.
ELASTIC_OSD_LVG_REGEX="${ELASTIC_OSD_LVG_REGEX:-^sds-elastic-.*-osd-}"

# jq filter selecting BlockDevices usable as elastic OSDs: either still consumable
# (fresh run) or already backing an sds-elastic OSD LVMVolumeGroup (rerun).
# Requires the jq argument --arg re "$ELASTIC_OSD_LVG_REGEX".
ELASTIC_OSD_BD_SELECT='.status.consumable == true or ((.status.lvmVolumeGroupName // "") | test($re))'

# Waits until the raw additional disks that back the Ceph OSDs are available for
# sds-elastic. Expects ELASTIC_OSD_DISKS_PER_NODE OSD-candidate BlockDevices per
# worker node (one OSD per additional disk). A BlockDevice counts as a candidate
# when it is still consumable or was already consumed into an sds-elastic OSD LVG
# by an earlier run, so the check is idempotent across pipeline restarts.
elastic_blockdevices_ready() {
  local count=60
  local workers
  local blockdevices
  local disks_per_node="${ELASTIC_OSD_DISKS_PER_NODE:-1}"
  local expected

  workers="$(kubectl get nodes -o name | grep -c worker || true)"
  workers=$((workers))

  if [[ "$workers" -eq 0 ]]; then
    echo "[ERROR] No worker nodes found"
    return 1
  fi

  expected=$(( workers * disks_per_node ))

  for i in $(seq 1 "$count"); do
    blockdevices="$(kubectl get blockdevices.storage.deckhouse.io -o json | jq --arg re "$ELASTIC_OSD_LVG_REGEX" "[.items[] | select(${ELASTIC_OSD_BD_SELECT})] | length" || echo 0)"
    blockdevices=$((blockdevices))
    if [[ "$blockdevices" -ge "$expected" ]]; then
      echo "[SUCCESS] OSD-candidate blockdevices (${blockdevices}) is greater or equal to expected (${expected} = ${workers} workers x ${disks_per_node} disks)"
      kubectl get blockdevices.storage.deckhouse.io -o wide
      return 0
    fi

    echo "[INFO] Wait 10s until OSD-candidate blockdevices >= ${expected} (attempt ${i}/${count})"
    if (( i % 5 == 0 )); then
      echo "[DEBUG] Show blockdevices"
      kubectl get blockdevices.storage.deckhouse.io -o wide || true
      echo "[DEBUG] Show queue (first 25 lines)"
      d8 s queue list | head -n25 || echo "No queues"
    fi
    sleep 10
  done

  echo "[ERROR] OSD-candidate blockdevices did not reach ${expected} in time"
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
# Rook cluster bring-up (mon/mgr/osd) on nested VMs is slow: with several OSDs per node
# plus occasional sds-node-configurator restarts a full bring-up can take ~50 min, so the
# timeout is deliberately generous (240 x 15s = 60 min).
elastic_cluster_ready() {
  local ec_name="$1"
  local count=240
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
      kubectl get cephcluster -A -o wide 2>/dev/null || true
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
  kubectl get cephcluster -A -o yaml 2>/dev/null || true
  echo "::endgroup::"
  echo "::group::deckhouse logs"
  d8 s logs | tail -n 100 || true
  echo "::endgroup::"
  return 1
}
