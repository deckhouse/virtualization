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

# Sets the virtualization feature gates of the module config to those every
# given release accepts and the live cluster admits, and verifies the result.
#
# Usage: DEV_MODULE_SOURCE=<repo> patch-virtualization-feature-gates.sh <release>...
#
# During a release upgrade this runs twice. Before the image tag is patched it
# is called with both releases, which drops the gates the new release does not
# know - otherwise the new module fails validation and never installs. After the
# upgrade it is called with the new release alone, which enables the gates only
# that release supports.
#
# A gate the cluster refuses - locked in this edition, or needing a newer
# Kubernetes - is dropped with a warning instead of failing the run: the point of
# the release e2e is the upgrade, not the gate.

set -Eeuo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=.github/scripts/bash/e2e/common.sh
source "${SCRIPT_DIR}/common.sh"
# Sourced for virtualization_ready and virt_handler_ready, used after the patch.
# shellcheck source=.github/scripts/bash/e2e/wait-virtualization-ready.sh
source "${SCRIPT_DIR}/wait-virtualization-ready.sh"

if [ "$#" -eq 0 ]; then
  echo "[ERROR] Usage: $(basename -- "${BASH_SOURCE[0]}") <release>..." >&2
  exit 1
fi

require_env DEV_MODULE_SOURCE
# shellcheck disable=SC2153,SC2154 # set in the workflow, checked by require_env above
dev_module_source="${DEV_MODULE_SOURCE}"
summary_file="${GITHUB_STEP_SUMMARY:-/dev/stdout}"

wanted="$(virtualization_feature_gates "${dev_module_source}" "$@")"
echo "[INFO] Releases: $*"
echo "[INFO] Feature gates they all accept: ${wanted//$'\n'/ }"

# The webhook validating the gates is served by virtualization-controller itself,
# so a rejection right after an image switch says nothing about the gate. Get an
# answer out of the webhook first, and treat silence as the defect it is.
if ! moduleconfig_writable; then
  echo "[ERROR] The module config webhook never answered, feature gates cannot be validated" >&2
  exit 1
fi

accepted=""
dropped_rows=""
while IFS= read -r gate; do
  [ -n "${gate}" ] || continue

  candidate_json="$(printf '%s\n%s\n' "${accepted}" "${gate}" | jq -Rsc 'split("\n") | map(select(length > 0))')"
  if error="$(gate_accepted "${candidate_json}")"; then
    accepted="$(printf '%s\n%s' "${accepted}" "${gate}")"
    continue
  fi

  error="$(tr '\n' ' ' <<< "${error}")"
  echo "[WARN] Feature gate ${gate} is not admitted by the cluster, dropping it: ${error}"
  echo "::warning title=Feature gate ${gate} was dropped::${error}"
  dropped_rows="${dropped_rows}- \`${gate}\`: ${error}"$'\n'
done <<< "${wanted}"

if [ -n "${dropped_rows}" ]; then
  {
    # The release list tells the two runs of this script apart: both write into
    # the summary of the same job.
    echo "## Feature gates dropped from the module config ($*)"
    echo
    printf '%s' "${dropped_rows}"
    echo
  } >> "${summary_file}"
fi

gates_json="$(jq -Rsc 'split("\n") | map(select(length > 0))' <<< "${accepted}")"
current_json="$(kubectl get mc virtualization -o jsonpath='{.spec.settings.featureGates}')"

echo "[INFO] Feature gates to apply: ${gates_json}"

if [ "${current_json}" = "${gates_json}" ]; then
  echo "[INFO] Module config already lists exactly these gates, nothing to patch"
  exit 0
fi

echo "[INFO] Patching feature gates: ${current_json:-none} -> ${gates_json}"
kubectl patch mc virtualization --type merge -p "{\"spec\":{\"settings\":{\"featureGates\":${gates_json}}}}"

patched_json="$(kubectl get mc virtualization -o jsonpath='{.spec.settings.featureGates}')"
if [ "${patched_json}" != "${gates_json}" ]; then
  echo "[ERROR] Feature gates were not applied: expected ${gates_json}, got ${patched_json:-none}" >&2
  exit 1
fi

echo "[INFO] Feature gates in effect: ${patched_json}"

# A new gate set re-renders the module - the GPU gate, for one, adds GPUsWithDRA
# to the KubeVirt CR - which restarts the virtualization components, and the next
# job must not start against them mid-restart. Known limit: the re-render is only
# requested here, so it may not have begun by the time this wait starts, and then
# the wait passes on the state the restart is about to leave.
echo "[INFO] Waiting for the module to settle after the feature gate patch"
virtualization_ready
virt_handler_ready
