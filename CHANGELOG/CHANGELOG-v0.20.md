# Changelog v0.20

## Features


 - **[module]** move module from preview stage to general availability [#1109](https://github.com/deckhouse/virtualization/pull/1109)
 - **[vm]** The `InternalVirtualMachine` will be updated if the `VirtualMachine` is stopped. [#1078](https://github.com/deckhouse/virtualization/pull/1078)

## Fixes


 - **[cli]** restore terminal after console disconnect, correct vnc closure [#1085](https://github.com/deckhouse/virtualization/pull/1085)
 - **[core]** set live migrations defaults [#1082](https://github.com/deckhouse/virtualization/pull/1082)
 - **[module]** Restore system-cluster-critical priority class for critical components. [#1113](https://github.com/deckhouse/virtualization/pull/1113)
 - **[vdsnapshot]** unfreeze virtual machine filesystem if snapshotting failed [#1117](https://github.com/deckhouse/virtualization/pull/1117)

## Chore


 - **[core]** Addressing static analyzer warnings in virt-handler and virt-launcher. [#1127](https://github.com/deckhouse/virtualization/pull/1127)
 - **[core]** build fuse3 [#1102](https://github.com/deckhouse/virtualization/pull/1102)
 - **[module]** Mitigate CVEs in Python parts, remove unused code. [#1103](https://github.com/deckhouse/virtualization/pull/1103)

