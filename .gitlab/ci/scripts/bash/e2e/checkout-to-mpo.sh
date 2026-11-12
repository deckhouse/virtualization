#!/usr/bin/env bash

# Copyright 2026 Flant JSC
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

# Points the virtualization ModulePullOverride at the module image tag given as
# the first argument and waits until every workload of the module runs the
# images of exactly that build. CI puts the module under test on a cluster with
# it before the e2e suite and rolls the cluster back after.

set -euo pipefail

tag="${1:?usage: checkout-to-mpo.sh <module image tag>}"
repo="dev-registry.deckhouse.io/sys/deckhouse-oss/modules/virtualization"
image="$repo:$tag"

digest=$(crane digest "$image")
digests=$(mktemp)
trap 'rm -f "$digests"' EXIT
crane export "$image" - | tar -Oxf - images_digests.json | jq -r '.[]' > "$digests"

kubectl patch mpo virtualization --type merge -p "{\"spec\":{\"imageTag\":\"$tag\"}}"
kubectl wait mpo/virtualization --for=jsonpath='{.status.imageDigest}'="$digest" --timeout=10m

# The ModulePullOverride reports the digest before Deckhouse restarts, and only
# the restarted Deckhouse renders the workloads of the new build. So wait until
# no workload references a module image outside this build, then for the rollout.
stale_images() {
  kubectl -n d8-virtualization get deploy,ds -o json |
    jq -r --arg repo "$repo@" '.items[].spec.template.spec
      | (.containers[], .initContainers[]?) | .image
      | select(startswith($repo)) | sub(".*@"; "")' |
    sort -u | grep -vxF -f "$digests" || true
}

deadline=$((SECONDS + 600))
until [[ -z "$(stale_images)" ]]; do
  if (( SECONDS >= deadline )); then
    echo "workloads in d8-virtualization still reference images outside $tag:" >&2
    stale_images >&2
    exit 1
  fi
  sleep 10
done

kubectl -n d8-virtualization get deploy,ds -o name |
  xargs -r -n1 kubectl -n d8-virtualization rollout status --timeout=10m
