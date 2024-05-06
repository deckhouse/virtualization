#!/bin/bash
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
export NEW_NAME

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

if ! kubectl -n "${NAMESPACE}" get deployment/"${NEW_NAME}" &>/dev/null; then
  CTR_NUMBER=0
  if [[ -n $CONTAINER_NAME ]]; then
    for ctr in $(kubectl -n "${NAMESPACE}" get deployment/"${DEPLOYMENT}" -o jsonpath='{.spec.template.spec.containers[*].name}'); do
      if [[ "$ctr" != "$CONTAINER_NAME" ]]; then
         CTR_NUMBER=$(echo "$CTR_NUMBER + 1" | bc)
         continue
      fi
      break
    done
  fi
  kubectl -n "${NAMESPACE}" get deployment/"${DEPLOYMENT}" -ojson | \
  jq --argjson CTR_NUMBER $CTR_NUMBER '.metadata.name = env.NEW_NAME |
      .spec.template.spec.containers[$CTR_NUMBER].command = [ "/bin/bash", "-c", "--" ] |
      .spec.template.spec.containers[$CTR_NUMBER].args = [ "while true; do sleep 60; done;" ] |
      .spec.replicas = 1 |
      .spec.template.metadata.labels.mirror = "true" |
      .spec.template.metadata.labels.ownerName = env.NEW_NAME' | \
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
