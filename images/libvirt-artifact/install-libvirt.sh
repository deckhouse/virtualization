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

# List of files and destinations
FILE_LIST=$(cat <<'EOF'
src/libvirt_probes.stp to /usr/share/systemtap/tapset
src/access/org.libvirt.api.policy to /usr/share/polkit-1/actions
src/qemu/libvirt_qemu_probes.stp to /usr/share/systemtap/tapset
src/libvirt.so.0.10010.0 to /usr/local/lib64
src/libvirt-qemu.so.0.10010.0 to /usr/local/lib64
src/libvirt-lxc.so.0.10010.0 to /usr/local/lib64
src/libvirt-admin.so.0.10010.0 to /usr/local/lib64
src/libvirt_driver_interface.so to /usr/lib64/libvirt/connection-driver
src/lockd.so to /usr/lib64/libvirt/lock-driver
src/sanlock.so to /usr/lib64/libvirt/lock-driver
src/libvirt_driver_network.so to /usr/lib64/libvirt/connection-driver
src/libvirt_driver_nodedev.so to /usr/lib64/libvirt/connection-driver
src/libvirt_driver_nwfilter.so to /usr/lib64/libvirt/connection-driver
src/libvirt_driver_secret.so to /usr/lib64/libvirt/connection-driver
src/libvirt_driver_storage.so to /usr/lib64/libvirt/connection-driver
src/libvirt_storage_backend_fs.so to /usr/lib64/libvirt/storage-backend
src/libvirt_storage_backend_disk.so to /usr/lib64/libvirt/storage-backend
src/libvirt_storage_backend_gluster.so to /usr/lib64/libvirt/storage-backend
src/libvirt_storage_backend_iscsi.so to /usr/lib64/libvirt/storage-backend
src/libvirt_storage_backend_iscsi-direct.so to /usr/lib64/libvirt/storage-backend
src/libvirt_storage_backend_logical.so to /usr/lib64/libvirt/storage-backend
src/libvirt_storage_backend_mpath.so to /usr/lib64/libvirt/storage-backend
src/libvirt_storage_backend_rbd.so to /usr/lib64/libvirt/storage-backend
src/libvirt_storage_backend_scsi.so to /usr/lib64/libvirt/storage-backend
src/libvirt_storage_backend_vstorage.so to /usr/lib64/libvirt/storage-backend
src/libvirt_storage_backend_zfs.so to /usr/lib64/libvirt/storage-backend
src/libvirt_storage_file_fs.so to /usr/lib64/libvirt/storage-file
src/libvirt_storage_file_gluster.so to /usr/lib64/libvirt/storage-file
src/libvirt_driver_lxc.so to /usr/lib64/libvirt/connection-driver
src/libvirt_driver_ch.so to /usr/lib64/libvirt/connection-driver
src/libvirt_driver_qemu.so to /usr/lib64/libvirt/connection-driver
src/libvirt_driver_vbox.so to /usr/lib64/libvirt/connection-driver
src/libvirtd to /usr/sbin
src/virtproxyd to /usr/sbin
src/virtinterfaced to /usr/sbin
src/virtlockd to /usr/sbin
src/virtlogd to /usr/sbin
src/virtnetworkd to /usr/sbin
src/virtnodedevd to /usr/sbin
src/virtnwfilterd to /usr/sbin
src/virtsecretd to /usr/sbin
src/virtstoraged to /usr/sbin
src/virtlxcd to /usr/sbin
src/virtchd to /usr/sbin
src/virtqemud to /usr/sbin
src/virtvboxd to /usr/sbin
src/libvirt_iohelper to /usr/libexec
src/virt-ssh-helper to /usr/bin
src/libvirt_sanlock_helper to /usr/libexec
src/libvirt_leaseshelper to /usr/libexec
src/libvirt_parthelper to /usr/libexec
src/libvirt_lxc to /usr/libexec
src/virt-qemu-run to /usr/bin
src/test_libvirt_lockd.aug to /usr/share/augeas/lenses/tests
src/test_libvirt_sanlock.aug to /usr/share/augeas/lenses/tests
src/test_virtlockd.aug to /usr/share/augeas/lenses/tests
src/test_virtlogd.aug to /usr/share/augeas/lenses/tests
src/test_libvirtd_network.aug to /usr/share/augeas/lenses/tests
src/test_libvirtd_lxc.aug to /usr/share/augeas/lenses/tests
src/test_libvirtd_qemu.aug to /usr/share/augeas/lenses/tests
src/test_libvirtd.aug to /usr/share/augeas/lenses/tests
src/test_virtproxyd.aug to /usr/share/augeas/lenses/tests
src/test_virtinterfaced.aug to /usr/share/augeas/lenses/tests
src/test_virtnetworkd.aug to /usr/share/augeas/lenses/tests
src/test_virtnodedevd.aug to /usr/share/augeas/lenses/tests
src/test_virtnwfilterd.aug to /usr/share/augeas/lenses/tests
src/test_virtsecretd.aug to /usr/share/augeas/lenses/tests
src/test_virtstoraged.aug to /usr/share/augeas/lenses/tests
src/test_virtlxcd.aug to /usr/share/augeas/lenses/tests
src/test_virtchd.aug to /usr/share/augeas/lenses/tests
src/test_virtqemud.aug to /usr/share/augeas/lenses/tests
src/test_virtvboxd.aug to /usr/share/augeas/lenses/tests
src/libvirt_functions.stp to /usr/share/systemtap/tapset
tools/virt-host-validate to /usr/bin
tools/virt-login-shell to /usr/bin
tools/virt-login-shell-helper to /usr/libexec
tools/virsh to /usr/bin
tools/virt-admin to /usr/bin
tools/virt-pki-validate to /usr/bin
tools/virt-pki-query-dn to /usr/bin
tools/nss/libnss_libvirt.so.2 to /usr/lib64
tools/nss/libnss_libvirt_guest.so.2 to /usr/lib64
tools/wireshark/src/libvirt.so to /usr/lib64/wireshark/plugins/4.4/epan
tools/ssh-proxy/libvirt-ssh-proxy to /usr/libexec
po/as/LC_MESSAGES/libvirt.mo to /usr/local/share/locale/as/LC_MESSAGES
po/bg/LC_MESSAGES/libvirt.mo to /usr/local/share/locale/bg/LC_MESSAGES
po/bn_IN/LC_MESSAGES/libvirt.mo to /usr/local/share/locale/bn_IN/LC_MESSAGES
po/bs/LC_MESSAGES/libvirt.mo to /usr/local/share/locale/bs/LC_MESSAGES
po/ca/LC_MESSAGES/libvirt.mo to /usr/local/share/locale/ca/LC_MESSAGES
po/cs/LC_MESSAGES/libvirt.mo to /usr/local/share/locale/cs/LC_MESSAGES
po/da/LC_MESSAGES/libvirt.mo to /usr/local/share/locale/da/LC_MESSAGES
po/de/LC_MESSAGES/libvirt.mo to /usr/local/share/locale/de/LC_MESSAGES
po/el/LC_MESSAGES/libvirt.mo to /usr/local/share/locale/el/LC_MESSAGES
po/en_GB/LC_MESSAGES/libvirt.mo to /usr/local/share/locale/en_GB/LC_MESSAGES
po/es/LC_MESSAGES/libvirt.mo to /usr/local/share/locale/es/LC_MESSAGES
po/fi/LC_MESSAGES/libvirt.mo to /usr/local/share/locale/fi/LC_MESSAGES
po/fr/LC_MESSAGES/libvirt.mo to /usr/local/share/locale/fr/LC_MESSAGES
po/gu/LC_MESSAGES/libvirt.mo to /usr/local/share/locale/gu/LC_MESSAGES
po/hi/LC_MESSAGES/libvirt.mo to /usr/local/share/locale/hi/LC_MESSAGES
po/hu/LC_MESSAGES/libvirt.mo to /usr/local/share/locale/hu/LC_MESSAGES
po/id/LC_MESSAGES/libvirt.mo to /usr/local/share/locale/id/LC_MESSAGES
po/it/LC_MESSAGES/libvirt.mo to /usr/local/share/locale/it/LC_MESSAGES
po/ja/LC_MESSAGES/libvirt.mo to /usr/local/share/locale/ja/LC_MESSAGES
po/ka/LC_MESSAGES/libvirt.mo to /usr/local/share/locale/ka/LC_MESSAGES
po/kn/LC_MESSAGES/libvirt.mo to /usr/local/share/locale/kn/LC_MESSAGES
po/ko/LC_MESSAGES/libvirt.mo to /usr/local/share/locale/ko/LC_MESSAGES
po/mk/LC_MESSAGES/libvirt.mo to /usr/local/share/locale/mk/LC_MESSAGES
po/ml/LC_MESSAGES/libvirt.mo to /usr/local/share/locale/ml/LC_MESSAGES
po/mr/LC_MESSAGES/libvirt.mo to /usr/local/share/locale/mr/LC_MESSAGES
po/ms/LC_MESSAGES/libvirt.mo to /usr/local/share/locale/ms/LC_MESSAGES
po/nb/LC_MESSAGES/libvirt.mo to /usr/local/share/locale/nb/LC_MESSAGES
po/nl/LC_MESSAGES/libvirt.mo to /usr/local/share/locale/nl/LC_MESSAGES
po/or/LC_MESSAGES/libvirt.mo to /usr/local/share/locale/or/LC_MESSAGES
po/pa/LC_MESSAGES/libvirt.mo to /usr/local/share/locale/pa/LC_MESSAGES
po/pl/LC_MESSAGES/libvirt.mo to /usr/local/share/locale/pl/LC_MESSAGES
po/pt/LC_MESSAGES/libvirt.mo to /usr/local/share/locale/pt/LC_MESSAGES
po/pt_BR/LC_MESSAGES/libvirt.mo to /usr/local/share/locale/pt_BR/LC_MESSAGES
po/ru/LC_MESSAGES/libvirt.mo to /usr/local/share/locale/ru/LC_MESSAGES
po/si/LC_MESSAGES/libvirt.mo to /usr/local/share/locale/si/LC_MESSAGES
po/sr/LC_MESSAGES/libvirt.mo to /usr/local/share/locale/sr/LC_MESSAGES
po/sr@latin/LC_MESSAGES/libvirt.mo to /usr/local/share/locale/sr@latin/LC_MESSAGES
po/sv/LC_MESSAGES/libvirt.mo to /usr/local/share/locale/sv/LC_MESSAGES
po/ta/LC_MESSAGES/libvirt.mo to /usr/local/share/locale/ta/LC_MESSAGES
po/te/LC_MESSAGES/libvirt.mo to /usr/local/share/locale/te/LC_MESSAGES
po/tr/LC_MESSAGES/libvirt.mo to /usr/local/share/locale/tr/LC_MESSAGES
po/uk/LC_MESSAGES/libvirt.mo to /usr/local/share/locale/uk/LC_MESSAGES
po/vi/LC_MESSAGES/libvirt.mo to /usr/local/share/locale/vi/LC_MESSAGES
po/zh_CN/LC_MESSAGES/libvirt.mo to /usr/local/share/locale/zh_CN/LC_MESSAGES
po/zh_TW/LC_MESSAGES/libvirt.mo to /usr/local/share/locale/zh_TW/LC_MESSAGES
po/hr/LC_MESSAGES/libvirt.mo to /usr/local/share/locale/hr/LC_MESSAGES
po/ro/LC_MESSAGES/libvirt.mo to /usr/local/share/locale/ro/LC_MESSAGES
/home/builder/libvirt-10.10.0/include/libvirt/libvirt-admin.h to /usr/include/libvirt
/home/builder/libvirt-10.10.0/include/libvirt/libvirt-domain-checkpoint.h to /usr/include/libvirt
/home/builder/libvirt-10.10.0/include/libvirt/libvirt-domain.h to /usr/include/libvirt
/home/builder/libvirt-10.10.0/include/libvirt/libvirt-domain-snapshot.h to /usr/include/libvirt
/home/builder/libvirt-10.10.0/include/libvirt/libvirt-event.h to /usr/include/libvirt
/home/builder/libvirt-10.10.0/include/libvirt/libvirt.h to /usr/include/libvirt
/home/builder/libvirt-10.10.0/include/libvirt/libvirt-host.h to /usr/include/libvirt
/home/builder/libvirt-10.10.0/include/libvirt/libvirt-interface.h to /usr/include/libvirt
/home/builder/libvirt-10.10.0/include/libvirt/libvirt-lxc.h to /usr/include/libvirt
/home/builder/libvirt-10.10.0/include/libvirt/libvirt-network.h to /usr/include/libvirt
/home/builder/libvirt-10.10.0/include/libvirt/libvirt-nodedev.h to /usr/include/libvirt
/home/builder/libvirt-10.10.0/include/libvirt/libvirt-nwfilter.h to /usr/include/libvirt
/home/builder/libvirt-10.10.0/include/libvirt/libvirt-qemu.h to /usr/include/libvirt
/home/builder/libvirt-10.10.0/include/libvirt/libvirt-secret.h to /usr/include/libvirt
/home/builder/libvirt-10.10.0/include/libvirt/libvirt-storage.h to /usr/include/libvirt
/home/builder/libvirt-10.10.0/include/libvirt/libvirt-stream.h to /usr/include/libvirt
/home/builder/libvirt-10.10.0/include/libvirt/virterror.h to /usr/include/libvirt
/home/builder/libvirt-10.10.0/build/include/libvirt/libvirt-common.h to /usr/include/libvirt
/home/builder/libvirt-10.10.0/src/cpu_map/arm_a64fx.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/arm_cortex-a53.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/arm_cortex-a57.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/arm_cortex-a72.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/arm_Falkor.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/arm_FT-2000plus.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/arm_features.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/arm_Kunpeng-920.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/arm_Neoverse-N1.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/arm_Neoverse-N2.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/arm_Neoverse-V1.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/arm_Tengyun-S2500.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/arm_ThunderX299xx.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/arm_vendors.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/index.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/ppc64_POWER6.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/ppc64_POWER7.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/ppc64_POWER8.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/ppc64_POWER9.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/ppc64_POWER10.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/ppc64_POWERPC_e5500.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/ppc64_POWERPC_e6500.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/ppc64_vendors.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_486.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_athlon.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Broadwell-IBRS.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Broadwell-noTSX-IBRS.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Broadwell-noTSX.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Broadwell-v1.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Broadwell-v2.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Broadwell-v3.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Broadwell-v4.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Broadwell.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Cascadelake-Server-noTSX.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Cascadelake-Server-v1.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Cascadelake-Server-v2.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Cascadelake-Server-v3.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Cascadelake-Server-v4.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Cascadelake-Server-v5.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Cascadelake-Server.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Conroe.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Cooperlake-v1.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Cooperlake-v2.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Cooperlake.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_core2duo.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_coreduo.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_cpu64-rhel5.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_cpu64-rhel6.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Denverton-v1.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Denverton-v2.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Denverton-v3.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Denverton.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Dhyana-v1.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Dhyana-v2.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Dhyana.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_EPYC-IBPB.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_EPYC-v1.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_EPYC-v2.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_EPYC-v3.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_EPYC-v4.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_EPYC.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_EPYC-Genoa.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_EPYC-Milan-v1.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_EPYC-Milan-v2.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_EPYC-Milan.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_EPYC-Rome-v1.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_EPYC-Rome-v2.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_EPYC-Rome-v3.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_EPYC-Rome-v4.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_EPYC-Rome.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_features.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_GraniteRapids-v1.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_GraniteRapids.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Haswell-IBRS.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Haswell-noTSX-IBRS.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Haswell-noTSX.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Haswell-v1.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Haswell-v2.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Haswell-v3.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Haswell-v4.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Haswell.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Icelake-Client-noTSX.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Icelake-Client.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Icelake-Server-noTSX.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Icelake-Server-v1.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Icelake-Server-v2.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Icelake-Server-v3.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Icelake-Server-v4.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Icelake-Server-v5.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Icelake-Server-v6.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Icelake-Server-v7.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Icelake-Server.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_IvyBridge-IBRS.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_IvyBridge-v1.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_IvyBridge-v2.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_IvyBridge.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_KnightsMill.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_kvm32.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_kvm64.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_n270.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Nehalem-IBRS.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Nehalem-v1.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Nehalem-v2.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Nehalem.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Opteron_G1.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Opteron_G2.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Opteron_G3.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Opteron_G4.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Opteron_G5.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Penryn.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_pentium.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_pentium2.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_pentium3.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_pentiumpro.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_phenom.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_qemu32.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_qemu64.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_SandyBridge-IBRS.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_SandyBridge-v1.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_SandyBridge-v2.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_SandyBridge.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_SapphireRapids-v1.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_SapphireRapids-v2.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_SapphireRapids-v3.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_SapphireRapids.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_SierraForest-v1.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_SierraForest.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Skylake-Client-IBRS.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Skylake-Client-noTSX-IBRS.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Skylake-Client-v1.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Skylake-Client-v2.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Skylake-Client-v3.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Skylake-Client-v4.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Skylake-Client.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Skylake-Server-IBRS.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Skylake-Server-noTSX-IBRS.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Skylake-Server-v1.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Skylake-Server-v2.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Skylake-Server-v3.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Skylake-Server-v4.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Skylake-Server-v5.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Skylake-Server.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Snowridge-v1.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Snowridge-v2.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Snowridge-v3.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Snowridge-v4.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Snowridge.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_vendors.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Westmere-IBRS.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Westmere-v1.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Westmere-v2.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/cpu_map/x86_Westmere.xml to /usr/share/libvirt/cpu_map
/home/builder/libvirt-10.10.0/src/conf/schemas/basictypes.rng to /usr/share/libvirt/schemas
/home/builder/libvirt-10.10.0/src/conf/schemas/capability.rng to /usr/share/libvirt/schemas
/home/builder/libvirt-10.10.0/src/conf/schemas/cpu.rng to /usr/share/libvirt/schemas
/home/builder/libvirt-10.10.0/src/conf/schemas/cputypes.rng to /usr/share/libvirt/schemas
/home/builder/libvirt-10.10.0/src/conf/schemas/domainbackup.rng to /usr/share/libvirt/schemas
/home/builder/libvirt-10.10.0/src/conf/schemas/domaincaps.rng to /usr/share/libvirt/schemas
/home/builder/libvirt-10.10.0/src/conf/schemas/domaincheckpoint.rng to /usr/share/libvirt/schemas
/home/builder/libvirt-10.10.0/src/conf/schemas/domaincommon.rng to /usr/share/libvirt/schemas
/home/builder/libvirt-10.10.0/src/conf/schemas/domain.rng to /usr/share/libvirt/schemas
/home/builder/libvirt-10.10.0/src/conf/schemas/domainoverrides.rng to /usr/share/libvirt/schemas
/home/builder/libvirt-10.10.0/src/conf/schemas/domainsnapshot.rng to /usr/share/libvirt/schemas
/home/builder/libvirt-10.10.0/src/conf/schemas/inactiveDomain.rng to /usr/share/libvirt/schemas
/home/builder/libvirt-10.10.0/src/conf/schemas/interface.rng to /usr/share/libvirt/schemas
/home/builder/libvirt-10.10.0/src/conf/schemas/networkcommon.rng to /usr/share/libvirt/schemas
/home/builder/libvirt-10.10.0/src/conf/schemas/networkport.rng to /usr/share/libvirt/schemas
/home/builder/libvirt-10.10.0/src/conf/schemas/network.rng to /usr/share/libvirt/schemas
/home/builder/libvirt-10.10.0/src/conf/schemas/nodedev.rng to /usr/share/libvirt/schemas
/home/builder/libvirt-10.10.0/src/conf/schemas/nwfilterbinding.rng to /usr/share/libvirt/schemas
/home/builder/libvirt-10.10.0/src/conf/schemas/nwfilter_params.rng to /usr/share/libvirt/schemas
/home/builder/libvirt-10.10.0/src/conf/schemas/nwfilter.rng to /usr/share/libvirt/schemas
/home/builder/libvirt-10.10.0/src/conf/schemas/privatedata.rng to /usr/share/libvirt/schemas
/home/builder/libvirt-10.10.0/src/conf/schemas/secret.rng to /usr/share/libvirt/schemas
/home/builder/libvirt-10.10.0/src/conf/schemas/storagecommon.rng to /usr/share/libvirt/schemas
/home/builder/libvirt-10.10.0/src/conf/schemas/storagepoolcaps.rng to /usr/share/libvirt/schemas
/home/builder/libvirt-10.10.0/src/conf/schemas/storagepool.rng to /usr/share/libvirt/schemas
/home/builder/libvirt-10.10.0/src/conf/schemas/storagevol.rng to /usr/share/libvirt/schemas
/home/builder/libvirt-10.10.0/build/src/remote/libvirtd.qemu.logrotate to /etc/logrotate.d
/home/builder/libvirt-10.10.0/build/src/remote/libvirtd.lxc.logrotate to /etc/logrotate.d
/home/builder/libvirt-10.10.0/build/src/remote/libvirtd.libxl.logrotate to /etc/logrotate.d
/home/builder/libvirt-10.10.0/build/src/remote/libvirtd.logrotate to /etc/logrotate.d
/home/builder/libvirt-10.10.0/src/remote/libvirtd.sysctl to /usr/lib/sysctl.d
/home/builder/libvirt-10.10.0/src/remote/libvirtd.policy to /usr/share/polkit-1/actions
/home/builder/libvirt-10.10.0/src/remote/libvirtd.rules to /usr/share/polkit-1/rules.d
/home/builder/libvirt-10.10.0/src/remote/libvirtd.sasl to /etc/sasl2
/home/builder/libvirt-10.10.0/build/src/network/default.xml to /etc/libvirt/qemu/networks
/home/builder/libvirt-10.10.0/src/network/libvirt.zone to /usr/lib/firewalld/zones
/home/builder/libvirt-10.10.0/src/network/libvirt-routed.zone to /usr/lib/firewalld/zones
/home/builder/libvirt-10.10.0/src/network/libvirt-to-host.policy to /usr/lib/firewalld/policies
/home/builder/libvirt-10.10.0/src/network/libvirt-routed-out.policy to /usr/lib/firewalld/policies
/home/builder/libvirt-10.10.0/src/network/libvirt-routed-in.policy to /usr/lib/firewalld/policies
/home/builder/libvirt-10.10.0/src/nwfilter/xml/allow-arp.xml to /etc/libvirt/nwfilter
/home/builder/libvirt-10.10.0/src/nwfilter/xml/allow-dhcp-server.xml to /etc/libvirt/nwfilter
/home/builder/libvirt-10.10.0/src/nwfilter/xml/allow-dhcp.xml to /etc/libvirt/nwfilter
/home/builder/libvirt-10.10.0/src/nwfilter/xml/allow-dhcpv6-server.xml to /etc/libvirt/nwfilter
/home/builder/libvirt-10.10.0/src/nwfilter/xml/allow-dhcpv6.xml to /etc/libvirt/nwfilter
/home/builder/libvirt-10.10.0/src/nwfilter/xml/allow-incoming-ipv4.xml to /etc/libvirt/nwfilter
/home/builder/libvirt-10.10.0/src/nwfilter/xml/allow-incoming-ipv6.xml to /etc/libvirt/nwfilter
/home/builder/libvirt-10.10.0/src/nwfilter/xml/allow-ipv4.xml to /etc/libvirt/nwfilter
/home/builder/libvirt-10.10.0/src/nwfilter/xml/allow-ipv6.xml to /etc/libvirt/nwfilter
/home/builder/libvirt-10.10.0/src/nwfilter/xml/clean-traffic-gateway.xml to /etc/libvirt/nwfilter
/home/builder/libvirt-10.10.0/src/nwfilter/xml/clean-traffic.xml to /etc/libvirt/nwfilter
/home/builder/libvirt-10.10.0/src/nwfilter/xml/no-arp-ip-spoofing.xml to /etc/libvirt/nwfilter
/home/builder/libvirt-10.10.0/src/nwfilter/xml/no-arp-mac-spoofing.xml to /etc/libvirt/nwfilter
/home/builder/libvirt-10.10.0/src/nwfilter/xml/no-arp-spoofing.xml to /etc/libvirt/nwfilter
/home/builder/libvirt-10.10.0/src/nwfilter/xml/no-ip-multicast.xml to /etc/libvirt/nwfilter
/home/builder/libvirt-10.10.0/src/nwfilter/xml/no-ip-spoofing.xml to /etc/libvirt/nwfilter
/home/builder/libvirt-10.10.0/src/nwfilter/xml/no-ipv6-multicast.xml to /etc/libvirt/nwfilter
/home/builder/libvirt-10.10.0/src/nwfilter/xml/no-ipv6-spoofing.xml to /etc/libvirt/nwfilter
/home/builder/libvirt-10.10.0/src/nwfilter/xml/no-mac-broadcast.xml to /etc/libvirt/nwfilter
/home/builder/libvirt-10.10.0/src/nwfilter/xml/no-mac-spoofing.xml to /etc/libvirt/nwfilter
/home/builder/libvirt-10.10.0/src/nwfilter/xml/no-other-l2-traffic.xml to /etc/libvirt/nwfilter
/home/builder/libvirt-10.10.0/src/nwfilter/xml/no-other-rarp-traffic.xml to /etc/libvirt/nwfilter
/home/builder/libvirt-10.10.0/src/nwfilter/xml/qemu-announce-self-rarp.xml to /etc/libvirt/nwfilter
/home/builder/libvirt-10.10.0/src/nwfilter/xml/qemu-announce-self.xml to /etc/libvirt/nwfilter
/home/builder/libvirt-10.10.0/src/qemu/libvirt-qemu.sysusers.conf to /usr/lib/sysusers.d
/home/builder/libvirt-10.10.0/src/qemu/postcopy-migration.sysctl to /usr/lib/sysctl.d
/home/builder/libvirt-10.10.0/src/test/test-screenshot.png to /usr/share/libvirt
/home/builder/libvirt-10.10.0/src/admin/libvirt-admin.conf to /etc/libvirt
/home/builder/libvirt-10.10.0/build/src/locking/qemu-lockd.conf to /etc/libvirt
/home/builder/libvirt-10.10.0/build/src/locking/qemu-sanlock.conf to /etc/libvirt
/home/builder/libvirt-10.10.0/src/locking/virtlockd.conf to /etc/libvirt
/home/builder/libvirt-10.10.0/src/logging/virtlogd.conf to /etc/libvirt
/home/builder/libvirt-10.10.0/build/src/network/network.conf to /etc/libvirt
/home/builder/libvirt-10.10.0/src/lxc/lxc.conf to /etc/libvirt
/home/builder/libvirt-10.10.0/build/src/qemu/qemu.conf to /etc/libvirt
/home/builder/libvirt-10.10.0/src/libvirt.conf to /etc/libvirt
/home/builder/libvirt-10.10.0/src/locking/libvirt_lockd.aug to /usr/share/augeas/lenses
/home/builder/libvirt-10.10.0/src/locking/libvirt_sanlock.aug to /usr/share/augeas/lenses
/home/builder/libvirt-10.10.0/src/locking/virtlockd.aug to /usr/share/augeas/lenses
/home/builder/libvirt-10.10.0/src/logging/virtlogd.aug to /usr/share/augeas/lenses
/home/builder/libvirt-10.10.0/src/network/libvirtd_network.aug to /usr/share/augeas/lenses
/home/builder/libvirt-10.10.0/src/lxc/libvirtd_lxc.aug to /usr/share/augeas/lenses
/home/builder/libvirt-10.10.0/src/qemu/libvirtd_qemu.aug to /usr/share/augeas/lenses
/home/builder/libvirt-10.10.0/build/src/libvirtd.conf to /etc/libvirt
/home/builder/libvirt-10.10.0/build/src/libvirtd.aug to /usr/share/augeas/lenses
/home/builder/libvirt-10.10.0/build/src/virtproxyd.conf to /etc/libvirt
/home/builder/libvirt-10.10.0/build/src/virtproxyd.aug to /usr/share/augeas/lenses
/home/builder/libvirt-10.10.0/build/src/virtinterfaced.conf to /etc/libvirt
/home/builder/libvirt-10.10.0/build/src/virtinterfaced.aug to /usr/share/augeas/lenses
/home/builder/libvirt-10.10.0/build/src/virtnetworkd.conf to /etc/libvirt
/home/builder/libvirt-10.10.0/build/src/virtnetworkd.aug to /usr/share/augeas/lenses
/home/builder/libvirt-10.10.0/build/src/virtnodedevd.conf to /etc/libvirt
/home/builder/libvirt-10.10.0/build/src/virtnodedevd.aug to /usr/share/augeas/lenses
/home/builder/libvirt-10.10.0/build/src/virtnwfilterd.conf to /etc/libvirt
/home/builder/libvirt-10.10.0/build/src/virtnwfilterd.aug to /usr/share/augeas/lenses
/home/builder/libvirt-10.10.0/build/src/virtsecretd.conf to /etc/libvirt
/home/builder/libvirt-10.10.0/build/src/virtsecretd.aug to /usr/share/augeas/lenses
/home/builder/libvirt-10.10.0/build/src/virtstoraged.conf to /etc/libvirt
/home/builder/libvirt-10.10.0/build/src/virtstoraged.aug to /usr/share/augeas/lenses
/home/builder/libvirt-10.10.0/build/src/virtlxcd.conf to /etc/libvirt
/home/builder/libvirt-10.10.0/build/src/virtlxcd.aug to /usr/share/augeas/lenses
/home/builder/libvirt-10.10.0/build/src/virtchd.conf to /etc/libvirt
/home/builder/libvirt-10.10.0/build/src/virtchd.aug to /usr/share/augeas/lenses
/home/builder/libvirt-10.10.0/build/src/virtqemud.conf to /etc/libvirt
/home/builder/libvirt-10.10.0/build/src/virtqemud.aug to /usr/share/augeas/lenses
/home/builder/libvirt-10.10.0/build/src/virtvboxd.conf to /etc/libvirt
/home/builder/libvirt-10.10.0/build/src/virtvboxd.aug to /usr/share/augeas/lenses
/home/builder/libvirt-10.10.0/src/remote/virt-guest-shutdown.target to /usr/lib/systemd/system
/home/builder/libvirt-10.10.0/build/src/libvirtd.service to /usr/lib/systemd/system
/home/builder/libvirt-10.10.0/build/src/libvirtd.socket to /usr/lib/systemd/system
/home/builder/libvirt-10.10.0/build/src/libvirtd-ro.socket to /usr/lib/systemd/system
/home/builder/libvirt-10.10.0/build/src/libvirtd-admin.socket to /usr/lib/systemd/system
/home/builder/libvirt-10.10.0/build/src/libvirtd-tcp.socket to /usr/lib/systemd/system
/home/builder/libvirt-10.10.0/build/src/libvirtd-tls.socket to /usr/lib/systemd/system
/home/builder/libvirt-10.10.0/build/src/virtproxyd.service to /usr/lib/systemd/system
/home/builder/libvirt-10.10.0/build/src/virtproxyd.socket to /usr/lib/systemd/system
/home/builder/libvirt-10.10.0/build/src/virtproxyd-ro.socket to /usr/lib/systemd/system
/home/builder/libvirt-10.10.0/build/src/virtproxyd-admin.socket to /usr/lib/systemd/system
/home/builder/libvirt-10.10.0/build/src/virtproxyd-tcp.socket to /usr/lib/systemd/system
/home/builder/libvirt-10.10.0/build/src/virtproxyd-tls.socket to /usr/lib/systemd/system
/home/builder/libvirt-10.10.0/build/src/virtinterfaced.service to /usr/lib/systemd/system
/home/builder/libvirt-10.10.0/build/src/virtinterfaced.socket to /usr/lib/systemd/system
/home/builder/libvirt-10.10.0/build/src/virtinterfaced-ro.socket to /usr/lib/systemd/system
/home/builder/libvirt-10.10.0/build/src/virtinterfaced-admin.socket to /usr/lib/systemd/system
/home/builder/libvirt-10.10.0/build/src/virtlockd.service to /usr/lib/systemd/system
/home/builder/libvirt-10.10.0/build/src/virtlockd.socket to /usr/lib/systemd/system
/home/builder/libvirt-10.10.0/build/src/virtlockd-admin.socket to /usr/lib/systemd/system
/home/builder/libvirt-10.10.0/build/src/virtlogd.service to /usr/lib/systemd/system
/home/builder/libvirt-10.10.0/build/src/virtlogd.socket to /usr/lib/systemd/system
/home/builder/libvirt-10.10.0/build/src/virtlogd-admin.socket to /usr/lib/systemd/system
/home/builder/libvirt-10.10.0/build/src/virtnetworkd.service to /usr/lib/systemd/system
/home/builder/libvirt-10.10.0/build/src/virtnetworkd.socket to /usr/lib/systemd/system
/home/builder/libvirt-10.10.0/build/src/virtnetworkd-ro.socket to /usr/lib/systemd/system
/home/builder/libvirt-10.10.0/build/src/virtnetworkd-admin.socket to /usr/lib/systemd/system
/home/builder/libvirt-10.10.0/build/src/virtnodedevd.service to /usr/lib/systemd/system
/home/builder/libvirt-10.10.0/build/src/virtnodedevd.socket to /usr/lib/systemd/system
/home/builder/libvirt-10.10.0/build/src/virtnodedevd-ro.socket to /usr/lib/systemd/system
/home/builder/libvirt-10.10.0/build/src/virtnodedevd-admin.socket to /usr/lib/systemd/system
/home/builder/libvirt-10.10.0/build/src/virtnwfilterd.service to /usr/lib/systemd/system
/home/builder/libvirt-10.10.0/build/src/virtnwfilterd.socket to /usr/lib/systemd/system
/home/builder/libvirt-10.10.0/build/src/virtnwfilterd-ro.socket to /usr/lib/systemd/system
/home/builder/libvirt-10.10.0/build/src/virtnwfilterd-admin.socket to /usr/lib/systemd/system
/home/builder/libvirt-10.10.0/build/src/virtsecretd.service to /usr/lib/systemd/system
/home/builder/libvirt-10.10.0/build/src/virtsecretd.socket to /usr/lib/systemd/system
/home/builder/libvirt-10.10.0/build/src/virtsecretd-ro.socket to /usr/lib/systemd/system
/home/builder/libvirt-10.10.0/build/src/virtsecretd-admin.socket to /usr/lib/systemd/system
/home/builder/libvirt-10.10.0/build/src/virtstoraged.service to /usr/lib/systemd/system
/home/builder/libvirt-10.10.0/build/src/virtstoraged.socket to /usr/lib/systemd/system
/home/builder/libvirt-10.10.0/build/src/virtstoraged-ro.socket to /usr/lib/systemd/system
/home/builder/libvirt-10.10.0/build/src/virtstoraged-admin.socket to /usr/lib/systemd/system
/home/builder/libvirt-10.10.0/build/src/virtlxcd.service to /usr/lib/systemd/system
/home/builder/libvirt-10.10.0/build/src/virtlxcd.socket to /usr/lib/systemd/system
/home/builder/libvirt-10.10.0/build/src/virtlxcd-ro.socket to /usr/lib/systemd/system
/home/builder/libvirt-10.10.0/build/src/virtlxcd-admin.socket to /usr/lib/systemd/system
/home/builder/libvirt-10.10.0/build/src/virtchd.service to /usr/lib/systemd/system
/home/builder/libvirt-10.10.0/build/src/virtchd.socket to /usr/lib/systemd/system
/home/builder/libvirt-10.10.0/build/src/virtchd-ro.socket to /usr/lib/systemd/system
/home/builder/libvirt-10.10.0/build/src/virtchd-admin.socket to /usr/lib/systemd/system
/home/builder/libvirt-10.10.0/build/src/virtqemud.service to /usr/lib/systemd/system
/home/builder/libvirt-10.10.0/build/src/virtqemud.socket to /usr/lib/systemd/system
/home/builder/libvirt-10.10.0/build/src/virtqemud-ro.socket to /usr/lib/systemd/system
/home/builder/libvirt-10.10.0/build/src/virtqemud-admin.socket to /usr/lib/systemd/system
/home/builder/libvirt-10.10.0/build/src/virtvboxd.service to /usr/lib/systemd/system
/home/builder/libvirt-10.10.0/build/src/virtvboxd.socket to /usr/lib/systemd/system
/home/builder/libvirt-10.10.0/build/src/virtvboxd-ro.socket to /usr/lib/systemd/system
/home/builder/libvirt-10.10.0/build/src/virtvboxd-admin.socket to /usr/lib/systemd/system
/home/builder/libvirt-10.10.0/tools/virt-login-shell.conf to /etc/libvirt
/home/builder/libvirt-10.10.0/build/tools/virt-xml-validate to /usr/bin
/home/builder/libvirt-10.10.0/build/tools/virt-sanlock-cleanup to /usr/sbin
/home/builder/libvirt-10.10.0/tools/virt-qemu-sev-validate to /usr/bin
/home/builder/libvirt-10.10.0/build/tools/libvirt-guests.sh to /usr/libexec
/home/builder/libvirt-10.10.0/build/tools/libvirt-guests.service to /usr/lib/systemd/system
/home/builder/libvirt-10.10.0/tools/virt-qemu-qmp-proxy to /usr/bin
/home/builder/libvirt-10.10.0/build/tools/bash-completion/virsh to /usr/share/bash-completion/completions
/home/builder/libvirt-10.10.0/build/tools/bash-completion/virt-admin to /usr/share/bash-completion/completions
/home/builder/libvirt-10.10.0/build/tools/ssh-proxy/30-libvirt-ssh-proxy.conf to /etc/ssh/ssh_config.d
/home/builder/libvirt-10.10.0/examples/c/admin/client_close.c to /usr/share/doc/libvirt/examples/c/admin
/home/builder/libvirt-10.10.0/examples/c/admin/client_info.c to /usr/share/doc/libvirt/examples/c/admin
/home/builder/libvirt-10.10.0/examples/c/admin/client_limits.c to /usr/share/doc/libvirt/examples/c/admin
/home/builder/libvirt-10.10.0/examples/c/admin/list_clients.c to /usr/share/doc/libvirt/examples/c/admin
/home/builder/libvirt-10.10.0/examples/c/admin/list_servers.c to /usr/share/doc/libvirt/examples/c/admin
/home/builder/libvirt-10.10.0/examples/c/admin/logging.c to /usr/share/doc/libvirt/examples/c/admin
/home/builder/libvirt-10.10.0/examples/c/admin/threadpool_params.c to /usr/share/doc/libvirt/examples/c/admin
/home/builder/libvirt-10.10.0/examples/c/domain/dommigrate.c to /usr/share/doc/libvirt/examples/c/domain
/home/builder/libvirt-10.10.0/examples/c/domain/domtop.c to /usr/share/doc/libvirt/examples/c/domain
/home/builder/libvirt-10.10.0/examples/c/domain/info1.c to /usr/share/doc/libvirt/examples/c/domain
/home/builder/libvirt-10.10.0/examples/c/domain/rename.c to /usr/share/doc/libvirt/examples/c/domain
/home/builder/libvirt-10.10.0/examples/c/domain/suspend.c to /usr/share/doc/libvirt/examples/c/domain
/home/builder/libvirt-10.10.0/examples/c/misc/event-test.c to /usr/share/doc/libvirt/examples/c/misc
/home/builder/libvirt-10.10.0/examples/c/misc/hellolibvirt.c to /usr/share/doc/libvirt/examples/c/misc
/home/builder/libvirt-10.10.0/examples/c/misc/openauth.c to /usr/share/doc/libvirt/examples/c/misc
/home/builder/libvirt-10.10.0/examples/polkit/libvirt-acl.rules to /usr/share/doc/libvirt/examples/polkit
/home/builder/libvirt-10.10.0/examples/sh/virt-lxc-convert to /usr/share/doc/libvirt/examples/sh
/home/builder/libvirt-10.10.0/examples/systemtap/amd-sev-es-vmsa.stp to /usr/share/doc/libvirt/examples/systemtap
/home/builder/libvirt-10.10.0/examples/systemtap/events.stp to /usr/share/doc/libvirt/examples/systemtap
/home/builder/libvirt-10.10.0/examples/systemtap/lock-debug.stp to /usr/share/doc/libvirt/examples/systemtap
/home/builder/libvirt-10.10.0/examples/systemtap/qemu-monitor.stp to /usr/share/doc/libvirt/examples/systemtap
/home/builder/libvirt-10.10.0/examples/systemtap/rpc-monitor.stp to /usr/share/doc/libvirt/examples/systemtap
/home/builder/libvirt-10.10.0/examples/xml/storage/pool-dir.xml to /usr/share/doc/libvirt/examples/xml/storage
/home/builder/libvirt-10.10.0/examples/xml/storage/pool-fs.xml to /usr/share/doc/libvirt/examples/xml/storage
/home/builder/libvirt-10.10.0/examples/xml/storage/pool-logical.xml to /usr/share/doc/libvirt/examples/xml/storage
/home/builder/libvirt-10.10.0/examples/xml/storage/pool-netfs.xml to /usr/share/doc/libvirt/examples/xml/storage
/home/builder/libvirt-10.10.0/examples/xml/storage/vol-cow.xml to /usr/share/doc/libvirt/examples/xml/storage
/home/builder/libvirt-10.10.0/examples/xml/storage/vol-qcow.xml to /usr/share/doc/libvirt/examples/xml/storage
/home/builder/libvirt-10.10.0/examples/xml/storage/vol-qcow2.xml to /usr/share/doc/libvirt/examples/xml/storage
/home/builder/libvirt-10.10.0/examples/xml/storage/vol-raw.xml to /usr/share/doc/libvirt/examples/xml/storage
/home/builder/libvirt-10.10.0/examples/xml/storage/vol-sparse.xml to /usr/share/doc/libvirt/examples/xml/storage
/home/builder/libvirt-10.10.0/examples/xml/storage/vol-vmdk.xml to /usr/share/doc/libvirt/examples/xml/storage
/home/builder/libvirt-10.10.0/examples/xml/test/testdev.xml to /usr/share/doc/libvirt/examples/xml/test
/home/builder/libvirt-10.10.0/examples/xml/test/testnodeinline.xml to /usr/share/doc/libvirt/examples/xml/test
/home/builder/libvirt-10.10.0/examples/xml/test/testdomfc4.xml to /usr/share/doc/libvirt/examples/xml/test
/home/builder/libvirt-10.10.0/examples/xml/test/testdomfv0.xml to /usr/share/doc/libvirt/examples/xml/test
/home/builder/libvirt-10.10.0/examples/xml/test/testnode.xml to /usr/share/doc/libvirt/examples/xml/test
/home/builder/libvirt-10.10.0/examples/xml/test/testnetdef.xml to /usr/share/doc/libvirt/examples/xml/test
/home/builder/libvirt-10.10.0/examples/xml/test/testvol.xml to /usr/share/doc/libvirt/examples/xml/test
/home/builder/libvirt-10.10.0/examples/xml/test/testnetpriv.xml to /usr/share/doc/libvirt/examples/xml/test
/home/builder/libvirt-10.10.0/examples/xml/test/testpool.xml to /usr/share/doc/libvirt/examples/xml/test
/home/builder/libvirt-10.10.0/build/libvirt.pc to /usr/lib64/pkgconfig
/home/builder/libvirt-10.10.0/build/libvirt-qemu.pc to /usr/lib64/pkgconfig
/home/builder/libvirt-10.10.0/build/libvirt-lxc.pc to /usr/lib64/pkgconfig
/home/builder/libvirt-10.10.0/build/libvirt-admin.pc to /usr/lib64/pkgconfig
symlink pointing to /usr/local/lib64/libvirt.so.0.10010.0 to /usr/lib64/libvirt.so.0
symlink pointing to /usr/local/lib64/libvirt.so.0 to /usr/lib64/libvirt.so
symlink pointing to /usr/local/lib64/libvirt-qemu.so.0.10010.0 to /usr/lib64/libvirt-qemu.so.0
symlink pointing to /usr/local/lib64/libvirt-qemu.so.0 to /usr/lib64/libvirt-qemu.so
symlink pointing to /usr/local/lib64/libvirt-lxc.so.0.10010.0 to /usr/lib64/libvirt-lxc.so.0
symlink pointing to /usr/local/lib64/libvirt-lxc.so.0 to /usr/lib64/libvirt-lxc.so
symlink pointing to /usr/local/lib64/libvirt-admin.so.0.10010.0 to /usr/lib64/libvirt-admin.so.0
symlink pointing to /usr/local/lib64/libvirt-admin.so.0 to /usr/lib64/libvirt-admin.so
EOF
)

