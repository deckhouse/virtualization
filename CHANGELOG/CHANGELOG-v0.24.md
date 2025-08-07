# Changelog v0.24

## [MALFORMED]


 - #1263 invalid type "feat"
 - #1288 invalid type "feat"

## Features


 - **[api]** Remove setting of VolumeSnapshotClass. Set field deprecated in CRDS's. [#1274](https://github.com/deckhouse/virtualization/pull/1274)
 - **[core]** bump kubevirt to tag v1.3.1-v12n.8. Set mac address for non default pod network and improve reason when live-migration failed [#1287](https://github.com/deckhouse/virtualization/pull/1287)
 - **[vd]** Add `Exporting` phase and new conditions to `VirtualDisk` status [#1256](https://github.com/deckhouse/virtualization/pull/1256)
 - **[vm]** Add additional network interfaces for VirtualMachines. [#1253](https://github.com/deckhouse/virtualization/pull/1253)

## Fixes


 - **[core]** fix CVE-2025-22868 [#1322](https://github.com/deckhouse/virtualization/pull/1322)
 - **[module]** Fix helm template to be compatible with CustomCertificate https mode. [#1297](https://github.com/deckhouse/virtualization/pull/1297)
 - **[observability]** fix alerts D8InternalVirtualizationVirtHandlerTargetAbsent and D8InternalVirtualizationVirtHandlerTargetDown, by removing them and adding virtualization virt metrics state [#1291](https://github.com/deckhouse/virtualization/pull/1291)
 - **[vd]** Fail with error on insufficient PVC size [#1295](https://github.com/deckhouse/virtualization/pull/1295)
 - **[vd]** Set ImageNotReady/ClusterImageNotReady condition when VI/CVI is missing. [#1286](https://github.com/deckhouse/virtualization/pull/1286)
 - **[vd]** Improve virtual disk protection logic during deletion [#1285](https://github.com/deckhouse/virtualization/pull/1285)
 - **[vm]** Fix an issue where multiple networks of type "Main" could be specified in a virtual machine's spec. [#1299](https://github.com/deckhouse/virtualization/pull/1299)
 - **[vm]** Add react on create virtual machine event for WorkloadUpdater controller [#1293](https://github.com/deckhouse/virtualization/pull/1293)
 - **[vm]** Add validation to ensure that names in spec.blockDeviceRefs do not exceed the maximum allowed lengths. [#1276](https://github.com/deckhouse/virtualization/pull/1276)

## Chore


 - **[api]** Update the IsStorageClassDeprecated method to accept a StorageClass pointer instead of a string. [#1264](https://github.com/deckhouse/virtualization/pull/1264)
 - **[docs]** Examples of using the user interface have been added to the documentation [#1270](https://github.com/deckhouse/virtualization/pull/1270)

