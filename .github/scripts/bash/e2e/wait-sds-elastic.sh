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

# Namespaces that carry the sds-elastic bring-up: the module itself, the Rook/Ceph
# operator it runs and the modules it depends on. Matched by pattern instead of a
# hardcoded list, because a stalled bring-up usually creates only some of them.
ELASTIC_DIAG_NS_REGEX="${ELASTIC_DIAG_NS_REGEX:-^d8-(sds-elastic|csi-ceph|sds-node-configurator|rook)}"

# How many log lines to keep per container in the failure dump.
ELASTIC_DIAG_LOG_TAIL="${ELASTIC_DIAG_LOG_TAIL:-200}"

# Echoes the existing namespaces of the sds-elastic bring-up, one per line.
elastic_diag_namespaces() {
  kubectl get ns -o name 2>/dev/null | sed 's|^namespace/||' | grep -E "${ELASTIC_DIAG_NS_REGEX}" || true
}

# Echoes a one-line pod summary per sds-elastic namespace, so the periodic progress
# output shows whether the module controllers are up at all without flooding the job
# log. A namespace with 0 pods means the module never deployed anything.
elastic_diag_pods_brief() {
  local ns
  local pods
  local total
  local bad

  for ns in $(elastic_diag_namespaces); do
    pods="$(kubectl -n "${ns}" get pods --no-headers 2>/dev/null || true)"
    # grep -c . counts non-empty lines, so an empty pod list reports 0 and not 1.
    total="$(printf '%s\n' "${pods}" | grep -c . || true)"
    bad="$(printf '%s\n' "${pods}" | grep -vE '\s(Running|Completed)\s' | grep -c . || true)"
    echo "[DEBUG] ${ns}: ${total} pods, ${bad} not Running/Completed"
  done
}

# Dumps everything needed to tell why an sds-elastic bring-up stalled.
#
# The wait loops used to print only the objects that were supposed to appear plus
# deckhouse-controller logs. Those logs say nothing about the module's own
# controllers, so a stalled ElasticCluster with an empty .status left no trace of the
# reason in the job log: it was impossible to tell a missing node label from a
# crashed controller from a Rook failure. This dump answers all three.
elastic_dump_diagnostics() {
  local ns
  local pod

  echo "[DEBUG] Collect sds-elastic diagnostics"

  echo "::group::nodes"
  kubectl get nodes -o wide --show-labels || true
  echo "::endgroup::"

  # The ElasticCluster storage.nodeSelector matches a storage.deckhouse.io/* label
  # that sds-elastic is expected to place on its data nodes. When that label is
  # missing the selector matches nothing and the cluster never starts.
  echo "::group::node storage labels"
  kubectl get nodes -o json 2>/dev/null | jq -r '
    .items[]
    | .metadata.name + " " + (
        [ .metadata.labels // {} | to_entries[]
          | select(.key | startswith("storage.deckhouse.io/"))
          | .key + "=" + .value
        ]
        | if length == 0 then "<no storage.deckhouse.io labels>" else join(",") end
      )' || true
  echo "::endgroup::"

  echo "::group::modules"
  kubectl get modules sds-elastic csi-ceph sds-node-configurator -o wide || true
  echo "::endgroup::"

  echo "::group::moduleconfigs"
  kubectl get moduleconfigs sds-elastic csi-ceph sds-node-configurator -o yaml || true
  echo "::endgroup::"

  # The ElasticCluster blockDeviceSelector matches app=elastic-osd, and a BlockDevice
  # rediscovery can drop that label, so print the labels and not just the devices.
  echo "::group::BlockDevices"
  kubectl get blockdevices.storage.deckhouse.io -o wide --show-labels || true
  echo "::endgroup::"

  echo "::group::namespaces"
  kubectl get ns || true
  echo "::endgroup::"

  for ns in $(elastic_diag_namespaces); do
    echo "::group::${ns} pods"
    kubectl -n "${ns}" get pods -o wide || true
    echo "::endgroup::"

    echo "::group::${ns} events"
    kubectl -n "${ns}" get events --sort-by=.lastTimestamp 2>/dev/null | tail -n 50 || true
    echo "::endgroup::"

    echo "::group::${ns} container logs"
    for pod in $(kubectl -n "${ns}" get pods -o name 2>/dev/null); do
      echo "--- ${ns}/${pod}"
      kubectl -n "${ns}" logs "${pod}" --all-containers --prefix --tail="${ELASTIC_DIAG_LOG_TAIL}" 2>&1 || true
      # A crash-looping controller keeps its reason in the previous container only.
      kubectl -n "${ns}" logs "${pod}" --all-containers --prefix --previous --tail="${ELASTIC_DIAG_LOG_TAIL}" 2>/dev/null || true
    done
    echo "::endgroup::"
  done

  echo "::group::deckhouse logs"
  d8 s logs | tail -n 100 || true
  echo "::endgroup::"
}

# Waits until the raw additional disks that back the Ceph OSDs are discovered by
# sds-node-configurator. Expects ELASTIC_OSD_DISKS_PER_NODE consumable BlockDevices
# per worker node (one OSD per additional disk).
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
    blockdevices="$(kubectl get blockdevices.storage.deckhouse.io -o json | jq '[.items[] | select(.status.consumable == true)] | length' || echo 0)"
    blockdevices=$((blockdevices))
    if [[ "$blockdevices" -ge "$expected" ]]; then
      echo "[SUCCESS] Consumable blockdevices (${blockdevices}) is greater or equal to expected (${expected} = ${workers} workers x ${disks_per_node} disks)"
      kubectl get blockdevices.storage.deckhouse.io -o wide
      return 0
    fi

    echo "[INFO] Wait 10s until consumable blockdevices >= ${expected} (attempt ${i}/${count})"
    if (( i % 5 == 0 )); then
      echo "[DEBUG] Show blockdevices"
      kubectl get blockdevices.storage.deckhouse.io -o wide || true
      echo "[DEBUG] Show queue (first 25 lines)"
      d8 s queue list | head -n25 || echo "No queues"
    fi
    sleep 10
  done

  echo "[ERROR] Consumable blockdevices did not reach ${expected} in time"
  elastic_dump_diagnostics
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
      echo "[DEBUG] LVMVolumeGroups"
      kubectl get lvmvolumegroup -o wide || true
      elastic_diag_pods_brief
      echo "[DEBUG] Show queue (first 25 lines)"
      d8 s queue list | head -n25 || echo "No queues"
    fi
    sleep 15
  done

  echo "[ERROR] ElasticCluster ${ec_name} did not become Ready/HEALTH_OK in time"
  echo "::group::ElasticCluster"
  kubectl get ec "$ec_name" -o yaml || true
  kubectl describe ec "$ec_name" || true
  echo "::endgroup::"
  echo "::group::LVMVolumeGroups"
  kubectl get lvmvolumegroup -o wide || true
  kubectl get lvmvolumegroup -o yaml || true
  echo "::endgroup::"
  echo "::group::CephCluster"
  kubectl get cephcluster -A -o yaml 2>/dev/null || true
  echo "::endgroup::"
  elastic_dump_diagnostics
  return 1
}
