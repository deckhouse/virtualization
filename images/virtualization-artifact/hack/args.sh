#!/bin/bash

ARGS=()
FLAGS=()

function set_flags_args() {
    for arg in "$@"; do
        case "${arg}" in
            --*|-*)
                FLAGS+=( "$arg" )
                ;;
            *)
                ARGS+=( "$arg" )
                ;;
        esac
    done
}

function parse_flag() {
    local NAME=$1
    local SHORT_NAME=$2
    local DEFAULT="${3:-}"

    local RESULT=""
    for f in ${FLAGS[*]}; do
        case "${f}" in
        --${NAME}=*|-${SHORT_NAME}=*)
            RESULT="${f#*=}"
            break
            ;;
        "--${NAME}"|"-${SHORT_NAME}")
            RESULT="TRUE"
            break
            ;;
        esac
    done

    if [ -n "${DEFAULT}" ] && [ -z "${RESULT}" ]; then
        RESULT="${DEFAULT}"
    fi
    echo "${RESULT}"
}

function must_parse_flag() {
    result=$(parse_flag "$1" "$2" "$3")
    if [ -z "${result}" ]; then
        exit 1
    fi
    echo "${result}"
}
