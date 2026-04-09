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

set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
lvg_generator_script="${script_dir}/../sds-node-configurator/lvg-gen.sh"
manifest=sds-local-lsc.yaml
localStorageClassName=nested-local-thin
targetThinPoolName=thin-data

discover_lvgs() {
  kubectl get lvmvolumegroup -o json | jq -rc \
    --arg targetThinPoolName "${targetThinPoolName}" '
      .items[]
      | select((.spec.thinPools // []) | any(.name == $targetThinPoolName))
      | {name: .metadata.name}
    '
}

lvgs=$(discover_lvgs)

if [[ -z "${lvgs}" ]]; then
  echo "[WARNING] No LVMVolumeGroup resources with thin pool ${targetThinPoolName} found"
  echo "[INFO] Trying to create missing LVMVolumeGroup resources via ${lvg_generator_script}"
  kubectl get lvmvolumegroup -o wide || true

  if [[ ! -x "${lvg_generator_script}" ]]; then
    chmod +x "${lvg_generator_script}"
  fi

  "${lvg_generator_script}"

  lvgs=$(discover_lvgs)
fi

if [[ -z "${lvgs}" ]]; then
  echo "[ERROR] No LVMVolumeGroup resources with thin pool ${targetThinPoolName} found after creation attempt"
  kubectl get lvmvolumegroup -o wide || true
  exit 1
fi

cat << EOF > "${manifest}"
---
apiVersion: storage.deckhouse.io/v1alpha1
kind: LocalStorageClass
metadata:
  name: ${localStorageClassName}
spec:
  lvm:
    type: Thin
    lvmVolumeGroups:
EOF

for lvg in ${lvgs}; do
  lvg_name=$(echo "${lvg}" | jq -r '.name')
  echo "[INFO] Add LVMVolumeGroup ${lvg_name} to LocalStorageClass"
cat << EOF >> "${manifest}"
      - name: ${lvg_name}
        thin:
          poolName: ${targetThinPoolName}
EOF
done

cat << EOF >> "${manifest}"
  reclaimPolicy: Delete
  volumeBindingMode: WaitForFirstConsumer
EOF

kubectl apply -f "${manifest}"

for i in $(seq 1 60); do
  lsc_phase=$(kubectl get localstorageclass "${localStorageClassName}" -o jsonpath='{.status.phase}' 2>/dev/null || true)
  if [[ "${lsc_phase}" == "Created" ]]; then
    echo "[SUCCESS] LocalStorageClass ${localStorageClassName} is Created"
    kubectl get localstorageclass "${localStorageClassName}" -o yaml
    kubectl get storageclass "${localStorageClassName}"
    exit 0
  fi

  echo "[INFO] Waiting for LocalStorageClass ${localStorageClassName} to become Created (attempt ${i}/60)"
  if (( i % 5 == 0 )); then
    kubectl get localstorageclass "${localStorageClassName}" -o yaml || true
  fi
  sleep 10
done

echo "[ERROR] LocalStorageClass ${localStorageClassName} was not created in time"
kubectl get localstorageclass "${localStorageClassName}" -o yaml || true
kubectl get storageclass || true
exit 1
