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
    //tools/csv-generator/... \
    //tools/cdi-containerimage-server/... \
    //tools/cdi-image-size-detection/... \
    //tools/cdi-source-update-poller/... \
    //cmd/...
