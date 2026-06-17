#!/usr/bin/env bash

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
