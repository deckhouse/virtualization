#!/bin/bash
TANK_CONFIG_PATH=load.yaml
TARGET_ADDRESS='95.143.190.246'


function Help() {
# Display Help
   cat <<EOF

Syntax: scriptTemplate [-c|t|h]:
options:
c     Path of config yandex-tank (Example: ./load.yaml) 
t     Target address for 
h     Print this help
   
EOF
}

while getopts "c:t:h" opt; do
  case $opt in
    c) TANK_CONFIG_PATH=$OPTARG ;;
    t) TARGET_ADDRESS=$OPTARG ;;
    h) Help
       exit 0;;
    \?) echo "Error: Invalid option -$OPTARG" >&2
        Help
        exit 1 ;;
  esac
done

if [ -z $TARGET_ADDRESS ]; then echo "Target addres must be defined"; exit 1;fi
if [ ! -f $TANK_CONFIG_PATH ]; then echo "Config file must be defined"; exit 1;fi

function prepare_tank_config {
    sed -i '' -E "s/address: .+/address: $TARGET_ADDRESS/" $TANK_CONFIG_PATH
    sed -i '' -E "s|\[Host: .+\]|\[Host: $TARGET_ADDRESS\]|" $TANK_CONFIG_PATH
}

function main {
    prepare_tank_config

    docker run \
        --rm \
        -v $(pwd)/$TANK_CONFIG_PATH:/var/loadtest/$TANK_CONFIG_PATH \
        -v $SSH_AUTH_SOCK:/ssh-agent -e SSH_AUTH_SOCK=/ssh-agent \
        --net host \
        -it yandex/yandex-tank -c $TANK_CONFIG_PATH
}

main