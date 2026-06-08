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

set -Eeuo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=.github/scripts/bash/e2e/common.sh
source "${SCRIPT_DIR}/common.sh"

if [ "$#" -ne 1 ]; then
  echo "[ERROR] Usage: $0 <github-output>" >&2
  exit 1
fi

github_output="$1"

version_json="$(kubectl version -o json)"
server_version="$(echo "${version_json}" | jq -r '.serverVersion.gitVersion')"
server_major="$(echo "${version_json}" | jq -r '.serverVersion.major' | tr -cd '0-9')"
server_minor="$(echo "${version_json}" | jq -r '.serverVersion.minor' | tr -cd '0-9')"

if [[ -z "${server_major}" || -z "${server_minor}" ]]; then
  echo "[ERROR] Failed to parse Kubernetes server version: ${server_version}"
  exit 1
fi

label_filter=""
usb_supported=false

if (( server_major > 1 || (server_major == 1 && server_minor >= 34) )); then
  usb_supported=true
  echo "[INFO] Kubernetes server version ${server_version} supports USB E2E tests"
else
  label_filter="!usb-precheck"
  echo "[INFO] Kubernetes server version ${server_version} does not support USB E2E tests"
  echo "[INFO] USB-labeled specs will be excluded with label filter: ${label_filter}"
fi

{
  echo "server-version=${server_version}"
  echo "usb-supported=${usb_supported}"
  echo "label-filter=${label_filter}"
} >> "${github_output}"
