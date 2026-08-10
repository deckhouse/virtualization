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
fail_on_alerts="${FAIL_ON_ALERTS:-true}"
watch_result="${WATCH_RESULT:-}"
summary_file="${GITHUB_STEP_SUMMARY:-/dev/stdout}"

# The watch runs as a single job and does not know which pipeline phase it is
# observing, so the phase is derived here from the upgrade timestamps. A zero
# means the corresponding job never reported one.
started="${RELEASE_UPGRADE_STARTED_AT:-0}"
finished="${RELEASE_UPGRADE_FINISHED_AT:-0}"
[[ "${started}" =~ ^[0-9]+$ ]] || started=0
[[ "${finished}" =~ ^[0-9]+$ ]] || finished=0

# shellcheck disable=SC2016 # $started and $finished are jq variables, passed in via --argjson
phase_program='
def phase_of(upgrade_started; upgrade_finished):
  if upgrade_started == 0 or .observedAt < upgrade_started
  then { phase: "pre-upgrade", order: 0 }
  elif upgrade_finished == 0 or .observedAt < upgrade_finished
  then { phase: "upgrade", order: 1 }
  else { phase: "post-upgrade", order: 2 }
  end;
map(. + phase_of($started; $finished))
| unique_by([.phase, .name, .id])
| sort_by([.order, .name])
'

shopt -s nullglob
logs=("${alerts_dir}"/*.jsonl)
shopt -u nullglob

if [ "${#logs[@]}" -eq 0 ]; then
  echo "[WARN] No ClusterAlerts logs found in ${alerts_dir}"
  alerts='[]'
else
  echo "[INFO] Reading collected ClusterAlerts from: ${logs[*]}"
  echo "[INFO] Upgrade window: started_at=${started}, finished_at=${finished}"
  alerts="$(jq -s \
    --argjson started "${started}" \
    --argjson finished "${finished}" \
    "${phase_program}" "${logs[@]}")"
fi

count="$(jq 'length' <<< "${alerts}")"

# Alert summaries are markdown ending with a newline; flatten them for the
# table and for annotations.
oneline='def oneline: gsub("\\s+"; " ") | sub("^ "; "") | sub(" $"; "");'

{
  echo "## ClusterAlerts in the nested cluster"
  echo
} >> "${summary_file}"

if [ "${count}" -eq 0 ]; then
  # An empty report means "nothing was firing" only if the watch actually ran.
  if [ -n "${watch_result}" ] && [ "${watch_result}" != "success" ]; then
    echo "The watch job did not complete (result: \`${watch_result}\`), so alerts were **not** monitored." >> "${summary_file}"
    echo "::warning title=ClusterAlerts were not monitored::The watch job result is '${watch_result}'"
    exit 0
  fi

  echo "No \`${alert_prefix}*\` alerts were firing during the release rollover." >> "${summary_file}"
  echo "[INFO] No ${alert_prefix}* alerts were firing during the release rollover"
  exit 0
fi

{
  echo "| Phase | Alert | Severity | First seen | Summary |"
  echo "|---|---|---|---|---|"
  jq -r "${oneline}"' .[] | "| \(.phase) | \(.name) | \(.severityLevel) | \(.firstSeen) | \(.summary | oneline | gsub("\\|"; "\\|")) |"' <<< "${alerts}"
  echo
  echo "<details><summary>Alert details</summary>"
  echo
  jq -r '.[] | "#### \(.name) — \(.phase)\n\n- severity level: \(.severityLevel)\n- labels: `\(.labels | tojson)`\n\n\(.description)\n"' <<< "${alerts}"
  echo "</details>"
} >> "${summary_file}"

# Annotations put the alerts on top of the run page, not only in the summary.
jq -r "${oneline}"' .[] | "::warning title=ClusterAlert \(.name)::[\(.phase)] \(.summary | oneline)"' <<< "${alerts}"

echo "[INFO] Firing alerts:"
jq -r '.[] | "  [\(.phase)] \(.name) (severity \(.severityLevel))"' <<< "${alerts}"

if [ "${fail_on_alerts}" != "true" ]; then
  echo "[INFO] FAIL_ON_ALERTS is not 'true', not failing the job"
  exit 0
fi

# Failing here is what paints this job red; the job itself is
# continue-on-error, so the workflow conclusion stays successful.
echo "[ERROR] ${count} ClusterAlert(s) were firing in the nested cluster, see the job summary" >&2
trap - ERR
exit 1
