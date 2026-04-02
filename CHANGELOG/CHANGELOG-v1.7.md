# Changelog v1.7

## Features


 - **[vm]** Reduced USB device downtime during virtual machine migration. [#2098](https://github.com/deckhouse/virtualization/pull/2098)
 - **[vm]** Added a garbage collector for completed and failed virtual machine pods:
    - Pods older than 24 hours are deleted.
    - No more than 2 completed pods are retained. [#2091](https://github.com/deckhouse/virtualization/pull/2091)

## Fixes


 - **[core]** Fixed validation for the `AlwaysForced` virtual machine migration policy: `VirtualMachineOperation` resources with the `Evict` or `Migrate` type without explicit `force=true` are now rejected for this policy. [#2120](https://github.com/deckhouse/virtualization/pull/2120)
 - **[core]** Fixed the creation of block devices from VMDK files (especially for VMDKs in the `streamOptimized` format used in exports from VMware). [#2065](https://github.com/deckhouse/virtualization/pull/2065)
 - **[vm]** Block devices can now be attached and detached even if the virtual machine is running on a cordoned node. [#2163](https://github.com/deckhouse/virtualization/pull/2163)
 - **[vm]** Fixed virtual machine eviction during node drain: pods responsible for block device attachments are no longer removed from a cordoned node before virtual machine migration is complete. [#2153](https://github.com/deckhouse/virtualization/pull/2153)
 - **[vm]** Fixed double storage quota consumption during migration of a virtual machine with local storage. [#2148](https://github.com/deckhouse/virtualization/pull/2148)
 - **[vm]** Fixed an issue where a virtual machine could get stuck in the `Maintenance` state during restore from a snapshot. [#2144](https://github.com/deckhouse/virtualization/pull/2144)
 - **[vm]** Stabilized the operation of USB devices for virtualization on Deckhouse version ≥1.76 and Kubernetes version ≥1.33. [#2137](https://github.com/deckhouse/virtualization/pull/2137)
 - **[vm]** Fixed the detection of USB devices on the host: previously, there was a possibility of duplicate USB devices appearing. [#2122](https://github.com/deckhouse/virtualization/pull/2122)

