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

    echo "exec process name: ${PROCESS_NAME}"
    echo "exec command: kubectl -n ${NAMESPACE} port-forward services/${SERVICE} ${PYROSCOPE_PORT}:${PORT}"
    exec -a "${PROCESS_NAME}" kubectl -n "${NAMESPACE}" port-forward "services/${SERVICE}" "${PYROSCOPE_PORT}:${PORT}" &
    docker compose -f "${DOCKER_COMPOSE_FILE}" up -d
}

function wipe() {
    docker compose -f "${DOCKER_COMPOSE_FILE}" down
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

docker compose version &>/dev/null || (echo "No docker compose found" ; exit 1 )

CMD="${ARGS[0]}"
case "$CMD" in
    "run")
        run
        echo "Pyroscope launched successfully."
        echo "Open http://localhost:4040 in your browser."
        ;;
    "wipe")
        wipe
        ;;
    *)
        usage_exit 1
        ;;
esac

