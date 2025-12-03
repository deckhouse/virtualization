#!/usr/bin/env bash

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
  topology: Ignored
EOF

kubectl apply -f ${manifest}