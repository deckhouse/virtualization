#!/usr/bin/env bash

kubectl delete validatingwebhookconfigurations.admissionregistration.k8s.io d8-csi-ceph-sc-validation
kubectl apply -f - <<EOF
  allowVolumeExpansion: true
  apiVersion: storage.k8s.io/v1
  kind: StorageClass
  metadata:
    annotations:
      storage.deckhouse.io/volumesnapshotclass: ceph-pool-r2-csi-rbd
    labels:
      storage.deckhouse.io/managed-by: d8-ceph-storage-class-controller
      storage.deckhouse.io/migratedFromCephClusterAuthentication: "true"
    name: ceph-pool-r2-csi-rbd-layering
  mountOptions:
  - discard
  parameters:
    clusterID: 60b9231e-edfd-45d5-820f-dd2508066085
    csi.storage.k8s.io/controller-expand-secret-name: csi-ceph-secret-for-ceph-cluster-1
    csi.storage.k8s.io/controller-expand-secret-namespace: d8-csi-ceph
    csi.storage.k8s.io/fstype: ext4
    csi.storage.k8s.io/node-stage-secret-name: csi-ceph-secret-for-ceph-cluster-1
    csi.storage.k8s.io/node-stage-secret-namespace: d8-csi-ceph
    csi.storage.k8s.io/provisioner-secret-name: csi-ceph-secret-for-ceph-cluster-1
    csi.storage.k8s.io/provisioner-secret-namespace: d8-csi-ceph
    imageFeatures: layering
    pool: ceph-rbd-pool-r2
  provisioner: rbd.csi.ceph.com
  reclaimPolicy: Delete
  volumeBindingMode: Immediate
EOF

sleep 3
kubectl get storageclass