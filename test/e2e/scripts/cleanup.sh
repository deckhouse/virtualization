#!/usr/bin/env bash

# Copyright 2025 Flant JSC
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

set -euo pipefail

delete_resources_with_prefix_or_label() {
    local SELECTOR_TYPE=$1
    local VALUE=$2
    local RESOURCE_TYPE=$3
    local RESOURCES
    if [ "$SELECTOR_TYPE" == "prefix" ]; then
        RESOURCES=$(kubectl get "$RESOURCE_TYPE" -o name | grep "$VALUE" || true)
    elif [ "$SELECTOR_TYPE" == "label" ]; then
        RESOURCES=$(kubectl get "$RESOURCE_TYPE" -l "$VALUE" -o name || true)
    else
        echo "Invalid selector type: $SELECTOR_TYPE"
        exit 1
    fi
    if [[ -n "$RESOURCES" ]]; then
        echo "Deleting $RESOURCE_TYPE:"
        echo "$RESOURCES" | awk '{print "  - " $1}'
        echo "$RESOURCES" | xargs -r echo kubectl delete
    else
        echo "No $RESOURCE_TYPE found with selector type $SELECTOR_TYPE and value $VALUE"
    fi
}

E2E_PREFIX="${E2E_PREFIX:-v12n-$(git rev-parse --short=5 HEAD)}"
E2E_LABEL="${E2E_LABEL:-v12n-e2e}"

echo "Using E2E_PREFIX: $E2E_PREFIX"
echo "Using E2E_LABEL: $E2E_LABEL"


delete_resources_with_prefix_or_label "prefix" "$E2E_PREFIX" "namespaces"

delete_resources_with_prefix_or_label "prefix" "$E2E_PREFIX" "projects"

readarray -t CLEANUP_RESOURCES < <(yq '.cleanupResources[]' default_config.yaml)
for RESOURCE in "${CLEANUP_RESOURCES[@]}"; do
    delete_resources_with_prefix_or_label "prefix" "$E2E_PREFIX" "$RESOURCE"
done

readarray -t CLEANUP_RESOURCES < <(yq '.cleanupResources[]' default_config.yaml)
for RESOURCE in "${CLEANUP_RESOURCES[@]}"; do
    delete_resources_with_prefix_or_label "label" "$E2E_LABEL" "$RESOURCE"
done


