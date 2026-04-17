# Changelog v1.8

## Features


 - **[core]** Add VMOP migration progress reporting with detailed lifecycle reasons and smoother sync-phase progress calculation. [#2182](https://github.com/deckhouse/virtualization/pull/2182)
 - **[vm]** Simple implementation of CPU hotplug [#2147](https://github.com/deckhouse/virtualization/pull/2147)
 - **[vm]** Initial implementation of memory hotplug support: change spec.memory will set new memory size with live migration. [#2110](https://github.com/deckhouse/virtualization/pull/2110)

## Fixes


 - **[api]** Enhanced reliability of the upload server for block devices by adding readiness checks and fixing configuration delays to prevent upload errors. [#2178](https://github.com/deckhouse/virtualization/pull/2178)
 - **[cli]** Mute warnings about empty home directory while executing other d8 commands. [#2204](https://github.com/deckhouse/virtualization/pull/2204)
 - **[core]** Fixed vulnerabilities CVE-2026-32280, CVE-2026-32281, CVE-2026-32282, CVE-2026-32283, CVE-2026-32288, CVE-2026-32289 [#2196](https://github.com/deckhouse/virtualization/pull/2196)
 - **[core]** Fixed vulnerabilities CVE-2026-34986. [#2188](https://github.com/deckhouse/virtualization/pull/2188)
 - **[core]** Fix KVVM resync for unschedulable VMs when VMClass placement settings are changed. [#2177](https://github.com/deckhouse/virtualization/pull/2177)
 - **[core]** Fixed vulnerabilities CVE-2026-25679, CVE-2026-27142, CVE-2026-27139, CVE-2026-33186, CVE-2026-34040, CVE-2026-33997. [#2175](https://github.com/deckhouse/virtualization/pull/2175)
 - **[core]** Move virt-handler DaemonSet to hostNetwork [#2174](https://github.com/deckhouse/virtualization/pull/2174)
 - **[module]** Add VM PhaseAge column while keeping Age in VirtualMachine brief output. [#2203](https://github.com/deckhouse/virtualization/pull/2203)
 - **[observability]** Fix gaps in live migration metrics. [#2170](https://github.com/deckhouse/virtualization/pull/2170)
 - **[vm]** Fix VM getting stuck with a frozen filesystem when frozen during migration. [#2225](https://github.com/deckhouse/virtualization/pull/2225)
 - **[vm]** Remove the cilium KVVMI annotation when disconnecting the Main network. [#2185](https://github.com/deckhouse/virtualization/pull/2185)
 - **[vm]** CLI no longer sends force=false by default, and AlwaysForced policy now rejects only explicit force=false while preserving policy defaults for unset force. [#2179](https://github.com/deckhouse/virtualization/pull/2179)
 - **[vmop]** Force handling fixed for VM lifecycle operations: CLI stop now passes --force, and VMOP restart now honors spec.force in controller logic. [#2168](https://github.com/deckhouse/virtualization/pull/2168)

## Chore


 - **[core]** Revert hostPath changing for virt-handler volumes previously done for compatibility with the original Kubevirt. [#2226](https://github.com/deckhouse/virtualization/pull/2226)
 - **[core]** Fix vulnerabilitie CVE-2026-39883. [#2200](https://github.com/deckhouse/virtualization/pull/2200)
 - **[vmip]** Prevent creation of a VM IP outside the allowed IP range. [#2201](https://github.com/deckhouse/virtualization/pull/2201)
 - **[vmsnapshot]** Retry filesystem unfreeze if it fails during deletion or snapshot failure. [#2161](https://github.com/deckhouse/virtualization/pull/2161)

