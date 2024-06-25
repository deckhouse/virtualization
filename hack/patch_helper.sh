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

# Helper to automate some routine operations with patches:
# - Apply all patches in a separate branch to help you make a new patch.
# - Apply patches before some patch to help you change specified patch.
# - Clone and checkout specified repo.
# - Create temporary branches.

set -eu -o pipefail

function usage() {
  cat <<EOF
$(basename ${0}) OPTIONS

  --repo repo-url            Repository to clone, e.g. https://github.com/org/project-name.git

  --ref git-reference        Repository reference to checkout, branch or tag: main, v1.0.0

  --patches-dir dir-path     Directory where patches reside: ./patches, /project/patches

  --stop-at path|name        Helper will stop after applying specified patch.
                             Specify full path or just a name:  010-some-worthy-fix.patch
EOF
}

function main() {
  local REPO
  local REF
  local PATCHES_DIR
  local STOP_AT

  while [[ $# -gt 0 ]]; do
      case "$1" in
      --repo)
          shift
          if [[ $# -ne 0 ]]; then
            REPO="${1}"
          fi
          ;;
      --ref)
          shift
          if [[ $# -ne 0 ]]; then
            REF="${1}"
          fi
          ;;
      --patches-dir)
          shift
          if [[ $# -ne 0 ]]; then
            PATCHES_DIR="${1}"
          fi
          ;;
      --stop-at)
          shift
          if [[ $# -ne 0 ]]; then
            STOP_AT="${1}"
          fi
          ;;
      *)
          echo "ERROR: Invalid argument: $1"
          echo
          usage
          exit 1
          ;;
      esac
      shift
  done

  if [[ -z $REPO ]] ; then
    echo "Repository URL is required!" && echo && usage && exit 1
  fi
  if [[ -z $REF ]] ; then
    echo "Ref is required for checkout!" && echo && usage && exit 1
  fi
  if [[ -z $PATCHES_DIR ]] ; then
    echo "Patches directory is required!" && echo && usage && exit 1
  fi
  if [[ ! -d ${PATCHES_DIR} ]] ; then
    echo "'${PATCHES_DIR}' is not a directory!" && echo && usage && exit 1
  fi
  if [[ -n $STOP_AT ]] ; then
    # User specifies stop, but patch is not in patch dir.
    stop_at_name=$(basename $STOP_AT)
    if ! find ${PATCHES_DIR} -type f -name ${stop_at_name} | grep ${stop_at_name} 2>&1 >/dev/null ; then
      echo "Patch ${STOP_AT} not found in '${PATCHES_DIR}' directory" && echo && usage && exit 1
    fi
  fi

  # Transform arguments.
  branch=patching/$(date +%Y-%m-%d-%H%M%S)

  project=${REPO##*/}
  project=${project%.git}

  clone_dir=${REF//./-}
  clone_dir=__${project}_${clone_dir//\//-}

  patches_path=${PATCHES_DIR}
  if [[ ${PATCHES_DIR} = ./* || ${PATCHES_DIR} = ../* ]] ; then
    patches_path=$(pwd)/${PATCHES_DIR}
  fi

  # Clone and checkout.
  if [[ ! -d ${clone_dir} ]] ; then
    git clone --branch ${REF} ${REPO} ${clone_dir}
  fi

  cd ${clone_dir}

  echo "Cleanup workdir for ref $(git name-rev --name-only HEAD) ..."

  if ! git diff --exit-code 2>&1 >/dev/null ; then
    echo "Workdir is dirty, stash changes ..."
    git stash
  fi
  git reset --hard HEAD

  if [[ -n ${STOP_AT} ]] ; then
    echo "Create temporary branch ${branch} with patches from ${PATCHES_DIR} until ${STOP_AT} ..."
  else
    echo "Create temporary branch ${branch} and apply patches from ${PATCHES_DIR} ..."
  fi

  git checkout ${REF} --no-track -b ${branch}

  success=true
  stop_at_name=$(basename "${STOP_AT}")
  for patch_path in ${patches_path}/*.patch ; do
    name=$(basename ${patch_path})
    echo -n "Apply ${name} ... "
    if git apply --ignore-space-change --ignore-whitespace ${patch_path} ; then
      echo OK
      if [[ -n ${stop_at_name} && ${stop_at_name} = ${name} ]] ; then
        echo "Stop applying patches. NOTE: ${name} is left uncommitted."
        break
      fi
      git add .
      git commit -a -m "Apply patch ${name}"
    else
      echo FAIL
      success=false
      break
    fi
  done

  if [ "${success}" = false ] ; then
    exit 1
  fi

  echo
  echo "Congrats!"
  echo "Patches applied, you can make changes in '${clone_dir}' directory and then add a new patch:"
  echo "cd ${clone_dir}"
  echo "git diff  --patch > nnn-my-new-feature.patch"
  echo "cd -"
  echo "mv ${clone_dir}/nnn-my-new-feature.patch ${PATCHES_DIR}"
  echo "git add ${PATCHES_DIR}/nnn-my-new-feature.patch"
}

main "$@"
