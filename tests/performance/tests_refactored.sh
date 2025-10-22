#!/usr/bin/env bash

set -eEo pipefail
# set -x

# Performance testing script for Kubernetes Virtual Machines - Refactored Version
# This script provides both full scenario execution and individual step execution capabilities

# Source all library modules
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib/common.sh"
source "$SCRIPT_DIR/lib/vm_operations.sh"
source "$SCRIPT_DIR/lib/migration.sh"
source "$SCRIPT_DIR/lib/statistics.sh"
source "$SCRIPT_DIR/lib/controller.sh"
source "$SCRIPT_DIR/lib/reporting.sh"
source "$SCRIPT_DIR/lib/scenarios.sh"

# Parse command line arguments
parse_arguments() {
  while [[ $# -gt 0 ]]; do
    case $1 in
      -s|--scenario)
        SCENARIO_NUMBER="$2"
        shift 2
        ;;
      -c|--count)
        MAIN_COUNT_RESOURCES="$2"
        shift 2
        ;;
      --batch-size)
        MAX_BATCH_SIZE="$2"
        shift 2
        ;;
      --enable-batch)
        BATCH_DEPLOYMENT_ENABLED=true
        shift
        ;;
      --clean-reports)
        CLEAN_REPORTS=true
        shift
        ;;
      --step)
        INDIVIDUAL_STEP="$2"
        shift 2
        ;;
      --from-step)
        FROM_STEP="$2"
        shift 2
        ;;
      --list-steps)
        list_available_steps
        exit 0
        ;;
      --scenario-dir)
        SCENARIO_DIR="$2"
        shift 2
        ;;
      --vi-type)
        VI_TYPE="$2"
        shift 2
        ;;
      --no-pre-cleanup)
        NO_PRE_CLEANUP=true
        shift
        ;;
      --no-post-cleanup)
        NO_POST_CLEANUP=true
        shift
        ;;
      -h|--help)
        show_help
        exit 0
        ;;
      *)
        echo "Unknown option: $1"
        show_help
        exit 1
        ;;
    esac
  done
}

show_help() {
  cat << EOF
Usage: $0 [OPTIONS]

Performance testing script for Kubernetes Virtual Machines

OPTIONS:
  -s, --scenario NUMBER    Scenario number to run (1 or 2, default: 1)
  -c, --count NUMBER       Number of resources to create (default: 2)
  --batch-size NUMBER      Maximum resources per batch (default: 1200)
  --enable-batch          Force batch deployment mode
  --step STEP_NAME         Run a specific step only
  --from-step STEP_NAME    Run all steps starting from STEP_NAME
  --list-steps             List all available steps
  --scenario-dir DIR       Directory for scenario data (required for individual steps)
  --vi-type TYPE           Virtual image type (required for some steps)
  --clean-reports          Clean all report directories before running
  --no-pre-cleanup         Do not cleanup resources before running
  --no-post-cleanup        Do not cleanup resources after running
  -h, --help              Show this help message

EXAMPLES:
  # Full scenario execution (original behavior)
  $0                       # Run scenario 1 with 2 resources (default)
  $0 -s 1 -c 4            # Run scenario 1 with 4 resources
  $0 -s 2 -c 10           # Run scenario 2 with 10 resources
  $0 -c 15000 --batch-size 1200 # Deploy 15000 resources in batches of 1200

  # Individual step execution (new feature)
  $0 --list-steps         # List available steps
  $0 --step cleanup --scenario-dir /path/to/scenario --vi-type persistentVolumeClaim
  $0 --step vm-deployment --scenario-dir /path/to/scenario --vi-type persistentVolumeClaim
  $0 --step statistics-collection --scenario-dir /path/to/scenario
  $0 --step vm-operations --scenario-dir /path/to/scenario
  $0 --step vm-undeploy-deploy --scenario-dir /path/to/scenario
  $0 --step vm-operations-test --scenario-dir /path/to/scenario
  $0 --step migration-tests --scenario-dir /path/to/scenario
  $0 --step migration-parallel-2x --scenario-dir /path/to/scenario
  $0 --step migration-parallel-4x --scenario-dir /path/to/scenario
  $0 --step migration-parallel-8x --scenario-dir /path/to/scenario
  $0 --step controller-restart --scenario-dir /path/to/scenario --vi-type persistentVolumeClaim
  $0 --step drain-node --scenario-dir /path/to/scenario
  $0 --step final-operations --scenario-dir /path/to/scenario

  # Run from a step
  $0 --from-step vm-operations --scenario-dir /path/to/scenario --vi-type persistentVolumeClaim

  # Skip cleanup before/after
  $0 --no-pre-cleanup --no-post-cleanup

SCENARIOS:
  1 - persistentVolumeClaim (default)
  2 - containerRegistry (currently disabled)

BATCH DEPLOYMENT:
  For large deployments (>1200 resources), the script automatically uses batch deployment.
  Each batch deploys up to 1200 resources with 30-second delays between batches.
  Use --batch-size to customize batch size and --enable-batch to force batch mode.

AVAILABLE STEPS:
  1. cleanup - Clean up existing resources
  2. vm-deployment - Deploy VMs with disks
  3. statistics-collection - Gather initial statistics
  4. vm-operations - Stop and start all VMs
  5. vm-undeploy-deploy - Undeploy and redeploy 10% VMs
  6. vm-operations-test - Test stop/start operations on 10% VMs
  7. migration-tests - Run migration tests (5% and 10%)
  8. migration-parallel-2x - Migrate with parallelMigrationsPerCluster at 2x nodes
  9. migration-parallel-4x - Migrate with parallelMigrationsPerCluster at 4x nodes
 10. migration-parallel-8x - Migrate with parallelMigrationsPerCluster at 8x nodes
 11. controller-restart - Test controller restart with VM creation
 12. drain-node - Run drain node workload
 13. final-operations - Final statistics and optional cleanup

EOF
}

