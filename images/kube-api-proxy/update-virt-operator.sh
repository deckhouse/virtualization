#!/bin/bash

set -eo pipefail

imagesDigests=$(cat <<EOF
  {
  "libguestfs": "sha256:b69f698f71b623bd4e19734a5ecf8316fdac239323c1664f28b818fb2c272605",

  "cdiOperator": "sha256:0e8819f2da9bc0d972ef0f3f166eb851886f80464906bb567f4d7d88567976fd",
  "cdiController": "sha256:4846ec910e9a170111ce9d3da302f951121f9b829f40b802e5be2075274fdc74",
  "cdiUploadserver": "sha256:cf7c758cd2c69435d6fcacef0b70950578b9c6d4d0a722c2b1461882302e46cf",
  "cdiCloner": "sha256:3f3a00b2bc277c6b85c680ba7839ffb43f115941ef96ce3d99197825bd8d9a35",
  "cdiApiserver": "sha256:af25e4eb707aa89938b1e10952eeed34fe70552ef70c290f4513e9716e7ba93f",
  "cdiUploadproxy": "sha256:6e0c54351030193e650cce63c86e7bebb51bd109adaa62ccddbbf17a1b5c5e0d",
  "cdiImporter": "sha256:485cc49f4f04effc039edc64a2a9668e7c29a2b3a4416c0c38d9ff8f8dc66ee3",

  "virtOperator": "sha256:9a3fe6731d89e7e06eddbfbad5d480f67760f1fab1444751f1a2f4856b6272d5",
  "virtController": "sha256:d02b4e1451fd5364ebac46f58d2f041e04d2be6872ac3283d0d0a9699b24c765",
  "virtLauncher": "sha256:ba7a680da1f16f206a0f9e98c9627491d0b234b5e65328d47fc7d575d4668f2f",
  "virtExportserver": "sha256:5ec6c2f5723dbbce39ecff8738119e0f9320c61441ae746fd500d585e3b7b726",
  "virtHandler": "sha256:fa424409cfeca48fe487142ef1d60c62e0dcfd6d67a886f8afe686f1ffcd297c",
  "virtExportproxy": "sha256:3a33f8c70f3ea8c7390b6c8889a54c7d7a8c93702afe5e92353271b39514305a",
  "virtApi": "sha256:87a22cfc28f6ed04cb68c2e8c28f136adc81a84851c87b664220b44b4d04022d",

  "kubeApiProxy": "sha256:d6553bfb2a76be0c018c6567bd84eb7b0c5217df07342c585b6f54cc4117285d",

  "cdiArtifact": "sha256:2c27b9115688f0f9da87f978c6f274f757602b8f99294f7b5854d91cd23a6e66",
  "virtArtifact": "sha256:f82b12dcb0ef3da2a5a796b9d3222a8c8f1cd207f6ed991623e0da6679803ff0",
  "stub":"stub"
  }
EOF
)

virtControllerSHA=$(jq -r '.virtController' <<<"$imagesDigests")
virtOperatorSHA=$(jq -r '.virtOperator' <<<"$imagesDigests")
virtAPISHA=$(jq -r '.virtApi' <<<"$imagesDigests")

proxySHA="dev-registry.deckhouse.io/virt/dev/diafour/kube-api-proxy:latest"
#proxySHA=$(jq -r '.kubeApiProxy' <<<"$imagesDigests")

patch=$(cat <<EOF
{"spec":{"template":{"spec":{
          "volumes": [{
            "name":"kube-api-proxy-kubeconfig",
            "configMap": {"name": "kube-api-proxy-kubeconfig" }
          }],
          "containers":[{
            "name":"virt-operator",
            "args": [ "--port", "8443", "-v", "4"],
            "command": ["virt-operator", "--kubeconfig=/kubeconfig.local/proxy.kubeconfig"],
            "image":"dev-registry.deckhouse.io/sys/deckhouse-oss/modules/virtualization@${virtOperatorSHA}",
            "env": [
             {"name": "VIRT_OPERATOR_IMAGE",
              "value": "dev-registry.deckhouse.io/sys/deckhouse-oss/modules/virtualization@${virtOperatorSHA}"
             },
             {"name": "VIRT_CONTROLLER_IMAGE",
              "value": "dev-registry.deckhouse.io/sys/deckhouse-oss/modules/virtualization@${virtControllerSHA}"
             },
             {"name": "VIRT_API_IMAGE",
              "value": "dev-registry.deckhouse.io/sys/deckhouse-oss/modules/virtualization@${virtControllerSHA}"
             }
            ],
            "volumeMounts":[{
              "name": "kube-api-proxy-kubeconfig",
              "mountPath": "/kubeconfig.local"
            }]
          }, {
            "name": "proxy",
            "image": "${proxySHA}",
            "imagePullPolicy": "Always",
            "command": ["/proxy"],
            "securityContext": {
              "allowPrivilegeEscalation": false,
              "capabilities": {"drop": ["ALL"]},
              "seccompProfile": {
                "type": "RuntimeDefault"
              }
            },
            "terminationMessagePath": "/dev/termination-log",
            "terminationMessagePolicy": "File"
          }]
        }}}}
EOF
)

echo Patch Deployment/virt-operator ...
kubectl -n d8-virtualization patch --type=strategic -p "${patch}" deploy/virt-operator

echo Restart Deployment/virt-operator ...
kubectl -n d8-virtualization scale deploy/virt-operator --replicas=0
sleep 2
kubectl -n d8-virtualization scale deploy/virt-operator --replicas=1


#echo $virtControllerSHA $virtOperatorSHA
