#!/usr/bin/env bash

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
set -euo pipefail

# Usage:
#   build_parent_kubeconfig.sh -o /path/to/kubeconfig -a https://api.server -t <token>

out=""
api="${E2E_K8S_URL:-}"
tok="${E2E_SA_TOKEN:-}"

while getopts ":o:a:t:" opt; do
  case $opt in
    o) out="$OPTARG" ;;
    a) api="$OPTARG" ;;
    t) tok="$OPTARG" ;;
    *) echo "Usage: $0 -o <kubeconfig_path> -a <api_url> -t <token>" >&2; exit 2 ;;
  esac
done

if [ -z "${out}" ] || [ -z "${api}" ] || [ -z "${tok}" ]; then
  echo "Usage: $0 -o <kubeconfig_path> -a <api_url> -t <token>" >&2
  exit 2
fi

mkdir -p "$(dirname "$out")"
cat >"$out" <<EOF
apiVersion: v1
kind: Config
clusters:
- cluster:
    server: ${api}
    insecure-skip-tls-verify: true
  name: parent
contexts:
- context:
    cluster: parent
    user: sa
  name: parent
current-context: parent
users:
- name: sa
  user:
    token: "${tok}"
EOF
chmod 600 "$out"
echo "KUBECONFIG=$out"
