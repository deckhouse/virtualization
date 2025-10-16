#!/usr/bin/env bash

# VM operations library for performance testing
# This module handles VM lifecycle management operations

# Source common utilities
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

wait_vm_vd() {
  local sleep_time=${1:-10}

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

  # Additional wait using kubectl wait
  log_info "Additional wait for deployment to be fully available..."
  kubectl wait --for=condition=Available=True deployment/virtualization-controller -n d8-virtualization --timeout=300s
}

wait_vm() {
  local sleep_time=${1:-10}
  local expected_count=$2
  local VMTotal
  local VMRunning

  while true; do
    VMRunning=$(kubectl -n $NAMESPACE get vm | grep "Running" | wc -l)

    if [ -n "$expected_count" ]; then
      VMTotal=$expected_count
    else
      VMTotal=$(kubectl -n $NAMESPACE get vm -o name | wc -l)
    fi
      
    if [ $VMRunning -eq $VMTotal ]; then
      echo "All vms are ready"
      echo "$(formatted_date $(get_timestamp))"
      echo ""
      break
    fi

    echo ""
    echo "Waiting for vms to be running..."
    echo "VM Running: $VMRunning/$VMTotal"
    echo ""
    echo "Waiting for $sleep_time seconds..."
    sleep $sleep_time
    echo ""

  done

  # Additional wait using kubectl wait
  log_info "Additional wait for deployment to be fully available..."
  kubectl wait --for=condition=Available=True deployment/virtualization-controller -n d8-virtualization --timeout=300s
}

