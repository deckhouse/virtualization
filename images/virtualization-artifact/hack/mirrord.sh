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
set -eo pipefail

function usage {
    cat <<EOF
Usage: $0 COMMAND OPTIONS

Commands:
  run    Run locally executed application in a cluster environment.
         Arguments:
         --app            Path to main.go
         --namespace      Namespace of deployment
         --deployment     Deployment where application should be injected
         --container-name Container where application should be injected
         --flags          Arguments for application.

  wipe   Stop and cleanup.
         Arguments:
         --namespace     Namespace of deployment
         --deployment    Deployment where application was injected
  "Examples:"
    # Run"
      $(basename "$0") run --app="/path/to/main.go" --deployment="your deployment" --namespace="your namespace" --flags="--app-flag1=flag1, app-flag2=flag2"
    # Wipe"
      $(basename "$0") wipe --deployment="your deployment" --namespace="your namespace"
EOF
  exit 1
}

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd -P)"
BIN_DIR="${SCRIPT_DIR}/../bin"
CONFIG_MIRRORD="${SCRIPT_DIR}/mirrord-config.json"

COMMAND=$1
shift
# Set naming variable
while [[ $# -gt 0 ]]; do
    case "$1" in
    --app=*)
        APP="${1#*=}"
        shift
        ;;
    --deployment=*)
        DEPLOYMENT="${1#*=}"
        BINARY="${1#*=}"
        shift
        ;;
    --namespace=*)
        NAMESPACE="${1#*=}"
        shift
        ;;
    --container-name=*)
        CONTAINER_NAME="${1#*=}"
        shift
        ;;
    --flags=*)
        FLAGS="${1#*=}"
        shift
        ;;
    *)
        echo "Invalid argument: $1"
        usage
        exit 1
        ;;
    esac
done

NEW_NAME="mirrord-copy-${DEPLOYMENT}"

if [[ $COMMAND == "run" ]] &&  [[ -n $DEPLOYMENT ]] && [[ -n $BINARY ]] && [[ -n $NAMESPACE ]] && [[ -n $APP ]]; then
 echo "Starting mirror..."
elif [[ $COMMAND == "wipe" ]] && [[ -n $DEPLOYMENT ]] && [[ -n $BINARY ]] && [[ -n $NAMESPACE ]]; then
    echo "Stopping mirror..."
    echo "Delete deployment ${NAMESPACE}/${NEW_NAME}"
    kubectl -n "${NAMESPACE}" delete --cascade="foreground" --grace-period 0 deployment "${NEW_NAME}"
    kubectl -n "${NAMESPACE}" scale deployment "${DEPLOYMENT}" --replicas 1
    exit 0
else
  usage
fi

if [ ! -d "${BIN_DIR}" ]; then
  mkdir "${BIN_DIR}"
fi

go build -ldflags='-linkmode external' -o "${BIN_DIR}/${BINARY}" "${APP}"
chmod +x "${BIN_DIR}/${BINARY}"

if ! kubectl -n "${NAMESPACE}" get "deployment/${NEW_NAME}" &>/dev/null; then
  kubectl -n "${NAMESPACE}" get "deployment/${DEPLOYMENT}" -ojson | \
  jq --arg CONTAINER_NAME "$CONTAINER_NAME" --arg NEW_NAME "$NEW_NAME" '.metadata.name = $NEW_NAME |
    (.spec.template.spec.containers[] | select(.name == $CONTAINER_NAME) ) |= (.command= [ "/bin/sh", "-c", "--" ] | .args = [ "while true; do sleep 60; done;" ] | .image = "alpine:3.20.1") |
    .spec.replicas = 1 |
    .spec.template.metadata.labels.mirror = "true" |
    .spec.template.metadata.labels.ownerName = $NEW_NAME' | \
  kubectl create -f -
fi

kubectl -n "${NAMESPACE}" wait pod --for=jsonpath='{.status.phase}'=Running -l mirror=true,ownerName="${NEW_NAME}" --timeout 60s
kubectl -n "${NAMESPACE}" scale deployment "${DEPLOYMENT}" --replicas 0
kubectl -n "${NAMESPACE}" wait --for=jsonpath='{.spec.replicas}'=0 deployment "${DEPLOYMENT}"

TARGET="deployment/${NEW_NAME}"
if [[ -n $CONTAINER_NAME ]]; then
  POD_NAME=$(kubectl -n "${NAMESPACE}" get pod -l mirror=true,ownerName="${NEW_NAME}" -o jsonpath='{.items[0].metadata.name}')
  TARGET="pod/${POD_NAME}/container/${CONTAINER_NAME}"
fi

mirrord exec --config-file "${CONFIG_MIRRORD}"  \
  --target "${TARGET}"                          \
  --target-namespace "${NAMESPACE}"             \
  "${BIN_DIR}/${BINARY}" -- $(echo $FLAGS | sed 's!"!!g')