list_available_steps() {
  cat << EOF
Available test steps:

1. cleanup - Clean up existing resources
2. vm-deployment - Deploy VMs with disks
3. statistics-collection - Gather initial statistics
4. vm-operations - Stop and start all VMs
5. vm-undeploy-deploy - Undeploy and redeploy 10% VMs
6. vm-operations-test - Test stop/start operations on 10% VMs
7. migration-tests - Run migration tests (5% and 10%)
8. migration-parallel-2x - Migrate with parallelMigrationsPerCluster at 2x nodes
9. migration-parallel-4x - Migrate with parallelMigrationsPerCluster at 4x nodes
10. migration-parallel-8x - Migrate with parallelMigrationsPerCluster at 8x nodes
8. controller-restart - Test controller restart with VM creation
11. drain-node - Run drain node workload
12. final-operations - Final statistics and optional cleanup

Usage: $0 --step STEP_NAME --scenario-dir DIR [--vi-type TYPE]
EOF
}

# Helper function to get step number
get_step_number() {
  local step_name="$1"
  local step_number=1
  for step in "${ALL_STEPS[@]}"; do
    if [ "$step" = "$step_name" ]; then
      echo "$step_number"
      return
    fi
    step_number=$((step_number + 1))
  done
  echo "0"
}

# Individual step execution functions
run_step_cleanup() {
  local scenario_dir="$1"
  local vi_type="$2"
  local step_number=$(get_step_number "cleanup")
  
  log_info "=== Running Step $step_number: cleanup ==="
  init_logging "step_cleanup" "$vi_type" "$MAIN_COUNT_RESOURCES"
  
  log_step_start "Cleanup existing resources"
  local cleanup_start=$(get_timestamp)
  stop_migration
  remove_vmops
  undeploy_resources
  local cleanup_end=$(get_timestamp)
  local cleanup_duration=$((cleanup_end - cleanup_start))
  log_step_end "Cleanup existing resources" "$cleanup_duration"
  
  log_success "Cleanup step completed"
}

