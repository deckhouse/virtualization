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

# Runs the release smoke suite against the cluster of the current kubeconfig,
# the way the "E2E Release Test Reusable Pipeline" runs it in the nested
# cluster, so a change to the suite can be checked without bootstrapping one:
#
#   pre      create the VMs, start iperf, save the upgrade context
#   upgrade  move the VMs the way a module upgrade would (see UPGRADE_MODE)
#   post     check the VMs survived: guest access, disks, no restart, iperf, VMOPs
#   cleanup  delete the release Project with everything in it
#   all      pre, upgrade, post, then cleanup unless KEEP=yes (default: cleanup)
#
# Usage: run-local.sh [pre|upgrade|post|cleanup|all]
#
# Environment:
#   UPGRADE_MODE   evict   (default) an Evict VMOP for every Running VM, the
#                          migrations a new virt-handler/virt-launcher triggers;
#                          fails if the iperf server VM cannot be evicted
#                  none    nothing moves; post skips the migration checks
#                  manual  pause until Enter, do the disruption yourself
#                          (restart virt-handler, roll a dev build, ...);
#                          RELEASE_UPGRADE_MIGRATES_VMS=false if nothing migrated
#                  mpo     a real upgrade: patch the ModulePullOverride to
#                          NEW_RELEASE; needs CURRENT_RELEASE, NEW_RELEASE,
#                          DEV_MODULE_SOURCE and crane logged in to it.
#                          The cluster stays on NEW_RELEASE afterwards.
#   RELEASE_STORAGE_CLASS_REPLICATED       required: the storage classes that
#   RELEASE_STORAGE_CLASS_LOCAL            stand in for the nested cluster's
#   RELEASE_STORAGE_CLASS_NFS              nested-thin-r1, nested-local-thin, nfs
#   STATE_DIR      where the phases hand over state (default: ${TMPDIR:-/tmp}/release-e2e-local)
#   KEEP           yes - leave the resources after "all" (default: no)
#   TIMEOUT_SECONDS  how long the upgrade phase waits for the Evict VMOPs to
#                  settle, in evict and mpo modes (default: 1200)
#   GINKGO_ARGS    extra ginkgo flags, e.g. "--timeout=2h"
#
# The suite needs what the pipeline prepares in the nested cluster: the
# generic-for-e2e VirtualMachineClass (create-e2e-vmclass.sh) and the
# multitenancy-manager "default" ProjectTemplate. The prechecks report what is
# missing.

set -Eeuo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
E2E_DIR="$(cd -- "${SCRIPT_DIR}/.." && pwd)"
REPO_DIR="$(cd -- "${E2E_DIR}/../.." && pwd)"
CI_SCRIPT_DIR="${REPO_DIR}/.github/scripts/bash/e2e"

STATE_DIR="${STATE_DIR:-${TMPDIR:-/tmp}/release-e2e-local}"
CONTEXT_PATH="${STATE_DIR}/release-upgrade-context.json"
MIGRATES_VMS_PATH="${STATE_DIR}/migrates-vms"
UPGRADE_MODE="${UPGRADE_MODE:-evict}"

log() {
  echo "[INFO] $*"
}

fail() {
  echo "[ERROR] $*" >&2
  exit 1
}

require_tools() {
  local tool

  for tool in "$@"; do
    command -v "${tool}" >/dev/null || fail "${tool} not found in PATH"
  done
}

release_namespace() {
  [ -f "${CONTEXT_PATH}" ] || fail "no upgrade context at ${CONTEXT_PATH}: run the pre phase first"
  jq -r '.namespace' "${CONTEXT_PATH}"
}

# The scenarios default to the storage classes of the nested cluster, which
# another cluster rarely has, so the harness takes them explicitly.
precheck_storage_classes() {
  local storage_classes var
  local problems=()

  storage_classes="$(kubectl get storageclass -o name)"
  for var in RELEASE_STORAGE_CLASS_REPLICATED RELEASE_STORAGE_CLASS_LOCAL RELEASE_STORAGE_CLASS_NFS; do
    if [ -z "${!var:-}" ]; then
      problems+=("${var} is not set")
    elif ! grep -qxF "storageclass.storage.k8s.io/${!var}" <<< "${storage_classes}"; then
      problems+=("${var}=${!var}: no such StorageClass")
    fi
  done

  if [ "${#problems[@]}" -gt 0 ]; then
    printf '[ERROR] %s\n' "${problems[@]}" >&2
    cat >&2 <<EOF

Set the storage classes for the replicated, local and NFS disks, for example:

  export RELEASE_STORAGE_CLASS_REPLICATED=linstor-thin-r1
  export RELEASE_STORAGE_CLASS_LOCAL=sds-local-thin-wffc
  export RELEASE_STORAGE_CLASS_NFS=nfs-4-1-wffc
  task release:local

StorageClasses in this cluster:
$(kubectl get storageclass)
EOF
    exit 1
  fi

  export RELEASE_STORAGE_CLASS_REPLICATED RELEASE_STORAGE_CLASS_LOCAL RELEASE_STORAGE_CLASS_NFS
  log "Storage classes: replicated=${RELEASE_STORAGE_CLASS_REPLICATED} local=${RELEASE_STORAGE_CLASS_LOCAL} nfs=${RELEASE_STORAGE_CLASS_NFS}"
}

