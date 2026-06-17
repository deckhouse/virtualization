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

if [ "$#" -ne 3 ]; then
  echo "[ERROR] Usage: $0 <namespace> <prefix> <default-user>" >&2
  exit 1
fi

NAMESPACE="$1"
PREFIX="$2"
DEFAULT_USER="$3"
export NAMESPACE DEFAULT_USER

nested_master=$(kubectl -n "${NAMESPACE}" get vm -l "group=${PREFIX}-master" -o jsonpath="{.items[0].metadata.name}")

echo "[INFO] Pods in namespace $NAMESPACE"
kubectl get pods -n "${NAMESPACE}"
echo ""

echo "[INFO] VMs in namespace $NAMESPACE"
kubectl get vm -n "${NAMESPACE}"
echo ""

echo "[INFO] VDs in namespace $NAMESPACE"
kubectl get vd -n "${NAMESPACE}"
echo ""

echo "Check connection to master"
d8vssh "${nested_master}" 'echo master os-release: ; cat /etc/os-release; echo " "; echo master hostname: ; hostname'
echo ""
