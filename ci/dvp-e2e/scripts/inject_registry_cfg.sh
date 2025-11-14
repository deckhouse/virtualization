#!/usr/bin/env bash
set -euo pipefail

# Usage:
#   inject_registry_cfg.sh -f /path/to/values.yaml -v <docker_cfg_b64>

file=""
val="${REGISTRY_DOCKER_CFG:-}"

while getopts ":f:v:" opt; do
  case $opt in
    f) file="$OPTARG" ;;
    v) val="$OPTARG" ;;
    *) echo "Usage: $0 -f <values.yaml> -v <docker_cfg_b64>" >&2; exit 2 ;;
  esac
done

if [ -z "${file}" ] || [ -z "${val}" ]; then
  echo "Usage: $0 -f <values.yaml> -v <docker_cfg_b64>" >&2
  exit 2
fi

export VAL="$val"
yq eval --inplace '.deckhouse.registryDockerCfg = strenv(VAL)' "$file"
