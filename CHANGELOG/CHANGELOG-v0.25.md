# Changelog v0.25

## Know before update


 - In version v0.25.0, support for the module's operation with CRI Containerd V2 has been added.
    After upgrading CRI from Containerd v1 to Containerd v2, it is necessary to recreate the images that were created using virtualization module version v0.24.0 and earlier.

## Features


 - **[core]** In version v0.25.0, support for the module's operation with CRI Containerd V2 has been added. [#1395](https://github.com/deckhouse/virtualization/pull/1395)
    In version v0.25.0, support for the module's operation with CRI Containerd V2 has been added.
    After upgrading CRI from Containerd v1 to Containerd v2, it is necessary to recreate the images that were created using virtualization module version v0.24.0 and earlier.
 - **[observability]** New Prometheus metrics have been added to track the phase of resources such as `VirtualMachineSnapshot`, `VirtualDiskSnapshot`, `VirtualImage`, and `ClusterVirtualImage`. [#1356](https://github.com/deckhouse/virtualization/pull/1356)
 - **[vm]** MAC address management for additional network interfaces has been added using the `VirtualMachineMACAddress` and `VirtualMachineMACAddressLease` resources. [#1350](https://github.com/deckhouse/virtualization/pull/1350)
 - **[vm]** Added the ability to attach additional network interfaces to a virtual machine for networks provided by the SDN module. For this, the SDN module must be enabled in the cluster. [#1253](https://github.com/deckhouse/virtualization/pull/1253)
 - **[vmclass]** An annotation has been added to set the default VirtualMachineClass.
    To designate a `VirtualMachineClass` as the default, you need to add the annotation 
    `virtualmachineclass.virtualization.deckhouse.io/is-default-class=true` to it.
    This allows creating VMs with an empty `spec.virtualMachineClassName` field, which will be automatically filled with the default class. [#1305](https://github.com/deckhouse/virtualization/pull/1305)

## Fixes


 - **[module]** Added validation to ensure that virtual machine subnets do not overlap with system subnets (podSubnetCIDR and serviceSubnetCIDR). [#1324](https://github.com/deckhouse/virtualization/pull/1324)
 - **[vi]** To create a virtual image on a `PersistentVolumeClaim`, the storage must support the RWX and Block modes; otherwise, a warning will be displayed. [#1289](https://github.com/deckhouse/virtualization/pull/1289)
 - **[vm]** Fixed an issue where changing the operating system type caused the machine to enter a reboot loop. [#1358](https://github.com/deckhouse/virtualization/pull/1358)
 - **[vm]** Fixed an issue where a virtual machine would hang in the Starting phase when project quotas were insufficient. A quota shortage message will now be displayed in the virtual machine's status. To allow the machine to continue starting, the project quotas need to be increased. [#1314](https://github.com/deckhouse/virtualization/pull/1314)

