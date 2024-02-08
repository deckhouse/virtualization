#!/bin/bash
set -eo pipefail

function usage { echo "Usage: $0 [run/wipe]" ; exit 1; }

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd -P)"
BIN_DIR="${SCRIPT_DIR}/../bin"
APP="${SCRIPT_DIR}/../cmd/virtualization-controller/main.go"
CONFIG_MIRRORD="${SCRIPT_DIR}/mirrord-config.json"
BINARY="virtualization-controller"
NAMESPACE="d8-virtualization"
DEPLOYMENT="virtualization-controller"
NEW_NAME="mirrord-copy-${DEPLOYMENT}"
export NEW_NAME

if [[ -z $1 ]]; then
  usage
elif [[ $1 == "wipe" ]]; then
  echo "Stopping mirror..."
  echo "Delete deployment ${NAMESPACE}/${NEW_NAME}"
  kubectl -n "${NAMESPACE}" delete deployment/"${NEW_NAME}"
  kubectl -n "${NAMESPACE}" scale deployment "${DEPLOYMENT}" --replicas 1
  exit 0
elif [[ $1 == "run" ]]; then
  echo "Starting mirror..."
else
  usage
fi


if [ ! -d "${BIN_DIR}" ]; then
  mkdir "${BIN_DIR}"
fi

go build -ldflags='-linkmode external' -o "${BIN_DIR}/${BINARY}" "${APP}"
chmod +x "${BIN_DIR}/${BINARY}"

if ! kubectl -n "${NAMESPACE}" get deployment/"${NEW_NAME}" &>/dev/null; then
  kubectl -n "${NAMESPACE}" get deployment/"${DEPLOYMENT}" -ojson | \
  jq '.metadata.name = env.NEW_NAME |
      .spec.template.spec.containers[0].command = [ "/bin/bash", "-c", "--" ] |
      .spec.template.spec.containers[0].args = [ "while true; do sleep 60; done;" ] |
      .spec.replicas = 1 |
      .spec.template.metadata.labels.mirror = "true"' | \
  kubectl create -f -
fi

kubectl wait pod --for=jsonpath='{.status.phase}'=Running -l mirror=true,app="${DEPLOYMENT}" --timeout 60s
kubectl -n "${NAMESPACE}" scale deployment "${DEPLOYMENT}" --replicas 0
mirrord exec --config-file "${CONFIG_MIRRORD}"  --target "deployment/${NEW_NAME}" --target-namespace "${NAMESPACE}" "${BIN_DIR}/${BINARY}"
