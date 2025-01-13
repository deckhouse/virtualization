#!/bin/bash

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

set -e

# Source edksetup.sh
versionEdk2=stable202411
gitRepoName=edk2

export EDK_TOOLS_PATH="/${gitRepoName}-${versionEdk2}/BaseTools"
export PACKAGES_PATH="/${gitRepoName}-${versionEdk2}/BaseTools:/edk2-platforms:/edk2-staging"

cd "/${gitRepoName}-${versionEdk2}"
. edksetup.sh

# Ensure the Build directory is clean
rm -rf Build/*

# Build OVMF firmware
build_ovmf() {
    local target=$1
    local out_code=$2
    local out_vars=$3
    local dsc_file=$4
    local build_dir=$5
    local build_opts=$6

    build -a X64 -t GCC5 -p $dsc_file -b RELEASE $build_opts
    cp $build_dir/RELEASE_GCC5/FV/$(basename $out_code) /FIRMWARE/$(basename $out_code)
    if [[ -n "$out_vars" ]]; then
        cp $build_dir/RELEASE_GCC5/FV/$(basename $out_vars) /FIRMWARE/$(basename $out_vars)
    fi
    rm -rf Build/*
}

# Build Standard OVMF
build_ovmf "Standard OVMF" \
    "/FIRMWARE/OVMF_CODE.fd" \
    "/FIRMWARE/OVMF_VARS.fd" \
    "OvmfPkg/OvmfPkgX64.dsc" \
    "Build/OvmfX64" \
    ""

# Build Secure Boot OVMF
build_ovmf "Secure Boot OVMF" \
    "/FIRMWARE/OVMF_CODE.secboot.fd" \
    "/FIRMWARE/OVMF_VARS.secboot.fd" \
    "OvmfPkg/OvmfPkgX64.dsc" \
    "Build/OvmfX64" \
    "-D SECURE_BOOT_ENABLE"

# Build Confidential Computing OVMF
build_ovmf "Confidential Computing OVMF" \
    "/FIRMWARE/OVMF_CODE.cc.fd" \
    "" \
    "OvmfPkg/OvmfQemuCc.dsc" \
    "Build/OvmfQemuCc" \
    ""

# Build AMD SEV OVMF
build_ovmf "AMD SEV OVMF" \
    "/FIRMWARE/OVMF.amdsev.fd" \
    "" \
    "OvmfPkg/OvmfPkgX64.dsc" \
    "Build/OvmfX64" \
    "-D AMD_SEV=TRUE"

# Build Intel TDX OVMF
build_ovmf "Intel TDX OVMF" \
    "/FIRMWARE/OVMF.inteltdx.fd" \
    "" \
    "OvmfPkg/OvmfQemuTdx.dsc" \
    "Build/OvmfQemuTdx" \
    ""

# Build Intel TDX Secure Boot OVMF
build_ovmf "Intel TDX Secure Boot OVMF" \
    "/FIRMWARE/OVMF.inteltdx.secboot.fd" \
    "" \
    "OvmfPkg/OvmfQemuTdx.dsc" \
    "Build/OvmfQemuTdx" \
    "-D SECURE_BOOT_ENABLE"

# Build UEFI Shell
build -a X64 -t GCC5 -p ShellPkg/ShellPkg.dsc -b RELEASE
cp Build/Shell/RELEASE_GCC5/X64/Shell.efi /FIRMWARE/Shell.efi
rm -rf Build/*

# Create UEFI Shell ISO
mkdir -p /iso/efi/boot
cp /FIRMWARE/Shell.efi /iso/efi/boot/bootx64.efi
genisoimage -o /FIRMWARE/UefiShell.iso -efi-boot-part --efi-boot-image -no-emul-boot /iso
rm -rf /iso

# Build EnrollDefaultKeys.efi from edk2-apps
cd /edk2-apps
. ../edk2/edksetup.sh

# Build EnrollDefaultKeys.efi
build -a X64 -t GCC5 -p SecMainPkg/SecMainPkg.dsc -m SecureBootEnrollDefaultKeys/EnrollDefaultKeys.inf -b RELEASE
cp Build/SecMainPkg/RELEASE_GCC5/X64/EnrollDefaultKeys.efi /FIRMWARE/EnrollDefaultKeys.efi
rm -rf Build/*

# Build DBXUpdate binary
cd /openssl req -new -x509 -newkey rsa:2048 -subj "/CN=Test DBX Update/" -keyout dbxupdate_key.pem -out dbxupdate_cert.pem -nodes -days 365
sbattach --remove /FIRMWARE/EnrollDefaultKeys.efi
sbattach --attach dbxupdate_cert.pem /FIRMWARE/EnrollDefaultKeys.efi
cp /FIRMWARE/EnrollDefaultKeys.efi /FIRMWARE/DBXUpdate-20230509.x64.bin