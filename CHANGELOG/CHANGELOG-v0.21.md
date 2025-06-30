# Changelog v0.21

## Features


 - **[core]** Add automatic rebalancing of virtual machines to optimize load distribution among cluster nodes based on CPU usage threshold (80%) and affinity/anti-affinity rules. This functionality is activated only when the `descheduler` module is enabled. [#962](https://github.com/deckhouse/virtualization/pull/962)
 - **[module]** Add detection of virtualization-capable nodes (with /dev/kvm enabled and support for VMX/SVM processor instructions) to schedule virtual machine deployment only on suitable nodes. [#1076](https://github.com/deckhouse/virtualization/pull/1076)
 - **[vm]** Add the ability for dynamic attachment (hotplug) of a virtual disk in `Filesystem` mode to a virtual machine. [#1060](https://github.com/deckhouse/virtualization/pull/1060)

## Fixes


 - **[vd]** Fix the update of the `.status.observedGeneration` field for a virtual disk in the Ready state if the image from which the disk was created no longer exists in the cluster. [#1124](https://github.com/deckhouse/virtualization/pull/1124)
 - **[vmip]** Fix a potential hang during the deletion of a VirtualMachineIPAddress resource when deleting a virtual machine. [#1185](https://github.com/deckhouse/virtualization/pull/1185)

