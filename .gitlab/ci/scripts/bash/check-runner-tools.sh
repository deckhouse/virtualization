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

set -euo pipefail

if [[ "$#" -eq 0 ]]; then
  echo "Usage: $0 <tool> [<tool>...]" >&2
  exit 2
fi

missing=()
for tool in "$@"; do
  if ! command -v "$tool" >/dev/null 2>&1; then
    missing+=("$tool")
  fi
done

if [[ "${#missing[@]}" -gt 0 ]]; then
  echo "ERROR: required runner tool(s) are missing: ${missing[*]}" >&2
  echo "This pipeline is designed for GitLab Runner shell executor." >&2
  echo "Container images and package-manager installs are not used by these jobs; install the tools on the runner host." >&2
  exit 1
fi

printf 'Runner tools OK:'
printf ' %s' "$@"
printf '\n'
