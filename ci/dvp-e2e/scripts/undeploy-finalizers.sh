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
# Aggressive namespace cleanup with finalizer removal
# Usage: ./undeploy-finalizers.sh <namespace> [kubeconfig]

set -euo pipefail

NAMESPACE="${1:-}"
KUBECONFIG="${2:-}"

if [ -z "$NAMESPACE" ]; then
    echo "[ERR] Usage: $0 <namespace> [kubeconfig]" >&2
    exit 1
fi

if [ -n "$KUBECONFIG" ]; then
    export KUBECONFIG="$KUBECONFIG"
fi

echo "[UNDEPLOY] Waiting for namespace $NAMESPACE deletion..."

# Wait for namespace deletion with timeout
deleted=false
deadline=$((SECONDS+300))
while (( SECONDS < deadline )); do
  if ! kubectl --request-timeout=8s get "namespace/$NAMESPACE" >/dev/null 2>&1; then
    deleted=true; break
  fi
  phase=$(kubectl --request-timeout=8s get "namespace/$NAMESPACE" -o jsonpath='{.status.phase}' 2>/dev/null || echo "?")
  dt=$(kubectl --request-timeout=8s get "namespace/$NAMESPACE" -o jsonpath='{.metadata.deletionTimestamp}' 2>/dev/null || echo "-")
  echo "[UNDEPLOY] $NAMESPACE phase=$phase deletionTimestamp=$dt"
  sleep 5
done

if ! $deleted; then
  echo "[WARN] Namespace $NAMESPACE stuck terminating. Forcing finalizer removal..."
  # Try to drop finalizers from the most common CRs to unblock deletion
  for kind in \
    virtualmachines.virtualization.deckhouse.io \
    virtualmachineinstances.kubevirt.io \
    virtualdisks.virtualization.deckhouse.io \
    virtualizationimages.virtualization.deckhouse.io \
    internalvirtualizationstorageprofiles.virtualization.deckhouse.io \
    datavolumes.cdi.kubevirt.io \
    volumesnapshots.snapshot.storage.k8s.io \
    nfsstorageclasses.virtualization.deckhouse.io; do
    kubectl -n "$NAMESPACE" get "$kind" -o name 2>/dev/null | while read -r obj; do
      [ -z "$obj" ] && continue
      kubectl -n "$NAMESPACE" patch "$obj" --type json -p='[{"op":"remove","path":"/metadata/finalizers"}]' 2>/dev/null || true
    done
  done
  # Optional generic sweep across all namespaced resources
  while read -r res; do
    kubectl -n "$NAMESPACE" get "$res" -o name 2>/dev/null \
      | xargs -r -n1 -I{} kubectl -n "$NAMESPACE" patch {} --type json -p='[{"op":"remove","path":"/metadata/finalizers"}]' 2>/dev/null || true
  done < <(kubectl api-resources --verbs=list --namespaced -o name 2>/dev/null)
  # Re-try namespace delete (force) just in case
  kubectl delete ns "$NAMESPACE" --wait=false --force --grace-period=0 2>/dev/null || true
  # Drop finalizers from the namespace itself
  kubectl get ns "$NAMESPACE" -o json | jq 'del(.spec.finalizers)|.metadata.finalizers=[]' | kubectl replace --raw "/api/v1/namespaces/$NAMESPACE/finalize" -f - >/dev/null 2>&1 || true
  # Final short polling to confirm deletion
  for i in $(seq 1 12); do
    if ! kubectl --request-timeout=8s get "namespace/$NAMESPACE" >/dev/null 2>&1; then
      echo "[OK] $NAMESPACE removed"; break
    fi
    echo "[UNDEPLOY] $NAMESPACE still terminating ($i/12)"; sleep 5
  done
fi
