#!/usr/bin/env bash

# Reporting library for performance testing
# This module handles report generation and summary

# Source common utilities
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

# Function to create summary report
create_summary_report() {
    local scenario_name="$1"
    local vi_type="$2"
    local scenario_dir="$3"
    local start_time="$4"
    local end_time="$5"
    local total_duration="$6"
    local cleanup_duration="${7:-0}"
    local deploy_duration="${8:-0}"
    local stats_duration="${9:-0}"
    local stop_duration="${10:-0}"
    local start_vm_duration="${11:-0}"
    local undeploy_duration="${12:-0}"
    local deploy_remaining_duration="${13:-0}"
    local vm_stats_duration="${14:-0}"
    local vm_ops_duration="${15:-0}"
    local vm_ops_stop_duration="${16:-0}"
    local vm_ops_start_duration="${17:-0}"
    local migration_duration="${18:-0}"
    local cleanup_ops_duration="${19:-0}"
    local migration_percent_duration="${20:-0}"
    local controller_duration="${21:-0}"
    local final_stats_duration="${22:-0}"
    local final_cleanup_duration="${23:-0}"
    
    local summary_file="$scenario_dir/summary.txt"
    
    # Calculate percentages safely
    local cleanup_percent=$(calculate_percentage "$cleanup_duration" "$total_duration")
    local deploy_percent=$(calculate_percentage "$deploy_duration" "$total_duration")
    local stats_percent=$(calculate_percentage "$stats_duration" "$total_duration")
    local stop_percent=$(calculate_percentage "$stop_duration" "$total_duration")
    local start_vm_percent=$(calculate_percentage "$start_vm_duration" "$total_duration")
    local undeploy_percent=$(calculate_percentage "$undeploy_duration" "$total_duration")
    local deploy_remaining_percent=$(calculate_percentage "$deploy_remaining_duration" "$total_duration")
    local vm_stats_percent=$(calculate_percentage "$vm_stats_duration" "$total_duration")
    local vm_ops_percent=$(calculate_percentage "$vm_ops_duration" "$total_duration")
    local vm_ops_stop_percent=$(calculate_percentage "$vm_ops_stop_duration" "$total_duration")
    local vm_ops_start_percent=$(calculate_percentage "$vm_ops_start_duration" "$total_duration")
    local migration_percent=$(calculate_percentage "$migration_duration" "$total_duration")
    local cleanup_ops_percent=$(calculate_percentage "$cleanup_ops_duration" "$total_duration")
    local migration_percent_percent=$(calculate_percentage "$migration_percent_duration" "$total_duration")
    local controller_percent=$(calculate_percentage "$controller_duration" "$total_duration")
    local final_stats_percent=$(calculate_percentage "$final_stats_duration" "$total_duration")
    local final_cleanup_percent=$(calculate_percentage "$final_cleanup_duration" "$total_duration")
    
    cat > "$summary_file" << EOF
================================================================================
                    PERFORMANCE TEST SUMMARY REPORT
================================================================================

Scenario: $scenario_name
Virtual Image Type: $vi_type
Test Date: $(formatted_date $start_time)
Duration: $(format_duration $total_duration)

================================================================================
                            EXECUTION TIMELINE
================================================================================

Start Time:     $(formatted_date $start_time)
End Time:       $(formatted_date $end_time)
Total Duration: $(format_duration $total_duration)

================================================================================
                            STEP DURATION BREAKDOWN
================================================================================

$(printf "%-55s %10s  %10s\n" "Phase" "Duration" "Percentage")
$(printf "%-55s %10s  %10s\n" "-------------------------------------------------------" "----------" "----------")
$(printf "%-55s %10s  %10s\n" "Cleanup" "$(format_duration $cleanup_duration)" "$(printf "%5.1f" $cleanup_percent)%")
$(printf "%-55s %10s  %10s\n" "Deploy VMs [$MAIN_COUNT_RESOURCES]" "$(format_duration $deploy_duration)" "$(printf "%5.1f" $deploy_percent)%")
$(printf "%-55s %10s  %10s\n" "Statistics Collection" "$(format_duration $stats_duration)" "$(printf "%5.1f" $stats_percent)%")
$(printf "%-55s %10s  %10s\n" "VM Stop [$MAIN_COUNT_RESOURCES]" "$(format_duration $stop_duration)" "$(printf "%5.1f" $stop_percent)%")
$(printf "%-55s %10s  %10s\n" "VM Start [$MAIN_COUNT_RESOURCES]" "$(format_duration $start_vm_duration)" "$(printf "%5.1f" $start_vm_percent)%")
$(printf "%-55s %10s  %10s\n" "VM Undeploy 10% VMs [$PERCENT_RESOURCES] (keeping disks)" "$(format_duration $undeploy_duration)" "$(printf "%5.1f" $undeploy_percent)%")
$(printf "%-55s %10s  %10s\n" "Deploying 10% VMs [$PERCENT_RESOURCES] (keeping disks)" "$(format_duration $deploy_remaining_duration)" "$(printf "%5.1f" $deploy_remaining_percent)%")
$(printf "%-55s %10s  %10s\n" "VM Statistics: Deploying 10% VMs ([$PERCENT_RESOURCES] VMs)" "$(format_duration $vm_stats_duration)" "$(printf "%5.1f" $vm_stats_percent)%")
$(printf "%-55s %10s  %10s\n" "VM Operations: Stopping VMs [$PERCENT_RESOURCES]" "$(format_duration $vm_ops_stop_duration)" "$(printf "%5.1f" $vm_ops_stop_percent)%")
$(printf "%-55s %10s  %10s\n" "VM Operations: Start VMs [$PERCENT_RESOURCES]" "$(format_duration $vm_ops_start_duration)" "$(printf "%5.1f" $vm_ops_start_percent)%")
$(printf "%-55s %10s  %10s\n" "Migration Setup (${MIGRATION_PERCENTAGE_5}% - ${MIGRATION_5_COUNT} VMs)" "$(format_duration $migration_duration)" "$(printf "%5.1f" $migration_percent)%")
$(printf "%-55s %10s  %10s\n" "Stop Migration ${MIGRATION_PERCENTAGE_5}% (${MIGRATION_5_COUNT} VMs)" "$(format_duration $cleanup_ops_duration)" "$(printf "%5.1f" $cleanup_ops_percent)%")
$(printf "%-55s %10s  %10s\n" "Migration Percentage ${MIGRATION_10_COUNT} VMs (10%)" "$(format_duration $migration_percent_duration)" "$(printf "%5.1f" $migration_percent_percent)%")
$(printf "%-55s %10s  %10s\n" "Controller Restart" "$(format_duration $controller_duration)" "$(printf "%5.1f" $controller_percent)%")
$(printf "%-55s %10s  %10s\n" "Final Statistics" "$(format_duration $final_stats_duration)" "$(printf "%5.1f" $final_stats_percent)%")
$(printf "%-55s %10s  %10s\n" "Final Cleanup" "$(format_duration $final_cleanup_duration)" "$(printf "%5.1f" $final_cleanup_percent)%")

================================================================================
                            PERFORMANCE METRICS
================================================================================

$(printf "%-25s %10s\n" "Total VMs Tested:" "$MAIN_COUNT_RESOURCES")
$(printf "%-25s %10s\n" "VM Deployment Time:" "$(format_duration $deploy_duration)")
$(printf "%-25s %10s\n" "VM Stop Time:" "$(format_duration $stop_duration)")
$(printf "%-25s %10s\n" "VM Start Time:" "$(format_duration $start_vm_duration)")
$(printf "%-25s %10s\n" "Controller Restart Time:" "$(format_duration $controller_duration)")
$(printf "%-25s %10s\n" "Migration 5% Time:" "$(format_duration $migration_duration)")
$(printf "%-25s %10s\n" "Migration 10% Time:" "$(format_duration $migration_percent_duration)")
================================================================================
                            FILES GENERATED
================================================================================

$(printf "%-25s %s\n" "Log File:" "$scenario_dir/test.log")
$(printf "%-25s %s\n" "VM Operations Log:" "$scenario_dir/vm_operations.log")
$(printf "%-25s %s\n" "Statistics Directory:" "$scenario_dir/statistics/")
$(printf "%-25s %s\n" "VPA Data Directory:" "$scenario_dir/vpa/")
$(printf "%-25s %s\n" "Summary Report:" "$scenario_dir/summary.txt")

================================================================================
EOF

    log_info "Summary report created: $summary_file"
}

