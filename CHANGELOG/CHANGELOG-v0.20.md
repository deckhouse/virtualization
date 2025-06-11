# Changelog v0.20

## Features


 - **[core]** Add events for the Bound and Released states of the VirtualMachineIPAddressLease. [#1146](https://github.com/deckhouse/virtualization/pull/1146)
 - **[images]** Add VI/CVI InUse condtion [#859](https://github.com/deckhouse/virtualization/pull/859)
 - **[module]** move module from preview stage to general availability [#1109](https://github.com/deckhouse/virtualization/pull/1109)
 - **[module]** Add a smibios parameter to define the virtualization nesting level. [#559](https://github.com/deckhouse/virtualization/pull/559)
 - **[vm]** The `InternalVirtualMachine` will be updated if the `VirtualMachine` is stopped. [#1078](https://github.com/deckhouse/virtualization/pull/1078)
 - **[vmop]** Improved phase handling for the `migrate` and `evict` operation [#1128](https://github.com/deckhouse/virtualization/pull/1128)

## Fixes


 - **[cli]** restore terminal after console disconnect, correct vnc closure [#1085](https://github.com/deckhouse/virtualization/pull/1085)
 - **[core]** Add additional events for different cases for VirtualMachineIPAddress. [#1147](https://github.com/deckhouse/virtualization/pull/1147)
 - **[core]** set live migrations defaults [#1082](https://github.com/deckhouse/virtualization/pull/1082)
 - **[vdsnapshot]** unfreeze virtual machine filesystem if snapshotting failed [#1117](https://github.com/deckhouse/virtualization/pull/1117)

## Chore


 - **[core]** Addressing static analyzer warnings in virt-handler and virt-launcher. [#1127](https://github.com/deckhouse/virtualization/pull/1127)
 - **[core]** build gcc [#1112](https://github.com/deckhouse/virtualization/pull/1112)
 - **[core]** build fuse3 [#1102](https://github.com/deckhouse/virtualization/pull/1102)
 - **[module]** Mitigate CVEs in Python parts, remove unused code. [#1103](https://github.com/deckhouse/virtualization/pull/1103)

