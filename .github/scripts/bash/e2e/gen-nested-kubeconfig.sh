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
# shellcheck source=.github/scripts/bash/e2e/d8-ssh.sh
source "${SCRIPT_DIR}/d8-ssh.sh"

if [ "$#" -ne 5 ]; then
  echo "[ERROR] Usage: $0 <kubeconfig-path> <namespace> <prefix> <default-user> <github-output>" >&2
  exit 1
fi

kubeconfig_path="$1"
NAMESPACE="$2"
PREFIX="$3"
DEFAULT_USER="$4"
github_output="$5"
export NAMESPACE DEFAULT_USER

nested_master=$(kubectl -n "${NAMESPACE}" get vm -l "group=${PREFIX}-master" -o jsonpath="{.items[0].metadata.name}")

echo "[INFO] Copy script for generating kubeconfig in nested cluster"
echo "[INFO] Copy scripts/gen-kubeconfig.sh to master"
d8vscp "./scripts/gen-kubeconfig.sh" "${DEFAULT_USER}@${nested_master}.${NAMESPACE}:/tmp/gen-kubeconfig.sh"
echo ""
d8vscp "./scripts/deckhouse-queue.sh" "${DEFAULT_USER}@${nested_master}.${NAMESPACE}:/tmp/deckhouse-queue.sh"
echo ""

echo "[INFO] Set file exec permissions"
d8vssh 'chmod +x /tmp/{gen-kubeconfig.sh,deckhouse-queue.sh}'
d8vssh 'ls -la /tmp/'
echo "[INFO] Check d8 queue in nested cluster"
d8vssh 'sudo /tmp/deckhouse-queue.sh'

echo "[INFO] Generate kube conf in nested cluster"
echo "[INFO] Run gen-kubeconfig.sh in nested cluster"
d8vssh "sudo /tmp/gen-kubeconfig.sh nested-sa nested nested-e2e /${kubeconfig_path}"
echo ""

echo "[INFO] Copy kubeconfig to runner"
echo "[INFO] ${DEFAULT_USER}@${nested_master}.$NAMESPACE:/${kubeconfig_path} ./${kubeconfig_path}"
d8vscp "${DEFAULT_USER}@${nested_master}.$NAMESPACE:/${kubeconfig_path}" "./${kubeconfig_path}"

echo "[INFO] Set rights for kubeconfig"
echo "[INFO] sudo chown 1001:1001 ${kubeconfig_path}"
sudo chown 1001:1001 "${kubeconfig_path}"
echo " "

echo "[INFO] Kubeconf to github output"
CONFIG=$(base64 -w 0 < "${kubeconfig_path}")
CONFIG=$(echo "${CONFIG}" | base64 -w 0)
echo "kubeconfig=$CONFIG" >> "$github_output"
