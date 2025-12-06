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

manifest=sds-lvg.yaml
LVMVG_SIZE=45Gi

devs=$(kubectl get blockdevices.storage.deckhouse.io -o json | jq '.items[] | {name: .metadata.name, node: .status.nodeName, dev_path: .status.path}' -rc)

rm -rf "${manifest}"

echo detected block devices: "$devs"

for line in ${devs}; do
  dev_name=$(echo $line | jq -r '.name');
  dev_node=$(echo $line | jq -r '.node');
  node_name=$(echo $dev_node | grep -o 'worker.*');
  dev_path=$(echo $line | jq -r '.dev_path' | cut -d "/" -f3);
  echo "${dev_node} ${dev_name}"
cat << EOF >> "${manifest}"
---
apiVersion: storage.deckhouse.io/v1alpha1
kind: LVMVolumeGroup
metadata:
  name: vg-data-${node_name}-${dev_path}
spec:
  actualVGNameOnTheNode: vg-thin-data
  type: Local
  local:
    nodeName: ${dev_node}
  blockDeviceSelector:
    matchExpressions:
    - key: kubernetes.io/metadata.name
      operator: In
      values:
      - ${dev_name}
  thinPools:
  - name: thin-data
    size: ${LVMVG_SIZE}
    allocationLimit: 100%
EOF

done

kubectl apply -f "${manifest}"
