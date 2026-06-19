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
# shellcheck source=.github/scripts/bash/e2e/deckhouse.sh
source "${SCRIPT_DIR}/deckhouse.sh"

require_env DEV_REGISTRY_DOCKER_CFG
require_env CURRENT_RELEASE

required_env_value() {
  local name="$1"

  require_env "${name}"
  printf '%s' "${!name}"
}

dev_registry_docker_cfg="$(required_env_value DEV_REGISTRY_DOCKER_CFG)"
current_release="$(required_env_value CURRENT_RELEASE)"

REGISTRY="$(registry_host_from_docker_cfg "${dev_registry_docker_cfg}")"

echo "[INFO] Apply ModuleSource prod config"
kubectl_apply_with_retry 20 10 show_deckhouse_state <<EOF
apiVersion: deckhouse.io/v1alpha1
kind: ModuleSource
metadata:
  name: deckhouse-dev
spec:
  registry:
    ca: ""
    dockerCfg: "${dev_registry_docker_cfg}"
    repo: "${REGISTRY}/sys/deckhouse-oss/modules"
    scheme: HTTPS
EOF

kubectl wait --for=jsonpath='{.status.phase}'=Active ms deckhouse-dev --timeout=30s

echo "[INFO] Apply Virtualization module config with current-release tag: ${current_release}"
kubectl_apply_with_retry 20 10 show_deckhouse_state <<EOF
apiVersion: deckhouse.io/v1alpha1
kind: ModuleConfig
metadata:
  name: virtualization
spec:
  enabled: true
  settings:
    dvcr:
      storage:
        persistentVolumeClaim:
          size: 10Gi
          storageClassName: nested-thin-r1
        type: PersistentVolumeClaim
    virtualMachineCIDRs:
      - 192.168.10.0/24
  source: deckhouse-dev
  version: 1
---
apiVersion: deckhouse.io/v1alpha2
kind: ModulePullOverride
metadata:
  name: virtualization
spec:
  imageTag: ${current_release}
  scanInterval: 10h
EOF

echo "[INFO] Show ModuleSource"
kubectl get ms

echo "[INFO] Show module config virtualization info"
kubectl get mc virtualization

echo "[INFO] Show ModulePullOverride virtualization info"
kubectl get mpo virtualization
