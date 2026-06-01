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

if [ "$#" -lt 1 ] || [ "$#" -gt 2 ]; then
  echo "[ERROR] Usage: $0 <setup-cluster-type-path> [kubeconfig-b64]" >&2
  exit 1
fi

setup_cluster_type_path="$1"
kubeconfig_b64="${2:-}"

sudo chown -fR 1001:1001 "${setup_cluster_type_path}" || true
yq e '.deckhouse.registryDockerCfg = "None"' -i "./${setup_cluster_type_path}/values.yaml" || true
yq e 'select(.kind == "InitConfiguration").deckhouse.registryDockerCfg = "None"' -i "./${setup_cluster_type_path}/tmp/config.yaml" || echo "The config.yaml file is not generated, skipping"
yq e '.discovered.registry_url = "None"' -i "./${setup_cluster_type_path}/tmp/discovered-values.yaml" || echo "The discovered-values.yaml file is not generated, skipping editing registry_url"
yq e '.discovered.registry_auth = "None"' -i "./${setup_cluster_type_path}/tmp/discovered-values.yaml" || echo "The discovered-values.yaml file is not generated, skipping editing registry_auth"

if [ -n "${kubeconfig_b64}" ]; then
  echo "${kubeconfig_b64}" | base64 -d | base64 -d > "./${setup_cluster_type_path}/kube-config" || echo "kubeconfig not available, skipping"
else
  echo "kubeconfig not available, skipping"
fi
