#!/usr/bin/env bash
set -euo pipefail

# nested_diag.sh — диагностика nested‑кластеров с родителя.
# Требует: kubectl, jq. Параметр --deep использует d8 (опционально).
#
# Usage:
#   nested_diag.sh [--prefix PREFIX] [--limit N] [--namespaces ns1,ns2] [--deep]
# Defaults:
#   --prefix nightly-nested-e2e-
#   --limit  2 (игнорируется при --namespaces)
#   --deep   добавляет проверки внутри nested (Deckhouse/Ceph/SDS)

PREFIX="nightly-nested-e2e-"
LIMIT=2
NAMESPACES=""
DEEP=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --prefix)   PREFIX=${2:-$PREFIX}; shift 2 ;;
    --limit)    LIMIT=${2:-$LIMIT}; shift 2 ;;
    --namespaces|--ns) NAMESPACES=${2:-""}; shift 2 ;;
    --deep)     DEEP=1; shift ;;
    -h|--help)
      echo "Usage: $0 [--prefix PREFIX] [--limit N] [--namespaces ns1,ns2] [--deep]"
      exit 0 ;;
    *) echo "Unknown arg: $1" >&2; exit 1 ;;
  esac
done

need_bin() { command -v "$1" >/dev/null 2>&1 || { echo "ERR: '$1' not found" >&2; exit 1; }; }
need_bin kubectl
need_bin jq

indent() { sed 's/^/  /'; }
warn() { echo "[WARN] $*"; }

