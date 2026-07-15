# Changelog v1.10

## Features


 - **[api]** Add the Superseded phase for VirtualMachineOperation resources interrupted by a newer operation. [#2492](https://github.com/deckhouse/virtualization/pull/2492)
 - **[core]** Add security events [#2611](https://github.com/deckhouse/virtualization/pull/2611)
 - **[core]** Automatically restore VirtualImage and ClusterVirtualImage from the ImageLost phase once the image reappears in DVCR, without re-importing the data. [#2564](https://github.com/deckhouse/virtualization/pull/2564)
 - **[core]** CDI has been removed: disks and images are now provisioned by the module's own importer and PVC populator, with live import progress. [#2394](https://github.com/deckhouse/virtualization/pull/2394)
 - **[core]** throttle the live-migration sync phase per source node [#2230](https://github.com/deckhouse/virtualization/pull/2230)
 - **[disks]** VirtualDisk names may use the full Kubernetes name length instead of being capped at 60 characters. [#2548](https://github.com/deckhouse/virtualization/pull/2548)
 - **[images]** VirtualImage and ClusterVirtualImage names may use the full Kubernetes name length instead of being capped at 49 and 48 characters. [#2548](https://github.com/deckhouse/virtualization/pull/2548)
 - **[module]** Optional per-namespace authorization for DVCR (`dvcr.tenantRegistryAuthorization`) isolating container image access between namespaces. [#2586](https://github.com/deckhouse/virtualization/pull/2586)
 - **[module]** Limit concurrent inbound live migrations per target node, configurable via ModuleConfig annotations. [#2362](https://github.com/deckhouse/virtualization/pull/2362)
 - **[network]** add metrics for conntrack sync [#2542](https://github.com/deckhouse/virtualization/pull/2542)
 - **[network]** bind live migration to a dedicated SystemNetwork [#2222](https://github.com/deckhouse/virtualization/pull/2222)
 - **[vm]** Add IPAM for additional network interfaces: automatic (DHCP) and static IP allocation via SDN IPAddress resources. [#2612](https://github.com/deckhouse/virtualization/pull/2612)
 - **[vm]** Add hotplug CPU and memory via in-place resize (alpha, behind feature gate HotplugCPUAndMemoryWithInPlaceResize). [#2247](https://github.com/deckhouse/virtualization/pull/2247)
 - **[vm]** VM networks moved to an eBPF datapath: stable additional-network connectivity with lower overhead. [#2212](https://github.com/deckhouse/virtualization/pull/2212)
 - **[vmpool]** Add VirtualMachinePool (EE/SE+) for declarative group management of virtual machines, scalable via the standard scale subresource, HPA and KEDA. [#2572](https://github.com/deckhouse/virtualization/pull/2572)

## Fixes


 - **[core]** Do not fail VM reconcile while a hotplug image is being attached via VMBDA. [#2574](https://github.com/deckhouse/virtualization/pull/2574)
 - **[core]** Fixed flapping of the VirtualMachineIPAddressReady condition when the VM had no guest-agent IP. [#2553](https://github.com/deckhouse/virtualization/pull/2553)
 - **[core]** VirtualMachineClass: for cpu.type=Discovery recompute CPU features from the current nodes on every reconcile (no stale cache) and separate the discovery nodeSelector pool (basis for the universal CPU model) from schedulable nodes derived from spec.nodeSelector; for cpu.type=Discovery and Features restore status.cpuFeatures.notEnabledCommon population. [#2501](https://github.com/deckhouse/virtualization/pull/2501)
 - **[cvi]** ClusterVirtualImage status messages are clearer and no longer expose internal implementation details. [#2558](https://github.com/deckhouse/virtualization/pull/2558)
 - **[disks]** Upload-type VirtualDisk no longer gets stuck in Pending when the shared upload host certificate becomes invalid. [#2610](https://github.com/deckhouse/virtualization/pull/2610)
 - **[disks]** Upload-type VirtualDisk again recovers automatically when the upload host changes, for example after publicDomainTemplate is updated. [#2610](https://github.com/deckhouse/virtualization/pull/2610)
 - **[images]** Upload-type VirtualImage and ClusterVirtualImage no longer get stuck in Pending when the shared upload host certificate becomes invalid. [#2610](https://github.com/deckhouse/virtualization/pull/2610)
 - **[images]** Upload endpoint now follows publicDomainTemplate changes instead of keeping the host it was created with. [#2527](https://github.com/deckhouse/virtualization/pull/2527)
 - **[module]** Restricted unauthorized access to the virtualization USB/IP gateway port. [#2571](https://github.com/deckhouse/virtualization/pull/2571)
 - **[vd]** VirtualDisk status messages are clearer and no longer expose internal implementation details. [#2558](https://github.com/deckhouse/virtualization/pull/2558)
 - **[vdsnapshot]** VirtualDiskSnapshot status messages have clearer wording. [#2558](https://github.com/deckhouse/virtualization/pull/2558)
 - **[vi]** PVC-backed virtual images are now attached to virtual machines in read-only mode, like registry-backed images. [#2620](https://github.com/deckhouse/virtualization/pull/2620)
 - **[vi]** VirtualImage status messages are clearer and no longer expose internal implementation details. [#2558](https://github.com/deckhouse/virtualization/pull/2558)
 - **[vm]** Block devices attached to a VM with disabled paravirtualization are no longer switched to the SATA bus and attach correctly. [#2645](https://github.com/deckhouse/virtualization/pull/2645)
 - **[vm]** Virtual machines with local disks can now be evacuated, updated and re-migrated even while a restart is pending, instead of getting stuck unable to migrate. [#2634](https://github.com/deckhouse/virtualization/pull/2634)
 - **[vm]** VMs requiring a pinned TSC frequency (Windows/Hyper-V, invtsc) no longer hang in Pending on heterogeneous clusters — the frequency is now picked among the nodes the VM can actually be scheduled on. [#2630](https://github.com/deckhouse/virtualization/pull/2630)
 - **[vm]** Disks and CD-ROMs now move to the virtio-scsi bus after enabling paravirtualization, so ISO drives can be unplugged from a running VM. [#2624](https://github.com/deckhouse/virtualization/pull/2624)
 - **[vm]** Live migrations no longer fail by target timeout while waiting for a free inbound migration slot. [#2616](https://github.com/deckhouse/virtualization/pull/2616)
 - **[vm]** Fix VM stuck until the child KVVMI in Failed phase is deleted manually. [#2596](https://github.com/deckhouse/virtualization/pull/2596)
 - **[vm]** VirtualMachine status messages are clearer and no longer expose internal implementation details. [#2558](https://github.com/deckhouse/virtualization/pull/2558)
 - **[vm]** VirtualMachine creation without an explicit VM class now reports a clear error when no default VirtualMachineClass is configured. [#2534](https://github.com/deckhouse/virtualization/pull/2534)
 - **[vm]** A VM can start again after a failed local-disk migration restart when `kvvmi` is already deleted and migrated volumes, including VMBDA-attached volumes, must be restored from target PVCs back to the current source PVCs. [#2506](https://github.com/deckhouse/virtualization/pull/2506)
 - **[vm]** Prevent Main-only virtual machines from requiring restart due to implicit default network template synchronization. [#2484](https://github.com/deckhouse/virtualization/pull/2484)
 - **[vm]** Fix hotplug volume cleanup after migration target pod termination. [#2457](https://github.com/deckhouse/virtualization/pull/2457)
 - **[vm]** VM pod volume error handling now includes FailedMapVolume and surfaces more complete pod volume diagnostics. [#2433](https://github.com/deckhouse/virtualization/pull/2433)
 - **[vm]** Fallback CPU/memory hotplug updates to restart when project quota cannot fit migration-time resources. [#2419](https://github.com/deckhouse/virtualization/pull/2419)
 - **[vmbda]** VirtualMachineBlockDeviceAttachment status messages are clearer and no longer expose internal implementation details. [#2558](https://github.com/deckhouse/virtualization/pull/2558)
 - **[vmclass]** VirtualMachineClass sizing policy errors now report the total memory to set and the field to change instead of per-core values, and CPU cores step and minimum-only memory limits are now enforced. [#2602](https://github.com/deckhouse/virtualization/pull/2602)
 - **[vmsnapshot]** VirtualMachineSnapshot status messages have clearer wording. [#2558](https://github.com/deckhouse/virtualization/pull/2558)

## Chore


 - **[api]** Integrate crd-enricher into the API CRD codegen pipeline. [#2622](https://github.com/deckhouse/virtualization/pull/2622)
 - **[core]** Fixed vulnerabilities:
    - CVE-2026-39822
    - CVE-2026-42505
    - CVE-2026-39828
    - CVE-2026-39829
    - CVE-2026-39830
    - CVE-2026-39831
    - CVE-2026-39832
    - CVE-2026-39835
    - CVE-2026-42508
    - CVE-2026-46595
    - CVE-2026-46597
    - CVE-2026-39827
    - CVE-2026-39833
    - CVE-2026-39834
    - CVE-2026-46598
    - CVE-2026-25681
    - CVE-2026-27136
    - CVE-2026-33814
    - CVE-2026-39821
    - CVE-2026-25680
    - CVE-2026-42502
    - CVE-2026-42506
    - CVE-2026-39824
    - GO-2026-5932 [#2673](https://github.com/deckhouse/virtualization/pull/2673)
 - **[core]** Fixed vulnerabilities:
    - CVE-2026-39822
    - CVE-2026-42505
    - CVE-2026-39828
    - CVE-2026-39829
    - CVE-2026-39830
    - CVE-2026-39831
    - CVE-2026-39832
    - CVE-2026-39835
    - CVE-2026-42508
    - CVE-2026-46595
    - CVE-2026-46597
    - CVE-2026-39827
    - CVE-2026-39833
    - CVE-2026-39834
    - CVE-2026-46598
    - CVE-2026-25681
    - CVE-2026-27136
    - CVE-2026-33814
    - CVE-2026-39821
    - CVE-2026-25680
    - CVE-2026-42502
    - CVE-2026-42506
    - CVE-2026-39824
    - GO-2026-5932 [#2648](https://github.com/deckhouse/virtualization/pull/2648)
 - **[core]** Fixed vulnerabilities:
    - CVE-2026-25680
    - CVE-2026-25681
    - CVE-2026-27136
    - CVE-2026-33814
    - CVE-2026-39821
    - CVE-2026-39827
    - CVE-2026-39828
    - CVE-2026-39829
    - CVE-2026-39830
    - CVE-2026-39832
    - CVE-2026-39835
    - CVE-2026-41579
    - CVE-2026-42502
    - CVE-2026-42506
    - CVE-2026-42508
    - CVE-2026-46595
    - CVE-2026-46597 [#2557](https://github.com/deckhouse/virtualization/pull/2557)
 - **[core]** Fixed vulnerability:
    - CVE-2026-2303 [#2515](https://github.com/deckhouse/virtualization/pull/2515)
 - **[vm]** Migrate VirtualMachineImageHotplug e2e test to new framework. [#2443](https://github.com/deckhouse/virtualization/pull/2443)
 - **[vmclass]** Ensure vmclass contract: discovered CPU features should be frozen after the first discovery. [#2664](https://github.com/deckhouse/virtualization/pull/2664)

