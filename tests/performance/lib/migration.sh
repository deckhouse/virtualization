#!/usr/bin/env bash

# Migration testing library for performance testing
# This module handles migration testing functionality

# Source common utilities
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

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

  # Additional wait using kubectl wait
  log_info "Additional wait for deployment to be fully available..."
  kubectl wait --for=condition=Available=True deployment/virtualization-controller -n d8-virtualization --timeout=300s
}

# NEW: Wait for migration completion before proceeding
wait_migration_completion() {
  local start_time=$(get_timestamp)
  
  log_info "Waiting for migration to complete"
  log_vmop_operation "Waiting for migration to complete"
  
  # Wait for all vmops to complete
  wait_vmops_complete
  
  local end_time=$(get_timestamp)
  local duration=$((end_time - start_time))
  log_info "Migration completion wait finished in $(format_duration $duration)"
  log_vmop_operation "Migration completion wait finished in $(format_duration $duration)"
}

remove_vmops() {
  local namespace=${1:-$NAMESPACE}
  
  while true; do
    log_info "Check if all vmops are removed"
    log_vmop_operation "Checking vmops for removal"
    local vmop_total=$(( $(kubectl -n $namespace get vmop | wc -l )-1 ))
    local vmop_list=$(kubectl -n $namespace get vmop | grep "Completed" | awk '{print $1}')
    local vmop_failed_list=$(kubectl -n $namespace get vmop | grep "Failed" | awk '{print $1}')
    log_warning "VMOP failed list: $vmop_failed_list"
    log_vmop_operation "VMOP failed list: $vmop_failed_list"

    vmop_list+=" $vmop_failed_list"

    log_info "VMOP total: $( if [ $vmop_total -le 0 ]; then echo "0"; else echo $vmop_total; fi )"
    log_vmop_operation "VMOP total: $( if [ $vmop_total -le 0 ]; then echo "0"; else echo $vmop_total; fi )"
    if [ $vmop_total -le 0 ]; then
      log_success "All vmops are removed"
      log_vmop_operation "All vmops are removed"
      break
    fi
    
    for vmop in $vmop_list; do
      kubectl -n $namespace delete vmop $vmop --wait=false || true
      log_vmop_operation "Deleted vmop: $vmop"
    done

  # Additional wait using kubectl wait
  log_info "Additional wait for deployment to be fully available..."
  kubectl wait --for=condition=Available=True deployment/virtualization-controller -n d8-virtualization --timeout=300s
    
    log_info "Wait 2 sec"
    sleep 2
  done

  # Additional wait using kubectl wait
  log_info "Additional wait for deployment to be fully available..."
  kubectl wait --for=condition=Available=True deployment/virtualization-controller -n d8-virtualization --timeout=300s
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

  # Additional wait using kubectl wait
  log_info "Additional wait for deployment to be fully available..."
  kubectl wait --for=condition=Available=True deployment/virtualization-controller -n d8-virtualization --timeout=300s
}

# FIXED: Wait for vmops to complete (including Failed status) and check VMs are Running
wait_vmops_complete() {
  local sleep_time=${1:-2}
  local start_time=$(get_timestamp)

  while true; do
    local vmop_total=$(( $(kubectl -n $NAMESPACE get vmop | wc -l)-1 ))
    local VMOPCompleted=$(kubectl -n $NAMESPACE get vmop | grep "Completed" | wc -l)
    local VMOPFailed=$(kubectl -n $NAMESPACE get vmop | grep "Failed" | wc -l)
    local VMOPInProgress=$(kubectl -n $NAMESPACE get vmop | grep "InProgress" | wc -l)

    if [ $vmop_total -eq -1 ]; then
      vmop_total=0
    fi

    # Consider vmops complete if they are either Completed or Failed (not InProgress)
    local VMOPFinished=$((VMOPCompleted + VMOPFailed))
    
    if [ $VMOPFinished -eq $vmop_total ] && [ $VMOPInProgress -eq 0 ]; then
      # Additional check: ensure all VMs are Running
      local VMRunning=$(kubectl -n $NAMESPACE get vm | grep "Running" | wc -l)
      local VMTotal=$(kubectl -n $NAMESPACE get vm -o name | wc -l)
      
      if [ $VMRunning -eq $VMTotal ]; then
        local end_time=$(get_timestamp)
        local duration=$((end_time - start_time))
        formatted_duration=$(format_duration "$duration")
        
        log_info "VMOPs completed - Duration: $duration seconds"
        log_info "Execution time: $formatted_duration"
        log_info "Completed: $VMOPCompleted, Failed: $VMOPFailed, Total: $vmop_total"
        log_info "All VMs are Running: $VMRunning/$VMTotal"
        log_vmop_operation "VMOPs completed - Duration: $duration seconds"
        log_vmop_operation "Completed: $VMOPCompleted, Failed: $VMOPFailed, Total: $vmop_total"
        log_vmop_operation "All VMs are Running: $VMRunning/$VMTotal"
        break
      else
        log_info "VMOPs finished but VMs not all Running yet: $VMRunning/$VMTotal"
        log_vmop_operation "VMOPs finished but VMs not all Running yet: $VMRunning/$VMTotal"
      fi
    fi
    
    log_info "Waiting for vmops to be ready... Completed: $VMOPCompleted, Failed: $VMOPFailed, InProgress: $VMOPInProgress, Total: $vmop_total"
    log_vmop_operation "Waiting for vmops to be ready... Completed: $VMOPCompleted, Failed: $VMOPFailed, InProgress: $VMOPInProgress, Total: $vmop_total"
    sleep $sleep_time
  done

  # Additional wait using kubectl wait
  log_info "Additional wait for deployment to be fully available..."
  kubectl wait --for=condition=Available=True deployment/virtualization-controller -n d8-virtualization --timeout=300s
}

migration_percent_vms() {
  local target_count=${1:-$PERCENT_RESOURCES}
  local namespace=${2:-$NAMESPACE}
  local start_time=$(get_timestamp)

  log_info "Starting migration of $target_count VMs"
  log_info "Start time: $(formatted_date $start_time)"
  log_vm_operation "Starting migration of $target_count VMs"

  local vms=( $(kubectl -n $NAMESPACE get vm --no-headers | awk '$2 == "Running" {print $1}' | tail -n $target_count) )

  for vm in "${vms[@]}"; do
    log_info "Migrating VM [$vm] via evict"
    log_vm_operation "Migrating VM [$vm] via evict"
    d8 v -n $NAMESPACE evict $vm --wait=false
  done

  # Additional wait using kubectl wait
  # log_info "Additional wait for deployment to be fully available..."
  # kubectl wait --for=condition=Available=True deployment/virtualization-controller -n d8-virtualization --timeout=300s

  wait_vmops_complete

  local end_time=$(get_timestamp)
  local duration=$((end_time - start_time))
  local formatted_duration=$(format_duration "$duration")
  
  log_info "Migration completed - End time: $(formatted_date $end_time)"
  log_success "Migrated $target_count VMs in $formatted_duration"
  log_vm_operation "Migration completed - Migrated $target_count VMs in $formatted_duration"
}

