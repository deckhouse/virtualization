#!/bin/bash

# Copyright 2024 Flant JSC
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
set -o errexit
set -o nounset
set -o pipefail

function usage {
    cat <<EOF
Usage: $(basename "$0") { core | subresources | crds | all }
Example:
   $(basename "$0") core
EOF
}

function source::settings {
    SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd -P)"
    API_ROOT="${SCRIPT_DIR}/.."
    ROOT="${API_ROOT}/.."
    CODEGEN_PKG="$(go env GOMODCACHE)/$(go list -f '{{.Path}}@{{.Version}}' -m k8s.io/code-generator)"
    THIS_PKG="github.com/deckhouse/virtualization/api"
    # TODO: Temporary filter until all CRDs become auto-generated.
    ALLOWED_RESOURCE_GEN_CRD=("VirtualMachineClass"
                              "VirtualMachineBlockDeviceAttachment"
                              "VirtualMachineSnapshot"
                              "VirtualMachineRestore"
                              "VirtualMachineOperation"
                              "VirtualMachineSnapshotOperation"
                              "VirtualDisk"
                              "VirtualImage"
                              "ClusterVirtualImage")

    source "${CODEGEN_PKG}/kube_codegen.sh"
}

function generate::subresources {
    kube::codegen::gen_helpers                           \
        --boilerplate "${SCRIPT_DIR}/boilerplate.go.txt" \
        "${API_ROOT}/subresources"

    go tool openapi-gen                                                                           \
        --output-pkg "openapi"                                                                    \
        --output-dir "${ROOT}/images/virtualization-artifact/pkg/apiserver/api/generated/openapi" \
        --output-file "zz_generated.openapi.go"                                                   \
        --go-header-file "${SCRIPT_DIR}/boilerplate.go.txt"                                       \
        -r /dev/null                                                                              \
       "${THIS_PKG}/core/v1alpha2" "${THIS_PKG}/subresources/v1alpha2" "kubevirt.io/api/core/v1" "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1" "k8s.io/api/core/v1" "k8s.io/apimachinery/pkg/apis/meta/v1" "k8s.io/apimachinery/pkg/api/resource" "k8s.io/apimachinery/pkg/version"
}

function generate::core {
    kube::codegen::gen_helpers                           \
        --boilerplate "${SCRIPT_DIR}/boilerplate.go.txt" \
        "${API_ROOT}/core"

    kube::codegen::gen_client                            \
        --with-watch                                     \
        --output-dir "${API_ROOT}/client/generated"      \
        --output-pkg "${THIS_PKG}/client/generated"      \
        --boilerplate "${SCRIPT_DIR}/boilerplate.go.txt" \
        "${API_ROOT}"
}

function generate::crds {
    OUTPUT_BASE=$(mktemp -d)
    trap 'rm -rf "${OUTPUT_BASE}"' ERR EXIT

    go tool controller-gen crd paths="${API_ROOT}/core/v1alpha2/..." output:crd:dir="${OUTPUT_BASE}"

    # shellcheck disable=SC2044
    for file in $(find "${OUTPUT_BASE}"/* -type f -iname "*.yaml"); do
        # TODO: Temporary filter until all CRDs become auto-generated.
        # shellcheck disable=SC2002
        if ! [[ " ${ALLOWED_RESOURCE_GEN_CRD[*]} " =~ [[:space:]]$(cat "$file" | yq '.spec.names.kind')[[:space:]] ]]; then
            continue
        fi
        cp "$file" "${ROOT}/crds/$(echo $file | awk -Fio_ '{print $2}')"
    done
}

WHAT=$1
if [ "$#" != 1 ] || [ "${WHAT}" == "--help" ] ; then
    usage
    exit
fi

case "$WHAT" in
    core)
        source::settings
        generate::core
        ;;
    subresources)
        source::settings
        generate::subresources
        ;;
    crds)
        source::settings
        generate::crds
        ;;
    all)
        source::settings
        generate::core
        generate::subresources
        generate::crds
        ;;
    *)
        echo "Invalid argument: $WHAT"
        usage
        exit 1
        ;;
esac


