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

# Retry a d8 command on transient transport failures (websocket "close 1006",
# "Broken pipe", ssh "exit status 255"). d8 reports all of these as exit 1, so
# any non-zero exit is retried.
# ponytail: also retries long readiness waits (deckhouse-queue.sh); their re-run
# re-checks state and stays within the e2e step timeout, so no per-call opt-out.
d8_retry() {
  local i
  for ((i = 1; i <= 3; i++)); do
    if "$@"; then
      return 0
    fi
    if [ "$i" -lt 3 ]; then
      echo "[WARN] d8 command failed (attempt ${i}/3), retrying in 15s..." >&2
      sleep 15
    fi
  done
  return 1
}

d8vssh() {
  require_env DEFAULT_USER
  require_env NAMESPACE

  local default_user="${DEFAULT_USER:-}"
  local namespace="${NAMESPACE:-}"
  local host
  local cmd

  case "$#" in
  1)
    require_env nested_master
    host="${nested_master:-}"
    cmd="$1"
    ;;
  2)
    host="$1"
    cmd="$2"
    ;;
  *)
    echo "[ERROR] Usage: d8vssh [host] command" >&2
    return 1
    ;;
  esac

  d8_retry d8 v ssh -i ./tmp/ssh/cloud \
    --local-ssh=true \
    --local-ssh-opts="-o StrictHostKeyChecking=no" \
    --local-ssh-opts="-o UserKnownHostsFile=/dev/null" \
    --local-ssh-opts="-o ServerAliveInterval=15" \
    --local-ssh-opts="-o ServerAliveCountMax=8" \
    --local-ssh-opts="-o ConnectTimeout=10" \
    "${default_user}@${host}.${namespace}" \
    -c "$cmd"
}

d8vscp() {
  local source=$1
  local dest=$2

  d8_retry d8 v scp -i ./tmp/ssh/cloud \
    --local-ssh=true \
    --local-ssh-opts="-o StrictHostKeyChecking=no" \
    --local-ssh-opts="-o UserKnownHostsFile=/dev/null" \
    "$source" "$dest"
}
