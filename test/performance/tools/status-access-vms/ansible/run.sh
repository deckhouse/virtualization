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
u     Enable or disable ansible unreachable host file (Only 'true' or 'false' required; -u false )
h     Print this help
   
EOF
}

while getopts "s:n:u:h" opt; do
  case $opt in
    s) SSK_KEY=$OPTARG ;;
    n) NAMESPACE=$OPTARG ;;
    u) UNREACHABLE_HOST_FILE=$OPTARG ;;
    h) Help
       exit 0;;
    \?) echo "Error: Invalid option -$OPTARG" >&2
        Help
        exit 1 ;;
  esac
done

function enable_unreachable_host {
    local ENABLE=$1

    if $ENABLE; then
        echo "Enable write to file unreachable host"
        sed -i -E "s|^# retry_files_enabled=true|retry_files_enabled=true|" $ANSIBLE_CFG
        sed -i -E "s|^# retry_files_save_path=./retry|retry_files_save_path=./retry|" $ANSIBLE_CFG
    else
        echo "Disable write to file unreachable host"
        sed -i -E "s|^retry_files_enabled=true|# retry_files_enabled=true|" $ANSIBLE_CFG
        sed -i -E "s|^retry_files_save_path=./retry|# retry_files_save_path=./retry|" $ANSIBLE_CFG
    fi
}

function darwin_sed {
    if $ENABLE; then
        echo "Enable write to file unreachable host"
        sed -i '' -E "s|^# retry_files_enabled=true|retry_files_enabled=true|" $ANSIBLE_CFG
        sed -i '' -E "s|^# retry_files_save_path=./retry|retry_files_save_path=./retry|" $ANSIBLE_CFG
    else
        echo "Disable write to file unreachable host"
        sed -i '' -E "s|^retry_files_enabled=true|# retry_files_enabled=true|" $ANSIBLE_CFG
        sed -i '' -E "s|^retry_files_save_path=./retry|# retry_files_save_path=./retry|" $ANSIBLE_CFG
    fi
}

function enable_unreachable_host_file {
    local ENABLE=$1
    
    if [ "$(uname)" = "Darwin" ]; then
        darwin_sed $ENABLE
    elif [ "$(uname)" = "Linux" ]; then
        enable_unreachable_host $ENABLE
    else
        echo "unknown OS"
        echo "try linux"
        enable_unreachable_host $ENABLE
    fi

}

function prepare_ssh_key {
    chmod 600 $SSK_KEY
    if [ "$(uname)" = "Darwin" ]; then
        sed -i '' -E "s|private_key_file=.+|private_key_file=${SSK_KEY}|" $ANSIBLE_CFG
    elif [ "$(uname)" = "Linux" ]; then
        sed -i -E "s|private_key_file=.+|private_key_file=${SSK_KEY}|" $ANSIBLE_CFG
    else
        echo "unknown OS"
        echo "try linux"
        sed -i -E "s|private_key_file=.+|private_key_file=${SSK_KEY}|" $ANSIBLE_CFG
    fi
    
}

exit_handler() {
    echo "Exit"
    exit 0
}
trap 'exit_handler' EXIT

if [ -z $NAMESPACE ]; then echo "Namespace must be defined"; exit 1;fi

if [[ $UNREACHABLE_HOST_FILE == true ]] ; then
    enable_unreachable_host_file true
else
    enable_unreachable_host_file false
fi

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
        
        echo "Try to access all hosts from inventory $(date); Namespace: $NAMESPACE"
        ansible-playbook playbook.yaml | sed -n '/PLAY RECAP/,$p' > $ANSIBLE_REPORT_FILE
        while [ ! -f $ANSIBLE_REPORT_FILE ]; do sleep 2; done
        
        echo $(wc -l $ANSIBLE_REPORT_FILE)

        if [ "$(uname)" = "Darwin" ]; then
            HOSTS_TOTAL=$(( $(wc -l $ANSIBLE_REPORT_FILE | grep -Eo '\d{1,7}') - 2 ))
        elif [ "$(uname)" = "Linux" ]; then
            HOSTS_TOTAL=$(( $(wc -l $ANSIBLE_REPORT_FILE | cut -d ' ' -f1) - 2 ))
        fi
        

        HOSTS_OK=$(( $(grep -E 'ok=[1-9]+' $ANSIBLE_REPORT_FILE | wc -l) ))
        HOSTS_UNREACHABLE=$(( $(grep -E 'unreachable=[1-9]+' $ANSIBLE_REPORT_FILE | wc -l) ))
        OK_PCT=$(bc -l <<< "scale=2; $HOSTS_OK/$HOSTS_TOTAL*100")
        
        if [[ $HOSTS_UNREACHABLE -ne 0 ]]; then
            grep -E 'unreachable=[1-9]+' $ANSIBLE_REPORT_FILE
        fi

        echo "OK hosts count:$HOSTS_OK pct.:$OK_PCT% | Unreachable hosts $HOSTS_UNREACHABLE | Total hosts $HOSTS_TOTAL"
        echo "Wait 2 sec"
        echo -e "---\n"
        sleep 2
    done
}

main