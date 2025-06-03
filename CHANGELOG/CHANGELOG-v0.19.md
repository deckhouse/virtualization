# Changelog v0.19

## Fixes


 - **[module]** Fix a potential displacement of service components of virtualization under high load on control plane nodes. Set system-cluster-critical priority class for critical components. [#1113](https://github.com/deckhouse/virtualization/pull/1113)
 - **[module]** Fix the update parameters for control-plane components in HA mode, which caused the update to hang. [#1092](https://github.com/deckhouse/virtualization/pull/1092)
 - **[vm]** Set the maximum size of the embedded cloud-init block to 2048 bytes. [#1083](https://github.com/deckhouse/virtualization/pull/1083)
 - **[vm]** Add a mechanism to clean up zombie processes from the virtual machine container. [#1058](https://github.com/deckhouse/virtualization/pull/1058)
 - **[vm]** Optimize the display of conditions. Now only relevant conditions are shown in the virtual machine status. [#1023](https://github.com/deckhouse/virtualization/pull/1023)
 - **[vmclass]** Fix an issue when creating a VirtualMachineClass resource that could remain in a Pending state during cluster creation or when adding nodes [#1075](https://github.com/deckhouse/virtualization/pull/1075)
 - **[vmip]** Fix an issue with the creation of duplicate VirtualMachineIPAddressLease resources for a single virtual machine. [#1081](https://github.com/deckhouse/virtualization/pull/1081)
 - **[vmip]** Fix creating many vmipleases from one vmip. [#1012](https://github.com/deckhouse/virtualization/pull/1012)
 - **[vmsnapshot]** Fix an issue where the VirtualMachineSnapshot resource could remain in a Pending state because it was unable to determine the status of the virtual machine agent. [#1065](https://github.com/deckhouse/virtualization/pull/1065)

## Chore


 - **[core]** Remove unused binary files from containers as part of the certification preparation. [#1042](https://github.com/deckhouse/virtualization/pull/1042)
 - **[module]** Update module dependencies to address existing vulnerabilities CVE-2024-45337,CVE-2025-22869, CVE-2025-22870, CVE-2025-22872, CVE-2025-27144, CVE-2024-45336, CVE-2024-45341, CVE-2025-22866, CVE-2025-22871. [#1039](https://github.com/deckhouse/virtualization/pull/1039)

