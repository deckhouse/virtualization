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

usage() {
    cat <<EOF
    Usage: $0 [OPTIONS]
    Options:
    
    Set the source base directory:      -s, --src-base PATH (example: /mysourcedir)
    Set the build base directory:       -b, --build-dir FOLDER (example: mybuildfolder)
    Set the destination base directory: -d, --dest-base PATH (example: /mydestdir)
    Show this help message and exit:    -h, --help
EOF
    exit 0
}

parse_args() {
    while [[ $# -gt 0 ]]; do
    case "$1" in
        -s|--src-base)
            if [[ -n "$2" && "$2" != "-"* ]]; then
                SRC_BASE="$2"
                shift 2
            else
                echo "Error: Option '$1' requires a non-empty argument."
                usage
            fi
            ;;
        -d|--dest-base)
            if [[ -n "$2" && "$2" != "-"* ]]; then
                DEST_BASE="$2"
                shift 2
            else
                echo "Error: Option '$1' requires a non-empty argument."
                usage
            fi
            ;;
        -b|--build-dir)
            if [[ -n "$2" && "$2" != "-"* ]]; then
                BUILD_DIR="$2"
                shift 2
            else
                echo "Error: Option '$1' requires a non-empty argument."
                usage
            fi
            ;;
        -v|--version-num)
            if [[ -n "$2" && "$2" != "-"* ]]; then
                VERSION_NUM="$2"
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

    if [ -n $BUILD_BASE ]; then
        SRC_BUILD="$SRC_BASE/$BUILD_DIR"
    else
        SRC_BUILD="$SRC_BASE"
    fi
}

parse_args $@

if [ -z $VERSION_NUM ]; then
    echo "Error: Option '--version-num' is missed but required"
    usage
    exit 1
