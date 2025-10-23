#!/usr/bin/env bash

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

SLEEP_TIME=5

# ===
LOG_FILE="vs-pvc-deploy_$(date +"%Y%m%d_%H%M%S").log"
# ===

# == datete functions ==
formatted_date() {
  local timestamp="$1"
  
  # Check if timestamp is valid (not empty and is a number)
  if [ -z "$timestamp" ] || ! [[ "$timestamp" =~ ^[0-9]+$ ]]; then
    # Use current time if timestamp is invalid
    date +"%H:%M:%S %d-%m-%Y"
    return
  fi
  
  # Use OS-specific date command
  case "$OS_TYPE" in
    "macOS")
      date -r "$timestamp" +"%H:%M:%S %d-%m-%Y" 2>/dev/null || date +"%H:%M:%S %d-%m-%Y"
      ;;
    "Linux")
      date -d "@$timestamp" +"%H:%M:%S %d-%m-%Y" 2>/dev/null || date +"%H:%M:%S %d-%m-%Y"
      ;;
    *)
      # Fallback - try both methods
      if date -r "$timestamp" +"%H:%M:%S %d-%m-%Y" 2>/dev/null; then
        # macOS style worked
        date -r "$timestamp" +"%H:%M:%S %d-%m-%Y"
      elif date -d "@$timestamp" +"%H:%M:%S %d-%m-%Y" 2>/dev/null; then
        # Linux style worked
        date -d "@$timestamp" +"%H:%M:%S %d-%m-%Y"
      else
        # Last resort - use current time
        date +"%H:%M:%S %d-%m-%Y"
      fi
      ;;
  esac
}

get_current_date() {
    date +"%H:%M:%S %d-%m-%Y"
}

get_timestamp() {
    date +%s
}

format_duration() {
  local total_seconds=$1
  local hours=$((total_seconds / 3600))
  local minutes=$(( (total_seconds % 3600) / 60 ))
  local seconds=$((total_seconds % 60))
  printf "%02d:%02d:%02d\n" "$hours" "$minutes" "$seconds"
}

# ===


exit_trap() {
  echo ""
  echo "Cleanup"
  echo "Exiting..."
  exit 0
}

trap exit_trap SIGINT SIGTERM

get_default_storage_class() {
    if [ -n "${STORAGE_CLASS:-}" ]; then
        echo "$STORAGE_CLASS"
    else
        kubectl get storageclass -o json \
            | jq -r '.items[] | select(.metadata.annotations."storageclass.kubernetes.io/is-default-class" == "true") | .metadata.name'
    fi
}

log_info() {
    local message="$1"
    local timestamp=$(get_current_date)
    echo -e "${BLUE}[INFO]${NC} $message"
    if [ -n "$LOG_FILE" ]; then
        echo "[$timestamp] [INFO] $message" >> "$LOG_FILE"
    fi
}

log_success() {
    local message="$1"
    local timestamp=$(get_current_date)
    echo -e "${GREEN}[SUCCESS]${NC} $message"
    if [ -n "$LOG_FILE" ]; then
        echo "[$timestamp] [SUCCESS] $message" >> "$LOG_FILE"
    fi
}

log_warning() {
    local message="$1"
    local timestamp=$(get_current_date)
    echo -e "${YELLOW}[WARNING]${NC} $message"
    if [ -n "$LOG_FILE" ]; then
        echo "[$timestamp] [WARNING] $message" >> "$LOG_FILE"
    fi
}

log_error() {
    local message="$1"
    local timestamp=$(get_current_date)
    echo -e "${RED}[ERROR]${NC} $message"
    if [ -n "$LOG_FILE" ]; then
        echo "[$timestamp] [ERROR] $message" >> "$LOG_FILE"
    fi
}


create_vi() {
  local vi_name=${1:-"perf-persistentvolumeclaim"}
  local ns=${2:-"perf-vs"}
  local sc=${3:-"ceph-pool-r2-csi-rbd"}

  kubectl apply -f - <<EOF
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualImage
metadata:
  name: ${vi_name}
  namespace: ${ns}
  labels:
    vms: perf
    test: blockdevices
spec:
  storage: PersistentVolumeClaim
  persistentVolumeClaim:
    storageClassName: ${sc}
  dataSource:
    type: "HTTP"
    http:
      url: https://0e773854-6b4e-4e76-a65b-d9d81675451a.selstorage.ru/alpine/alpine-v3-20.qcow2
EOF
  kubectl wait --for=condition=Ready --timeout=300s -n ${ns} virtualimage ${vi_name} || exit 1
}

