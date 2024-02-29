#!/bin/bash
set -eu -o pipefail

kubectl delete -n d8-virtualization kubevirts.x.virtualization.deckhouse.io kubevirt
kubectl delete cdis.x.virtualization.deckhouse.io cdi
kubectl wait --for=delete cdis.x.virtualization.deckhouse.io cdi --timeout=180s
kubectl wait --for=delete -n d8-virtualization kubevirts.x.virtualization.deckhouse.io kubevirt --timeout=180s