fi
# List of files and destinations of qemu
# Commented lines - binary for additional features. 
# 
# The specific format of the list, 'SOURCE_FILE to DESTINATION', 
# is due to the output of the installation scripts. To make it easier to add them to this list.
FILE_LIST=$(cat <<EOF
$SRC_BUILD/trace/trace-events-all to /usr/share/qemu
$SRC_BUILD/ui-opengl.so to /usr/lib64/qemu
$SRC_BUILD/ui-egl-headless.so to /usr/lib64/qemu
$SRC_BUILD/ui-dbus.so to /usr/lib64/qemu
$SRC_BUILD/hw-display-virtio-gpu.so to /usr/lib64/qemu
$SRC_BUILD/hw-display-virtio-gpu-pci.so to /usr/lib64/qemu
$SRC_BUILD/hw-display-virtio-vga.so to /usr/lib64/qemu
$SRC_BUILD/hw-usb-redirect.so to /usr/lib64/qemu
$SRC_BUILD/hw-usb-host.so to /usr/lib64/qemu
$SRC_BUILD/hw-s390x-virtio-gpu-ccw.so to /usr/lib64/qemu
$SRC_BUILD/audio-pa.so to /usr/lib64/qemu
$SRC_BUILD/audio-pipewire.so to /usr/lib64/qemu
$SRC_BUILD/audio-dbus.so to /usr/lib64/qemu
$SRC_BUILD/block-blkio.so to /usr/lib64/qemu
$SRC_BUILD/block-curl.so to /usr/lib64/qemu
$SRC_BUILD/block-rbd.so to /usr/lib64/qemu
$SRC_BUILD/accel-qtest-x86_64.so to /usr/lib64/qemu
$SRC_BUILD/accel-tcg-x86_64.so to /usr/lib64/qemu
$SRC_BUILD/qemu-system-x86_64 to /usr/bin
# $SRC_BUILD/qemu-system-i386 to /usr/bin
$SRC_BUILD/qga/qemu-ga to /usr/bin
$SRC_BUILD/qemu-keymap to /usr/bin
$SRC_BUILD/qemu-img to /usr/bin
$SRC_BUILD/qemu-io to /usr/bin
$SRC_BUILD/qemu-nbd to /usr/bin
$SRC_BUILD/storage-daemon/qemu-storage-daemon to /usr/bin
$SRC_BUILD/contrib/elf2dmp/elf2dmp to /usr/bin
$SRC_BUILD/qemu-edid to /usr/bin
$SRC_BUILD/contrib/vhost-user-gpu/vhost-user-gpu to /usr/libexec
$SRC_BUILD/qemu-bridge-helper to /usr/libexec
$SRC_BUILD/qemu-pr-helper to /usr/bin
$SRC_BUILD/qemu-vmsr-helper to /usr/bin
$SRC_BUILD/pc-bios/edk2-i386-code.fd to /usr/share/qemu
$SRC_BUILD/pc-bios/edk2-i386-secure-code.fd to /usr/share/qemu
$SRC_BUILD/pc-bios/edk2-i386-vars.fd to /usr/share/qemu
$SRC_BUILD/pc-bios/edk2-x86_64-code.fd to /usr/share/qemu
$SRC_BUILD/pc-bios/edk2-x86_64-secure-code.fd to /usr/share/qemu
$SRC_BUILD/pc-bios/keymaps/en-gb to /usr/share/qemu/keymaps
$SRC_BUILD/pc-bios/keymaps/en-us to /usr/share/qemu/keymaps
$SRC_BUILD/pc-bios/keymaps/ru to /usr/share/qemu/keymaps
$SRC_BASE/include/qemu/qemu-plugin.h to /usr/include
$SRC_BASE/ui/icons/qemu_16x16.png to /usr/share/icons/hicolor/16x16/apps
$SRC_BASE/ui/icons/qemu_24x24.png to /usr/share/icons/hicolor/24x24/apps
$SRC_BASE/ui/icons/qemu_32x32.png to /usr/share/icons/hicolor/32x32/apps
$SRC_BASE/ui/icons/qemu_48x48.png to /usr/share/icons/hicolor/48x48/apps
$SRC_BASE/ui/icons/qemu_64x64.png to /usr/share/icons/hicolor/64x64/apps
$SRC_BASE/ui/icons/qemu_128x128.png to /usr/share/icons/hicolor/128x128/apps
$SRC_BASE/ui/icons/qemu_256x256.png to /usr/share/icons/hicolor/256x256/apps
$SRC_BASE/ui/icons/qemu_512x512.png to /usr/share/icons/hicolor/512x512/apps
$SRC_BASE/ui/icons/qemu_32x32.bmp to /usr/share/icons/hicolor/32x32/apps
$SRC_BASE/ui/icons/qemu.svg to /usr/share/icons/hicolor/scalable/apps
# $SRC_BASE/ui/qemu.desktop to /usr/share/applications
$SRC_BUILD/contrib/vhost-user-gpu/50-qemu-gpu.json to /usr/share/qemu/vhost-user
$SRC_BASE/pc-bios/bios.bin to /usr/share/qemu
$SRC_BASE/pc-bios/bios-256k.bin to /usr/share/qemu
$SRC_BASE/pc-bios/bios-microvm.bin to /usr/share/qemu
$SRC_BASE/pc-bios/qboot.rom to /usr/share/qemu
$SRC_BASE/pc-bios/vgabios.bin to /usr/share/qemu
$SRC_BASE/pc-bios/vgabios-cirrus.bin to /usr/share/qemu
$SRC_BASE/pc-bios/vgabios-stdvga.bin to /usr/share/qemu
$SRC_BASE/pc-bios/vgabios-vmware.bin to /usr/share/qemu
$SRC_BASE/pc-bios/vgabios-qxl.bin to /usr/share/qemu
$SRC_BASE/pc-bios/vgabios-virtio.bin to /usr/share/qemu
$SRC_BASE/pc-bios/vgabios-ramfb.bin to /usr/share/qemu
$SRC_BASE/pc-bios/vgabios-bochs-display.bin to /usr/share/qemu
$SRC_BASE/pc-bios/vgabios-ati.bin to /usr/share/qemu
$SRC_BASE/pc-bios/openbios-sparc32 to /usr/share/qemu
$SRC_BASE/pc-bios/openbios-sparc64 to /usr/share/qemu
$SRC_BASE/pc-bios/openbios-ppc to /usr/share/qemu
$SRC_BASE/pc-bios/QEMU,tcx.bin to /usr/share/qemu
$SRC_BASE/pc-bios/QEMU,cgthree.bin to /usr/share/qemu
$SRC_BASE/pc-bios/pxe-e1000.rom to /usr/share/qemu
$SRC_BASE/pc-bios/pxe-eepro100.rom to /usr/share/qemu
$SRC_BASE/pc-bios/pxe-ne2k_pci.rom to /usr/share/qemu
$SRC_BASE/pc-bios/pxe-pcnet.rom to /usr/share/qemu
$SRC_BASE/pc-bios/pxe-rtl8139.rom to /usr/share/qemu
$SRC_BASE/pc-bios/pxe-virtio.rom to /usr/share/qemu
$SRC_BASE/pc-bios/efi-e1000.rom to /usr/share/qemu
$SRC_BASE/pc-bios/efi-eepro100.rom to /usr/share/qemu
$SRC_BASE/pc-bios/efi-ne2k_pci.rom to /usr/share/qemu
$SRC_BASE/pc-bios/efi-pcnet.rom to /usr/share/qemu
$SRC_BASE/pc-bios/efi-rtl8139.rom to /usr/share/qemu
$SRC_BASE/pc-bios/efi-virtio.rom to /usr/share/qemu
$SRC_BASE/pc-bios/efi-e1000e.rom to /usr/share/qemu
$SRC_BASE/pc-bios/efi-vmxnet3.rom to /usr/share/qemu
$SRC_BASE/pc-bios/qemu-nsis.bmp to /usr/share/qemu
$SRC_BASE/pc-bios/multiboot.bin to /usr/share/qemu
$SRC_BASE/pc-bios/multiboot_dma.bin to /usr/share/qemu
$SRC_BASE/pc-bios/linuxboot.bin to /usr/share/qemu
$SRC_BASE/pc-bios/linuxboot_dma.bin to /usr/share/qemu
$SRC_BASE/pc-bios/kvmvapic.bin to /usr/share/qemu
$SRC_BASE/pc-bios/pvh.bin to /usr/share/qemu
$SRC_BASE/pc-bios/s390-ccw.img to /usr/share/qemu
$SRC_BASE/pc-bios/slof.bin to /usr/share/qemu
$SRC_BASE/pc-bios/skiboot.lid to /usr/share/qemu
$SRC_BASE/pc-bios/palcode-clipper to /usr/share/qemu
$SRC_BASE/pc-bios/u-boot.e500 to /usr/share/qemu
$SRC_BASE/pc-bios/u-boot-sam460-20100605.bin to /usr/share/qemu
$SRC_BASE/pc-bios/qemu_vga.ndrv to /usr/share/qemu
$SRC_BASE/pc-bios/edk2-licenses.txt to /usr/share/qemu
$SRC_BASE/pc-bios/hppa-firmware.img to /usr/share/qemu
$SRC_BASE/pc-bios/hppa-firmware64.img to /usr/share/qemu
$SRC_BASE/pc-bios/npcm7xx_bootrom.bin to /usr/share/qemu
$SRC_BASE/pc-bios/vof.bin to /usr/share/qemu
$SRC_BASE/pc-bios/vof-nvram.bin to /usr/share/qemu
$SRC_BASE/pc-bios/bamboo.dtb to /usr/share/qemu
$SRC_BASE/pc-bios/canyonlands.dtb to /usr/share/qemu
$SRC_BASE/pc-bios/petalogix-s3adsp1800.dtb to /usr/share/qemu
$SRC_BASE/pc-bios/petalogix-ml605.dtb to /usr/share/qemu
$SRC_BUILD/pc-bios/descriptors/50-edk2-i386-secure.json to /usr/share/qemu/firmware
$SRC_BUILD/pc-bios/descriptors/50-edk2-x86_64-secure.json to /usr/share/qemu/firmware
$SRC_BUILD/pc-bios/descriptors/60-edk2-i386.json to /usr/share/qemu/firmware
$SRC_BUILD/pc-bios/descriptors/60-edk2-x86_64.json to /usr/share/qemu/firmware
EOF
)

