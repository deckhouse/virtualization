# Changelog v1.1

## Features


 - **[module]** Added the `D8VirtualizationDVCRInsufficientCapacityRisk` alert, which warns of the risk of insufficient free space in the virtual machine image storage (DVCR). [#1461](https://github.com/deckhouse/virtualization/pull/1461)
 - **[module]** Added the `KubeNodeAwaitingVirtualMachinesEvictionBeforeShutdown` alert, which is triggered when the node hosting the virtual machines is about to shut down but VM evacuation is not yet complete. [#1268](https://github.com/deckhouse/virtualization/pull/1268)
 - **[vm]** Added the ability to migrate VMs using disks on local storage. Restrictions:
    - The feature is not available in the CE edition.
    - Migration is only possible for running VMs (`phase: Running`).
    - Migration of VMs with local disks connected via `VirtualMachineBlockDeviceAttachment` (hotplug) is not supported yet.
    
    Added the ability to migrate storage for VM disks (change `StorageClass`). Restrictions:
    - The feature is not available in the CE edition.
    - Migration is only possible for running VMs (`phase: Running`).
    - Storage migration for disks connected via `VirtualMachineBlockDeviceAttachment` (hotplug) is not supported yet. [#1360](https://github.com/deckhouse/virtualization/pull/1360)
 - **[vmop]** Added an operation with the `Clone` type to create a clone of a VM from an existing VM (`VirtualMachineOperation` `.spec.type: Clone`). [#1418](https://github.com/deckhouse/virtualization/pull/1418)

## Fixes


 - **[core]** Fixed an issue in containerdv2 where storage providing a PVC with the FileSystem type was incorrectly attached via `VirtualMachineBlockDeviceAttachment`. [#1548](https://github.com/deckhouse/virtualization/pull/1548)
 - **[core]** Added error reporting in the status of disks and images when the data source (URL) is unavailable. [#1534](https://github.com/deckhouse/virtualization/pull/1534)
 - **[module]** fix CVE-2025-58058 and CVE-2025-54410 [#1572](https://github.com/deckhouse/virtualization/pull/1572)
 - **[observability]** Fixed the graph on the virtual machine dashboard that displays memory copy statistics during VM migration. [#1474](https://github.com/deckhouse/virtualization/pull/1474)
 - **[vd]** Fixed live disk migration between storage classes using different drivers. Limitations:
    - Migration between `Block` and `Filesystem` is not supported. Only migrations between the same volume modes are allowed: from `Block` to `Block` and from `Filesystem` to `Filesystem`. [#1613](https://github.com/deckhouse/virtualization/pull/1613)
 - **[vd]** respect user-specified storage class when restoring from snapshot [#1417](https://github.com/deckhouse/virtualization/pull/1417)
 - **[vi]** When creating virtual images from virtual disk snapshots, the `spec.persistentVolumeClaim.storageClassName` parameter is now respected. Previously, it could be ignored. [#1533](https://github.com/deckhouse/virtualization/pull/1533)
 - **[vm]** In the `Migrating` state, detailed error information is now displayed when a live migration of a virtual machine fails. [#1569](https://github.com/deckhouse/virtualization/pull/1569)
 - **[vm]** Fixed the `NetworkReady` condition output. It no longer shows the `Unknown` state and appears only when needed. [#1567](https://github.com/deckhouse/virtualization/pull/1567)
 - **[vm]** Prohibit duplicate networks in the virtual machine `.spec.network` specification. [#1545](https://github.com/deckhouse/virtualization/pull/1545)
 - **[vmbda]** Fixed a bug where, when detaching a virtual image through `VirtualMachineBlockDeviceAttachment`, the resource could get stuck in the Terminating state. [#1542](https://github.com/deckhouse/virtualization/pull/1542)
 - **[vmclass]** Use qemu64 CPU model for Discovery and Features types to fix nested virtualization on AMD hosts [#1446](https://github.com/deckhouse/virtualization/pull/1446)
 - **[vmip]** Added validation for static IP addresses to avoid creating a `VirtualMachineIPAddress` resource with an IP already in use in the cluster. [#1530](https://github.com/deckhouse/virtualization/pull/1530)
 - **[vmop]** Fix the problem where a disk that in the "Terminating" phase  was wrongly added to kvvm's volumes during a restore operation in Strict mode. [#1493](https://github.com/deckhouse/virtualization/pull/1493)
 - **[vmop]** Fixed garbage collector behavior: previously, all VMOP objects were deleted after restarting the virtualization controller, ignoring cleanup rules. [#1471](https://github.com/deckhouse/virtualization/pull/1471)

## Chore


 - **[observability]** Added Prometheus metrics for virtual machine snapshots (`d8_virtualization_virtualmachinesnapshot_info`) and virtual disk snapshots (`d8_virtualization_virtualdisksnapshot_info`), showing which objects they are associated with. [#1555](https://github.com/deckhouse/virtualization/pull/1555)

