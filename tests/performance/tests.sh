#!/usr/bin/env bash

set -eEo pipefail

# Global configuration
NAMESPACE="perf"
STORAGE_CLASS=""
VI_TYPE="persistentVolumeClaim" # containerRegistry, persistentVolumeClaim
COUNT=2
SLEEP_TIME=5
REPORT_DIR="report"
MIGRATION_DURATION="5m"
MIGRATION_PERCENTAGE=10
ACTIVE_CLUSTER_PERCENTAGE=90
CONTROLLER_NAMESPACE="d8-virtualization"


# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Logging functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

exit_trap() {
  echo ""
  echo "Cleanup"
  echo "Exiting..."
  exit 0
}

format_duration() {
  local total_seconds=$1
  local hours=$((total_seconds / 3600))
  local minutes=$(( (total_seconds % 3600) / 60 ))
  local seconds=$((total_seconds % 60))
  printf "%02d:%02d:%02d\n" "$hours" "$minutes" "$seconds"
}

formatted_date() {
  date -d "@$1" +"%H:%M:%S %d-%m-%Y"
}

get_current_date() {
    date +"%H:%M:%S %d-%m-%Y"
}

get_timestamp() {
    date +%s
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

create_report_dir() {
  local dir=${1:-"statistics"}
  mkdir -p "$REPORT_DIR/$dir"
}

remove_report_dir() {
  local dir=${1:-$REPORT_DIR}
  rm -rf $dir
}

gather_all_statistics() {
  local report_dir=${1:-"$REPORT_DIR/statistics"}
  task statistic:get-stat:all

  mv tools/statistic/*.csv ${report_dir}
}

gather_vm_statistics() {
  local report_dir=${1:-"$REPORT_DIR/statistics"}
  task statistic:get-stat:vm

  mv tools/statistic/*.csv ${report_dir}
}

gather_vd_statistics() {
  local report_dir=${1:-"$REPORT_DIR/statistics"}
  task statistic:get-stat:vd

  mv tools/statistic/*.csv ${report_dir}
}

collect_vpa() {
  local vpa_dir="$REPORT_DIR/vpa"
  mkdir -p ${vpa_dir}
  local VPAS=( $(kubectl -n d8-virtualization get vpa -o name) )
  
  for vpa in $VPAS; do
    vpa_name=$(echo $vpa | cut -d "/" -f2)
    file="vpa_${vpa_name}.yaml"
    kubectl -n d8-virtualization get $vpa -o yaml > "${vpa_dir}/${file}_$(formatted_date $(date +%s))"
  done
}

wait_vm_vd() {
  local sleep_time=${1:-5}

  while true; do
    local VDReady=$(kubectl -n $NAMESPACE get vd | grep "Ready" | wc -l)
    local VDTotal=$(kubectl -n $NAMESPACE get vd -o name | wc -l)

    local VMReady=$(kubectl -n $NAMESPACE get vm | grep "Running" | wc -l)
    local VMTotal=$(kubectl -n $NAMESPACE get vm -o name | wc -l)

    if [ $VDReady -eq $VDTotal ] && [ $VMReady -eq $VMTotal ]; then
      echo "All vms and vds are ready"
      echo "$(formatted_date $(get_timestamp))"
      echo ""
      break
    fi

    echo ""
    echo "Waiting for vms and vds to be ready..."
    echo "VM Running: $VMReady/$VMTotal"
    echo "VD Ready: $VDReady/$VDTotal"
    echo ""
    echo "Waiting for $sleep_time seconds..."
    sleep $sleep_time
    echo ""
  done
}

wait_vm() {
  local sleep_time=${1:-5}
  local expected_count=$2
  local up=false

  while true; do
    local VMReady=$(kubectl -n $NAMESPACE get vm | grep "Running" | wc -l)
    local VMTotal=$(kubectl -n $NAMESPACE get vm -o name | wc -l)

    if [ -n "$expected_count" ]; then
      if [ $VMReady -eq $expected_count ]; then
        echo "All vms are ready"
        echo "$(formatted_date $(get_timestamp))"
        echo ""
        break
      fi
    else
      if [ $VMReady -eq $VMTotal ]; then
        echo "All vms are ready"
        echo "$(formatted_date $(get_timestamp))"
        echo ""
        break
      fi
    fi

    echo ""
    echo "Waiting for vms to be ready..."
    echo "VM Running: $VMReady/$VMTotal"
    echo ""
    echo "Waiting for $sleep_time seconds..."
    sleep $sleep_time
    echo ""

  done
}

wait_vd() {
  local sleep_time=${1:-5}
  local expected_count=$2
  local up=false

  while true; do
    local VDReady=$(kubectl -n $NAMESPACE get vm | grep "Running" | wc -l)
    local VDTotal=$(kubectl -n $NAMESPACE get vm -o name | wc -l)

    if [ -n "$expected_count" ]; then
      if [ $VDReady -eq $expected_count ]; then
        echo "All vds are ready"
        echo "$(formatted_date $(get_timestamp))"
        echo ""
        break
      fi
    else
      if [ $VDReady -eq $VDTotal ]; then
        echo "All vds are ready"
        echo "$(formatted_date $(get_timestamp))"
        echo ""
        break
      fi
    fi

    echo ""
    echo "Waiting for vds to be ready..."
    echo "VD ready: $VDReady/$VDTotal"
    echo ""
    echo "Waiting for $sleep_time seconds..."
    sleep $sleep_time
    echo ""

  done
}

wait_for_resources() {
  local resource_type=$1
  local report_file=$2
  local expected_count=$3
  local start_time=$(get_timestamp)
  local check_interval=5 # seconds

  case $resource_type in
    "all")
      log_info "Waiting for VMs and VDs to be ready"
      wait_vm_vd $check_interval
      ;;
    "vm")
      log_info "Waiting for VMs to be ready"
      wait_vm $check_interval $expected_count
      ;;
    "vd")
      log_info "Waiting for VDs to be ready"
      wait_vd $check_interval $expected_count
      ;;
    *)
      log_error "Unknown resource type: $resource_type"
      return 1
      ;;
  esac

}

start_migration() {
  # supoprt duration format: 0m - infinite, 30s - 30 seconds, 1h - 1 hour, 2h30m - 2 hours and 30 minutes
  local duration=${1:-"5m"}
  local SESSION="test-perf"
  
  echo "Create tmux session: $SESSION"
  tmux -2 new-session -d -s "${SESSION}"

  tmux new-window -t "$SESSION:1" -n "perf"
  tmux split-window -h -t 0      # Pane 0 (left), Pane 1 (right)
  tmux split-window -v -t 1      # Pane 1 (top), Pane 2 (bottom)

  tmux select-pane -t 0
  tmux send-keys "k9s -n perf" C-m
  tmux resize-pane -t 1 -x 50%
  
  echo "Start migration in $SESSION, pane 1"
  tmux select-pane -t 1
  tmux send-keys "NS=perf TARGET=5 DURATION=${duration} task evicter:run:migration" C-m
  tmux resize-pane -t 1 -x 50%

  tmux select-pane -t 2
  tmux resize-pane -t 2 -x 50%
  echo "For watching migration in $SESSION, connect to session with command:"
  echo "tmux -2 attach -t ${SESSION}"

  echo ""

}

stop_migration() {
  local SESSION="test-perf"
  tmux -2 kill-session -t "${SESSION}" || true
}

stop_vm() {
  local count=$1
  local sleep_time=${2:-5}

  local start_time=$(get_timestamp)

  local scanarion_name="scenario_1_${VI_TYPE}"
  local report_file="$REPORT_DIR/$scanarion_name/scenario_report.txt"

  if [ -z "$count" ]; then
    local vms=($(kubectl -n $NAMESPACE get vm | grep "Running" | awk '{print $1}'))
  else
    # Stop vm from the end
    local vms=($(kubectl -n $NAMESPACE get vm | grep "Running" | awk '{print $1}' | tail -n $count))
  fi

  if [ ${#vms[@]} -eq 0 ]; then
    log_warning "No running VMs found to stop"
    echo "0"
    return
  fi

  for vm in "${vms[@]}"; do
    echo "Stopping VM $vm"
    d8 v -n $NAMESPACE stop $vm
  done
  
  local stopped_vm=()
  total=${#vms[@]}
  
  # Wait for vms to stop
  while true; do

    for vm in "${vms[@]}"; do
      local status=$(kubectl -n $NAMESPACE get vm $vm -o jsonpath='{.status.phase}')
      if [ "$status" == "Stopped" ]; then
        stopped_vm+=($vm)
      fi
    done
    
    stopped=${#stopped_vm[@]}
    stopped_vm=()
    
    if [ $stopped -eq $total ]; then
      echo "All vms are stoped"
      local end_time=$(get_timestamp)
      local duration=$((end_time - start_time))
      formatted_duration=$(format_duration "$duration")
      echo "" >> $report_file
      echo "Stop $stopped VMs time:" >> $report_file
      echo "Duration: $duration seconds" >> $report_file
      echo "Execution time: $formatted_duration" >> $report_file
      echo "" >> $report_file

      log_info "Duration: $duration seconds"
      log_info "Execution time: $formatted_duration"
      log_info ""
      break
    fi

    echo ""
    echo "Waiting for vms to be stopped..."
    echo "VM stopped: $stopped/$total"
    echo ""
    echo "Waiting for $sleep_time seconds..."
    sleep $sleep_time
    echo ""
  done
}

start_vm() {
  local count=$1
  local sleep_time=${2:-5}

  local start_time=$(get_timestamp)

  local scanarion_name="scenario_1_${VI_TYPE}"
  local report_file="$REPORT_DIR/$scanarion_name/scenario_report.txt"

  if [ -z "$count" ]; then
    local vms=($(kubectl -n $NAMESPACE get vm | grep "Stopped" | awk '{print $1}'))
  else
    # Start vm from the end
    local vms=($(kubectl -n $NAMESPACE get vm | grep "Stopped" | awk '{print $1}' | tail -n $count))
  fi

  if [ ${#vms[@]} -eq 0 ]; then
    log_warning "No running VMs found to stop"
    echo "0"
    return
  fi

  for vm in "${vms[@]}"; do
    echo "Running VM $vm"
    d8 v -n $NAMESPACE start $vm
  done

  # Wait for vms to be running
  local running_vm=()
  total=${#vms[@]}

  while true; do
    
    for vm in "${vms[@]}"; do
      local status=$(kubectl -n perf get vm $vm -o jsonpath='{.status.phase}')
      if [ "$status" == "Running" ]; then
        running_vm+=($vm)
      fi
    done
    
    running=${#running_vm[@]}
    running_vm=()
    
    
    if [ $running -eq $total ]; then
      echo "All vms are running"
      local end_time=$(get_timestamp)
      local duration=$((end_time - start_time))
      formatted_duration=$(format_duration "$duration")
      echo "" >> $report_file
      echo "Start $running VMs time:" >> $report_file
      echo "Duration: $duration seconds" >> $report_file
      echo "Execution time: $formatted_duration" >> $report_file
      echo "" >> $report_file

      log_info "Duration: $duration seconds"
      log_info "Execution time: $formatted_duration"
      log_info ""
      break
    fi

    echo ""
    echo "Waiting for vms to be running..."
    echo "VM running: $running/$total"
    echo ""
    echo "Waiting for $sleep_time seconds..."
    sleep $sleep_time
    echo ""
  done
}

undeploy_resources() {
  local sleep_time=${1:-5}
  task destroy:all
  
  while true; do
    VDTotal=$(kubectl -n $NAMESPACE get vd -o name | wc -l)
    VMTotal=$(kubectl -n $NAMESPACE get vm -o name | wc -l)
    
    if [ $VDTotal -eq 0 ] && [ $VMTotal -eq 0 ]; then
      echo "All vms and vds are destroyed"
      log_info "$(formatted_date $(date +%s))"
      echo ""
      break
    fi

    echo ""
    echo "Waiting for vms and vds to be destroyed..."
    echo "VM to destroy: $VMTotal"
    echo "VD to destroy: $VDTotal"
    echo ""
    echo "Waiting for $sleep_time seconds..."
    sleep $sleep_time
    echo ""
  done
}

deploy_vms_with_disks() {
  local count=$1
  local vi_type=$2
  local start_time=$(get_timestamp)

  local scanarion_name="scenario_1_${VI_TYPE}"
  local report_file="$REPORT_DIR/$scanarion_name/scenario_report.txt"

  echo "Scenario 1" > $report_file
  echo "Deploying $count VMs with disks from $vi_type" >> $report_file
  echo "Start time: $(formatted_date $start_time)" >> $report_file
  
  log_info "Deploying $count VMs with disks from $vi_type"
    
  task apply:all \
      COUNT=$count \
      NAMESPACE=$NAMESPACE \
      STORAGE_CLASS=$(get_default_storage_class) \
      VIRTUALDISK_TYPE=virtualDisk \
      VIRTUALIMAGE_TYPE=$vi_type

  wait_vm_vd $SLEEP_TIME

  local end_time=$(get_timestamp)

  echo "End time: $(formatted_date $end_time)" >> $report_file
  
  local duration=$((end_time - start_time))
  formatted_duration=$(format_duration "$duration")
  echo "Duration: $duration seconds" >> $report_file
  echo "Execution time: $formatted_duration" >> $report_file

  log_info "Duration: $duration seconds"
  log_info "Execution time: $formatted_duration"
  log_info ""

}

deploy_disks_only() {
  local count=$1
  local vi_type=$2
  local start_time=$(get_timestamp)

  local scanarion_name="scenario_1_${VI_TYPE}"
  local report_file="$REPORT_DIR/$scanarion_name/scenario_report.txt"
  
  log_info "Deploying $count disks from $vi_type"
  
  task apply:disks \
      COUNT=$count \
      NAMESPACE=$NAMESPACE \
      STORAGE_CLASS=$(get_default_storage_class) \
      VIRTUALDISK_TYPE=virtualDisk \
      VIRTUALIMAGE_TYPE=$vi_type
  
  wait_for_resources "vd" $count
  
  local end_time=$(get_timestamp)
  local duration=$((end_time - start_time))
  
  log_success "Deployed $count disks in $(format_duration $duration)"
  echo "$duration"
}


# === Test cases ===
RESCOUNT=5
MIGRATION_DURATION="1m"
WAIT_MIGRATION=$( echo "$MIGRATION_DURATION" | sed 's/m//' )

# cd ..
remove_report_dir
undeploy_resources
stop_migration

VI_TYPE="persistentVolumeClaim" # containerRegistry, persistentVolumeClaim
create_report_dir "scenario_1_${VI_TYPE}/statistics"

#======== pvc ===
deploy_vms_with_disks $RESCOUNT $VI_TYPE
gather_all_statistics "$REPORT_DIR/scenario_1_${VI_TYPE}/statistics"

start_test_pvc=$(get_timestamp)
start_migration "$MIGRATION_DURATION"

stop_vm 3
start_vm 3
while true; do
  current_time=$(get_timestamp)
  duration=$((current_time - start_test_pvc))
  if [ $duration -ge $(( $WAIT_MIGRATION*60 )) ]; then
    stop_migration
    break
  fi
  log_info "Waiting for migration to complete"
  log_info "Duration: $duration seconds from $(( $WAIT_MIGRATION*60 ))"
  sleep 1
done

undeploy_resources
#=========

VI_TYPE="containerRegistry"
create_report_dir "scenario_1_${VI_TYPE}/statistics"

#==== registry === 
deploy_vms_with_disks $RESCOUNT $VI_TYPE
gather_all_statistics "$REPORT_DIR/scenario_1_${VI_TYPE}/statistics"

start_test_pvc=$(get_timestamp)
start_migration "$MIGRATION_DURATION"

stop_vm 2
start_vm 2
while true; do
  current_time=$(get_timestamp)
  duration=$((current_time - start_test_pvc))
  if [ $duration -ge $(( $WAIT_MIGRATION*60 )) ]; then
    stop_migration
    break
  fi
  log_info "Waiting for migration to complete"
  log_info "Duration: $duration seconds from $(( $WAIT_MIGRATION*60 ))"
  sleep 1
done
undeploy_resources
#====

log_success "Done"