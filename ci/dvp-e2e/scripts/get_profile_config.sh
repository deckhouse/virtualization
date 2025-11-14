#!/bin/bash

# Copyright 2025 Flant JSC
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

# Script to get storage class configuration from profiles.json
# Usage: get_profile_config.sh <profile_name>

set -euo pipefail

PROFILE="${1:-}"
PROFILES_FILE="${2:-./profiles.json}"

if [[ -z "$PROFILE" ]]; then
  echo "Usage: $0 <profile_name> [profiles_file]" >&2
  exit 1
fi

if [[ ! -f "$PROFILES_FILE" ]]; then
  echo "Profiles file not found: $PROFILES_FILE" >&2
  exit 1
fi

# Use jq to find profile by exact name only
PROFILE_CONFIG=$(jq -r --arg profile "$PROFILE" '
  .[] | select(.name == $profile) |
  "\(.storage_class)|\(.image_storage_class)|\(.snapshot_storage_class)|\(.worker_data_disk_size // "10Gi")"
' "$PROFILES_FILE")

if [[ -z "$PROFILE_CONFIG" || "$PROFILE_CONFIG" == "null" ]]; then
  echo "Profile '$PROFILE' not found in $PROFILES_FILE" >&2
  echo "Available profiles:" >&2
  jq -r '.[] | "  - \(.name)"' "$PROFILES_FILE" >&2
  exit 1
fi

# Split the result and export variables
IFS='|' read -r SC IMG_SC SNAP_SC ATTACH_SIZE <<< "$PROFILE_CONFIG"

echo "STORAGE_CLASS=$SC"
echo "IMAGE_STORAGE_CLASS=$IMG_SC"
echo "SNAPSHOT_STORAGE_CLASS=$SNAP_SC"
echo "ATTACH_DISK_SIZE=$ATTACH_SIZE"
