#!/bin/bash
set -o errexit
set -o nounset
set -o pipefail

function usage {
  cat <<EOF
Usage: $(basename "$0") <core/subresources/all>
Example:
   $(basename "$0") controller
EOF
}

function source::settings {
  SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd -P)"
  SCRIPT_ROOT="${SCRIPT_DIR}/.."
  CODEGEN_PKG="$(go env GOMODCACHE)/$(go list -f '{{.Path}}@{{.Version}}' -m k8s.io/code-generator)"
  MODULE="github.com/deckhouse/virtualization/api"
  source "${CODEGEN_PKG}/kube_codegen.sh"
}

function generate::subresources {
          cd "${SCRIPT_ROOT}"

          bash "${CODEGEN_PKG}/generate-groups.sh" "deepcopy" "${MODULE}/subresources" . ":subresources" \
            --go-header-file "${SCRIPT_ROOT}/scripts/boilerplate.go.txt" \
            --output-base "${SCRIPT_ROOT}"

          cd "${SCRIPT_ROOT}/subresources"

          bash "${CODEGEN_PKG}/generate-groups.sh" "deepcopy,conversion" "${MODULE}/subresources" . ":v1alpha2" \
          --go-header-file "${SCRIPT_ROOT}/scripts/boilerplate.go.txt" \
          --output-base "${SCRIPT_ROOT}/subresources"

          chmod +x "${GOPATH}/bin/openapi-gen"
          "${GOPATH}/bin/openapi-gen" \
            -i "${MODULE}/core/v1alpha2,${MODULE}/subresources/v1alpha2,kubevirt.io/api/core/v1,k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1,k8s.io/api/core/v1,k8s.io/apimachinery/pkg/apis/meta/v1,k8s.io/apimachinery/pkg/api/resource,k8s.io/apimachinery/pkg/version" \
            -p pkg/apiserver/api/generated/openapi/        \
            -O zz_generated.openapi                        \
            -o "${SCRIPT_ROOT}"                            \
            -h "${SCRIPT_ROOT}/scripts/boilerplate.go.txt" -r /dev/null
}

function generate::core {
          cd "${SCRIPT_ROOT}/core"

          bash "${CODEGEN_PKG}/generate-groups.sh" "deepcopy" "${MODULE}/core" . ":v1alpha2" \
            --go-header-file "${SCRIPT_ROOT}/scripts/boilerplate.go.txt" \
            --output-base "${SCRIPT_ROOT}/core"

          OUTPUT_BASE=$(mktemp -d)
          trap 'rm -rf "${OUTPUT_BASE}"' ERR EXIT

          bash "${CODEGEN_PKG}/generate-groups.sh" "client,lister,informer" "${MODULE}/client/generated" "${MODULE}" "core:v1alpha2" \
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
  subresources)
    source::settings
    generate::subresources
    ;;
  all)
    source::settings
    generate::core
    generate::subresources
    ;;
*)
    echo "Invalid argument: $WHAT"
    usage
    exit 1
    ;;
esac


