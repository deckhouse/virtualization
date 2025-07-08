# Changelog v0.20

## Features


 - **[module]** The new minimum required version of Deckhouse 1.69.4 has been set, which is necessary for the operation of the virtualization module. The virtualization module has been moved from the preview stage to general availability. [#1109](https://github.com/deckhouse/virtualization/pull/1109)
 - **[module]** Add the smibios parameter to determine the level of virtualization nesting. This parameter allows automatic detection of whether a node is running on physical hardware or in a DVP virtualized environment. [#559](https://github.com/deckhouse/virtualization/pull/559)
 - **[vmip]** Add events for the `VirtualMachineIPAddress` resource. [#1147](https://github.com/deckhouse/virtualization/pull/1147)
 - **[vmipl]** Add events for the `VirtualMachineIPAddressLease` resource. [#1146](https://github.com/deckhouse/virtualization/pull/1146)

## Fixes


 - **[api]** The allowed name lengths for resources have been adjusted and the corresponding validation has been added:
    - VirtualMachine: 63 characters
    - ClusterVirtualImage: 36 characters
    - VirtualImage: 37 characters
    - VirtualDisk: 60 characters [#1177](https://github.com/deckhouse/virtualization/pull/1177)
 - **[core]** Default parameters for live migration have been set: Migration bandwidth: 5 Gbps (approximately 640 MB/s); Each node will perform no more than one outgoing migration at a time; The total number of simultaneous migrations in the cluster is limited to the number of nodes running virtual machines. [#1082](https://github.com/deckhouse/virtualization/pull/1082)
 - **[module]** Fixed a hang during virtualization version upgrade in an HA cluster with two system nodes. [#1173](https://github.com/deckhouse/virtualization/pull/1173)
 - **[vd]** The creation of virtual disks using the storage class of the `local-path-provisioner` module has been fixed.
    Support for storage classes managed by the local-path-provisioner module will be discontinued starting from version 0.22. [#1228](https://github.com/deckhouse/virtualization/pull/1228)
 - **[vdsnapshot]** Fix the unfreezing of the virtual machine's file system in case of an error during snapshot creation. [#1117](https://github.com/deckhouse/virtualization/pull/1117)
 - **[vmop]** Fix the premature transition of a resource to the InProgress state if a migration is scheduled but has not started. Now, it remains in the Pending state until the migration begins. [#1128](https://github.com/deckhouse/virtualization/pull/1128)

## Chore


 - **[module]** Address CVEs related to Python hooks: CVE-2024-12797, CVE-2025-47273. [#1103](https://github.com/deckhouse/virtualization/pull/1103)

