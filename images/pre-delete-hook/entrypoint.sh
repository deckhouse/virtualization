#!/bin/bash
set -eu -o pipefail

KUBEVIRT_RESOURCE="dvpinternalkubevirts.internal.virtualization.deckhouse.io"
echo "Delete Kubevirt configuration ..."
kubectl delete -n d8-virtualization ${KUBEVIRT_RESOURCE} kubevirt || true
echo "Wait for Kubevirt deletion ..."
kubectl wait --for=delete -n d8-virtualization ${KUBEVIRT_RESOURCE} kubevirt --timeout=180s || true

CDI_RESOURCE="dvpinternalcdis.internal.virtualization.deckhouse.io"
echo "Delete CDI configuration ..."
kubectl delete ${CDI_RESOURCE} cdi || true
echo "Wait for CDI deletion ..."
kubectl wait --for=delete ${CDI_RESOURCE} cdi --timeout=180s || true
