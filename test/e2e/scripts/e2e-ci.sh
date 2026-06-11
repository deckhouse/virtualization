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

TIMEOUT="${TIMEOUT:-3h}"
FOCUS="${FOCUS:-}"
LABELS="${LABELS:-}"
CSI="${CSI:-unknown}"
E2E_IMAGE_BASE_URL="${E2E_IMAGE_BASE_URL:-}"

readonly DEFAULT_IMAGE_BASE_URL="https://89d64382-20df-4581-8cc7-80df331f67fa.selstorage.ru"

date_tag="$(date +"%Y-%m-%d")"
e2e_report_file="e2e_report_${CSI}_${date_tag}.json"
e2e_output_file="e2e_output_${CSI}_${date_tag}.log"

rewrite_testdata_image_urls() {
  local image_base_url="${E2E_IMAGE_BASE_URL%/}"

  # No custom base URL requested: testdata already points to the default storage.
  if [ -z "${image_base_url}" ] || [ "${image_base_url}" = "${DEFAULT_IMAGE_BASE_URL}" ]; then
    return
  fi

  # Nothing to rewrite if testdata has not been copied.
  if [ ! -d /tmp/testdata ]; then
    return
  fi

  echo "[INFO] Rewriting testdata image base URL to: ${image_base_url}"
  # Escape sed replacement special characters (& and the # delimiter).
  local escaped_image_base_url="${image_base_url//&/\\&}"
  escaped_image_base_url="${escaped_image_base_url//#/\\#}"

  find /tmp/testdata -type f -exec sed -i "s#${DEFAULT_IMAGE_BASE_URL}#${escaped_image_base_url}#g" {} +
}

echo "[INFO] Kubernetes server version: ${SERVER_K8S_VERSION:-unknown}"
echo "[INFO] USB E2E supported: ${USB_SUPPORTED:-unknown}"
if [ -n "${LABELS}" ]; then
  echo "[INFO] Applying Ginkgo label filter: ${LABELS}"
fi

rewrite_testdata_image_urls

./scripts/precheck-prepare_ci.sh

set +e
ginkgo_args=(
  -v
  --race
  --timeout="${TIMEOUT}"
  --json-report="${e2e_report_file}"
)

if [ -n "${LABELS}" ]; then
  ginkgo_args+=(--label-filter="${LABELS}")
fi

if [ -n "${FOCUS}" ]; then
  ginkgo_args+=(--focus="${FOCUS}")
fi

go tool ginkgo "${ginkgo_args[@]}" . 2>&1 | tee "${e2e_output_file}"
ginkgo_exit_code="${PIPESTATUS[0]}"
set -e

echo "[INFO] Exit code: ${ginkgo_exit_code}"
exit "${ginkgo_exit_code}"
