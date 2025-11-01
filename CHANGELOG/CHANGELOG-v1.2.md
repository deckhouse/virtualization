# Changelog v1.2

## [MALFORMED]


 - #1588 invalid type "refactor"

## Features


 - **[module]** Validate dvcr section in ModuleConfig. [#1628](https://github.com/deckhouse/virtualization/pull/1628)
 - **[module]** add project/namespace virtual machines overview dashboard and fix virtualmachine dashboard [#1603](https://github.com/deckhouse/virtualization/pull/1603)
 - **[observability]** Added three new Prometheus metrics for VirtualDisk monitoring (capacity_bytes, info, and status_inuse). [#1592](https://github.com/deckhouse/virtualization/pull/1592)
 - **[vmrestore]** VirtualMachineRestore is deprecated and replaced by VirtualMachineOperation with type 'Restore'. [#1631](https://github.com/deckhouse/virtualization/pull/1631)

## Fixes


 - **[core]** More frequent polling for applied checksum to get audit events about changed checksum. [#1637](https://github.com/deckhouse/virtualization/pull/1637)
 - **[core]** fix map iterate concurrency panic in vm-route-forge [#1602](https://github.com/deckhouse/virtualization/pull/1602)
 - **[module]** Improving audit events names. Also add ignoring system service acconts. [#1611](https://github.com/deckhouse/virtualization/pull/1611)
 - **[module]** Improve module control audit event name field. Also process event only for supported methods and skip  service accounts events. [#1604](https://github.com/deckhouse/virtualization/pull/1604)
 - **[vd]** Do not block volume migrations if failed snapshot exists. [#1647](https://github.com/deckhouse/virtualization/pull/1647)
 - **[vdsnapshot]** A virtual disk snapshot will be in the Failed phase if `spec.requiredConsistency` is `true`, but a virtual machine has not been stopped or filesystem has not been frozen during the snapshotting process. [#1605](https://github.com/deckhouse/virtualization/pull/1605)
 - **[vm]** Fixed an issue where the Virtualization Controller could panic on unexpected block device deletion. [#1585](https://github.com/deckhouse/virtualization/pull/1585)
 - **[vmbda]** Fix missing Serial for Attached images and disks in intvirtvm. [#1580](https://github.com/deckhouse/virtualization/pull/1580)
 - **[vmbda]** VMBDA now reports a clear error if the device is not available on the VM's node. [#1561](https://github.com/deckhouse/virtualization/pull/1561)
 - **[vmrestore]** Remove spurious error logs when snapshot secret name is not yet populated during initialization. [#1624](https://github.com/deckhouse/virtualization/pull/1624)

## Chore


 - **[core]** More renames for containers to work with containerd v2. [#1579](https://github.com/deckhouse/virtualization/pull/1579)
 - **[module]** Fix build for p11-kit, fix mount of /var/log/libvirt in virt-launcher image. [#1576](https://github.com/deckhouse/virtualization/pull/1576)
 - **[module]** Use at least golang 1.24 for all components. [#1575](https://github.com/deckhouse/virtualization/pull/1575)

