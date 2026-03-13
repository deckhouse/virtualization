# Changelog v1.7

## Features


 - **[api]** VirtualDisk owner reference is now saved at snapshot time and restored when restoring from snapshot, so restored disks are again owned by the restored VirtualMachine. [#2032](https://github.com/deckhouse/virtualization/pull/2032)
 - **[vm]** add conntrack synchronization for live migration [#1939](https://github.com/deckhouse/virtualization/pull/1939)
 - **[vmop]** add validation for local storage migration in CE edition [#1950](https://github.com/deckhouse/virtualization/pull/1950)

## Fixes


 - **[core]** Exit maintenance mode after restore operation failure. [#2094](https://github.com/deckhouse/virtualization/pull/2094)
 - **[vm]** show CSI and volume errors in VM status [#1766](https://github.com/deckhouse/virtualization/pull/1766)

## Chore


 - **[module]** Add SecurityPolicyExceptions for module Pods with extended permissions (ds/virt-handler, ds/virtualization-dra, ds/vm-route-forge). [#2026](https://github.com/deckhouse/virtualization/pull/2026)

