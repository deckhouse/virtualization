#!/usr/bin/env bash


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


kubectl() {
  sudo /opt/deckhouse/bin/kubectl $@
}

d8() {
  sudo /opt/deckhouse/bin/d8 $@
}


d8_queue_main() {
  echo "$( d8 p queue main | grep -Po '(?<=length )([0-9]+)' )"
}

d8_queue_list() {
  d8 p queue list | grep -Po '([0-9]+)(?= active)'
}

d8_queue() {
  count=20
  local main_queue_ready=false
  local list_queue_ready=false

  for i in $(seq 1 $count) ; do
    if [ $(d8_queue_main) == "0" ]; then
      echo "main queue is clear"
      main_queue_ready=true
    else
      d8 p queue main | head -n25
    fi

    if [ $(d8_queue_list) == "0" ]; then
      echo "list queue is clear"
      list_queue_ready=true
    else
      d8 p queue list | head -n25
    fi

    if [ "$main_queue_ready" = true ] && [ "$list_queue_ready" = true ]; then
      break
    fi
    echo "Wait until queues are ready ${i}/${count}"
    sleep 60
  done
}

d8_ready() {
  local ready=false
  local count=10
  for i in $(seq 1 $count) ; do
    echo "Wait until deckhouse is ready ${i}/${count}"
    if kubectl -n d8-system wait deploy/deckhouse --for condition=available --timeout=60s; then
      ready=true
      break
    fi
  done

  if [ "$ready" = true ]; then
    log_success "Deckhouse is Ready!"
    echo "Checking queues"
    d8_queue
  else
    log_error "Deckhouse is not ready after ${count}m, check its queue for errors:"
    d8 p queue main | head -n25
    exit 1
  fi
}

start_time=$(date +%s)
log_info "Checking that deckhouse is ready"
d8_ready
end_time=$(date +%s)
difference=$((end_time - start_time))
log_success "Deckhouse is ready after $(date -ud "@$difference" +'%H:%M:%S')"
