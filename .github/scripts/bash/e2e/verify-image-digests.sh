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

require_env NEW_RELEASE
require_env DEV_MODULE_SOURCE

required_env_value() {
  local name="$1"

  require_env "${name}"
  printf '%s' "${!name}"
}

new_release="$(required_env_value NEW_RELEASE)"
dev_module_source="$(required_env_value DEV_MODULE_SOURCE)"

MODULE_IMAGE="${dev_module_source}/virtualization:${new_release}"
echo "[INFO] Extracting images_digests.json from virtualization:${new_release}"
images_hash="$(crane export "${MODULE_IMAGE}" - | tar -Oxf - images_digests.json)"
echo "[INFO] Expected image digests:"
echo "::group::images_digests.json"
echo "${images_hash}" | jq .
echo "::endgroup::"

audit_status="$(kubectl get mc virtualization -o=jsonpath='{.spec.settings.audit.enabled}' 2>/dev/null || true)"
audit_image_skip="true"
if [[ -n "${audit_status}" && "${audit_status}" == "true" ]]; then
  audit_image_skip="false"
fi

SKIP_IMAGES=()
if [[ "${audit_image_skip}" == "true" ]]; then
  SKIP_IMAGES+=("virtualizationAudit")
fi
SKIP_IMAGES+=("virtualizationDraUsb")

is_skipped_image() {
  local img="$1"

  if [[ -z "${img}" ]]; then
    return 1
  fi

  for skip in "${SKIP_IMAGES[@]}"; do
    if [[ "${img}" == "${skip}" ]]; then
      return 0
    fi
  done

  return 1
}

retry_count=0
max_retries=120
sleep_interval=5

while true; do
  all_hashes_found=true

  v12n_pods="$(kubectl -n d8-virtualization get pods -o json | jq -c)"

  while IFS= read -r image_entry; do
    image="$(echo "${image_entry}" | jq -r '.key')"
    hash="$(echo "${image_entry}" | jq -r '.value')"

    if [[ "${image,,}" =~ (libguestfs|predeletehook) ]]; then
      continue
    fi

    if is_skipped_image "${image}"; then
      echo "- SKIP ${image}"
      continue
    fi

    if echo "${v12n_pods}" | grep -q "${hash}"; then
      echo "- OK   ${image} ${hash}"
    else
      echo "- MISS ${image} ${hash}"
      all_hashes_found=false
    fi
  done < <(echo "${images_hash}" | jq -c '. | to_entries | sort_by(.key)[]')

  if [[ "${all_hashes_found}" == "true" ]]; then
    echo "[SUCCESS] All image hashes found in pods after upgrade to ${new_release}"
    break
  fi

  retry_count=$((retry_count + 1))
  echo "[INFO] Some hashes are missing, rechecking... Attempt: ${retry_count}/${max_retries}"

  if [[ "${retry_count}" -ge "${max_retries}" ]]; then
    echo "[ERROR] Timeout reached after $((retry_count * sleep_interval))s. Some image hashes are still missing."
    echo "::group::pods in d8-virtualization"
    kubectl -n d8-virtualization get pods -o wide || true
    echo "::endgroup::"
    exit 1
  fi

  sleep "${sleep_interval}"
done
