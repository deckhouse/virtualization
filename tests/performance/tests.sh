#!/usr/bin/env bash

set -eEo pipefail
# set -x

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
      --bootstrap-only)
        BOOTSTRAP_ONLY=true
        shift
        ;;
      --continue)
        CONTINUE_AFTER_BOOTSTRAP=true
        shift
        ;;
      --keep-resources)
        KEEP_RESOURCES=true
        shift
        ;;
      --clean-reports)
        CLEAN_REPORTS=true
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
  --bootstrap-only        Only deploy resources, skip tests
  --continue              Continue tests after bootstrap (use with --bootstrap-only)
  --keep-resources        Keep resources after tests (don't cleanup)
  --clean-reports          Clean all report directories before running
  -h, --help              Show this help message

EXAMPLES:
  $0                       # Run scenario 1 with 2 resources (default)
  $0 -s 1 -c 4            # Run scenario 1 with 4 resources
  $0 -s 2 -c 10           # Run scenario 2 with 10 resources
  $0 --scenario 1 --count 6 # Run scenario 1 with 6 resources
  $0 -c 15000 --batch-size 1200 # Deploy 15000 resources in batches of 1200
  $0 --bootstrap-only -c 1000 # Only deploy 1000 resources, skip tests
  $0 --continue -c 1000   # Continue tests after bootstrap
  $0 --keep-resources     # Keep resources after tests (don't cleanup)
  $0 --clean-reports      # Clean all reports and run default scenario

SCENARIOS:
  1 - persistentVolumeClaim (default)
  2 - containerRegistry (currently disabled)

BATCH DEPLOYMENT:
  For large deployments (>1200 resources), the script automatically uses batch deployment.
  Each batch deploys up to 1200 resources with 30-second delays between batches.
  Use --batch-size to customize batch size and --enable-batch to force batch mode.

DEPLOYMENT CONTROL:
  --bootstrap-only: Only deploy resources, skip all tests (useful for large deployments)
  --continue: Continue tests after bootstrap (use with --bootstrap-only)
  --keep-resources: Keep resources after tests (don't cleanup at the end)
  
  Workflow examples:
  1. Deploy only: ./tests.sh --bootstrap-only -c 5000
  2. Continue tests: ./tests.sh --continue -c 5000
  3. Keep resources: ./tests.sh --keep-resources -c 1000

EOF
}

# Detect operating system
detect_os() {
  if [[ "$OSTYPE" == "darwin"* ]] || [[ "$(uname)" == "Darwin" ]]; then
    echo "macOS"
  elif [[ "$OSTYPE" == "linux-gnu"* ]] || [[ "$(uname)" == "Linux" ]]; then
    echo "Linux"
  else
    echo "Unknown"
  fi
}

# Set OS-specific variables
OS_TYPE=$(detect_os)

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
# Store original controller replicas count
ORIGINAL_CONTROLLER_REPLICAS=""
# Centralized logging
ORIGINAL_DECHOUSE_CONTROLLER_REPLICAS=""
LOG_FILE=""
CURRENT_SCENARIO=""
VM_OPERATIONS_LOG=""

# Large scale deployment configuration
MAX_BATCH_SIZE=1200  # Maximum resources per batch
TOTAL_TARGET_RESOURCES=15000  # Total target resources
BATCH_DEPLOYMENT_ENABLED=false  # Enable batch deployment for large numbers

# New deployment control options
BOOTSTRAP_ONLY=false  # Only deploy resources, skip tests
CONTINUE_AFTER_BOOTSTRAP=false  # Continue tests after bootstrap
KEEP_RESOURCES=false  # Keep resources after tests (don't cleanup)

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

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

# VM Operations logging functions
log_vm_operation() {
    local message="$1"
    local timestamp=$(get_current_date)
    if [ -n "$VM_OPERATIONS_LOG" ]; then
        echo "[$timestamp] [VM_OP] $message" >> "$VM_OPERATIONS_LOG"
    fi
}

log_vmop_operation() {
    local message="$1"
    local timestamp=$(get_current_date)
    if [ -n "$VM_OPERATIONS_LOG" ]; then
        echo "[$timestamp] [VMOP] $message" >> "$VM_OPERATIONS_LOG"
    fi
}

# Function to log duration details to file
log_duration() {
    local step_name="$1"
    local duration="$2"
    local timestamp=$(get_current_date)
    local formatted_duration=$(format_duration "$duration")
    if [ -n "$LOG_FILE" ]; then
        echo "[$timestamp] [DURATION] $step_name: $formatted_duration" >> "$LOG_FILE"
    fi
}

# Function to log step start with timestamp
log_step_start() {
    local step_name="$1"
    local timestamp=$(get_current_date)
    echo -e "${CYAN}[STEP_START] $step_name${NC}"
    if [ -n "$LOG_FILE" ]; then
        echo "[$timestamp] [STEP_START] $step_name" >> "$LOG_FILE"
    fi
}

# Function to log step end with duration
log_step_end() {
    local step_name="$1"
    local duration="$2"
    local timestamp=$(get_current_date)
    local formatted_duration=$(format_duration "$duration")
    echo -e "${CYAN}[STEP_END] $step_name${NC}"
    if [ -n "$LOG_FILE" ]; then
        echo "[$timestamp] [STEP_END] $step_name completed in $formatted_duration" >> "$LOG_FILE"
    fi
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
  local timestamp="$1"
  
  # Check if timestamp is valid (not empty and is a number)
  if [ -z "$timestamp" ] || ! [[ "$timestamp" =~ ^[0-9]+$ ]]; then
    # Use current time if timestamp is invalid
    date +"%H:%M:%S %d-%m-%Y"
    return
  fi
  
  # Use OS-specific date command
  case "$OS_TYPE" in
    "macOS")
      date -r "$timestamp" +"%H:%M:%S %d-%m-%Y" 2>/dev/null || date +"%H:%M:%S %d-%m-%Y"
      ;;
    "Linux")
      date -d "@$timestamp" +"%H:%M:%S %d-%m-%Y" 2>/dev/null || date +"%H:%M:%S %d-%m-%Y"
      ;;
    *)
      # Fallback - try both methods
      if date -r "$timestamp" +"%H:%M:%S %d-%m-%Y" 2>/dev/null; then
        # macOS style worked
        date -r "$timestamp" +"%H:%M:%S %d-%m-%Y"
      elif date -d "@$timestamp" +"%H:%M:%S %d-%m-%Y" 2>/dev/null; then
        # Linux style worked
        date -d "@$timestamp" +"%H:%M:%S %d-%m-%Y"
      else
        # Last resort - use current time
        date +"%H:%M:%S %d-%m-%Y"
      fi
      ;;
  esac
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


