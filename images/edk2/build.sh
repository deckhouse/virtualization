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

# UEFI Revocation List File can be downloaded from https://uefi.org

usage() {
    cat <<EOF
    Usage: $0 [OPTIONS]
    Options:
    
    Set brranch:                        --branch (example: v2.1 2.3 stable2024)
    Set repository name:                --repo-name (example: edk2 libvirt etc)
    Show this help message and exit:    -h, --help
EOF
    exit 0
}

echo_dbg() {
  local str=$1
  echo ""
  echo "===$str==="
  echo ""
}

parse_args() {
    while [[ $# -gt 0 ]]; do
    case "$1" in
        --debug)
            set -x 
            shift
            ;;
        --branch)
            if [[ -n "$2" && "$2" != "-"* ]]; then
                edk2Branch="$2"
                shift 2
            else
                echo "Error: Option '$1' requires a non-empty argument."
                usage
            fi
            ;;
        --repo-name)
            if [[ -n "$2" && "$2" != "-"* ]]; then
                gitRepoName="$2"
                shift 2
            else
                echo "Error: Option '$1' requires a non-empty argument."
                usage
            fi
            ;;
        -h|--help)
            usage
            ;;
        *)
            echo "Error: Unknown option '$1'"
            usage
            ;;
        esac
    done
}

parse_args $@

if [[ -z "$edk2Branch" ]]; then
    echo "Error: Option '--branch' is missed but required"
    usage
    exit 1
fi

if [[ -z "$gitRepoName" ]]; then
    echo "Error: Option '--repo-name' is missed but required"
    usage
    exit 1
fi

EDK2_DIR="/${gitRepoName}-${edk2Branch}"
FIRMWARE="/FIRMWARE"

mv -f /Logo.bmp $EDK2_DIR/MdeModulePkg/Logo/
echo "=== cd $EDK2_DIR ==="
cd $EDK2_DIR

mkdir -p ${FIRMWARE}

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
OVMF_4M_FLAGS="${CC_FLAGS} -D FD_SIZE_4MB=TRUE -D NETWORK_TLS_ENABLE=TRUE -D NETWORK_ISCSI_ENABLE=TRUE"

# secure boot features
OVMF_SB_FLAGS="${OVMF_SB_FLAGS} -D SECURE_BOOT_ENABLE=TRUE"
OVMF_SB_FLAGS="${OVMF_SB_FLAGS} -D SMM_REQUIRE=TRUE"
OVMF_SB_FLAGS="${OVMF_SB_FLAGS} -D EXCLUDE_SHELL_FROM_FD=TRUE -D BUILD_SHELL=FALSE"

if ! command -v build 2>&1 >/dev/null
then
    echo "build could not be found"
    exit 1
fi

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

prep() {
  build -a X64 -p MdeModulePkg/MdeModulePkg.dsc -t GCC5 -b RELEASE
}

# Build with SB and SMM; exclude UEFI shell.
build_ovmf() {
  # echo_dbg "build ${OVMF_4M_FLAGS} -a X64 -p OvmfPkg/OvmfPkgX64.dsc"
  build -a X64 \
    -t GCC5 \
    -p OvmfPkg/OvmfPkgX64.dsc \
    -DCC_MEASUREMENT_ENABLE=TRUE -DNETWORK_HTTP_BOOT_ENABLE=TRUE -DNETWORK_IP6_ENABLE=TRUE -DNETWORK_TLS_ENABLE --pcd PcdFirmwareVendor=L"DVP distribution of EDK II\\0" --pcd PcdFirmwareVersionString=L"2025.02-1\\0" --pcd PcdFirmwareReleaseDateString=L"03/02/2025\\0" -DTPM2_ENABLE=TRUE -DFD_SIZE_4MB -b RELEASE
    cp -p Build/OvmfX64/*/FV/OVMF_CODE.fd $FIRMWARE/OVMF_CODE.fd
    cp -p Build/OvmfX64/*/FV/OVMF_VARS.fd $FIRMWARE/OVMF_VARS.fd
  # build ${OVMF_4M_FLAGS} \
  #   -a X64 -p OvmfPkg/OvmfPkgX64.dsc \
  #   -DCC_MEASUREMENT_ENABLE=TRUE \
  #   --pcd PcdFirmwareVendor=L"DVP distribution of EDK II\\0" \
  #   --pcd PcdFirmwareVersionString=L"2025.02-1\\0" \
  #   --pcd PcdFirmwareReleaseDateString=L"03/02/2025\\0"
  
  # cp -p Build/OvmfX64/*/FV/OVMF_CODE.fd $FIRMWARE/OVMF_CODE.fd
  # cp -p Build/OvmfX64/*/FV/OVMF_VARS.fd $FIRMWARE/OVMF_VARS.fd
}

