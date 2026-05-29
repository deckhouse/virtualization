#!/usr/bin/env bash

set -Eeuo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=.github/scripts/bash/e2e/common.sh
source "${SCRIPT_DIR}/common.sh"

manifest="$(mktemp)"
trap 'rm -f "$manifest"' EXIT

cat > "$manifest"

count=12
delay=10

for i in $(seq 1 "$count"); do
  echo "[INFO] Apply ClusterNetworks attempt ${i}/${count}"
  if kubectl apply -f "$manifest"; then
    exit 0
  fi

  if [ "$i" -lt "$count" ]; then
    echo "[WARN] Failed to apply ClusterNetworks, retrying in ${delay} seconds..."
    kubectl -n d8-sdn get endpoints controller-sdn-admission || true
    kubectl get clusternetworks.network.deckhouse.io || true
    sleep "$delay"
  fi
done

echo "[ERROR] Failed to apply ClusterNetworks after ${count} attempts"
kubectl -n d8-sdn get pods,svc,endpoints || true
kubectl get clusternetworks.network.deckhouse.io || true
exit 1
