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
# Usage: check-image-digests.sh --repo <repo> --tag <tag>
# Exit:  0 all pullable, 1 some missing, 2 module image not found.

set -Eeuo pipefail

repo=""
tag=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo) repo="$2"; shift 2 ;;
    --tag) tag="$2"; shift 2 ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
done
[[ -n "$repo" && -n "$tag" ]] || { echo "usage: $0 --repo <repo> --tag <tag>" >&2; exit 2; }

echo "[INFO] Repo ${repo}"
echo "[INFO] Tag  ${tag}"

if ! crane digest "${repo}:${tag}" >/dev/null 2>&1; then
  echo "::error::module image ${repo}:${tag} not found"
  exit 2
fi

digests="$(crane export "${repo}:${tag}" - | tar -Oxf - images_digests.json)"

ok_images=()
missing_images=()
while read -r component digest; do
  image="${repo}@${digest}"
  if crane digest "$image" >/dev/null 2>&1; then
    echo "✅ ${component}"
    ok_images+=("$(printf '%-25s %s' "${component}:" "$image")")
  else
    echo "❌ ${component}"
    missing_images+=("$(printf '%-25s %s' "${component}:" "$image")")
  fi
done < <(echo "${digests}" | jq -r 'to_entries[] | "\(.key) \(.value)"')

echo "──────────────────────────────"
if [[ ${#missing_images[@]} -eq 0 ]]; then
  echo "🎉 All ${#ok_images[@]} component images are pullable"
  exit 0
fi

echo "⚠️  Missing component images:"
for img in "${missing_images[@]}"; do
  echo "  - $img"
done
echo "Total missing: ${#missing_images[@]}"
exit 1
