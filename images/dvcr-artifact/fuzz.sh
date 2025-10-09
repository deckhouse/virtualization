#!/bin/bash

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

set -e

go clean -testcache -fuzzcache

mkdir -p /tmp/fuzz

fuzz_pids=()
inactivityTimeout=7200  # 2 hours = 7200 seconds

cleanup() {
  echo -e "\nReceived interrupt. Stopping all fuzz tests..."

  for pid in "${FUZZ_PIDS[@]}"; do
    kill "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
  done

  # kill workers if they are still running
  pids=$(ps aux | grep 'fuzzworker' | awk '{print $2}')
  if [[ ! -z "$pids" ]]; then
    echo "$pids" | xargs kill 2>/dev/null || true
    sleep 1  # wait a moment for them to terminate
    echo "$pids" | xargs kill -9 2>/dev/null || true
  fi

  echo "All fuzz tests stopped. Exiting."
  exit 0
}

trap 'cleanup' SIGINT

files=$(grep -r --include='**_test.go' --files-with-matches 'func Fuzz' .)
for file in ${files}; do
  funcs=$(grep -o 'func \(Fuzz\w*\)' $file | sed -E 's/func (Fuzz[^ ]*).*/\1/' | sort | uniq)

  for func in ${funcs}; do
    echo "Fuzzing $func in $file"
    parentDir=$(dirname $file)

    logfile="/tmp/fuzz/fuzz_$(basename "$func")_$(date +%s).log"

    go test $parentDir -fuzz=$func -cover -v > "$logfile" 2>&1 &
    fuzz_pid=$!
    fuzz_pids+=("$fuzz_pid")

    last_new_path=$(date +%s)
    last_new_count=-1

    while ps -p "$fuzz_pid" > /dev/null 2>&1; do
      # Check if any new path was found in recent log
      new_path_line=$(tail -n 50 "$logfile" | grep -oE 'new interesting: [0-9]+' | tail -1)
      if [[ -n "$new_path_line" ]]; then
        current_new_count=$(echo "$new_path_line" | awk '{print $3}')

        # Only update timestamp if new value is higher than before
        if (( current_new_count > last_new_count )); then
          echo "Test: $func. New path count increased to $current_new_count at $(date '+%T %Y-%m-%d')"
          last_new_path=$(date +%s)
          last_new_count=$current_new_count
        fi
      fi

      current_time=$(date +%s)
      inactive_duration=$((current_time - last_new_path))

      if (( inactive_duration > inactivityTimeout )); then
        echo "Test: $func. No new paths for $inactivityTimeout seconds. Stopping this fuzz test."
        kill "$fuzz_pid" 2>/dev/null || true
        wait "$fuzz_pid" 2>/dev/null || true

        # kill workers if they are still running
        pids=$(ps aux | grep 'fuzzworker' | awk '{print $2}')
        if [[ ! -z "$pids" ]]; then
          echo "$pids" | xargs kill 2>/dev/null || true
          sleep 1  # wait a moment for them to terminate
          echo "$pids" | xargs kill -9 2>/dev/null || true
        fi

        break
      fi

      sleep 60  # check every minute
    done

    echo "Finished fuzzing $func"
  done
done

echo "All fuzz tests completed."
