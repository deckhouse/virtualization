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
  local namespace=${2:-$NAMESPACE}
  task statistic:get-stat:all NAMESPACE=$namespace

  mv tools/statistic/*.csv ${report_dir}
}

gather_vm_statistics() {
  local report_dir=${1:-"$REPORT_DIR/statistics"}
  local namespace=${2:-$NAMESPACE}
  task statistic:get-stat:vm NAMESPACE=$namespace

  mv tools/statistic/*.csv ${report_dir}
}

gather_vd_statistics() {
  local report_dir=${1:-"$REPORT_DIR/statistics"}
  local namespace=${2:-$NAMESPACE}
  task statistic:get-stat:vd NAMESPACE=$namespace

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
    local VMRunning=$(kubectl -n $NAMESPACE get vm | grep "Running" | wc -l)
    local VMTotal=$(kubectl -n $NAMESPACE get vm -o name | wc -l)

    if [ -n "$expected_count" ]; then
      if [ $VMRunning -eq $expected_count ]; then
        echo "All vms are ready"
        echo "$(formatted_date $(get_timestamp))"
        echo ""
        break
      fi
    else
      if [ $VMRunning -eq $VMTotal ]; then
        echo "All vms are ready"
        echo "$(formatted_date $(get_timestamp))"
        echo ""
        break
      fi
    fi

    echo ""
    echo "Waiting for vms to be running..."
    echo "VM Running: $VMRunning/$VMTotal"
    echo ""
    echo "Waiting for $sleep_time seconds..."
    sleep $sleep_time
    echo ""

  done
}

wait_vd() {
  local sleep_time=${1:-5}
  local expected_count=$2

  while true; do
    local VDReady=$(kubectl -n $NAMESPACE get vd | grep "Ready" | wc -l)
    local VDTotal=$(kubectl -n $NAMESPACE get vd -o name | wc -l)

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
  local expected_count=$2
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

start_migration_old() {
  # supoprt duration format: 0m - infinite, 30s - 30 seconds, 1h - 1 hour, 2h30m - 2 hours and 30 minutes
  local duration=${1:-"0m"}
  local target=${2:-"5"}
  local session="test-perf"
  
  echo "Create tmux session: $session"
  tmux -2 new-session -d -s "${session}"

  tmux new-window -t "$session:1" -n "$NAMESPACE"
  tmux split-window -h -t 0      # Pane 0 (left), Pane 1 (right)
  tmux split-window -v -t 1      # Pane 1 (top), Pane 2 (bottom)

  tmux select-pane -t 0
  tmux send-keys "k9s -n $NAMESPACE" C-m
  tmux resize-pane -t 1 -x 50%
  
  echo "Start migration in $session, pane 1"
  tmux select-pane -t 1
  tmux send-keys "NS=$NAMESPACE TARGET=${target} DURATION=${duration} task evicter:run:migration" C-m
  tmux resize-pane -t 1 -x 50%

  tmux select-pane -t 2
  tmux resize-pane -t 2 -x 50%
  echo "For watching migration in $session, connect to session with command:"
  echo "tmux -2 attach -t ${session}"

  echo ""

}

start_migration() {
  local duration=${1:-"0m"}
  local target=${2:-"5"}
  local session="test-perf"
  local ns="${NAMESPACE:-perf}"

  echo "Create tmux session: $session"
  tmux new-session -d -s "${session}" -n "${ns}" # windows named "ns"

  # split window
  tmux split-window  -h  -t "${session}:0"         # Pane 0 (left), Pane 1 (right)
  tmux split-window  -v  -t "${session}:0.1"       # Split right pane; .1

  # 3) Посылаем команды в нужные панели явно
  tmux select-pane   -t "${session}:0.0"
  tmux send-keys     -t "${session}:0.0" "k9s -n ${ns}" C-m
  tmux resize-pane   -t "${session}:0.1" -x 50%

  echo "Start migration in $session, pane 1"
  tmux select-pane   -t "${session}:0.1"
  tmux send-keys     -t "${session}:0.1" "NS=${ns} TARGET=${target} DURATION=${duration} task evicter:run:migration" C-m
  tmux resize-pane   -t "${session}:0.1" -x 50%

  tmux select-pane   -t "${session}:0.2"
  tmux resize-pane   -t "${session}:0.2" -x 50%

  echo "For watching migration in $session, attach with:"
  echo "tmux -2 attach -t ${session}"

  # Optional
  # if [ -n "${TMUX:-}" ]; then
  #   tmux switch-client -t "${session}" # switch client to created session inside tmux
  # else
  #   tmux -2 attach -t "${session}" # from bash tmux — just attach to created session
  # fi
}


stop_migration() {
  local SESSION="test-perf"
  tmux send-keys -t "${SESSION}:1.1" C-c || true
  sleep 1
  tmux -2 kill-session -t "${SESSION}" || true
}

wait_migration() {
  local timeout=${1:-"5m"}
  local wait_migration=$( echo "$timeout" | sed 's/m//' )
  local start_time=$(get_timestamp)
  
  log_info "Waiting for migration to complete"
  log_info "Duration: $timeout minutes"

  while true; do
    current_time=$(get_timestamp)
    duration=$((current_time - start_time))
    if [ $duration -ge $(( $wait_migration*60 )) ]; then
      log_info "Migration timeout reached, stopping migrator"
      stop_migration
      log_success "Migration completed"
      break
    fi
    log_info "Waiting for migration to complete"
    log_info "Duration: $duration seconds from $(( $WAIT_MIGRATION*60 ))"
    sleep 1
  done
}

remove_vmops() {
  local namespace=${1:-$NAMESPACE}
  
  while true; do
    log_info "Check if all vmops are removed"
    local vmop_total=$(( $(kubectl -n $namespace get vmop | wc -l )-1 ))
    local vmop_list=$(kubectl -n $namespace get vmop | grep "Completed" | awk '{print $1}')
    local vmop_failed_list=$(kubectl -n $namespace get vmop | grep "Failed" | awk '{print $1}')
    log_warning "VMOP failed list: $vmop_failed_list"

    vmop_list+=" $vmop_failed_list"

    log_info "VMOP total: $( if [ $vmop_total -le 0 ]; then echo "0"; else echo $vmop_total; fi )"
    if [ $vmop_total -le 0 ]; then
      log_success "All vmops are removed"
      break
    fi
    
    for vmop in $vmop_list; do
      kubectl -n $namespace delete vmop $vmop || true
    done
    
    log_info "Wait 2 sec"
    sleep 2
  done
}

wait_vmops() {
  local sleep_time=${1:-2}

  while true; do
    local VMOPInProgress=$(kubectl -n $NAMESPACE get vmop | grep "InProgress" | wc -l)

    if [ $VMOPInProgress -eq 0 ]; then
      echo "All vmops are ready"
      echo "$(formatted_date $(get_timestamp))"
      echo ""
      break
    fi
    
    echo ""
    echo "Waiting for vmops to be ready..."
    echo "VMOP InProgress: $VMOPInProgress"
    echo ""
    echo "Waiting for $sleep_time seconds..."
    sleep $sleep_time
    echo ""
  done
}

wait_vmops_complete() {
  local sleep_time=${1:-2}
  local start_time=$(get_timestamp)

  local scenario_name="$SCENARIO${VI_TYPE}"
  local report_file="$REPORT_DIR/$scenario_name/scenario_report.txt"

  while true; do
    local vmop_total=$(( $(kubectl -n $NAMESPACE get vmop | wc -l)-1 ))
    local VMOPCompleted=$(kubectl -n $NAMESPACE get vmop | grep "Completed" | wc -l)

    if [ $vmop_total -eq -1 ]; then
      vmop_total=0
    fi

    if [ $VMOPCompleted -eq $vmop_total ]; then
      local end_time=$(get_timestamp)
      local duration=$((end_time - start_time))
      formatted_duration=$(format_duration "$duration")
      echo "" >> $report_file
      echo "wait_vmops_complete" >> $report_file
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
    echo "Waiting for vmops to be ready..."
    echo "VMOP Completed: $VMOPCompleted/$vmop_total"
    echo ""
    echo "Waiting for $sleep_time seconds..."
    sleep $sleep_time
    echo ""
  done
}

stop_vm() {
  local count=$1
  local sleep_time=${2:-5}

  local start_time=$(get_timestamp)

  local scenario_name="$SCENARIO${VI_TYPE}"
  local report_file="$REPORT_DIR/$scenario_name/scenario_report.txt"

  if [ -z "$count" ]; then
    local vms=($(kubectl -n $NAMESPACE get vm | grep "Running" | awk '{print $1}'))
  else
    # Stop vm from the end
    local vms=($(kubectl -n $NAMESPACE get vm | grep "Running" | awk '{print $1}' | tail -n $count))
  fi

  if [ ${#vms[@]} -eq 0 ]; then
    log_warning "No running VMs found to stop"
    echo "0"
    return 0
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
      echo "Stop vms" >> $report_file
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

  local scenario_name="$SCENARIO${VI_TYPE}"
  local report_file="$REPORT_DIR/$scenario_name/scenario_report.txt"

  if [ -z "$count" ]; then
    local vms=($(kubectl -n $NAMESPACE get vm | grep "Stopped" | awk '{print $1}'))
  else
    # Start vm from the end
    local vms=($(kubectl -n $NAMESPACE get vm | grep "Stopped" | awk '{print $1}' | tail -n $count))
  fi

  if [ ${#vms[@]} -eq 0 ]; then
    log_warning "No running VMs found to run"
    echo "0"
    return
  fi

  for vm in "${vms[@]}"; do
    echo "Running VM $vm"
    d8 v -n $NAMESPACE start $vm
  done


  while true; do

    # Wait for vms to be running    
    if [ -z "$count" ]; then
      local vms=($(kubectl -n $NAMESPACE get vm | grep "Stopped" | awk '{print $1}'))
    else
      # Start vm from the end
      local vms=($(kubectl -n $NAMESPACE get vm | grep "Stopped" | awk '{print $1}' | tail -n $count))
    fi

    local total=${#vms[@]}
    local running_vm=0
    
    for vm in "${vms[@]}"; do
      local status=$(kubectl -n perf get vm $vm -o jsonpath='{.status.phase}')
      if [ "$status" == "Running" ]; then
        (( running_vm+=1 ))
      fi
    done

    if [ $running_vm -eq $total ]; then
      echo "All vms are running"
      local end_time=$(get_timestamp)
      local duration=$((end_time - start_time))
      formatted_duration=$(format_duration "$duration")
      echo "" >> $report_file
      echo "Start vm" >> $report_file
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
    echo "VM running: $running_vm/$total"
    echo ""
    echo "Waiting for $sleep_time seconds..."
    sleep $sleep_time
    echo ""
  done
}

migration_percent_vms() {
  local percent=${1:-$PERCENT_RESOURCES}
  local namespace=${2:-$NAMESPACE}
  local start_time=$(get_timestamp)

  local target=$(( $MAIN_COUNT_RESOURCES * $percent / 100 ))

  local scenario_name="$SCENARIO${VI_TYPE}"
  local report_file="$REPORT_DIR/$scenario_name/scenario_report.txt"
  
  echo "" >> $report_file
  echo "=== migration_${percent}_percent_vms ===" >> $report_file
  echo "Undeploy all VMs and disks" >> $report_file
  echo "Start time: $(formatted_date $start_time)" >> $report_file

  local vms=$(kubectl -n $NAMESPACE get vm -o name | grep Running | awk '{print $1}' | tail -n $target)

  for vm in $vms; do
    echo "Migrate vm [$vm] via evict"
    d8 v -n $NAMESPACE evict $vm
  done

  wait_vmops_complete

  local end_time=$(get_timestamp)
  local duration=$((end_time - start_time))
  local formatted_duration=$(format_duration "$duration")
  echo "" >> $report_file
  echo "End time: $(formatted_date $end_time)" >> $report_file
  echo "Duration: $duration seconds" >> $report_file
  echo "Execution time: $formatted_duration" >> $report_file
  echo "=== migration_${percent}_percent_vms ===" >> $report_file
  echo "" >> $report_file
  
  log_success "Migrated $target VMs in $(format_duration $duration)"

}

undeploy_resources() {
  local sleep_time=${1:-5}
  local start_time=$(get_timestamp)
  local VDTotal
  local VMTotal
  local VMITotal

  local scenario_name="$SCENARIO${VI_TYPE}"
  local report_file="$REPORT_DIR/$scenario_name/scenario_report.txt"

  echo "undeploy_resources" >> $report_file
  echo "Undeploy all VMs and disks" >> $report_file
  echo "Start time: $(formatted_date $start_time)" >> $report_file

  log_info "Undeploy all VMs and disks"

  task destroy:all
  
  while true; do
    VDTotal=$(kubectl -n $NAMESPACE get vd -o name | wc -l)
    VMTotal=$(kubectl -n $NAMESPACE get vm -o name | wc -l)
    VMITotal=$(kubectl -n $NAMESPACE get vi -o name | wc -l)
    
    if [ $VDTotal -eq 0 ] && [ $VMTotal -eq 0 ] && [ $VMITotal -eq 0 ]; then
      local end_time=$(get_timestamp)
      local duration=$((end_time - start_time))
      local formatted_duration=$(format_duration "$duration")
      echo "End time: $(formatted_date $end_time)" >> $report_file
      echo "Duration: $duration seconds" >> $report_file
      echo "Execution time: $formatted_duration" >> $report_file
      echo "" >> $report_file

      echo "All vms and vds are destroyed"
      log_info "Duration: $duration seconds"
      log_info "Execution time: $formatted_duration"
      log_info ""
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

  local scenario_name="$SCENARIO${VI_TYPE}"
  local report_file="$REPORT_DIR/$scenario_name/scenario_report.txt"

  echo "deploy_vms_with_disks" >> $report_file
  echo "Deploying $count VMs with disks from $vi_type" >> $report_file
  echo "Start time: $(formatted_date $start_time)" >> $report_file
  
  log_info "Deploying $count VMs with disks from $vi_type"
    
  task apply:all \
      COUNT=$count \
      NAMESPACE=$NAMESPACE \
      STORAGE_CLASS=$(get_default_storage_class) \
      VIRTUALDISK_TYPE=virtualDisk \
      VIRTUALIMAGE_TYPE=$vi_type || true

  wait_vm_vd $SLEEP_TIME

  local end_time=$(get_timestamp)

  echo "End time: $(formatted_date $end_time)" >> $report_file
  
  local duration=$((end_time - start_time))
  local formatted_duration=$(format_duration "$duration")
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

  local scenario_name="$SCENARIO${VI_TYPE}"
  local report_file="$REPORT_DIR/$scenario_name/scenario_report.txt"
  
  echo "deploy_disks_only" >> $report_file
  echo "Deploying $count VDs with type $vi_type" >> $report_file
  echo "Start time: $(formatted_date $start_time)" >> $report_file
  
  log_info "Deploying $count disks from $vi_type"
  
  task apply:disks \
      COUNT=$count \
      NAMESPACE=$NAMESPACE \
      STORAGE_CLASS=$(get_default_storage_class) \
      VIRTUALDISK_TYPE=virtualDisk \
      VIRTUALIMAGE_TYPE=$vi_type
  
  wait_vd $SLEEP_TIME
  
  local end_time=$(get_timestamp)
  local duration=$((end_time - start_time))
  local formatted_duration=$(format_duration "$duration")
  echo "End time: $(formatted_date $end_time)" >> $report_file
  echo "Duration: $duration seconds" >> $report_file
  echo "Execution time: $formatted_duration" >> $report_file
  echo "" >> $report_file
  
  log_success "Deployed $count disks in $(format_duration $duration)"
  echo "$duration"
}

deploy_vms_only() {
  local count=$1
  local namespace=${2:-$NAMESPACE}
  local start_time=$(get_timestamp)

  local scenario_name="$SCENARIO${VI_TYPE}"
  local report_file="$REPORT_DIR/$scenario_name/scenario_report.txt"

  echo "" >> $report_file
  echo "deploy_vms_only" >> $report_file
  echo "Deploying $count VDs with type $vi_type" >> $report_file
  echo "Start time: $(formatted_date $start_time)" >> $report_file

  log_info "Deploying $count VMs (disks already exist)"
  
  task apply:vms \
      COUNT=$count \
      NAMESPACE=$NAMESPACE
  
  # wait_for_resources "vm" $count
  wait_vm $SLEEP_TIME
  
  local end_time=$(get_timestamp)
  local duration=$((end_time - start_time))

  local end_time=$(get_timestamp)
  local duration=$((end_time - start_time))
  local formatted_duration=$(format_duration "$duration")
  echo "End time: $(formatted_date $end_time)" >> $report_file
  echo "Duration: $duration seconds" >> $report_file
  echo "Execution time: $formatted_duration" >> $report_file
  echo "" >> $report_file
  
  log_success "Deployed $count VMs in $(format_duration $duration)"
  echo "$duration"
}

undeploy_vms_only() {
  local count=${1:-0}
  local namespace=${2:-$NAMESPACE}
  local start_time=$(get_timestamp)

  local scenario_name="$SCENARIO${VI_TYPE}"
  local report_file="$REPORT_DIR/$scenario_name/scenario_report.txt"

  echo "" >> $report_file
  echo "undeploy_vms_only" >> $report_file
  echo "Undeploying $count VDs with type $vi_type" >> $report_file
  echo "Start time: $(formatted_date $start_time)" >> $report_file

  log_info "Undeploying $count VMs (disks already exist)"
  
  task apply:vms \
      COUNT=$count \
      NAMESPACE=$NAMESPACE
  
  while true; do
    local vms_count=$(kubectl -n $NAMESPACE get vm | wc -l)
    if [ $vms_count -eq 0 ]; then
      echo "All vms are undeployed"
      log_info "$(formatted_date $(date +%s))"
      echo ""
      
      local end_time=$(get_timestamp)
      local duration=$((end_time - start_time))
      local formatted_duration=$(format_duration "$duration")
      echo "End time: $(formatted_date $end_time)" >> $report_file
      echo "Duration: $duration seconds" >> $report_file
      echo "Execution time: $formatted_duration" >> $report_file
      echo "" >> $report_file
      break
    fi
    
    echo ""
    echo "Waiting for vms to be undeployed..."
    echo "VM to undeploy: $vms_count/0"
    echo ""
    echo "Waiting for $SLEEP_TIME seconds..."
    sleep $SLEEP_TIME
    echo ""
  done
  
  log_success "Undeployed VMs in $(format_duration $duration)"
  echo "$duration"
}

stop_virtualization_controller() {
  local start_time=$(get_timestamp)

  local scenario_name="$SCENARIO${VI_TYPE}"
  local report_file="$REPORT_DIR/$scenario_name/scenario_report.txt"

  echo "" >> $report_file
  echo "stop_virtualization_controller" >> $report_file
  echo "Start time: $(formatted_date $start_time)" >> $report_file

  kubectl -n d8-virtualization scale --replicas 0 deployment virtualization-controller

  while true; do
    local count_pods=$(kubectl -n d8-virtualization get pods | grep virtualization-controller | wc -l)
    
    if [ $count_pods -eq 0 ]; then
      local end_time=$(get_timestamp)
      local duration=$((end_time - start_time))
      local formatted_duration=$(format_duration "$duration")
      echo "End time: $(formatted_date $end_time)" >> $report_file
      echo "Duration: $duration seconds" >> $report_file
      echo "Execution time: $formatted_duration" >> $report_file
      echo "" >> $report_file
      break
    fi
    
    echo "Waiting for virtualization-controller to be stopped..."
    echo "Pods to stop: $count_pods"
    sleep 2
  done
}

start_virtualization_controller() {
  local start_time=$(get_timestamp)

  local scenario_name="$SCENARIO${VI_TYPE}"
  local report_file="$REPORT_DIR/$scenario_name/scenario_report.txt"

  echo "" >> $report_file
  echo "start_virtualization_controller" >> $report_file
  echo "Start time: $(formatted_date $start_time)" >> $report_file

  kubectl -n d8-virtualization scale --replicas 1 deployment virtualization-controller
  while true; do
    local count_pods=$(kubectl -n d8-virtualization get pods | grep virtualization-controller | grep "Running"| wc -l)
    if [ $count_pods -ge 1 ]; then
      local end_time=$(get_timestamp)
      local duration=$((end_time - start_time))

      local end_time=$(get_timestamp)
      local duration=$((end_time - start_time))
      local formatted_duration=$(format_duration "$duration")
      echo "End time: $(formatted_date $end_time)" >> $report_file
      echo "Duration: $duration seconds" >> $report_file
      echo "Execution time: $formatted_duration" >> $report_file
      echo "" >> $report_file
      break
    fi
    echo "Waiting for virtualization-controller to be started..."
    echo "Pods to start: $count_pods"
    sleep 2
  done
}

# === Test cases ===
MAIN_COUNT_RESOURCES=20 # vms and vds
PERCENT_VMS=10
MIGRATION_DURATION="1m"
MIGRATION_PERCENTAGE_10=10
MIGRATION_PERCENTAGE_5=5
WAIT_MIGRATION=$( echo "$MIGRATION_DURATION" | sed 's/m//' )

PERCENT_RESOURCES=$(( $MAIN_COUNT_RESOURCES * $PERCENT_VMS / 100 ))
if [ $PERCENT_RESOURCES -eq 0 ]; then
  PERCENT_RESOURCES=1
fi

# cd ..
# VI_TYPE="containerRegistry" # containerRegistry, persistentVolumeClaim
VI_TYPE="persistentVolumeClaim" # containerRegistry, persistentVolumeClaim
prepare_for_tests() {
  remove_report_dir
  create_report_dir "$SCENARIO${VI_TYPE}/statistics"
  undeploy_resources
  stop_migration

  # prepare for tests
  remove_report_dir
}

prepare_for_tests

# VI_TYPE="persistentVolumeClaim" # containerRegistry, persistentVolumeClaim
# #======== pvc ===

SN=1
log_info "Start scenario ${SN}"
SCENARIO="scenario_${SN}_"
log_info "Create report dirs"
create_report_dir "$SCENARIO${VI_TYPE}/statistics"
create_report_dir "$SCENARIO${VI_TYPE}/statistics/deploy_vm_${MAIN_COUNT_RESOURCES}"
create_report_dir "$SCENARIO${VI_TYPE}/statistics/deploy_vm_${PERCENT_RESOURCES}"
create_report_dir "$SCENARIO${VI_TYPE}/statistics/vm_${PERCENT_RESOURCES}_deploy"

START_TIME_SN=$(get_timestamp)
echo "Test perf vms" >> $REPORT_DIR/$SCENARIO${VI_TYPE}/statistics/scenario_report.txt
echo "Start: $(formatted_date $START_TIME_SN)" >> $REPORT_DIR/$SCENARIO${VI_TYPE}/statistics/scenario_report.txt
echo "" >> $REPORT_DIR/$SCENARIO${VI_TYPE}/statistics/scenario_report.txt

log_info "Deploy resources"
deploy_vms_with_disks $MAIN_COUNT_RESOURCES $VI_TYPE
log_info "Gather statistics gather_all_statistics"
gather_all_statistics "$REPORT_DIR/$SCENARIO${VI_TYPE}/statistics"
log_info "Wait 30 seconds, before stop vms"
sleep 30
log_info "Stop vms - $(get_current_date)"
stop_vm
log_info "Wait 30 seconds, before start vms"
sleep 30
log_info "Start vms - $(get_current_date)"
start_vm

log_info "Undeploy vms - $(get_current_date)"
undeploy_vms_only 0

# Time for 10% of VMs with disks in the active cluster
# delete PERCENT_RESOURCES vms
start_migration "0m" $MIGRATION_PERCENTAGE_5
# prepare for tests, remove 10% of VMs
deploy_vms_only $(( $MAIN_COUNT_RESOURCES-$PERCENT_RESOURCES )) $VI_TYPE

# deploy 10% of VMs and gather statistics
echo "== deploy ${PERCENT_RESOURCES}% of VMs and gather statistics ==" >> $REPORT_DIR/$SCENARIO${VI_TYPE}/statistics/deploy_vm_${PERCENT_RESOURCES}/scenario_report.txt
deploy_vms_only $MAIN_COUNT_RESOURCES $VI_TYPE
gather_vm_statistics "$REPORT_DIR/$SCENARIO${VI_TYPE}/statistics/deploy_vm_${PERCENT_RESOURCES}"

echo "== ${PERCENT_RESOURCES} ==" >> $REPORT_DIR/$SCENARIO${VI_TYPE}/statistics/deploy_vm_${PERCENT_RESOURCES}/scenario_report.txt
stop_vm $PERCENT_RESOURCES
start_vm $PERCENT_RESOURCES
stop_migration
remove_vmops
log_info "Start migration"
migration_percent_vms
# log_info "Wait 5 seconds"
# sleep 5
# stop_migration
# wait_vmops_complete
log_info "Wait 5 seconds"
sleep 5

deploy_vms_only $(( $MAIN_COUNT_RESOURCES-1 )) $VI_TYPE
log_info "Stop virtualization controller"
stop_virtualization_controller
log_info "Start virtualization controller"
start_virtualization_controller
deploy_vms_with_disks $MAIN_COUNT_RESOURCES $VI_TYPE
wait_for_resources "all"
gather_all_statistics "$REPORT_DIR/$SCENARIO${VI_TYPE}/statistics"
log_info "Wait 1 seconds"
sleep 1
undeploy_resources
log_success "Done with scenario ${SN}"

END_TIME_SN=$(get_timestamp)
DURATION_SN=$((END_TIME_SN - START_TIME_SN))
FORMATTED_DURATION_SN=$(format_duration "$DURATION_SN")
echo "" >> $REPORT_DIR/$SCENARIO${VI_TYPE}/statistics/scenario_report.txt
echo "End: $(formatted_date $END_TIME_SN)" >> $REPORT_DIR/$SCENARIO${VI_TYPE}/statistics/scenario_report.txt
echo "Duration: $DURATION_SN seconds" >> $REPORT_DIR/$SCENARIO${VI_TYPE}/statistics/scenario_report.txt
echo "Execution time: $FORMATTED_DURATION_SN" >> $REPORT_DIR/$SCENARIO${VI_TYPE}/statistics/scenario_report.txt

# # All vms CSSD
# test_scenario() {
#   SN=1
#   log_info "Start scenario ${SN}"
#   SCENARIO="scenario_${SN}_"
#   create_report_dir "$SCENARIO${VI_TYPE}/statistics"
#   deploy_vms_with_disks $COUNT_RESOURCES $VI_TYPE
#   gather_all_statistics "$REPORT_DIR/$SCENARIO${VI_TYPE}/statistics"
#   stop_vm
#   start_vm
#   undeploy_resources
#   log_success "Done with scenario ${SN}"

#   log_info "Wait 10 seconds"
#   sleep 10
#   echo ""

#   SN=2
#   log_info "Start scenario ${SN}"
#   SCENARIO="scenario_${SN}_"
#   create_report_dir "$SCENARIO${VI_TYPE}/statistics"
#   deploy_disks_only $COUNT_RESOURCES $VI_TYPE
#   gather_vd_statistics "$REPORT_DIR/$SCENARIO${VI_TYPE}/statistics"
#   deploy_vms_only $COUNT_RESOURCES $VI_TYPE
#   gather_vm_statistics "$REPORT_DIR/$SCENARIO${VI_TYPE}/statistics"
#   stop_vm
#   start_vm
#   undeploy_resources
#   log_success "Done with scenario ${SN}"

#   log_info "Wait 10 seconds"
#   sleep 10
#   echo ""

#   SN=3
#   log_info "Start scenario ${SN}"
#   SCENARIO="scenario_${SN}_"
#   COUNT_SCENARIO_3=2
#   create_report_dir "$SCENARIO${VI_TYPE}/statistics"
#   deploy_vms_with_disks $COUNT_RESOURCES $VI_TYPE
#   start_migration #infinite
#   deploy_vms_with_disks $COUNT_SCENARIO_3 $VI_TYPE
#   stop_vm $COUNT_SCENARIO_3
#   start_vm $COUNT_SCENARIO_3
#   wait_migration "2m"
#   undeploy_resources
#   log_success "Done with scenario ${SN}"

#   log_info "Wait 10 seconds"
#   sleep 10
#   echo ""

#   SN=4
#   log_info "Start scenario ${SN}"
#   SCENARIO="scenario_${SN}_"
#   COUNT_SCENARIO_4=2
#   create_report_dir "$SCENARIO${VI_TYPE}/statistics"
#   deploy_vms_with_disks $COUNT_RESOURCES $VI_TYPE
#   start_migration #infinite
#   deploy_disks_only $COUNT_RESOURCES $VI_TYPE
#   deploy_vms_only $COUNT_RESOURCES $VI_TYPE
#   gather_vm_statistics "$REPORT_DIR/$SCENARIO${VI_TYPE}/statistics"
#   stop_vm $COUNT_SCENARIO_4
#   start_vm $COUNT_SCENARIO_4
#   wait_migration "2m"
#   undeploy_resources
#   log_success "Done with scenario ${SN}"

#   log_info "Wait 10 seconds"
#   sleep 10
#   echo ""

#   SN=5
#   log_info "Start scenario ${SN}"
#   SCENARIO="scenario_${SN}_"
#   create_report_dir "$SCENARIO${VI_TYPE}/statistics"
#   deploy_vms_with_disks $COUNT_RESOURCES $VI_TYPE
#   start_migration "2m" "10"
#   # need calculate duration
#   # NS=$NAMESPACE TARGET=${target} DURATION=${duration} task evicter:run:migration
#   undeploy_resources
#   log_success "Done with scenario ${SN}"

#   log_info "Wait 10 seconds"
#   sleep 10
#   echo ""

#   SN=6
#   log_info "Start scenario ${SN}"
#   SCENARIO="scenario_${SN}_"
#   create_report_dir "$SCENARIO${VI_TYPE}/statistics"
#   deploy_vms_with_disks $COUNT_RESOURCES $VI_TYPE
#   stop_vm $COUNT_RESOURCES
#   start_vm $COUNT_RESOURCES
#   undeploy_resources
#   log_success "Done with scenario ${SN}"

#   log_info "Wait 10 seconds"
#   sleep 10
#   echo ""

#   SN=7
#   log_info "Start scenario ${SN}"
#   SCENARIO="scenario_${SN}_"
#   COUNT_SCENARIO_4=1
#   create_report_dir "$SCENARIO${VI_TYPE}/statistics"
#   deploy_vms_with_disks $COUNT_RESOURCES $VI_TYPE
#   stop_virtualization_controller
#   sn7_start_time=$(get_timestamp)
#   start_virtualization_controller
#   deploy_vms_with_disks $COUNT_SCENARIO_4 $VI_TYPE
#   gather_all_statistics "$REPORT_DIR/$SCENARIO${VI_TYPE}/statistics"
#   undeploy_resources
#   log_success "Done with scenario ${SN}"
# }
