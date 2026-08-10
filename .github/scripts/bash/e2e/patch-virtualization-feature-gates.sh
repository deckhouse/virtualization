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

# Sets the virtualization feature gates of the module config to those supported
# by every given release, and verifies the result.
#
# Usage: patch-virtualization-feature-gates.sh <release>...
#
# During a release upgrade this runs twice. Before the image tag is patched it
# is called with both releases, which drops the gates the new release does not
# know - otherwise the new module fails validation and never installs. After the
# upgrade it is called with the new release alone, which enables the gates only
# that release supports.

set -Eeuo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=.github/scripts/bash/e2e/common.sh
source "${SCRIPT_DIR}/common.sh"

if [ "$#" -eq 0 ]; then
  echo "[ERROR] Usage: $(basename -- "${BASH_SOURCE[0]}") <release>..." >&2
  exit 1
fi

gates_json="$(virtualization_feature_gates "$@" | jq -Rsc 'split("\n") | map(select(length > 0))')"
current_json="$(kubectl get mc virtualization -o jsonpath='{.spec.settings.featureGates}')"

echo "[INFO] Feature gates supported by $*: ${gates_json}"

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
