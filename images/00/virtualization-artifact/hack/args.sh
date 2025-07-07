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
