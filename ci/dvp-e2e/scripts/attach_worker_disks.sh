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
#   attach_worker_disks.sh -n namespace -s storage_class -z disk_size -c disk_count [-k kubeconfig]

namespace=""
storage_class=""
disk_size="10Gi"
disk_count="2"
kubeconfig="${KUBECONFIG:-}"

while getopts ":n:s:z:c:k:" opt; do
  case $opt in
    n) namespace="$OPTARG" ;;
    s) storage_class="$OPTARG" ;;
    z) disk_size="$OPTARG" ;;
    c) disk_count="$OPTARG" ;;
    k) kubeconfig="$OPTARG" ;;
    *) 
      echo "Usage: $0 -n <namespace> -s <storage_class> -z <disk_size> -c <disk_count> [-k <kubeconfig>]" >&2
      exit 2 
      ;;
  esac
done

if [ -z "${namespace}" ] || [ -z "${storage_class}" ]; then
  echo "Usage: $0 -n <namespace> -s <storage_class> -z <disk_size> -c <disk_count> [-k <kubeconfig>]" >&2
  exit 2
fi

if [ -n "${kubeconfig}" ]; then
  export KUBECONFIG="${kubeconfig}"
fi

echo "[INFRA] Attaching ${disk_count} storage disks to worker VMs using hotplug in namespace ${namespace}"

# Wait for worker VMs
for i in $(seq 1 50); do
  worker_count=$(kubectl -n "${namespace}" get vm -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}' 2>/dev/null | grep -c worker || echo "0")
  if [ "$worker_count" -gt 0 ]; then
    echo "[INFRA] Found $worker_count worker VMs"
    break
  fi
  echo "[INFRA] Waiting for worker VMs... ($i/50)"
  sleep 10
done

# Get worker VMs
mapfile -t workers < <(kubectl -n "${namespace}" get vm -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}' 2>/dev/null | grep worker || true)