run_step_vm_deployment() {
  local scenario_dir="$1"
  local vi_type="$2"
  local step_number=$(get_step_number "vm-deployment")
  
  log_info "=== Running Step $step_number: vm-deployment ==="
  init_logging "step_vm-deployment" "$vi_type" "$MAIN_COUNT_RESOURCES"
  
  # Check cluster resources before deployment
  log_step_start "Check cluster resources"
  check_cluster_resources $MAIN_COUNT_RESOURCES
  log_step_end "Check cluster resources" "0"
  
  log_step_start "Deploy VMs [$MAIN_COUNT_RESOURCES]"
  local deploy_start=$(get_timestamp)
  deploy_vms_with_disks_smart $MAIN_COUNT_RESOURCES $vi_type
  local deploy_end=$(get_timestamp)
  local deploy_duration=$((deploy_end - deploy_start))
  log_step_end "Deploy VMs [$MAIN_COUNT_RESOURCES]" "$deploy_duration"
  
  log_success "VM deployment step completed"
}

run_step_statistics_collection() {
  local scenario_dir="$1"
  local vi_type="$2"
  local step_number=$(get_step_number "statistics-collection")
  
  log_info "=== Running Step $step_number: statistics-collection ==="
  init_logging "step_statistics-collection" "$vi_type" "$MAIN_COUNT_RESOURCES"
  
  log_step_start "Statistics Collection"
  local stats_start=$(get_timestamp)
  gather_all_statistics "$scenario_dir/statistics"
  collect_vpa "$scenario_dir"
  local stats_end=$(get_timestamp)
  local stats_duration=$((stats_end - stats_start))
  log_step_end "Statistics Collection" "$stats_duration"
  
  log_success "Statistics collection step completed"
}

run_step_vm_operations() {
  local scenario_dir="$1"
  local vi_type="$2"
  local step_number=$(get_step_number "vm-operations")
  
  log_info "=== Running Step $step_number: vm-operations ==="
  init_logging "step_vm-operations" "$vi_type" "$MAIN_COUNT_RESOURCES"
  
  log_info "Stopping all VMs [$MAIN_COUNT_RESOURCES]"
  log_step_start "VM Stop"
  local stop_start=$(get_timestamp)
  stop_vm
  local stop_end=$(get_timestamp)
  local stop_duration=$((stop_end - stop_start))
  log_step_end "VM Stop" "$stop_duration"
  
  log_info "Waiting 10 seconds before starting VMs"
  sleep 10
  
  log_info "Starting all VMs [$MAIN_COUNT_RESOURCES]"
  log_step_start "VM Start"
  local start_vm_start=$(get_timestamp)
  start_vm
  local start_vm_end=$(get_timestamp)
  local start_vm_duration=$((start_vm_end - start_vm_start))
  log_step_end "VM Start" "$start_vm_duration"
  
  log_success "VM operations step completed"
}

run_step_vm_undeploy_deploy() {
  local scenario_dir="$1"
  local vi_type="$2"
  local step_number=$(get_step_number "vm-undeploy-deploy")
  
  log_info "=== Running Step $step_number: vm-undeploy-deploy ==="
  init_logging "step_vm-undeploy-deploy" "$vi_type" "$MAIN_COUNT_RESOURCES"
  
  log_info "Undeploying 10% VMs [$PERCENT_RESOURCES] (keeping disks)"
  log_step_start "VM Undeploy 10% VMs [$PERCENT_RESOURCES]"
  local undeploy_start=$(get_timestamp)
  undeploy_vms_only $PERCENT_RESOURCES
  local undeploy_end=$(get_timestamp)
  local undeploy_duration=$((undeploy_end - undeploy_start))
  log_step_end "VM Undeploy 10% VMs [$PERCENT_RESOURCES]" "$undeploy_duration"
  
  log_info "Deploying 10% VMs ([$PERCENT_RESOURCES] VMs)"
  log_step_start "Deploying 10% VMs [$PERCENT_RESOURCES]"
  local deploy_remaining_start=$(get_timestamp)
  deploy_vms_only_smart $MAIN_COUNT_RESOURCES
  local deploy_remaining_end=$(get_timestamp)
  local deploy_remaining_duration=$((deploy_remaining_end - deploy_remaining_start))
  log_step_end "Deploying 10% VMs [$PERCENT_RESOURCES]" "$deploy_remaining_duration"
  
  log_success "VM undeploy/deploy step completed"
}

