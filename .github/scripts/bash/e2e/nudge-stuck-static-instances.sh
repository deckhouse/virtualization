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

# Workaround for a caps-controller-manager defect: a StaticInstance whose first SSH
# check fails is never retried, and the only recovery is MachineHealthCheck with
# nodeStartupTimeout 20m. dhctl gives up on resources after 15m, so bootstrap fails
# even though the cluster heals itself minutes later. See tmp/bug2.md.
#
# This script runs alongside `task dhctl-bootstrap`, watches StaticInstances in the
# nested cluster and does MHC's job earlier: deletes the Machine of an instance that
# passed the TCP check but never reached Running. The MachineSet recreates it and caps
# picks it up with fresh state.
#
# Remove once caps retries the check on its own.

set -Eeuo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=.github/scripts/bash/e2e/common.sh
source "${SCRIPT_DIR}/common.sh"
# shellcheck source=.github/scripts/bash/e2e/d8-ssh.sh
source "${SCRIPT_DIR}/d8-ssh.sh"

if [ "$#" -ne 3 ]; then
  echo "[ERROR] Usage: $0 <namespace> <prefix> <default-user>" >&2
  exit 1
fi

NAMESPACE="$1"
PREFIX="$2"
DEFAULT_USER="$3"
export NAMESPACE DEFAULT_USER

# Seconds a StaticInstance may sit past its TCP check before it counts as stuck.
# Healthy nodes reach Running in about two minutes, so 5m leaves room for a slow
# one without waiting out dhctl's 15m budget.
STUCK_AFTER="${STUCK_AFTER:-300}"
POLL_INTERVAL="${POLL_INTERVAL:-30}"
# Two nudges are all that fit: detection at 5m leaves a recreated node ready by ~7m,
# a second round by ~12m, and a third would be detected after dhctl has already given up.
NUDGE_LIMIT="${NUDGE_LIMIT:-2}"

CAPI_NAMESPACE="d8-cloud-instance-manager"
NESTED_KUBECTL="sudo -n /opt/deckhouse/bin/kubectl --kubeconfig=/etc/kubernetes/admin.conf"

nested_master=""
declare -A nudges=()
declare -A last_nudge=()
declare -A gave_up=()

# Echoes the master VM name, empty if it is not up yet.
resolve_master() {
  kubectl -n "${NAMESPACE}" get vm -l "group=${PREFIX}-master" \
    -o jsonpath="{.items[0].metadata.name}" 2>/dev/null || true
}

# Echoes stdout of a kubectl command run on the nested master, empty on any failure:
# during bootstrap the node, the API and the CRDs all appear at different moments, and
# every one of those gaps is expected rather than fatal.
nested_kubectl() {
  d8vssh "${nested_master}" "${NESTED_KUBECTL} $*" 2>/dev/null || true
}

# Echoes the names of StaticInstances that passed the TCP check but never reached
# Running, one per line.
stuck_instances() {
  local instances
  instances="$(nested_kubectl "-n ${CAPI_NAMESPACE} get staticinstance -o json")"
  [ -n "${instances}" ] || return 0

  jq -r --argjson after "${STUCK_AFTER}" '
    .items[]
    | select(.status.currentStatus.phase != "Running")
    | select(.status.machineRef.name != null)
    | . as $si
    | (.status.conditions // [])[]
    | select(.type == "CheckTcpConnection" and .status == "True")
    | select(now - (.lastTransitionTime | fromdateiso8601) > $after)
    | $si.metadata.name
  ' <<<"${instances}" 2>/dev/null || true
}

# Deletes the Machine that owns the StaticMachine reserved by the given StaticInstance.
nudge() {
  local instance="$1"
  local static_machine machine

  # Parsed with jq rather than kubectl jsonpath: the filter syntax would need quotes
  # escaped through both the local shell and the remote one.
  static_machine="$(nested_kubectl "-n ${CAPI_NAMESPACE} get staticinstance ${instance} -o json" |
    jq -r '.status.machineRef.name // empty')"
  if [ -z "${static_machine}" ]; then
    echo "[WARN] ${instance} has no machineRef anymore, skipping" >&2
    return 0
  fi

  # The Machine and the StaticMachine happen to share a name, but the ownerRef is what
  # actually ties them together.
  machine="$(nested_kubectl "-n ${CAPI_NAMESPACE} get staticmachine ${static_machine} -o json" |
    jq -r 'first(.metadata.ownerReferences[]? | select(.kind == "Machine") | .name) // empty')"
  if [ -z "${machine}" ]; then
    echo "[WARN] StaticMachine ${static_machine} has no owning Machine, skipping" >&2
    return 0
  fi

  echo "::warning::${instance} stuck past its TCP check for ${STUCK_AFTER}s, deleting Machine ${machine} to force a retry (caps defect workaround)"
  nested_kubectl "-n ${CAPI_NAMESPACE} delete machine ${machine} --wait=false" >/dev/null
  nudges["${instance}"]=$((${nudges["${instance}"]:-0} + 1))
  last_nudge["${instance}"]="${SECONDS}"
}

trap 'echo "[INFO] StaticInstance watcher stopped"; exit 0' TERM INT

echo "[INFO] Watching StaticInstances in ${NAMESPACE}: stuck after ${STUCK_AFTER}s, at most ${NUDGE_LIMIT} nudges each"

while true; do
  if [ -z "${nested_master}" ]; then
    nested_master="$(resolve_master)"
  fi

  if [ -n "${nested_master}" ]; then
    while read -r instance; do
      [ -n "${instance}" ] || continue

      if [ "${nudges["${instance}"]:-0}" -ge "${NUDGE_LIMIT}" ]; then
        if [ -z "${gave_up["${instance}"]:-}" ]; then
          echo "::warning::${instance} is still not Running after ${NUDGE_LIMIT} nudges, leaving it to MachineHealthCheck"
          gave_up["${instance}"]=1
        fi
        continue
      fi

      # CheckTcpConnection keeps its old timestamp across a nudge, so the instance looks
      # stuck again on the very next poll. Wait out a full window before nudging twice.
      if [ -n "${last_nudge["${instance}"]:-}" ] &&
        [ $((SECONDS - last_nudge["${instance}"])) -lt "${STUCK_AFTER}" ]; then
        continue
      fi

      nudge "${instance}"
    done < <(stuck_instances)
  fi

  sleep "${POLL_INTERVAL}"
done