run_suite() {
  # Same settings as run-release-e2e.sh: serial, migration limits untouched,
  # nothing deleted (the cleanup phase does that).
  # RUNNER_TEMP is where the framework saves the dumps of a failed spec.
  local ginkgo_args=()
  read -r -a ginkgo_args <<< "${GINKGO_ARGS:-}"

  (
    cd "${E2E_DIR}"
    E2E_CONFIG="${E2E_CONFIG:-${E2E_DIR}/default_config.yaml}" \
    RUNNER_TEMP="${STATE_DIR}" \
    MIGRATION_LIMITS_PRECHECK=no \
    POST_CLEANUP=never \
      go tool ginkgo -v --race --timeout=45m "${ginkgo_args[@]}" ./release
  )
}

phase_pre() {
  log "Cluster: $(kubectl config current-context)"
  kubectl get modules virtualization

  if [ -f "${CONTEXT_PATH}" ]; then
    fail "${CONTEXT_PATH} is left from a previous run in namespace $(release_namespace): run cleanup first"
  fi
  mkdir -p "${STATE_DIR}"
  rm -f "${MIGRATES_VMS_PATH}"

  precheck_storage_classes
  RELEASE_TEST_PHASE=pre-upgrade \
  RELEASE_UPGRADE_CONTEXT_PATH="${CONTEXT_PATH}" \
    run_suite

  log "Upgrade context:"
  cat "${CONTEXT_PATH}"
}

# Waits until the expected number of Evict VMOPs exists in the namespace and
# every one of them is Completed or Failed.
wait_evict_vmops_terminal() {
  local namespace="$1"
  local expected="$2"
  local deadline=$(( $(date +%s) + ${TIMEOUT_SECONDS:-1200} ))
  local phases total terminal

  while true; do
    phases="$(kubectl -n "${namespace}" get vmop \
      -o jsonpath='{range .items[?(@.spec.type=="Evict")]}{.status.phase}{"\n"}{end}')"
    total="$(grep -c . <<< "${phases}" || true)"
    terminal="$(grep -cE '^(Completed|Failed)$' <<< "${phases}" || true)"

    if [ "${total}" -eq "${expected}" ] && [ "${terminal}" -eq "${expected}" ]; then
      log "All ${expected} Evict VMOP(s) are terminal"
      kubectl -n "${namespace}" get vmop
      return 0
    fi
    if [ "$(date +%s)" -ge "${deadline}" ]; then
      kubectl -n "${namespace}" get vmop || true
      fail "Evict VMOPs did not settle in time: ${terminal}/${expected} terminal (${total}/${expected} found)"
    fi

    log "${terminal}/${expected} Evict VMOP(s) terminal (${total}/${expected} found), re-checking in 10s..."
    sleep 10
  done
}

create_evict_vmop() {
  local namespace="$1"
  local vm="$2"

  log "Evicting ${vm}"
  kubectl create -f - <<EOF
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualMachineOperation
metadata:
  generateName: local-evict-${vm}-
  namespace: ${namespace}
spec:
  type: Evict
  virtualMachineName: ${vm}
EOF
}

upgrade_evict() {
  local namespace="$1"
  local iperf_server phase vm created

  iperf_server="$(jq -r '.iperfServerVM' "${CONTEXT_PATH}")"

  # A module upgrade migrates the VMs some time after the iperf client
  # starts; the post phase checks that the iperf run brackets the migration.
  sleep 10

  # The post phase measures the migration window on the iperf server VM's
  # Evict, so without one there is nothing to check: stop here, not in post.
  phase="$(kubectl -n "${namespace}" get vm "${iperf_server}" -o jsonpath='{.status.phase}' 2>/dev/null || true)"
  [ "${phase}" = "Running" ] ||
    fail "the iperf server VM ${iperf_server} is ${phase:-missing}, not Running: nothing to migrate"
  create_evict_vmop "${namespace}" "${iperf_server}" ||
    fail "Evict of the iperf server VM ${iperf_server} was refused; the post phase needs its migration window"
  created=1

  for vm in $(kubectl -n "${namespace}" get vm \
    -o jsonpath='{range .items[?(@.status.phase=="Running")]}{.metadata.name}{"\n"}{end}'); do
    [ "${vm}" != "${iperf_server}" ] || continue
    # The webhook refuses some migrations (an RWO disk next to a hotplugged
    # one); the workload updater hits the same refusal during a real upgrade.
    if create_evict_vmop "${namespace}" "${vm}"; then
      created=$((created + 1))
    else
      echo "[WARN] Evict of ${vm} was refused"
    fi
  done

  wait_evict_vmops_terminal "${namespace}" "${created}"
  echo true > "${MIGRATES_VMS_PATH}"
}

