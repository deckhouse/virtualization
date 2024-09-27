#!/bin/bash

set -e

source hack/build/common.sh
source hack/build/config.sh

rm -rf ${CMD_OUT_DIR}
mkdir -p ${CMD_OUT_DIR}/dump

# Build all binaries for amd64
bazel build \
    --verbose_failures \
    --config=${ARCHITECTURE} \
    --sandbox_debug \
    //tools/csv-generator:csv-generator \
    //tools/cdi-containerimage-server:cdi-containerimage-server \
    //tools/cdi-image-size-detection:cdi-image-size-detection \
    //tools/cdi-source-update-poller:cdi-source-update-poller \
    //cmd/cdi-apiserver:cdi-apiserver \
    //cmd/cdi-cloner:cdi-cloner \
    //cmd/cdi-controller:cdi-controller \
    //cmd/cdi-importer:cdi-importer \
    //cmd/cdi-operator:cdi-operator \
    //cmd/cdi-uploadproxy:cdi-uploadproxy \
    //cmd/cdi-uploadserver:cdi-uploadserver