run_step_vm_operations_test() {
  local scenario_dir="$1"
  local vi_type="$2"
  local step_number=$(get_step_number "vm-operations-test")
  
  log_info "=== Running Step $step_number: vm-operations-test ==="
  init_logging "step_vm-operations-test" "$vi_type" "$MAIN_COUNT_RESOURCES"
  
  log_info "Testing VM stop/start operations for 10% VMs"
  log_step_start "VM Operations Test"
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
  log_step_end "VM Operations Test" "$vm_ops_duration"
  
  log_success "VM operations test step completed"
}

run_step_migration_tests() {
  local scenario_dir="$1"
  local vi_type="$2"
  local step_number=$(get_step_number "migration-tests")
  
  log_info "=== Running Step $step_number: migration-tests ==="
  init_logging "step_migration-tests" "$vi_type" "$MAIN_COUNT_RESOURCES"
  
  # Start 5% migration in background
  local migration_duration_time="0m"
  log_info "Starting migration test ${MIGRATION_PERCENTAGE_5}% (${MIGRATION_5_COUNT} VMs)"
  log_step_start "Migration Setup"
  local migration_start=$(get_timestamp)
  start_migration $migration_duration_time $MIGRATION_PERCENTAGE_5
  local migration_end=$(get_timestamp)
  local migration_duration=$((migration_end - migration_start))
  log_info "Migration test ${MIGRATION_PERCENTAGE_5}% VMs setup completed in $(format_duration $migration_duration)"
  log_step_end "Migration Setup ${MIGRATION_PERCENTAGE_5}% (${MIGRATION_5_COUNT} VMs) Started" "$migration_duration"
  
  # VM operations test - stop/start 10% VMs while migration is running in background
  log_info "Testing VM stop/start operations for 10% VMs while migration is running"
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
  
  # Stop migration and wait for completion
  log_step_start "Stop Migration ${MIGRATION_PERCENTAGE_5}% (${MIGRATION_5_COUNT} VMs)"
  local cleanup_ops_start=$(get_timestamp)
  stop_migration
  wait_migration_completion
  remove_vmops
  local cleanup_ops_end=$(get_timestamp)
  local cleanup_ops_duration=$((cleanup_ops_end - cleanup_ops_start))
  log_info "Migration stop and cleanup completed in $(format_duration $cleanup_ops_duration)"
  log_step_end "Stop Migration ${MIGRATION_PERCENTAGE_5}% (${MIGRATION_5_COUNT} VMs)" "$cleanup_ops_duration"
  
  # Migration percentage test - Migrate 10% VMs
  log_info "Testing migration of ${MIGRATION_10_COUNT} VMs (10%)"
  log_step_start "Migration Percentage ${MIGRATION_10_COUNT} VMs (10%)"
  local migration_percent_start=$(get_timestamp)
  migration_percent_vms $MIGRATION_10_COUNT
  local migration_percent_end=$(get_timestamp)
  local migration_percent_duration=$((migration_percent_end - migration_percent_start))
  log_step_end "Migration Percentage ${MIGRATION_10_COUNT} VMs (10%)" "$migration_percent_duration"
  
  log_success "Migration tests step completed"
}

run_step_controller_restart() {
  local scenario_dir="$1"
  local vi_type="$2"
  local step_number=$(get_step_number "controller-restart")
  
  log_info "=== Running Step $step_number: controller-restart ==="
  init_logging "step_controller-restart" "$vi_type" "$MAIN_COUNT_RESOURCES"
  
  log_info "Testing controller restart with 1 VM creation"
  log_step_start "Controller Restart"
  local controller_start=$(get_timestamp)
  
  # Stop controller first
  stop_virtualization_controller
  
  # Create 1 VM and disk while controller is stopped
  log_info "Creating 1 VM and disk while controller is stopped [$((MAIN_COUNT_RESOURCES + 1)) VMs total]"
  create_vm_while_controller_stopped $vi_type
  
  # Start controller and measure time for VM to become ready
  log_info "Starting controller and waiting for VM to become ready"
  start_virtualization_controller
  wait_for_new_vm_after_controller_start
  local controller_end_time=$(get_timestamp)
  local controller_duration=$((controller_end_time - controller_start))
  
  log_info "Controller restart test completed in $(format_duration $controller_duration)"
  log_step_end "Controller Restart" "$controller_duration"
  
  log_success "Controller restart step completed"
}

