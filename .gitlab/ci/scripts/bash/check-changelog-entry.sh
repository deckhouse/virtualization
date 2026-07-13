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

# Thin bash wrapper around check_changelog_entry.py.
#
# Allows the job yml to call a single bash script while keeping the
# actual validation logic in Python.
#
# Picks the first available interpreter: python3, python.
# Required environment is documented in check_changelog_entry.py.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

if command -v python3 >/dev/null 2>&1; then
  PYTHON_BIN=python3
elif command -v python >/dev/null 2>&1; then
  PYTHON_BIN=python
else
  echo "ERROR: neither python3 nor python is installed on the runner." >&2
  exit 1
fi

exec "${PYTHON_BIN}" "${SCRIPT_DIR}/../python/check_changelog_entry.py"
