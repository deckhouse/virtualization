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

# Check that every component image referenced by a module tag is pullable.
# Reads images_digests.json from the module image and verifies each digest.
#
# Usage: check-image-digests.sh <repo> <tag>
# Exit:  0 all pullable, 1 some missing, 2 module image not found.

set -Eeuo pipefail

R="${1:?usage: check-image-digests.sh <repo> <tag>}"
TAG="${2:?usage: check-image-digests.sh <repo> <tag>}"

if ! crane digest "${R}:${TAG}" >/dev/null 2>&1; then
  echo "::error::module image ${R}:${TAG} not found"
  exit 2
fi

json="$(crane export "${R}:${TAG}" - | tar -Oxf - images_digests.json)"

missing=0
while read -r name digest; do
  if crane digest "${R}@${digest}" >/dev/null 2>&1; then
    echo "OK    ${name}"
  else
    echo "MISS  ${name}  ${digest}"
    missing=$((missing + 1))
  fi
done < <(echo "${json}" | jq -r 'to_entries[] | "\(.key) \(.value)"')

echo "${missing} component image(s) missing"
[ "${missing}" -eq 0 ]
