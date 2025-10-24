#!/usr/bin/env bash

# Controller management library for performance testing
# This module handles controller lifecycle management

# Source common utilities
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

stop_virtualization_controller() {
  local start_time=$(get_timestamp)

  log_info "Stopping virtualization controller"
  # Get original replicas count before stopping
  ORIGINAL_CONTROLLER_REPLICAS=$(kubectl -n d8-virtualization get deployment virtualization-controller -o jsonpath="{.spec.replicas}" 2>/dev/null || echo "1")
  log_info "Original controller replicas: $ORIGINAL_CONTROLLER_REPLICAS"
  log_info "Start time: $(formatted_date $start_time)"

  local scale_start=$(get_timestamp)
  kubectl -n d8-virtualization scale --replicas 0 deployment virtualization-controller
  local scale_end=$(get_timestamp)
  local scale_duration=$((scale_end - scale_start))
  log_info "Scale down command completed in $(format_duration $scale_duration)"

  local wait_start=$(get_timestamp)
  while true; do
    local count_pods=$(kubectl -n d8-virtualization get pods | grep virtualization-controller | wc -l)
    
    if [ $count_pods -eq 0 ]; then
      local wait_end=$(get_timestamp)
      local wait_duration=$((wait_end - wait_start))
      local end_time=$(get_timestamp)
      local duration=$((end_time - start_time))
      local formatted_duration=$(format_duration "$duration")
      
      log_info "Wait for controller stop completed in $(format_duration $wait_duration)"
      log_info "Controller stopped - End time: $(formatted_date $end_time)"
      log_info "Scale command: $(format_duration $scale_duration), Wait time: $(format_duration $wait_duration)"
      log_success "Controller stopped in $formatted_duration"
      break
    fi
    
    log_info "Waiting for virtualization-controller to be stopped... Pods: $count_pods"
    sleep 2
  done

  # Additional wait using kubectl wait
  log_info "Additional wait for deployment to be fully available..."
  kubectl wait --for=condition=Available=True deployment/virtualization-controller -n d8-virtualization --timeout=300s
}

start_virtualization_controller() {
  local start_time=$(get_timestamp)

  log_info "Starting Virtualization-controller"
  log_info "Restoring controller to original replicas: ${ORIGINAL_CONTROLLER_REPLICAS:-1}"
  log_info "Start time: $(formatted_date $start_time)"

  local scale_start=$(get_timestamp)
  kubectl -n d8-virtualization scale --replicas ${ORIGINAL_CONTROLLER_REPLICAS:-1} deployment virtualization-controller
  local scale_end=$(get_timestamp)
  local scale_duration=$((scale_end - scale_start))
  log_info "Scale up command completed in $(format_duration $scale_duration)"

  log_info "Wait for deployment for Virtualization-controller to be fully available..."
  kubectl wait --for=condition=Available=True deployment/virtualization-controller -n d8-virtualization --timeout=300s
  local end_time=$(get_timestamp)
  local duration=$((end_time - start_time))
  local formatted_duration=$(format_duration "$duration")
  
  log_info "Virtualization-controller started - End time: $(formatted_date $end_time)"
  log_success "Virtualization-controller started in $formatted_duration"
}

# FIXED: Create VM while controller is stopped using task apply:all
create_vm_while_controller_stopped() {
  local vi_type=$1
  local start_time=$(get_timestamp)
  
  log_info "Creating 1 VM and disk while controller is stopped using task apply:all"
  log_info "Start time: $(formatted_date $start_time)"
  log_vm_operation "Creating 1 VM and disk while controller is stopped using task apply:all"
  
  # Deploy MAIN_COUNT_RESOURCES + 1 VMs using task apply:all
  log_info "Deploying 1 new VM"
  
  local task_start=$(get_timestamp)
  task apply:all \
      COUNT=$((MAIN_COUNT_RESOURCES + 1)) \
      NAMESPACE=$NAMESPACE \
      STORAGE_CLASS=$(get_default_storage_class) \
      VIRTUALDISK_TYPE=virtualDisk \
      VIRTUALIMAGE_TYPE=$vi_type || true
  local task_end=$(get_timestamp)
  local task_duration=$((task_end - task_start))
  log_info "Task apply:all completed in $(format_duration $task_duration)"
}

wait_for_new_vm_after_controller_start() {
  # Wait for the last VM and VD to be ready
  log_info "Waiting for the last VM and VD to be ready"
  local wait_start=$(get_timestamp)

  # Get the name of the last VM and VD
  local last_vm=$(kubectl -n $NAMESPACE get vm --no-headers | tail -n 1 | awk '{print $1}')
  local last_vd=$(kubectl -n $NAMESPACE get vd --no-headers | tail -n 1 | awk '{print $1}')
  
  log_info "Waiting for last VM: $last_vm and last VD: $last_vd"
  
  # Wait for the last VM to be Running
  while true; do
    local vm_status=$(kubectl -n $NAMESPACE get vm $last_vm -o jsonpath="{.status.phase}" 2>/dev/null || echo "NotFound")
    local vd_status=$(kubectl -n $NAMESPACE get vd $last_vd -o jsonpath="{.status.phase}" 2>/dev/null || echo "NotFound")
    
    if [ "$vm_status" == "Running" ] && [ "$vd_status" == "Ready" ]; then
      local wait_end=$(get_timestamp)
      local wait_duration=$((wait_end - wait_start))
      log_info "Last VM and VD are ready in $(format_duration $wait_duration)"
      break
    fi
    
    log_info "Waiting for last VM ($last_vm): $vm_status, last VD ($last_vd): $vd_status"
    sleep 5
  done
}