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
  start           Start pyroscope and alloy.
                  Flags:
                  --namespace,-n  (required)  Namespace of application.
                  --service,-s    (required)  Connecting service.
                  --port,-p       (required)  Connecting port service.

  stop            Stop pyroscope and alloy.
  wipe-data       Wipe pyroscope data.
  info-data       Show pyroscope data info.

  start-pyroscope Start pyroscope only.
  stop-pyroscope  Stop pyroscope only.

Examples:
  # Start"
    $(basename "$0") start --service="your service" --port="your service port" --namespace="your namespace"
  # Stop"
    $(basename "$0") stop
EOF
}

DIR="$(dirname "$0")"
DOCKER_COMPOSE_FILE="${DIR}/pyroscope/docker-compose.yaml"
DOCKER_COMPOSE_FILE_PYROSCOPE_ONLY="${DIR}/pyroscope/docker-compose-pyroscope-only.yaml"
PYROSCOPE_VOLUME_NAME="pyroscope_pyroscope-data"
PYROSCOPE_PORT="8081"
PROCESS_NAME="PyroscopePortForward"

# shellcheck disable=SC2120
function usage_exit {
    local rc="${1:-"0"}"
    usage
    exit "$rc"
}

function start() {
    if [[ -z $NAMESPACE ]] || [[ -z $SERVICE ]] || [[ -z $PORT ]]; then
        usage_exit 1
    fi

    echo "exec process name: ${PROCESS_NAME}"
    echo "exec command: kubectl -n ${NAMESPACE} port-forward services/${SERVICE} ${PYROSCOPE_PORT}:${PORT}"
    exec -a "${PROCESS_NAME}" kubectl -n "${NAMESPACE}" port-forward "services/${SERVICE}" "${PYROSCOPE_PORT}:${PORT}" &
    docker compose -f "${DOCKER_COMPOSE_FILE}" up -d
}

function stop() {
    docker compose -f "${DOCKER_COMPOSE_FILE}" down
    pkill -f "${PROCESS_NAME}"
}

function wipe-data() {
    docker volume rm "${PYROSCOPE_VOLUME_NAME}"
}

function info-data() {
    docker volume inspect "${PYROSCOPE_VOLUME_NAME}"
}

function start-pyroscope() {
    docker compose -f "${DOCKER_COMPOSE_FILE_PYROSCOPE_ONLY}" up -d
}

function stop-pyroscope() {
    docker compose -f "${DOCKER_COMPOSE_FILE_PYROSCOPE_ONLY}" down
}

source "${DIR}/args.sh"
set_flags_args "$@"

if [[ $(parse_flag "help" "h") == "TRUE" ]]; then
    usage_exit
fi

NAMESPACE=$(parse_flag "namespace" "n")
SERVICE=$(parse_flag "service" "s")
PORT=$(parse_flag "port" "p")

docker compose version &>/dev/null || (echo "No docker compose found" ; exit 1 )

CMD="${ARGS[0]}"
case "$CMD" in
    "start")
        start
        echo "Pyroscope launched successfully."
        echo "Open http://localhost:4040 in your browser."
        ;;
    "stop")
        stop
        ;;
    "wipe-data")
        wipe-data
        ;;
    "info-data")
        info-data
        ;;
    "start-pyroscope")
        start-pyroscope
        echo "Pyroscope launched successfully."
        echo "Open http://localhost:4040 in your browser."
        ;;
    "stop-pyroscope")
        stop-pyroscope
        ;;
    *)
        usage_exit 1
        ;;
esac