create_vs() {
  local vs_name=${1:-"vs-perf"}
  local vi_name=${2:-"perf-persistentvolumeclaim"}
  local ns=${3:-"perf-vs"}
  local sc=${4:-"ceph-pool-r2-csi-rbd"}
  
  local pvc_name=$(kubectl -n $ns get vi $vi_name -o jsonpath='{.status.target.persistentVolumeClaimName}')

  kubectl apply -f - <<EOF
apiVersion: snapshot.storage.k8s.io/v1
kind: VolumeSnapshot
metadata:
  labels:
    vms: perf
    test: blockdevices
  name: ${vs_name}
  namespace: ${ns}
spec:
  source:
    persistentVolumeClaimName: ${pvc_name}
EOF
  kubectl wait --for=jsonpath='{.status.readyToUse}'=true --timeout=300s -n ${ns} volumesnapshot ${vs_name} || exit 1
}

create_pvc() {
  local pvc_name=${1:-"pvc-perf"}
  local vs_name=${2:-"vs-perf"}
  local ns=${3:-"perf-vs"}
  local sc=${4:-"ceph-pool-r2-csi-rbd"}
  
  kubectl apply -f - <<EOF
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: ${pvc_name}
  namespace: ${ns}
  labels:
    vms: perf
    test: blockdevices
spec:
  volumeMode: Block
  accessModes:
    - ReadWriteMany
  resources:
    requests:
      storage: 300Mi
  storageClassName: ${sc}
  dataSource:
    name: ${vs_name}
    kind: VolumeSnapshot
    apiGroup: snapshot.storage.k8s.io
EOF
}

create_vd() {
  local vd_name=${1:-"vd-perf"}
  local vs_name=${2:-"vs-perf"}
  local ns=${3:-"perf-vs"}
  local sc=${4:-"ceph-pool-r2-csi-rbd"}
  
  local pvc_name=$(kubectl -n $ns get vs $vs_name -o jsonpath='{.status.source.persistentVolumeClaimName}')

  kubectl apply -f - <<EOF
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualDisk
metadata:
  name: ${vd_name}
  namespace: ${ns}
  labels:
    vms: perf
    test: blockdevices
spec:
  persistentVolumeClaim:
    size: 300Mi
    storageClassName: ${sc}
  dataSource:
    type: "ObjectRef"
    objectRef:
      kind: "VolumeSnapshot"
      name: ${vs_name}
EOF
}

create_ns() {
  local ns=${1:-"perf-vs"}
  
  if kubectl get ns ${ns} &>/dev/null;then
    echo "NS exist"
  else 
    kubectl create ns ${ns}
    echo "NS ${ns} created"
  fi
}

# GLOBAL Values
NAMESPACE="perf-vs"
SC="ceph-pool-r2-csi-rbd-immediate"
VS_NAME="vs-perf"
PVC_NAME="pvc-perf"
VI_NAME="vi-perf"


main(){
  local pvc_count=${1:-1000}
  local start_time=$(get_timestamp)

  local pvc_name=""
  local pvc_bound=0

  create_ns ${NAMESPACE}
  
  log_info "Starting deployment volume_snapshot"
  log_info "Start time: $(formatted_date $start_time)"

  log_info "Create virtual image"
  create_vi ${VI_NAME} ${NAMESPACE} ${SC}

  log_info "Create snapshot"
  create_vs ${VS_NAME} ${VI_NAME} ${NAMESPACE} ${SC}

  log_info "Create persistent volume claim 1000"
  for i in $(seq 0 $(($pvc_count-1)) ); do
    pvc_name=$(printf "%s-%05d" ${PVC_NAME} ${i})
    create_pvc ${pvc_name} ${VS_NAME} ${NAMESPACE} ${SC}
  done

  log_info "Wait for bound"
  local bound_start=$(get_timestamp)
  while true; do
    log_info "Waiting for bound"
    sleep ${SLEEP_TIME}
    pvc_bound=0
    for i in $(seq 0 $((pvc_count - 1))); do
      pvc_name=$(printf "%s-%05d" ${PVC_NAME} ${i})
      bound=$(kubectl -n ${NAMESPACE} get pvc ${pvc_name} -o jsonpath='{.status.phase}' 2>/dev/null)
      if [ "$bound" == "Bound" ]; then
        ((pvc_bound++))
      fi
    done

    if [ $pvc_bound -eq $pvc_count ]; then
      local end_time=$(get_timestamp)
      local total_duration=$((end_time - start_time))
      local bound_duration=$((end_time - bound_start))
      log_info "Bound duration: $(format_duration $bound_duration)"
      log_info "Total duration: $(format_duration $total_duration)"
      
      local formatted_duration=$(format_duration "$total_duration")
      log_success "Bound all volumes in $formatted_duration"
      break
    fi

    log_info "Waiting for bound... $pvc_bound/$pvc_count"

  done

}


main $@



# новый sc
# k delete validatingwebhookconfigurations.admissionregistration.k8s.io d8-csi-ceph-sc-validation
# создать новый sc с только imageFeatures: layering