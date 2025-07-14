# Changelog v0.21

## Features


 - **[core]** Add automatic rebalancing of virtual machines to optimize load distribution among cluster nodes based on CPU usage threshold (80%) and affinity/anti-affinity rules. This functionality is activated only when the `descheduler` module is enabled. [#962](https://github.com/deckhouse/virtualization/pull/962)
 - **[module]** Add detection of virtualization-capable nodes (with /dev/kvm enabled and support for VMX/SVM processor instructions) to schedule virtual machine deployment only on suitable nodes. [#1076](https://github.com/deckhouse/virtualization/pull/1076)
 - **[vm]** Add the ability for dynamic attachment (hotplug) of a virtual disk in `Filesystem` mode to a virtual machine. [#1060](https://github.com/deckhouse/virtualization/pull/1060)

## Fixes


 - **[vd]** The creation of virtual disks using the storage class of the `local-path-provisioner` module has been fixed.
    Support for storage classes managed by the local-path-provisioner module will be discontinued starting from version 0.22. [#1228](https://github.com/deckhouse/virtualization/pull/1228)
 - **[vd]** Fix the update of the `.status.observedGeneration` field for a virtual disk in the Ready state if the image from which the disk was created no longer exists in the cluster. [#1124](https://github.com/deckhouse/virtualization/pull/1124)
 - **[vmip]** Fix the deletion of old VirtualMachineIPAddress resources that may have had a legacy finalizer blocking deletion. [#1220](https://github.com/deckhouse/virtualization/pull/1220)
 - **[vmip]** Fix a potential hang during the deletion of a VirtualMachineIPAddress resource when deleting a virtual machine. [#1185](https://github.com/deckhouse/virtualization/pull/1185)