# Centralized logging functions
init_logging() {
    local scenario_name=$1
    local vi_type=$2
    local count=$3
    local timestamp=$(date +"%Y%m%d_%H%M%S")
    local scenario_dir="$REPORT_DIR/${scenario_name}_${vi_type}_${count}vm_${timestamp}"
    LOG_FILE="$scenario_dir/test.log"
    VM_OPERATIONS_LOG="$scenario_dir/vm_operations.log"
    CURRENT_SCENARIO="${scenario_name}_${vi_type}_${count}vm_${timestamp}"
    mkdir -p "$(dirname "$LOG_FILE")"
    echo "=== Test started at $(get_current_date) ===" > "$LOG_FILE"
    echo "=== VM Operations Report started at $(get_current_date) ===" > "$VM_OPERATIONS_LOG"
}


# Function to calculate percentage safely
calculate_percentage() {
    local duration="$1"
    local total="$2"
    
    # Check if values are valid numbers and not zero
    if [[ -z "$duration" || -z "$total" || "$duration" -eq 0 || "$total" -eq 0 ]]; then
        echo "0.0"
        return
    fi
    
    # Use bc with error handling
    local result=$(echo "scale=1; $duration * 100 / $total" | bc 2>/dev/null || echo "0.0")
    echo "$result"
}

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
    local drain_stats_duration="${23:-0}"
    local final_cleanup_duration="${24:-0}"
    local migration_parallel_2x_duration="${25:-0}"
    local migration_parallel_4x_duration="${26:-0}"
    local migration_parallel_8x_duration="${27:-0}"
    
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
    local drain_stats_percent=$(calculate_percentage "$drain_stats_duration" "$total_duration")
    local final_cleanup_percent=$(calculate_percentage "$final_cleanup_duration" "$total_duration")
    local migration_parallel_2x_percent=$(calculate_percentage "$migration_parallel_2x_duration" "$total_duration")
    local migration_parallel_4x_percent=$(calculate_percentage "$migration_parallel_4x_duration" "$total_duration")
    local migration_parallel_8x_percent=$(calculate_percentage "$migration_parallel_8x_duration" "$total_duration")
    
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
$(printf "%-55s %10s  %10s\n" "Migration Setup (${MIGRATION_PERCENTAGE_5}% - ${MIGRATION_5_COUNT} VMs)" "$(format_duration $migration_duration)" "$(printf "%5.1f" $migration_percent)%")
$(printf "%-55s %10s  %10s\n" "VM Operations: Stopping VMs [$PERCENT_RESOURCES]" "$(format_duration $vm_ops_stop_duration)" "$(printf "%5.1f" $vm_ops_stop_percent)%")
$(printf "%-55s %10s  %10s\n" "VM Operations: Start VMs [$PERCENT_RESOURCES]" "$(format_duration $vm_ops_start_duration)" "$(printf "%5.1f" $vm_ops_start_percent)%")
$(printf "%-55s %10s  %10s\n" "Stop Migration ${MIGRATION_PERCENTAGE_5}% (${MIGRATION_5_COUNT} VMs)" "$(format_duration $cleanup_ops_duration)" "$(printf "%5.1f" $cleanup_ops_percent)%")
$(printf "%-55s %10s  %10s\n" "Migration Percentage ${MIGRATION_10_COUNT} VMs (10%)" "$(format_duration $migration_percent_duration)" "$(printf "%5.1f" $migration_percent_percent)%")
$(printf "%-55s %10s  %10s\n" "Migration parallelMigrationsPerCluster 2x nodes" "$(format_duration $migration_parallel_2x_duration)" "$(printf "%5.1f" $migration_parallel_2x_percent)%")
$(printf "%-55s %10s  %10s\n" "Migration parallelMigrationsPerCluster 4x nodes" "$(format_duration $migration_parallel_4x_duration)" "$(printf "%5.1f" $migration_parallel_4x_percent)%")
$(printf "%-55s %10s  %10s\n" "Migration parallelMigrationsPerCluster 8x nodes" "$(format_duration $migration_parallel_8x_duration)" "$(printf "%5.1f" $migration_parallel_8x_percent)%")
$(printf "%-55s %10s  %10s\n" "Controller Restart" "$(format_duration $controller_duration)" "$(printf "%5.1f" $controller_percent)%")
$(printf "%-55s %10s  %10s\n" "Final Statistics" "$(format_duration $final_stats_duration)" "$(printf "%5.1f" $final_stats_percent)%")
$(printf "%-55s %10s  %10s\n" "Drain node" "$(format_duration $drain_stats_duration)" "$(printf "%5.1f" $drain_stats_percent)%")
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
$(printf "%-25s %10s\n" "Drain Node Time:" "$(format_duration $drain_stats_duration)")
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


create_report_dir() {
  local scenario_name=$1
  local vi_type=$2
  local count=$3
  local timestamp=$(date +"%Y%m%d_%H%M%S")
  local base_dir="$REPORT_DIR/${scenario_name}_${vi_type}_${count}vm_${timestamp}"
  mkdir -p "$base_dir/statistics"
  mkdir -p "$base_dir/vpa"
  echo "$base_dir"
}

remove_report_dir() {
  local dir=${1:-$REPORT_DIR}
  rm -rf $dir
}

