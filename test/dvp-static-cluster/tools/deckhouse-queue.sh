#!/usr/bin/env bash

# Copyright 2025 Flant JSC
# 
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
# 
#      http://www.apache.org/licenses/LICENSE-2.0
# 
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

get_current_date() {
  date +"%H:%M:%S %d-%m-%Y"
}

get_timestamp() {
  date +%s
}

log_info() {
  local message="$1"
  local timestamp=$(get_current_date)
  echo -e "[$timestamp] ${BLUE}[INFO]${NC} $message"
}

log_success() {
  local message="$1"
  local timestamp=$(get_current_date)
  echo -e "[$timestamp] ${GREEN}[SUCCESS]${NC} $message"
}

log_warning() {
  local message="$1"
  local timestamp=$(get_current_date)
  echo -e "[$timestamp] ${YELLOW}[WARNING]${NC} $message"
}

log_error() {
  local message="$1"
  local timestamp=$(get_current_date)
  echo -e "[$timestamp] ${RED}[ERROR]${NC} $message"
}

kubectl() {
  /opt/deckhouse/bin/kubectl $@
  # sudo /opt/deckhouse/bin/kubectl $@
}

d8() {
  /opt/deckhouse/bin/d8 $@
  # sudo /opt/deckhouse/bin/d8 $@
}


d8_queue_main() {
  echo "$( d8 p queue main | grep -Po '(?<=length )([0-9]+)' )"
}

d8_queue_list() {
  d8 p queue list | grep -Po '([0-9]+)(?= active)'
}

d8_queue() {
  local count=90
  # local main_queue_ready=false
  local list_queue_ready=false

  for i in $(seq 1 $count) ; do
    # if [ $(d8_queue_main) == "0" ]; then
    #   echo "main queue is clear"
    #   main_queue_ready=true
    # else
    #   echo "Show main queue"
    #   d8 p queue main | head -n25 || echo "Failed to retrieve main queue"
    # fi

    if [ $(d8_queue_list) == "0" ]; then
      echo "list queue list is clear"
      list_queue_ready=true
    else
      echo "Show queue list"
      d8 p queue list | head -n25 || echo "Failed to retrieve queue"
    fi

    if [ "$list_queue_ready" = true ]; then
    # if [ "$main_queue_ready" = true ] && [ "$list_queue_ready" = true ]; then
      break
    fi
    echo "Wait until queues are empty ${i}/${count}"
    sleep 10
  done
}

d8_ready() {
  local ready=false
  local count=60
  common_start_time=$(get_timestamp)
  for i in $(seq 1 $count) ; do
    start_time=$(get_timestamp)
    if kubectl -n d8-system wait deploy/deckhouse --for condition=available --timeout=20s 2>/dev/null; then
      ready=true
      break
    fi
    end_time=$(get_timestamp)
    difference=$((end_time - start_time))
    log_info "Wait until deckhouse is ready ${i}/${count} after ${difference}s"
    if (( i % 5 == 0 )); then
      kubectl -n d8-system get pods
      d8 p queue list | head -n25 || echo "Failed to retrieve queue"
    fi
  done

  if [ "$ready" = true ]; then
    log_success "Deckhouse is Ready!"
    echo "Checking queues"
    d8_queue
  else
    common_end_time=$(get_timestamp)
    common_difference=$((common_end_time - common_start_time))
    common_formatted_difference=$(date -u +'%H:%M:%S' -d "@$common_difference")
    log_error "Deckhouse is not ready after ${count} attempts and ${common_formatted_difference} time, check its queue for errors:"
    d8 p queue main | head -n25
    exit 1
  fi
}

start_time=$(get_timestamp)
log_info "Checking that deckhouse is ready"
d8_ready
end_time=$(get_timestamp)
difference=$((end_time - start_time))
log_success "Deckhouse is ready after $(date -ud "@$difference" +'%H:%M:%S')"