run_step_final_operations() {
  local scenario_dir="$1"
  local vi_type="$2"
  local step_number=$(get_step_number "final-operations")
  
  log_info "=== Running Step $step_number: final-operations ==="
  init_logging "step_final-operations" "$vi_type" "$MAIN_COUNT_RESOURCES"
  
  log_step_start "Final Statistics"
  local final_stats_start=$(get_timestamp)
  gather_all_statistics "$scenario_dir/statistics"
  collect_vpa "$scenario_dir"
  local final_stats_end=$(get_timestamp)
  local final_stats_duration=$((final_stats_end - final_stats_start))
  log_step_end "Final Statistics" "$final_stats_duration"
  
  log_info "Waiting 30 seconds before cleanup"
  sleep 30
  
  if [ "${NO_POST_CLEANUP:-false}" = "true" ]; then
    log_warning "Skipping final cleanup as requested"
  else
    log_step_start "Final Cleanup"
    local final_cleanup_start=$(get_timestamp)
    undeploy_resources
    local final_cleanup_end=$(get_timestamp)
    local final_cleanup_duration=$((final_cleanup_end - final_cleanup_start))
    log_step_end "Final Cleanup" "$final_cleanup_duration"
  fi
  
  log_success "Final operations step completed"
}

# Main execution function for individual steps
run_individual_step() {
  local step_name="$1"
  local scenario_dir="$2"
  local vi_type="$3"
  
  # Find step number
  local step_number=1
  for step in "${ALL_STEPS[@]}"; do
    if [ "$step" = "$step_name" ]; then
      break
    fi
    step_number=$((step_number + 1))
  done
  
  log_info "=== Executing Step $step_number: $step_name ==="
  
  case "$step_name" in
    "cleanup")
      run_step_cleanup "$scenario_dir" "$vi_type"
      ;;
    "vm-deployment")
      run_step_vm_deployment "$scenario_dir" "$vi_type"
      ;;
    "statistics-collection")
      run_step_statistics_collection "$scenario_dir" "$vi_type"
      ;;
    "vm-operations")
      run_step_vm_operations "$scenario_dir" "$vi_type"
      ;;
    "vm-undeploy-deploy")
      run_step_vm_undeploy_deploy "$scenario_dir" "$vi_type"
      ;;
    "vm-operations-test")
      run_step_vm_operations_test "$scenario_dir" "$vi_type"
      ;;
    "migration-tests")
      run_step_migration_tests "$scenario_dir" "$vi_type"
      ;;
    "migration-parallel-2x")
      run_step_migration_parallel_2x "$scenario_dir" "$vi_type"
      ;;
    "migration-parallel-4x")
      run_step_migration_parallel_4x "$scenario_dir" "$vi_type"
      ;;
    "migration-parallel-8x")
      run_step_migration_parallel_8x "$scenario_dir" "$vi_type"
      ;;
    "controller-restart")
      run_step_controller_restart "$scenario_dir" "$vi_type"
      ;;
    "drain-node")
      run_step_drain_node "$scenario_dir" "$vi_type"
      ;;
    "final-operations")
      run_step_final_operations "$scenario_dir" "$vi_type"
      ;;
    *)
      log_error "Unknown step: $step_name"
      echo "Available steps:"
      list_available_steps
      exit 1
      ;;
  esac
}

# Additional steps aligned with original tests.sh
run_step_migration_parallel_2x() {
  local scenario_dir="$1"
  local vi_type="$2"
  local step_number=$(get_step_number "migration-parallel-2x")

  log_info "=== Running Step $step_number: migration-parallel-2x ==="
  init_logging "step_migration-parallel-2x" "$vi_type" "$MAIN_COUNT_RESOURCES"
  local amountNodes=$(kubectl get nodes --no-headers -o name | wc -l)
  local migration_parallel_2x=$(( amountNodes*2 ))
  log_info "Testing migration with parallelMigrationsPerCluster [$migration_parallel_2x (2x)]"
  log_step_start "Migration parallel 2x"
  local start_ts=$(get_timestamp)
  scale_deckhouse 0
  migration_config "640Mi" "800" "$migration_parallel_2x" "1" "150"
  migration_percent_vms $MIGRATION_10_COUNT
  local end_ts=$(get_timestamp)
  log_step_end "Migration parallel 2x" "$((end_ts-start_ts))"
}

