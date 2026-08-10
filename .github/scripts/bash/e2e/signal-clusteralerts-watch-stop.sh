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

# Tells the ClusterAlerts watch that the pipeline is over. The marker is a
# ConfigMap in the nested cluster because the watch runs on its own runner and
# runners share no filesystem.

set -Eeuo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=.github/scripts/bash/e2e/common.sh
source "${SCRIPT_DIR}/common.sh"

stop_namespace="${WATCH_STOP_NAMESPACE:-default}"
stop_configmap="${WATCH_STOP_CONFIGMAP:-e2e-clusteralerts-watch-stop}"

echo "[INFO] Creating stop marker: configmap ${stop_configmap} in namespace ${stop_namespace}"

# Never fail the pipeline over the marker: if it cannot be created, the watch
# ends on its own timeout instead.
if kubectl -n "${stop_namespace}" create configmap "${stop_configmap}" \
  --from-literal=run_id="${GITHUB_RUN_ID:-unknown}"; then
  echo "[INFO] Stop marker created"
else
  echo "[WARN] Failed to create the stop marker, the watch will end on its own timeout"
fi
