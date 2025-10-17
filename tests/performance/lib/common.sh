#!/usr/bin/env bash

# Common utilities and configuration for performance testing
# This module provides shared functionality used across all other modules

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
LOG_FILE=""
CURRENT_SCENARIO=""
VM_OPERATIONS_LOG=""

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Centralized logging functions
init_logging() {
    local scenario_name=$1
    local vi_type=$2
    LOG_FILE="$REPORT_DIR/${scenario_name}_${vi_type}/test.log"
    VM_OPERATIONS_LOG="$REPORT_DIR/${scenario_name}_${vi_type}/vm_operations.log"
    CURRENT_SCENARIO="${scenario_name}_${vi_type}"
    mkdir -p "$(dirname "$LOG_FILE")"
    echo "=== Test started at $(get_current_date) ===" > "$LOG_FILE"
    echo "=== VM Operations Report started at $(get_current_date) ===" > "$VM_OPERATIONS_LOG"
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

create_report_dir() {
  local scenario_name=$1
  local vi_type=$2
  local base_dir="$REPORT_DIR/${scenario_name}_${vi_type}"
  mkdir -p "$base_dir/statistics"
  mkdir -p "$base_dir/vpa"
  echo "$base_dir"
}

remove_report_dir() {
  local dir=${1:-$REPORT_DIR}
  rm -rf $dir
}

# Function to prepare for tests
prepare_for_tests() {
  log_info "Preparing for tests"
  log_info "Operating System: $OS_TYPE"
}

