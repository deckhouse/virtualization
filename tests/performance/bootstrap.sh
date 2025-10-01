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
                  (Required) --count, -c: count of virtual machines to create.
                  (Optional) --namespace, -n: namespace for virtual machines. If not defined - using default namespace.
                  (Optional) --storage-class, -s: storage-class for virtual machine disks. If not defined - using default SC.
                  (Optional) --resources-prefix, -p (default: performance): prefix to be used for resource names.
  ---
  destroy         Destroy set of virtual machines.

Global Arguments:
  --name, -r (default: performance): name for release of virtual machine.
  --resources, -R (default: 'all'): resources to manage. Possible values: 'vds', 'vms' or 'all'.

Examples:
  Bootstrap:
    $(basename "$0") apply --count=1
    $(basename "$0") apply -c 1 -n default -s ceph-pool-r2-csi-rbd
    $(basename "$0") apply --resources=vds --count=1 --namespace=default --storage-class=default
    $(basename "$0") destroy --resources=vds --namespace=default
    $(basename "$0") destroy -R vds -n default
EOF
}

function handle_exit() {
  for p in $(jobs -p); do pgrep -P "${p}" | xargs kill -9 ; done
}

function validate_global_args() {
  if [ "${RESOURCES}" != "all" ] && [ "${RESOURCES}" != "vms" ] && [ "${RESOURCES}" != "vds" ]; then
    echo "ERROR: Invalid --resources flag: allowed values 'vds', 'vms' or 'all'"
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

  args=( upgrade --install "${RELEASE_NAME}" . -n "${NAMESPACE}" --create-namespace --set "count=${COUNT}" --set "resourcesPrefix=${RESOURCES_PREFIX}" --set "resources.default=${RESOURCES}" )
  if [ -n "${STORAGE_CLASS}" ]; then
      args+=( --set "resources.storageClassName=${STORAGE_CLASS}" )
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
    --count=*|-c=*)
        COUNT="${1#*=}"
        shift
        ;;
    -c)
        COUNT="$2"
        shift 2
        ;;
    --namespace=*|-n=*)
        NAMESPACE="${1#*=}"
        shift
        ;;
    -n)
        NAMESPACE="$2"
        shift 2
        ;;
    --storage-class=*|-s=*)
        STORAGE_CLASS="${1#*=}"
        shift
        ;;
    -s)
        STORAGE_CLASS="$2"
        shift 2
        ;;
    --name=*|-r=*)
        RELEASE_NAME="${1#*=}"
        shift
        ;;
    -r)
        RELEASE_NAME="$2"
        shift 2
        ;;
    --resources=*|-R=*)
        RESOURCES="${1#*=}"
        shift
        ;;
    -R)
        RESOURCES="$2"
        shift 2
        ;;
    --resources-prefix=*|-p=*)
        RESOURCES_PREFIX="${1#*=}"
        shift
        ;;
    -p)
        RESOURCES_PREFIX="$2"
        shift 2
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
