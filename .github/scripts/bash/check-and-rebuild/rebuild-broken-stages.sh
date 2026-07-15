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

# Delete stale stages so the next werf build rebuilds broken component images.
# Usage: rebuild-broken-stages.sh --repo <repo> --report <build-report.json>

set -Eeuo pipefail

repo=""
report=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo) repo="$2"; shift 2 ;;
    --report) report="$2"; shift 2 ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
done
[[ -n "$repo" && -n "$report" ]] || { echo "usage: $0 --repo <repo> --report <file>" >&2; exit 2; }
[[ -f "$report" ]] || { echo "::error::report ${report} not found"; exit 1; }

# Warn (don't fail) if the tag is already gone.
delete_tag() {
  crane delete "${repo}:$1" && echo "  deleted ${repo}:$1" || echo "::warning::failed to delete $1"
}

# A GC'd component keeps a resolvable stage tag, so werf reuses the stage and
# re-reports the dead digest. The dead digest can't be deleted, but the tag can:
# drop every stage by tag so werf rebuilds and re-pushes a live image.
while read -r img; do
  final="$(jq -r --arg i "$img" '.Images[$i].DockerImageDigest // empty' "$report")"
  [[ -n "$final" ]] || continue
  crane digest "${repo}@${final}" >/dev/null 2>&1 && continue
  echo "broken: ${img} — deleting its stages"
  while read -r t; do
    [[ -n "$t" ]] || continue
    delete_tag "$t"
  done < <(jq -r --arg i "$img" '.Images[$i].Stages[].DockerTag // empty' "$report")
done < <(jq -r '.Images | to_entries[] | select(.value.Final == true) | .key' "$report")

# Regenerate the digests chain so fresh component digests reach the module :tag.
for img in images-digests prepare-bundle; do
  t="$(jq -r --arg i "$img" '.Images[$i].DockerTag // empty' "$report")"
  [[ -n "$t" ]] && delete_tag "$t" || echo "::warning::no tag for ${img} in report"
done
