#!/usr/bin/env bash
set -euo pipefail

# nested_diag.sh — диагностика nested‑кластеров с родителя.
# Требует: kubectl, jq. Параметр --deep использует d8 (опционально).
#
# Usage:
#   nested_diag.sh [--prefix PREFIX] [--limit N] [--namespaces ns1,ns2] [--deep] [--ssh-user USER] [--ssh-key PATH]
# Defaults:
#   --prefix nightly-nested-e2e-
#   --limit  2 (игнорируется при --namespaces)
#   --deep   добавляет проверки внутри nested (Deckhouse/Ceph/SDS)

PREFIX="nightly-nested-e2e-"
LIMIT=2
NAMESPACES=""
DEEP=0
SSH_USER="ubuntu"
SSH_KEY=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --prefix)   PREFIX=${2:-$PREFIX}; shift 2 ;;
    --limit)    LIMIT=${2:-$LIMIT}; shift 2 ;;
    --namespaces|--ns) NAMESPACES=${2:-""}; shift 2 ;;
    --deep)     DEEP=1; shift ;;
    --ssh-user) SSH_USER=${2:-""}; shift 2 ;;
    --ssh-key)  SSH_KEY=${2:-""}; shift 2 ;;
    -h|--help)
      echo "Usage: $0 [--prefix PREFIX] [--limit N] [--namespaces ns1,ns2] [--deep] [--ssh-user USER] [--ssh-key PATH]"
      exit 0 ;;
    *) echo "Unknown arg: $1" >&2; exit 1 ;;
  esac
done

need_bin() { command -v "$1" >/dev/null 2>&1 || { echo "ERR: '$1' not found" >&2; exit 1; }; }
need_bin kubectl
need_bin jq

indent() { sed 's/^/  /'; }
warn() { echo "[WARN] $*"; }

# Try to obtain SSH private key from Secret e2e-ssh-key in a namespace when --ssh-key not provided
ensure_ns_ssh_key() {
  local ns=$1
  if [[ -n "$SSH_KEY" && -f "$SSH_KEY" ]]; then
    return 0
  fi
  if ! kubectl -n "$ns" get secret e2e-ssh-key >/dev/null 2>&1; then
    warn "Secret e2e-ssh-key not found in namespace $ns; provide --ssh-key explicitly for --deep"
    return 1
  fi
  local b64
  b64=$(kubectl -n "$ns" get secret e2e-ssh-key -o jsonpath='{.data.cloud}' 2>/dev/null || true)
  if [[ -z "$b64" || "$b64" == "null" ]]; then
    warn "Secret e2e-ssh-key in $ns has no 'cloud' key (private key)"
    return 1
  fi
  local tmpk
  tmpk=$(mktemp -t nested-ssh.XXXXXX)
  echo "$b64" | base64 -d > "$tmpk" 2>/dev/null || echo "$b64" | base64 -D > "$tmpk" 2>/dev/null || {
    rm -f "$tmpk"; warn "Failed to decode private key from Secret e2e-ssh-key in $ns"; return 1;
  }
  chmod 600 "$tmpk"
  SSH_KEY="$tmpk"
  echo "[INFO] Using SSH key from Secret e2e-ssh-key in $ns: $SSH_KEY"
}

