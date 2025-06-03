# Changelog v0.18

## Features


 - **[module]** A dashboard has been added showing memory synchronization statistics of the VM during migration. [#1029](https://github.com/deckhouse/virtualization/pull/1029)
 - **[module]** An audit controller has been added to track security events related to the virtualization module's resources. [#801](https://github.com/deckhouse/virtualization/pull/801)
 - **[vm]** Report I/O errors to guest OS instead of stopping VM, allowing the guest system to deal with the problem (e.g., through retry mechanisms, failover). [#983](https://github.com/deckhouse/virtualization/pull/983)
 - **[vm]** Ability to force migration with CPU throttling. Live migration policy can be set in VM and user can override its value with VMOP. [#890](https://github.com/deckhouse/virtualization/pull/890)
 - **[vmsnapshot]** The status of the VirtualMachineSnapshot resource now displays information about the resources included in the snapshot. [#978](https://github.com/deckhouse/virtualization/pull/978)

## Fixes


 - **[module]** Fix the polk alerts annotations of the virtual machine state. [#1094](https://github.com/deckhouse/virtualization/pull/1094)
 - **[vd]** Fix cleanup for CVI and VI when creating from object reference with the type VirtualDisk. [#996](https://github.com/deckhouse/virtualization/pull/996)
 - **[vm]** The collection of metrics based on conditions has been fixed. `AwaitingRestartToApplyConfiguration`, `ConfigurationApplied`, `AgentReady` and `FirmwareUpToDate` conditions of the virtual machine are no longer displayed to the user unless they contain an issue or an important message. However, this change was not initially reflected in the metrics. It has now been corrected. [#1069](https://github.com/deckhouse/virtualization/pull/1069)
 - **[vm]** The InUse condition is now correctly removed when the virtual machine class is no longer used by any VM. [#1009](https://github.com/deckhouse/virtualization/pull/1009)
 - **[vm]** Resolved an issue where it was impossible to stop a VM if there were unapplied changes in its configuration. [#991](https://github.com/deckhouse/virtualization/pull/991)
 - **[vm]** Improved the logic for handling VM conditions and enhanced the status output for more accurate monitoring. [#931](https://github.com/deckhouse/virtualization/pull/931)
 - **[vm]** To enhance security, all images will be mounted as `read-only`. [#796](https://github.com/deckhouse/virtualization/pull/796)
 - **[vmipl]** Fixed an issue with the incorrect removal of the finalizer from the VirtualMachineIPLease resource. [#1006](https://github.com/deckhouse/virtualization/pull/1006)

## Chore


 - **[docs]** Updated the documentation to align with the virtualization updates in version 0.18.0. [#992](https://github.com/deckhouse/virtualization/pull/992)

