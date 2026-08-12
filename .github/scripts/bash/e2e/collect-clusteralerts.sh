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

# Collects the alerts of the virtualization module that fired in the nested
# cluster during the release rollover. Runs once, at the end of the pipeline,
# against the nested kubeconfig.
#
# Prometheus is the source rather than the ClusterAlerts objects: a ClusterAlert
# exists only while its alert fires, so a single late look at the API server
# would see nothing of what happened during the upgrade, while the ALERTS series
# keep the whole history of the observation window.
#
# Only alertstate="firing" is collected, which is the same set the ClusterAlerts
# of a cluster hold. Pending is deliberately left out: components restart during
# a rollover, so rules with a `for` clause go pending on almost every run, and
# reporting those buries the alerts that actually fired.

set -Eeuo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=.github/scripts/bash/e2e/common.sh
source "${SCRIPT_DIR}/common.sh"

require_env CLUSTERALERTS_DIR

alerts_dir="${CLUSTERALERTS_DIR:-}"
alert_prefix="${CLUSTERALERTS_PREFIX:-D8Virtualization}"
prometheus_namespace="d8-monitoring"
prometheus_selector="prometheus=main"
# Port of the prometheus container inside the pod, discovered below.
# port-forward joins the pod network namespace, so a listener bound to
# localhost there is reachable too.
prometheus_port=""
local_port="19090"
# The ALERTS series exists only while its alert is active, so an alert shorter
# than the step can fall between two samples and be missed entirely. Prometheus
# allows 11000 points per series, and a rollover window is under half an hour, so
# a step this small is nowhere near the limit.
query_step="10"
ready_attempts=30
ready_delay=2

# The window starts when virtualization was configured, not when the pipeline
# started: before that the module does not exist, and its alerts cannot either.
window_start="${CLUSTERALERTS_WINDOW_STARTED_AT:-}"
window_end="$(date +%s)"

# Same semantics as in report-clusteralerts.sh: a zero means the corresponding
# job never reported a timestamp.
started="${RELEASE_UPGRADE_STARTED_AT:-0}"
finished="${RELEASE_UPGRADE_FINISHED_AT:-0}"
[[ "${started}" =~ ^[0-9]+$ ]] || started=0
[[ "${finished}" =~ ^[0-9]+$ ]] || finished=0

if [[ ! "${window_start}" =~ ^[0-9]+$ ]] || [ "${window_start}" -eq 0 ]; then
  echo "[ERROR] CLUSTERALERTS_WINDOW_STARTED_AT must be a unix timestamp, got: '${window_start}'" >&2
  exit 1
fi

range_file="${alerts_dir}/clusteralerts-query-range.json"
rules_file="${alerts_dir}/clusteralerts-rules.json"
templates_file="${alerts_dir}/clusteralerts-templates.json"
alerts_log="${alerts_dir}/clusteralerts.jsonl"
port_forward_log="${alerts_dir}/clusteralerts-port-forward.log"

port_forward_pid=""

cleanup() {
  [ -n "${port_forward_pid}" ] || return 0
  kill "${port_forward_pid}" 2>/dev/null || true
  wait "${port_forward_pid}" 2>/dev/null || true
  port_forward_pid=""
}

# --fail-with-body and not -f: Prometheus reports a rejected query with an HTTP
# error code and puts the reason in the body, which -f would throw away along
# with the only explanation of what went wrong.
prom_api() {
  local path="$1"
  shift
  curl -sS --fail-with-body --max-time 120 -G "http://127.0.0.1:${local_port}${path}" "$@"
}

# port-forward plus curl on the runner, not kubectl exec plus curl in the
# container: the Prometheus image ships no shell tools to query itself with.
start_port_forward() {
  local pod="$1" attempt

  echo "[INFO] Forwarding 127.0.0.1:${local_port} to ${pod}:${prometheus_port} in ${prometheus_namespace}"
  kubectl -n "${prometheus_namespace}" port-forward \
    "pod/${pod}" "${local_port}:${prometheus_port}" > "${port_forward_log}" 2>&1 &
  port_forward_pid=$!
  trap cleanup EXIT

  # The tunnel needs a moment, and probing the query API instead of /-/ready
  # checks exactly what the collection is about to use.
  for ((attempt = 1; attempt <= ready_attempts; attempt++)); do
    if prom_api /api/v1/query --data-urlencode 'query=1' > /dev/null 2>&1; then
      echo "[INFO] Prometheus API is reachable after ${attempt} attempt(s)"
      return 0
    fi

    if ! kill -0 "${port_forward_pid}" 2>/dev/null; then
      echo "[ERROR] kubectl port-forward exited early:" >&2
      cat "${port_forward_log}" >&2 || true
      return 1
    fi

    sleep "${ready_delay}"
  done

  echo "[ERROR] Prometheus API did not become reachable after ${ready_attempts} attempt(s):" >&2
  cat "${port_forward_log}" >&2 || true
  return 1
}

# Annotation templates live in the rules, not in the series, so they are fetched
# separately and rendered per series below.
fetch_rules() {
  if prom_api /api/v1/rules --data-urlencode 'type=alert' > "${rules_file}" &&
    [ "$(jq -r '.status // ""' "${rules_file}")" = "success" ]; then
    return 0
  fi

  # A missing summary must never sink the report: the alert itself is the news.
  echo "[WARN] Failed to fetch alerting rules, alerts will be reported without a summary"
  printf '%s\n' '{"status":"error","data":{"groups":[]}}' > "${rules_file}"
}

