#!/usr/bin/env bash

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

set -euo pipefail

# Usage:
#   configure_sds_storage.sh -k kubeconfig -s storage_class [-d dvcr_size]

kubeconfig=""
storage_class="linstor-thin-r2"
dvcr_size="5Gi"

while getopts ":k:s:d:" opt; do
  case $opt in
    k) kubeconfig="$OPTARG" ;;
    s) storage_class="$OPTARG" ;;
    d) dvcr_size="$OPTARG" ;;
    *) 
      echo "Usage: $0 -k <kubeconfig> -s <storage_class> [-d <dvcr_size>]" >&2
      exit 2 
      ;;
  esac
done

if [ -z "${kubeconfig}" ] || [ ! -f "${kubeconfig}" ]; then
  echo "Error: kubeconfig is required and must exist" >&2
  exit 2
fi

export KUBECONFIG="${kubeconfig}"

# Step 0: Wait for API server
echo "[SDS] Waiting for API server to be ready..."
for i in $(seq 1 50); do
  if kubectl get nodes >/dev/null 2>&1; then
    echo "[SDS] API server is ready!"
    break
  fi
  echo "[SDS] API server not ready yet, retry $i/50"
  sleep 10
done

# Step 1: Enable sds-node-configurator
echo "[SDS] Step 1: Enabling sds-node-configurator..."
cat <<'EOF' | kubectl apply -f -
apiVersion: deckhouse.io/v1alpha2
kind: ModulePullOverride
metadata:
  name: sds-node-configurator
spec:
  imageTag: main
  scanInterval: 15s
EOF

cat <<'EOF' | kubectl -n d8-system apply -f -
apiVersion: deckhouse.io/v1alpha1
kind: ModuleConfig
metadata:
  name: sds-node-configurator
  namespace: d8-system
spec:
  enabled: true
  version: 1
  settings:
    disableDs: false
    enableThinProvisioning: true
EOF

# Step 2: Wait for sds-node-configurator
echo "[SDS] Step 2: Waiting for sds-node-configurator to be Ready..."
if ! kubectl wait module sds-node-configurator --for=jsonpath='{.status.phase}'=Ready --timeout=600s >/dev/null 2>&1; then
  echo "[WARN] sds-node-configurator did not reach Ready within 10 minutes" >&2
fi

# Step 3: Enable sds-replicated-volume
echo "[SDS] Step 3: Enabling sds-replicated-volume..."
cat <<'EOF' | kubectl apply -f -
apiVersion: deckhouse.io/v1alpha2
kind: ModulePullOverride
metadata:
  name: sds-replicated-volume
spec:
  imageTag: main
  scanInterval: 15s
EOF

cat <<'EOF' | kubectl -n d8-system apply -f -
apiVersion: deckhouse.io/v1alpha1
kind: ModuleConfig
metadata:
  name: sds-replicated-volume
  namespace: d8-system
spec:
  enabled: true
  version: 1
EOF

# Step 4: Wait for sds-replicated-volume
echo "[SDS] Step 4: Waiting for sds-replicated-volume to be Ready..."
if ! kubectl wait module sds-replicated-volume --for=jsonpath='{.status.phase}'=Ready --timeout=600s >/dev/null 2>&1; then
  echo "[WARN] sds-replicated-volume did not reach Ready within 10 minutes" >&2
fi

# Step 6: Create LVMVolumeGroups per node
echo "[SDS] Creating per-node LVMVolumeGroups (type=Local)..."
NODES=$(kubectl get nodes -o json \
  | jq -r '.items[] | select(.metadata.labels["node-role.kubernetes.io/control-plane"]!=true and .metadata.labels["node-role.kubernetes.io/master"]!=true) | .metadata.name')

if [ -z "$NODES" ]; then
  NODES=$(kubectl get nodes -o json | jq -r '.items[].metadata.name')
fi

for node in $NODES; do
  [ -z "$node" ] && continue
  MATCH_EXPR=$(yq eval -n '
    .key = "storage.deckhouse.io/device-path" |
    .operator = "In" |
    .values = ["/dev/sdb","/dev/vdb","/dev/xvdb","/dev/sdc","/dev/vdc","/dev/xvdc","/dev/sdd","/dev/vdd","/dev/xvdd"]
  ')
  NODE="$node" MATCH_EXPR="$MATCH_EXPR" yq eval -n '
    .apiVersion = "storage.deckhouse.io/v1alpha1" |
    .kind = "LVMVolumeGroup" |
    .metadata.name = "data-" + env(NODE) |
    .spec.type = "Local" |
    .spec.local.nodeName = env(NODE) |
    .spec.actualVGNameOnTheNode = "data" |
    .spec.blockDeviceSelector.matchExpressions = [ env(MATCH_EXPR) ]
  ' | kubectl apply -f -
done

# Step 7: Create ReplicatedStoragePool
echo "[SDS] Creating ReplicatedStoragePool 'data' from LVMVolumeGroups..."
LVGS=$(printf "%s\n" $NODES | sed 's/^/      - name: data-/')

cat <<EOF | kubectl apply -f -
apiVersion: storage.deckhouse.io/v1alpha1
kind: ReplicatedStoragePool
metadata:
  name: data
spec:
  type: LVM
  lvmVolumeGroups:
$LVGS
EOF

# Step 8: Create ReplicatedStorageClass
echo "[SDS] Creating ReplicatedStorageClass '${storage_class}' (r2, thin)..."
cat <<EOF | kubectl apply -f -
apiVersion: storage.deckhouse.io/v1alpha1
kind: ReplicatedStorageClass
metadata:
  name: ${storage_class}
spec:
  storagePool: data
  reclaimPolicy: Delete
  topology: Ignored
  volumeAccess: Local
EOF

# Step 9: Ensure StorageClass exists
if ! kubectl get sc "${storage_class}" >/dev/null 2>&1; then
  echo "[ERR] StorageClass '${storage_class}' not found in nested cluster" >&2
  exit 1
fi

# Step 10: Set default StorageClass
echo "[SDS] Setting '${storage_class}' as default StorageClass via ModuleConfig global..."
PATCH=$(jq -cn --arg v "${storage_class}" '[{"op":"replace","path":"/spec/settings/defaultClusterStorageClass","value":$v}]')
kubectl patch mc global --type='json' -p="$PATCH"

echo "[SDS] SDS storage configuration complete!"
