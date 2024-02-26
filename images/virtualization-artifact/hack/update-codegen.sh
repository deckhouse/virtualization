#!/bin/bash
set -o errexit
set -o nounset
set -o pipefail

function usage {
  cat <<EOF
Usage: $(basename "$0") <core/operations/all>
Example:
   $(basename "$0") controller
EOF
}

function source::settings {
  SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd -P)"
  SCRIPT_ROOT="${SCRIPT_DIR}/.."
  CODEGEN_PKG="$(go env GOMODCACHE)/$(go list -f '{{.Path}}@{{.Version}}' -m k8s.io/code-generator)"
  MODULE="github.com/deckhouse/virtualization-controller"
  source "${CODEGEN_PKG}/kube_codegen.sh"
}

function generate::operations {
          bash "${CODEGEN_PKG}/generate-groups.sh" deepcopy "${MODULE}" . "api:operations" \
            --go-header-file "${SCRIPT_ROOT}/scripts/boilerplate.go.txt" \
            --output-base "${SCRIPT_ROOT}"
          bash "${CODEGEN_PKG}/generate-groups.sh" "deepcopy" "${MODULE}" ./api "operations:v1alpha1" \
          --go-header-file "${SCRIPT_ROOT}/scripts/boilerplate.go.txt" \
          --output-base "${SCRIPT_ROOT}"

          chmod +x "${GOPATH}/bin/openapi-gen"
          "${GOPATH}/bin/openapi-gen" \
            -i "${MODULE}/api/core/v1alpha2,${MODULE}/api/operations/v1alpha1,kubevirt.io/api/core/v1,k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1,k8s.io/api/core/v1,k8s.io/apimachinery/pkg/apis/meta/v1,k8s.io/apimachinery/pkg/api/resource,k8s.io/apimachinery/pkg/version" \
            -p pkg/apiserver/api/generated/openapi/        \
            -O zz_generated.openapi                        \
            -o "${SCRIPT_ROOT}"                            \
            -h "${SCRIPT_ROOT}/scripts/boilerplate.go.txt" -r /dev/null
}

function generate::core {
          OUTPUT_BASE=$(mktemp -d)
          trap 'rm -rf "${OUTPUT_BASE}"' ERR EXIT
          bash "${CODEGEN_PKG}/generate-groups.sh" "deepcopy" "${MODULE}" ./api "core:v1alpha2" \
            --go-header-file "${SCRIPT_ROOT}/scripts/boilerplate.go.txt" \
            --output-base "${SCRIPT_ROOT}"

          bash "${CODEGEN_PKG}/generate-groups.sh" "client,lister,informer" "${MODULE}/api/client" "${MODULE}/api" "core:v1alpha2" \
          --go-header-file "${SCRIPT_ROOT}/scripts/boilerplate.go.txt" \
          --output-base "${OUTPUT_BASE}"
          cp -R "${OUTPUT_BASE}/${MODULE}/." "${SCRIPT_ROOT}"
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
  operations)
    source::settings
    generate::operations
    ;;
  all)
    source::settings
    generate::core
    generate::operations
    ;;
*)
    echo "Invalid argument: $WHAT"
    usage
    exit 1
    ;;
esac


