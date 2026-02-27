#!/usr/bin/env bash

# Copyright 2026 Flant JSC
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
# Build usbip-core, usbip-host, vhci-hcd for the running kernel and put .ko into a tmp dir.
# Needs: kernel headers (e.g. linux-headers-$(uname -r)), make, gcc. No full kernel source.
#
# Usage:
#   ./build-usbip-modules.sh [OUTPUT_DIR]
#   OUTPUT_DIR defaults to /tmp/usbip-modules-$(uname -r)
#   USBIP_SRC defaults to the directory where this script lives (driver source).
#
# Env:
#   KVER       - kernel version to build for (default: uname -r)
#   USBIP_SRC  - path to driver source (must contain .c and Makefile.standalone)
#   OUTPUT_DIR - output directory (overrides optional argument)
#
# Minimal deps: bash, make, gcc, kernel headers package (e.g. linux-headers-${KVER}).

set -e

KVER="${KVER:-$(uname -r)}"
for base in /lib/modules /usr/lib/modules; do
  [[ -d "${base}/${KVER}/build" || -L "${base}/${KVER}/build" ]] || continue
  KBUILD="${base}/${KVER}/build"
  break
done
KBUILD="${KBUILD:-/lib/modules/${KVER}/build}"
USBIP_SRC="${USBIP_SRC:-$(cd "$(dirname "$0")" && pwd)}"
OUTPUT_DIR="${OUTPUT_DIR:-${1:-/tmp/usbip-modules-${KVER}}}"

if [[ ! -d "$KBUILD" && ! -L "$KBUILD" ]]; then
  echo "build-usbip-modules: kernel build dir not found for ${KVER}" >&2
  echo "Install kernel headers (kernel-devel or linux-headers-${KVER}) on the host." >&2
  exit 1
fi

# Resolve symlink so make sees the real path (must be visible in container)
if [[ -L "$KBUILD" ]]; then
  KBUILD=$(readlink -f "$KBUILD")
fi
if [[ ! -d "$KBUILD" ]]; then
  echo "build-usbip-modules: build is a symlink but its target is not visible in the container." >&2
  echo "  resolved path: $KBUILD" >&2
  echo "Mount the kernel build tree from the host, e.g.:" >&2
  echo "  -v /usr/src/kernels:/usr/src/kernels:ro" >&2
  exit 1
fi

if [[ ! -f "$USBIP_SRC/Makefile" ]]; then
  echo "build-usbip-modules: $USBIP_SRC/Makefile not found" >&2
  exit 1
fi

make -C "$KBUILD" M="$USBIP_SRC" CC="${CC:-gcc}" modules

mkdir -p "$OUTPUT_DIR"
cp -f "$USBIP_SRC"/usbip-core.ko "$USBIP_SRC"/usbip-host.ko "$USBIP_SRC"/vhci-hcd.ko "$OUTPUT_DIR/"

echo "Built for ${KVER}; modules in: ${OUTPUT_DIR}"
ls -la "$OUTPUT_DIR"/*.ko
