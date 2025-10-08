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
convert_version() {
    local version="${1#v}"

    # Split the version string into major, minor, and patch parts
    # and construct the compact version by combining major and zero-padded minor
    IFS='.' read -r major minor patch <<< "$version"
    printf "%d%03d\n" "$major" "$minor"
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

# 10.10.0 -> 10010, 10.0.5 -> 10005
lib_version=$(convert_version $VERSION_NUM)

# List of files and destinations of libvirt
# Commented lines - binary for additional features.
#
# The specific format of the list, 'SOURCE_FILE to DESTINATION',
# is due to the output of the installation scripts. To make it easier to add them to this list.

FILE_LIST=$(cat <<EOF
# $SRC_BUILD/src/libvirt_probes.stp to /usr/share/systemtap/tapset
$SRC_BUILD/src/access/org.libvirt.api.policy to /usr/share/polkit-1/actions
# $SRC_BUILD/src/qemu/libvirt_qemu_probes.stp to /usr/share/systemtap/tapset
$SRC_BUILD/src/libvirt.so.0.${lib_version}.0 to /usr/local/lib64
$SRC_BUILD/src/libvirt-qemu.so.0.${lib_version}.0 to /usr/local/lib64
$SRC_BUILD/src/libvirt-lxc.so.0.${lib_version}.0 to /usr/local/lib64
$SRC_BUILD/src/libvirt-admin.so.0.${lib_version}.0 to /usr/local/lib64
$SRC_BUILD/src/libvirt_driver_interface.so to /usr/lib64/libvirt/connection-driver
$SRC_BUILD/src/lockd.so to /usr/lib64/libvirt/lock-driver
# $SRC_BUILD/src/sanlock.so to /usr/lib64/libvirt/lock-driver
$SRC_BUILD/src/libvirt_driver_network.so to /usr/lib64/libvirt/connection-driver
$SRC_BUILD/src/libvirt_driver_nodedev.so to /usr/lib64/libvirt/connection-driver
$SRC_BUILD/src/libvirt_driver_nwfilter.so to /usr/lib64/libvirt/connection-driver
$SRC_BUILD/src/libvirt_driver_secret.so to /usr/lib64/libvirt/connection-driver
$SRC_BUILD/src/libvirt_driver_storage.so to /usr/lib64/libvirt/connection-driver
$SRC_BUILD/src/libvirt_storage_backend_fs.so to /usr/lib64/libvirt/storage-backend
# $SRC_BUILD/src/libvirt_storage_backend_disk.so to /usr/lib64/libvirt/storage-backend
# $SRC_BUILD/src/libvirt_storage_backend_gluster.so to /usr/lib64/libvirt/storage-backend
# $SRC_BUILD/src/libvirt_storage_backend_iscsi.so to /usr/lib64/libvirt/storage-backend
# $SRC_BUILD/src/libvirt_storage_backend_iscsi-direct.so to /usr/lib64/libvirt/storage-backend
# $SRC_BUILD/src/libvirt_storage_backend_logical.so to /usr/lib64/libvirt/storage-backend
# $SRC_BUILD/src/libvirt_storage_backend_mpath.so to /usr/lib64/libvirt/storage-backend
# $SRC_BUILD/src/libvirt_storage_backend_rbd.so to /usr/lib64/libvirt/storage-backend
# $SRC_BUILD/src/libvirt_storage_backend_scsi.so to /usr/lib64/libvirt/storage-backend
# $SRC_BUILD/src/libvirt_storage_backend_vstorage.so to /usr/lib64/libvirt/storage-backend
# $SRC_BUILD/src/libvirt_storage_backend_zfs.so to /usr/lib64/libvirt/storage-backend
$SRC_BUILD/src/libvirt_storage_file_fs.so to /usr/lib64/libvirt/storage-file
# $SRC_BUILD/src/libvirt_storage_file_gluster.so to /usr/lib64/libvirt/storage-file
# $SRC_BUILD/src/libvirt_driver_lxc.so to /usr/lib64/libvirt/connection-driver
# $SRC_BUILD/src/libvirt_driver_ch.so to /usr/lib64/libvirt/connection-driver
$SRC_BUILD/src/libvirt_driver_qemu.so to /usr/lib64/libvirt/connection-driver
# $SRC_BUILD/src/libvirt_driver_vbox.so to /usr/lib64/libvirt/connection-driver
$SRC_BUILD/src/libvirtd to /usr/sbin
$SRC_BUILD/src/virtproxyd to /usr/sbin
$SRC_BUILD/src/virtinterfaced to /usr/sbin
$SRC_BUILD/src/virtlockd to /usr/sbin
$SRC_BUILD/src/virtlogd to /usr/sbin
$SRC_BUILD/src/virtnetworkd to /usr/sbin
$SRC_BUILD/src/virtnodedevd to /usr/sbin
$SRC_BUILD/src/virtnwfilterd to /usr/sbin
$SRC_BUILD/src/virtsecretd to /usr/sbin
$SRC_BUILD/src/virtstoraged to /usr/sbin
# $SRC_BUILD/src/virtlxcd to /usr/sbin
# $SRC_BUILD/src/virtchd to /usr/sbin
$SRC_BUILD/src/virtqemud to /usr/sbin
# $SRC_BUILD/src/virtvboxd to /usr/sbin
$SRC_BUILD/src/libvirt_iohelper to /usr/libexec
$SRC_BUILD/src/virt-ssh-helper to /usr/bin
# $SRC_BUILD/src/libvirt_sanlock_helper to /usr/libexec
$SRC_BUILD/src/libvirt_leaseshelper to /usr/libexec
# $SRC_BUILD/src/libvirt_parthelper to /usr/libexec
# $SRC_BUILD/src/libvirt_lxc to /usr/libexec
$SRC_BUILD/src/virt-qemu-run to /usr/bin
# $SRC_BUILD/src/test_libvirt_lockd.aug to /usr/share/augeas/lenses/tests
# $SRC_BUILD/src/test_libvirt_sanlock.aug to /usr/share/augeas/lenses/tests
# $SRC_BUILD/src/test_virtlockd.aug to /usr/share/augeas/lenses/tests
$SRC_BUILD/src/test_virtlogd.aug to /usr/share/augeas/lenses/tests
# $SRC_BUILD/src/test_libvirtd_network.aug to /usr/share/augeas/lenses/tests
# $SRC_BUILD/src/test_libvirtd_lxc.aug to /usr/share/augeas/lenses/tests
# $SRC_BUILD/src/test_libvirtd_qemu.aug to /usr/share/augeas/lenses/tests
# $SRC_BUILD/src/test_libvirtd.aug to /usr/share/augeas/lenses/tests
# $SRC_BUILD/src/test_virtproxyd.aug to /usr/share/augeas/lenses/tests
# $SRC_BUILD/src/test_virtinterfaced.aug to /usr/share/augeas/lenses/tests
# $SRC_BUILD/src/test_virtnetworkd.aug to /usr/share/augeas/lenses/tests
# $SRC_BUILD/src/test_virtnodedevd.aug to /usr/share/augeas/lenses/tests
# $SRC_BUILD/src/test_virtnwfilterd.aug to /usr/share/augeas/lenses/tests
# $SRC_BUILD/src/test_virtsecretd.aug to /usr/share/augeas/lenses/tests
# $SRC_BUILD/src/test_virtstoraged.aug to /usr/share/augeas/lenses/tests
# $SRC_BUILD/src/test_virtlxcd.aug to /usr/share/augeas/lenses/tests
# $SRC_BUILD/src/test_virtchd.aug to /usr/share/augeas/lenses/tests
# $SRC_BUILD/src/test_virtqemud.aug to /usr/share/augeas/lenses/tests
# $SRC_BUILD/src/test_virtvboxd.aug to /usr/share/augeas/lenses/tests
$SRC_BUILD/src/libvirt_functions.stp to /usr/share/systemtap/tapset
$SRC_BUILD/tools/virt-host-validate to /usr/bin
# $SRC_BUILD/tools/virt-login-shell to /usr/bin
# $SRC_BUILD/tools/virt-login-shell-helper to /usr/libexec
# $SRC_BUILD/tools/virsh to /usr/bin
# $SRC_BUILD/tools/virt-admin to /usr/bin
$SRC_BUILD/tools/virt-pki-validate to /usr/bin
$SRC_BUILD/tools/virt-pki-query-dn to /usr/bin
$SRC_BUILD/tools/nss/libnss_libvirt.so.2 to /usr/lib64
$SRC_BUILD/tools/nss/libnss_libvirt_guest.so.2 to /usr/lib64
# $SRC_BUILD/tools/wireshark/src/libvirt.so to /usr/lib64/wireshark/plugins/4.4/epan
$SRC_BUILD/tools/ssh-proxy/libvirt-ssh-proxy to /usr/libexec
$SRC_BASE/po/en_GB/LC_MESSAGES/libvirt.mo to /usr/local/share/locale/en_GB/LC_MESSAGES
$SRC_BASE/po/ru/LC_MESSAGES/libvirt.mo to /usr/local/share/locale/ru/LC_MESSAGES
$SRC_BASE/include/libvirt/libvirt-admin.h to /usr/include/libvirt
$SRC_BASE/include/libvirt/libvirt-domain-checkpoint.h to /usr/include/libvirt
$SRC_BASE/include/libvirt/libvirt-domain.h to /usr/include/libvirt
$SRC_BASE/include/libvirt/libvirt-domain-snapshot.h to /usr/include/libvirt
$SRC_BASE/include/libvirt/libvirt-event.h to /usr/include/libvirt
$SRC_BASE/include/libvirt/libvirt.h to /usr/include/libvirt
$SRC_BASE/include/libvirt/libvirt-host.h to /usr/include/libvirt
$SRC_BASE/include/libvirt/libvirt-interface.h to /usr/include/libvirt
$SRC_BASE/include/libvirt/libvirt-lxc.h to /usr/include/libvirt
$SRC_BASE/include/libvirt/libvirt-network.h to /usr/include/libvirt
$SRC_BASE/include/libvirt/libvirt-nodedev.h to /usr/include/libvirt
$SRC_BASE/include/libvirt/libvirt-nwfilter.h to /usr/include/libvirt
$SRC_BASE/include/libvirt/libvirt-qemu.h to /usr/include/libvirt
$SRC_BASE/include/libvirt/libvirt-secret.h to /usr/include/libvirt
$SRC_BASE/include/libvirt/libvirt-storage.h to /usr/include/libvirt
$SRC_BASE/include/libvirt/libvirt-stream.h to /usr/include/libvirt
$SRC_BASE/include/libvirt/virterror.h to /usr/include/libvirt
$SRC_BUILD/include/libvirt/libvirt-common.h to /usr/include/libvirt
# $SRC_BASE/src/cpu_map/arm_a64fx.xml to /usr/share/libvirt/cpu_map
# $SRC_BASE/src/cpu_map/arm_cortex-a53.xml to /usr/share/libvirt/cpu_map
# $SRC_BASE/src/cpu_map/arm_cortex-a57.xml to /usr/share/libvirt/cpu_map
# $SRC_BASE/src/cpu_map/arm_cortex-a72.xml to /usr/share/libvirt/cpu_map
# $SRC_BASE/src/cpu_map/arm_Falkor.xml to /usr/share/libvirt/cpu_map
# $SRC_BASE/src/cpu_map/arm_FT-2000plus.xml to /usr/share/libvirt/cpu_map
# $SRC_BASE/src/cpu_map/arm_features.xml to /usr/share/libvirt/cpu_map
# $SRC_BASE/src/cpu_map/arm_Kunpeng-920.xml to /usr/share/libvirt/cpu_map
# $SRC_BASE/src/cpu_map/arm_Neoverse-N1.xml to /usr/share/libvirt/cpu_map
# $SRC_BASE/src/cpu_map/arm_Neoverse-N2.xml to /usr/share/libvirt/cpu_map
# $SRC_BASE/src/cpu_map/arm_Neoverse-V1.xml to /usr/share/libvirt/cpu_map
# $SRC_BASE/src/cpu_map/arm_Tengyun-S2500.xml to /usr/share/libvirt/cpu_map
# $SRC_BASE/src/cpu_map/arm_ThunderX299xx.xml to /usr/share/libvirt/cpu_map
# $SRC_BASE/src/cpu_map/arm_vendors.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/index.xml to /usr/share/libvirt/cpu_map
# $SRC_BASE/src/cpu_map/ppc64_POWER6.xml to /usr/share/libvirt/cpu_map
# $SRC_BASE/src/cpu_map/ppc64_POWER7.xml to /usr/share/libvirt/cpu_map
# $SRC_BASE/src/cpu_map/ppc64_POWER8.xml to /usr/share/libvirt/cpu_map
# $SRC_BASE/src/cpu_map/ppc64_POWER9.xml to /usr/share/libvirt/cpu_map
# $SRC_BASE/src/cpu_map/ppc64_POWER10.xml to /usr/share/libvirt/cpu_map
# $SRC_BASE/src/cpu_map/ppc64_POWERPC_e5500.xml to /usr/share/libvirt/cpu_map
# $SRC_BASE/src/cpu_map/ppc64_POWERPC_e6500.xml to /usr/share/libvirt/cpu_map
# $SRC_BASE/src/cpu_map/ppc64_vendors.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_486.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_athlon.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Broadwell-IBRS.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Broadwell-noTSX-IBRS.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Broadwell-noTSX.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Broadwell-v1.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Broadwell-v2.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Broadwell-v3.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Broadwell-v4.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Broadwell.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Cascadelake-Server-noTSX.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Cascadelake-Server-v1.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Cascadelake-Server-v2.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Cascadelake-Server-v3.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Cascadelake-Server-v4.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Cascadelake-Server-v5.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Cascadelake-Server.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Conroe.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Cooperlake-v1.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Cooperlake-v2.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Cooperlake.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_core2duo.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_coreduo.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_cpu64-rhel5.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_cpu64-rhel6.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Denverton-v1.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Denverton-v2.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Denverton-v3.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Denverton.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Dhyana-v1.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Dhyana-v2.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Dhyana.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_EPYC-IBPB.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_EPYC-v1.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_EPYC-v2.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_EPYC-v3.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_EPYC-v4.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_EPYC.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_EPYC-Genoa.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_EPYC-Milan-v1.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_EPYC-Milan-v2.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_EPYC-Milan.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_EPYC-Rome-v1.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_EPYC-Rome-v2.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_EPYC-Rome-v3.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_EPYC-Rome-v4.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_EPYC-Rome.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_features.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_GraniteRapids-v1.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_GraniteRapids.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Haswell-IBRS.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Haswell-noTSX-IBRS.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Haswell-noTSX.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Haswell-v1.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Haswell-v2.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Haswell-v3.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Haswell-v4.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Haswell.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Icelake-Client-noTSX.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Icelake-Client.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Icelake-Server-noTSX.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Icelake-Server-v1.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Icelake-Server-v2.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Icelake-Server-v3.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Icelake-Server-v4.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Icelake-Server-v5.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Icelake-Server-v6.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Icelake-Server-v7.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Icelake-Server.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_IvyBridge-IBRS.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_IvyBridge-v1.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_IvyBridge-v2.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_IvyBridge.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_KnightsMill.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_kvm32.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_kvm64.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_n270.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Nehalem-IBRS.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Nehalem-v1.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Nehalem-v2.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Nehalem.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Opteron_G1.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Opteron_G2.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Opteron_G3.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Opteron_G4.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Opteron_G5.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Penryn.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_pentium.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_pentium2.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_pentium3.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_pentiumpro.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_phenom.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_qemu32.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_qemu64.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_SandyBridge-IBRS.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_SandyBridge-v1.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_SandyBridge-v2.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_SandyBridge.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_SapphireRapids-v1.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_SapphireRapids-v2.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_SapphireRapids-v3.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_SapphireRapids.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_SierraForest-v1.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_SierraForest.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Skylake-Client-IBRS.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Skylake-Client-noTSX-IBRS.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Skylake-Client-v1.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Skylake-Client-v2.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Skylake-Client-v3.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Skylake-Client-v4.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Skylake-Client.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Skylake-Server-IBRS.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Skylake-Server-noTSX-IBRS.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Skylake-Server-v1.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Skylake-Server-v2.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Skylake-Server-v3.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Skylake-Server-v4.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Skylake-Server-v5.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Skylake-Server.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Snowridge-v1.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Snowridge-v2.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Snowridge-v3.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Snowridge-v4.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Snowridge.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_vendors.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Westmere-IBRS.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Westmere-v1.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Westmere-v2.xml to /usr/share/libvirt/cpu_map
$SRC_BASE/src/cpu_map/x86_Westmere.xml to /usr/share/libvirt/cpu_map
# $SRC_BASE/src/conf/schemas/basictypes.rng to /usr/share/libvirt/schemas
# $SRC_BASE/src/conf/schemas/capability.rng to /usr/share/libvirt/schemas
# $SRC_BASE/src/conf/schemas/cpu.rng to /usr/share/libvirt/schemas
# $SRC_BASE/src/conf/schemas/cputypes.rng to /usr/share/libvirt/schemas
# $SRC_BASE/src/conf/schemas/domainbackup.rng to /usr/share/libvirt/schemas
# $SRC_BASE/src/conf/schemas/domaincaps.rng to /usr/share/libvirt/schemas
# $SRC_BASE/src/conf/schemas/domaincheckpoint.rng to /usr/share/libvirt/schemas
# $SRC_BASE/src/conf/schemas/domaincommon.rng to /usr/share/libvirt/schemas
# $SRC_BASE/src/conf/schemas/domain.rng to /usr/share/libvirt/schemas
# $SRC_BASE/src/conf/schemas/domainoverrides.rng to /usr/share/libvirt/schemas
# $SRC_BASE/src/conf/schemas/domainsnapshot.rng to /usr/share/libvirt/schemas
# $SRC_BASE/src/conf/schemas/inactiveDomain.rng to /usr/share/libvirt/schemas
# $SRC_BASE/src/conf/schemas/interface.rng to /usr/share/libvirt/schemas
# $SRC_BASE/src/conf/schemas/networkcommon.rng to /usr/share/libvirt/schemas
# $SRC_BASE/src/conf/schemas/networkport.rng to /usr/share/libvirt/schemas
# $SRC_BASE/src/conf/schemas/network.rng to /usr/share/libvirt/schemas
# $SRC_BASE/src/conf/schemas/nodedev.rng to /usr/share/libvirt/schemas
# $SRC_BASE/src/conf/schemas/nwfilterbinding.rng to /usr/share/libvirt/schemas
# $SRC_BASE/src/conf/schemas/nwfilter_params.rng to /usr/share/libvirt/schemas
# $SRC_BASE/src/conf/schemas/nwfilter.rng to /usr/share/libvirt/schemas
# $SRC_BASE/src/conf/schemas/privatedata.rng to /usr/share/libvirt/schemas
# $SRC_BASE/src/conf/schemas/secret.rng to /usr/share/libvirt/schemas
# $SRC_BASE/src/conf/schemas/storagecommon.rng to /usr/share/libvirt/schemas
# $SRC_BASE/src/conf/schemas/storagepoolcaps.rng to /usr/share/libvirt/schemas
# $SRC_BASE/src/conf/schemas/storagepool.rng to /usr/share/libvirt/schemas
# $SRC_BASE/src/conf/schemas/storagevol.rng to /usr/share/libvirt/schemas
# $SRC_BUILD/src/remote/libvirtd.qemu.logrotate to /etc/logrotate.d
# $SRC_BUILD/src/remote/libvirtd.lxc.logrotate to /etc/logrotate.d
# $SRC_BUILD/src/remote/libvirtd.libxl.logrotate to /etc/logrotate.d
# $SRC_BUILD/src/remote/libvirtd.logrotate to /etc/logrotate.d
$SRC_BASE/src/remote/libvirtd.sysctl to /usr/lib/sysctl.d
$SRC_BASE/src/remote/libvirtd.policy to /usr/share/polkit-1/actions
$SRC_BASE/src/remote/libvirtd.rules to /usr/share/polkit-1/rules.d
$SRC_BASE/src/remote/libvirtd.sasl to /etc/sasl2
# $SRC_BUILD/src/network/default.xml to /etc/libvirt/qemu/networks
# $SRC_BASE/src/network/libvirt.zone to /usr/lib/firewalld/zones
# $SRC_BASE/src/network/libvirt-routed.zone to /usr/lib/firewalld/zones
# $SRC_BASE/src/network/libvirt-to-host.policy to /usr/lib/firewalld/policies
# $SRC_BASE/src/network/libvirt-routed-out.policy to /usr/lib/firewalld/policies
# $SRC_BASE/src/network/libvirt-routed-in.policy to /usr/lib/firewalld/policies
# $SRC_BASE/src/nwfilter/xml/allow-arp.xml to /etc/libvirt/nwfilter
# $SRC_BASE/src/nwfilter/xml/allow-dhcp-server.xml to /etc/libvirt/nwfilter
# $SRC_BASE/src/nwfilter/xml/allow-dhcp.xml to /etc/libvirt/nwfilter
# $SRC_BASE/src/nwfilter/xml/allow-dhcpv6-server.xml to /etc/libvirt/nwfilter
# $SRC_BASE/src/nwfilter/xml/allow-dhcpv6.xml to /etc/libvirt/nwfilter
# $SRC_BASE/src/nwfilter/xml/allow-incoming-ipv4.xml to /etc/libvirt/nwfilter
# $SRC_BASE/src/nwfilter/xml/allow-incoming-ipv6.xml to /etc/libvirt/nwfilter
# $SRC_BASE/src/nwfilter/xml/allow-ipv4.xml to /etc/libvirt/nwfilter
# $SRC_BASE/src/nwfilter/xml/allow-ipv6.xml to /etc/libvirt/nwfilter
# $SRC_BASE/src/nwfilter/xml/clean-traffic-gateway.xml to /etc/libvirt/nwfilter
# $SRC_BASE/src/nwfilter/xml/clean-traffic.xml to /etc/libvirt/nwfilter
# $SRC_BASE/src/nwfilter/xml/no-arp-ip-spoofing.xml to /etc/libvirt/nwfilter
# $SRC_BASE/src/nwfilter/xml/no-arp-mac-spoofing.xml to /etc/libvirt/nwfilter
# $SRC_BASE/src/nwfilter/xml/no-arp-spoofing.xml to /etc/libvirt/nwfilter
# $SRC_BASE/src/nwfilter/xml/no-ip-multicast.xml to /etc/libvirt/nwfilter
# $SRC_BASE/src/nwfilter/xml/no-ip-spoofing.xml to /etc/libvirt/nwfilter
# $SRC_BASE/src/nwfilter/xml/no-ipv6-multicast.xml to /etc/libvirt/nwfilter
# $SRC_BASE/src/nwfilter/xml/no-ipv6-spoofing.xml to /etc/libvirt/nwfilter
# $SRC_BASE/src/nwfilter/xml/no-mac-broadcast.xml to /etc/libvirt/nwfilter
# $SRC_BASE/src/nwfilter/xml/no-mac-spoofing.xml to /etc/libvirt/nwfilter
# $SRC_BASE/src/nwfilter/xml/no-other-l2-traffic.xml to /etc/libvirt/nwfilter
# $SRC_BASE/src/nwfilter/xml/no-other-rarp-traffic.xml to /etc/libvirt/nwfilter
# $SRC_BASE/src/nwfilter/xml/qemu-announce-self-rarp.xml to /etc/libvirt/nwfilter
# $SRC_BASE/src/nwfilter/xml/qemu-announce-self.xml to /etc/libvirt/nwfilter
# $SRC_BASE/src/qemu/libvirt-qemu.sysusers.conf to /usr/lib/sysusers.d
# $SRC_BASE/src/qemu/postcopy-migration.sysctl to /usr/lib/sysctl.d
# $SRC_BASE/src/test/test-screenshot.png to /usr/share/libvirt
# $SRC_BASE/src/admin/libvirt-admin.conf to /etc/libvirt
# $SRC_BUILD/src/locking/qemu-lockd.conf to /etc/libvirt
# $SRC_BUILD/src/locking/qemu-sanlock.conf to /etc/libvirt
# $SRC_BASE/src/locking/virtlockd.conf to /etc/libvirt
$SRC_BASE/src/logging/virtlogd.conf to /etc/libvirt
$SRC_BUILD/src/network/network.conf to /etc/libvirt
# $SRC_BASE/src/lxc/lxc.conf to /etc/libvirt
# $SRC_BUILD/src/qemu/qemu.conf to /etc/libvirt
$SRC_BASE/src/libvirt.conf to /etc/libvirt
# $SRC_BASE/src/locking/libvirt_lockd.aug to /usr/share/augeas/lenses
# $SRC_BASE/src/locking/libvirt_sanlock.aug to /usr/share/augeas/lenses
# $SRC_BASE/src/locking/virtlockd.aug to /usr/share/augeas/lenses
# $SRC_BASE/src/logging/virtlogd.aug to /usr/share/augeas/lenses
# $SRC_BASE/src/network/libvirtd_network.aug to /usr/share/augeas/lenses
# $SRC_BASE/src/lxc/libvirtd_lxc.aug to /usr/share/augeas/lenses
# $SRC_BASE/src/qemu/libvirtd_qemu.aug to /usr/share/augeas/lenses
$SRC_BUILD/src/libvirtd.conf to /etc/libvirt
# $SRC_BUILD/src/libvirtd.aug to /usr/share/augeas/lenses
$SRC_BUILD/src/virtproxyd.conf to /etc/libvirt
# $SRC_BUILD/src/virtproxyd.aug to /usr/share/augeas/lenses
$SRC_BUILD/src/virtinterfaced.conf to /etc/libvirt
# $SRC_BUILD/src/virtinterfaced.aug to /usr/share/augeas/lenses
# $SRC_BUILD/src/virtnetworkd.conf to /etc/libvirt
# $SRC_BUILD/src/virtnetworkd.aug to /usr/share/augeas/lenses
$SRC_BUILD/src/virtnodedevd.conf to /etc/libvirt
# $SRC_BUILD/src/virtnodedevd.aug to /usr/share/augeas/lenses
$SRC_BUILD/src/virtnwfilterd.conf to /etc/libvirt
# $SRC_BUILD/src/virtnwfilterd.aug to /usr/share/augeas/lenses
# $SRC_BUILD/src/virtsecretd.conf to /etc/libvirt
# $SRC_BUILD/src/virtsecretd.aug to /usr/share/augeas/lenses
# $SRC_BUILD/src/virtstoraged.conf to /etc/libvirt
# $SRC_BUILD/src/virtstoraged.aug to /usr/share/augeas/lenses
# $SRC_BUILD/src/virtlxcd.conf to /etc/libvirt
# $SRC_BUILD/src/virtlxcd.aug to /usr/share/augeas/lenses
# $SRC_BUILD/src/virtchd.conf to /etc/libvirt
# $SRC_BUILD/src/virtchd.aug to /usr/share/augeas/lenses
# $SRC_BUILD/src/virtqemud.conf to /etc/libvirt
# $SRC_BUILD/src/virtqemud.aug to /usr/share/augeas/lenses
# $SRC_BUILD/src/virtvboxd.conf to /etc/libvirt
# $SRC_BUILD/src/virtvboxd.aug to /usr/share/augeas/lenses
$SRC_BASE/src/remote/virt-guest-shutdown.target to /usr/lib/systemd/system
# $SRC_BUILD/src/libvirtd.service to /usr/lib/systemd/system
$SRC_BUILD/src/libvirtd.socket to /usr/lib/systemd/system
$SRC_BUILD/src/libvirtd-ro.socket to /usr/lib/systemd/system
$SRC_BUILD/src/libvirtd-admin.socket to /usr/lib/systemd/system
$SRC_BUILD/src/libvirtd-tcp.socket to /usr/lib/systemd/system
$SRC_BUILD/src/libvirtd-tls.socket to /usr/lib/systemd/system
# $SRC_BUILD/src/virtproxyd.service to /usr/lib/systemd/system
$SRC_BUILD/src/virtproxyd.socket to /usr/lib/systemd/system
$SRC_BUILD/src/virtproxyd-ro.socket to /usr/lib/systemd/system
$SRC_BUILD/src/virtproxyd-admin.socket to /usr/lib/systemd/system
$SRC_BUILD/src/virtproxyd-tcp.socket to /usr/lib/systemd/system
$SRC_BUILD/src/virtproxyd-tls.socket to /usr/lib/systemd/system
# $SRC_BUILD/src/virtinterfaced.service to /usr/lib/systemd/system
$SRC_BUILD/src/virtinterfaced.socket to /usr/lib/systemd/system
$SRC_BUILD/src/virtinterfaced-ro.socket to /usr/lib/systemd/system
$SRC_BUILD/src/virtinterfaced-admin.socket to /usr/lib/systemd/system
# $SRC_BUILD/src/virtlockd.service to /usr/lib/systemd/system
$SRC_BUILD/src/virtlockd.socket to /usr/lib/systemd/system
$SRC_BUILD/src/virtlockd-admin.socket to /usr/lib/systemd/system
# $SRC_BUILD/src/virtlogd.service to /usr/lib/systemd/system
# $SRC_BUILD/src/virtlogd.socket to /usr/lib/systemd/system
# $SRC_BUILD/src/virtlogd-admin.socket to /usr/lib/systemd/system
# $SRC_BUILD/src/virtnetworkd.service to /usr/lib/systemd/system
$SRC_BUILD/src/virtnetworkd.socket to /usr/lib/systemd/system
$SRC_BUILD/src/virtnetworkd-ro.socket to /usr/lib/systemd/system
$SRC_BUILD/src/virtnetworkd-admin.socket to /usr/lib/systemd/system
# $SRC_BUILD/src/virtnodedevd.service to /usr/lib/systemd/system
$SRC_BUILD/src/virtnodedevd.socket to /usr/lib/systemd/system
$SRC_BUILD/src/virtnodedevd-ro.socket to /usr/lib/systemd/system
$SRC_BUILD/src/virtnodedevd-admin.socket to /usr/lib/systemd/system
# $SRC_BUILD/src/virtnwfilterd.service to /usr/lib/systemd/system
# $SRC_BUILD/src/virtnwfilterd.socket to /usr/lib/systemd/system
# $SRC_BUILD/src/virtnwfilterd-ro.socket to /usr/lib/systemd/system
# $SRC_BUILD/src/virtnwfilterd-admin.socket to /usr/lib/systemd/system
# $SRC_BUILD/src/virtsecretd.service to /usr/lib/systemd/system
$SRC_BUILD/src/virtsecretd.socket to /usr/lib/systemd/system
$SRC_BUILD/src/virtsecretd-ro.socket to /usr/lib/systemd/system
$SRC_BUILD/src/virtsecretd-admin.socket to /usr/lib/systemd/system
# $SRC_BUILD/src/virtstoraged.service to /usr/lib/systemd/system
$SRC_BUILD/src/virtstoraged.socket to /usr/lib/systemd/system
$SRC_BUILD/src/virtstoraged-ro.socket to /usr/lib/systemd/system
$SRC_BUILD/src/virtstoraged-admin.socket to /usr/lib/systemd/system
# $SRC_BUILD/src/virtlxcd.service to /usr/lib/systemd/system
# $SRC_BUILD/src/virtlxcd.socket to /usr/lib/systemd/system
# $SRC_BUILD/src/virtlxcd-ro.socket to /usr/lib/systemd/system
# $SRC_BUILD/src/virtlxcd-admin.socket to /usr/lib/systemd/system
# $SRC_BUILD/src/virtchd.service to /usr/lib/systemd/system
$SRC_BUILD/src/virtchd.socket to /usr/lib/systemd/system
$SRC_BUILD/src/virtchd-ro.socket to /usr/lib/systemd/system
$SRC_BUILD/src/virtchd-admin.socket to /usr/lib/systemd/system
# $SRC_BUILD/src/virtqemud.service to /usr/lib/systemd/system
$SRC_BUILD/src/virtqemud.socket to /usr/lib/systemd/system
$SRC_BUILD/src/virtqemud-ro.socket to /usr/lib/systemd/system
$SRC_BUILD/src/virtqemud-admin.socket to /usr/lib/systemd/system
# $SRC_BUILD/src/virtvboxd.service to /usr/lib/systemd/system
# $SRC_BUILD/src/virtvboxd.socket to /usr/lib/systemd/system
# $SRC_BUILD/src/virtvboxd-ro.socket to /usr/lib/systemd/system
# $SRC_BUILD/src/virtvboxd-admin.socket to /usr/lib/systemd/system
$SRC_BASE/tools/virt-login-shell.conf to /etc/libvirt
$SRC_BUILD/tools/virt-xml-validate to /usr/bin
# $SRC_BUILD/tools/virt-sanlock-cleanup to /usr/sbin
$SRC_BASE/tools/virt-qemu-sev-validate to /usr/bin
$SRC_BUILD/tools/libvirt-guests.sh to /usr/libexec
# $SRC_BUILD/tools/libvirt-guests.service to /usr/lib/systemd/system
# $SRC_BASE/tools/virt-qemu-qmp-proxy to /usr/bin
# $SRC_BUILD/tools/bash-completion/virsh to /usr/share/bash-completion/completions
# $SRC_BUILD/tools/bash-completion/virt-admin to /usr/share/bash-completion/completions
# $SRC_BUILD/tools/ssh-proxy/30-libvirt-ssh-proxy.conf to /etc/ssh/ssh_config.d
$SRC_BUILD/libvirt.pc to /usr/lib64/pkgconfig
$SRC_BUILD/libvirt-qemu.pc to /usr/lib64/pkgconfig
$SRC_BUILD/libvirt-lxc.pc to /usr/lib64/pkgconfig
$SRC_BUILD/libvirt-admin.pc to /usr/lib64/pkgconfig
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
