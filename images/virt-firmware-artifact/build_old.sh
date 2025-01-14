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
export PACKAGES_PATH="/${gitRepoName}-${versionEdk2}/BaseTools:/edk2-platforms"

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

    # build -a X64 -t GCC5 -p $dsc_file -b RELEASE $build_opts
    # cp $build_dir/RELEASE_GCC5/FV/$(basename $out_code) /FIRMWARE/$(basename $out_code)
    # if [[ -n "$out_vars" ]]; then
    #     cp $build_dir/RELEASE_GCC5/FV/$(basename $out_vars) /FIRMWARE/$(basename $out_vars)
    # fi
    build -a X64 -t GCC5 -p $dsc_file -b RELEASE $build_opts
    cp $build_dir/RELEASE_GCC5/FV/OVMF_CODE.fd /FIRMWARE/$(basename $out_code)
    if [[ -n "$out_vars" ]]; then
        cp $build_dir/RELEASE_GCC5/FV/OVMF_VARS.fd /FIRMWARE/$(basename $out_vars)
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
# build_ovmf "Confidential Computing OVMF" \
#     "/FIRMWARE/OVMF_CODE.cc.fd" \
#     "" \
#     "OvmfPkg/OvmfQemuCc.dsc" \
#     "Build/OvmfQemuCc" \
#     ""

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
    "OvmfPkg/IntelTdx/IntelTdxX64.dsc" \
    "Build/IntelTdx" \
    ""
# "OvmfPkg/OvmfQemuTdx.dsc" \

# Build Intel TDX Secure Boot OVMF
build_ovmf "Intel TDX Secure Boot OVMF" \
    "/FIRMWARE/OVMF.inteltdx.secboot.fd" \
    "" \
    "OvmfPkg/IntelTdx/IntelTdxX64.dsc" \
    "Build/IntelTdx" \
    "-D SECURE_BOOT_ENABLE"
# "Build/OvmfQemuTdx" \
# "OvmfPkg/OvmfQemuTdx.dsc" \