# Build with SB and SMM with secure boot; exclude UEFI shell.
build_ovmf_secboot() {
  # echo_dbg "build ${OVMF_4M_FLAGS} ${OVMF_SB_FLAGS} -a X64 -p OvmfPkg/OvmfPkgX64.dsc"
  build -a X64 \
		-t GCC5 \
		-p OvmfPkg/OvmfPkgX64.dsc \
		-DCC_MEASUREMENT_ENABLE=TRUE -DNETWORK_HTTP_BOOT_ENABLE=TRUE -DNETWORK_IP6_ENABLE=TRUE -DNETWORK_TLS_ENABLE --pcd PcdFirmwareVendor=L"DVP distribution of EDK II\\0" --pcd PcdFirmwareVersionString=L"2025.02-1\\0" --pcd PcdFirmwareReleaseDateString=L"03/02/2025\\0" -DTPM2_ENABLE=TRUE -DFD_SIZE_4MB -DBUILD_SHELL=FALSE -DSECURE_BOOT_ENABLE=TRUE -DSMM_REQUIRE=TRUE -b RELEASE
    cp -p Build/OvmfX64/*/FV/OVMF_CODE.fd           $FIRMWARE/OVMF_CODE.secboot.fd
    cp -p Build/OvmfX64/*/FV/OVMF_VARS.fd           $FIRMWARE/OVMF_VARS.secboot.fd
    cp -p Build/OvmfX64/*/X64/EnrollDefaultKeys.efi $FIRMWARE/
    cp -p Build/OvmfX64/*/X64/Shell.efi             $FIRMWARE/
  # build ${OVMF_4M_FLAGS} ${OVMF_SB_FLAGS} \
  #   -a X64 -p OvmfPkg/OvmfPkgX64.dsc \
  #   --pcd PcdFirmwareVendor=L"DVP distribution of EDK II\\0" \
  #   --pcd PcdFirmwareVersionString=L"2025.02-1\\0" \
  #   --pcd PcdFirmwareReleaseDateString=L"03/02/2025\\0"
    
  # cp -p Build/OvmfX64/*/FV/OVMF_CODE.fd           $FIRMWARE/OVMF_CODE.secboot.fd
  # cp -p Build/OvmfX64/*/FV/OVMF_VARS.fd           $FIRMWARE/OVMF_VARS.secboot.fd
  # cp -p Build/OvmfX64/*/X64/EnrollDefaultKeys.efi $FIRMWARE/
  # cp -p Build/OvmfX64/*/X64/Shell.efi             $FIRMWARE/
}

# Build AmdSev and IntelTdx variants
build_ovmf_amdsev() {
  touch OvmfPkg/AmdSev/Grub/grub.efi

  
  build ${OVMF_4M_FLAGS} -a X64 -p OvmfPkg/AmdSev/AmdSevX64.dsc \
    --pcd PcdFirmwareVendor=L"DVP distribution of EDK II\\0" \
    --pcd PcdFirmwareVersionString=L"2025.02-1\\0" \
    --pcd PcdFirmwareReleaseDateString=L"03/02/2025\\0"

  cp -p Build/AmdSev/*/FV/OVMF.fd $FIRMWARE/OVMF.amdsev.fd
}

build_ovmf_inteltdx() {
  build ${OVMF_4M_FLAGS} -a X64 -p OvmfPkg/IntelTdx/IntelTdxX64.dsc \
    --pcd PcdFirmwareVendor=L"DVP distribution of EDK II\\0" \
    --pcd PcdFirmwareVersionString=L"2025.02-1\\0" \
    --pcd PcdFirmwareReleaseDateString=L"03/02/2025\\0"
  cp -p Build/IntelTdx/*/FV/OVMF.fd $FIRMWARE/OVMF.inteltdx.fd
}

# Build ovmf (x64) shell iso with EnrollDefaultKeys
build_shell() {
  echo_dbg "build shell"
  build ${OVMF_4M_FLAGS} -a X64 -p ShellPkg/ShellPkg.dsc
  build ${OVMF_4M_FLAGS} -a IA32 -p ShellPkg/ShellPkg.dsc

  cp -p Build/Shell/*/X64/ShellPkg/Application/Shell/Shell/OUTPUT/Shell.efi $FIRMWARE/
  cp -p Build/OvmfX64/*/X64/EnrollDefaultKeys.efi $FIRMWARE/
}


enroll() {
  virt-fw-vars --input  $FIRMWARE/OVMF_VARS.fd \
              --output  $FIRMWARE/OVMF_VARS.secboot.fd \
              --set-dbx $FIRMWARE/DBXUpdate-20230509.x64.bin \
              --secure-boot --enroll-generate dvp.deckhouse.io

  virt-fw-vars --input  $FIRMWARE/OVMF.inteltdx.fd \
              --output  $FIRMWARE/OVMF.inteltdx.secboot.fd \
              --set-dbx $FIRMWARE/DBXUpdate-20230509.x64.bin \
              --secure-boot --enroll-generate dvp.deckhouse.io
}

# no sec boot but makes json happy
no_enroll() {
  cp -p $FIRMWARE/OVMF_VARS.fd $FIRMWARE/OVMF_VARS.secboot.fd
  cp -p $FIRMWARE/OVMF.inteltdx.fd $FIRMWARE/OVMF.inteltdx.secboot.fd  
}


echo_dbg "prep"
prep 2>&1 > /dev/null

echo_dbg "build_ovmf"
build_ovmf 2>&1 > /dev/null

echo_dbg "build_ovmf_secboot"
build_ovmf_secboot 2>&1 > /dev/null

echo "build_ovmf_amdsev"
build_ovmf_amdsev 2>&1 > /dev/null

echo "build_ovmf_inteltdx"
build_ovmf_inteltdx 2>&1 > /dev/null

build_iso $FIRMWARE
ls -la $FIRMWARE
# enroll
# no_enroll
