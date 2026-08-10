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

on_error() {
  local exit_code=$?
  echo "[ERROR] Command failed with exit code ${exit_code} at line ${BASH_LINENO[0]}: ${BASH_COMMAND}" >&2
}

require_env() {
  local name="$1"

  if [ -z "${!name:-}" ]; then
    echo "[ERROR] Required environment variable is not set: ${name}" >&2
    exit 1
  fi
}

# Echoes the registry host parsed from a base64-encoded dockerconfigjson.
registry_host_from_docker_cfg() {
  local docker_cfg="$1"
  base64 -d <<< "$docker_cfg" | jq -r '.auths | to_entries[0].key'
}

# Echoes the modules repo path for a given registry host.
# dev registries serve modules under sys/deckhouse-oss, stage/prod under deckhouse/ee.
modules_repo_for_registry() {
  local registry="$1"
  if [[ "$registry" =~ dev- ]]; then
    printf '%s/sys/deckhouse-oss/modules' "$registry"
  else
    printf '%s/deckhouse/ee/modules' "$registry"
  fi
}

# Echoes the virtualization feature gates supported by every given release, one
# per line. A gate the pulled module does not know fails ModulePullOverride
# validation and leaves the module uninstalled, so a gate is only listed when
# none of the releases predate it. In-place resize never shipped in the 1.9
# line. Anything that is not a release tag (a PR reference, a build off main)
# carries the current gates.
# Usage: virtualization_feature_gates [release]...
virtualization_feature_gates() {
  local release

  echo "HotplugCPUWithLiveMigration"
  echo "HotplugMemoryWithLiveMigration"

  for release in "$@"; do
    if [[ "${release}" =~ ^v([0-9]+)\.([0-9]+)\. ]] && (( BASH_REMATCH[1] == 1 && BASH_REMATCH[2] < 10 )); then
      return 0
    fi
  done

  echo "HotplugCPUAndMemoryWithInPlaceResize"
}

# Reads a manifest from stdin and applies it with retries.
# Usage: kubectl_apply_with_retry [count] [delay] [diag_fn]
# diag_fn is an optional function name invoked on each failed attempt.
kubectl_apply_with_retry() {
  local count="${1:-12}"
  local delay="${2:-10}"
  local diag_fn="${3:-}"
  local manifest i
  manifest="$(mktemp)"
  # shellcheck disable=SC2064 # expand manifest path now so the RETURN trap cleans the right file
  trap "rm -f '${manifest}'" RETURN
  cat > "$manifest"

  for ((i = 1; i <= count; i++)); do
    echo "[INFO] kubectl apply attempt ${i}/${count}"
    if kubectl apply -f "$manifest"; then
      return 0
    fi

    if [ "$i" -lt "$count" ]; then
      echo "[WARN] kubectl apply failed, retrying in ${delay}s..."
      if [ -n "$diag_fn" ]; then
        "$diag_fn" || true
      fi
      sleep "$delay"
    fi
  done

  echo "[ERROR] kubectl apply failed after ${count} attempts" >&2
  if [ -n "$diag_fn" ]; then
    "$diag_fn" || true
  fi
  return 1
}

trap on_error ERR
