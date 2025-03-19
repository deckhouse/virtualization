#!/usr/bin/env bash

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

set -Eeuo pipefail
shopt -s failglob

FILE_TEMPLATE_BINS=""
TEMPLATE_BINS=""
OUT_DIR=""

tools=("ldd" "readlink" "awk" "dirname" "ls" "cat")
for tool in "${tools[@]}"; do
  if ! command -v "$tool" >/dev/null 2>&1; then
    echo "$tool is not installed."
    exit 1
  fi
done

function Help() {
   # Display Help
   cat<<'EOF'
   Copy binaries and their libraries to a folder
   Only one input parameter allowed (-f or -i) !!!
   
   Syntax: scriptTemplate [-h|f|i|o]
   options:
   
   -f     Files with paths to binaries; Support mask like /sbin/m*
   -i     Paths to binaries separated by space; Support mask like /sbin/m*; Example: /bin/chmod /bin/mount /sbin/m*
          List of binaries should be in double quotes, -i /bin/chmod /bin/mount
   -o     Output directory (Default value: '/relocate')
   -h     Print this help

EOF
}

while getopts ":h:i:f:o:" option; do
    case $option in
      h) # display Help
         Help
         exit;;
      f)
        FILE_TEMPLATE_BINS=$OPTARG
        ;;
      i)
        TEMPLATE_BINS=$OPTARG
        ;;
      o)
        OUT_DIR=$OPTARG
        ;;
      \?)
        echo "Error: Invalid option"
        exit;;
    esac
done

if [[ -z $OUT_DIR ]];then
  OUT_DIR="/relocate"
fi
mkdir -p "${OUT_DIR}"

function relocate_item() {
  local file=$1
  
  if [[ $file =~ ^(/lib|/lib64|/bin|/sbin) ]];then
    file="/usr${file}"
  fi
  
  local new_place="${OUT_DIR}$(dirname ${file})"

  mkdir -p ${new_place}
  cp -a ${file} ${new_place} || true

  # if symlink, copy original file too
  local orig_file="$(readlink -f ${file})"
  if [[ "${file}" != "${orig_file}" ]]; then
    cp -a ${orig_file} ${new_place} || true
  fi
}

function relocate_lib() {
  local item=$1
  if ! [[ $item =~ /(BINS|VBINS) ]];then
    relocate_item ${item}
  fi

  for lib in $(ldd ${item} 2>/dev/null | awk '{if ($2=="=>") print $3; else print $1}'); do
    # don't try to relocate linux-vdso.so lib due to this lib is virtual
    if [[ "${lib}" =~ "linux-vdso" || "${lib}" == "not" ]]; then
      continue
    fi
    relocate_item ${lib}
  done
}

function get_binary_path () {
  local bin
  BINARY_LIST=()

  for bin in "$@"; do
    if [[ ! -f $bin ]] || [ "${bin}" == "${OUT_DIR}" ]; then
      echo "Not found $bin"
      exit 1
    fi
    BINARY_LIST+=$(ls -la $bin 2>/dev/null | awk '{print $9}')" "
  done

  if [[ -z $BINARY_LIST ]]; then echo "No binaryes for replace"; exit 1; fi;
}

# if get file with binaryes (-f)
if [[ -n $FILE_TEMPLATE_BINS ]] && [[ -f $FILE_TEMPLATE_BINS ]] && [[ -z $TEMPLATE_BINS ]]; then
  BIN_TEMPLATE=$(cat $FILE_TEMPLATE_BINS)
  get_binary_path ${BIN_TEMPLATE}
# Or get paths to bin via raw input (-i)
elif [[ -n $TEMPLATE_BINS ]] && [[ -z $FILE_TEMPLATE_BINS ]]; then
  get_binary_path ${TEMPLATE_BINS}
else
  Help
  exit
fi


for binary in ${BINARY_LIST[@]}; do
  relocate_lib ${binary}
done