#!/bin/bash
set -eo pipefail

function usage {
    cat <<EOF
  "Usage: $0 [run/wipe] --app=<?> --namespace=<?> --deployment=<?>"
  "Examples:"
    # Run"
      $(basename "$0") run --app="/path/to/main.go" --deployment="your deployment" --namespace="your namespace"
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
    kubectl -n "${NAMESPACE}" delete deployment/"${NEW_NAME}"
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
  kubectl -n "${NAMESPACE}" get deployment/"${DEPLOYMENT}" -ojson | \
  jq '.metadata.name = env.NEW_NAME |
      .spec.template.spec.containers[0].command = [ "/bin/bash", "-c", "--" ] |
      .spec.template.spec.containers[0].args = [ "while true; do sleep 60; done;" ] |
      .spec.replicas = 1 |
      .spec.template.metadata.labels.mirror = "true"' | \
  kubectl create -f -
fi

kubectl -n "${NAMESPACE}" wait pod --for=jsonpath='{.status.phase}'=Running -l mirror=true,app="${DEPLOYMENT}" --timeout 60s
kubectl -n "${NAMESPACE}" scale deployment "${DEPLOYMENT}" --replicas 0

mirrord exec --config-file "${CONFIG_MIRRORD}"  \
  --target "deployment/${NEW_NAME}"             \
  --target-namespace "${NAMESPACE}"             \
  "${BIN_DIR}/${BINARY}" -- $FLAGS
