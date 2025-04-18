# Changelog v0.17

## [MALFORMED]


 - #709 invalid type "feat", unknown section "e2e"
 - #713 invalid type "feat"
 - #743 missing section, missing summary, missing type, unknown section ""
 - #750 invalid type "test"
 - #752 invalid type "test"
 - #754 invalid type "test"
 - #770 invalid type "refactor"
 - #774 invalid type "feat"
 - #782 invalid type "refactor"
 - #784 unknown section "doc"
 - #787 invalid type "test"
 - #791 invalid type "feat"
 - #792 invalid type "refactor"
 - #794 unknown section "test"
 - #797 unknown section "test"
 - #805 invalid type "feat"
 - #809 invalid type "feat"
 - #811 invalid type "refactor"
 - #816 invalid type "test"
 - #828 invalid type "refactor"
 - #838 invalid type "refactor"
 - #851 invalid type "vm", unknown section "test"

## Features


 - **[core]** Restrict libvirt socket access to virt-launcher only. [#817](https://github.com/deckhouse/virtualization/pull/817)
 - **[core]** Add `vlctl` tool as a virsh replacement compatible with restricted libvirt socket. [#817](https://github.com/deckhouse/virtualization/pull/817)
 - **[module]** Add VM grafana dashboard [#861](https://github.com/deckhouse/virtualization/pull/861)
 - **[vm]** add QemuVersion and LibvirtVersion for VirtualMachine [#907](https://github.com/deckhouse/virtualization/pull/907)
 - **[vm]** Add workload-updater controller to trigger VM migration for firmware updates by creating a VMOP resource; deletion of the VMOP resource cancels the migration. [#881](https://github.com/deckhouse/virtualization/pull/881)
 - **[vm]** We have implemented an automatic CPU topology configuration mechanism for VMs. The number of cores/sockets depends on the number of cores in .spec.cpu.cores. For more details, refer to the documentation. [#747](https://github.com/deckhouse/virtualization/pull/747)
 - **[vm]** VirtualMachine.Status.BlockDeviceRefs contains hotplugged images [#681](https://github.com/deckhouse/virtualization/pull/681)
 - **[vmop]** Implement new migration design. Delete VMOP type Evict, equivalent to working as cancel migration. [#857](https://github.com/deckhouse/virtualization/pull/857)

## Fixes


 - **[api]** fix the issue of block devices getting stuck in the Terminating phase. [#920](https://github.com/deckhouse/virtualization/pull/920)
 - **[core]** remove host-passthrough VMC [#926](https://github.com/deckhouse/virtualization/pull/926)
 - **[core]** remove missed cdi-uploadproxy image [#867](https://github.com/deckhouse/virtualization/pull/867)
 - **[core]** Add to cdi config HonorWaitForFirstConsumer [#850](https://github.com/deckhouse/virtualization/pull/850)
 - **[core]** Virtual machines can run on Linux nodes with broken implementations of the getsockopt syscall. [#843](https://github.com/deckhouse/virtualization/pull/843)
 - **[core]** Revert scsi disk serial truncate in QEMU [#842](https://github.com/deckhouse/virtualization/pull/842)
 - **[core]** Rename internal resources to not conflict with the original Kubevirt installation. [#839](https://github.com/deckhouse/virtualization/pull/839)
 - **[core]** add mknod binary to virt-launcher image [#834](https://github.com/deckhouse/virtualization/pull/834)
 - **[core]** add blockdev binary to cdi-importer and cdi-controller [#820](https://github.com/deckhouse/virtualization/pull/820)
 - **[core]** add swtpm configs and gnutls-utils to virt-launcher image [#819](https://github.com/deckhouse/virtualization/pull/819)
 - **[core]** fix symlinks and add missing binaries virt-launcher [#815](https://github.com/deckhouse/virtualization/pull/815)
 - **[core]** fix dvcr images imports, cdi components to distroless images [#806](https://github.com/deckhouse/virtualization/pull/806)
 - **[images]** fix unprotect uploader with nil pod arg [#899](https://github.com/deckhouse/virtualization/pull/899)
 - **[vd]** Optimized the creation time for empty disks [#786](https://github.com/deckhouse/virtualization/pull/786)
 - **[vd]** fix resizing handler and cover it with unit tests [#685](https://github.com/deckhouse/virtualization/pull/685)
 - **[vi]** bug fixes related to VirtualImage and VDSnapshot ObjectRef [#781](https://github.com/deckhouse/virtualization/pull/781)
 - **[vm]** resolving EFI Boot Issues on Windows Systems with more than 8 cores [#910](https://github.com/deckhouse/virtualization/pull/910)
 - **[vm]** fix errors with power state operations [#873](https://github.com/deckhouse/virtualization/pull/873)
 - **[vm]** add missed but attached block device references to the status of the virtual machine [#841](https://github.com/deckhouse/virtualization/pull/841)
 - **[vm]** correct maximum CPU sockets assignment in domain specification [#832](https://github.com/deckhouse/virtualization/pull/832)
 - **[vm]** correct CPU core validation logic for range checks [#824](https://github.com/deckhouse/virtualization/pull/824)
 - **[vm]** fix description in generated code [#818](https://github.com/deckhouse/virtualization/pull/818)
 - **[vm]** rename filesystemReady condition to filesystemFrozen [#714](https://github.com/deckhouse/virtualization/pull/714)
 - **[vmbda]** fixed a bug that prevented the deletion of the vmbda device when its block device was already specified in the virtual machine's specification. [#760](https://github.com/deckhouse/virtualization/pull/760)

## Chore


 - **[api]** add generic handler reconciler [#869](https://github.com/deckhouse/virtualization/pull/869)
 - **[core]** remove libguestfs image [#807](https://github.com/deckhouse/virtualization/pull/807)
 - **[core]** change vm-router-forge image to distroless [#790](https://github.com/deckhouse/virtualization/pull/790)
 - **[core]** change virt-launcher image to distroless [#773](https://github.com/deckhouse/virtualization/pull/773)
 - **[core]** change distroless user to deckhouse [#757](https://github.com/deckhouse/virtualization/pull/757)
 - **[core]** virt-handler image to distroless [#748](https://github.com/deckhouse/virtualization/pull/748)
 - **[core]** virtualization api,controller to distroless [#745](https://github.com/deckhouse/virtualization/pull/745)
 - **[core]** virt operator,exportproxy,exportserver to distroless [#744](https://github.com/deckhouse/virtualization/pull/744)
 - **[core]** apply new cilium network priority api [#733](https://github.com/deckhouse/virtualization/pull/733)
 - **[core]** change dvcr images to distroless [#715](https://github.com/deckhouse/virtualization/pull/715)
 - **[docs]** move old changelog to CHANGELOG directory [#878](https://github.com/deckhouse/virtualization/pull/878)
 - **[docs]** Change external link from code example on the Installation page. [#856](https://github.com/deckhouse/virtualization/pull/856)
 - **[docs]** add permissions block to e2e README.md [#840](https://github.com/deckhouse/virtualization/pull/840)
 - **[docs]** add more cloud images sources [#813](https://github.com/deckhouse/virtualization/pull/813)
 - **[docs]** add ansible provisioning guide to FAQ [#803](https://github.com/deckhouse/virtualization/pull/803)
 - **[images]** fix test description [#876](https://github.com/deckhouse/virtualization/pull/876)
 - **[module]** moving tls certificates generations to go hooks sdk. [#701](https://github.com/deckhouse/virtualization/pull/701)
 - **[vdsnapshot]** disable consistency check [#928](https://github.com/deckhouse/virtualization/pull/928)
 - **[vm]** fix panic [#939](https://github.com/deckhouse/virtualization/pull/939)
 - **[vm]** fix migration to the same node [#938](https://github.com/deckhouse/virtualization/pull/938)
 - **[vm]** fix cloud init [#912](https://github.com/deckhouse/virtualization/pull/912)

