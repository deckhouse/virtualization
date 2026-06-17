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

# Read a required env var by name and print its value.
# Indirect expansion (${!name}) keeps shellcheck from flagging the env vars as unassigned.
required_env_value() {
  local name="$1"

  require_env "${name}"
  printf '%s' "${!name}"
}

require_env STORAGE_CLASS_NAME
require_env E2E_CONFIG

release_test_phase="$(required_env_value RELEASE_TEST_PHASE)"
runner_temp="$(required_env_value RUNNER_TEMP)"

echo "[INFO] Release test phase: ${release_test_phase}"
echo "[INFO] Storage type: $(required_env_value CSI)"
echo ""

case "${release_test_phase}" in
  pre-upgrade)
    require_env RELEASE_UPGRADE_CONTEXT_PATH
    echo "[INFO] Current release tag: $(required_env_value CURRENT_RELEASE)"
    echo "[INFO] Verifying virtualization module is running"
    kubectl get modules virtualization
    kubectl get mpo virtualization
    echo ""
    echo "[INFO] Running dedicated release suite"
    echo "[INFO] Resources will be intentionally left in the cluster for the upgrade test"
    ;;
  post-upgrade)
    require_env RELEASE_UPGRADE_STARTED_AT
    echo "[INFO] New release tag: $(required_env_value NEW_RELEASE)"
    echo "[INFO] Verifying virtualization module is running with new release"
    kubectl get modules virtualization || true
    kubectl get mpo virtualization || true
    echo ""
    echo "[INFO] Reusing namespace: $(required_env_value RELEASE_NAMESPACE)"
    ;;
  *)
    echo "[ERROR] Unsupported RELEASE_TEST_PHASE: ${release_test_phase}" >&2
    exit 1
    ;;
esac

# test/e2e is a separate Go module: "go tool ginkgo" and the ./release suite
# both resolve relative to it, so run from there.
echo "[INFO] Changing directory to ./test/e2e/"
cd ./test/e2e/
ginkgo_result="$(mktemp -p "${runner_temp}")"
ginkgo_exit_code=0
go tool ginkgo \
  -v --race --timeout=45m \
  ./release | tee "${ginkgo_result}" || ginkgo_exit_code=$?
echo "[INFO] Exit code: ${ginkgo_exit_code}"

if [ "${release_test_phase}" = "post-upgrade" ]; then
  echo "[INFO] Cluster is intentionally left running (no cleanup)"
fi

exit "${ginkgo_exit_code}"
