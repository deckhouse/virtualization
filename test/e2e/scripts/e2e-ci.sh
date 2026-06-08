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

date_tag="$(date +"%Y-%m-%d")"
e2e_report_file="e2e_report_${CSI}_${date_tag}.json"
e2e_output_file="e2e_output_${CSI}_${date_tag}.log"

echo "[INFO] Kubernetes server version: ${SERVER_K8S_VERSION:-unknown}"
echo "[INFO] USB E2E supported: ${USB_SUPPORTED:-unknown}"
if [ -n "${LABELS}" ]; then
  echo "[INFO] Applying Ginkgo label filter: ${LABELS}"
fi

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
