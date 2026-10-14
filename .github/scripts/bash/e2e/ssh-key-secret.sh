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

# Keeps the nested-cluster SSH keypair in the Secret "ssh-key" of the test namespace.
#
# A rerun of failed jobs reuses the namespace and its VMs, but runs on a fresh runner
# where ssh-gen would create a new keypair. cloud-init writes authorized_keys only on
# the first boot, so the VMs keep the old key and dhctl cannot log in.
#
#   restore: copy the keypair from the Secret into <ssh-dir> (no-op if there is none),
#            so ssh-gen skips and the VMs get the key they already trust.
#   save:    store the keypair from <ssh-dir> into the Secret (create or update).

set -Eeuo pipefail

if [ "$#" -ne 3 ]; then
  echo "[ERROR] Usage: $0 <restore|save> <namespace> <ssh-dir>" >&2
  exit 1
fi

MODE="$1"
NAMESPACE="$2"
SSH_DIR="$3"
SECRET_NAME="ssh-key"
PRIV_KEY_FILE="${SSH_DIR}/cloud"
PUB_KEY_FILE="${SSH_DIR}/cloud.pub"

case "${MODE}" in
  restore)
    if ! kubectl get namespace "${NAMESPACE}" >/dev/null 2>&1; then
      echo "[INFO] Namespace ${NAMESPACE} does not exist yet, a new SSH keypair will be generated"
      exit 0
    fi
    if ! kubectl -n "${NAMESPACE}" get secret "${SECRET_NAME}" >/dev/null 2>&1; then
      echo "[INFO] Secret ${NAMESPACE}/${SECRET_NAME} not found, a new SSH keypair will be generated"
      exit 0
    fi

    mkdir -p "${SSH_DIR}"
    rm -f "${PRIV_KEY_FILE}" "${PUB_KEY_FILE}"
    (
      umask 077
      kubectl -n "${NAMESPACE}" get secret "${SECRET_NAME}" -o jsonpath='{.data.cloud}' | base64 -d > "${PRIV_KEY_FILE}"
    )
    if [ ! -s "${PRIV_KEY_FILE}" ]; then
      echo "[ERROR] Secret ${NAMESPACE}/${SECRET_NAME} has no 'cloud' key" >&2
      exit 1
    fi
    # Secrets saved before cloud.pub was stored hold only the private key. Whether
    # ssh-keygen -y prints the comment depends on the OpenSSH version, so set it here.
    ssh-keygen -y -f "${PRIV_KEY_FILE}" | awk '{print $1, $2, "cloud"}' > "${PUB_KEY_FILE}"
    chmod 0600 "${PUB_KEY_FILE}"
    chmod 0400 "${PRIV_KEY_FILE}"
    echo "[INFO] Restored SSH keypair from Secret ${NAMESPACE}/${SECRET_NAME}"
    ;;
  save)
    if [ ! -f "${PRIV_KEY_FILE}" ] || [ ! -f "${PUB_KEY_FILE}" ]; then
      echo "[WARN] No SSH keypair in ${SSH_DIR}, nothing to save"
      exit 0
    fi
    if ! kubectl get namespace "${NAMESPACE}" >/dev/null 2>&1; then
      echo "[WARN] Namespace ${NAMESPACE} does not exist, nothing to save"
      exit 0
    fi

    kubectl -n "${NAMESPACE}" create secret generic "${SECRET_NAME}" \
      --from-file=cloud="${PRIV_KEY_FILE}" \
      --from-file=cloud.pub="${PUB_KEY_FILE}" \
      --dry-run=client -o yaml | kubectl apply -f - >/dev/null
    echo "[INFO] Saved SSH keypair to Secret ${NAMESPACE}/${SECRET_NAME}"
    ;;
  *)
    echo "[ERROR] Unknown mode: ${MODE}, expected restore or save" >&2
    exit 1
    ;;
esac