wait_vd() {
  local sleep_time=${1:-10}
  local expected_count=$2
  local VDReady
  local VDTotal

  while true; do
    VDReady=$(kubectl -n $NAMESPACE get vd | grep "Ready" | wc -l)

    if [ -n "$expected_count" ]; then
      VDTotal=$expected_count
    else
      VDTotal=$(kubectl -n $NAMESPACE get vd -o name | wc -l)
    fi

    if [ $VDReady -eq $VDTotal ]; then
      echo "All vds are ready"
      echo "$(formatted_date $(get_timestamp))"
      echo ""
      break
    fi

    echo ""
    echo "Waiting for vds to be ready..."
    echo "VD ready: $VDReady/$VDTotal"
    echo ""
    echo "Waiting for $sleep_time seconds..."
    sleep $sleep_time
    echo ""

  done

  # Additional wait using kubectl wait
  log_info "Additional wait for deployment to be fully available..."
  kubectl wait --for=condition=Available=True deployment/virtualization-controller -n d8-virtualization --timeout=300s
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

stop_vm() {
  local count=$1
  local sleep_time=${2:-5}
  local start_time=$(get_timestamp)
  local stopped_vm

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

  log_info "Stopping ${#vms[@]} VMs"
  log_vm_operation "Stopping ${#vms[@]} VMs: ${vms[*]}"
  for vm in "${vms[@]}"; do
    log_info "Stopping VM $vm"
    log_vm_operation "Stopping VM $vm"
    d8 v -n $NAMESPACE stop $vm --wait=false
  done

  # Additional wait using kubectl wait
  log_info "Additional wait for deployment to be fully available..."
  kubectl wait --for=condition=Available=True deployment/virtualization-controller -n d8-virtualization --timeout=300s
  
  local total=${#vms[@]}
  
  # Wait for vms to stop
  while true; do
    stopped_vm=0

    for vm in "${vms[@]}"; do
      local status=$(kubectl -n $NAMESPACE get vm $vm -o jsonpath='{.status.phase}')
      if [ "$status" == "Stopped" ]; then
        (( stopped_vm+=1 ))
      fi
    done

  # Additional wait using kubectl wait
  # log_info "Additional wait for deployment to be fully available..."
  # kubectl wait --for=condition=Available=True deployment/virtualization-controller -n d8-virtualization --timeout=300s
    
    stopped=${#stopped_vm[@]}
    
    if [ $stopped_vm -eq $total ]; then
      local end_time=$(get_timestamp)
      local duration=$((end_time - start_time))
      formatted_duration=$(format_duration "$duration")
      
      log_success "All VMs stopped - Duration: $duration seconds"
      log_info "Execution time: $formatted_duration"
      log_vm_operation "All VMs stopped - Duration: $duration seconds"
      break
    fi

    log_info "Waiting for VMs to be stopped... VM stopped: $stopped_vm/$total"
    log_vm_operation "Waiting for VMs to be stopped... VM stopped: $stopped_vm/$total"
    sleep $sleep_time
  done

  # Additional wait using kubectl wait
  # log_info "Additional wait for deployment to be fully available..."
  # kubectl wait --for=condition=Available=True deployment/virtualization-controller -n d8-virtualization --timeout=300s
}

# FIXED: Properly wait for VMs to be Running
start_vm() {
  local count=$1
  local sleep_time=${2:-5}
  local start_time=$(get_timestamp)

  if [ -z "$count" ]; then
    local vms=($(kubectl -n $NAMESPACE get vm | grep "Stopped" | awk '{print $1}'))
  else
    # Start vm from the end
    local vms=($(kubectl -n $NAMESPACE get vm | grep "Stopped" | awk '{print $1}' | tail -n $count))
  fi

  if [ ${#vms[@]} -eq 0 ]; then
    log_warning "No stopped VMs found to start"
    echo "0"
    return
  fi

  log_info "Starting ${#vms[@]} VMs"
  log_vm_operation "Starting ${#vms[@]} VMs: ${vms[*]}"
  for vm in "${vms[@]}"; do
    log_info "Starting VM $vm"
    log_vm_operation "Starting VM $vm"
    d8 v -n $NAMESPACE start $vm
  done

  # Additional wait using kubectl wait
  log_info "Additional wait for deployment to be fully available..."
  kubectl wait --for=condition=Available=True deployment/virtualization-controller -n d8-virtualization --timeout=300s

  # Store the VMs we started for monitoring
  local started_vms=("${vms[@]}")
  local total=${#started_vms[@]}
  
  while true; do
    local running_vm=0
    
    for vm in "${started_vms[@]}"; do
      local status=$(kubectl -n $NAMESPACE get vm $vm -o jsonpath='{.status.phase}' 2>/dev/null || echo "NotFound")
      if [ "$status" == "Running" ]; then
        (( running_vm+=1 ))
      fi
    done

  # Additional wait using kubectl wait
  log_info "Additional wait for deployment to be fully available..."
  kubectl wait --for=condition=Available=True deployment/virtualization-controller -n d8-virtualization --timeout=300s

    if [ $running_vm -eq $total ]; then
      local end_time=$(get_timestamp)
      local duration=$((end_time - start_time))
      formatted_duration=$(format_duration "$duration")
      
      log_success "All VMs started - Duration: $duration seconds"
      log_info "Execution time: $formatted_duration"
      log_vm_operation "All VMs started - Duration: $duration seconds"
      break
    fi

    log_info "Waiting for VMs to be running... VM running: $running_vm/$total"
    log_vm_operation "Waiting for VMs to be running... VM running: $running_vm/$total"
    sleep $sleep_time
  done

  # Additional wait using kubectl wait
  log_info "Additional wait for deployment to be fully available..."
  kubectl wait --for=condition=Available=True deployment/virtualization-controller -n d8-virtualization --timeout=300s
}

deploy_vms_with_disks() {
  local count=$1
  local vi_type=$2
  local start_time=$(get_timestamp)
  
  log_info "Deploying $count VMs with disks from $vi_type"
  log_info "Start time: $(formatted_date $start_time)"
    
  local task_start=$(get_timestamp)
  task apply:all \
      COUNT=$count \
      NAMESPACE=$NAMESPACE \
      STORAGE_CLASS=$(get_default_storage_class) \
      VIRTUALDISK_TYPE=virtualDisk \
      VIRTUALIMAGE_TYPE=$vi_type

  local task_end=$(get_timestamp)
  local task_duration=$((task_end - task_start))
  log_info "Task apply:all completed in $(format_duration $task_duration)"
  log_duration "Task apply:all" "$task_duration"

  local wait_start=$(get_timestamp)
  wait_vm_vd $SLEEP_TIME
  local wait_end=$(get_timestamp)
  local wait_duration=$((wait_end - wait_start))
  log_info "Wait for VMs and VDs completed in $(format_duration $wait_duration)"
  log_duration "Wait for VMs and VDs" "$wait_duration"

  local end_time=$(get_timestamp)
  local duration=$((end_time - start_time))
  local formatted_duration=$(format_duration "$duration")
  
  log_info "Deployment completed - End time: $(formatted_date $end_time)"
  log_info "Task execution: $(format_duration $task_duration), Wait time: $(format_duration $wait_duration)"
  log_success "Deployed $count VMs with disks in $formatted_duration"
}

deploy_disks_only() {
  local count=$1
  local vi_type=$2
  local start_time=$(get_timestamp)
  
  log_info "Deploying $count disks from $vi_type"
  log_info "Start time: $(formatted_date $start_time)"
  
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
  
  log_info "Disk deployment completed - End time: $(formatted_date $end_time)"
  log_success "Deployed $count disks in $formatted_duration"
  echo "$duration"
}

deploy_vms_only() {
  local count=$1
  local namespace=${2:-$NAMESPACE}
  local start_time=$(get_timestamp)

  log_info "Deploying $count VMs (disks already exist)"
  log_info "Start time: $(formatted_date $start_time)"
  
  local task_start=$(get_timestamp)
  task apply:vms \
      COUNT=$count \
      NAMESPACE=$NAMESPACE
  local task_end=$(get_timestamp)
  local task_duration=$((task_end - task_start))
  log_info "Task apply:vms completed in $(format_duration $task_duration)"
  log_duration "Task apply:vms" "$task_duration"
  
  local wait_start=$(get_timestamp)
  wait_vm $SLEEP_TIME
  local wait_end=$(get_timestamp)
  local wait_duration=$((wait_end - wait_start))
  log_info "Wait for VMs completed in $(format_duration $wait_duration)"
  log_duration "Wait for VMs" "$wait_duration"

  local end_time=$(get_timestamp)
  local duration=$((end_time - start_time))
  local formatted_duration=$(format_duration "$duration")
  
  log_info "VM deployment completed - End time: $(formatted_date $end_time)"
  log_info "Task execution: $(format_duration $task_duration), Wait time: $(format_duration $wait_duration)"
  log_success "Deployed $count VMs in $formatted_duration"
  echo "$duration"
}

# FIXED: Properly undeploy VMs from the end
undeploy_vms_only() {
  local count=${1:-0}
  local namespace=${2:-$NAMESPACE}
  local start_time=$(get_timestamp)

  log_info "Undeploying $count VMs from the end (disks will remain)"
  log_info "Start time: $(formatted_date $start_time)"
  
  # Get list of VMs and select the last 'count' ones
  local vms=($(kubectl -n $NAMESPACE get vm -o name | tail -n $count))
  
  if [ ${#vms[@]} -eq 0 ]; then
    log_warning "No VMs found to undeploy"
    echo "0"
    return 0
  fi
  
  log_info "Undeploying ${#vms[@]} VMs: ${vms[*]}"
  log_vm_operation "Undeploying ${#vms[@]} VMs from the end: ${vms[*]}"
  
  local delete_start=$(get_timestamp)
  for vm in "${vms[@]}"; do
    log_info "Deleting VM $vm"
    log_vm_operation "Deleting VM $vm"
    kubectl -n $NAMESPACE delete $vm --wait=false || true
  done

  local delete_end=$(get_timestamp)
  local delete_duration=$((delete_end - delete_start))
  log_info "VM deletion commands completed in $(format_duration $delete_duration)"
  log_vm_operation "VM deletion commands completed in $(format_duration $delete_duration)"

  local wait_start=$(get_timestamp)
  while true; do
    local remaining_vms=0
    local current_time=$(get_timestamp)
    
    log_info "Deleting remaining VMs..."
    for vm in "${vms[@]}"; do
      if kubectl -n $NAMESPACE get $vm >/dev/null 2>&1; then
        log_info "Deleting VM $vm"
        kubectl -n $NAMESPACE delete $vm --wait=false || true
      fi
    done
    
    for vm in "${vms[@]}"; do
      # Check if VM exists and is not in Terminating state
      local vm_status=$(kubectl -n $NAMESPACE get $vm -o jsonpath='{.status.phase}' 2>/dev/null || echo "NotFound")
      if [ "$vm_status" != "NotFound" ] && [ "$vm_status" != "Terminating" ]; then
        ((remaining_vms++))
        log_info "VM $vm still exists with status: $vm_status"
      fi
    done
    
    if [ $remaining_vms -eq 0 ]; then
      local wait_end=$(get_timestamp)
      local wait_duration=$((wait_end - wait_start))
      local end_time=$(get_timestamp)
      local duration=$((end_time - start_time))
      local formatted_duration=$(format_duration "$duration")
      
      log_info "Wait for VMs undeploy completed in $(format_duration $wait_duration)"
      log_info "All $count VMs undeployed - End time: $(formatted_date $end_time)"
      log_info "Delete commands: $(format_duration $delete_duration), Wait time: $(format_duration $wait_duration)"
      log_success "Undeployed $count VMs in $formatted_duration"
      log_vm_operation "Undeployed $count VMs in $formatted_duration"
      break
    fi
    
    log_info "Waiting for VMs to be undeployed... Remaining: $remaining_vms/$count"
    log_vm_operation "Waiting for VMs to be undeployed... Remaining: $remaining_vms/$count"
    sleep $SLEEP_TIME
  done

  # Additional wait using kubectl wait
  log_info "Additional wait for deployment to be fully available..."
  kubectl wait --for=condition=Available=True deployment/virtualization-controller -n d8-virtualization --timeout=300s
  
  # echo "$duration"
}

undeploy_resources() {
  local sleep_time=${1:-5}
  local start_time=$(get_timestamp)
  local VDTotal
  local VMTotal
  local VMITotal

  log_info "Undeploying all VMs and disks"
  log_info "Start time: $(formatted_date $start_time)"

  task destroy:all \
    NAMESPACE=$NAMESPACE 
  # Wait a bit for Helm to process the deletion
  sleep 5
  
  # # Force delete any remaining resources
  # kubectl -n $NAMESPACE delete vm --all --ignore-not-found=true --force --grace-period=0
  # kubectl -n $NAMESPACE delete vd --all --ignore-not-found=true --force --grace-period=0
  # kubectl -n $NAMESPACE delete vi --all --ignore-not-found=true --force --grace-period=0  
  # local max_wait_time=600  # Maximum wait time in seconds (10 minutes)
  # local wait_timeout=$((start_time + max_wait_time))
  
  while true; do
    # local current_time=$(get_timestamp)
    
    # Check for timeout
    # if [ $current_time -gt $wait_timeout ]; then
    #   log_warning "Timeout reached while waiting for resources to be destroyed"
    #   break
    # fi
    
    VDTotal=$(kubectl -n $NAMESPACE get vd -o name | wc -l)
    VMTotal=$(kubectl -n $NAMESPACE get vm -o name | wc -l)
    VMITotal=$(kubectl -n $NAMESPACE get vi -o name | wc -l)
    
    if [ $VDTotal -eq 0 ] && [ $VMTotal -eq 0 ] && [ $VMITotal -eq 0 ]; then
      local end_time=$(get_timestamp)
      local duration=$((end_time - start_time))
      local formatted_duration=$(format_duration "$duration")
      
      log_info "All VMs and VDs destroyed - End time: $(formatted_date $end_time)"
      log_success "Undeploy completed in $formatted_duration"
      break
    fi

    log_info "Waiting for VMs and VDs to be destroyed... VM: $VMTotal, VD: $VDTotal, VI: $VMITotal"
    sleep $sleep_time
  done

  # Additional wait using kubectl wait
  log_info "Additional wait for deployment to be fully available..."
  kubectl wait --for=condition=Available=True deployment/virtualization-controller -n d8-virtualization --timeout=300s
}
