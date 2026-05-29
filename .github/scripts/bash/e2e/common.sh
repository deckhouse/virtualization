#!/usr/bin/env bash

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

trap on_error ERR