upgrade_mpo() {
  local namespace="$1"
  local output

  : "${CURRENT_RELEASE:?CURRENT_RELEASE is required for UPGRADE_MODE=mpo}"
  : "${NEW_RELEASE:?NEW_RELEASE is required for UPGRADE_MODE=mpo}"
  : "${DEV_MODULE_SOURCE:?DEV_MODULE_SOURCE is required for UPGRADE_MODE=mpo}"
  require_tools crane tar

  log "Patching ModulePullOverride virtualization: ${CURRENT_RELEASE} -> ${NEW_RELEASE}"
  kubectl patch mpo virtualization --type merge -p "{\"spec\":{\"imageTag\":\"${NEW_RELEASE}\"}}"

  bash "${CI_SCRIPT_DIR}/verify-image-digests.sh"

  output="$(mktemp -p "${STATE_DIR}")"
  RELEASE_NAMESPACE="${namespace}" GITHUB_OUTPUT="${output}" \
    bash "${CI_SCRIPT_DIR}/wait-vmops-migration-terminal.sh"
  sed -n 's/^migrates_vms=//p' "${output}" > "${MIGRATES_VMS_PATH}"
  rm -f "${output}"
}

upgrade_manual() {
  log "Disrupt the VMs in namespace ${1} now, then press Enter"
  read -r
  echo "${RELEASE_UPGRADE_MIGRATES_VMS:-true}" > "${MIGRATES_VMS_PATH}"
}

phase_upgrade() {
  local namespace evicts
  namespace="$(release_namespace)"

  # One upgrade per pre run: the waits and the post phase count every Evict
  # VMOP in the namespace, so leftovers from an earlier upgrade would skew them.
  if [ -f "${MIGRATES_VMS_PATH}" ]; then
    fail "namespace ${namespace} has already been through the upgrade phase: run cleanup and pre again"
  fi
  evicts="$(kubectl -n "${namespace}" get vmop \
    -o jsonpath='{range .items[?(@.spec.type=="Evict")]}{.metadata.name}{"\n"}{end}')"
  if [ -n "${evicts}" ]; then
    fail "namespace ${namespace} already has Evict VMOPs (${evicts//$'\n'/ }): run cleanup and pre again"
  fi

  log "Upgrade mode: ${UPGRADE_MODE}, namespace: ${namespace}"
  case "${UPGRADE_MODE}" in
    evict)  upgrade_evict "${namespace}" ;;
    mpo)    upgrade_mpo "${namespace}" ;;
    manual) upgrade_manual "${namespace}" ;;
    none)   echo false > "${MIGRATES_VMS_PATH}" ;;
    *)      fail "unsupported UPGRADE_MODE: ${UPGRADE_MODE}" ;;
  esac
}

phase_post() {
  local namespace
  namespace="$(release_namespace)"
  [ -f "${MIGRATES_VMS_PATH}" ] || fail "no ${MIGRATES_VMS_PATH}: run the upgrade phase first"

  precheck_storage_classes
  RELEASE_TEST_PHASE=post-upgrade \
  RELEASE_NAMESPACE="${namespace}" \
  RELEASE_UPGRADE_MIGRATES_VMS="$(cat "${MIGRATES_VMS_PATH}")" \
    run_suite
}

phase_cleanup() {
  local namespace

  # A pre phase that failed before saving the context leaves its Project
  # without a record; it is not deleted blindly, others may share the cluster.
  if [ ! -f "${CONTEXT_PATH}" ]; then
    log "No upgrade context at ${CONTEXT_PATH}. Release Projects in the cluster:"
    kubectl get project -o name | grep '/v12n-e2e-release-' || echo "  none"
    return 0
  fi
  namespace="$(release_namespace)"

  # The Project owns the namespace: deleting it takes every resource along.
  log "Deleting Project ${namespace}"
  kubectl delete project "${namespace}" --ignore-not-found --wait=true --timeout=10m
  rm -f "${CONTEXT_PATH}" "${MIGRATES_VMS_PATH}"
}

require_tools kubectl jq go

case "${1:-all}" in
  pre)     phase_pre ;;
  upgrade) phase_upgrade ;;
  post)    phase_post ;;
  cleanup) phase_cleanup ;;
  all)
    phase_pre
    phase_upgrade
    phase_post
    if [ "${KEEP:-no}" = "yes" ]; then
      log "KEEP=yes: leaving namespace $(release_namespace); run '$0 cleanup' to delete it"
    else
      phase_cleanup
    fi
    ;;
  *) fail "usage: $(basename -- "$0") [pre|upgrade|post|cleanup|all]" ;;
esac
