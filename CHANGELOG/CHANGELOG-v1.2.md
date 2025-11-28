# Changelog v1.2

## Features


 - **[core]** Added the `VirtualMachineSnapshotOperation` resource for creating a virtual machine based on a `VirtualMachineSnapshot`. [#1727](https://github.com/deckhouse/virtualization/pull/1727)
 - **[module]** Added the ability to clean up DVCR from non-existent project and cluster images:
    - By default, this feature is disabled.
    - To enable cleanup, set a schedule in the module settings: `.spec.settings.dvcr.gc.schedule`. [#1688](https://github.com/deckhouse/virtualization/pull/1688)
 - **[module]** Added validation for the virtualization ModuleConfig that prevents decreasing the DVCR storage size and changing its StorageClass. [#1628](https://github.com/deckhouse/virtualization/pull/1628)
 - **[module]** Improved audit events by using more informative messages that include virtual machine names and user information. [#1611](https://github.com/deckhouse/virtualization/pull/1611)
 - **[observability]** Added new metrics for disks:
    - `d8_virtualization_virtualdisk_capacity_bytes`: Metric showing the disk size.
    - `d8_virtualization_virtualdisk_info`: Metric with information about the disk configuration.
    - `d8_virtualization_virtualdisk_status_inuse`: Metric showing the current use of the disk by a virtual machine or for creating other block devices. [#1592](https://github.com/deckhouse/virtualization/pull/1592)
 - **[vmbda]** Added detailed error output in the `Attached` condition of the `VirtualMachineBlockDeviceAttachment` resource when a block device is unavailable on the virtual machine node. [#1561](https://github.com/deckhouse/virtualization/pull/1561)
 - **[vmclass]** For the `VirtualMachineClass` resource, version `v1alpha2` is deprecated. Use version `v1alpha3` instead:
    - In version `v1alpha3`, the `.spec.sizingPolicies.coreFraction` field is now a string with a percentage (for example, "50%"), similar to the field in a virtual machine. [#1601](https://github.com/deckhouse/virtualization/pull/1601)
 - **[vmrestore]** The `VirtualMachineRestore` resource is deprecated. Use the following resources instead:
    - `VirtualMachineOperation` with type `Clone`: For cloning an existing virtual machine.
    - `VirtualMachineOperation` with type `Restore`: For restoring an existing virtual machine to a state from a snapshot.
    - `VirtualMachineSnapshotOperation`: For creating a new virtual machine based on a snapshot. [#1631](https://github.com/deckhouse/virtualization/pull/1631)

## Fixes


 - **[core]** add missing libraries to virt-launcher build [#1761](https://github.com/deckhouse/virtualization/pull/1761)
 - **[core]** Fixed the MethodNotAllowed error for patch and watch operations when querying the `VirtualMachineClass` resource via command-line utilities (d8 k, kubectl). [#1666](https://github.com/deckhouse/virtualization/pull/1666)
 - **[images]** Fixed an issue that prevented deleting `VirtualImage` and `ClusterVirtualImage` resources for a stopped virtual machine. [#1669](https://github.com/deckhouse/virtualization/pull/1669)
 - **[module]** Fixed RBAC for the `user` and `editor` cluster roles. [#1749](https://github.com/deckhouse/virtualization/pull/1749)
 - **[module]** Fixed the `D8VirtualizationVirtualMachineFirmwareOutOfDate` alert, which could be duplicated when virtualization runs in HA mode. [#1739](https://github.com/deckhouse/virtualization/pull/1739)
 - **[module]** Added the ability to modify or delete the `VirtualMachineClass` resource named "generic". The virtualization module will no longer restore it to its original state. [#1597](https://github.com/deckhouse/virtualization/pull/1597)
 - **[vdsnapshot]** Fixed an error that could lead to inconsistencies between `VirtualMachineSnapshot` and `VirtualDiskSnapshot` resources when creating a snapshot of a virtual machine with multiple disks. [#1668](https://github.com/deckhouse/virtualization/pull/1668)

## Chore


 - **[core]** Fixed vulnerability CVE-2025-64324. [#1702](https://github.com/deckhouse/virtualization/pull/1702)

