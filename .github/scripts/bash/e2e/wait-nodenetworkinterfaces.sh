#!/usr/bin/env bash

# Copyright 2026 Flant JSC
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

set -Eeuo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=.github/scripts/bash/e2e/common.sh
source "${SCRIPT_DIR}/common.sh"

count=60
success=false
wait_time_seconds=5

for i in $(seq 1 "$count"); do
  nodes=$(kubectl get nodes -o name | wc -l)
  actual=$(kubectl get nodenetworkinterfaces -o json | jq -r '.items[] | select(.status.operationalState == "Up") | .metadata.name' | wc -l) || true
  expected=$((nodes * 2))

  echo "[INFO] Attempt $i/$count: expected=$expected, actual=$actual"

  if [ "$actual" -ge "$expected" ]; then
    echo "[SUCCESS] All nodenetworkinterfaces are present (expected=$expected, actual=$actual)"
    kubectl get nodenetworkinterfaces
    success=true
    break
  fi

  if (( i % 5 == 0 )); then
    echo "::group::[DEBUG] show namespaces d8-sdn"
    kubectl -n d8-sdn get pods || true
    echo "::endgroup::"

    echo "::group::[DEBUG] show nodenetworkinterfaces d8-sdn"
    kubectl get nodenetworkinterfaces || true
    echo "::endgroup::"

    echo "[INFO] Retrying in 10 seconds..."
    sleep "$wait_time_seconds"
  elif [ "$i" -lt "$count" ]; then
    echo "[INFO] Retrying in 10 seconds..."
    sleep "$wait_time_seconds"
  fi
done

if [ "$success" = false ]; then
  echo "[ERROR] Failed to get all nodenetworkinterfaces after $count attempts (expected=$expected)"
  echo "[DEBUG] Show namespaces d8-sdn"
  kubectl -n d8-sdn get pods || true
  echo "[DEBUG] Show nodenetworkinterfaces d8-sdn"
  kubectl get nodenetworkinterfaces || true
  exit 1
fi
