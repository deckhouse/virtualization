#!/bin/bash

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

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd -P)"
ROOT="${SCRIPT_DIR}/../../.."
API_ROOT="${SCRIPT_DIR}/../api"
CODEGEN_PKG="$(go env GOMODCACHE)/$(go list -f '{{.Path}}@{{.Version}}' -m k8s.io/code-generator)"
source "${CODEGEN_PKG}/kube_codegen.sh"
ALLOWED_RESOURCE_GEN_CRD=("WireguardSystemNetwork")
THIS_PKG="github.com/deckhouse/virtualization-dra/api"

function generate::helpers {
    kube::codegen::gen_helpers                           \
        --boilerplate "${SCRIPT_DIR}/boilerplate.go.txt" \
        "${API_ROOT}"
}

function generate::clients {
      kube::codegen::gen_client                            \
          --with-watch                                     \
          --output-dir "${API_ROOT}/client/generated"      \
          --output-pkg "${THIS_PKG}/client/generated"      \
          --boilerplate "${SCRIPT_DIR}/boilerplate.go.txt" \
          "${API_ROOT}/../"
}

function generate::crds {
    OUTPUT_BASE=$(mktemp -d)
    trap 'rm -rf "${OUTPUT_BASE}"' ERR EXIT

    go tool controller-gen crd paths="${API_ROOT}/v1alpha1/..." output:crd:dir="${OUTPUT_BASE}"

    # shellcheck disable=SC2044
    for file in $(find "${OUTPUT_BASE}"/* -type f -iname "*.yaml"); do
        # TODO: Temporary filter until all CRDs become auto-generated.
        # shellcheck disable=SC2002
        if ! [[ " ${ALLOWED_RESOURCE_GEN_CRD[*]} " =~ [[:space:]]$(cat "$file" | yq '.spec.names.kind')[[:space:]] ]]; then
            continue
        fi
        cp "$file" "${ROOT}/crds/embedded/$(echo $file | awk -Fio_ '{print $2}')"
    done
}

generate::helpers
generate::clients
generate::crds