# Build UEFI Shell
build -a X64 -t GCC5 -p ShellPkg/ShellPkg.dsc -b RELEASE
cp Build/Shell/RELEASE_GCC5/X64/ShellPkg/Application/Shell/Shell/OUTPUT/Shell.efi /FIRMWARE/Shell.efi
# cp Build/Shell/RELEASE_GCC5/X64/Shell.efi /FIRMWARE/Shell.efi
rm -rf Build/*

# # Build EnrollDefaultKeys.efi from edk2-apps
# cd /edk2-staging
# source ../${gitRepoName}-${versionEdk2}/edksetup.sh

# Build EnrollDefaultKeys.efi
export BUILD_TYPE=RELEASE
export EDK2_TOOLCHAIN=GCC5
export PCD_RELEASE_DATE=$(date "+%m/%d/%Y")
export PCD_FLAGS='--pcd PcdFirmwareVendor=L"DVP distribution of EDK II\\0" '
export PCD_FLAGS+='--pcd PcdFirmwareVersionString=L"1.0\\0" '
export PCD_FLAGS+='--pcd PcdFirmwareReleaseDateString=L"${PCD_RELEASE_DATE}\\0" '
export COMMON_FLAGS="-DCC_MEASUREMENT_ENABLE=TRUE "
export COMMON_FLAGS+="-DNETWORK_HTTP_BOOT_ENABLE=TRUE "
export COMMON_FLAGS+="-DNETWORK_IP6_ENABLE=TRUE "
export COMMON_FLAGS+="-DNETWORK_TLS_ENABLE "
export COMMON_FLAGS+="${PCD_FLAGS} "
export OVMF_COMMON_FLAGS="${COMMON_FLAGS} "
export OVMF_COMMON_FLAGS+="-DTPM2_ENABLE=TRUE "
export OVMF_4M_FLAGS="${OVMF_COMMON_FLAGS} -DFD_SIZE_4MB "
export OVMF_4M_SECBOOT_FLAGS="${OVMF_4M_FLAGS} -DBUILD_SHELL=FALSE -DSECURE_BOOT_ENABLE=TRUE -DSMM_REQUIRE=TRUE "

build -a X64 -t ${EDK2_TOOLCHAIN} -p OvmfPkg/OvmfPkgX64.dsc ${OVMF_4M_SECBOOT_FLAGS} -b ${BUILD_TYPE}

build -a X64 -t GCC5 -p OvmfPkg/OvmfPkgX64.dsc  -b RELEASE
build -a X64 -t GCC5 -p OvmfPkg/OvmfPkgX64.dsc -m OvmfPkg/EnrollDefaultKeys/EnrollDefaultKeys.inf -b RELEASE
cp Build/SecMainPkg/RELEASE_GCC5/X64/EnrollDefaultKeys.efi /FIRMWARE/EnrollDefaultKeys.efi
rm -rf Build/*

# Build DBXUpdate binary
cd /openssl req -new -x509 -newkey rsa:2048 -subj "/CN=Test DBX Update/" -keyout dbxupdate_key.pem -out dbxupdate_cert.pem -nodes -days 365
sbattach --remove /FIRMWARE/EnrollDefaultKeys.efi
sbattach --attach dbxupdate_cert.pem /FIRMWARE/EnrollDefaultKeys.efi
cp /FIRMWARE/EnrollDefaultKeys.efi /FIRMWARE/DBXUpdate-20230509.x64.bin

# Create UEFI Shell ISO
# mkdir -p /iso/efi/boot
# cp /FIRMWARE/Shell.efi /iso/efi/boot/bootx64.efi
# genisoimage -o /FIRMWARE/UefiShell.iso -efi-boot-part --efi-boot-image -no-emul-boot /iso
# rm -rf /iso

# Create UEFI Shell ISO
# mkdir -p /iso/EFI/BOOT
# cp /FIRMWARE/Shell.efi /iso/EFI/BOOT/BOOTX64.EFI

(
  UEFI_SHELL_BINARY=Build/Ovmf3264/DEBUG_%{TOOLCHAIN}/X64/Shell.efi
  ENROLLER_BINARY=Build/Ovmf3264/DEBUG_%{TOOLCHAIN}/X64/EnrollDefaultKeys.efi
  UEFI_SHELL_IMAGE=uefi_shell.img
  ISO_IMAGE=UefiShell.iso

  UEFI_SHELL_BINARY_BNAME=$(basename -- "$UEFI_SHELL_BINARY")
  UEFI_SHELL_SIZE=$(stat --format=%s -- "$UEFI_SHELL_BINARY")
  ENROLLER_SIZE=$(stat --format=%s -- "$ENROLLER_BINARY")

  # add 1MB then 10% for metadata
  UEFI_SHELL_IMAGE_KB=$((
    (UEFI_SHELL_SIZE + ENROLLER_SIZE + 1 * 1024 * 1024) * 11 / 10 / 1024
  ))

  # create non-partitioned FAT image
  rm -f -- "$UEFI_SHELL_IMAGE"
  mkdosfs -C "$UEFI_SHELL_IMAGE" -n UEFI_SHELL -- "$UEFI_SHELL_IMAGE_KB"

  # copy the shell binary into the FAT image
  export MTOOLS_SKIP_CHECK=1
  mmd   -i "$UEFI_SHELL_IMAGE"                       ::efi
  mmd   -i "$UEFI_SHELL_IMAGE"                       ::efi/boot
  mcopy -i "$UEFI_SHELL_IMAGE"  "$UEFI_SHELL_BINARY" ::efi/boot/bootx64.efi
  mcopy -i "$UEFI_SHELL_IMAGE"  "$ENROLLER_BINARY"   ::
  mdir  -i "$UEFI_SHELL_IMAGE"  -/                   ::

  # build ISO with FAT image file as El Torito EFI boot image
  genisoimage -input-charset ASCII -J -rational-rock \
    -efi-boot "$UEFI_SHELL_IMAGE" -no-emul-boot \
    -o "$ISO_IMAGE" -- "$UEFI_SHELL_IMAGE"
)


# Create a UEFI bootable ISO using xorriso
# xorriso -as mkisofs \
#   -iso-level 3 \
#   -V "UEFI Shell" \
#   -e EFI/BOOT/BOOTX64.EFI \
#   -no-emul-boot \
#   -o /FIRMWARE/UefiShell.iso \
#   /iso

# # Clean up
# rm -rf /iso


# genisoimage -input-charset ASCII -J -rational-rock \
#     -efi-boot "$UEFI_SHELL_IMAGE" -no-emul-boot \
#     -o /FIRMWARE/UefiShell.iso -- "$UEFI_SHELL_IMAGE"


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