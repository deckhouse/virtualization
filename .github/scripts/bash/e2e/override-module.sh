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

# Installs an external module from the dev registry at a given image tag via
# ModulePullOverride, so the nightly runs against a merge request build of the
# module before it is released. Deckhouse resolves the override tag in the
# module's active ModuleSource, so the ModuleConfig is pointed at the dev source
# first; otherwise the tag would be looked up in the release registry.
#
# Usage: override-module.sh <module> <image-tag>

require_env DEV_REGISTRY_DOCKER_CFG

module="${1:?module name is required}"
image_tag="${2:?image tag is required}"
# shellcheck disable=SC2153,SC2154
dev_registry_docker_cfg="${DEV_REGISTRY_DOCKER_CFG}"

show_module_state() {
  echo "[DEBUG] Module ${module}"
  kubectl get modules "${module}" -o wide || true
  echo "[DEBUG] ModuleConfig ${module}"
  kubectl get mc "${module}" -o yaml || true
  echo "[DEBUG] ModulePullOverride ${module}"
  kubectl get mpo "${module}" -o yaml || true
  show_deckhouse_state
}

switch_module_source() {
  echo "[INFO] Point ModuleConfig ${module} at ModuleSource ${DEV_MODULE_SOURCE}"
  run_with_retry 12 10 kubectl patch mc "${module}" --type merge \
    -p "{\"spec\":{\"source\":\"${DEV_MODULE_SOURCE}\"}}"
}

wait_for_module_source() {
  local count=30
  local delay=10
  local source

  for i in $(seq 1 "$count"); do
    source="$(kubectl get modules "${module}" -o jsonpath='{.properties.source}' 2>/dev/null || true)"
    echo "[INFO] Wait for module ${module} to switch to ${DEV_MODULE_SOURCE} ${i}/${count}, source=${source:-none}"

    if [ "${source}" = "${DEV_MODULE_SOURCE}" ]; then
      return 0
    fi

    if (( i % 5 == 0 )); then
      show_module_state
    fi

    if [ "$i" -lt "$count" ]; then
      sleep "$delay"
    fi
  done

  echo "[ERROR] Module ${module} did not switch to ModuleSource ${DEV_MODULE_SOURCE}" >&2
  show_module_state
  return 1
}

apply_module_pull_override() {
  echo "[INFO] Apply ModulePullOverride ${module} with imageTag ${image_tag}"
  kubectl_apply_with_retry 20 10 show_module_state <<EOF
apiVersion: deckhouse.io/v1alpha2
kind: ModulePullOverride
metadata:
  name: ${module}
spec:
  imageTag: ${image_tag}
  # A merge request tag moves with every push to its branch; a long interval
  # keeps the module from rolling its pods in the middle of the test run.
  scanInterval: 120h
EOF
}

# Under an active override Deckhouse reports the image tag as the module
# version, so version == tag && phase == Ready means the override is installed.
wait_for_module_override() {
  local count=60
  local delay=10
  local version
  local phase

  for i in $(seq 1 "$count"); do
    version="$(kubectl get modules "${module}" -o jsonpath='{.properties.version}' 2>/dev/null || true)"
    phase="$(kubectl get modules "${module}" -o jsonpath='{.status.phase}' 2>/dev/null || true)"
    echo "[INFO] Wait for module ${module} ${image_tag} to be Ready ${i}/${count}, version=${version:-unknown}, phase=${phase:-unknown}"

    if [ "${version}" = "${image_tag}" ] && [ "${phase}" = "Ready" ]; then
      echo "[SUCCESS] Module ${module} runs ${image_tag} from ${DEV_MODULE_SOURCE}"
      kubectl get modules "${module}" -o wide
      kubectl get mpo "${module}"
      return 0
    fi

    if (( i % 5 == 0 )); then
      show_module_state
    fi

    if [ "$i" -lt "$count" ]; then
      sleep "$delay"
    fi
  done

  echo "[ERROR] Module ${module} did not become Ready with imageTag ${image_tag}" >&2
  show_module_state
  return 1
}

apply_dev_module_source "${dev_registry_docker_cfg}"
wait_for_modulesource_active
wait_for_module_dev_source "${module}"
switch_module_source
wait_for_module_source
apply_module_pull_override
wait_for_module_override
wait_for_deckhouse_queue
