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
# shellcheck source=.github/scripts/bash/e2e/wait-sds-elastic.sh
source "${SCRIPT_DIR}/wait-sds-elastic.sh"

ELASTIC_CLUSTER_NAME=elastic
ELASTIC_STORAGE_CLASS=nested-ceph-rbd
# StorageClassMigration needs a second RBD storage class as a migration target.
ELASTIC_STORAGE_CLASSES=(nested-ceph-rbd nested-ceph-rbd-r3)

# sds-elastic (Ceph via Rook) is Experimental, so enable it and its dependencies
# (sds-node-configurator, csi-ceph). On the stage profile the modules are absent in
# the stage registry, so pull them from the deckhouse-prod ModuleSource created by
# enable-sdn.sh; otherwise use the default deckhouse source.
apply_module_configs() {
  local source_field="  source: deckhouse"

  if [ -n "${MODULE_SOURCE_REGISTRY_CFG:-}" ]; then
    source_field="  source: deckhouse-prod"
  fi

  kubectl apply -f - <<EOF
---
apiVersion: deckhouse.io/v1alpha1
kind: ModuleConfig
metadata:
  name: sds-node-configurator
spec:
  enabled: true
  version: 1
${source_field}
---
apiVersion: deckhouse.io/v1alpha1
kind: ModuleConfig
metadata:
  name: csi-ceph
spec:
  enabled: true
  version: 1
${source_field}
  settings:
    cephfsEnabled: false
---
apiVersion: deckhouse.io/v1alpha1
kind: ModuleConfig
metadata:
  name: sds-elastic
spec:
  enabled: true
  version: 1
${source_field}
  settings:
    dataNodes:
      nodeSelector:
        node-role.deckhouse.io/worker: ""
EOF
}

echo "[INFO] Allow experimental modules (sds-elastic is Experimental)"
kubectl patch mc deckhouse --type=merge \
  -p '{"spec":{"settings":{"allowExperimentalModules":true}}}'

d8_queue

if [ -n "${MODULE_SOURCE_REGISTRY_CFG:-}" ]; then
  echo "[INFO] Apply sds-elastic ModuleConfigs with deckhouse-prod source (stage profile)"
else
  echo "[INFO] Apply sds-elastic ModuleConfigs with deckhouse source"
fi
apply_module_configs

echo "[INFO] Wait for sds-node-configurator to be ready"
kubectl wait --for=jsonpath='{.status.phase}'=Ready modules sds-node-configurator --timeout=300s

echo "[INFO] Wait for csi-ceph to be ready"
kubectl wait --for=jsonpath='{.status.phase}'=Ready modules csi-ceph --timeout=600s

echo "[INFO] Wait for sds-elastic to be ready"
kubectl wait --for=jsonpath='{.status.phase}'=Ready modules sds-elastic --timeout=600s

echo "[INFO] Wait for raw block devices to be discovered"
elastic_blockdevices_ready

# Label the OSD candidates so the ElasticCluster blockDeviceSelector (app=elastic-osd)
# picks them up. On a rerun some devices are already consumed into OSD LVGs and are no
# longer consumable, so also (re)label those to keep the selector matching them even if
# the label was lost during a BlockDevice rediscovery.
echo "[INFO] Label OSD-candidate block devices as OSDs (app=elastic-osd)"
for bd in $(kubectl get blockdevices.storage.deckhouse.io -o json | jq -r --arg re "$ELASTIC_OSD_LVG_REGEX" ".items[] | select(${ELASTIC_OSD_BD_SELECT}) | .metadata.name"); do
  echo "[INFO] Label blockdevice ${bd} app=elastic-osd"
  kubectl label blockdevice "${bd}" app=elastic-osd --overwrite
done

echo "[INFO] Create ElasticCluster (host networking)"
kubectl apply -f ./elastic-cluster.yaml

echo "[INFO] Wait for ElasticCluster to be ready"
elastic_cluster_ready "${ELASTIC_CLUSTER_NAME}"

echo "[INFO] Create ElasticStorageClasses (RBD: replica-2 default + replica-3 migration target)"
kubectl apply -f ./elastic-storage-class.yaml

for sc in "${ELASTIC_STORAGE_CLASSES[@]}"; do
  echo "[INFO] Wait for StorageClass ${sc} to appear"
  for i in $(seq 1 30); do
    if kubectl get storageclass "${sc}" >/dev/null 2>&1; then
      echo "[SUCCESS] StorageClass ${sc} is present"
      break
    fi
    echo "[INFO] Wait 10s for StorageClass ${sc} (attempt ${i}/30)"
    sleep 10
  done
done

echo "[INFO] Set default cluster storage class to ${ELASTIC_STORAGE_CLASS}"
kubectl patch mc global --type='json' \
  -p='[{"op": "replace", "path": "/spec/settings/defaultClusterStorageClass", "value": "'"${ELASTIC_STORAGE_CLASS}"'"}]'

echo "[INFO] Show existing storageclasses and volumesnapshotclasses"
kubectl get storageclass
kubectl get volumesnapshotclass || echo "[WARNING] No volumesnapshotclasses found"
