#!/usr/bin/env bash

set -Eeuo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=.github/scripts/bash/e2e/common.sh
source "${SCRIPT_DIR}/common.sh"

clusternetworks_diag() {
  kubectl -n d8-sdn get pods,svc,endpoints || true
  kubectl get clusternetworks.network.deckhouse.io || true
}

kubectl_apply_with_retry 12 10 clusternetworks_diag
