#!/bin/bash
set -o errexit
set -o nounset
set -o pipefail

function usage {
  cat <<EOF
Usage: $(basename "$0") <controller/apiserver/all>
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

function generate::apiserver {
        bash "${CODEGEN_PKG}/generate-groups.sh" deepcopy "${MODULE}" "./pkg/apiserver" "apis:operations" \
          --go-header-file "${SCRIPT_ROOT}/scripts/boilerplate.go.txt" \
          --output-base "${SCRIPT_ROOT}"
        bash "${CODEGEN_PKG}/generate-groups.sh" deepcopy "${MODULE}" "./pkg/apiserver/apis" "operations:v1alpha1" \
          --go-header-file "${SCRIPT_ROOT}/scripts/boilerplate.go.txt" \
          --output-base "${SCRIPT_ROOT}"

        chmod +x "${GOPATH}/bin/openapi-gen"
        "${GOPATH}/bin/openapi-gen" \
          -i "${MODULE}/pkg/apiserver/apis/operations/v1alpha1,k8s.io/apimachinery/pkg/apis/meta/v1,k8s.io/apimachinery/pkg/api/resource,k8s.io/apimachinery/pkg/version" \
          -p pkg/apiserver/api/generated/openapi/        \
          -O zz_generated.openapi                        \
          -o "${SCRIPT_ROOT}"                            \
          -h "${SCRIPT_ROOT}/scripts/boilerplate.go.txt" -r /dev/null
}

function generate::controller {
        bash "${CODEGEN_PKG}/generate-groups.sh" deepcopy "${MODULE}" . "api:v1alpha2" \
          --go-header-file "${SCRIPT_ROOT}/scripts/boilerplate.go.txt" \
          --output-base "${SCRIPT_ROOT}"
}


WHAT=$1
if [ "$#" != 1 ] || [ "${WHAT}" == "--help" ] ; then
  usage
  exit
fi

case "$WHAT" in
  controller)
    source::settings
    generate::controller
    ;;
  apiserver*)
    source::settings
    generate::apiserver
    ;;
  all)
    source::settings
    generate::apiserver
    generate::controller
    ;;
*)
    echo "Invalid argument: $WHAT"
    usage
    exit 1
    ;;
esac


