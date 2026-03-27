# Changelog v1.7

## Features


 - **[api]** VirtualDisk owner reference is now saved at snapshot time and restored when restoring from snapshot, so restored disks are again owned by the restored VirtualMachine. [#2032](https://github.com/deckhouse/virtualization/pull/2032)
 - **[core]** Add quota override label for PVC migration to prevent double counting. [#2148](https://github.com/deckhouse/virtualization/pull/2148)
 - **[core]** Reducing USB device downtime during VM migration. [#2098](https://github.com/deckhouse/virtualization/pull/2098)
 - **[vm]** Add garbage collection for completed/failed VM pods [#2091](https://github.com/deckhouse/virtualization/pull/2091)
 - **[vm]** separate scheduling for USB 2.0 (High-Speed) and USB 3.0 (SuperSpeed) over USBIP [#2045](https://github.com/deckhouse/virtualization/pull/2045)
 - **[vm]** add conntrack synchronization for live migration [#1939](https://github.com/deckhouse/virtualization/pull/1939)
 - **[vmop]** add validation for local storage migration in CE edition [#1950](https://github.com/deckhouse/virtualization/pull/1950)

## Fixes


 - **[api]** Improve storage class validation error messages for VirtualDisk and VirtualImage on PVC. [#2115](https://github.com/deckhouse/virtualization/pull/2115)
 - **[core]** Fix VM getting stuck in Maintenance mode during snapshot restore. [#2144](https://github.com/deckhouse/virtualization/pull/2144)
 - **[core]** Fix validation to require force=true for AlwaysForced liveMigrationPolicy. UI migrations without explicit force flag are now properly rejected. [#2120](https://github.com/deckhouse/virtualization/pull/2120)
 - **[core]** PreferForced live migration policy now uses autoConverge=true by default, but respects force=false. [#2111](https://github.com/deckhouse/virtualization/pull/2111)
 - **[core]** Exit maintenance mode after restore operation failure. [#2094](https://github.com/deckhouse/virtualization/pull/2094)
 - **[core]** Enable NRI hook in DRA USB driver to restore allocated device state after restart and guard Prepare/Unprepare until synchronization completes. [#2087](https://github.com/deckhouse/virtualization/pull/2087)
 - **[vm]** Fix eviction webhook label mismatch for hotplug pods. [#2153](https://github.com/deckhouse/virtualization/pull/2153)
 - **[vm]** Stabilize the order of network interfaces [#2001](https://github.com/deckhouse/virtualization/pull/2001)
 - **[vm]** show CSI and volume errors in VM status [#1766](https://github.com/deckhouse/virtualization/pull/1766)

## Chore


 - **[module]** Add SecurityPolicyExceptions for module Pods with extended permissions (ds/virt-handler, ds/virtualization-dra, ds/vm-route-forge). [#2026](https://github.com/deckhouse/virtualization/pull/2026)

