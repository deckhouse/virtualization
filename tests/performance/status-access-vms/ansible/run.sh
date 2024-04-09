#!/bin/bash
SSK_KEY="../../ssh/id_ed"
ANSIBLE_CFG="./ansible.cfg"
INVENTORY_FILE="inventory/hosts.yml"

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

while getopts "s:n:h" opt; do
  case $opt in
    s) SSK_KEY=$OPTARG ;;
    n) NAMESPACE=$OPTARG ;;
    h) Help
       exit 0;;
    \?) echo "Error: Invalid option -$OPTARG" >&2
        Help
        exit 1 ;;
  esac
done

if [ -z $NAMESPACE ]; then echo "Namespace must be defined"; exit 1;fi

function prepare_ssh_key {
    chmod 600 $SSK_KEY
    sed -i '' -E "s|private_key_file=.+|private_key_file=${SSK_KEY}|" $ANSIBLE_CFG
}

exit_handler() {
    echo "Canceld"
    exit 0
}
trap 'exit_handler' EXIT

function generate_inventory {
    VMS=$(kubectl -n $NAMESPACE get vm -o=jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}')
    mkdir -p inventory
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
        
        HOSTS_TOTAL=$(( $(wc -l $ANSIBLE_REPORT_FILE | grep -Eo '\d{1,4}') - 2 ))
        HOSTS_OK=$(( $(grep -E 'ok=[1-9]+' $ANSIBLE_REPORT_FILE | wc -l) ))
        HOSTS_UNREACHABLE=$(( $(grep -E 'unreachable=[1-9]+' $ANSIBLE_REPORT_FILE | wc -l) ))
        OK_PCT=$(bc -l <<< "scale=2; $HOSTS_OK/$HOSTS_TOTAL*100")
        
        if [[ $HOSTS_UNREACHABLE -ne 0 ]]; then
            grep 'unreachable=1' $ANSIBLE_REPORT_FILE
        fi

        echo "OK hosts count:$HOSTS_OK pct.:$OK_PCT% | Unreachable hosts $HOSTS_UNREACHABLE | Total hosts $HOSTS_TOTAL"
        echo "Wait 2 sec"
        echo -e "---\n"
        sleep 2
    done
}

main