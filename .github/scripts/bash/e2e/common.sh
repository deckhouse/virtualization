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

# Echoes the feature gates a release accepts, one per line. The module bundle
# carries openapi/config-values.yaml, and its enum is the very schema the
# ModuleConfig of that release is validated against - so every release states
# its own gate list and no version table has to be kept by hand.
# Usage: module_feature_gates <module_source> <release>
module_feature_gates() {
  local module_source="$1"
  local release="$2"

  crane export "${module_source}/virtualization:${release}" - |
    tar -Oxf - openapi/config-values.yaml |
    yq '.properties.featureGates.items.enum[]'
}

# Echoes the gates every given release accepts, one per line, in the order the
# first release lists them.
# Usage: virtualization_feature_gates <module_source> <release>...
virtualization_feature_gates() {
  local module_source="$1"
  shift
  local gates release other

  gates="$(module_feature_gates "${module_source}" "$1")"
  shift

  for release in "$@"; do
    other="$(module_feature_gates "${module_source}" "${release}")"
    gates="$(grep -xF -f <(printf '%s\n' "${other}") <<< "${gates}" || true)"
  done

  if [ -n "${gates}" ]; then
    printf '%s\n' "${gates}"
  fi
}

# Server-side dry-run of the feature gate patch: the moduleconfig webhook
# declares sideEffects: None, so the real admission chain can be asked whether a
# gate list would be accepted without writing anything. Echoes the admission
# error when it is not.
# Usage: gate_accepted <gates_json>
gate_accepted() {
  local gates_json="$1"
  local output

  if output="$(kubectl patch mc virtualization --type merge --dry-run=server \
    -p "{\"spec\":{\"settings\":{\"featureGates\":${gates_json}}}}" 2>&1)"; then
    return 0
  fi

  printf '%s' "${output}"
  return 1
}

# Waits until the moduleconfig webhook answers again. It is served by
# virtualization-controller itself and has no failurePolicy, so right after an
# image switch every patch is rejected for a while. The probe dry-runs the gates
# the config already carries: that adds no gate, so the validator returns nil
# and only an unreachable webhook can fail it.
# Usage: moduleconfig_writable [count] [delay]
moduleconfig_writable() {
  local count="${1:-12}"
  local delay="${2:-10}"
  local current error i

  for ((i = 1; i <= count; i++)); do
    current="$(kubectl get mc virtualization -o jsonpath='{.spec.settings.featureGates}' 2>/dev/null || true)"

    if error="$(gate_accepted "${current:-[]}")"; then
      return 0
    fi

    echo "[WARN] Module config is not writable yet (attempt ${i}/${count}): ${error}"
    if [ "$i" -lt "$count" ]; then
      sleep "$delay"
    fi
  done

  return 1
}

# Echoes images_digests.json packaged in the module image of a given release.
# Usage: module_images_digests <module_source> <release>
module_images_digests() {
  local module_source="$1"
  local release="$2"

  crane export "${module_source}/virtualization:${release}" - | tar -Oxf - images_digests.json
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
