#!/usr/bin/env bash

# Copyright 2025 Flant JSC
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

manifest=sds-rsp-rsc.yaml
replicatedStoragePoolName=thin-data

pools=$(kubectl get lvmvolumegroup -o json | jq '.items[] | {name: .metadata.name, thinPoolName: .spec.thinPools[0].name}' -rc)

cat << EOF > "${manifest}"
---
apiVersion: storage.deckhouse.io/v1alpha1
kind: ReplicatedStoragePool
metadata:
  name: $replicatedStoragePoolName
spec:
  type: LVMThin
  lvmVolumeGroups:
EOF

for pool in ${pools}; do
  vg_name=$(echo $pool | jq -r '.name');
  pool_node=$(echo $pool | jq -r '.thinPoolName');
  echo "${pool_node} ${vg_name}"
cat << EOF >> "${manifest}"
    - name: ${vg_name}
      thinPoolName: ${pool_node}
EOF
done

cat << EOF >> "${manifest}"
---
apiVersion: storage.deckhouse.io/v1alpha1
kind: ReplicatedStorageClass
metadata:
  name: nested-thin-r2
spec:
  replication: Availability
  storagePool: $replicatedStoragePoolName
  reclaimPolicy: Delete
  volumeAccess: PreferablyLocal
  topology: Ignored
---
apiVersion: storage.deckhouse.io/v1alpha1
kind: ReplicatedStorageClass
metadata:
  name: nested-thin-r1
spec:
  replication: None
  storagePool: $replicatedStoragePoolName
  reclaimPolicy: Delete
  volumeAccess: PreferablyLocal
  topology: Ignored
---
apiVersion: storage.deckhouse.io/v1alpha1
kind: ReplicatedStorageClass
metadata:
  name: nested-thin-r1-immediate
spec:
  replication: None
  storagePool: $replicatedStoragePoolName
  reclaimPolicy: Delete
  volumeAccess: Any
  topology: Ignored
EOF

kubectl apply -f ${manifest}

DEFAULT_STORAGE_CLASS=nested-thin-r1
kubectl patch mc global --type='json' -p='[{"op": "replace", "path": "/spec/settings/defaultClusterStorageClass", "value": "'"$DEFAULT_STORAGE_CLASS"'"}]'

sleep 2
echo "Showing Storage Classes"
kubectl get storageclass
echo "  "