# Base directories for source and build files
SRC_BASE="/home/builder/libvirt-10.10.0/build"          # Actual source code path
BUILD_BASE="/home/builder/libvirt-10.10.0/build"  # Build directory
DEST_BASE="/BINS"                                 # Base path for installation (usually root)


# Function to copy files
copy_file() {
    local source_file="$1"
    local dest_dir="$2"

    # Compute the full source path
    if [[ "$source_file" == /* ]]; then
        SOURCE_PATH="$source_file"
    else
        SOURCE_PATH="$SRC_BASE/$source_file"
    fi

    # Ensure the source file exists
    if [ ! -e "$SOURCE_PATH" ]; then
        echo "Error: Source file not found: $SOURCE_PATH"
        return
    fi

    # Create destination directory if it does not exist
    mkdir -p "$DEST_BASE$dest_dir"

    # Copy the file
    cp -p "$SOURCE_PATH" "$DEST_BASE$dest_dir"
    echo "Copied $SOURCE_PATH to $DEST_BASE$dest_dir"
}

# Function to create symlink
create_symlink() {
    local target="$1"
    local link_name="$2"

    # Compute the full target path
    if [[ "$target" == /* ]]; then
        TARGET_PATH="$target"
    else
        TARGET_PATH="$BUILD_BASE/$target"
    fi

    # Ensure the target exists
    if [ ! -e "$TARGET_PATH" ]; then
        echo "Error: Target file for symlink not found: $TARGET_PATH"
        return
    fi

    LINK_DIR=$(dirname "$DEST_BASE$link_name")
    mkdir -p "$LINK_DIR"

    ln -sf "$TARGET_PATH" "$DEST_BASE$link_name"
    echo "Created symlink: $DEST_BASE$link_name -> $TARGET_PATH"
}

# Read the list and process each line
while IFS= read -r LINE; do
    # Skip empty lines and comments
    [[ -z "$LINE" ]] && continue
    [[ "$LINE" =~ ^\# ]] && continue

    if [[ "$LINE" == symlink\ pointing\ to* ]]; then
        # Handle symlink creation
        REST=${LINE#symlink pointing to }
        if [[ "$REST" =~ ^(.+?)\ to\ (.+)$ ]]; then
            TARGET="${BASH_REMATCH[1]}"
            LINK_NAME="${BASH_REMATCH[2]}"
            create_symlink "$TARGET" "$LINK_NAME"
        else
            echo "Invalid symlink line: $LINE"
        fi
    else
        # Handle file copying
        if [[ "$LINE" =~ ^(.+?)\ to\ (.+)$ ]]; then
            SOURCE_FILE="${BASH_REMATCH[1]}"
            DEST_DIR="${BASH_REMATCH[2]}"
            copy_file "$SOURCE_FILE" "$DEST_DIR"
        else
            echo "Invalid line: $LINE"
        fi
    fi
done <<< "$FILE_LIST"