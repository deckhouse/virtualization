#!/bin/bash

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

# Checks that no workspace module carries replace directives: go.work is the
# single source of truth for them. A replace in a module go.mod applies to
# the whole workspace and silently skews dependency resolution, and image
# builds run in workspace mode where only go.work is honored consistently.

set -euo pipefail

cd "$(dirname "$0")/.."

fail=0
for dir in $(go work edit -json | jq -r '.Use[].DiskPath'); do
  mod="$dir/go.mod"
  while IFS=$'\t' read -r old new; do
    [ -z "$old" ] && continue
    echo "REPLACE in $mod: $old => $new (move it to go.work)"
    fail=1
  done < <(go mod edit -json "$mod" | jq -r '.Replace[]? | [.Old.Path, "\(.New.Path)@\(.New.Version // "local")"] | @tsv')
done

if [ "$fail" -ne 0 ]; then
  echo
  echo "Module go.mod files must not contain replace directives; declare them in go.work."
  exit 1
fi

echo "OK: no replace directives in workspace module go.mod files"
