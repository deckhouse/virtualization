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
# Apply patches from patches/ to each kernel version tree in .out/
# Usage: ./apply-patches.sh [out-dir]
# Default out-dir is .out (relative to script directory)

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OUT_DIR="${1:-.out}"
PATCHES_DIR="${SCRIPT_DIR}/patches"

cd "$SCRIPT_DIR"
OUT_DIR="$(realpath "$OUT_DIR")"

if [[ ! -d "$OUT_DIR" ]]; then
  echo "Error: output directory '$OUT_DIR' does not exist" >&2
  exit 1
fi

if [[ ! -d "$PATCHES_DIR" ]]; then
  echo "Error: patches directory '$PATCHES_DIR' does not exist" >&2
  exit 1
fi

for version_dir in "$OUT_DIR"/*/; do
  [[ -d "$version_dir" ]] || continue
  version="$(basename "$version_dir")"
  for patch in "$PATCHES_DIR"/*.patch; do
    [[ -f "$patch" ]] || continue
    echo "Applying $(basename "$patch") to $version..."
    if ! patch -d "$version_dir" -p0 --forward --silent < "$patch"; then
      if patch -d "$version_dir" -p0 --reverse --check --silent < "$patch" 2>/dev/null; then
        echo "  (already applied, skipping)"
      else
        echo "  FAILED" >&2
        exit 1
      fi
    fi
  done
done

echo "Done."
