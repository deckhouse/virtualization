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

require_env NAMESPACE
require_env STORAGE_TYPE
require_env DECKHOUSE_CHANNEL
require_env POD_SUBNET_CIDR
require_env SERVICE_SUBNET_CIDR
require_env K8S_VERSION
require_env PROD_IO_REGISTRY_DOCKER_CFG
require_env VIRTUALIZATION_IMAGE_URL
require_env DEFAULT_USER
require_env APT_MIRROR_ENABLED
require_env APT_MIRROR_NAME
require_env APT_MIRROR_URL
require_env CLUSTER_CONFIG_WORKERS_MEMORY
require_env NESTED_CLUSTER_NETWORK_NAME
require_env DEV_REGISTRY_DOCKER_CFG
: "${DEV_REGISTRY_DOCKER_CFG:?}"

default_storage_class="$(kubectl get storageclass -o json \
  | jq -r '.items[] | select(.metadata.annotations."storageclass.kubernetes.io/is-default-class" == "true") | .metadata.name')"

export DEFAULT_STORAGE_CLASS="${default_storage_class}"
envsubst < values.yaml.tmpl > values.yaml

mkdir -p tmp
touch tmp/discovered-values.yaml

registry="$(base64 -d <<< "${DEV_REGISTRY_DOCKER_CFG}" | jq '.auths | to_entries | .[] | .key' -r)"
auth="$(base64 -d <<< "${DEV_REGISTRY_DOCKER_CFG}" | jq '.auths | to_entries | .[] | .value.auth' -r)"

REGISTRY="${registry}" yq eval --inplace '.discovered.registry_url = env(REGISTRY)' tmp/discovered-values.yaml
AUTH="${auth}" yq eval --inplace '.discovered.registry_auth = env(AUTH)' tmp/discovered-values.yaml