if [ ${#workers[@]} -eq 0 ]; then
  echo "[INFRA] No worker VMs found; nothing to do"
  exit 0
fi

echo "[INFRA] Found ${#workers[@]} worker VMs: ${workers[*]}"

for vm in "${workers[@]}"; do
  [ -z "$vm" ] && continue
  echo "[INFRA] Processing VM: $vm"

  # Wait for VM to be Running
  for i in $(seq 1 50); do
    phase=$(kubectl -n "${namespace}" get vm "$vm" -o jsonpath='{.status.phase}' 2>/dev/null || true)
    if [ "$phase" = "Running" ]; then
      echo "[INFRA] VM $vm is Running"
      break
    fi
    echo "[INFRA] VM $vm phase=$phase; retry $i/50"
    sleep 10
  done

  for disk_num in $(seq 1 "${disk_count}"); do
    vd="storage-disk-${disk_num}-$vm"
    echo "[INFRA] Creating VirtualDisk $vd (${disk_size}, sc=${storage_class})"
    
    cat > "/tmp/vd-$vd.yaml" <<EOF
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualDisk
metadata:
  name: $vd
  namespace: ${namespace}
spec:
  persistentVolumeClaim:
    storageClassName: ${storage_class}
    size: ${disk_size}
EOF
    kubectl -n "${namespace}" get vd "$vd" >/dev/null 2>&1 || kubectl -n "${namespace}" apply -f "/tmp/vd-$vd.yaml"

    # Wait for VirtualDisk to be Ready
    echo "[INFRA] Waiting for VirtualDisk $vd to be Ready..."
    vd_phase=""
    for j in $(seq 1 50); do
      vd_phase=$(kubectl -n "${namespace}" get vd "$vd" -o jsonpath='{.status.phase}' 2>/dev/null || true)
      if [ "$vd_phase" = "Ready" ]; then
        echo "[INFRA] VirtualDisk $vd is Ready"
        break
      fi
      echo "[INFRA] VD $vd phase=$vd_phase; retry $j/50"
      sleep 5
    done
    
    if [ "$vd_phase" != "Ready" ]; then
      echo "[ERROR] VirtualDisk $vd not Ready"
      kubectl -n "${namespace}" get vd "$vd" -o yaml || true
      kubectl -n "${namespace}" get events --sort-by=.lastTimestamp | tail -n 100 || true
      exit 1
    fi

    # Wait for PVC
    pvc_name=""
    for j in $(seq 1 50); do
      pvc_name=$(kubectl -n "${namespace}" get vd "$vd" -o jsonpath='{.status.target.persistentVolumeClaimName}' 2>/dev/null || true)
      [ -n "$pvc_name" ] && break
      echo "[INFRA] Waiting for PVC name for VD $vd; retry $j/50"
      sleep 3
    done
    
    if [ -n "$pvc_name" ]; then
      echo "[INFRA] Waiting PVC $pvc_name to reach phase=Bound..."
      for j in $(seq 1 120); do
        pvc_phase=$(kubectl -n "${namespace}" get pvc "$pvc_name" -o jsonpath='{.status.phase}' 2>/dev/null || true)
        if [ "$pvc_phase" = "Bound" ]; then
          break
        fi
        [ $((j % 10)) -eq 0 ] && echo "[INFRA] PVC $pvc_name phase=$pvc_phase; retry $j/120"
        sleep 2
      done
      if [ "$pvc_phase" != "Bound" ]; then
        echo "[WARN] PVC $pvc_name not Bound after waiting"
      fi
    fi

    # Create hotplug attachment
    att="att-$vd"
    echo "[INFRA] Creating VirtualMachineBlockDeviceAttachment $att for VM $vm"
    cat > "/tmp/att-$att.yaml" <<EOF
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualMachineBlockDeviceAttachment
metadata:
  name: $att
  namespace: ${namespace}
spec:
  virtualMachineName: $vm
  blockDeviceRef:
    kind: VirtualDisk
    name: $vd
EOF
    kubectl -n "${namespace}" get vmbda "$att" >/dev/null 2>&1 || kubectl -n "${namespace}" apply -f "/tmp/att-$att.yaml"

    # Wait for attachment
    echo "[INFRA] Waiting for VMBDA $att to be Attached..."
    att_phase=""
    success_by_vm=0
    for i in $(seq 1 50); do
      att_phase=$(kubectl -n "${namespace}" get vmbda "$att" -o jsonpath='{.status.phase}' 2>/dev/null || true)
      if [ "$att_phase" = "Attached" ]; then
        echo "[INFRA] Disk $vd attached to VM $vm"
        break
      fi
      if kubectl -n "${namespace}" get vm "$vm" -o json 2>/dev/null \
          | jq -e --arg vd "$att" --arg disk "$vd" '
            ([.status.blockDeviceRefs[]? 
              | select(
                  (.virtualMachineBlockDeviceAttachmentName == $vd)
                  or (.name == $disk)
                )
              | select((.attached == true) and (.hotplugged == true))
            ] | length) > 0' >/dev/null; then
        echo "[INFRA] VM reports disk $vd attached/hotplugged; proceeding"
        success_by_vm=1
        break
      fi
      [ $((i % 10)) -eq 0 ] && echo "[INFRA] Disk $vd phase=$att_phase; retry $i/50"
      sleep 5
    done

    if [ "$att_phase" != "Attached" ] && [ "${success_by_vm:-0}" -ne 1 ]; then
      echo "[ERROR] VMBDA $att did not reach Attached state"
      kubectl -n "${namespace}" get vmbda "$att" -o yaml || true
      kubectl -n "${namespace}" get vm "$vm" -o json || true
      kubectl -n "${namespace}" get events --sort-by=.lastTimestamp | tail -n 100 || true
      exit 1
    fi
  done

  echo "[INFRA] VM $vm configured with hotplug disks"
done

echo "[INFRA] All worker VMs configured with storage disks via hotplug"
