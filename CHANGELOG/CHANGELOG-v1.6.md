# Changelog v1.6

## Features


 - **[api]** Added support for attaching USB devices to virtual machines using the `.spec.usbDevices` field. Introduced the `NodeUSBDevice` and `USBDevice` resources for managing USB devices within the cluster:
    - `NodeUSBDevice` (cluster-scoped): represents a USB device discovered on a specific node. Allows assigning a USB device for use in a specific namespace.
    - `USBDevice` (namespace-scoped): represents a USB device available for attachment to virtual machines in a given namespace. [#1913](https://github.com/deckhouse/virtualization/pull/1913)
 - **[module]** Enabled DVCR cleanup in clusters by default: daily at 02:00. You can override the schedule via `dvcr.gc.schedule` in the `virtualization` module ModuleConfig. [#2019](https://github.com/deckhouse/virtualization/pull/2019)
 - **[module]** Added information about virtual machine pods to the virtual machine dashboard. [#2002](https://github.com/deckhouse/virtualization/pull/2002)
 - **[module]** Added the `Virtualization / Overview` dashboard with an overview of the virtualization platform status. [#1956](https://github.com/deckhouse/virtualization/pull/1956)

## Fixes


 - **[module]** Fixed vulnerabilities CVE-2026-24051 and CVE-2025-15558. [#2057](https://github.com/deckhouse/virtualization/pull/2057)
 - **[observability]** Restored the previous placement of virtual machine dashboards due to a validation issue that could block the Deckhouse queue. [#2063](https://github.com/deckhouse/virtualization/pull/2063)
 - **[vd]** Fixed virtual disks hanging during creation in `WaitForFirstConsumer` mode on nodes with taints. [#1999](https://github.com/deckhouse/virtualization/pull/1999)
 - **[vm]** Fixed USB device discovery on nodes: corresponding `NodeUSBDevice` resources might not have been created. [#2085](https://github.com/deckhouse/virtualization/pull/2085)
 - **[vm]** Fixed cloning of a virtual machine with connected USB devices when using `VirtualMachineOperation` with the `Clone` type in `BestEffort` mode. [#2076](https://github.com/deckhouse/virtualization/pull/2076)
 - **[vm]** If only the `Main` network is specified in `.spec.networks`, the `sdn` module is no longer required. [#2027](https://github.com/deckhouse/virtualization/pull/2027)
 - **[vm]** Labels and annotations now work properly on virtual machines. [#1971](https://github.com/deckhouse/virtualization/pull/1971)
 - **[vm]** Fixed virtual machine migration with disks attached via `VirtualMachineBlockDeviceAttachment` (hotplug): the target pod could exceed memory limits (`OOMKilled`). [#1947](https://github.com/deckhouse/virtualization/pull/1947)
 - **[vmbda]** Fixed an incorrect `Pending` phase for the `VirtualMachineBlockDeviceAttachment` resource during virtual machine migration. [#2011](https://github.com/deckhouse/virtualization/pull/2011)
 - **[vmbda]** To remove disks and images attached to a virtual machine via `VirtualMachineBlockDeviceAttachment` (hotplug), you must first detach them from the virtual machine by deleting the corresponding `vmbda`. This information has been added to the `vmbda` status. [#2000](https://github.com/deckhouse/virtualization/pull/2000)

## Chore


 - **[vm]** Added the `--from-file` flag to the `vlctl` utility for viewing domain information from a local libvirt XML file. [#2014](https://github.com/deckhouse/virtualization/pull/2014)

