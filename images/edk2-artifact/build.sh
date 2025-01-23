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

# set -e

versionEdk2="stable202411"
gitRepoName="edk2"
EDK2_DIR="/${gitRepoName}-${versionEdk2}"
FIRMWARE="/FIRMWARE"

DBXDATE="20230509"
UEFI_BIN_BASE_URL="https://uefi.org/sites/default/files/resources"

cp -f Logo.bmp $EDK2_DIR/MdeModulePkg/Logo/
cd $EDK2_DIR

mkdir -p ${FIRMWARE}

download_DBXUpdate() {
    local dst_dir=$1

    if [ -z $dst_dir ];then dst_dir="$FIRMWARE"; fi

    # curl -L $UEFI_BIN_BASE_URL/x86_DBXUpdate$DBXDATE.bin -o $dst_dir/DBXUpdate-$DBXDATE.x86.bin
    curl -L $UEFI_BIN_BASE_URL/x64_DBXUpdate_$DBXDATE.bin -o $dst_dir/DBXUpdate-$DBXDATE.x64.bin
}

echo_dbg() {
  local str=$1
  echo ""
  echo "===$str==="
  echo ""
}

# compiler
CC_FLAGS="-t GCC5"
CC_FLAGS="${CC_FLAGS} -b RELEASE"

CC_FLAGS="${CC_FLAGS} --cmd-len=65536"
CC_FLAGS="${CC_FLAGS} -D NETWORK_IP6_ENABLE=TRUE"
CC_FLAGS="${CC_FLAGS} -D NETWORK_HTTP_BOOT_ENABLE=TRUE -D NETWORK_ALLOW_HTTP_CONNECTIONS=TRUE"
CC_FLAGS="${CC_FLAGS} -D TPM2_ENABLE=TRUE -D TPM2_CONFIG_ENABLE=TRUE"
CC_FLAGS="${CC_FLAGS} -D TPM1_ENABLE=FALSE"
CC_FLAGS="${CC_FLAGS} -D CAVIUM_ERRATUM_27456=TRUE"

# ovmf features
OVMF_2M_FLAGS="${CC_FLAGS} -D FD_SIZE_2MB=TRUE -D NETWORK_TLS_ENABLE=FALSE -D NETWORK_ISCSI_ENABLE=FALSE"
OVMF_4M_FLAGS="${CC_FLAGS} -D FD_SIZE_4MB=TRUE -D NETWORK_TLS_ENABLE=TRUE -D NETWORK_ISCSI_ENABLE=TRUE"

# secure boot features
OVMF_SB_FLAGS="${OVMF_SB_FLAGS} -D SECURE_BOOT_ENABLE=TRUE"
OVMF_SB_FLAGS="${OVMF_SB_FLAGS} -D SMM_REQUIRE=TRUE"
OVMF_SB_FLAGS="${OVMF_SB_FLAGS} -D EXCLUDE_SHELL_FROM_FD=TRUE -D BUILD_SHELL=FALSE"

unset MAKEFLAGS

. edksetup.sh

build_iso() {
  dir="$1"
  UEFI_SHELL_BINARY=${dir}/Shell.efi
  ENROLLER_BINARY=${dir}/EnrollDefaultKeys.efi
  UEFI_SHELL_IMAGE=uefi_shell.img
  ISO_IMAGE=${dir}/UefiShell.iso

  UEFI_SHELL_BINARY_BNAME=$(basename -- "$UEFI_SHELL_BINARY")
  UEFI_SHELL_SIZE=$(stat --format=%s -- "$UEFI_SHELL_BINARY")
  ENROLLER_SIZE=$(stat --format=%s -- "$ENROLLER_BINARY")

  # add 1MB then 10 percent for metadata
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
  xorrisofs -input-charset ASCII -J -rational-rock \
    -e "$UEFI_SHELL_IMAGE" -no-emul-boot \
    -o "$ISO_IMAGE" "$UEFI_SHELL_IMAGE"
}

# Build with neither SB nor SMM; include UEFI shell.
# mkdir -p OVMF
echo_dbg "build ${OVMF_2M_FLAGS} -a X64 -p OvmfPkg/OvmfPkgX64.dsc"
build ${OVMF_2M_FLAGS} -a X64 -p OvmfPkg/OvmfPkgX64.dsc
cp -p Build/OvmfX64/*/FV/OVMF_CODE.fd $FIRMWARE/OVMF_CODE.fd
cp -p Build/OvmfX64/*/FV/OVMF_VARS.fd $FIRMWARE/OVMF_VARS.fd

# Build 4MB with neither SB nor SMM; include UEFI shell.
echo_dbg "build ${OVMF_4M_FLAGS} -a X64 -p OvmfPkg/OvmfPkgX64.dsc"
build ${OVMF_4M_FLAGS} -a X64 -p OvmfPkg/OvmfPkgX64.dsc
cp -p Build/OvmfX64/*/FV/OVMF_CODE.fd $FIRMWARE/OVMF_CODE_4M.fd
cp -p Build/OvmfX64/*/FV/OVMF_VARS.fd $FIRMWARE/OVMF_VARS_4M.fd

