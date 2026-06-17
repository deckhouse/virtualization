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
# shellcheck source=.github/scripts/bash/e2e/common.sh
source "${SCRIPT_DIR}/common.sh"

if ! kubectl get vmclass generic-for-e2e 2>/dev/null; then
  kubectl get vmclass/generic -o json | jq 'del(.status) | del(.metadata) | .metadata = {"name":"generic-for-e2e","annotations":{"virtualmachineclass.virtualization.deckhouse.io/is-default-class":"true"}} ' | kubectl create -f -
fi

echo "[INFO] Showing existing vmclasses"
kubectl get vmclass
