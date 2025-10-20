#!/usr/bin/env bash

# Scenarios library for performance testing
# This module handles scenario execution orchestration

# Source all other libraries
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"
source "$(dirname "${BASH_SOURCE[0]}")/vm_operations.sh"
source "$(dirname "${BASH_SOURCE[0]}")/migration.sh"
source "$(dirname "${BASH_SOURCE[0]}")/statistics.sh"
source "$(dirname "${BASH_SOURCE[0]}")/controller.sh"
source "$(dirname "${BASH_SOURCE[0]}")/reporting.sh"

# Function to run a single scenario
run_scenario() {
  local scenario_name=$1
  local vi_type=$2
  
  log_info "=== Starting scenario: $scenario_name with $vi_type ==="
  
  # Initialize logging and create report directory
  init_logging "$scenario_name" "$vi_type"
  remove_report_dir "$REPORT_DIR/${scenario_name}_${vi_type}"
  local scenario_dir=$(create_report_dir "$scenario_name" "$vi_type")
  
  # Clean up any existing resources
  log_info "Cleaning up existing resources"
  log_step_start "Cleanup up existing resources"
  local cleanup_start=$(get_timestamp)
  stop_migration
  remove_vmops
  undeploy_resources
  local cleanup_end=$(get_timestamp)
  local cleanup_duration=$((cleanup_end - cleanup_start))
  log_info "Cleanup completed in $(format_duration $cleanup_duration)"
  log_step_end "Cleanup up existing resources" "$cleanup_duration"
  
  local start_time=$(get_timestamp)
  log_info "== Scenario started at $(formatted_date $start_time) =="
  
  # Main test sequence
  log_step_start "Deploy VMs [$MAIN_COUNT_RESOURCES]"
  local deploy_start=$(get_timestamp)
  deploy_vms_with_disks $MAIN_COUNT_RESOURCES $vi_type
  local deploy_end=$(get_timestamp)
  local deploy_duration=$((deploy_end - deploy_start))
  log_info "VM [$MAIN_COUNT_RESOURCES] deploy completed in $(format_duration $deploy_duration)"
  log_step_end "End VM Deployment [$MAIN_COUNT_RESOURCES]" "$deploy_duration"
  
  log_step_start "Start Statistics Collection"
  local stats_start=$(get_timestamp)
  gather_all_statistics "$scenario_dir/statistics"
  collect_vpa "$scenario_dir"
  local stats_end=$(get_timestamp)
  local stats_duration=$((stats_end - stats_start))
  log_info "Statistics collection completed in $(format_duration $stats_duration)"
  log_step_end "End Statistics Collection" "$stats_duration"
  
  log_info "Waiting 10 seconds before stopping VMs"
  sleep 10
  
  log_info "Stopping all VMs [$MAIN_COUNT_RESOURCES]"
  log_step_start "VM Stop"
  local stop_start=$(get_timestamp)
  stop_vm
  local stop_end=$(get_timestamp)
  local stop_duration=$((stop_end - stop_start))
  log_info "VM stop completed in $(format_duration $stop_duration)"
  log_step_end "End Stopping VMs [$MAIN_COUNT_RESOURCES]" "$stop_duration"
  
  log_info "Waiting 10 seconds before starting VMs"
  sleep 10
  
  log_info "Starting all VMs [$MAIN_COUNT_RESOURCES]"
  log_step_start "VM Start [$MAIN_COUNT_RESOURCES]"
  local start_vm_start=$(get_timestamp)
  start_vm
  local start_vm_end=$(get_timestamp)
  local start_vm_duration=$((start_vm_end - start_vm_start))
  log_info "VM start completed in $(format_duration $start_vm_duration)"
  log_step_end "End VM Start [$MAIN_COUNT_RESOURCES]" "$start_vm_duration"
  
  log_info "Undeploying 10% VMs [$PERCENT_RESOURCES] (keeping disks)"
  log_step_start "VM Undeploy 10% VMs [$PERCENT_RESOURCES] (keeping disks)"
  local undeploy_start=$(get_timestamp)
  undeploy_vms_only $PERCENT_RESOURCES
  local undeploy_end=$(get_timestamp)
  local undeploy_duration=$((undeploy_end - undeploy_start))
  log_info "VM Undeploy 10% VMs [$PERCENT_RESOURCES] completed in $(format_duration $undeploy_duration)"
  log_step_end "VM Undeploy 10% VMs [$PERCENT_RESOURCES] (keeping disks)" "$undeploy_duration"

  # CORRECTED ORDER: Deploy 10% VMs and gather statistics (пункт 8)
  log_info "Deploying 10% VMs ([$PERCENT_RESOURCES] VMs) and gathering statistics"
  log_step_start "Deploying 10% VMs [$PERCENT_RESOURCES]"
  local deploy_remaining_start=$(get_timestamp)
  deploy_vms_only $MAIN_COUNT_RESOURCES
  local deploy_remaining_end=$(get_timestamp)
  local deploy_remaining_duration=$((deploy_remaining_end - deploy_remaining_start))
  log_info "10% VMs deployment completed in $(format_duration $deploy_remaining_duration)"
  log_step_end "End Deploying 10% VMs ([$PERCENT_RESOURCES] VMs) " "$deploy_remaining_duration"
  
  # Gather statistics for 10% VMs (пункт 8.1)
  log_step_start "VM Statistics: Deploying 10% VMs ([$PERCENT_RESOURCES] VMs)"
  local vm_stats_start=$(get_timestamp)
  gather_specific_vm_statistics "$scenario_dir/statistics" "$NAMESPACE" "$PERCENT_RESOURCES"
  local vm_stats_end=$(get_timestamp)
  local vm_stats_duration=$((vm_stats_end - vm_stats_start))
  log_info "VM statistics collection completed in $(format_duration $vm_stats_duration)"
  log_step_end "End VM Statistics: Deploying 10% VMs ([$PERCENT_RESOURCES] VMs)" "$vm_stats_duration"
  
  # Start 5% migration in background (пункт 7)
  local migration_duration_time="0m"
  log_info "Starting migration test ${MIGRATION_PERCENTAGE_5}% (${MIGRATION_5_COUNT} VMs)"
  log_step_start "Migration Setup"
  local migration_start=$(get_timestamp)
  start_migration $migration_duration_time $MIGRATION_PERCENTAGE_5
  local migration_end=$(get_timestamp)
  local migration_duration=$((migration_end - migration_start))
  log_info "Migration test ${MIGRATION_PERCENTAGE_5}% VMs setup completed in $(format_duration $migration_duration)"
  log_step_end "Migration Setup ${MIGRATION_PERCENTAGE_5}% (${MIGRATION_5_COUNT} VMs) Started" "$migration_duration"

  # VM operations test - stop/start 10% VMs (пункты 9-10)
  log_info "Testing VM stop/start operations for 10% VMs"
  log_step_start "VM Operations"
  local vm_ops_start=$(get_timestamp)
  
  log_step_start "VM Operations: Stopping VMs [$PERCENT_RESOURCES]"
  local vm_ops_stop_start=$(get_timestamp)
  stop_vm $PERCENT_RESOURCES
  local vm_ops_stop_end=$(get_timestamp)
  local vm_ops_stop_duration=$((vm_ops_stop_end - vm_ops_stop_start))
  log_step_end "VM Operations: Stopping VMs [$PERCENT_RESOURCES]" "$vm_ops_stop_duration"
  
  sleep 2
  
  log_step_start "VM Operations: Start VMs [$PERCENT_RESOURCES]"
  local vm_ops_start_vm_start=$(get_timestamp)
  start_vm $PERCENT_RESOURCES
  local vm_ops_start_vm_end=$(get_timestamp)
  local vm_ops_start_vm_duration=$((vm_ops_start_vm_end - vm_ops_start_vm_start))
  log_step_end "VM Operations: Start VMs [$PERCENT_RESOURCES]" "$vm_ops_start_vm_duration"
  
  local vm_ops_end=$(get_timestamp)
  local vm_ops_duration=$((vm_ops_end - vm_ops_start))
  log_info "VM operations test completed in $(format_duration $vm_ops_duration)"
  log_step_end "VM Operations: Stop/Start VMs [$PERCENT_RESOURCES]" "$vm_ops_duration"
  
  # Stop migration and wait for completion (пункт 11)
  log_step_start "Stop Migration ${MIGRATION_PERCENTAGE_5}% (${MIGRATION_5_COUNT} VMs)"
  local cleanup_ops_start=$(get_timestamp)
  stop_migration
  wait_migration_completion
  remove_vmops
  local cleanup_ops_end=$(get_timestamp)
  local cleanup_ops_duration=$((cleanup_ops_end - cleanup_ops_start))
  log_info "Migration stop and cleanup completed in $(format_duration $cleanup_ops_duration)"
  log_step_end "Stop Migration ${MIGRATION_PERCENTAGE_5}% (${MIGRATION_5_COUNT} VMs)" "$cleanup_ops_duration"
  
  # Migration percentage test - Migrate 10% VMs (пункт 12)
  log_info "Testing migration of ${MIGRATION_10_COUNT} VMs (10%)"
  log_step_start "Migration Percentage ${MIGRATION_10_COUNT} VMs (10%)"
  local migration_percent_start=$(get_timestamp)
  migration_percent_vms $MIGRATION_10_COUNT
  local migration_percent_end=$(get_timestamp)
  local migration_percent_duration=$((migration_percent_end - migration_percent_start))
  log_info "Migration percentage test completed in $(format_duration $migration_percent_duration)"
  log_step_end "End Migration Percentage ${MIGRATION_10_COUNT} VMs (10%)" "$migration_percent_duration"

  log_info "Waiting 30 seconds"
  sleep 30

  # Controller restart test
  log_info "Testing controller restart with 1 VM creation"
  log_step_start "Controller Restart"
  local controller_start=$(get_timestamp)
  
  # Stop controller first
  stop_virtualization_controller
  
  # Create 1 VM and disk while controller is stopped
  log_info "Creating 1 VM and disk while controller is stopped [$((MAIN_COUNT_RESOURCES + 1)) VMs total]"
  local vm_creation_start=$(get_timestamp)
  local vm_creation_end=$(get_timestamp)
  local vm_creation_duration=$((vm_creation_end - vm_creation_start))
  log_info "VM creation while controller stopped completed in $(format_duration $vm_creation_duration)"
  
  # Start controller and measure time for VM to become ready
  log_info "Starting controller and waiting for VM to become ready"
  local controller_start_time=$(get_timestamp)
  start_virtualization_controller
  create_vm_while_controller_stopped $vi_type
  wait_for_new_vm_after_controller_start
  local controller_end_time=$(get_timestamp)
  local controller_duration=$((controller_end_time - controller_start))
  local vm_ready_duration=$((controller_end_time - controller_start_time))
  
  log_info "Controller restart test completed in $(format_duration $controller_duration)"
  log_info "VM became ready after controller start in $(format_duration $vm_ready_duration)"
  log_step_end "Controller Restart" "$controller_duration"
  
  # Final deployment and statistics
  # log_info "Final deployment and statistics collection"
  # log_step_start "Final Deployment"
  # local final_deploy_start=$(get_timestamp)
  # deploy_vms_with_disks $MAIN_COUNT_RESOURCES $vi_type
  # wait_for_resources "all"
  # local final_deploy_end=$(get_timestamp)
  # local final_deploy_duration=$((final_deploy_end - final_deploy_start))
  # log_info "Final deployment completed in $(format_duration $final_deploy_duration)"
  # log_step_end "Final Deployment" "$final_deploy_duration"
  
  log_step_start "Final Statistics"
  local final_stats_start=$(get_timestamp)
  gather_all_statistics "$scenario_dir/statistics"
  collect_vpa "$scenario_dir"
  local final_stats_end=$(get_timestamp)
  local final_stats_duration=$((final_stats_end - final_stats_start))
  log_info "Final statistics collection completed in $(format_duration $final_stats_duration)"
  log_step_end "Final Statistics" "$final_stats_duration"
  
  log_info "Waiting 30 second before cleanup"
  sleep 30
  
  log_step_start "Final Cleanup"
  local final_cleanup_start=$(get_timestamp)
  undeploy_resources
  local final_cleanup_end=$(get_timestamp)
  local final_cleanup_duration=$((final_cleanup_end - final_cleanup_start))
  log_info "Final cleanup completed in $(format_duration $final_cleanup_duration)"
  log_step_end "Final Cleanup" "$final_cleanup_duration"
  
  local end_time=$(get_timestamp)
  local duration=$((end_time - start_time))
  local formatted_duration=$(format_duration "$duration")
  
  log_success "Scenario $scenario_name completed in $formatted_duration"
  log_info "Scenario ended at $(formatted_date $end_time)"
  
  # Create summary report
  create_summary_report "$scenario_name" "$vi_type" "$scenario_dir" "$start_time" "$end_time" "$duration" \
    "$cleanup_duration" "$deploy_duration" "$stats_duration" "$stop_duration" "$start_vm_duration" \
    "$undeploy_duration" "$deploy_remaining_duration" "$vm_stats_duration" \
    "$vm_ops_duration" "$vm_ops_stop_duration" "$vm_ops_start_vm_duration" "$migration_duration" "$cleanup_ops_duration" "$migration_percent_duration" \
    "$controller_duration" "$final_stats_duration" "$final_cleanup_duration"
  
  # Summary of all step durations
  log_info "=== Scenario $scenario_name Duration Summary ==="
  log_duration "Cleanup" "$cleanup_duration"
  log_duration "VM Deployment" "$deploy_duration"
  log_duration "Statistics Collection" "$stats_duration"
  log_duration "VM Stop" "$stop_duration"
  log_duration "VM Start" "$start_vm_duration"
  log_duration "VM Undeploy" "$undeploy_duration"
  log_duration "Remaining VMs Deploy" "$deploy_remaining_duration"
  log_duration "VM Statistics" "$vm_stats_duration"
  log_duration "Migration Setup" "$migration_duration"
  log_duration "VM Operations" "$vm_ops_duration"
  log_duration "VM Operations: Stopping VMs" "$vm_ops_stop_duration"
  log_duration "VM Operations: Start VMs" "$vm_ops_start_vm_duration"
  log_duration "Migration Cleanup" "$cleanup_ops_duration"
  log_duration "Migration Percentage" "$migration_percent_duration"
  log_duration "Controller Restart" "$controller_duration"
  log_duration "Final Statistics" "$final_stats_duration"
  log_duration "Final Cleanup" "$final_cleanup_duration"
  log_duration "Total Scenario Duration" "$duration"
  log_info "=== End Duration Summary ==="
}