templates_program='
[ .data.groups[]?.rules[]?
  | select(.type == "alerting")
  | { key: .name,
      value: {
        summary: (.annotations.summary // ""),
        description: (.annotations.description // "")
      }
    }
] | from_entries
'

# shellcheck disable=SC2016 # $started, $finished and $templates_wrapper are jq variables, passed in on the command line
records_program='
($templates_wrapper[0] // {}) as $templates
|
def phase_of($t):
  if $started == 0 or $t < $started then { phase: "pre-upgrade", order: 0 }
  elif $finished == 0 or $t < $finished then { phase: "upgrade", order: 1 }
  else { phase: "post-upgrade", order: 2 }
  end;

# The annotations are Go templates. Every $labels.X reference is substituted by
# the value the series carries for that label, and whatever template action is
# left afterwards becomes a "?" placeholder: notably $value, the sample value the
# ALERTS series does not carry, and references to labels absent from the series.
def render($labels):
  reduce ($labels | to_entries[]) as $l
    (.; gsub("\\{\\{\\s*\\$labels\\." + $l.key + "\\s*\\}\\}"; $l.value))
  | gsub("\\{\\{[^{}]*\\}\\}"; "?");

[ .data.result[]
  | . as $series
  | ($series.metric.alertname // "") as $name
  | ($series.metric | del(.__name__, .alertname, .alertstate)) as $labels
  # One record per phase the series touches: an alert that spans the upgrade is
  # news in every phase it was active in, and grouping the samples by phase
  # intersects its active interval with the upgrade window.
  | [ $series.values[] | .[0] | floor | { t: ., ph: phase_of(.) } ]
  | group_by(.ph.order)
  | .[]
  | { phase: .[0].ph.phase,
      order: .[0].ph.order,
      name: $name,
      severityLevel: ($series.metric.severity_level // ""),
      labels: $labels,
      firstSeen: (map(.t) | min | todate),
      lastSeen: (map(.t) | max | todate),
      summary: (($templates[$name].summary // "") | render($labels)),
      description: (($templates[$name].description // "") | render($labels)),
      id: ([$name, ($labels | tojson)] | join("|"))
    }
]
| unique_by([.order, .id])
| sort_by([.order, .name])
| .[]
'

mkdir -p "${alerts_dir}"

echo "[INFO] Collecting firing alerts matching '${alert_prefix}*' from Prometheus in ${prometheus_namespace}"
echo "[INFO] Observation window: ${window_start}..${window_end} ($(( window_end - window_start ))s), step ${query_step}s"
echo "[INFO] Upgrade window: started_at=${started}, finished_at=${finished}"

# "|| true" so an empty item list, on which the jsonpath itself fails, reaches the
# explicit error below instead of aborting on the jsonpath error.
pod="$(kubectl -n "${prometheus_namespace}" get pod \
  -l "${prometheus_selector}" \
  --field-selector=status.phase=Running \
  -o jsonpath='{.items[0].metadata.name}' || true)"

if [ -z "${pod}" ]; then
  echo "[ERROR] No Running pod matching '${prometheus_selector}' in namespace ${prometheus_namespace}" >&2
  exit 1
fi

# Asking the pod which port carries the API beats assuming one: an authenticating
# sidecar may well be the container that owns 9090 there.
prometheus_port="$(kubectl -n "${prometheus_namespace}" get pod "${pod}" \
  -o jsonpath='{.spec.containers[?(@.name=="prometheus")].ports[?(@.name=="web")].containerPort}' || true)"
prometheus_port="${prometheus_port:-9090}"

start_port_forward "${pod}"

# query_range and not query: an instant query would only see what is still
# active now, while the report is about what was active during the rollover.
if ! prom_api /api/v1/query_range \
  --data-urlencode "query=ALERTS{alertname=~\"${alert_prefix}.*\",alertstate=\"firing\"}" \
  --data-urlencode "start=${window_start}" \
  --data-urlencode "end=${window_end}" \
  --data-urlencode "step=${query_step}" > "${range_file}"; then
  echo "[ERROR] Prometheus rejected the range query 'ALERTS{alertname=~\"${alert_prefix}.*\",alertstate=\"firing\"}' over ${window_start}..${window_end} with step ${query_step}s" >&2
  echo "[ERROR] Response: $(jq -rc '.error // .' "${range_file}" 2>/dev/null || head -c 500 "${range_file}")" >&2
  exit 1
fi

fetch_rules

# Through a file rather than --argjson: a cluster carries hundreds of alerting
# rules, and their annotations do not fit in the 128 KiB Linux allows a single
# command line argument.
jq "${templates_program}" "${rules_file}" > "${templates_file}"

jq -c \
  --argjson started "${started}" \
  --argjson finished "${finished}" \
  --slurpfile templates_wrapper "${templates_file}" \
  "${records_program}" "${range_file}" > "${alerts_log}"

count="$(grep -c . "${alerts_log}" || true)"
echo "[INFO] Collected ${count} alert record(s) into ${alerts_log}"
jq -r '"  [\(.phase)] \(.name) (\(.firstSeen) .. \(.lastSeen))"' "${alerts_log}"
