#!/usr/bin/env bash
# Copyright 2026 Flant JSC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Runs golangci-lint in every directory that ships a .golangci.yaml.
#
# Executed inside the golangci-lint image by the lint:go job, with the
# repository mounted at the working directory. Excludes the same upstream /
# vendored modules as .github/workflows/dev_module_build.yml (lint_go):
# images/cdi-cloner/cloner-startup, images/dvcr-artifact. The GH prune of
# ./test/performance/shatal was a no-op (path never existed; the real module is
# test/performance/tools/shatal), so shatal is intentionally linted here,
# matching actual current behavior.

set -euo pipefail

mapfile -t config_dirs < <(
  find . \
    -path ./images/cdi-cloner/cloner-startup -prune -o \
    -path ./images/dvcr-artifact -prune -o \
    -type f -name '.golangci.yaml' -not -type l -printf '%h\0' |
    xargs -0 -n1 | sort -u
)

echo "Found ${#config_dirs[@]} directories with golangci-lint configurations"

error_count=0
for dir in "${config_dirs[@]}"; do
  echo "------------------------------------------------------------"
  echo "Linting: ${dir}"
  pushd "$dir" >/dev/null
  if ! golangci-lint run; then
    error_count=$((error_count + 1))
  fi
  popd >/dev/null
done

if [ "$error_count" -gt 0 ]; then
  echo "${error_count} directory/directories failed golangci-lint"
  exit 1
fi
echo "All golangci-lint checks passed"