run_step_migration_parallel_4x() {
  local scenario_dir="$1"
  local vi_type="$2"
  local step_number=$(get_step_number "migration-parallel-4x")

  log_info "=== Running Step $step_number: migration-parallel-4x ==="
  init_logging "step_migration-parallel-4x" "$vi_type" "$MAIN_COUNT_RESOURCES"
  local amountNodes=$(kubectl get nodes --no-headers -o name | wc -l)
  local migration_parallel_4x=$(( amountNodes*4 ))
  log_info "Testing migration with parallelMigrationsPerCluster [$migration_parallel_4x (4x)]"
  log_step_start "Migration parallel 4x"
  local start_ts=$(get_timestamp)
  migration_config "640Mi" "800" "$migration_parallel_4x" "1" "150"
  migration_percent_vms $MIGRATION_10_COUNT
  local end_ts=$(get_timestamp)
  log_step_end "Migration parallel 4x" "$((end_ts-start_ts))"
}

run_step_migration_parallel_8x() {
  local scenario_dir="$1"
  local vi_type="$2"
  local step_number=$(get_step_number "migration-parallel-8x")

  log_info "=== Running Step $step_number: migration-parallel-8x ==="
  init_logging "step_migration-parallel-8x" "$vi_type" "$MAIN_COUNT_RESOURCES"
  local amountNodes=$(kubectl get nodes --no-headers -o name | wc -l)
  local migration_parallel_8x=$(( amountNodes*8 ))
  log_info "Testing migration with parallelMigrationsPerCluster [$migration_parallel_8x (8x)]"
  log_step_start "Migration parallel 8x"
  local start_ts=$(get_timestamp)
  migration_config "640Mi" "800" "$migration_parallel_8x" "1" "150"
  migration_percent_vms $MIGRATION_10_COUNT
  migration_config
  log_info "Restoring original deckhouse controller replicas to [$ORIGINAL_DECHOUSE_CONTROLLER_REPLICAS]"
  scale_deckhouse $ORIGINAL_DECHOUSE_CONTROLLER_REPLICAS
  local end_ts=$(get_timestamp)
  log_step_end "Migration parallel 8x" "$((end_ts-start_ts))"
}

run_step_drain_node() {
  local scenario_dir="$1"
  local vi_type="$2"
  local step_number=$(get_step_number "drain-node")

  log_info "=== Running Step $step_number: drain-node ==="
  init_logging "step_drain-node" "$vi_type" "$MAIN_COUNT_RESOURCES"
  log_info "Draining node via workload"
  log_step_start "Drain node"
  local start_ts=$(get_timestamp)
  drain_node
  local end_ts=$(get_timestamp)
  log_step_end "Drain node" "$((end_ts-start_ts))"
}

# Ordered steps for full step-runner alignment with original tests.sh
ALL_STEPS=(
  cleanup
  vm-deployment
  statistics-collection
  vm-operations
  vm-undeploy-deploy
  vm-operations-test
  migration-tests
  migration-parallel-2x
  migration-parallel-4x
  migration-parallel-8x
  controller-restart
  drain-node
  final-operations
)

run_steps_from() {
  local start_step="$1"
  local scenario_dir="$2"
  local vi_type="$3"

  local started=false
  local step_number=1
  for step in "${ALL_STEPS[@]}"; do
    if [ "$started" = false ]; then
      if [ "$step" = "$start_step" ]; then
        started=true
      else
        step_number=$((step_number + 1))
        continue
      fi
    fi
    log_info "=== Executing Step $step_number: $step ==="
    run_individual_step "$step" "$scenario_dir" "$vi_type"
    step_number=$((step_number + 1))
  done
}

# === Test configuration ===
# Default values (can be overridden by command line arguments)
SCENARIO_NUMBER=${SCENARIO_NUMBER:-1}
MAIN_COUNT_RESOURCES=${MAIN_COUNT_RESOURCES:-2} # vms and vds (reduced for testing)
PERCENT_VMS=10  # 10% of total resources
MIGRATION_DURATION="1m"
MIGRATION_PERCENTAGE_10=10  # 10% for migration
MIGRATION_PERCENTAGE_5=5    # 5% for migration
WAIT_MIGRATION=$( echo "$MIGRATION_DURATION" | sed 's/m//' )