copy_file() {
    local SOURCE_PATH="$1"
    local dest_dir="$2"

    # Ensure the source file exists
    if [ ! -e "$SOURCE_PATH" ]; then
        echo "Error: Source file not found: $SOURCE_PATH"
        return
    fi

    # Create destination directory if it does not exist
    mkdir -p "$DEST_BASE$dest_dir"

    # Copy the file
    # install -s "$SOURCE_PATH" "$DEST_BASE$dest_dir"
    if ! [[ "$SOURCE_PATH" =~ \.(txt|log|sh|service|conf|xml|target|socket|bin|json|img|png|fd|dtb|rom) ]];then
        strip "$SOURCE_PATH"
    fi
    cp -p "$SOURCE_PATH" "$DEST_BASE$dest_dir"
    echo "Copied $SOURCE_PATH to $DEST_BASE$dest_dir"
}

main() {
    # Read the list and process each line
    while IFS= read -r LINE; do
        # Skip empty lines and comments
        [[ -z "$LINE" ]] && continue
        [[ "$LINE" =~ ^\# ]] && continue

        # Handle file copying
        if [[ "$LINE" =~ ^(.+?)\ to\ (.+)$ ]]; then
            SOURCE_FILE="${BASH_REMATCH[1]}"
            DEST_DIR="${BASH_REMATCH[2]}"
            copy_file "$SOURCE_FILE" "$DEST_DIR"
        else
            echo "Invalid line: $LINE"
        fi

    done <<< "$FILE_LIST"
}

main