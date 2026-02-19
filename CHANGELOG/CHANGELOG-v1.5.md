# Changelog v1.5

## Features


 - **[observability]** Added a table with virtual machine operations to the `Namespace / Virtual Machine` dashboard. [#1935](https://github.com/deckhouse/virtualization/pull/1935)
 - **[vm]** Added support for targeted migration of virtual machines. To do this, create a [VirtualMachineOperation](/modules/virtualization/cr.html#virtualmachineoperation) resource with the `Migrate` type and specify `.spec.migrate.nodeSelector` to migrate the virtual machine to the corresponding node. [#1874](https://github.com/deckhouse/virtualization/pull/1874)

## Fixes


 - **[core]** Fixed an issue with starting virtual machines using the `EFIWithSecureBoot` bootloader when configured with more than 12 vCPUs. [#1916](https://github.com/deckhouse/virtualization/pull/1916)
 - **[module]** Platform system components in user projects are protected from deletion by users. [#1880](https://github.com/deckhouse/virtualization/pull/1880)
 - **[module]** During virtual machine migration, temporary double consumption of resources is no longer counted in project quotas. System component resources required for starting and running virtual machines are no longer counted in project quotas. [#1872](https://github.com/deckhouse/virtualization/pull/1872)
 - **[vd]** Fixed an issue with creating a virtual disk from a virtual image stored on a `PersistentVolumeClaim` (with `.spec.storage` set to `PersistentVolumeClaim`). [#1983](https://github.com/deckhouse/virtualization/pull/1983)
 - **[vd]** Fixed an issue with live migration of a virtual machine between StorageClass with the `Filesystem` type. [#1940](https://github.com/deckhouse/virtualization/pull/1940)
 - **[vm]** Fixed a possible virtual machine hang in the `Pending` state during migration when changing the StorageClass. [#1903](https://github.com/deckhouse/virtualization/pull/1903)
 - **[vmop]** Fixed an issue with cloning a virtual machine whose disks use storage in `WaitForFirstConsumer` mode. [#1926](https://github.com/deckhouse/virtualization/pull/1926)

## Chore


 - **[core]** Change virt-launcher and hotplug pod prefix. [#1867](https://github.com/deckhouse/virtualization/pull/1867)
 - **[vd]** When viewing disks, the name of the virtual machine they are attached to is now displayed (`d8 k get vd`). [#1889](https://github.com/deckhouse/virtualization/pull/1889)

