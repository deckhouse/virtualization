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

function usage {
    cat <<EOF
Usage: $(basename "$0") COMMAND OPTIONS

Commands:
  apply           Apply virtual machines.
                  Arguments:
                  (Required) --count: count of virtual machines to create.
                  (Optional) --namespace: namespace for virtual machines. If not defined - using default namespace.
                  (Optional) --storage-class: storage-class for virtual machine disks. If not defined - using default SC.
                  (Optional) --resources-prefix (default: performance): prefix to be used fo resource names.
  ---
  destroy         Destroy set of virtual machines.

Global Arguments:
  --name (default: performance): name for release of virtual machine.
  --resources: (default: 'all'): resources to manage. Possible values: 'disks', 'vms' or 'all'.

Examples:
  Bootstrap:
    $(basename "$0") apply --count=1
    $(basename "$0") apply --resources=disks --count=1 --namespace=default --storage-class=default
    $(basename "$0") destroy --resources=disks --namespace=default
EOF
}

function handle_exit() {
  for p in $(jobs -p); do pgrep -P "${p}" | xargs kill -9 ; done
}

function validate_global_args() {
  if [ "${RESOURCES}" != "all" ] && [ "${RESOURCES}" != "vms" ] && [ "${RESOURCES}" != "disks" ]; then
    echo "ERROR: Invalid --resources flag: allowed values 'disks', 'vms' or 'all'"
    usage
    exit 1
  fi
}

function validate_apply_args() {
  if [ -z "${COUNT}" ]; then
    echo "ERROR: --count flag is missed but required"
    usage
    exit 1
  fi

  if [ -z "${RESOURCES_PREFIX}" ]; then
    echo "ERROR: --resources-prefix flag is empty"
    usage
    exit 1
  fi
}

function apply() {
  echo "Apply resources: ${RESOURCES}"

  args=( upgrade --install "${RELEASE_NAME}" . -n "${NAMESPACE}" --create-namespace --set "count=${COUNT}" --set "resourcesPrefix=${RESOURCES_PREFIX}" --set "resources=${RESOURCES}" )
  if [ -n "${STORAGE_CLASS}" ]; then
      args+=( --set "storageClass=${STORAGE_CLASS}" )
  fi

  helm "${args[@]}"
}

function destroy() {
  echo "Delete resources: ${RESOURCES}"

  echo "$(date +"%Y-%m-%d %H:%M:%S") - Deleting release [${RELEASE_NAME}]"
  helm uninstall "${RELEASE_NAME}" -n "${NAMESPACE}"
  echo "$(date +"%Y-%m-%d %H:%M:%S") - Release [${RELEASE_NAME}] was deleted"

}

if [ "$#" -eq 0 ] || [ "${1}" == "--help" ] ; then
  usage
  exit
fi

trap 'handle_exit' EXIT INT ERR

COMMAND=$1
RELEASE_NAME="performance"
NAMESPACE=$(kubectl config view --minify -o jsonpath='{..namespace}')
RESOURCES="all"
RESOURCES_PREFIX="performance"

shift
# Set naming variable
while [[ $# -gt 0 ]]; do
    case "$1" in
    --count=*)
        COUNT="${1#*=}"
        shift
        ;;
    --namespace=*)
        NAMESPACE="${1#*=}"
        shift
        ;;
    --storage-class=*)
        STORAGE_CLASS="${1#*=}"
        shift
        ;;
    --name=*)
        RELEASE_NAME="${1#*=}"
        shift
        ;;
    --resources=*)
        RESOURCES="${1#*=}"
        shift
        ;;
    --resources-prefix=*)
        RESOURCES_PREFIX="${1#*=}"
        shift
        ;;
    *)
        echo "ERROR: Invalid argument: $1"
        usage
        exit 1
        ;;
    esac
done

case "${COMMAND}" in
  apply)
    validate_global_args
    validate_apply_args
    apply
    ;;
  destroy)
    validate_global_args
    destroy
    ;;
*)
    echo "ERROR: Invalid argument: ${COMMAND}"
    usage
    exit 1
    ;;
esac
