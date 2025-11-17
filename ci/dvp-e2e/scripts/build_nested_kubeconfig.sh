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
#   build_nested_kubeconfig.sh -o /path/to/kubeconfig -n namespace -d domain -k parent_kubeconfig -s ssh_key -u user

out=""
namespace=""
domain=""
parent_kubeconfig=""
ssh_key=""
ssh_user="ubuntu"

while getopts ":o:n:d:k:s:u:" opt; do
  case $opt in
    o) out="$OPTARG" ;;
    n) namespace="$OPTARG" ;;
    d) domain="$OPTARG" ;;
    k) parent_kubeconfig="$OPTARG" ;;
    s) ssh_key="$OPTARG" ;;
    u) ssh_user="$OPTARG" ;;
    *) 
      echo "Usage: $0 -o <output_kubeconfig> -n <namespace> -d <domain> -k <parent_kubeconfig> -s <ssh_key> [-u <ssh_user>]" >&2
      exit 2 
      ;;
  esac
done

if [ -z "${out}" ] || [ -z "${namespace}" ] || [ -z "${domain}" ] || [ -z "${parent_kubeconfig}" ] || [ -z "${ssh_key}" ]; then
  echo "Usage: $0 -o <output_kubeconfig> -n <namespace> -d <domain> -k <parent_kubeconfig> -s <ssh_key> [-u <ssh_user>]" >&2
  exit 2
fi

if [ ! -s "${parent_kubeconfig}" ]; then
  echo "[ERR] parent kubeconfig not found at ${parent_kubeconfig}" >&2
  exit 1
fi

if [ ! -f "${ssh_key}" ]; then
  echo "[ERR] SSH key not found at ${ssh_key}" >&2
  exit 1
fi

# Create output directory
OUT_DIR="$(dirname "$out")"
if ! mkdir -p "${OUT_DIR}"; then
  echo "[ERR] Failed to create output directory: ${OUT_DIR}" >&2
  exit 1
fi

# Find master VM
echo "[INFO] Finding master VM in namespace ${namespace}..."
MASTER_NAME=$(KUBECONFIG="${parent_kubeconfig}" kubectl -n "${namespace}" get vm -l dvp.deckhouse.io/node-group=master -o jsonpath='{.items[0].metadata.name}')
if [ -z "$MASTER_NAME" ]; then
  echo "[ERR] master VM not found in namespace ${namespace}" >&2
  exit 1
fi
echo "[INFO] Found master VM: ${MASTER_NAME}"

# Get token via SSH
TOKEN_FILE="$(dirname "$out")/token.txt"
rm -f "$TOKEN_FILE"
SSH_OK=0

echo "[INFO] Obtaining token from nested cluster..."
for attempt in $(seq 1 6); do
  if KUBECONFIG="${parent_kubeconfig}" d8 v ssh \
    --username="${ssh_user}" \
    --identity-file="${ssh_key}" \
    --local-ssh=true \
    --local-ssh-opts="-o StrictHostKeyChecking=no" \
    --local-ssh-opts="-o UserKnownHostsFile=/dev/null" \
    "${MASTER_NAME}.${namespace}" \
    -c '
      set -euo pipefail
      SUDO="sudo /opt/deckhouse/bin/kubectl"
      $SUDO -n kube-system get sa e2e-admin >/dev/null 2>&1 || $SUDO -n kube-system create sa e2e-admin >/dev/null 2>&1
      $SUDO -n kube-system get clusterrolebinding e2e-admin >/dev/null 2>&1 || $SUDO -n kube-system create clusterrolebinding e2e-admin --clusterrole=cluster-admin --serviceaccount=kube-system:e2e-admin >/dev/null 2>&1
      for i in $(seq 1 10); do
        TOKEN=$($SUDO -n kube-system create token e2e-admin --duration=24h 2>/dev/null) && echo "$TOKEN" && break
        echo "[WARN] Failed to create token (attempt $i/10); retrying in 3s" >&2
        sleep 3
      done
      if [ -z "${TOKEN:-}" ]; then
        echo "[ERR] Unable to create token for e2e-admin after 10 attempts" >&2
        exit 1
      fi
    ' > "$TOKEN_FILE"; then
    SSH_OK=1
    break
  fi
  echo "[WARN] d8 v ssh attempt $attempt failed; retry in 15s..."
  sleep 15
done

if [ "$SSH_OK" -ne 1 ] || [ ! -s "$TOKEN_FILE" ]; then
  echo "[ERR] Failed to obtain nested token via d8 v ssh after multiple attempts" >&2
  cat "$TOKEN_FILE" 2>/dev/null || true
  exit 1
fi

NESTED_TOKEN=$(cat "$TOKEN_FILE")
SERVER_URL="https://api.${namespace}.${domain}"

# Generate kubeconfig
cat > "$out" <<EOF
apiVersion: v1
kind: Config
clusters:
- cluster:
    insecure-skip-tls-verify: true
    server: ${SERVER_URL}
  name: nested
contexts:
- context:
    cluster: nested
    user: e2e-admin
  name: nested
current-context: nested
users:
- name: e2e-admin
  user:
    token: ${NESTED_TOKEN}
EOF

chmod 600 "$out"
rm -f "$TOKEN_FILE"

echo "[INFO] Generated nested kubeconfig at ${out}"
