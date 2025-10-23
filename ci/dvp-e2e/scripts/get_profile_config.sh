#!/bin/bash
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

# Use jq to find profile by name or alias
PROFILE_CONFIG=$(jq -r --arg profile "$PROFILE" '
  .[] |
  select(.name == $profile or (.aliases[]? | . == $profile)) |
  "\(.storage_class)|\(.image_storage_class)|\(.snapshot_storage_class)|\(.worker_data_disk_size // "10Gi")"
' "$PROFILES_FILE")

if [[ -z "$PROFILE_CONFIG" || "$PROFILE_CONFIG" == "null" ]]; then
  echo "Profile '$PROFILE' not found in $PROFILES_FILE" >&2
  echo "Available profiles:" >&2
  jq -r '.[] | "  - \(.name) (aliases: \(.aliases | join(", ")))"' "$PROFILES_FILE" >&2
  exit 1
fi

# Split the result and export variables
IFS='|' read -r SC IMG_SC SNAP_SC ATTACH_SIZE <<< "$PROFILE_CONFIG"

echo "STORAGE_CLASS=$SC"
echo "IMAGE_STORAGE_CLASS=$IMG_SC"
echo "SNAPSHOT_STORAGE_CLASS=$SNAP_SC"
echo "ATTACH_DISK_SIZE=$ATTACH_SIZE"
