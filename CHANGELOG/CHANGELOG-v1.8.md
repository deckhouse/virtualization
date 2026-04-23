# Changelog v1.8

## Features


 - **[vm]** Added the `progress` field to the status of `VirtualMachineOperation` resources with the `Evict` and `Migrate` types to show operation progress. The corresponding `PROGRESS` column is displayed when running `d8 k get vmop` [#2182](https://github.com/deckhouse/virtualization/pull/2182)
 - **[vm]** Added the ability to change the number of CPUs in a virtual machine without manually stopping it. The new value is applied via live migration. To enable this functionality, add `HotplugCPUWithLiveMigration` to `.spec.settings.featureGates` in the `ModuleConfig` of the `virtualization` module. [#2147](https://github.com/deckhouse/virtualization/pull/2147)
 - **[vm]** Added initial support for changing virtual machine memory without manually stopping the virtual machine. The new `.spec.memory` value is applied via live migration. To enable this functionality, add `HotplugMemoryWithLiveMigration` to `.spec.settings.featureGates` in the `ModuleConfig` of the `virtualization` module. [#2110](https://github.com/deckhouse/virtualization/pull/2110)

## Fixes


 - **[api]** When uploading disks and images with the `Upload` type, the `WaitForUserUpload` phase no longer occurs prematurely while the resource is not yet ready for upload. [#2178](https://github.com/deckhouse/virtualization/pull/2178)
 - **[core]** Added automatic cleanup of `NodeUSBDevice` resources that are absent on the node and are not assigned to a namespace or project. [#2220](https://github.com/deckhouse/virtualization/pull/2220)
 - **[vm]** Fixed an issue with an unfrozen filesystem during virtual machine snapshot creation if the freeze occurred during migration. [#2225](https://github.com/deckhouse/virtualization/pull/2225)
 - **[vm]** Fixed removal of the `Main` network from a virtual machine: the virtual machine no longer uses an IP address from the virtualization CIDR after the network is removed. [#2185](https://github.com/deckhouse/virtualization/pull/2185)
 - **[vm]** Optimized virtual machine migration: it now uses `hostNetwork`, allowing the host MTU to be used instead of the pod MTU. [#2174](https://github.com/deckhouse/virtualization/pull/2174)
 - **[vmsnapshot]** Fixed snapshot creation for a virtual machine without the `Main` network. [#2176](https://github.com/deckhouse/virtualization/pull/2176)

## Chore


 - **[core]** Fix vulnerability CVE-2026-39883. [#2200](https://github.com/deckhouse/virtualization/pull/2200)
 - **[core]** Fixed vulnerabilities CVE-2026-32280, CVE-2026-32281, CVE-2026-32282, CVE-2026-32283, CVE-2026-32288, CVE-2026-32289 [#2196](https://github.com/deckhouse/virtualization/pull/2196)
 - **[core]** Fixed vulnerabilities CVE-2026-34986. [#2188](https://github.com/deckhouse/virtualization/pull/2188)
 - **[core]** Fixed vulnerabilities CVE-2026-25679, CVE-2026-27142, CVE-2026-27139, CVE-2026-33186, CVE-2026-34040, CVE-2026-33997. [#2175](https://github.com/deckhouse/virtualization/pull/2175)

