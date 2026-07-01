# Changelog v1.10

## Features


 - **[api]** Add the Superseded phase for VirtualMachineOperation resources interrupted by a newer operation. [#2492](https://github.com/deckhouse/virtualization/pull/2492)
 - **[disks]** VirtualDisk names may use the full Kubernetes name length instead of being capped at 60 characters. [#2548](https://github.com/deckhouse/virtualization/pull/2548)
 - **[images]** VirtualImage and ClusterVirtualImage names may use the full Kubernetes name length instead of being capped at 49 and 48 characters. [#2548](https://github.com/deckhouse/virtualization/pull/2548)
 - **[module]** Limit concurrent inbound live migrations per target node, configurable via ModuleConfig annotations. [#2362](https://github.com/deckhouse/virtualization/pull/2362)
 - **[vm]** Add hotplug CPU and memory via in-place resize (alpha, behind feature gate HotplugCPUAndMemoryWithInPlaceResize). [#2247](https://github.com/deckhouse/virtualization/pull/2247)

## Fixes


 - **[core]** Fixed flapping of the VirtualMachineIPAddressReady condition when the VM had no guest-agent IP. [#2553](https://github.com/deckhouse/virtualization/pull/2553)
 - **[core]** VirtualMachineClass: for cpu.type=Discovery recompute CPU features from the current nodes on every reconcile (no stale cache) and separate the discovery nodeSelector pool (basis for the universal CPU model) from schedulable nodes derived from spec.nodeSelector; for cpu.type=Discovery and Features restore status.cpuFeatures.notEnabledCommon population. [#2501](https://github.com/deckhouse/virtualization/pull/2501)
 - **[cvi]** ClusterVirtualImage status messages are clearer and no longer expose internal implementation details. [#2558](https://github.com/deckhouse/virtualization/pull/2558)
 - **[images]** Upload endpoint now follows publicDomainTemplate changes instead of keeping the host it was created with. [#2527](https://github.com/deckhouse/virtualization/pull/2527)
 - **[vd]** VirtualDisk status messages are clearer and no longer expose internal implementation details. [#2558](https://github.com/deckhouse/virtualization/pull/2558)
 - **[vdsnapshot]** VirtualDiskSnapshot status messages have clearer wording. [#2558](https://github.com/deckhouse/virtualization/pull/2558)
 - **[vi]** VirtualImage status messages are clearer and no longer expose internal implementation details. [#2558](https://github.com/deckhouse/virtualization/pull/2558)
 - **[vm]** VirtualMachine status messages are clearer and no longer expose internal implementation details. [#2558](https://github.com/deckhouse/virtualization/pull/2558)
 - **[vm]** VirtualMachine creation without an explicit VM class now reports a clear error when no default VirtualMachineClass is configured. [#2534](https://github.com/deckhouse/virtualization/pull/2534)
 - **[vm]** A VM can start again after a failed local-disk migration restart when `kvvmi` is already deleted and migrated volumes, including VMBDA-attached volumes, must be restored from target PVCs back to the current source PVCs. [#2506](https://github.com/deckhouse/virtualization/pull/2506)
 - **[vm]** Prevent Main-only virtual machines from requiring restart due to implicit default network template synchronization. [#2484](https://github.com/deckhouse/virtualization/pull/2484)
 - **[vm]** Fix hotplug volume cleanup after migration target pod termination. [#2457](https://github.com/deckhouse/virtualization/pull/2457)
 - **[vm]** VM pod volume error handling now includes FailedMapVolume and surfaces more complete pod volume diagnostics. [#2433](https://github.com/deckhouse/virtualization/pull/2433)
 - **[vm]** Fallback CPU/memory hotplug updates to restart when project quota cannot fit migration-time resources. [#2419](https://github.com/deckhouse/virtualization/pull/2419)
 - **[vmbda]** VirtualMachineBlockDeviceAttachment status messages are clearer and no longer expose internal implementation details. [#2558](https://github.com/deckhouse/virtualization/pull/2558)
 - **[vmsnapshot]** VirtualMachineSnapshot status messages have clearer wording. [#2558](https://github.com/deckhouse/virtualization/pull/2558)

## Chore


 - **[core]** Fixed vulnerability:
    - CVE-2026-2303 [#2515](https://github.com/deckhouse/virtualization/pull/2515)

