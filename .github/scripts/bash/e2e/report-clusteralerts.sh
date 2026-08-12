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

require_env CLUSTERALERTS_DIR

alerts_dir="${CLUSTERALERTS_DIR:-}"
alert_prefix="${CLUSTERALERTS_PREFIX:-D8Virtualization}"
collect_result="${COLLECT_RESULT:-}"
summary_file="${GITHUB_STEP_SUMMARY:-/dev/stdout}"

{
  echo "## ClusterAlerts in the nested cluster"
  echo
} >> "${summary_file}"

# Renders the "we could not check" verdict: an empty report is only good news
# when the collection itself succeeded.
not_collected() {
  local reason="$1"

  echo "${reason}" >> "${summary_file}"
  echo "::warning title=ClusterAlerts were not collected::${reason}"
  exit 0
}

shopt -s nullglob
logs=("${alerts_dir}"/*.jsonl)
shopt -u nullglob

if [ -n "${collect_result}" ] && [ "${collect_result}" != "success" ]; then
  not_collected "The collection step did not complete (result: \`${collect_result}\`), so alerts were **not** checked."
fi

# collect-clusteralerts.sh always writes its log, empty or not, so a missing one
# means the collection never got that far - a step that never ran reports no
# result at all.
if [ "${#logs[@]}" -eq 0 ]; then
  echo "[WARN] No collected ClusterAlerts found in ${alerts_dir}"
  not_collected "No collected alerts were found, so alerts were **not** checked."
fi

echo "[INFO] Reading collected ClusterAlerts from: ${logs[*]}"
# The phase and the firing interval come from the collector; sorting is repeated
# here only to keep the order stable across several log files.
alerts="$(jq -s 'sort_by([.order, .name])' "${logs[@]}")"

count="$(jq 'length' <<< "${alerts}")"

# Alert summaries are markdown ending with a newline; flatten them for the
# table and for annotations.
oneline='def oneline: gsub("\\s+"; " ") | sub("^ "; "") | sub(" $"; "");'

if [ "${count}" -eq 0 ]; then
  echo "No \`${alert_prefix}*\` alert fired during the release rollover." >> "${summary_file}"
  echo "[INFO] No ${alert_prefix}* alert fired during the release rollover"
  exit 0
fi

{
  echo "| Phase | Alert | Severity | First seen | Summary |"
  echo "|---|---|---|---|---|"
  jq -r "${oneline}"' .[] | "| \(.phase) | \(.name) | \(.severityLevel) | \(.firstSeen) | \(.summary | oneline | gsub("\\|"; "\\|")) |"' <<< "${alerts}"
  echo
  echo "<details><summary>Alert details</summary>"
  echo
  jq -r '.[] | "#### \(.name) — \(.phase)\n\n- severity level: \(.severityLevel)\n- active: \(.firstSeen) .. \(.lastSeen)\n- labels: `\(.labels | tojson)`\n\n\(.description)\n"' <<< "${alerts}"
  echo "</details>"
} >> "${summary_file}"

# Annotations put the alerts on top of the run page, not only in the summary.
jq -r "${oneline}"' .[]
  | "::warning title=ClusterAlert \(.name)::[\(.phase)] \(.summary | oneline)"' <<< "${alerts}"

echo "[INFO] Fired alerts:"
jq -r '.[] | "  [\(.phase)] \(.name) (severity \(.severityLevel))"' <<< "${alerts}"

# Failing here is what paints this job red; the job itself is
# continue-on-error, so the workflow conclusion stays successful. Only firing
# alerts reach this script, so any of them is worth the red.
echo "[ERROR] ${count} ClusterAlert(s) fired in the nested cluster, see the job summary" >&2
exit 1
