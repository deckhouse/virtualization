#!/usr/bin/env bash

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
CYAN='\033[0;36m'
NC='\033[0m' # No Color

log_info() {
  local message="$1"
  local timestamp=$(get_current_date)
  echo -e "${BLUE}[INFO]${NC} $message"
  if [ -n "$LOG_FILE" ]; then
      echo "[$timestamp] [INFO] $message" >> "$LOG_FILE"
  fi
}

log_success() {
  local message="$1"
  local timestamp=$(get_current_date)
  echo -e "${GREEN}[SUCCESS]${NC} $message"
  if [ -n "$LOG_FILE" ]; then
      echo "[$timestamp] [SUCCESS] $message" >> "$LOG_FILE"
  fi
}

log_warning() {
  local message="$1"
  local timestamp=$(get_current_date)
  echo -e "${YELLOW}[WARNING]${NC} $message"
  if [ -n "$LOG_FILE" ]; then
      echo "[$timestamp] [WARNING] $message" >> "$LOG_FILE"
  fi
}

log_error() {
  local message="$1"
  local timestamp=$(get_current_date)
  echo -e "${RED}[ERROR]${NC} $message"
  if [ -n "$LOG_FILE" ]; then
      echo "[$timestamp] [ERROR] $message" >> "$LOG_FILE"
  fi
}

exit_trap() {
  echo ""
  log_info "Exiting..."
  echo ""
  exit 0
}

kubectl() {
  sudo /opt/deckhouse/bin/kubectl $@
}

trap exit_trap SIGINT SIGTERM


SA_NAME=$1
CLUSTER_PREFIX=$2
CLUSTER_NAME=$3
FILE_NAME=$4

if [[ -z "$SA_NAME" ]] || [[ -z "$CLUSTER_PREFIX" ]] || [[ -z "$CLUSTER_NAME" ]]; then
  log_error "Usage: gen-sa.sh <SA_NAME> <CLUSTER_PREFIX> <CLUSTER_NAME> [FILE_NAME]"
  exit 1
fi

if [[ -z "$FILE_NAME" ]]; then
  FILE_NAME=kube.config
fi

SA_TOKEN=virt-${CLUSTER_PREFIX}-${SA_NAME}-token
SA_CAR_NAME=virt-${CLUSTER_PREFIX}-${SA_NAME}

USER_NAME=${SA_NAME}
CONTEXT_NAME=${CLUSTER_NAME}-${USER_NAME}
FILE_NAME=kube.config

if kubectl cluster-info > /dev/null 2>&1; then
  log_success "Access to Kubernetes cluster exists."
else
  log_error "No access to Kubernetes cluster or configuration issue."
  exit 1
fi

sleep 2
log_info "===="
log_info "Kubeconfig will be created successfully if you connected to k8s cluster via ssh tunnel or directly"
log_info "===="
sleep 2


log_info "Apply SA, Secrets and ClusterAuthorizationRule"
kubectl apply -f -<<EOF
apiVersion: v1
kind: ServiceAccount
metadata:
  name: $SA_NAME
  namespace: d8-service-accounts
---
apiVersion: v1
kind: Secret
metadata:
  name: ${SA_TOKEN}
  namespace: d8-service-accounts
  annotations:
    kubernetes.io/service-account.name: $SA_NAME
type: kubernetes.io/service-account-token
---
apiVersion: deckhouse.io/v1
kind: ClusterAuthorizationRule
metadata:
  name: ${SA_CAR_NAME}
spec:
  subjects:
  - kind: ServiceAccount
    name: ${SA_NAME}
    namespace: d8-service-accounts
  accessLevel: SuperAdmin
  allowAccessToSystemNamespaces: true
EOF
log_success "SA, Secrets and ClusterAuthorizationRule applied"


kubeconfig_cert_cluster_section() {
  kubectl config set-cluster $CLUSTER_NAME --insecure-skip-tls-verify=true
}

kubeconfig_set_credentials() {
  kubectl config set-credentials $USER_NAME \
  --token=$(kubectl -n d8-service-accounts get secret ${SA_TOKEN} -o json |jq -r '.data["token"]' | base64 -d) \
  --kubeconfig=$FILE_NAME
}

kubeconfig_set_context() {
  kubectl config set-context $CONTEXT_NAME \
    --cluster=$CLUSTER_NAME \
    --user=$USER_NAME \
    --kubeconfig=$FILE_NAME
}

log_info "Create kubeconfig"

kubeconfig_cert_cluster_section
kubeconfig_set_credentials
kubeconfig_set_context

log_success "kubeconfig created and stored in $FILE_NAME"


kubectl config use-context $CONTEXT_NAME --kubeconfig=$FILE_NAME
kubectl --kubeconfig=$FILE_NAME get nodes

log_success "Done"