list_ns() {
  if [[ -n "$NAMESPACES" ]]; then
    IFS=',' read -r -a arr <<< "$NAMESPACES"
    printf '%s\n' "${arr[@]}"
    return
  fi
  # Найти последние ns по префиксу и наличию jump-host
  mapfile -t all_ns < <(kubectl get ns -o json \
    | jq -r --arg pfx "$PREFIX" '.items | sort_by(.metadata.creationTimestamp) | reverse
      | .[].metadata.name | select(startswith($pfx))')
  sel=()
  for ns in "${all_ns[@]:-}"; do
    [[ -z "$ns" ]] && continue
    if kubectl -n "$ns" get deploy jump-host >/dev/null 2>&1; then
      sel+=("$ns")
    fi
    [[ ${#sel[@]} -ge $LIMIT ]] && break
  done
  if [[ ${#sel[@]} -eq 0 ]]; then
    echo "ERR: no namespaces with prefix '$PREFIX' and jump-host found" >&2
    exit 1
  fi
  printf '%s\n' "${sel[@]}"
}

summ_vm() {
  local ns=$1
  if ! kubectl -n "$ns" get vm >/dev/null 2>&1; then
    echo "(no VirtualMachine resources)"
    return
  fi
  kubectl -n "$ns" get vm -o custom-columns=NAME:.metadata.name,PHASE:.status.phase,NODE:.status.nodeName,IP:.status.ipAddress --no-headers 2>/dev/null || true
}

summ_vmbda() {
  local ns=$1
  if ! kubectl -n "$ns" get virtualmachineblockdeviceattachments >/dev/null 2>&1; then
    echo "(no VMBDA resources)"
    return
  fi
  kubectl -n "$ns" get virtualmachineblockdeviceattachments -o json \
    | jq -r '.items[] | [
      .metadata.name,
      .spec.virtualMachineName,
      (.status.phase // "-"),
      ((.status.conditions // []) | map(select(.type=="Attached"))[0]?.status // "-"),
      ((.status.conditions // []) | map(select(.type=="Attached"))[0]?.reason // "-"),
      ((.status.conditions // []) | map(select(.type=="Attached"))[0]?.message // "-")
    ] | @tsv'
}

summ_vd() {
  local ns=$1
  if ! kubectl -n "$ns" get vd >/dev/null 2>&1; then
    echo "(no VirtualDisk resources)"
    return
  fi
  mapfile -t vds < <(kubectl -n "$ns" get vd -o json | jq -r '.items[].metadata.name')
  for vd in "${vds[@]:-}"; do
    [[ -z "$vd" ]] && continue
    local phase sc pvc pvc_phase vmode
    phase=$(kubectl -n "$ns" get vd "$vd" -o jsonpath='{.status.phase}' 2>/dev/null || true)
    sc=$(kubectl -n "$ns" get vd "$vd" -o jsonpath='{.spec.persistentVolumeClaim.storageClassName}' 2>/dev/null || true)
    pvc=$(kubectl -n "$ns" get vd "$vd" -o jsonpath='{.status.target.persistentVolumeClaimName}' 2>/dev/null || true)
    if [[ -n "${pvc:-}" ]]; then
      pvc_phase=$(kubectl -n "$ns" get pvc "$pvc" -o jsonpath='{.status.phase}' 2>/dev/null || true)
      vmode=$(kubectl -n "$ns" get pvc "$pvc" -o jsonpath='{.spec.volumeMode}' 2>/dev/null || true)
    else
      pvc_phase="-" ; vmode="-"
    fi
    printf "%s\t%s\t%s\t%s\t%s\n" "$vd" "${phase:-}-" "${sc:-}-" "${pvc:-}-" "${pvc_phase:-}-/${vmode:-}-"
  done
}

summ_sc_modes() {
  local ns=$1
  if ! kubectl -n "$ns" get vd >/dev/null 2>&1; then
    return
  fi
  mapfile -t scs < <(kubectl -n "$ns" get vd -o json | jq -r '.items[].spec.persistentVolumeClaim.storageClassName' | sort -u)
  for sc in "${scs[@]:-}"; do
    [[ -z "$sc" || "$sc" == "null" ]] && continue
    if kubectl get sc "$sc" >/dev/null 2>&1; then
      echo -n "$sc "
      kubectl get sc "$sc" -o jsonpath='{.volumeBindingMode}{"\t"}{.provisioner}{"\n"}'
    else
      echo "$sc (missing) -"
    fi
  done
}

deep_nested() {
  local ns=$1
  if ! command -v d8 >/dev/null 2>&1; then
    warn "d8 not found; skip --deep for $ns"
    return
  fi
  local master
  master=$(kubectl -n "$ns" get vm -l dvp.deckhouse.io/node-group=master -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true)
  if [[ -z "${master:-}" ]]; then
    warn "master VM not found in $ns; skip deep"
    return
  fi
  
  # Check and install d8 CLI on master VM if needed
  echo "[DEEP] Ensuring d8 CLI is available on master VM..."
  d8 v ssh --local-ssh=true "${master}.${ns}" -c '
    if ! command -v d8 >/dev/null 2>&1; then
      echo "Installing d8 CLI..."
      curl -fsSL -o /tmp/d8-install.sh https://raw.githubusercontent.com/deckhouse/deckhouse-cli/main/d8-install.sh
      bash /tmp/d8-install.sh
      rm -f /tmp/d8-install.sh
    else
      echo "d8 CLI already installed"
    fi
  ' 2>&1 || warn "Failed to install d8 CLI on master VM"
  
  echo "[DEEP] Platform queue list"
  d8 v ssh --local-ssh=true "${master}.${ns}" -c 'd8 platform queue list --output json' 2>&1 | grep -v "The authenticity of host" | indent || true
  echo "[DEEP] Nested nodes"
  d8 v ssh --local-ssh=true "${master}.${ns}" -c 'sudo /opt/deckhouse/bin/kubectl get nodes -o wide' 2>/dev/null | indent || true
  echo "[DEEP] NodeGroups"
  d8 v ssh --local-ssh=true "${master}.${ns}" -c 'sudo /opt/deckhouse/bin/kubectl get nodegroup -o wide' 2>/dev/null | indent || true
  echo "[DEEP] d8-cloud-provider-dvp pods"
  d8 v ssh --local-ssh=true "${master}.${ns}" -c 'sudo /opt/deckhouse/bin/kubectl -n d8-cloud-provider-dvp get pods' 2>/dev/null | indent || true
  echo "[DEEP] d8-cloud-provider-dvp recent logs"
  d8 v ssh --local-ssh=true "${master}.${ns}" -c 'sudo /opt/deckhouse/bin/kubectl -n d8-cloud-provider-dvp logs -l app=cloud-provider-dvp --tail=120' 2>/dev/null | indent || true
  echo "[DEEP] d8-cloud-instance-manager pods"
  d8 v ssh --local-ssh=true "${master}.${ns}" -c 'sudo /opt/deckhouse/bin/kubectl -n d8-cloud-instance-manager get pods' 2>/dev/null | indent || true
  echo "[DEEP] d8-cloud-instance-manager recent events"
  d8 v ssh --local-ssh=true "${master}.${ns}" -c 'sudo /opt/deckhouse/bin/kubectl -n d8-cloud-instance-manager get events --sort-by=.lastTimestamp | tail -n 30' 2>/dev/null | indent || true
  echo "[DEEP] Default SC (mc.global)"
  d8 v ssh --local-ssh=true "${master}.${ns}" -c 'sudo /opt/deckhouse/bin/kubectl get mc global -o jsonpath="{.spec.settings.defaultClusterStorageClass}{"\n"}"' 2>/dev/null | indent || true
  echo "[DEEP] Default SC object"
  d8 v ssh --local-ssh=true "${master}.${ns}" -c 'sudo /opt/deckhouse/bin/kubectl get sc -o json | jq -r ".items[] | select(.metadata.annotations[\"storageclass.kubernetes.io/is-default-class\"]==\"true\") | .metadata.name"' 2>/dev/null | indent || true
  echo "[DEEP] Ceph namespace / rook-ceph-operator"
  d8 v ssh --local-ssh=true "${master}.${ns}" -c 'sudo /opt/deckhouse/bin/kubectl get ns d8-operator-ceph || true; sudo /opt/deckhouse/bin/kubectl -n d8-operator-ceph get pods -o wide 2>/dev/null || true' 2>/dev/null | indent
  echo "[DEEP] Ceph SC presence"
  d8 v ssh --local-ssh=true "${master}.${ns}" -c 'sudo /opt/deckhouse/bin/kubectl get sc | grep -E "ceph-pool-r2-csi-rbd" || true' 2>/dev/null | indent || true
  echo "[DEEP] SDS CRDs (if any)"
  d8 v ssh --local-ssh=true "${master}.${ns}" -c 'sudo /opt/deckhouse/bin/kubectl get crd | grep -E "lvmvolumegroups|replicatedstorage(pools|classes)" || true' 2>/dev/null | indent || true
}

main() {
  echo "== Nested clusters diagnostics (parent context) =="
  mapfile -t nslist < <(list_ns)
  for ns in "${nslist[@]}"; do
    echo
    echo "=== Namespace: $ns ==="
    echo "[VM] VirtualMachines (name phase node ip)"
    summ_vm "$ns" | indent

    echo "[VMBDA] (name vm phase Attached.status reason message)"
    summ_vmbda "$ns" | indent

    echo "[VD] (name phase sc pvc pvcPhase/volMode)"
    summ_vd "$ns" | indent

    echo "[SC used by VDs] (name volumeBindingMode provisioner)"
    summ_sc_modes "$ns" | indent

    echo "[Jump-host]"
    kubectl -n "$ns" get deploy/jump-host svc/jump-host 2>/dev/null | indent || true

    if [[ "$DEEP" -eq 1 ]]; then
      deep_nested "$ns"
    fi
  done
}

main "$@"

