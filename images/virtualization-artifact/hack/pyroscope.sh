#!/bin/bash

set -eo pipefail

function usage {
    cat <<EOF
Usage: $0 COMMAND OPTIONS

Commands:
  run    Run pyroscope.
         Flags:
         --namespace,-n  (required)  Namespace of application.
         --service,-s    (required)  Connecting service.
         --port,-p       (required)  Connecting port service.

  wipe   Stop and cleanup.

Examples:
  # Run"
    $(basename "$0") run --service="your service" --port="your service port" --namespace="your namespace"
  # Wipe"
    $(basename "$0") wipe
EOF
}

DIR="$(dirname "$0")"
DOCKER_COMPOSE_FILE="${DIR}/pyroscope/docker-compose.yaml"
PYROSCOPE_PORT="8081"
PROCESS_NAME="PyroscopePortForward"

# shellcheck disable=SC2120
function usage_exit {
    local rc="${1:-"0"}"
    usage
    exit "$rc"
}

function run() {
    if [[ -z $NAMESPACE ]] || [[ -z $SERVICE ]] || [[ -z $PORT ]]; then
        usage_exit 1
    fi
    local cmd=$1
    exec -a "${PROCESS_NAME}" kubectl port-forward "services/${SERVICE}" "${PYROSCOPE_PORT}:${PORT}" &
    cmd+=" -f ${DOCKER_COMPOSE_FILE} up -d"
    eval "$cmd"
}

function wipe() {
    local cmd=$1
    cmd+=" -f ${DOCKER_COMPOSE_FILE} down"
    eval "$cmd"
    pkill -f "${PROCESS_NAME}"
}

source "${DIR}/args.sh"
set_flags_args "$@"

if [[ $(parse_flag "help" "h") == "TRUE" ]]; then
    usage_exit
fi

NAMESPACE=$(parse_flag "namespace" "n")
SERVICE=$(parse_flag "service" "s")
PORT=$(parse_flag "port" "p")

cmd="docker compose"
which docker-compose &>/dev/null && cmd="docker-compose"

CMD="${ARGS[0]}"
case "$CMD" in
    "run")
        run "$cmd"
        echo "Pyroscope launched successfully."
        echo "Open http://localhost:4040 in your browser."
        ;;
    "wipe")
        wipe "$cmd"
        ;;
    *)
        usage_exit 1
        ;;
esac

