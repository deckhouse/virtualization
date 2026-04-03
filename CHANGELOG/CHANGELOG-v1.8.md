# Changelog v1.8

## Fixes


 - **[core]** Fixed vulnerabilities CVE-2026-25679, CVE-2026-27142, CVE-2026-27139, CVE-2026-33186, CVE-2026-34040, CVE-2026-33997. [#2175](https://github.com/deckhouse/virtualization/pull/2175)
 - **[observability]** Fix gaps in live migration metrics. [#2170](https://github.com/deckhouse/virtualization/pull/2170)
 - **[vm]** Remove the cilium KVVMI annotation when disconnecting the Main network. [#2185](https://github.com/deckhouse/virtualization/pull/2185)
 - **[vmop]** Force handling fixed for VM lifecycle operations: CLI stop now passes --force, and VMOP restart now honors spec.force in controller logic. [#2168](https://github.com/deckhouse/virtualization/pull/2168)