clean_all_reports() {
  if [ -d "$REPORT_DIR" ]; then
    log_info "Cleaning all report directories in $REPORT_DIR"
    rm -rf "$REPORT_DIR"/*
    log_success "All report directories cleaned"
  else
    log_info "Report directory $REPORT_DIR does not exist, nothing to clean"
  fi
}

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
  for vpa in "${VPAS[@]}"; do
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

wait_ptc_vmops_complete() {
  local sleep_time=${1:-2}
  local target_count=${2:-$MIGRATION_10_COUNT}
  local start_time=$(get_timestamp)
  local migrated_vms=0

  local vms_count=( $(kubectl -n $NAMESPACE get vm --no-headers | awk '{print $1}' | tail -n $target_count) )

  while true; do
    migrated_vms=0

    for vm in "${vms_count[@]}"; do
      completed=$(kubectl get vmop -n "$NAMESPACE" -o json | \
        jq -r --arg vm "$vm" '
          .items[] |
          select(
            .spec.virtualMachineName == $vm and
            (
              (.status.phase == "Completed") or
              (any(.status.conditions[]; .type == "Completed" and .status == "True"))
            )
          ) |
          .metadata.name
        ' | head -n1
        )
      if [ -n "$completed" ]; then
        ((migrated_vms++))
      fi
    done

      if [ $migrated_vms -eq $target_count ]; then
        break
      fi
  log_info "Migration progress: $migrated_vms/$target_count"
  sleep "$sleep_time"
  done
}


# Wait for vmops to complete (including Failed status) and check VMs are Running
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
    
    if [[ "$VMOPFinished" -eq "$vmop_total" ]] && [[ "$VMOPInProgress" -eq 0 ]]; then
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

# Properly wait for VMs to be Running
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
    d8 v -n $NAMESPACE evict $vm
  done

  wait_vmops_complete

  local end_time=$(get_timestamp)
  local duration=$((end_time - start_time))
  local formatted_duration=$(format_duration "$duration")
  
  log_info "Migration completed - End time: $(formatted_date $end_time)"
  log_success "Migrated $target_count VMs in $formatted_duration"
  log_vm_operation "Migration completed - Migrated $target_count VMs in $formatted_duration"
}

migration_percent_vms_waitptc_vmops() {
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
    d8 v -n $NAMESPACE evict $vm
  done

  wait_ptc_vmops_complete

  local end_time=$(get_timestamp)
  local duration=$((end_time - start_time))
  local formatted_duration=$(format_duration "$duration")
  
  log_info "Migration completed - End time: $(formatted_date $end_time)"
  log_success "Migrated $target_count VMs in $formatted_duration"
  log_vm_operation "Migration completed - Migrated $target_count VMs in $formatted_duration"
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
  
  while true; do
    local current_time=$(get_timestamp)
    
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

# Function to check cluster resources before large deployment
check_cluster_resources() {
  local target_count=$1
  local batch_size=${2:-$MAX_BATCH_SIZE}
  
  log_info "Checking cluster resources for deployment of $target_count resources"
  
  # Check available nodes
  local node_count=$(kubectl get nodes --no-headers | wc -l)
  log_info "Available nodes: $node_count"
  
  # Check available storage
  local storage_class=$(get_default_storage_class)
  log_info "Default storage class: $storage_class"
  
  # Check namespace resources
  local existing_vms=$(kubectl -n $NAMESPACE get vm --no-headers 2>/dev/null | wc -l || echo "0")
  local existing_vds=$(kubectl -n $NAMESPACE get vd --no-headers 2>/dev/null | wc -l || echo "0")
  
  log_info "Existing VMs in namespace: $existing_vms"
  log_info "Existing VDs in namespace: $existing_vds"
  
  # Calculate total resources needed
  local total_resources_needed=$((target_count * 2))  # VMs + VDs
  local total_existing=$((existing_vms + existing_vds))
  local total_after_deployment=$((total_existing + total_resources_needed))
  
  log_info "Total resources after deployment: $total_after_deployment"
  
  # Estimate time for deployment
  local estimated_batches=$(( (target_count + batch_size - 1) / batch_size ))
  local estimated_time_per_batch=300  # 5 minutes per batch (conservative estimate)
  local estimated_total_time=$((estimated_batches * estimated_time_per_batch))
  
  log_info "Estimated batches: $estimated_batches"
  log_info "Estimated total time: $(format_duration $estimated_total_time)"
  
  # Warning for very large deployments
  if [ $target_count -gt 10000 ]; then
    log_warning "Large deployment detected: $target_count resources"
    log_warning "This may take several hours to complete"
    log_warning "Consider running in background or with screen/tmux"
  fi
  
  return 0
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

# Universal deployment function for disks only
deploy_disks_only_smart() {
  local count=$1
  local vi_type=$2
  local batch_size=${3:-$MAX_BATCH_SIZE}
  
  log_info "Disk deployment decision for $count resources:"
  log_info "  - Batch size: $batch_size"
  log_info "  - Batch deployment enabled: $BATCH_DEPLOYMENT_ENABLED"
  
  if should_use_batch_deployment "$count"; then
    log_info "Using batch deployment for $count disks (batch size: $batch_size)"
    deploy_disks_only_batch "$count" "$vi_type" "$batch_size"
  else
    log_info "Using regular deployment for $count disks"
    deploy_disks_only "$count" "$vi_type"
  fi
}

# Universal deployment function for VMs only
deploy_vms_only_smart() {
  local count=$1
  local namespace=${2:-$NAMESPACE}
  local batch_size=${3:-$MAX_BATCH_SIZE}
  
  log_info "VM deployment decision for $count resources:"
  log_info "  - Batch size: $batch_size"
  log_info "  - Batch deployment enabled: $BATCH_DEPLOYMENT_ENABLED"
  
  if should_use_batch_deployment "$count"; then
    log_info "Using batch deployment for $count VMs (batch size: $batch_size)"
    deploy_vms_only_batch "$count" "$namespace" "$batch_size"
  else
    log_info "Using regular deployment for $count VMs"
    deploy_vms_only "$count" "$namespace"
  fi
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

# New function for batch deployment of disks only
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

# New function for batch deployment of VMs only
deploy_vms_only_batch() {
  local total_count=$1
  local namespace=${2:-$NAMESPACE}
  local batch_size=${3:-$MAX_BATCH_SIZE}
  local start_time=$(get_timestamp)
  
  log_info "Starting batch deployment of $total_count VMs (disks already exist)"
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
    
    # Deploy current batch of VMs (COUNT should be cumulative, not absolute)
    local cumulative_count=$((deployed_count + current_batch_size))
    log_info "Deploying VM batch $batch_number: $current_batch_size new VMs (total will be: $cumulative_count)"
    task apply:vms \
        COUNT=$cumulative_count \
        NAMESPACE=$NAMESPACE
    
    # Wait for current batch to be ready
    wait_vm $SLEEP_TIME
    
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
  
  log_success "Batch VM deployment completed: $deployed_count VMs in $formatted_duration"
  log_info "Average time per VM: $(( total_duration / deployed_count )) seconds"
  
  echo "$total_duration"
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
    
    # Check if any VMs still exist
    for vm in "${vms[@]}"; do
      if kubectl -n $NAMESPACE get $vm >/dev/null 2>&1; then
        log_info "VM $vm still exists, attempting deletion"
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

drain_node() {
  local start_time=$(get_timestamp)

  log_info "Start draining node"
  log_info "Start time: $(formatted_date $start_time)"
  
  local task_start=$(get_timestamp)
  
  local KUBECONFIG_MERGE=$(kubectl config view --merge --flatten | base64 -w 0)
  KUBECONFIG_BASE64=$KUBECONFIG_MERGE task shatal:run
  
  local task_end=$(get_timestamp)
  local task_duration=$((task_end - task_start))
  local end_time=$(get_timestamp)
  local formatted_duration=$(format_duration "$task_duration")
  
  log_info "Duration node completed - End time: $(formatted_date $end_time)"
  log_info "Task Duration node execution: $(format_duration $task_duration)"
  log_success "Duration node completed in $formatted_duration"
  echo "$task_duration"
}

scale_deckhouse() {
  local replicas=${1}
  ORIGINAL_DECHOUSE_CONTROLLER_REPLICAS=$(kubectl -n d8-system get deployment deckhouse -o jsonpath="{.spec.replicas}" 2>/dev/null || echo "1")
  log_info "Deckhouse controller replicas: $ORIGINAL_DECHOUSE_CONTROLLER_REPLICAS"
  log_info "Deckhouse controller scaled to $replicas"
  kubectl -n d8-system scale --replicas $replicas deployment deckhouse
  log_success "Deckhouse controller scaled to $replicas"
}

migration_config() {
  # default values
  #   {
  #     "bandwidthPerMigration": "640Mi",
  #     "completionTimeoutPerGiB": 800,
  #     "parallelMigrationsPerCluster": 8, # count all nodes
  #     "parallelOutboundMigrationsPerNode": 1,
  #     "progressTimeout": 150
  #   }
  local amountNodes=$(kubectl get nodes --no-headers -o name | wc -l)

  local bandwidthPerMigration=${1:-"640Mi"}
  local completionTimeoutPerGiB=${2:-"800"}
  local parallelMigrationsPerCluster=${3:-$amountNodes}
  local parallelOutboundMigrationsPerNode=${4:-"1"}
  local progressTimeout=${5:-"150"}

  echo "====== configure patch ======"
  echo "bandwidthPerMigration: $bandwidthPerMigration"
  echo "completionTimeoutPerGiB: $completionTimeoutPerGiB"
  echo "parallelMigrationsPerCluster: $parallelMigrationsPerCluster"
  echo "parallelOutboundMigrationsPerNode: $parallelOutboundMigrationsPerNode"
  echo "progressTimeout: $progressTimeout"
  
  patch_json=$(
  jq -n \
    --arg bpm "$bandwidthPerMigration" \
    --argjson ct $completionTimeoutPerGiB \
    --argjson pmc $parallelMigrationsPerCluster \
    --argjson pmon $parallelOutboundMigrationsPerNode \
    --argjson pt $progressTimeout \
    '{
      spec: {
        configuration: {
          migrations: {
            bandwidthPerMigration: $bpm,
            completionTimeoutPerGiB: $ct,
            parallelMigrationsPerCluster: $pmc,
            parallelOutboundMigrationsPerNode: $pmon,
            progressTimeout: $pt
          }
        }
      }
    }'
  )
  log_info "Checking restricted access policy"
  
  if kubectl get validatingadmissionpolicies.admissionregistration.k8s.io virtualization-restricted-access-policy >/dev/null 2>&1; then
    log_info "Deleting restricted access policy"
    kubectl delete validatingadmissionpolicies.admissionregistration.k8s.io virtualization-restricted-access-policy
  else
    log_info "No restricted access policy"
  fi

  sleep 1

  log_info "Patching kubevirt config"

  kubectl -n d8-virtualization patch \
    --as=system:sudouser \
    internalvirtualizationkubevirts.internal.virtualization.deckhouse.io config \
    --type=merge -p "$patch_json"

  log_success "Migration settings applyed"
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


# Calculate resources for migration percentages
MIGRATION_5_COUNT=$(( $MAIN_COUNT_RESOURCES * $MIGRATION_PERCENTAGE_5 / 100 ))
MIGRATION_10_COUNT=$(( $MAIN_COUNT_RESOURCES * $MIGRATION_PERCENTAGE_10 / 100 ))
if [ $MIGRATION_5_COUNT -eq 0 ]; then
  MIGRATION_5_COUNT=1
fi
if [ $MIGRATION_10_COUNT -eq 0 ]; then
  MIGRATION_10_COUNT=1
fi

# Function to run a single scenario
GLOBAL_WAIT_TIME_STEP=5
run_scenario() {
  local scenario_name=$1
  local vi_type=$2
  
  log_info "=== Starting scenario: $scenario_name with $vi_type ==="
  
  # Initialize logging and create report directory
  init_logging "$scenario_name" "$vi_type" "$MAIN_COUNT_RESOURCES"
  local scenario_dir=$(create_report_dir "$scenario_name" "$vi_type" "$MAIN_COUNT_RESOURCES")
  
  # Handle bootstrap-only mode
  if [ "$BOOTSTRAP_ONLY" = "true" ]; then
    log_info "=== BOOTSTRAP ONLY MODE ==="
    log_info "Deploying $MAIN_COUNT_RESOURCES resources without running tests"
    log_info "DEBUG: Starting bootstrap-only mode"
    
    # Skip cleanup if continuing after bootstrap or in bootstrap-only mode
    if [ "$CONTINUE_AFTER_BOOTSTRAP" = "false" ] && [ "$BOOTSTRAP_ONLY" = "false" ]; then
      # Step 1: Clean up any existing resources
      log_info "Step 1: Cleaning up existing resources"
      log_step_start "Step 1: Cleanup up existing resources"
      local cleanup_start=$(get_timestamp)
      stop_migration
      remove_vmops
      undeploy_resources
      local cleanup_end=$(get_timestamp)
      local cleanup_duration=$((cleanup_end - cleanup_start))
      log_info "Cleanup completed in $(format_duration $cleanup_duration)"
      log_step_end "Step 1: Cleanup up existing resources" "$cleanup_duration"
    else
      log_info "Step 1: Skipping cleanup (--continue or --bootstrap-only mode, preserving existing resources)"
    fi
    
    # Step 2: Check cluster resources before deployment
    log_step_start "Step 2: Check cluster resources"
    check_cluster_resources $MAIN_COUNT_RESOURCES
    log_step_end "Step 2: Check cluster resources" "0"
    
    # Step 3: Deploy resources only
    log_step_start "Step 3: Deploy VMs [$MAIN_COUNT_RESOURCES]"
    local deploy_start=$(get_timestamp)
    deploy_vms_with_disks_smart $MAIN_COUNT_RESOURCES $vi_type
    local deploy_end=$(get_timestamp)
    local deploy_duration=$((deploy_end - deploy_start))
    log_info "VM [$MAIN_COUNT_RESOURCES] deploy completed in $(format_duration $deploy_duration)"
    log_step_end "Step 3: End VM Deployment [$MAIN_COUNT_RESOURCES]" "$deploy_duration"
    
    log_success "Bootstrap completed: $MAIN_COUNT_RESOURCES resources deployed"
    log_info "Use --continue to run tests on deployed resources"
    log_info "Resources are preserved and ready for testing"
    log_info "DEBUG: Exiting run_scenario with return 0 (bootstrap-only mode)"
    return 0
  fi
  
  # Handle continue mode (skip deployment, assume resources already exist)
  if [ "$CONTINUE_AFTER_BOOTSTRAP" = "true" ]; then
    log_info "=== CONTINUE MODE ==="
    log_info "Continuing tests on existing resources (--continue enabled)"
    log_info "Skipping deployment, assuming $MAIN_COUNT_RESOURCES resources already exist"
    
    # Check if resources exist
    local existing_vms=$(kubectl -n $NAMESPACE get vm -o name | wc -l)
    local existing_vds=$(kubectl -n $NAMESPACE get vd -o name | wc -l)
    
    if [ $existing_vms -eq 0 ] && [ $existing_vds -eq 0 ]; then
      log_warning "No existing resources found. Please run bootstrap first:"
      log_warning "./tests.sh --bootstrap-only -c $MAIN_COUNT_RESOURCES"
      exit 1
    fi
    
    log_info "Found existing resources: $existing_vms VMs, $existing_vds VDs"
    log_info "Continuing with tests..."
  else
    # Step 1: Clean up any existing resources (skip in bootstrap-only mode)
    if [ "$BOOTSTRAP_ONLY" = "false" ]; then
      log_info "Step 1: Cleaning up existing resources"
      log_step_start "Step 1: Cleanup up existing resources"
      local cleanup_start=$(get_timestamp)
      stop_migration
      remove_vmops
      undeploy_resources
      local cleanup_end=$(get_timestamp)
      local cleanup_duration=$((cleanup_end - cleanup_start))
      log_info "Cleanup completed in $(format_duration $cleanup_duration)"
      log_step_end "Step 1: Cleanup up existing resources" "$cleanup_duration"
    else
      log_info "Step 1: Skipping cleanup (--bootstrap-only mode, preserving existing resources)"
    fi
  fi
  
  local start_time=$(get_timestamp)
  log_info "== Scenario started at $(formatted_date $start_time) =="
  
  # Step 2: Check cluster resources before deployment
  log_step_start "Step 2: Check cluster resources"
  check_cluster_resources $MAIN_COUNT_RESOURCES
  log_step_end "Step 2: Check cluster resources" "0"
  
  # Step 3: Main test sequence (skip deployment in continue mode)
  if [ "$CONTINUE_AFTER_BOOTSTRAP" = "true" ]; then
    log_info "Step 3: Skipping deployment (--continue mode, resources already exist)"
    log_step_start "Step 3: Deploy VMs [$MAIN_COUNT_RESOURCES]"
    log_info "VM [$MAIN_COUNT_RESOURCES] deployment skipped (continue mode)"
    log_step_end "Step 3: End VM Deployment [$MAIN_COUNT_RESOURCES]" "0"
  else
    log_step_start "Step 3: Deploy VMs [$MAIN_COUNT_RESOURCES]"
    local deploy_start=$(get_timestamp)
    deploy_vms_with_disks_smart $MAIN_COUNT_RESOURCES $vi_type
    local deploy_end=$(get_timestamp)
    local deploy_duration=$((deploy_end - deploy_start))
    log_info "VM [$MAIN_COUNT_RESOURCES] deploy completed in $(format_duration $deploy_duration)"
    log_step_end "Step 3: End VM Deployment [$MAIN_COUNT_RESOURCES]" "$deploy_duration"
  fi
  
  # Step 4: Statistics Collection
  log_step_start "Step 4: Start Statistics Collection"
  local stats_start=$(get_timestamp)
  gather_all_statistics "$scenario_dir/statistics"
  collect_vpa "$scenario_dir"
  local stats_end=$(get_timestamp)
  local stats_duration=$((stats_end - stats_start))
  log_info "Statistics collection completed in $(format_duration $stats_duration)"
  log_step_end "Step 4: End Statistics Collection" "$stats_duration"
  
  log_info "Waiting $GLOBAL_WAIT_TIME_STEP seconds before stopping VMs"
  sleep $GLOBAL_WAIT_TIME_STEP
  
  # Step 5: VM Stop
  log_info "Step 5: Stopping all VMs [$MAIN_COUNT_RESOURCES]"
  log_step_start "Step 5: VM Stop"
  local stop_start=$(get_timestamp)
  stop_vm
  local stop_end=$(get_timestamp)
  local stop_duration=$((stop_end - stop_start))
  log_info "VM stop completed in $(format_duration $stop_duration)"
  log_step_end "Step 5: End Stopping all VMs [$MAIN_COUNT_RESOURCES]" "$stop_duration"
  
  log_info "Waiting $GLOBAL_WAIT_TIME_STEP seconds before starting VMs"
  sleep $GLOBAL_WAIT_TIME_STEP
  
  # Step 6: VM Start
  log_info "Step 6: Starting all VMs [$MAIN_COUNT_RESOURCES]"
  log_step_start "Step 6: VM Start [$MAIN_COUNT_RESOURCES]"
  local start_vm_start=$(get_timestamp)
  start_vm
  local start_vm_end=$(get_timestamp)
  local start_vm_duration=$((start_vm_end - start_vm_start))
  log_info "VM start completed in $(format_duration $start_vm_duration)"
  log_step_end "Step 6: End Starting all VMs [$MAIN_COUNT_RESOURCES]" "$start_vm_duration"

  log_info "Waiting $GLOBAL_WAIT_TIME_STEP seconds before Starting migration test ${MIGRATION_PERCENTAGE_5}% (${MIGRATION_5_COUNT} VMs)"
  sleep $GLOBAL_WAIT_TIME_STEP

  # Step 7: Start 5% migration in background
  local migration_duration_time="0m"
  log_info "Step 7: Starting migration test ${MIGRATION_PERCENTAGE_5}% (${MIGRATION_5_COUNT} VMs)"
  log_step_start "Step 7: Migration Setup"
  local migration_start=$(get_timestamp)
  start_migration $migration_duration_time $MIGRATION_PERCENTAGE_5
  local migration_end=$(get_timestamp)
  local migration_duration=$((migration_end - migration_start))
  log_info "Migration test ${MIGRATION_PERCENTAGE_5}% VMs setup completed in $(format_duration $migration_duration)"
  log_step_end "Step 7: Migration Setup ${MIGRATION_PERCENTAGE_5}% (${MIGRATION_5_COUNT} VMs) Started" "$migration_duration"

  log_info "Waiting 10 seconds before Undeploying 10% VMs [$PERCENT_RESOURCES] (keeping disks)"
  sleep 10

  # Step 8: Start deploy undeploy vms
  log_step_start "Step 8: Undeploy VMs 10% [$PERCENT_RESOURCES]"
  local undeploy_pct_start=$(get_timestamp)
  deploy_vms_with_disks_smart $((MAIN_COUNT_RESOURCES-PERCENT_RESOURCES)) $vi_type
  local undeploy_pct_end=$(get_timestamp)
  local undeploy_pct_duration=$((undeploy_pct_end - undeploy_pct_start))
  log_info "Undeploy VMs 10% [$((MAIN_COUNT_RESOURCES-PERCENT_RESOURCES))] completed in $(format_duration $undeploy_pct_duration)"
  log_step_end "Step 8: Undeploy VMs 10% [$PERCENT_RESOURCES]" "$undeploy_pct_duration"

  log_info "Waiting 10 seconds before Deploying 10% VMs [$PERCENT_RESOURCES]"
  sleep 10

  # Step 9: Deploy VMs 10%
  log_step_start "Step 9: Deploy VMs 10% [$PERCENT_RESOURCES]"
  local deploy_pct_start=$(get_timestamp)
  deploy_vms_with_disks_smart $MAIN_COUNT_RESOURCES $vi_type
  local deploy_pct_end=$(get_timestamp)
  local deploy_pct_duration=$((deploy_pct_end - deploy_pct_start))
  log_info "Deploy VMs 10% [$PERCENT_RESOURCES] completed in $(format_duration $deploy_pct_duration)"
  log_step_end "Step 9: Deploy VMs 10% [$PERCENT_RESOURCES]" "$deploy_pct_duration"
  
  # Step 10: Statistics Collection Deploy 10%
  log_step_start "Step 10: Start Statistics Collection Deploy 10% [$PERCENT_RESOURCES]"
  local stats_start=$(get_timestamp)
  gather_all_statistics "$scenario_dir/statistics"
  collect_vpa "$scenario_dir"
  local stats_end=$(get_timestamp)
  local stats_duration=$((stats_end - stats_start))
  log_info "Statistics collection completed in $(format_duration $stats_duration)"
  log_step_end "Step 10: End Statistics Collection Deploy 10% [$PERCENT_RESOURCES]" "$stats_duration"
  # ====
  
  # Step 11: VM Undeploy 10% VMs
  log_info "Step 11: Undeploying 10% VMs [$PERCENT_RESOURCES] (keeping disks)"
  log_step_start "Step 11: VM Undeploy 10% VMs [$PERCENT_RESOURCES] (keeping disks)"
  local undeploy_start=$(get_timestamp)
  undeploy_vms_only $PERCENT_RESOURCES
  local undeploy_end=$(get_timestamp)
  local undeploy_duration=$((undeploy_end - undeploy_start))
  log_info "VM Undeploy 10% VMs [$PERCENT_RESOURCES] completed in $(format_duration $undeploy_duration)"
  log_step_end "Step 11: VM Undeploy 10% VMs [$PERCENT_RESOURCES] (keeping disks)" "$undeploy_duration"

  log_info "Waiting $GLOBAL_WAIT_TIME_STEP seconds before Deploying 10% VMs [$PERCENT_RESOURCES] (keeping disks)"
  sleep $GLOBAL_WAIT_TIME_STEP

  # Step 12: Deploy 10% VMs and gather statistics
  log_info "Step 12: Deploying 10% VMs ([$PERCENT_RESOURCES] VMs) (keeping disks)"
  log_step_start "Step 12: Deploying 10% VMs [$PERCENT_RESOURCES] (keeping disks)"
  local deploy_remaining_start=$(get_timestamp)
  deploy_vms_only_smart $MAIN_COUNT_RESOURCES
  local deploy_remaining_end=$(get_timestamp)
  local deploy_remaining_duration=$((deploy_remaining_end - deploy_remaining_start))
  log_info "10% VMs deployment completed in $(format_duration $deploy_remaining_duration)"
  log_step_end "Step 12: End Deploying 10% VMs [$PERCENT_RESOURCES] (keeping disks)" "$deploy_remaining_duration"

  log_info "Waiting $GLOBAL_WAIT_TIME_STEP seconds before gather VM Statistics: Deploying 10% VMs ([$PERCENT_RESOURCES] VMs)"
  sleep $GLOBAL_WAIT_TIME_STEP
  
  # Step 13: Gather statistics for 10% VMs
  log_step_start "Step 13: VM Statistics: Deploying 10% VMs ([$PERCENT_RESOURCES] VMs)"
  local vm_stats_start=$(get_timestamp)
  gather_specific_vm_statistics "$scenario_dir/statistics" "$NAMESPACE" "$PERCENT_RESOURCES"
  local vm_stats_end=$(get_timestamp)
  local vm_stats_duration=$((vm_stats_end - vm_stats_start))
  log_info "VM statistics collection completed in $(format_duration $vm_stats_duration)"
  log_step_end "Step 13: End VM Statistics: Deploying 10% VMs ([$PERCENT_RESOURCES] VMs)" "$vm_stats_duration"

  # Step 14: VM operations test - stop/start 10% VMs
  log_info "Step 14: Testing VM stop/start operations for 10% VMs"
  log_step_start "Step 14: VM Operations"
  local vm_ops_start=$(get_timestamp)
  
  log_step_start "VM Operations: Stopping VMs [$PERCENT_RESOURCES]"
  local vm_ops_stop_start=$(get_timestamp)
  stop_vm $PERCENT_RESOURCES
  local vm_ops_stop_end=$(get_timestamp)
  local vm_ops_stop_duration=$((vm_ops_stop_end - vm_ops_stop_start))
  log_step_end "VM Operations: Stopping VMs [$PERCENT_RESOURCES]" "$vm_ops_stop_duration"
  
  sleep $GLOBAL_WAIT_TIME_STEP
  
  log_step_start "VM Operations: Start VMs [$PERCENT_RESOURCES]"
  local vm_ops_start_vm_start=$(get_timestamp)
  start_vm $PERCENT_RESOURCES
  local vm_ops_start_vm_end=$(get_timestamp)
  local vm_ops_start_vm_duration=$((vm_ops_start_vm_end - vm_ops_start_vm_start))
  log_step_end "VM Operations: Start VMs [$PERCENT_RESOURCES]" "$vm_ops_start_vm_duration"
  
  local vm_ops_end=$(get_timestamp)
  local vm_ops_duration=$((vm_ops_end - vm_ops_start))
  log_info "VM operations test completed in $(format_duration $vm_ops_duration)"
  log_step_end "Step 14: VM Operations: Stop/Start VMs [$PERCENT_RESOURCES]" "$vm_ops_duration"
  
  # Step 15: Stop migration and wait for completion
  log_step_start "Step 15: Stop Migration ${MIGRATION_PERCENTAGE_5}% (${MIGRATION_5_COUNT} VMs)"
  local cleanup_ops_start=$(get_timestamp)
  stop_migration
  wait_migration_completion
  remove_vmops
  local cleanup_ops_end=$(get_timestamp)
  local cleanup_ops_duration=$((cleanup_ops_end - cleanup_ops_start))
  log_info "Migration stop and cleanup completed in $(format_duration $cleanup_ops_duration)"
  log_step_end "Step 15: Stop Migration ${MIGRATION_PERCENTAGE_5}% (${MIGRATION_5_COUNT} VMs)" "$cleanup_ops_duration"

  log_info "Waiting $GLOBAL_WAIT_TIME_STEP seconds before Migration Percentage ${MIGRATION_10_COUNT} VMs (10%)"
  sleep $GLOBAL_WAIT_TIME_STEP
  
  # Step 16: Migration percentage test - Migrate 10% VMs
  log_info "Step 16: Testing migration of ${MIGRATION_10_COUNT} VMs (10%)"
  log_step_start "Step 16: Migration Percentage ${MIGRATION_10_COUNT} VMs (10%)"
  local migration_percent_start=$(get_timestamp)
  migration_percent_vms $MIGRATION_10_COUNT
  local migration_percent_end=$(get_timestamp)
  local migration_percent_duration=$((migration_percent_end - migration_percent_start))
  log_info "Migration percentage test completed in $(format_duration $migration_percent_duration)"
  log_step_end "Step 16: End Migration Percentage ${MIGRATION_10_COUNT} VMs (10%)" "$migration_percent_duration"

  remove_vmops

  log_info "Waiting $GLOBAL_WAIT_TIME_STEP seconds"
  sleep $GLOBAL_WAIT_TIME_STEP

  #========
  # Step 17: Migration config
  #    bandwidthPerMigration=${1:-"640Mi"}
  #    completionTimeoutPerGiB=${2:-"800"}
  #    parallelMigrationsPerCluster=${3:-$amountNodes}
  #    parallelOutboundMigrationsPerNode=${4:-"1"}
  #    progressTimeout=${5:-"150"}
  
  log_info "Step 17: Set deckhouse controller replicas to [0]"
  scale_deckhouse 0
  local amountNodes=$(kubectl get nodes --no-headers -o name | wc -l)
  sleep 5

  local migration_parallel_2x=$(( $amountNodes*2 ))
  local migration_parallel_2x_start=$(get_timestamp)
  log_info "Step 17: Testing migration with parallelMigrationsPerCluster [$migration_parallel_2x (2x)]"
  log_step_start "Step 17: Testing migration with parallelMigrationsPerCluster [$migration_parallel_2x (2x)]"
  migration_config "640Mi" "800" "$migration_parallel_2x" "1" "150"
  migration_percent_vms $MIGRATION_10_COUNT
  local migration_parallel_2x_end=$(get_timestamp)
  local migration_parallel_2x_duration=$((migration_parallel_2x_end - migration_parallel_2x_start))
  log_step_end "Step 17: Testing migration with parallelMigrationsPerCluster [$migration_parallel_2x (2x)]" "$migration_parallel_2x_duration"

  log_info "Waiting 2 seconds before Cleanup vmops"
  sleep 2
  remove_vmops

  log_info "Waiting $GLOBAL_WAIT_TIME_STEP seconds"
  sleep $GLOBAL_WAIT_TIME_STEP

  # Step 18: Migration parallel 4x
  local migration_parallel_4x=$(( $amountNodes*4 ))
  local migration_parallel_4x_start=$(get_timestamp)
  log_info "Step 18: Testing migration with parallelMigrationsPerCluster [$migration_parallel_4x] (4x)"
  log_step_start "Step 18: Testing migration with parallelMigrationsPerCluster [$migration_parallel_4x] (4x)"
  migration_config "640Mi" "800" "$migration_parallel_4x" "1" "150"
  migration_percent_vms $MIGRATION_10_COUNT
  local migration_parallel_4x_end=$(get_timestamp)
  local migration_parallel_4x_duration=$((migration_parallel_4x_end - migration_parallel_4x_start))
  log_step_end "Step 18: Testing migration with parallelMigrationsPerCluster [$migration_parallel_4x] (4x)" "$migration_parallel_4x_duration"

  log_info "Waiting 2 seconds before Cleanup vmops"
  sleep 2
  remove_vmops

  log_info "Waiting $GLOBAL_WAIT_TIME_STEP seconds"
  sleep $GLOBAL_WAIT_TIME_STEP

  # Step 19: Migration parallel 8x
  local migration_parallel_8x=$(( $amountNodes*8 ))
  local migration_parallel_8x_start=$(get_timestamp)
  log_info "Step 19: Testing migration with parallelMigrationsPerCluster [$migration_parallel_8x] (8x)"
  log_step_start "Step 19: Testing migration with parallelMigrationsPerCluster [$migration_parallel_8x] (8x)"
  migration_config "640Mi" "800" "$migration_parallel_8x" "1" "150"
  migration_percent_vms $MIGRATION_10_COUNT
  local migration_parallel_8x_end=$(get_timestamp)
  local migration_parallel_8x_duration=$((migration_parallel_8x_end - migration_parallel_8x_start))
  log_step_end "Step 19: Testing migration with parallelMigrationsPerCluster [$migration_parallel_8x] (8x)" "$migration_parallel_8x_duration"

  log_info "Waiting 2 seconds before Cleanup vmops"
  sleep 2
  remove_vmops

  log_info "Back configuration migration back to original"
  migration_config
  log_info "Restoring original deckhouse controller replicas to [$ORIGINAL_DECHOUSE_CONTROLLER_REPLICAS]"
  scale_deckhouse $ORIGINAL_DECHOUSE_CONTROLLER_REPLICAS

  log_info "Waiting $GLOBAL_WAIT_TIME_STEP seconds"
  sleep $GLOBAL_WAIT_TIME_STEP
  #========

  # Step 20: Controller restart test
  log_info "Step 20: Testing controller restart with 1 VM creation"
  log_step_start "Step 20: Controller Restart"
  local controller_start=$(get_timestamp)
  
  # Stop controller first
  stop_virtualization_controller
  
  # Create 1 VM and disk while controller is stopped
  log_info "Creating 1 VM and disk while controller is stopped [$((MAIN_COUNT_RESOURCES + 1)) VMs total]"
  local vm_creation_start=$(get_timestamp)
  create_vm_while_controller_stopped $vi_type
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
  log_step_end "Step 20: Controller Restart" "$controller_duration"

  log_info "Waiting $GLOBAL_WAIT_TIME_STEP seconds"
  sleep $GLOBAL_WAIT_TIME_STEP
  
  # Step 21: Final Statistics
  log_step_start "Step 21: Final Statistics"
  local final_stats_start=$(get_timestamp)
  gather_all_statistics "$scenario_dir/statistics"
  collect_vpa "$scenario_dir"
  local final_stats_end=$(get_timestamp)
  local final_stats_duration=$((final_stats_end - final_stats_start))
  log_info "Final statistics collection completed in $(format_duration $final_stats_duration)"
  log_step_end "Step 21: Final Statistics" "$final_stats_duration"
  
  log_info "Waiting 30 second before drain node"
  sleep 30

  # Step 22: Drain node
  log_step_start "Step 22: Drain node"
  local drain_node_start=$(get_timestamp)
  drain_node
  local drain_stats_end=$(get_timestamp)
  local drain_stats_duration=$((drain_stats_end - drain_node_start))
  log_info "Drain node completed in $(format_duration $drain_stats_duration)"
  log_step_end "Step 22: Drain node" "$drain_stats_duration"

  # Skip final cleanup in bootstrap-only mode or when keep-resources is enabled
  if [ "$BOOTSTRAP_ONLY" = "false" ] && [ "$KEEP_RESOURCES" = "false" ]; then
    log_info "Waiting 30 second before cleanup"
    sleep 30
    
    # Step 23: Final Cleanup
    log_step_start "Step 23: Final Cleanup"
    local final_cleanup_start=$(get_timestamp)
    undeploy_resources
    local final_cleanup_end=$(get_timestamp)
    local final_cleanup_duration=$((final_cleanup_end - final_cleanup_start))
    log_info "Final cleanup completed in $(format_duration $final_cleanup_duration)"
    log_step_end "Step 23: Final Cleanup" "$final_cleanup_duration"
  else
    if [ "$BOOTSTRAP_ONLY" = "true" ]; then
      log_info "Skipping final cleanup (--bootstrap-only mode, resources preserved)"
    elif [ "$KEEP_RESOURCES" = "true" ]; then
      log_info "Skipping final cleanup (--keep-resources mode, resources preserved)"
    fi
  fi
  
  local end_time=$(get_timestamp)
  local duration=$((end_time - start_time))
  local formatted_duration=$(format_duration "$duration")
  
  log_success "Scenario $scenario_name completed in $formatted_duration"
  log_info "Scenario ended at $(formatted_date $end_time)"
  
  # Create summary report
  create_summary_report "$scenario_name" "$vi_type" "$scenario_dir" \
    "$start_time" "$end_time" "$duration" \
    "$cleanup_duration" "$deploy_duration" "$stats_duration" \
    "$stop_duration" "$start_vm_duration" "$undeploy_duration" \
    "$deploy_remaining_duration" "$vm_stats_duration" "$vm_ops_duration" \
    "$vm_ops_stop_duration" "$vm_ops_start_vm_duration" "$migration_duration" \
    "$cleanup_ops_duration" "$migration_percent_duration" "$controller_duration" \
    "$final_stats_duration" "$drain_stats_duration" "$final_cleanup_duration" \
    "$migration_parallel_2x_duration" "$migration_parallel_4x_duration" "$migration_parallel_8x_duration"
  
  # Summary of all step durations
  log_info "=== Scenario $scenario_name Duration Summary ==="
  log_duration "Step 1: Cleanup" "$cleanup_duration"
  log_duration "Step 2: Check cluster resources" "0"
  log_duration "Step 3: VM Deployment" "$deploy_duration"
  log_duration "Step 4: Statistics Collection" "$stats_duration"
  log_duration "Step 5: VM Stop" "$stop_duration"
  log_duration "Step 6: VM Start" "$start_vm_duration"
  log_duration "Step 7: Migration Setup" "$migration_duration"
  log_duration "Step 8: Undeploy VMs 10%" "$undeploy_duration"
  log_duration "Step 9: Deploy VMs 10%" "$deploy_remaining_duration"
  log_duration "Step 10: Statistics Collection Deploy 10%" "$vm_stats_duration"
  log_duration "Step 11: VM Undeploy 10% VMs" "$undeploy_duration"
  log_duration "Step 12: Deploying 10% VMs" "$deploy_remaining_duration"
  log_duration "Step 13: VM Statistics" "$vm_stats_duration"
  log_duration "Step 14: VM Operations" "$vm_ops_duration"
  log_duration "Step 14: VM Operations: Stopping VMs" "$vm_ops_stop_duration"
  log_duration "Step 14: VM Operations: Start VMs" "$vm_ops_start_vm_duration"
  log_duration "Step 15: Migration Cleanup" "$cleanup_ops_duration"
  log_duration "Step 16: Migration Percentage" "$migration_percent_duration"
  log_duration "Step 17: Migration parallelMigrationsPerCluster 2x nodes" "$migration_parallel_2x_duration"
  log_duration "Step 18: Migration parallelMigrationsPerCluster 4x nodes" "$migration_parallel_4x_duration"
  log_duration "Step 19: Migration parallelMigrationsPerCluster 8x nodes" "$migration_parallel_8x_duration"
  log_duration "Step 20: Controller Restart" "$controller_duration"
  log_duration "Step 21: Final Statistics" "$final_stats_duration"
  log_duration "Step 22: Drain node" "$drain_stats_duration"
  log_duration "Step 23: Final Cleanup" "$final_cleanup_duration"
  log_duration "Total Scenario Duration" "$duration"
  log_info "=== End Duration Summary ==="
}

# Function to prepare for tests
prepare_for_tests() {
  log_info "Preparing for tests"
  log_info "Operating System: $OS_TYPE"
  
  # Clean reports if requested
  if [ "${CLEAN_REPORTS:-false}" = "true" ]; then
    clean_all_reports
  fi
  
  # remove_report_dir
  # stop_migration
  # remove_vmops
  # undeploy_resources
}

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
log_info "========================================"

# Main execution
prepare_for_tests

# Run selected scenario
case $SCENARIO_NUMBER in
  1)
    VI_TYPE="persistentVolumeClaim"
    run_scenario "scenario_1" "$VI_TYPE"
    scenario_exit_code=$?
    if [ $scenario_exit_code -eq 0 ]; then
      if [ "$BOOTSTRAP_ONLY" = "true" ]; then
        log_success "Bootstrap completed successfully"
        exit 0
      else
        log_success "Scenario 1 (persistentVolumeClaim) completed successfully"
      fi
    else
      log_error "Scenario 1 failed with exit code: $scenario_exit_code"
      exit $scenario_exit_code
    fi
    ;;
  2)
    VI_TYPE="containerRegistry"
    run_scenario "scenario_2" "$VI_TYPE"
    scenario_exit_code=$?
    if [ $scenario_exit_code -eq 0 ]; then
      if [ "$BOOTSTRAP_ONLY" = "true" ]; then
        log_success "Bootstrap completed successfully"
        exit 0
      else
        log_success "Scenario 2 (containerRegistry) completed successfully"
      fi
    else
      log_error "Scenario 2 failed with exit code: $scenario_exit_code"
      exit $scenario_exit_code
    fi
    ;;
  *)
    log_error "Invalid scenario number: $SCENARIO_NUMBER. Use 1 or 2."
    exit 1
    ;;
esac

# Handle resource cleanup based on --keep-resources option
if [ "$KEEP_RESOURCES" = "true" ]; then
  log_info "=== KEEPING RESOURCES ==="
  log_info "Resources will be kept after tests (--keep-resources enabled)"
  log_success "All scenarios completed successfully - resources preserved"
else
  log_info "=== CLEANING UP RESOURCES ==="
  undeploy_resources
  log_success "All scenarios completed successfully - resources cleaned up"
fi
