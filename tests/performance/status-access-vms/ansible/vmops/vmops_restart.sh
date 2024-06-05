#!/bin/bash

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