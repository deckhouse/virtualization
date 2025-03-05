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

## Features


 - **[vm]** We have implemented an automatic CPU topology configuration mechanism for VMs. The number of cores/sockets depends on the number of cores in .spec.cpu.cores. For more details, refer to the documentation. [#747](https://github.com/deckhouse/virtualization/pull/747)
 - **[vm]** VirtualMachine.Status.BlockDeviceRefs contains hotplugged images [#681](https://github.com/deckhouse/virtualization/pull/681)

## Fixes


 - **[core]** add blockdev binary to cdi-importer and cdi-controller [#820](https://github.com/deckhouse/virtualization/pull/820)
 - **[core]** add swtpm configs and gnutls-utils to virt-launcher image [#819](https://github.com/deckhouse/virtualization/pull/819)
 - **[core]** fix symlinks and add missing binaries virt-launcher [#815](https://github.com/deckhouse/virtualization/pull/815)
 - **[core]** fix dvcr images imports, cdi components to distroless images [#806](https://github.com/deckhouse/virtualization/pull/806)
 - **[vd]** Optimized the creation time for empty disks [#786](https://github.com/deckhouse/virtualization/pull/786)
 - **[vd]** fix resizing handler and cover it with unit tests [#685](https://github.com/deckhouse/virtualization/pull/685)
 - **[vi]** bug fixes related to VirtualImage and VDSnapshot ObjectRef [#781](https://github.com/deckhouse/virtualization/pull/781)
 - **[vm]** correct CPU core validation logic for range checks [#824](https://github.com/deckhouse/virtualization/pull/824)
 - **[vm]** fix description in generated code [#818](https://github.com/deckhouse/virtualization/pull/818)
 - **[vm]** rename filesystemReady condition to filesystemFrozen [#714](https://github.com/deckhouse/virtualization/pull/714)
 - **[vmbda]** fixed a bug that prevented the deletion of the vmbda device when its block device was already specified in the virtual machine's specification. [#760](https://github.com/deckhouse/virtualization/pull/760)

## Chore


 - **[core]** remove libguestfs image [#807](https://github.com/deckhouse/virtualization/pull/807)
 - **[core]** change vm-router-forge image to distroless [#790](https://github.com/deckhouse/virtualization/pull/790)
 - **[core]** change virt-launcher image to distroless [#773](https://github.com/deckhouse/virtualization/pull/773)
 - **[core]** change distroless user to deckhouse [#757](https://github.com/deckhouse/virtualization/pull/757)
 - **[core]** virt-handler image to distroless [#748](https://github.com/deckhouse/virtualization/pull/748)
 - **[core]** virtualization api,controller to distroless [#745](https://github.com/deckhouse/virtualization/pull/745)
 - **[core]** virt operator,exportproxy,exportserver to distroless [#744](https://github.com/deckhouse/virtualization/pull/744)
 - **[core]** apply new cilium network priority api [#733](https://github.com/deckhouse/virtualization/pull/733)
 - **[core]** change dvcr images to distroless [#715](https://github.com/deckhouse/virtualization/pull/715)
 - **[docs]** add more cloud images sources [#813](https://github.com/deckhouse/virtualization/pull/813)
 - **[docs]** add ansible provisioning guide to FAQ [#803](https://github.com/deckhouse/virtualization/pull/803)
 - **[module]** moving tls certificates generations to go hooks sdk. [#701](https://github.com/deckhouse/virtualization/pull/701)

