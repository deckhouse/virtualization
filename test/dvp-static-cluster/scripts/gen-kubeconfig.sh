#!/usr/bin/env bash

# Copyright 2025 Flant JSC
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

get_current_date() {
  date +"%H:%M:%S %d-%m-%Y"
}

get_timestamp() {
  date +%s
}

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log_info() {
  local message="$1"
  local timestamp
  timestamp=$(get_current_date)
  echo -e "${BLUE}[INFO]${NC} $message"
  if [ -n "${LOG_FILE:-}" ]; then
    echo "[$timestamp] [INFO] $message" >> "$LOG_FILE"
  fi
}

log_success() {
  local message="$1"
  local timestamp
  timestamp=$(get_current_date)
  echo -e "${GREEN}[SUCCESS]${NC} $message"
  if [ -n "${LOG_FILE:-}" ]; then
    echo "[$timestamp] [SUCCESS] $message" >> "$LOG_FILE"
  fi
}

log_warning() {
  local message="$1"
  local timestamp
  timestamp=$(get_current_date)
  echo -e "${YELLOW}[WARNING]${NC} $message"
  if [ -n "${LOG_FILE:-}" ]; then
    echo "[$timestamp] [WARNING] $message" >> "$LOG_FILE"
  fi
}

log_error() {
  local message="$1"
  local timestamp
  timestamp=$(get_current_date)
  echo -e "${RED}[ERROR]${NC} $message" >&2
  if [ -n "${LOG_FILE:-}" ]; then
    echo "[$timestamp] [ERROR] $message" >> "$LOG_FILE"
  fi
}

exit_with_error() {
  local message="$1"
  local exit_code="${2:-1}"
  log_error "$message"
  exit "$exit_code"
}

on_signal() {
  local signal_name="$1"
  local exit_code="$2"
  echo ""
  log_warning "Received ${signal_name}. Exiting..."
  echo ""
  exit "$exit_code"
}

on_error() {
  local exit_code=$?
  local line_no="$1"
  local command="$2"
  log_error "Command failed with exit code ${exit_code} at line ${line_no}: ${command}"
  exit "$exit_code"
}

kubectl() {
  sudo /opt/deckhouse/bin/kubectl "$@"
}

trap 'on_error "${LINENO}" "${BASH_COMMAND}"' ERR
trap 'on_signal "SIGINT" 130' SIGINT
trap 'on_signal "SIGTERM" 143' SIGTERM

SA_NAME="${1:-}"
CLUSTER_PREFIX="${2:-}"
CLUSTER_NAME="${3:-}"
FILE_NAME="${4:-}"

if [[ -z "$SA_NAME" ]] || [[ -z "$CLUSTER_PREFIX" ]] || [[ -z "$CLUSTER_NAME" ]]; then
  exit_with_error "Usage: ${0} <SA_NAME> <CLUSTER_PREFIX> <CLUSTER_NAME> [FILE_NAME]"
fi

if [[ -z "$FILE_NAME" ]]; then
  FILE_NAME=/tmp/kube.config
fi

SA_TOKEN=virt-${CLUSTER_PREFIX}-${SA_NAME}-token
SA_CAR_NAME=virt-${CLUSTER_PREFIX}-${SA_NAME}

USER_NAME=${SA_NAME}
CONTEXT_NAME="${CLUSTER_NAME}"-"${USER_NAME}"

if kubectl cluster-info > /dev/null 2>&1; then
  log_success "Connection to Kubernetes cluster OK. Proceeding..."
else
  exit_with_error "No access to Kubernetes cluster or configuration issue."
fi

