#!/bin/bash

# Copyright 2024 Flant JSC
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

VMS_UNREACHBLE_FILE="../ansible/retry/playbook.retry"

function Help() {
# Display Help
   cat <<EOF

Syntax: scriptTemplate [-n|u|h]:
options:
n     Set namespace with VirtualMachines
u     File with list of unreacheble VirtualMachines for VMOPS (default "../ansible/retry/playbook.retry")
h     Print this help
   
EOF
}

while getopts "n:u:h" opt; do
  case $opt in
    n) NAMESPACE=$OPTARG ;;
    u) VMS_UNREACHBLE_FILE=$OPTARG ;;
    h) Help
       exit 0;;
    \?) echo "Error: Invalid option -$OPTARG" >&2
        Help
        exit 1 ;;
  esac
done

exit_handler() {
    echo "Exit"
    exit 0
}
trap 'exit_handler' EXIT

if [ ! -f $VMS_UNREACHBLE_FILE ]; then echo "File does not exist"; exit 1;fi
if [ -z $NAMESPACE ]; then echo "Namespace must be defined"; exit 1;fi

VMS_UNREACHBLE=( $(cut -d '.' -f1 $VMS_UNREACHBLE_FILE) )

echo "Generate VirtualMachineOperation for vm in namespace $NAMESPACE"
for vm in "${VMS_UNREACHBLE[@]}"; do

    kubectl apply -f -<<EOF
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualMachineOperation
metadata:
  name: restart-$vm
  namespace: $NAMESPACE
spec:
  virtualMachineName: $vm
  type: Restart
  force: true
EOF

done

echo "Sleep 10 sec"
sleep 10
echo "Clear all vmops"
kubectl -n $NAMESPACE delete vmops --all