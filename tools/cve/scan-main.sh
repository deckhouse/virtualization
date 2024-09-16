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

REPORT_FILE_NAME=$1
if [[ -z $REPORT_FILE_NAME ]];then echo "file must be define";exit 1;fi
# report_file_name="$(date +%Y-%m-%d)-report.txt"
# module_tag=main
module_tag=pr358
if [[ -z $module_tag ]]; then module_tag=main; fi

images=$(crane export dev-registry.deckhouse.io/sys/deckhouse-oss/modules/virtualization:${module_tag}  - | tar -Oxf - images_digests.json | jq '. | to_entries[]')

{
  while IFS= read -r image_hash; do
    name=$(echo ${image_hash} | jq .key -cr)
    image="dev-registry.deckhouse.io/sys/deckhouse-oss/modules/virtualization@$(echo ${image_hash} | jq .value -cr)"

    if [[ ${name} =~ Builder|Artifact ]]; then
      continue
    fi

    echo "⭐ ==============================================================================================================="
    echo "name: ${name}"
    echo "image: ${image}"
    echo "=================================================================================================================="

    trivy image ${image} -f table

  done <<< $(echo ${images} | jq -c .)
} > "${REPORT_FILE_NAME}"