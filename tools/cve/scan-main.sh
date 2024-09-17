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
module_tag=$2
if [[ -z $REPORT_FILE_NAME ]];then echo "file must be define";exit 1;fi
if [[ -z $module_tag ]]; then module_tag=main; fi

# Prepare images digests in form of "image_name image_sha256_digest".
images_digests=$(crane export dev-registry.deckhouse.io/sys/deckhouse-oss/modules/virtualization:${module_tag}  - | tar -Oxf - images_digests.json | jq -r 'to_entries[] | .name + " " + .value')

{
  while read name digest; do
    image="dev-registry.deckhouse.io/sys/deckhouse-oss/modules/virtualization@${digest}"

    if [[ ${name} =~ Builder|Artifact ]]; then
      continue
    fi

    echo "⭐ ==============================================================================================================="
    echo "name: ${name}"
    echo "image: ${image}"
    echo "=================================================================================================================="

    trivy image ${image} -f table

  done <<< "${images_digests}"
} > "${REPORT_FILE_NAME}"
