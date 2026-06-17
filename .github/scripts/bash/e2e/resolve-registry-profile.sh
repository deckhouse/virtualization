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

# Resolves the active registry profile into CI outputs.
#
# Reads the profile file (registry-profile.env) and emits the registry profile,
# the Deckhouse release channel, and the Deckhouse install image tag derived
# from the profile. Writes to GITHUB_OUTPUT when running in GitHub Actions and
# always prints the resolved values for logs.

set -Eeuo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=.github/scripts/bash/e2e/common.sh
source "${SCRIPT_DIR}/common.sh"

PROFILE_FILE="${REGISTRY_PROFILE_FILE:-${SCRIPT_DIR}/profile/registry-profile.env}"

if [ ! -f "$PROFILE_FILE" ]; then
  echo "[ERROR] Registry profile file not found: ${PROFILE_FILE}" >&2
  exit 1
fi

# shellcheck source=.github/scripts/bash/e2e/profile/registry-profile.env
# shellcheck disable=SC1091 # path is resolved at runtime
source "$PROFILE_FILE"

require_env REGISTRY_PROFILE
require_env DECKHOUSE_CHANNEL

case "$REGISTRY_PROFILE" in
  prod)
    deckhouse_version="$DECKHOUSE_CHANNEL"
    ;;
  stage)
    require_env STAGE_DECKHOUSE_VERSION
    deckhouse_version="$STAGE_DECKHOUSE_VERSION"
    ;;
  *)
    echo "[ERROR] Unknown REGISTRY_PROFILE='${REGISTRY_PROFILE}' (expected 'prod' or 'stage')" >&2
    exit 1
    ;;
esac

set_output() {
  local key="$1"
  local value="$2"
  echo "${key}=${value}"
  if [ -n "${GITHUB_OUTPUT:-}" ]; then
    echo "${key}=${value}" >> "$GITHUB_OUTPUT"
  fi
}

echo "[INFO] Resolved registry profile '${REGISTRY_PROFILE}'"
set_output registry_profile "$REGISTRY_PROFILE"
set_output deckhouse_channel "$DECKHOUSE_CHANNEL"
set_output deckhouse_version "$deckhouse_version"
