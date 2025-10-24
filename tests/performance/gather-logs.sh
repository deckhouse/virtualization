#!/bin/bash

set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log() {
    echo -e "${GREEN}[$(date +'%Y-%m-%d %H:%M:%S')]${NC} $1"
}

error() {
    echo -e "${RED}[ERROR]${NC} $1" >&2
}

warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

check_kubectl() {
    if ! command -v kubectl &> /dev/null; then
        error "kubectl not found. Install kubectl to use this script."
        exit 1
    fi
}

check_kubectl_connection() {
    if ! kubectl cluster-info &> /dev/null; then
        error "Cannot connect to Kubernetes cluster. Check kubectl configuration."
        exit 1
    fi
}

create_log_directory() {
    local timestamp=$(date +'%Y%m%d_%H%M%S')
    LOG_DIR="k8s_logs_${timestamp}"
    mkdir -p "$LOG_DIR"
    log "Created log directory: $LOG_DIR"
}

collect_pod_logs() {
    local namespace="$1"
    local pod_name="$2"
    local log_file="$3"

    log "Collecting logs for pod: $pod_name in namespace: $namespace"

    if kubectl logs "$pod_name" -n "$namespace" > "$log_file" 2>/dev/null; then
        log "Current pod logs saved to: $log_file"
    else
        warning "Failed to get logs for pod: $pod_name"
        echo "Logs unavailable for pod: $pod_name" > "$log_file"
    fi

    local previous_log_file="${log_file%.log}_previous.log"
    if kubectl logs "$pod_name" -n "$namespace" --previous > "$previous_log_file" 2>/dev/null; then
        log "Previous pod logs saved to: $previous_log_file"
    else
        warning "Previous logs unavailable for pod: $pod_name"
        echo "Previous logs unavailable for pod: $pod_name" > "$previous_log_file"
    fi
}

collect_namespace_logs() {
    local namespace="$1"

    log "Processing namespace: $namespace"

    if ! kubectl get namespace "$namespace" &> /dev/null; then
        warning "Namespace '$namespace' not found, skipping..."
        return
    fi

    local ns_dir="$LOG_DIR/$namespace"
    mkdir -p "$ns_dir"

    local pods=$(kubectl get pods -n "$namespace" --no-headers -o custom-columns=":metadata.name" 2>/dev/null || true)

    if [ -z "$pods" ]; then
        warning "No pods found in namespace '$namespace'"
        return
    fi

    while IFS= read -r pod_name; do
        if [ -n "$pod_name" ]; then
            local log_file="$ns_dir/${pod_name}.log"
            collect_pod_logs "$namespace" "$pod_name" "$log_file"
        fi
    done <<< "$pods"

    log "Completed processing namespace: $namespace"
}

create_archive() {
    log "Creating archive..."

    local archive_name="${LOG_DIR}.tar.gz"

    if tar -czf "$archive_name" "$LOG_DIR"; then
        log "Archive created: $archive_name"

        local archive_size=$(du -h "$archive_name" | cut -f1)
        log "Archive size: $archive_size"

        rm -rf "$LOG_DIR"
        log "Temporary directory removed"

        echo ""
        log "Done! Log archive: $archive_name"
    else
        error "Error creating archive"
        exit 1
    fi
}

main() {
    echo "=========================================="
    echo "  Kubernetes Log Collector"
    echo "=========================================="
    echo ""

    if [ $# -eq 0 ]; then
        error "At least one namespace must be specified"
        echo "Usage: $0 <namespace1> [namespace2] [namespace3] ..."
        echo "Example: $0 default kube-system monitoring"
        exit 1
    fi

    check_kubectl
    check_kubectl_connection

    create_log_directory

    for namespace in "$@"; do
        collect_namespace_logs "$namespace"
    done

    create_archive

    echo ""
    log "Log collection completed successfully!"
}

main "$@"