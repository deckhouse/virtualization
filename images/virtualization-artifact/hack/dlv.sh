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
  build <controller/apiserver/audit> Build docker image with dlv.
  push  <controller/apiserver/audit> Build and Push docker image with dlv.

Global Flags:
 --image,-i  (optional)  The name of the image being built.

Examples:
  # build"
    $(basename "$0") build controller --image=myimage:latest
  # push"
    $(basename "$0") push apiserver
EOF
}

# shellcheck disable=SC2120
function usage_exit {
    local rc="${1:-"0"}"
    usage
    exit "$rc"
}

function build_controller {
    build "dlv-controller.Dockerfile"
}

function build_apiserver {
    build "dlv-apiserver.Dockerfile"
}

function build_audit {
    build "dlv-audit.Dockerfile"
}

function build {
    local dockerfile=$1
    cd "$ROOT"
    docker build -f "./images/virtualization-artifact/hack/$dockerfile" -t "${IMAGE}" --platform=linux/x86_64 .
}

function push {
    docker push "${IMAGE}"
}

# shellcheck disable=SC2120
function print_patches_controller {
    local deployment=$1

    cat <<EOF

Run commands:
kubectl -n d8-virtualization scale deployment ${deployment} --replicas 1
kubectl -n d8-virtualization patch deployment ${deployment} --type='strategic' -p '{
    "spec": {
        "template": {
            "spec": {
                "containers": [{
                       "name": "${deployment}",
                        "image": "${IMAGE}",
                        "ports": [{"containerPort": 2345, "name": "dlv"}]
                        "readinessProbe": null,
                        "livenessProbe": null
                    },
                    {
                        "name": "proxy",
                        "readinessProbe": null,
                        "livenessProbe": null
                    },
                    {
                        "name": "kube-rbac-proxy",
                        "readinessProbe": null,
                        "livenessProbe": null
                    }
                }]
            }
        }
    }
}'
kubectl -n d8-virtualization port-forward deployments/${deployment} 2345:2345

EOF
}

DIR="$(dirname "$0")"
ROOT="${DIR}/../../../"
cd "$ROOT"

source "${DIR}/args.sh"
set_flags_args "$@"

if [[ $(parse_flag "help" "h") == "TRUE" ]]; then
    usage_exit
fi

IMAGE=$(parse_flag "image" "i")

if [[ -z $IMAGE ]] ; then
    IMAGE="ttl.sh/$(uuidgen | awk '{print tolower($0)}'):10m"
fi

CMD="${ARGS[0]}"
NAME="${ARGS[1]}"
if [[ -z $NAME ]]; then
   usage_exit 1
fi
case "$CMD" in
    "build")
        case "$NAME" in
            "controller")
                build_controller
                ;;
            "apiserver")
                build_apiserver
                ;;
            "audit")
                build_audit
                ;;
            *)
                usage_exit 1
                ;;
        esac
        echo "Built image ${IMAGE} successfully."
        ;;
    "push")
        deployment=""
        case "$NAME" in
            "controller")
                build_controller
                deployment="virtualization-controller"
                ;;
            "apiserver")
                build_apiserver
                deployment="virtualization-api"
                ;;
            "audit")
                build_audit
                deployment="virtualization-audit"
                ;;
            *)
                usage_exit 1
                ;;
        esac
        echo "Built image ${IMAGE} successfully."
        push
        echo "Push image ${IMAGE} successfully."
        print_patches_controller "${deployment}"
        ;;
    *)
        usage_exit 1
        ;;
esac

