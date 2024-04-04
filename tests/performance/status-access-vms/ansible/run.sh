#!/bin/bash
SSKKEY="../sshkeys/id_ed"
NAMESPACE=test-perf

function prepare_ssh_key {
    chmod 600 $SSKKEY
    sed -i '' -E "s|private_key_file=.+|private_key_file=${SSKKEY}|" ./ansible.cfg
}

sigint_handler() {
    echo "Aborted"
    exit 0
}
trap 'sigint_handler' SIGINT


function generate_inventory {
    vms=$(kubectl -n $NAMESPACE get vm -o=jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.status.ipAddress}{"\n"}{end}')
    mkdir -p inventory
    inventory_file="inventory/hosts.yml"
    echo "---
all:
  hosts:" > $inventory_file

    while IFS=$'\t' read -r vm_name vm_ip; do
        echo "    ${vm_name}.${NAMESPACE}:" >> $inventory_file
    done <<< "$vms"
}

function main {
    prepaer_ssh_key

    while true
    do
        echo "Generate inventory"
        generate_inventory
        echo "Try to access all hosts from inventory "
        ansible-playbook playbook.yaml | sed -n '/PLAY RECAP/,$p' > play_report.log
        ALLHOSTS=$(( $(wc -l play_report.log | grep -Eo '\d{1,4}') - 2 )) # One head line and 1 empty at EOF
        OKHOST=$(( $(grep 'ok=1' play_report.log | wc -l) ))
        UNREACHABLE=$(( $ALLHOSTS - $OKHOST ))
        echo "OK hosts $OKHOST | Unreachable hosts $UNREACHABLE | Total hosts $ALLHOSTS"
        echo "Wait 1 sec"
        sleep 1
    done
}

main