# Large scale deployment configuration
MAX_BATCH_SIZE=${MAX_BATCH_SIZE:-1200}  # Maximum resources per batch
TOTAL_TARGET_RESOURCES=${TOTAL_TARGET_RESOURCES:-15000}  # Total target resources
BATCH_DEPLOYMENT_ENABLED=${BATCH_DEPLOYMENT_ENABLED:-false}  # Enable batch deployment for large numbers

# Parse command line arguments
parse_arguments "$@"

# Recalculate resources after parsing command line arguments
PERCENT_RESOURCES=$(( $MAIN_COUNT_RESOURCES * $PERCENT_VMS / 100 ))
if [ $PERCENT_RESOURCES -eq 0 ]; then
  PERCENT_RESOURCES=1
fi

# Calculate resources for migration percentages
MIGRATION_5_COUNT=$(( $MAIN_COUNT_RESOURCES * $MIGRATION_PERCENTAGE_5 / 100 ))
MIGRATION_10_COUNT=$(( $MAIN_COUNT_RESOURCES * $MIGRATION_PERCENTAGE_10 / 100 ))
if [ $MIGRATION_5_COUNT -eq 0 ]; then
  MIGRATION_5_COUNT=1
fi
if [ $MIGRATION_10_COUNT -eq 0 ]; then
  MIGRATION_10_COUNT=1
fi

# Display configuration
log_info "=== Performance Test Configuration ==="
log_info "Scenario Number: $SCENARIO_NUMBER"
log_info "Resource Count: $MAIN_COUNT_RESOURCES"
log_info "Percent Resources (10%): $PERCENT_RESOURCES"
log_info "Migration 5% Count: $MIGRATION_5_COUNT"
log_info "Migration 10% Count: $MIGRATION_10_COUNT"
log_info "Batch Size: $MAX_BATCH_SIZE"
log_info "Batch Deployment Enabled: $BATCH_DEPLOYMENT_ENABLED"
log_info "========================================"

# Main execution
prepare_for_tests

# Check if running individual step or full scenario
if [ -n "$INDIVIDUAL_STEP" ] || [ -n "${FROM_STEP:-}" ]; then
  # Individual step execution
  if [ -z "$SCENARIO_DIR" ]; then
    log_error "Scenario directory is required for individual step execution"
    echo "Usage: $0 --step $INDIVIDUAL_STEP --scenario-dir DIR [--vi-type TYPE]"
    exit 1
  fi
  
  # Set default VI_TYPE if not provided
  if [ -z "$VI_TYPE" ]; then
    VI_TYPE="persistentVolumeClaim"
  fi
  # Optionally skip pre-cleanup by not running cleanup unless requested explicitly
  if [ -n "${FROM_STEP:-}" ]; then
    log_info "Running from step: $FROM_STEP"
    run_steps_from "$FROM_STEP" "$SCENARIO_DIR" "$VI_TYPE"
    log_success "From-step execution completed successfully"
  else
    log_info "Running individual step: $INDIVIDUAL_STEP"
    # Respect NO_PRE_CLEANUP/NO_POST_CLEANUP within steps; cleanup step is separate
    run_individual_step "$INDIVIDUAL_STEP" "$SCENARIO_DIR" "$VI_TYPE"
    log_success "Individual step completed successfully"
  fi
else
  # Full scenario execution (original behavior)
  case $SCENARIO_NUMBER in
    1)
      VI_TYPE="persistentVolumeClaim"
      run_scenario "scenario_1" "$VI_TYPE"
      log_success "Scenario 1 (persistentVolumeClaim) completed successfully"
      ;;
    2)
      VI_TYPE="containerRegistry"
      run_scenario "scenario_2" "$VI_TYPE"
      log_success "Scenario 2 (containerRegistry) completed successfully"
      ;;
    *)
      log_error "Invalid scenario number: $SCENARIO_NUMBER. Use 1 or 2."
      exit 1
      ;;
  esac
  
  undeploy_resources
  log_success "All scenarios completed successfully"
fi
