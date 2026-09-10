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
    local RESOURCES ALL
    # No `|| true` around kubectl: an API error must fail the script, otherwise a
    # dead API reads as an empty result. Only grep's "no match" is tolerated.
    if [ "$SELECTOR_TYPE" == "prefix" ]; then
        ALL=$(kubectl get "$RESOURCE_TYPE" -o name)
        RESOURCES=$(echo "$ALL" | grep "$VALUE" || true)
    elif [ "$SELECTOR_TYPE" == "label" ]; then
        RESOURCES=$(kubectl get "$RESOURCE_TYPE" -l "$VALUE" -o name)
    else
        echo "Invalid selector type: $SELECTOR_TYPE"
        exit 1
    fi
    if [[ -n "$RESOURCES" ]]; then
        echo "Deleting $RESOURCE_TYPE:"
        echo "$RESOURCES" | awk '{print "  - " $1}'
        # --ignore-not-found: deleting a Project takes its namespace with it.
        echo "$RESOURCES" | xargs -r kubectl delete --ignore-not-found
    else
        echo "No $RESOURCE_TYPE found with selector type $SELECTOR_TYPE and value $VALUE"
    fi
}

# Cluster-scoped objects whose FIELD names a namespace of this run: the
# reference outlives the namespace and is the only handle left on them.
names_referencing_e2e_namespace() {
    local RESOURCE_TYPE=$1 FIELD=$2
    kubectl get "$RESOURCE_TYPE" -o custom-columns="NAME:.metadata.name,NS:$FIELD" --no-headers |
        awk -v prefix="${E2E_NS_PREFIX}-" '$2 ~ "^" prefix { print $1 }'
}

# A variable, not a predicate: `set -e` is off inside an `if` condition, so a
# failed lookup there would read as "types not served" instead of failing.
SNAPSHOT_RECYCLE_BIN_AVAILABLE=false
detect_snapshot_recycle_bin() {
    local TYPES
    TYPES=$(kubectl api-resources -o name)
    if grep -qx "objectkeepers.deckhouse.io" <<<"$TYPES" &&
        grep -qx "volumesnapshotcontents.snapshot.storage.k8s.io" <<<"$TYPES"; then
        SNAPSHOT_RECYCLE_BIN_AVAILABLE=true
    fi
}

# state-snapshotter keeps a deleted VirtualDiskSnapshot recoverable for 30 days
# through a cluster-scoped ObjectKeeper chain that no label or namespace pass
# matches. On a pool cluster that is only garbage pinning storage snapshots.
delete_snapshot_recycle_bin() {
    local KEEPERS CONTENTS
    if [[ "$SNAPSHOT_RECYCLE_BIN_AVAILABLE" != true ]]; then
        echo "Cluster does not serve the snapshot recycle bin types, skipping"
        return 0
    fi

    # Release the Retain policy first, or deleting the chain orphans the snapshot
    # in the storage backend.
    CONTENTS=$(names_referencing_e2e_namespace volumesnapshotcontents .spec.volumeSnapshotRef.namespace)
    if [[ -n "$CONTENTS" ]]; then
        echo "Releasing retained volumesnapshotcontents:"
        echo "$CONTENTS" | awk '{print "  - " $1}'
        echo "$CONTENTS" | xargs -r -n1 kubectl patch volumesnapshotcontent \
            --type=merge -p '{"spec":{"deletionPolicy":"Delete"}}'
    fi

    KEEPERS=$(names_referencing_e2e_namespace objectkeepers.deckhouse.io .spec.followObjectRef.namespace)
    if [[ -n "$KEEPERS" ]]; then
        echo "Deleting objectkeepers:"
        echo "$KEEPERS" | awk '{print "  - " $1}'
        echo "$KEEPERS" | xargs -r kubectl delete objectkeepers.deckhouse.io --ignore-not-found
    else
        echo "No objectkeepers found for ${E2E_NS_PREFIX}-* namespaces"
    fi

    # Whatever the keeper cascade did not take goes directly.
    CONTENTS=$(names_referencing_e2e_namespace volumesnapshotcontents .spec.volumeSnapshotRef.namespace)
    if [[ -n "$CONTENTS" ]]; then
        echo "Deleting leftover volumesnapshotcontents:"
        echo "$CONTENTS" | awk '{print "  - " $1}'
        echo "$CONTENTS" | xargs -r kubectl delete volumesnapshotcontent --ignore-not-found
    fi
}

