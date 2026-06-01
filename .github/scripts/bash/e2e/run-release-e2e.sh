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

require_env CURRENT_RELEASE
require_env CSI
require_env STORAGE_CLASS_NAME
require_env E2E_CONFIG
require_env RELEASE_TEST_PHASE
require_env RELEASE_UPGRADE_CONTEXT_PATH
require_env RUNNER_TEMP
require_env GITHUB_WORKSPACE

required_env_value() {
  local name="$1"

  require_env "${name}"
  printf '%s' "${!name}"
}

current_release="$(required_env_value CURRENT_RELEASE)"
storage_type="$(required_env_value CSI)"
runner_temp="$(required_env_value RUNNER_TEMP)"
github_workspace="$(required_env_value GITHUB_WORKSPACE)"

echo "[INFO] Current release tag: ${current_release}"
echo "[INFO] Storage type: ${storage_type}"
echo ""
echo "[INFO] Verifying virtualization module is running"
kubectl get modules virtualization
kubectl get mpo virtualization
echo ""
echo "[INFO] Running dedicated release suite"
echo "[INFO] Resources will be intentionally left in the cluster for the upgrade test"

cd ./test/e2e/
GINKGO_RESULT="$(mktemp -p "${runner_temp}")"
junit_report="${github_workspace}/test/e2e/release_current_suite.xml"
set +e
go tool ginkgo \
  -v --race --timeout=45m \
  --junit-report="${junit_report}" \
  ./release | tee "${GINKGO_RESULT}"
GINKGO_EXIT_CODE=$?
set -e
echo "[INFO] Exit code: ${GINKGO_EXIT_CODE}"
exit "${GINKGO_EXIT_CODE}"