# The ServiceAccount, its token Secret and the ClusterAuthorizationRule are
# created by dhctl from charts/cluster-config/templates/nested-sa.yaml. They
# cannot be created from here: kubectl on the master runs as kubernetes-admin,
# and the user-authz admission denies it a SuperAdmin grant.
wait_for_sa_token() {
  log_info "Wait for the token secret ${SA_TOKEN} of ServiceAccount ${SA_NAME}"

  local max_attempts=60
  local retry_wait_seconds=10
  local attempt_number

  for ((attempt_number = 1; attempt_number <= max_attempts; attempt_number++)); do
    if [[ -n "$(kubectl -n d8-service-accounts get secret "${SA_TOKEN}" -o jsonpath='{.data.token}' 2>/dev/null)" ]]; then
      log_success "Token secret ${SA_TOKEN} is ready"
      return 0
    fi
    log_warning "Token secret ${SA_TOKEN} is not ready yet (attempt ${attempt_number}/${max_attempts}), retrying..."
    sleep "${retry_wait_seconds}"
  done

  log_error "Token secret ${SA_TOKEN} not found; check that dhctl created the resources from charts/cluster-config/templates/nested-sa.yaml"
  kubectl -n d8-service-accounts get sa,secret 2>&1 || true
  kubectl get clusterauthorizationrule "${SA_CAR_NAME}" 2>&1 || true
  return 1
}

wait_for_sa_token || exit_with_error "ServiceAccount token is not available"


kubeconfig_cert_cluster_section() {
  log_info "Set cluster config"

  local api_host
  if api_host=$(kubectl -n d8-user-authn get ing kubernetes-api -ojson 2>/dev/null | jq -r '.spec.rules[].host' | head -1) && [ -n "$api_host" ]; then
    log_info "Found kubernetes-api ingress in d8-user-authn"
  elif api_host=$(kubectl -n kube-system get ing kubernetes-api -ojson 2>/dev/null | jq -r '.spec.rules[].host' | head -1) && [ -n "$api_host" ]; then
    log_info "Found kubernetes-api ingress in kube-system"
  else
    exit_with_error "kubernetes-api ingress not found in d8-user-authn or kube-system"
  fi

  kubectl config set-cluster "${CLUSTER_NAME}" \
    --insecure-skip-tls-verify=true \
    --server=https://"${api_host}" \
    --kubeconfig="${FILE_NAME}"
}

kubeconfig_set_credentials() {
  log_info "Set credentials"
  kubectl config set-credentials "${USER_NAME}" \
    --token="$(kubectl -n d8-service-accounts get secret "${SA_TOKEN}" -o json |jq -r '.data["token"]' | base64 -d)" \
    --kubeconfig="${FILE_NAME}"
}

kubeconfig_set_context() {
  log_info "Set context"
  kubectl config set-context "${CONTEXT_NAME}" \
    --cluster="${CLUSTER_NAME}" \
    --user="${USER_NAME}" \
    --kubeconfig="${FILE_NAME}"
}

kubeconfig_set_current_context() {
  log_info "Set current context"
  kubectl config set current-context "${CONTEXT_NAME}" \
    --kubeconfig="${FILE_NAME}"
}

check_kubeconfig() {
  local output

  if output=$(kubectl --kubeconfig "${FILE_NAME}" get no 2>&1); then
    return 0
  fi

  log_warning "Generated kubeconfig is not ready yet"
  echo "${output}"
  kubectl --kubeconfig "${FILE_NAME}" auth can-i get nodes 2>&1 || true
  kubectl get clusterrolebinding "user-authz:${SA_CAR_NAME}:super-admin" -o wide 2>&1 || true

  if [[ -f "${FILE_NAME}" ]]; then
    cat "${FILE_NAME}"
  fi
  return 1
}

generate_kubeconfig() {
  log_info "Create kubeconfig"

  local max_attempts=60
  local retry_wait_seconds=10
  local attempt_number

  for ((attempt_number = 1; attempt_number <= max_attempts; attempt_number++)); do
    kubeconfig_cert_cluster_section
    kubeconfig_set_credentials
    kubeconfig_set_context
    kubeconfig_set_current_context

    if check_kubeconfig; then
      return 0
    fi

    log_warning "Kubeconfig validation failed (attempt ${attempt_number}/${max_attempts}), retrying..."
    sleep "${retry_wait_seconds}"
  done

  log_error "Unable to generate a working kubeconfig after ${max_attempts} attempts"
  return 1
}

generate_kubeconfig || exit_with_error "Kubeconfig generation failed"

log_success "kubeconfig created and stored in ${FILE_NAME}"
sudo chmod 444 "${FILE_NAME}"
ls -la "${FILE_NAME}"

log_success "Done"
