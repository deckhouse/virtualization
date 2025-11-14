#!/usr/bin/env bash

# Copyright 2025 Flant JSC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
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
