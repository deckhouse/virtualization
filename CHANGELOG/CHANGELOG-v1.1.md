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


 - **[observability]** Fixed the graph on the virtual machine dashboard that displays memory copy statistics during VM migration. [#1474](https://github.com/deckhouse/virtualization/pull/1474)
 - **[vd]** respect user-specified storage class when restoring from snapshot [#1417](https://github.com/deckhouse/virtualization/pull/1417)
 - **[vmclass]** Use qemu64 CPU model for Discovery and Features types to fix nested virtualization on AMD hosts [#1446](https://github.com/deckhouse/virtualization/pull/1446)
 - **[vmop]** Fix the problem where a disk that in the "Terminating" phase  was wrongly added to kvvm's volumes during a restore operation in Strict mode. [#1493](https://github.com/deckhouse/virtualization/pull/1493)
 - **[vmop]** Fixed garbage collector behavior: previously, all VMOP objects were deleted after restarting the virtualization controller, ignoring cleanup rules. [#1471](https://github.com/deckhouse/virtualization/pull/1471)

