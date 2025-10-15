#!/bin/bash
# Copyright 2025 Flant JSC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
# Bootstrap Rook Ceph CRDs/operator in nested cluster
# Usage: ./ceph-bootstrap.sh <kubeconfig>

set -euo pipefail

KUBECONFIG_FILE="${1:-}"

if [ -z "$KUBECONFIG_FILE" ]; then
    echo "[ERR] Usage: $0 <kubeconfig>" >&2
    exit 1
fi

export KUBECONFIG="$KUBECONFIG_FILE"

if ! command -v kubectl >/dev/null 2>&1; then
  echo "[ERR] kubectl not found in PATH"
  exit 1
fi

apply_with_retry() {
  local url="$1"; local tries=6; local delay=12
  for i in $(seq 1 "$tries"); do
    if kubectl --request-timeout=10s apply --server-side --force-conflicts -f "$url"; then
      return 0
    fi
    echo "[WARN] kubectl apply failed (attempt $i/$tries) for $url; sleeping $delay s..." >&2
    sleep "$delay"
  done
  echo "[ERR] kubectl apply failed after $tries attempts for $url" >&2
  return 1
}

disable_ape() {
  kubectl -n d8-admission-policy-engine scale deploy/gatekeeper-webhook --replicas=0 2>/dev/null || true
  kubectl delete validatingwebhookconfiguration admission-policy-engine.deckhouse.io --ignore-not-found 2>/dev/null || true
}

enable_ape() {
  kubectl -n d8-admission-policy-engine scale deploy/gatekeeper-webhook --replicas=1 2>/dev/null || true
}

trap enable_ape EXIT

disable_ape

echo "[CEPH] Applying Rook CRDs"
apply_with_retry https://raw.githubusercontent.com/rook/rook/release-1.14/deploy/examples/crds.yaml
echo "[CEPH] Applying common resources"
apply_with_retry https://raw.githubusercontent.com/rook/rook/release-1.14/deploy/examples/common.yaml
echo "[CEPH] Deploying rook-ceph operator"
apply_with_retry https://raw.githubusercontent.com/rook/rook/release-1.14/deploy/examples/operator.yaml
echo "[CEPH] Waiting for rook-ceph-operator deployment"
kubectl -n rook-ceph wait --for=condition=Available deploy/rook-ceph-operator --timeout=600s
