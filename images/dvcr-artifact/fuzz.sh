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

mkdir -p /fuzz

# Time threshold in seconds to stop if no new paths found
inactivityTimeout=7200  # 2 hours = 7200 seconds
fuzzTime=${FUZZ_TIME:-2m}

files=$(grep -r --include='**_test.go' --files-with-matches 'func Fuzz' .)
for file in ${files}; do
  funcs=$(grep -o 'func \(Fuzz\w*\)' $file)

  for func in ${funcs}; do
    echo "Fuzzing $func in $file"
    parentDir=$(dirname $file)

    logfile="/fuzz/fuzz_$(basename "$func")_$(date +%s).log"

    go test $parentDir -fuzz=$func -cover -parallel=1 -v > "$logfile" 2>&1 &
    fuzz_pid=$!

    last_new_path=$(date +%s)

    while ps -p "$fuzz_pid" > /dev/null 2>&1; do
      # Check if any new path was found in recent log
      new_path_line=$(tail -n 50 "$logfile" | grep "new interesting" | tail -1)
      if [[ -n "$new_path_line" ]]; then
        last_new_path=$(date +%s)
        echo "💡 New path found at $(date)"
      fi

      current_time=$(date +%s)
      inactive_duration=$((current_time - last_new_path))

      if (( inactive_duration > inactivityTimeout )); then
        echo "🕒 No new paths for $inactivityTimeout seconds. Stopping this fuzz test."
        kill "$fuzz_pid" 2>/dev/null || true
        wait "$fuzz_pid" 2>/dev/null || true
        break
      fi

      sleep 60  # check every minute
    done
  done
done

