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

ceph_user_pool=ceph-rbd-pool-r2
echo "Use user $ceph_user_pool"
echo "Set permissions for user $ceph_user_pool (mgr 'allow *' mon 'allow *' osd 'allow *' mds 'allow *')"
usr=$(kubectl -n d8-operator-ceph exec deployments/rook-ceph-tools -c ceph-tools -- \
    ceph auth get-or-create client.$ceph_user_pool mon 'allow *' mgr 'allow *' osd "allow *")
echo "Get fsid"
fsid=$(kubectl -n d8-operator-ceph exec deployments/rook-ceph-tools -c ceph-tools -- ceph fsid)

userKey="${usr#*key = }"
ceph_monitors_ip=$(kubectl -n d8-operator-ceph get svc | grep mon | awk '{print $3}')
monitors_yaml=$(
  for monitor_ip in $ceph_monitors_ip; do
    echo "    - $monitor_ip:6789"
  done
)

# Verify we have monitors
if [ -z "$monitors_yaml" ]; then
    echo "ERROR: No Ceph monitors found"
    exit 1
fi

echo "Create CephClusterConnection"
kubectl apply -f - <<EOF
apiVersion: storage.deckhouse.io/v1alpha1
kind: CephClusterConnection
metadata:
  name: ceph-cluster
spec:
  clusterID: $fsid
  monitors:
$monitors_yaml
  userID: $ceph_user_pool
  userKey: $userKey
EOF

echo "Configure $ceph_user_pool pool and pg_num=128 pgp_num=128 pg_autoscale_mode=off"
kubectl -n d8-operator-ceph exec deployments/rook-ceph-tools -c ceph-tools -- /bin/bash -c "
    ceph config set global osd_pool_default_size 2
    ceph osd pool set $ceph_user_pool pg_autoscale_mode off
    ceph osd pool set $ceph_user_pool pg_num 128
    ceph osd pool set $ceph_user_pool pgp_num 128
"

echo "Create CephStorageClass"
kubectl apply -f - <<EOF
apiVersion: storage.deckhouse.io/v1alpha1
kind: CephStorageClass
metadata:
  name: nested-ceph-pool-r2-csi-rbd
spec:
  clusterConnectionName: ceph-cluster
  rbd:
    defaultFSType: ext4
    pool: $ceph_user_pool
  reclaimPolicy: Delete
  type: RBD
EOF

echo "Verify StorageClass"
sleep 5
kubectl get sc


DEFAULT_STORAGE_CLASS=nested-ceph-pool-r2-csi-rbd
kubectl patch mc global --type='json' -p='[{"op": "replace", "path": "/spec/settings/defaultClusterStorageClass", "value": "'"$DEFAULT_STORAGE_CLASS"'"}]'

sleep 2
kubectl get sc
