# Batch deployment functions for VM operations

# Function to check if batch deployment should be used
should_use_batch_deployment() {
  local count=$1
  # Don't use batch deployment if batch size is too small (less than 10% of total)
  local min_batch_size=$((count / 10))
  if [ $min_batch_size -lt 1 ]; then
    min_batch_size=1
  fi
  
  # Warn if batch size is too small
  if [ $MAX_BATCH_SIZE -lt $min_batch_size ]; then
    log_warning "Batch size ($MAX_BATCH_SIZE) is too small for $count resources"
    log_warning "Minimum recommended batch size: $min_batch_size"
    log_warning "Using regular deployment instead of batch deployment"
    return 1  # false
  fi
  
  if [ "$BATCH_DEPLOYMENT_ENABLED" = "true" ] || [ $count -gt $MAX_BATCH_SIZE ]; then
    return 0  # true
  else
    return 1  # false
  fi
}

# Function to show deployment progress
show_deployment_progress() {
  local current_count=$1
  local total_count=$2
  local batch_number=$3
  local total_batches=$4
  local start_time=$5
  
  local current_time=$(get_timestamp)
  local elapsed_time=$((current_time - start_time))
  local progress_percent=$(( (current_count * 100) / total_count ))
  
  # Calculate estimated time remaining
  local estimated_total_time=0
  local estimated_remaining_time=0
  if [ $current_count -gt 0 ]; then
    estimated_total_time=$(( (elapsed_time * total_count) / current_count ))
    estimated_remaining_time=$((estimated_total_time - elapsed_time))
  fi
  
  log_info "Progress: $current_count/$total_count ($progress_percent%)"
  log_info "Batch: $batch_number/$total_batches"
  log_info "Elapsed: $(format_duration $elapsed_time)"
  if [ $estimated_remaining_time -gt 0 ]; then
    log_info "Estimated remaining: $(format_duration $estimated_remaining_time)"
  fi
}

# New function for batch deployment of large numbers of resources
deploy_vms_with_disks_batch() {
  local total_count=$1
  local vi_type=$2
  local batch_size=${3:-$MAX_BATCH_SIZE}
  local start_time=$(get_timestamp)
  
  log_info "Starting batch deployment of $total_count VMs with disks from $vi_type"
  log_info "Batch size: $batch_size resources per batch"
  log_info "Start time: $(formatted_date $start_time)"
  
  local deployed_count=0
  local batch_number=1
  local total_batches=$(( (total_count + batch_size - 1) / batch_size ))
  
  log_info "Total batches to deploy: $total_batches"
  
  while [ $deployed_count -lt $total_count ]; do
    local remaining_count=$((total_count - deployed_count))
    local current_batch_size=$batch_size
    
    # Adjust batch size for the last batch if needed
    if [ $remaining_count -lt $batch_size ]; then
      current_batch_size=$remaining_count
    fi
    
    log_info "=== Batch $batch_number/$total_batches ==="
    show_deployment_progress "$deployed_count" "$total_count" "$batch_number" "$total_batches" "$start_time"
    
    local batch_start=$(get_timestamp)
    
    # Deploy current batch (COUNT should be cumulative, not absolute)
    local cumulative_count=$((deployed_count + current_batch_size))
    log_info "Deploying batch $batch_number: $current_batch_size new resources (total will be: $cumulative_count)"
    task apply:all \
        COUNT=$cumulative_count \
        NAMESPACE=$NAMESPACE \
        STORAGE_CLASS=$(get_default_storage_class) \
        VIRTUALDISK_TYPE=virtualDisk \
        VIRTUALIMAGE_TYPE=$vi_type
    
    # Wait for current batch to be ready
    wait_vm_vd $SLEEP_TIME
    
    local batch_end=$(get_timestamp)
    local batch_duration=$((batch_end - batch_start))
    deployed_count=$((deployed_count + current_batch_size))
    
    log_success "Batch $batch_number completed in $(format_duration $batch_duration)"
    log_info "Total deployed so far: $deployed_count/$total_count"
    
    # Add delay between batches to avoid overwhelming the system
    if [ $batch_number -lt $total_batches ]; then
      log_info "Waiting 30 seconds before next batch..."
      sleep 30
    fi
    
    ((batch_number++))
  done
  
  local end_time=$(get_timestamp)
  local total_duration=$((end_time - start_time))
  local formatted_duration=$(format_duration "$total_duration")
  
  log_success "Batch deployment completed: $deployed_count VMs with disks in $formatted_duration"
  log_info "Average time per resource: $(( total_duration / deployed_count )) seconds"
  
  echo "$total_duration"
}

# Universal deployment function that automatically chooses between regular and batch deployment
deploy_vms_with_disks_smart() {
  local count=$1
  local vi_type=$2
  local batch_size=${3:-$MAX_BATCH_SIZE}
  
  log_info "Deployment decision for $count resources:"
  log_info "  - Batch size: $batch_size"
  log_info "  - Batch deployment enabled: $BATCH_DEPLOYMENT_ENABLED"
  
  if should_use_batch_deployment "$count"; then
    log_info "Using batch deployment for $count resources (batch size: $batch_size)"
    deploy_vms_with_disks_batch "$count" "$vi_type" "$batch_size"
  else
    log_info "Using regular deployment for $count resources"
    deploy_vms_with_disks "$count" "$vi_type"
  fi
}
