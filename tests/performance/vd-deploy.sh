#!/usr/bin/env bash

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

MAX_BATCH_SIZE=1000

NAMESPACE="perf"
SLEEP_TIME=5

# ===
LOG_FILE="vd-deploy_$(date +"%Y%m%d_%H%M%S").log"
# ===

get_current_date() {
    date +"%Y-%m-%d %H:%M:%S"
}

get_current_date() {
    date +"%H:%M:%S %d-%m-%Y"
}

get_timestamp() {
    date +%s
}

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

deploy_disks_only_batch() {
  local total_count=$1
  local vi_type=$2
  local batch_size=${3:-$MAX_BATCH_SIZE}
  local start_time=$(get_timestamp)
  
  log_info "Starting batch deployment of $total_count disks from $vi_type"
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
    
    # Deploy current batch of disks (COUNT should be cumulative, not absolute)
    local cumulative_count=$((deployed_count + current_batch_size))
    log_info "Deploying disk batch $batch_number: $current_batch_size new disks (total will be: $cumulative_count)"
    task apply:disks \
        COUNT=$cumulative_count \
        NAMESPACE=$NAMESPACE \
        STORAGE_CLASS=$(get_default_storage_class) \
        VIRTUALDISK_TYPE=virtualDisk \
        VIRTUALIMAGE_TYPE=$vi_type
    
    # Wait for current batch to be ready
    wait_vd $SLEEP_TIME
    
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
  
  log_success "Batch disk deployment completed: $deployed_count disks in $formatted_duration"
  log_info "Average time per disk: $(( total_duration / deployed_count )) seconds"
  
  echo "$total_duration"
}

# =======
TOTAL_VD=15000

log_info "Start Deploying disks [$TOTAL_VD]"
deploy_disks_only_batch $TOTAL_VD "persistentVolumeClaim" 1000

log_success "Disk deployment completed"

task destroy:disks \