E2E_PREFIX="${E2E_PREFIX:-v12n-$(git rev-parse --short=5 HEAD)}"
E2E_LABEL="v12n-e2e"
# Must match framework.NamespaceBasePrefix; same value as the label, but not the same thing.
E2E_NS_PREFIX="v12n-e2e"

echo "Using E2E_PREFIX: $E2E_PREFIX"
echo "Using E2E_LABEL: $E2E_LABEL"

delete_resources_with_prefix_or_label "prefix" "$E2E_PREFIX" "projects"
delete_resources_with_prefix_or_label "prefix" "$E2E_PREFIX" "namespaces"
readarray -t CLEANUP_RESOURCES < <(yq '.cleanupResources[]' default_config.yaml)
for RESOURCE in "${CLEANUP_RESOURCES[@]}"; do
    delete_resources_with_prefix_or_label "prefix" "$E2E_PREFIX" "$RESOURCE"
done

delete_resources_with_prefix_or_label "label" "$E2E_LABEL" "projects"
delete_resources_with_prefix_or_label "label" "$E2E_LABEL" "namespaces"
readarray -t CLEANUP_RESOURCES < <(yq '.cleanupResources[]' default_config.yaml)
for RESOURCE in "${CLEANUP_RESOURCES[@]}"; do
    delete_resources_with_prefix_or_label "label" "$E2E_LABEL" "$RESOURCE"
done

# After the object passes: a discovery failure must not cost the deletion.
detect_snapshot_recycle_bin
delete_snapshot_recycle_bin

# Verify instead of trusting the selectors: a Project namespace carries no e2e
# label, so only the Project pass can remove it. No `|| true` here either.
# A Project's namespace goes asynchronously, so wait before failing.
verify_nothing_left() {
    local wait_seconds=180 interval=10
    local deadline=$((SECONDS + wait_seconds))
    local all namespaces projects keepers contents left

    while :; do
        all=$(kubectl get namespaces -o name)
        namespaces=$(echo "$all" | grep "^namespace/${E2E_NS_PREFIX}-" || true)
        projects=$(kubectl get projects -l "$E2E_LABEL" -o name)
        keepers=""
        contents=""
        if [[ "$SNAPSHOT_RECYCLE_BIN_AVAILABLE" == true ]]; then
            keepers=$(names_referencing_e2e_namespace objectkeepers.deckhouse.io .spec.followObjectRef.namespace)
            contents=$(names_referencing_e2e_namespace volumesnapshotcontents .spec.volumeSnapshotRef.namespace)
        fi

        if [[ -z "$namespaces" && -z "$projects" && -z "$keepers" && -z "$contents" ]]; then
            echo "Verified: no e2e namespaces, projects or retained snapshots left"
            return 0
        fi

        if (( SECONDS >= deadline )); then
            echo "Cleanup left e2e objects behind after ${wait_seconds}s:" >&2
            for left in "$projects" "$namespaces" "$keepers" "$contents"; do
                if [[ -n "$left" ]]; then
                    echo "$left" | awk '{print "  - " $1}' >&2
                fi
            done
            return 1
        fi

        sleep "$interval"
    done
}

verify_nothing_left

# Deleting the suite's objects is not the same as handing back a usable cluster:
# whatever leaves deckhouse down, the lease must not go back as reusable. Give it
# a moment first, a rolling restart is normal.
verify_platform_healthy() {
    local wait_seconds=120 interval=10
    local deadline=$((SECONDS + wait_seconds))
    local pods not_ready

    while :; do
        pods=$(kubectl -n d8-system get pods -l app=deckhouse \
            -o jsonpath='{range .items[*]}{.metadata.name}{" "}{.status.containerStatuses[?(@.name=="deckhouse")].ready}{"\n"}{end}')
        not_ready=$(awk 'NF && $2 != "true" { print $1 }' <<<"$pods")

        if [[ -z "$not_ready" && -n "$pods" ]]; then
            echo "Verified: deckhouse is ready"
            return 0
        fi

        if ((SECONDS >= deadline)); then
            if [[ -z "$pods" ]]; then
                echo "No deckhouse pod found in d8-system after ${wait_seconds}s" >&2
                return 1
            fi
            echo "Deckhouse is not ready after ${wait_seconds}s, the cluster cannot serve the next run:" >&2
            echo "$not_ready" | awk '{print "  - " $1}' >&2
            echo "Why it died: kubectl -n d8-system logs <pod> -c deckhouse --previous" >&2
            return 1
        fi

        sleep "$interval"
    done
}

verify_platform_healthy
