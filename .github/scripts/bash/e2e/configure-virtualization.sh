#!/usr/bin/env bash

set -Eeuo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=.github/scripts/bash/e2e/common.sh
source "${SCRIPT_DIR}/common.sh"
# shellcheck source=.github/scripts/bash/e2e/deckhouse.sh
source "${SCRIPT_DIR}/deckhouse.sh"

require_env DEV_REGISTRY_DOCKER_CFG
require_env NESTED_STORAGE_CLASS_NAME
require_env VIRTUALIZATION_TAG

# shellcheck disable=SC2153,SC2154
dev_registry_docker_cfg="${DEV_REGISTRY_DOCKER_CFG}"
# shellcheck disable=SC2153,SC2154
nested_storage_class_name="${NESTED_STORAGE_CLASS_NAME}"
# shellcheck disable=SC2153,SC2154
virtualization_tag="${VIRTUALIZATION_TAG}"

show_modulesource_status() {
  local ms_json
  local phase
  local message

  if ! ms_json="$(kubectl get ms deckhouse-dev -o json 2>/dev/null)"; then
    echo "[DEBUG] ModuleSource deckhouse-dev is not found"
    return 0
  fi

  phase="$(jq -r '.status.phase // "unknown"' <<< "$ms_json")"
  message="$(jq -r '.status.message // ""' <<< "$ms_json")"

  echo "[DEBUG] ModuleSource deckhouse-dev phase: ${phase}"
  if echo "$message" | grep -Eqi '401 Unauthorized|Auth failed'; then
    echo "[DEBUG] ModuleSource deckhouse-dev problem: registry authentication failed (401 Unauthorized)"
  fi
}

wait_for_modulesource_active() {
  local count=30
  local delay=10
  local ms_json
  local phase
  local message

  for i in $(seq 1 "$count"); do
    ms_json="$(kubectl get ms deckhouse-dev -o json 2>/dev/null || true)"
    phase="$(jq -r '.status.phase // "unknown"' <<< "$ms_json" 2>/dev/null || true)"
    message="$(jq -r '.status.message // ""' <<< "$ms_json" 2>/dev/null || true)"

    echo "[INFO] Wait for ModuleSource deckhouse-dev to be Active ${i}/${count}, phase=${phase:-unknown}"
    if echo "$message" | grep -Eqi '401 Unauthorized|Auth failed'; then
      echo "[INFO] ModuleSource deckhouse-dev problem: registry authentication failed (401 Unauthorized)"
    fi

    if [ "$phase" = "Active" ]; then
      echo "[SUCCESS] ModuleSource deckhouse-dev is Active"
      kubectl get ms deckhouse-dev -o wide
      return 0
    fi

    if echo "$message" | grep -Eqi '401 Unauthorized|Auth failed'; then
      echo "[ERROR] ModuleSource deckhouse-dev registry authentication failed. Check DEV_REGISTRY_DOCKER_CFG credentials." >&2
      return 1
    fi

    if (( i % 5 == 0 )); then
      show_deckhouse_state
    fi

    if [ "$i" -lt "$count" ]; then
      sleep "$delay"
    fi
  done

  echo "[ERROR] ModuleSource deckhouse-dev did not become Active"
  show_modulesource_status
  show_deckhouse_state
  return 1
}

wait_for_virtualization_dev_source() {
  local count=60
  local delay=10
  local available_sources

  for i in $(seq 1 "$count"); do
    available_sources="$(kubectl get modules virtualization -o json 2>/dev/null | jq -r '.properties.availableSources // [] | join(",")' || true)"
    echo "[INFO] Wait for virtualization module source deckhouse-dev ${i}/${count}, availableSources=${available_sources:-none}"

    if echo ",${available_sources}," | grep -q ",deckhouse-dev,"; then
      echo "[SUCCESS] deckhouse-dev is available for virtualization module"
      kubectl get modules virtualization -o wide
      return 0
    fi

    if (( i % 5 == 0 )); then
      echo "[DEBUG] Show ModuleSource"
      show_modulesource_status
      echo "[DEBUG] Show virtualization module"
      kubectl get modules virtualization -o yaml || true
      show_deckhouse_state
    fi

    if [ "$i" -lt "$count" ]; then
      sleep "$delay"
    fi
  done

  echo "[ERROR] deckhouse-dev did not become available for virtualization module"
  show_modulesource_status
  kubectl get modules virtualization -o yaml || true
  return 1
}

apply_module_source() {
  local registry
  registry="$(registry_host_from_docker_cfg "$dev_registry_docker_cfg")"

  echo "[INFO] Apply ModuleSource dev config"
  kubectl_apply_with_retry 20 10 show_deckhouse_state <<EOF
apiVersion: deckhouse.io/v1alpha1
kind: ModuleSource
metadata:
  name: deckhouse-dev
spec:
  registry:
    ca: ""
    dockerCfg: "${dev_registry_docker_cfg}"
    repo: "${registry}/sys/deckhouse-oss/modules"
    scheme: HTTPS
EOF
}

apply_virtualization_module_config() {
  echo "[INFO] Apply Virtualization module config"
  kubectl_apply_with_retry 20 10 show_deckhouse_state <<EOF
apiVersion: deckhouse.io/v1alpha1
kind: ModuleConfig
metadata:
  name: virtualization
spec:
  enabled: true
  settings:
    dvcr:
      storage:
        persistentVolumeClaim:
          size: 10Gi
          storageClassName: ${nested_storage_class_name}
        type: PersistentVolumeClaim
    virtualMachineCIDRs:
      - 192.168.10.0/24
  source: deckhouse-dev
  version: 1
---
apiVersion: deckhouse.io/v1alpha2
kind: ModulePullOverride
metadata:
  name: virtualization
spec:
  imageTag: ${virtualization_tag}
  scanInterval: 120h
EOF
}

show_virtualization_config() {
  echo "[INFO] Show ModuleSource"
  kubectl get ms

  echo "[INFO] Show module config virtualization info"
  kubectl get mc virtualization

  echo "[INFO] Show ModulePullOverride virtualization info"
  kubectl get mpo virtualization
}

apply_module_source
wait_for_modulesource_active
wait_for_deckhouse_queue
wait_for_virtualization_dev_source
wait_for_deckhouse_queue
apply_virtualization_module_config
show_virtualization_config
