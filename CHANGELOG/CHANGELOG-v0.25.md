# Changelog v0.25

## [MALFORMED]


 - #1367 missing section, missing summary, missing type, unknown section ""
 - #1378 missing section, missing summary, missing type, unknown section ""
 - #1381 missing section, missing summary, missing type, unknown section ""

## Features


 - **[module]** fix helm templates after PR#1324 [#1346](https://github.com/deckhouse/virtualization/pull/1346)
 - **[observability]** Add phase metrics for VirtualMachineSnapshot, VirtualDiskSnapshot, VirtualImage and ClusterVirtualImage resources. [#1356](https://github.com/deckhouse/virtualization/pull/1356)
 - **[vm]** auto recreate pod with virtual machine if resource requirements updated. Watch project quota update and delete to enqueue VMI [#1296](https://github.com/deckhouse/virtualization/pull/1296)
 - **[vmclass]** VirtualMachinesClass can be assigned as default to automatically fill empty spec.virtualMachineClassName field in VirtualMachines. [#1305](https://github.com/deckhouse/virtualization/pull/1305)
 - **[vmop]** Surface "quota exceeded" error during migration. [#1310](https://github.com/deckhouse/virtualization/pull/1310)

## Fixes


 - **[api]** Enqueue stucked by quota entities when resource quota updated. [#1323](https://github.com/deckhouse/virtualization/pull/1323)
 - **[core]** Add SELinux type spc_t to InternalVirtualizationKubeVirt [#1321](https://github.com/deckhouse/virtualization/pull/1321)
 - **[vdsnapshot]** The VirtualDiskSnapshot retains its Ready status even if the VirtualDisk is simultaneously attached to another VirtualMachine after a successful snapshotting process. [#1370](https://github.com/deckhouse/virtualization/pull/1370)
 - **[vm]** Fixed TPM device cleanup when changing osType from Windows to Generic [#1358](https://github.com/deckhouse/virtualization/pull/1358)
 - **[vm]** A VMBDA should be correctly deleted when a VM is in the stopped phase. [#1351](https://github.com/deckhouse/virtualization/pull/1351)
 - **[vm]** Prevent "Starting" hang when quota is exceeded. [#1314](https://github.com/deckhouse/virtualization/pull/1314)
 - **[vmrestore]** Resource validation works properly when a virtual machine is restored in Safe mode. [#1357](https://github.com/deckhouse/virtualization/pull/1357)
 - **[vmsnapshot]** The Virtual Machine Snapshot controller correctly watches for the phase of virtual disks. [#1331](https://github.com/deckhouse/virtualization/pull/1331)

## Chore


 - **[module]** fix control plane alerts [#1330](https://github.com/deckhouse/virtualization/pull/1330)
 - **[module]** Rewrite module_config_validator.py hook in Go, remove python dependencies from the build. [#1324](https://github.com/deckhouse/virtualization/pull/1324)

