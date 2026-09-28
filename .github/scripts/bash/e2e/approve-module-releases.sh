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

# Some modules ship their latest release behind a manual-approval gate: Deckhouse
# leaves such a ModuleRelease Pending with the message
#   Release is waiting for the 'modules.deckhouse.io/approved: "true"' annotation
# and keeps the module on its previous version until that annotation is set. The
# nightly needs the latest storage-foundation (and its state-snapshotter
# dependency) for storage setup, so approve them once the cluster is up.
#
# Usage: approve-module-releases.sh [module ...]
MODULES=("$@")
if [ "${#MODULES[@]}" -eq 0 ]; then
  MODULES=(state-snapshotter storage-foundation)
fi

APPROVED_ANNOTATION="modules.deckhouse.io/approved=true"

# Waits for a Pending ModuleRelease of the module to appear, then approves every
# Pending release of it. Deckhouse deploys the newest approved release that meets
# its dependency constraints, so approving is idempotent and safe to repeat.
approve_pending_releases() {
  local module="$1"
  local count=30 delay=10 i names name

  for ((i = 1; i <= count; i++)); do
    names="$(kubectl get modulereleases -l "module=${module}" \
      -o jsonpath='{range .items[?(@.status.phase=="Pending")]}{.metadata.name}{" "}{end}' 2>/dev/null || true)"
    if [ -n "${names// }" ]; then
      break
    fi
    echo "[INFO] Wait for a Pending ${module} ModuleRelease ${i}/${count}"
    if [ "$i" -lt "$count" ]; then
      sleep "$delay"
    fi
  done

  if [ -z "${names// }" ]; then
    echo "[INFO] No Pending ${module} ModuleRelease to approve"
    return 0
  fi

  for name in $names; do
    echo "[INFO] Approve ModuleRelease ${name}"
    kubectl annotate modulerelease "${name}" "${APPROVED_ANNOTATION}" --overwrite
  done
}

for module in "${MODULES[@]}"; do
  approve_pending_releases "${module}"
done

# Approving state-snapshotter lets storage-foundation clear its dependency
# constraint; wait for every requested module to reconcile to Ready so storage
# setup does not race the rollout.
for module in "${MODULES[@]}"; do
  echo "[INFO] Wait for module ${module} to be Ready"
  kubectl wait --for=jsonpath='{.status.phase}'=Ready "modules/${module}" --timeout=600s
done

echo "[SUCCESS] Gated module releases approved"
