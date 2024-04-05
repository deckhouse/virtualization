#!/bin/bash
SSK_KEY="../../ssh/id_ed"

function Help() {
# Display Help
   cat <<EOF

Syntax: scriptTemplate [-s|n|h]:
options:
n     Set namespace with VirtualMachines
s     Path to ssh private key, default ../../ssh/id_ed
h     Print this help
   
EOF
}

while getopts "s:n:" opt; do
  case $opt in
    s) SSK_KEY=$OPTARG ;;
    n) NAMESPACE=$OPTARG ;;
    h) Help
       exit 1;;
    \?) echo "Error: Invalid option -$OPTARG" >&2
        Help
        exit 1 ;;
  esac
done

if [ -z $NAMESPACE ]; then echo "Namespace must be defined"; exit 1;fi

function prepare_ssh_key {
    chmod 600 $SSK_KEY
    sed -i '' -E "s|private_key_file=.+|private_key_file=${SSK_KEY}|" ./ansible.cfg
}

sigint_handler() {
    echo "Canceld"
    exit 0
}
trap 'sigint_handler' SIGINT


function generate_inventory {
    VMS=$(kubectl -n $NAMESPACE get vm -o=jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}')
    mkdir -p inventory
    INVENTORY_FILE="inventory/hosts.yml"
    echo "---
all:
  hosts:" > $INVENTORY_FILE

    while IFS= read -r VM_NAME; do
        echo "    ${VM_NAME}.${NAMESPACE}:" >> $INVENTORY_FILE
    done <<< "$VMS"
}

function main {
    prepare_ssh_key
    ANSIBLE_REPORT_FILE=play_report.log

    while true
    do
        echo "Generate inventory"
        generate_inventory
        
        echo "Try to access all hosts from inventory "
        ansible-playbook playbook.yaml | sed -n '/PLAY RECAP/,$p' > $ANSIBLE_REPORT_FILE
        while [ ! -f $ANSIBLE_REPORT_FILE ]; do sleep 1; done
        
        ALL_HOSTS=$(( $(wc -l $ANSIBLE_REPORT_FILE | grep -Eo '\d{1,4}') - 2 )) # One head line and 1 empty at EOF
        OK_HOSTS=$(( $(grep 'ok=1' $ANSIBLE_REPORT_FILE | wc -l) ))
        UNREACHABLE_HOSTS=$(( $(grep 'unreachable=1' $ANSIBLE_REPORT_FILE | wc -l) ))
        OK_PCT=$(bc -l <<< "scale=2; $OK_HOSTS/$ALL_HOSTS*100")
        
        echo "OK hosts count:$OK_HOSTS pct.:$OK_PCT% | Unreachable hosts $UNREACHABLE_HOSTS | Total hosts $ALL_HOSTS"
        echo "Wait 1 sec"
        sleep 1
    done
}

main