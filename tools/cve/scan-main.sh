#!/usr/bin/env bash

report_file_name="$(date +%Y-%m-%d)-report.txt"
# module_tag=main
module_tag=pr358

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
} > "${report_file_name}"
