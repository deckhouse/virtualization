#!/usr/bin/env bash

# Statistics collection library for performance testing
# This module handles statistics gathering and analysis

# Source common utilities
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

gather_all_statistics() {
  local report_dir=${1:-"$REPORT_DIR/statistics"}
  local namespace=${2:-$NAMESPACE}
  local start_time=$(get_timestamp)
  
  log_info "Gathering all statistics to $report_dir"
  log_info "Start time: $(formatted_date $start_time)"
  
  local task_start=$(get_timestamp)
  task statistic:get-stat:all NAMESPACE=$namespace OUTPUT_DIR=$(realpath $report_dir)
  local task_end=$(get_timestamp)
  local task_duration=$((task_end - task_start))
  log_info "Task statistic:get-stat:all completed in $(format_duration $task_duration)"
  log_duration "Task statistic:get-stat:all" "$task_duration"

  mv tools/statistic/*.csv ${report_dir} 2>/dev/null || true
  
  local end_time=$(get_timestamp)
  local duration=$((end_time - start_time))
  log_info "All statistics gathering completed in $(format_duration $duration)"
  log_success "All statistics gathered"
}

gather_vm_statistics() {
  local report_dir=${1:-"$REPORT_DIR/statistics"}
  local namespace=${2:-$NAMESPACE}
  local start_time=$(get_timestamp)
  
  log_info "Gathering VM statistics to $report_dir"
  log_info "Start time: $(formatted_date $start_time)"
  
  local task_start=$(get_timestamp)
  task statistic:get-stat:vm NAMESPACE=$namespace OUTPUT_DIR=$(realpath $report_dir)
  local task_end=$(get_timestamp)
  local task_duration=$((task_end - task_start))
  log_info "Task statistic:get-stat:vm completed in $(format_duration $task_duration)"
  log_duration "Task statistic:get-stat:vm" "$task_duration"

  mv tools/statistic/*.csv ${report_dir} 2>/dev/null || true
  
  local end_time=$(get_timestamp)
  local duration=$((end_time - start_time))
  log_info "VM statistics gathering completed in $(format_duration $duration)"
  log_success "VM statistics gathered"
}

gather_vd_statistics() {
  local report_dir=${1:-"$REPORT_DIR/statistics"}
  local namespace=${2:-$NAMESPACE}
  local start_time=$(get_timestamp)
  
  log_info "Gathering VD statistics to $report_dir"
  log_info "Start time: $(formatted_date $start_time)"
  
  local task_start=$(get_timestamp)
  task statistic:get-stat:vd NAMESPACE=$namespace OUTPUT_DIR=$(realpath $report_dir)
  local task_end=$(get_timestamp)
  local task_duration=$((task_end - task_start))
  log_info "Task statistic:get-stat:vd completed in $(format_duration $task_duration)"
  log_duration "Task statistic:get-stat:vd" "$task_duration"

  mv tools/statistic/*.csv ${report_dir} 2>/dev/null || true
  
  local end_time=$(get_timestamp)
  local duration=$((end_time - start_time))
  log_info "VD statistics gathering completed in $(format_duration $duration)"
  log_success "VD statistics gathered"
}

gather_specific_vm_statistics() {
  local report_dir=${1:-"$REPORT_DIR/statistics"}
  local namespace=${2:-$NAMESPACE}
  local vm_count=${3:-0}
  local start_time=$(get_timestamp)
  
  log_info "Gathering statistics for specific VMs (count: $vm_count) to $report_dir"
  log_info "Start time: $(formatted_date $start_time)"
  
  local task_start=$(get_timestamp)
  task statistic:get-stat:vm NAMESPACE=$namespace OUTPUT_DIR=$(realpath $report_dir) VM_COUNT=$vm_count
  local task_end=$(get_timestamp)
  local task_duration=$((task_end - task_start))
  log_info "Task statistic:get-stat:vm for specific VMs completed in $(format_duration $task_duration)"
  log_duration "Task statistic:get-stat:vm specific" "$task_duration"

  mv tools/statistic/*.csv ${report_dir} 2>/dev/null || true
  
  local end_time=$(get_timestamp)
  local duration=$((end_time - start_time))
  log_info "Specific VM statistics gathering completed in $(format_duration $duration)"
  log_success "Specific VM statistics gathered"
}

collect_vpa() {
  local scenario_dir=$1
  local vpa_dir="$scenario_dir/vpa"
  local start_time=$(get_timestamp)
  
  mkdir -p ${vpa_dir}
  log_info "Collecting VPA data to $vpa_dir"
  log_info "Start time: $(formatted_date $start_time)"
  
  local list_start=$(get_timestamp)
  local VPAS=( $(kubectl -n d8-virtualization get vpa -o name 2>/dev/null || true) )
  local list_end=$(get_timestamp)
  local list_duration=$((list_end - list_start))
  log_info "VPA list retrieval completed in $(format_duration $list_duration)"
  log_duration "VPA list retrieval" "$list_duration"
  
  if [ ${#VPAS[@]} -eq 0 ]; then
    log_warning "No VPA resources found"
    return
  fi
  
  local collect_start=$(get_timestamp)
  for vpa in $VPAS; do
    vpa_name=$(echo $vpa | cut -d "/" -f2)
    file="vpa_${vpa_name}.yaml"
    kubectl -n d8-virtualization get $vpa -o yaml > "${vpa_dir}/${file}_$(formatted_date $(get_timestamp))" 2>/dev/null || true
  done

  # Additional wait using kubectl wait
  log_info "Additional wait for deployment to be fully available..."
  kubectl wait --for=condition=Available=True deployment/virtualization-controller -n d8-virtualization --timeout=300s
  local collect_end=$(get_timestamp)
  local collect_duration=$((collect_end - collect_start))
  log_info "VPA data collection completed in $(format_duration $collect_duration)"
  log_duration "VPA data collection" "$collect_duration"
  
  local end_time=$(get_timestamp)
  local duration=$((end_time - start_time))
  log_info "VPA collection completed in $(format_duration $duration)"
  log_success "VPA data collected"
}