# Helper to run d8 v ssh with provided credentials and safe SSH opts
d8_ssh() {
  local host=$1; shift
  local args=(v ssh --local-ssh=true)
  # Pass each -o option separately to avoid ssh parsing errors
  args+=(--local-ssh-opts "-o StrictHostKeyChecking=no")
  args+=(--local-ssh-opts "-o UserKnownHostsFile=/dev/null")
  # Prefer explicit identity usage if key provided
  args+=(--local-ssh-opts "-o IdentitiesOnly=yes")
  [[ -n "$SSH_USER" ]] && args+=(--username="$SSH_USER")
  [[ -n "$SSH_KEY" ]] && args+=(--identity-file="$SSH_KEY")
  d8 "${args[@]}" "$host" "$@"
}

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
  # Ensure SSH key available (from flag or Secret in this ns)
  ensure_ns_ssh_key "$ns" || true
  
  # Ensure d8 CLI (with queue support) on the master VM
  echo "[DEEP] Ensuring d8 CLI is available on master VM..."
  d8_ssh "${master}.${ns}" -c '
    set -eu
    REQUIRED_D8_VER="v0.13.2"
    install_pinned() {
      echo "Installing d8 CLI ${REQUIRED_D8_VER}..."
      # Ensure curl
      if ! command -v curl >/dev/null 2>&1; then
        if command -v apt-get >/dev/null 2>&1; then
          sudo apt-get update -qq && sudo apt-get install -y -qq curl ca-certificates
        elif command -v apk >/dev/null 2>&1; then
          sudo apk add --no-cache curl ca-certificates
        elif command -v dnf >/dev/null 2>&1; then
          sudo dnf install -y -q curl ca-certificates
        elif command -v yum >/dev/null 2>&1; then
          sudo yum install -y -q curl ca-certificates
        fi
      fi
      # Ensure tar
      if ! command -v tar >/dev/null 2>&1; then
        if command -v apt-get >/dev/null 2>&1; then
          sudo apt-get update -qq && sudo apt-get install -y -qq tar
        elif command -v apk >/dev/null 2>&1; then
          sudo apk add --no-cache tar
        elif command -v dnf >/dev/null 2>&1; then
          sudo dnf install -y -q tar
        elif command -v yum >/dev/null 2>&1; then
          sudo yum install -y -q tar
        fi
      fi
      if ! command -v tar >/dev/null 2>&1; then
        echo "WARNING: 'tar' is not available, cannot install d8 CLI automatically" >&2
        return
      fi
      cd /tmp
      curl -fsSL -o d8.tgz "https://deckhouse.io/downloads/deckhouse-cli/${REQUIRED_D8_VER}/d8-${REQUIRED_D8_VER}-linux-amd64.tar.gz"
      tar -xzf d8.tgz linux-amd64/bin/d8
      sudo mv linux-amd64/bin/d8 /usr/local/bin/d8
      sudo chmod +x /usr/local/bin/d8
      rm -rf d8.tgz linux-amd64
    }
    if ! command -v d8 >/dev/null 2>&1; then
      install_pinned
    else
      if ! d8 platform queue --help >/dev/null 2>&1; then
        echo "Upgrading d8 CLI to ${REQUIRED_D8_VER} for queue support..."
        install_pinned
      else
        echo "d8 CLI already installed and supports queue"
      fi
    fi
    d8 --version || true
  ' 2>&1 || warn "Failed to prepare d8 CLI on master VM"
  
  echo "[DEEP] Platform queue list"
  d8_ssh "${master}.${ns}" -c '
    if command -v d8 >/dev/null 2>&1; then
      KCFG=""
      if [ -f /etc/kubernetes/admin.conf ]; then
        KCFG=/etc/kubernetes/admin.conf
      elif [ -f /var/lib/bashible/bootstrap/control-plane/kubeconfig ]; then
        KCFG=/var/lib/bashible/bootstrap/control-plane/kubeconfig
      elif [ -f /var/lib/bashible/bootstrap/kubeconfig ]; then
        KCFG=/var/lib/bashible/bootstrap/kubeconfig
      fi
      # If not found, derive kubeconfig from current cluster context via kubectl
      if [ -z "$KCFG" ] && command -v /opt/deckhouse/bin/kubectl >/dev/null 2>&1; then
        TMPKCFG="/tmp/d8-kubeconfig"
        # Try to render raw kubeconfig from in-cluster kubectl
        if sudo /opt/deckhouse/bin/kubectl config view --raw > "$TMPKCFG" 2>/dev/null; then
          sudo chmod 600 "$TMPKCFG" || true
          KCFG="$TMPKCFG"
        fi
      fi
      if [ -n "$KCFG" ]; then
        sudo -E env KUBECONFIG="$KCFG" d8 platform queue list --output json 2>/dev/null || \
        sudo -E env KUBECONFIG="$KCFG" d8 platform queue list
      else
        d8 platform queue list --output json 2>/dev/null || d8 platform queue list
      fi
    else
      echo "d8 CLI not installed on VM; skipping platform queue"
    fi
  ' 2>&1 | grep -v "The authenticity of host" | indent || true
  echo "[DEEP] Nested nodes"
  d8_ssh "${master}.${ns}" -c 'sudo /opt/deckhouse/bin/kubectl get nodes -o wide' 2>/dev/null | indent || true
  echo "[DEEP] NodeGroups"
  d8_ssh "${master}.${ns}" -c 'sudo /opt/deckhouse/bin/kubectl get nodegroup -o wide' 2>/dev/null | indent || true
  echo "[DEEP] d8-cloud-provider-dvp pods"
  d8_ssh "${master}.${ns}" -c 'sudo /opt/deckhouse/bin/kubectl -n d8-cloud-provider-dvp get pods' 2>/dev/null | indent || true
  echo "[DEEP] d8-cloud-provider-dvp recent logs"
  d8_ssh "${master}.${ns}" -c 'sudo /opt/deckhouse/bin/kubectl -n d8-cloud-provider-dvp logs -l app=cloud-provider-dvp --tail=120' 2>/dev/null | indent || true
  echo "[DEEP] d8-cloud-instance-manager pods"
  d8_ssh "${master}.${ns}" -c 'sudo /opt/deckhouse/bin/kubectl -n d8-cloud-instance-manager get pods' 2>/dev/null | indent || true
  echo "[DEEP] d8-cloud-instance-manager recent events"
  d8_ssh "${master}.${ns}" -c 'sudo /opt/deckhouse/bin/kubectl -n d8-cloud-instance-manager get events --sort-by=.lastTimestamp | tail -n 30' 2>/dev/null | indent || true
  echo "[DEEP] Default SC (mc.global)"
  d8_ssh "${master}.${ns}" -c 'sudo /opt/deckhouse/bin/kubectl get mc global -o jsonpath="{.spec.settings.defaultClusterStorageClass}{"\n"}"' 2>/dev/null | indent || true
  echo "[DEEP] Default SC object"
  d8_ssh "${master}.${ns}" -c 'sudo /opt/deckhouse/bin/kubectl get sc -o json | jq -r ".items[] | select(.metadata.annotations[\"storageclass.kubernetes.io/is-default-class\"]==\"true\") | .metadata.name"' 2>/dev/null | indent || true
  echo "[DEEP] Ceph namespace / rook-ceph-operator"
  d8_ssh "${master}.${ns}" -c 'sudo /opt/deckhouse/bin/kubectl get ns d8-operator-ceph || true; sudo /opt/deckhouse/bin/kubectl -n d8-operator-ceph get pods -o wide 2>/dev/null || true' 2>/dev/null | indent
  echo "[DEEP] Ceph SC presence"
  d8_ssh "${master}.${ns}" -c 'sudo /opt/deckhouse/bin/kubectl get sc | grep -E "ceph-pool-r2-csi-rbd" || true' 2>/dev/null | indent || true
  echo "[DEEP] SDS CRDs (if any)"
  d8_ssh "${master}.${ns}" -c 'sudo /opt/deckhouse/bin/kubectl get crd | grep -E "lvmvolumegroups|replicatedstorage(pools|classes)" || true' 2>/dev/null | indent || true
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
