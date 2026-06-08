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
# shellcheck source=.github/scripts/bash/e2e/wait-sds-replicated.sh
source "${SCRIPT_DIR}/wait-sds-replicated.sh"

d8_queue

kubectl apply -f ../sds-node-configurator/mc.yaml
kubectl apply -f mc.yaml

echo "[INFO] Wait for sds-node-configurator"
kubectl wait --for=jsonpath='{.status.phase}'=Ready modules sds-node-configurator --timeout=300s

echo "[INFO] Wait for sds-replicated-volume to be ready"
sds_replicated_ready
kubectl wait --for=jsonpath='{.status.phase}'=Ready modules sds-replicated-volume --timeout=300s

echo "[INFO] Wait BlockDevice are ready"
blockdevices_ready

echo "[INFO] Wait pods and webhooks sds-replicated pods"
sds_pods_ready

chmod +x ../sds-node-configurator/lvg-gen.sh
../sds-node-configurator/lvg-gen.sh

chmod +x rsc-gen.sh
./rsc-gen.sh

echo "[INFO] Show existing storageclasses"
if ! kubectl get storageclass | grep -q nested; then
  echo "[WARNING] No nested storageclasses"
else
  kubectl get storageclass | grep nested
  echo "[SUCCESS] Done"
fi
