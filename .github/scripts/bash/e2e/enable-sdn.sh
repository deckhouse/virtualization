#!/usr/bin/env bash

set -Eeuo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=.github/scripts/bash/e2e/common.sh
source "${SCRIPT_DIR}/common.sh"

show_sdn_state() {
  echo "[DEBUG] Module sdn"
  kubectl get modules sdn -o wide || true
  echo "[DEBUG] ModuleConfig sdn"
  kubectl get mc sdn -o yaml || true
  echo "[DEBUG] d8-sdn resources"
  kubectl -n d8-sdn get pods,deploy,ds,svc,endpoints || true
}

apply_sdn_module_config() {
  local count=12
  local delay=10

  for i in $(seq 1 "$count"); do
    echo "[INFO] Apply SDN ModuleConfig attempt ${i}/${count}"
    if kubectl apply -f - <<'EOF'
apiVersion: deckhouse.io/v1alpha1
kind: ModuleConfig
metadata:
  name: sdn
spec:
  enabled: true
EOF
    then
      echo "[SUCCESS] SDN ModuleConfig applied"
      kubectl get mc sdn
      return 0
    fi

    if [ "$i" -lt "$count" ]; then
      echo "[WARN] Failed to apply SDN ModuleConfig, retrying in ${delay} seconds..."
      show_sdn_state
      sleep "$delay"
    fi
  done

  echo "[ERROR] Failed to apply SDN ModuleConfig after ${count} attempts"
  show_sdn_state
  d8 s logs | tail -n 100 || true
  return 1
}

wait_for_sdn_module() {
  local count=60
  local delay=5
  local phase

  for i in $(seq 1 "$count"); do
    phase="$(kubectl get modules sdn -o jsonpath='{.status.phase}' 2>/dev/null || true)"
    echo "[INFO] Wait for modules/sdn to be Ready ${i}/${count}, phase=${phase:-unknown}"

    if [ "$phase" = "Ready" ]; then
      kubectl get modules sdn -o wide
      return 0
    fi

    if (( i % 5 == 0 )); then
      show_sdn_state
    fi

    if [ "$i" -lt "$count" ]; then
      sleep "$delay"
    fi
  done

  echo "[ERROR] modules/sdn is not Ready"
  show_sdn_state
  d8 s logs | tail -n 100 || true
  return 1
}

wait_for_sdn_workloads() {
  local timeout=600
  echo "[INFO] Wait for sdn deployments to be ready, timeout: ${timeout}s"
  kubectl -n d8-sdn wait --for=condition=Available deploy --all --timeout="${timeout}s"
  echo "[INFO] Wait for sdn daemonset agent to be ready, timeout: ${timeout}s"
  kubectl -n d8-sdn rollout status daemonset agent --timeout="${timeout}s"
}

wait_for_sdn_admission_endpoint() {
  local count=60
  local delay=5
  local endpoints

  for i in $(seq 1 "$count"); do
    endpoints="$(kubectl -n d8-sdn get endpoints controller-sdn-admission -o jsonpath='{.subsets[*].addresses[*].ip}' 2>/dev/null || true)"
    echo "[INFO] Wait for controller-sdn-admission endpoints ${i}/${count}, endpoints=${endpoints:-none}"

    if [ -n "$endpoints" ]; then
      kubectl -n d8-sdn get svc,endpoints controller-sdn-admission
      return 0
    fi

    if (( i % 5 == 0 )); then
      show_sdn_state
    fi

    if [ "$i" -lt "$count" ]; then
      sleep "$delay"
    fi
  done

  echo "[ERROR] controller-sdn-admission endpoints are not ready"
  show_sdn_state
  return 1
}

echo "[INFO] Enable SDN"
apply_sdn_module_config
wait_for_sdn_module
wait_for_sdn_workloads
wait_for_sdn_admission_endpoint
echo "[SUCCESS] Done"