# Build with SB and SMM; exclude UEFI shell.
echo_dbg "build ${OVMF_2M_FLAGS} ${OVMF_SB_FLAGS} -a X64 -p OvmfPkg/OvmfPkgX64.dsc"
build ${OVMF_2M_FLAGS} ${OVMF_SB_FLAGS} -a X64 -p OvmfPkg/OvmfPkgX64.dsc
cp -p Build/OvmfX64/*/FV/OVMF_CODE.fd $FIRMWARE/OVMF_CODE.secboot.fd

# Build 4MB with SB and SMM; exclude UEFI shell.
echo_dbg "build ${OVMF_4M_FLAGS} ${OVMF_SB_FLAGS} -a X64 -p OvmfPkg/OvmfPkgX64.dsc"
build ${OVMF_4M_FLAGS} ${OVMF_SB_FLAGS} -a X64 -p OvmfPkg/OvmfPkgX64.dsc
cp -p Build/OvmfX64/*/FV/OVMF_CODE.fd $FIRMWARE/OVMF_CODE_4M.secboot.fd

# Build AmdSev and IntelTdx variants
touch OvmfPkg/AmdSev/Grub/grub.efi   # dummy

echo_dbg "build ${OVMF_2M_FLAGS} -a X64 -p OvmfPkg/AmdSev/AmdSevX64.dsc"
build ${OVMF_2M_FLAGS} -a X64 -p OvmfPkg/AmdSev/AmdSevX64.dsc
cp -p Build/AmdSev/*/FV/OVMF.fd $FIRMWARE/OVMF.amdsev.fd

echo_dbg "build ${OVMF_2M_FLAGS} -a X64 -p OvmfPkg/IntelTdx/IntelTdxX64.dsc"
build ${OVMF_2M_FLAGS} -a X64 -p OvmfPkg/IntelTdx/IntelTdxX64.dsc
cp -p Build/IntelTdx/*/FV/OVMF.fd $FIRMWARE/OVMF.inteltdx.fd

# build shell
echo_dbg "build shell"
build ${OVMF_2M_FLAGS} -a X64 -p ShellPkg/ShellPkg.dsc
build ${OVMF_2M_FLAGS} -a IA32 -p ShellPkg/ShellPkg.dsc

# build ovmf (x64) shell iso with EnrollDefaultKeys
#cp Build/Ovmf3264/*/X64/Shell.efi $FIRMWARE/
cp -p Build/Shell/*/X64/ShellPkg/Application/Shell/Shell/OUTPUT/Shell.efi $FIRMWARE/
cp -p Build/OvmfX64/*/X64/EnrollDefaultKeys.efi $FIRMWARE/

build_iso $FIRMWARE
download_DBXUpdate

enroll() {
  virt-fw-vars --input   $FIRMWARE/OVMF_VARS.fd \
              --output  $FIRMWARE/OVMF_VARS.secboot.fd \
              --set-dbx $FIRMWARE/DBXUpdate-$DBXDATE.x64.bin \
              --secure-boot 
# --enroll-altlinux 
# --distro-keys altlinux

  virt-fw-vars --input   $FIRMWARE/OVMF_VARS_4M.fd \
              --output  $FIRMWARE/OVMF_VARS_4M.secboot.fd \
              --set-dbx $FIRMWARE/DBXUpdate-$DBXDATE.x64.bin \
              --secure-boot 
# --enroll-altlinux 
# --distro-keys altlinux

  virt-fw-vars --input   $FIRMWARE/OVMF.inteltdx.fd \
              --output  $FIRMWARE/OVMF.inteltdx.secboot.fd \
              --set-dbx $FIRMWARE/DBXUpdate-$DBXDATE.x64.bin \
              --secure-boot 
# --enroll-altlinux 
# --distro-keys altlinux
}

enroll

# cp -p $FIRMWARE/OVMF_VARS.fd $FIRMWARE/OVMF_VARS.secboot.fd
# cp -p $FIRMWARE/OVMF_VARS_4M.fd $FIRMWARE/OVMF_VARS_4M.secboot.fd
# cp -p $FIRMWARE/OVMF.inteltdx.fd $FIRMWARE/OVMF.inteltdx.secboot.fd


# build microvm
echo_dbg "build ${OVMF_2M_FLAGS} -a X64 -p OvmfPkg/Microvm/MicrovmX64.dsc"
build ${OVMF_2M_FLAGS} -a X64 -p OvmfPkg/Microvm/MicrovmX64.dsc
cp -p Build/MicrovmX64/*/FV/MICROVM.fd $FIRMWARE
