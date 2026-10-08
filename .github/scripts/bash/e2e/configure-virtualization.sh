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
require_env NESTED_STORAGE_CLASS_NAME
require_env VIRTUALIZATION_TAG

# shellcheck disable=SC2153,SC2154
dev_registry_docker_cfg="${DEV_REGISTRY_DOCKER_CFG}"
# shellcheck disable=SC2153,SC2154
nested_storage_class_name="${NESTED_STORAGE_CLASS_NAME}"
# shellcheck disable=SC2153,SC2154
virtualization_tag="${VIRTUALIZATION_TAG}"

# No featureGates here: the module webhook validates only gates being added to a
# live config, so gates set at creation time reach the controller unchecked and a
# gate this edition locks makes it exit on start - taking that very webhook with
# it. patch-virtualization-feature-gates.sh adds them once the module is Ready.
apply_virtualization_module_config() {
  echo "[INFO] Apply Virtualization module config"
  kubectl_apply_with_retry 20 10 show_deckhouse_state <<EOF
apiVersion: deckhouse.io/v1alpha1
kind: ModuleConfig
metadata:
  name: virtualization
  annotations:
    # The e2e suite migrates many VMs in parallel: every migration limit must
    # be disabled, otherwise migrations queue up and the specs time out (the
    # suite enforces this with the migration limits precheck).
    virtualization.deckhouse.io/max-active-outbound-migrations-per-node: disabled
    virtualization.deckhouse.io/max-active-inbound-migrations-per-node: disabled
    virtualization.deckhouse.io/max-active-migrations-per-cluster: disabled
spec:
  enabled: true
  settings:
    dvcr:
      storage:
        persistentVolumeClaim:
          size: 10Gi
          storageClassName: ${nested_storage_class_name}
        type: PersistentVolumeClaim
    virtualMachineCIDRs:
      - 192.168.10.0/24
  source: ${DEV_MODULE_SOURCE}
  version: 1
---
apiVersion: deckhouse.io/v1alpha2
kind: ModulePullOverride
metadata:
  name: virtualization
spec:
  imageTag: ${virtualization_tag}
  scanInterval: 120h
EOF
}

show_virtualization_config() {
  echo "[INFO] Show ModuleSource"
  kubectl get ms

  echo "[INFO] Show module config virtualization info"
  kubectl get mc virtualization

  echo "[INFO] Show ModulePullOverride virtualization info"
  kubectl get mpo virtualization
}

apply_dev_module_source "$dev_registry_docker_cfg"
wait_for_modulesource_active
wait_for_deckhouse_queue
wait_for_module_dev_source virtualization
wait_for_deckhouse_queue
apply_virtualization_module_config
show_virtualization_config
