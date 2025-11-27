#!/usr/bin/env bash

NAMESPACE=$1
STORAGE_CLASS=$2
VD_SIZE=$3

if [[ -z "${VD_SIZE}" ]]; then
  VD_SIZE=50Gi
fi

if [[ -z "${NAMESPACE}" ]]; then
  local sha=$(git rev-parse --short HEAD) || true
  NAMESPACE=nightly-e2e-short-${sha}
fi

if [[ -z "${STORAGE_CLASS}" ]]; then
  STORAGE_CLASS=$(kubectl get storageclass -o json | jq -r '.items[] | select(.metadata.annotations."storageclass.kubernetes.io/is-default-class" == "true") | .metadata.name')
fi

VMS=$(kubectl -n ${NAMESPACE} get vm -o json | jq -r '.items[] | select(.metadata.name | contains("worker")) | .metadata.name')

create_vd() {
  local name=$1
  local size=$2
  local storageClass=$3
  kubectl apply -f - <<EOF
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualDisk
metadata:
  name: ${name}
  namespace: ${NAMESPACE}
spec:
  persistentVolumeClaim:
    size: ${size}
    storageClassName: ${storageClass}
EOF
  
  kubectl -n ${NAMESPACE} wait --for=condition=Ready vd ${name} --timeout=300s
}

create_vmbda() {
  local name=$1
  local vm=$2
  local vd=$3
  kubectl apply -f - <<EOF
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualMachineBlockDeviceAttachment
metadata:
  name: ${name}
  namespace: ${NAMESPACE}
spec:
  blockDeviceRef:
    kind: VirtualDisk
    name: ${vd}
  virtualMachineName: ${vm}
EOF

}

main() {
  while IFS= read -r vm; do
    if [[ -z "${vm}" ]]; then
      continue
    fi
    vd_name="${vm}-vd-blank"
    create_vd "${vd_name}" "${VD_SIZE}" "${STORAGE_CLASS}"
    create_vmbda "vmbda-${vm}" "${vm}" "${vd_name}"
  done <<< "${VMS}"
}

main