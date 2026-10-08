#!/usr/bin/env bash

show_deckhouse_state() {
  echo "[DEBUG] Show deckhouse pods"
  kubectl -n d8-system get pods -l app=deckhouse -o wide || true
  echo "[DEBUG] Show queue (first 25 lines)"
  d8 s queue list | head -n25 || true
}

d8_queue_list() {
  d8 s queue list | grep -Po '([0-9]+)(?= active)' || echo "[WARNING] Failed to retrieve list queue"
}

d8_queue() {
  local count=90
  local delay=10
  local queue_count

  for i in $(seq 1 "$count"); do
    queue_count="$(d8_queue_list)"
    if [ -n "$queue_count" ] && [ "$queue_count" = "0" ]; then
      echo "[SUCCESS] Queue is clear"
      return 0
    fi

    echo "[INFO] Wait until queues are empty ${i}/${count}"
    if (( i % 5 == 0 )); then
      echo "[INFO] Show queue list"
      d8 s queue list | head -n25 || echo "[WARNING] Failed to retrieve list queue"
      echo " "
    fi

    if (( i % 10 == 0 )); then
      echo "[INFO] deckhouse logs"
      echo "::group::deckhouse logs"
      d8 s logs | tail -n 100
      echo "::endgroup::"
      echo " "
    fi

    if [ "$i" -lt "$count" ]; then
      sleep "$delay"
    fi
  done

  echo "[ERROR] Deckhouse queue is not clear after ${count} attempts"
  return 1
}

wait_for_deckhouse_queue() {
  local count=60
  local delay=10
  local queue_count

  for i in $(seq 1 "$count"); do
    queue_count="$(d8 s queue list | grep -Po '([0-9]+)(?= active)' || true)"
    echo "[INFO] Wait until Deckhouse queue is empty ${i}/${count}, active=${queue_count:-unknown}"

    if [ "$queue_count" = "0" ]; then
      echo "[SUCCESS] Deckhouse queue is empty"
      return 0
    fi

    if (( i % 5 == 0 )); then
      show_deckhouse_state
    fi

    if [ "$i" -lt "$count" ]; then
      sleep "$delay"
    fi
  done

  echo "[ERROR] Deckhouse queue is not empty"
  show_deckhouse_state
  return 1
}

# The helpers below need common.sh sourced first (kubectl_apply_with_retry,
# registry_host_from_docker_cfg, modules_repo_for_registry).

# ModuleSource of the dev registry. It serves the virtualization build under
# test and any module pinned to a merge request build via ModulePullOverride.
DEV_MODULE_SOURCE="deckhouse-dev"

show_modulesource_status() {
  local ms_json
  local phase
  local message

  if ! ms_json="$(kubectl get ms "${DEV_MODULE_SOURCE}" -o json 2>/dev/null)"; then
    echo "[DEBUG] ModuleSource ${DEV_MODULE_SOURCE} is not found"
    return 0
  fi

  phase="$(jq -r '.status.phase // "unknown"' <<< "$ms_json")"
  message="$(jq -r '.status.message // ""' <<< "$ms_json")"

  echo "[DEBUG] ModuleSource ${DEV_MODULE_SOURCE} phase: ${phase}"
  if echo "$message" | grep -Eqi '401 Unauthorized|Auth failed'; then
    echo "[DEBUG] ModuleSource ${DEV_MODULE_SOURCE} problem: registry authentication failed (401 Unauthorized)"
  fi
}

# Applies the dev ModuleSource from a base64-encoded dockerconfigjson.
# Usage: apply_dev_module_source <docker_cfg>
apply_dev_module_source() {
  local docker_cfg="$1"
  local registry
  registry="$(registry_host_from_docker_cfg "$docker_cfg")"

  echo "[INFO] Apply ModuleSource ${DEV_MODULE_SOURCE} config"
  kubectl_apply_with_retry 20 10 show_deckhouse_state <<EOF
apiVersion: deckhouse.io/v1alpha1
kind: ModuleSource
metadata:
  name: ${DEV_MODULE_SOURCE}
spec:
  registry:
    ca: ""
    dockerCfg: "${docker_cfg}"
    repo: "$(modules_repo_for_registry "${registry}")"
    scheme: HTTPS
EOF
}

wait_for_modulesource_active() {
  local count=30
  local delay=10
  local ms_json
  local phase
  local message

  for i in $(seq 1 "$count"); do
    ms_json="$(kubectl get ms "${DEV_MODULE_SOURCE}" -o json 2>/dev/null || true)"
    phase="$(jq -r '.status.phase // "unknown"' <<< "$ms_json" 2>/dev/null || true)"
    message="$(jq -r '.status.message // ""' <<< "$ms_json" 2>/dev/null || true)"

    echo "[INFO] Wait for ModuleSource ${DEV_MODULE_SOURCE} to be Active ${i}/${count}, phase=${phase:-unknown}"
    if echo "$message" | grep -Eqi '401 Unauthorized|Auth failed'; then
      echo "[INFO] ModuleSource ${DEV_MODULE_SOURCE} problem: registry authentication failed (401 Unauthorized)"
    fi

    if [ "$phase" = "Active" ]; then
      echo "[SUCCESS] ModuleSource ${DEV_MODULE_SOURCE} is Active"
      kubectl get ms "${DEV_MODULE_SOURCE}" -o wide
      return 0
    fi

    if echo "$message" | grep -Eqi '401 Unauthorized|Auth failed'; then
      echo "[ERROR] ModuleSource ${DEV_MODULE_SOURCE} registry authentication failed. Check DEV_REGISTRY_DOCKER_CFG credentials." >&2
      return 1
    fi

    if (( i % 5 == 0 )); then
      show_deckhouse_state
    fi

    if [ "$i" -lt "$count" ]; then
      sleep "$delay"
    fi
  done

  echo "[ERROR] ModuleSource ${DEV_MODULE_SOURCE} did not become Active"
  show_modulesource_status
  show_deckhouse_state
  return 1
}

# Waits until the dev ModuleSource lists the module, i.e. the module can be
# installed from it.
# Usage: wait_for_module_dev_source <module>
wait_for_module_dev_source() {
  local module="$1"
  local count=60
  local delay=10
  local available_sources

  for i in $(seq 1 "$count"); do
    available_sources="$(kubectl get modules "${module}" -o json 2>/dev/null | jq -r '.properties.availableSources // [] | join(",")' || true)"
    echo "[INFO] Wait for ${module} module source ${DEV_MODULE_SOURCE} ${i}/${count}, availableSources=${available_sources:-none}"

    if echo ",${available_sources}," | grep -q ",${DEV_MODULE_SOURCE},"; then
      echo "[SUCCESS] ${DEV_MODULE_SOURCE} is available for ${module} module"
      kubectl get modules "${module}" -o wide
      return 0
    fi

    if (( i % 5 == 0 )); then
      echo "[DEBUG] Show ModuleSource"
      show_modulesource_status
      echo "[DEBUG] Show ${module} module"
      kubectl get modules "${module}" -o yaml || true
      show_deckhouse_state
    fi

    if [ "$i" -lt "$count" ]; then
      sleep "$delay"
    fi
  done

  echo "[ERROR] ${DEV_MODULE_SOURCE} did not become available for ${module} module"
  show_modulesource_status
  kubectl get modules "${module}" -o yaml || true
  return 1
}
