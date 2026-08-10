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

# Collects firing ClusterAlerts of the virtualization module from the cluster
# the current kubeconfig points at, until the stop marker appears in that same
# cluster (see signal-clusteralerts-watch-stop.sh) or the timeout is reached.
#
# The marker lives in the cluster rather than on disk on purpose: the watch runs
# on its own runner, and runners share no filesystem.

set -Eeuo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=.github/scripts/bash/e2e/common.sh
source "${SCRIPT_DIR}/common.sh"

require_env CLUSTERALERTS_LOG

alerts_log="${CLUSTERALERTS_LOG:-}"
alert_prefix="${CLUSTERALERTS_PREFIX:-D8Virtualization}"
poll_interval="${POLL_INTERVAL:-15}"
# Keep this below the job timeout, otherwise the job is cancelled before the
# collected alerts can be uploaded.
timeout_seconds="${TIMEOUT_SECONDS:-17400}"
stop_namespace="${WATCH_STOP_NAMESPACE:-default}"
stop_configmap="${WATCH_STOP_CONFIGMAP:-e2e-clusteralerts-watch-stop}"

mkdir -p "$(dirname -- "${alerts_log}")"
: > "${alerts_log}"

deadline=$(( $(date +%s) + timeout_seconds ))

# A marker left over from a previous run against the same cluster (a re-run of
# failed jobs, for example) would stop the watch immediately.
kubectl -n "${stop_namespace}" delete configmap "${stop_configmap}" --ignore-not-found

echo "[INFO] Watching ClusterAlerts matching '${alert_prefix}*'"
echo "[INFO] Poll interval: ${poll_interval}s, watch timeout: ${timeout_seconds}s, log: ${alerts_log}"
echo "[INFO] Stop marker: configmap ${stop_configmap} in namespace ${stop_namespace}"

# A ClusterAlert object exists only while the alert is firing, so the log
# accumulates one line per poll per firing alert. Deduplication and the split
# into pipeline phases happen in report-clusteralerts.sh.
while [ "$(date +%s)" -lt "${deadline}" ]; do
  if kubectl -n "${stop_namespace}" get configmap "${stop_configmap}" >/dev/null 2>&1; then
    echo "[INFO] Stop marker found, ending the watch"
    exit 0
  fi

  # The nested API server can blink while the module is being rolled over,
  # so a failed poll must never end the watch.
  if ! snapshot="$(kubectl get clusteralerts -o json 2>&1)"; then
    echo "[WARN] Failed to read ClusterAlerts, retrying in ${poll_interval}s: ${snapshot}"
    sleep "${poll_interval}"
    continue
  fi

  printf '%s' "${snapshot}" | jq -c \
    --arg prefix "${alert_prefix}" \
    --argjson observedAt "$(date +%s)" \
    '.items[]
     | select((.alert.name // "") | startswith($prefix))
     | {
         observedAt: $observedAt,
         name: .alert.name,
         severityLevel: (.alert.severityLevel // ""),
         summary: (.alert.summary // ""),
         description: (.alert.description // ""),
         labels: (.alert.labels // {}),
         id: .metadata.name,
         firstSeen: (.status.startsAt // .metadata.creationTimestamp // "")
       }' >> "${alerts_log}" \
    || echo "[WARN] Failed to parse the ClusterAlerts snapshot, skipping this poll"

  sleep "${poll_interval}"
done

echo "[WARN] Watch timeout of ${timeout_seconds}s reached before the stop marker appeared"
