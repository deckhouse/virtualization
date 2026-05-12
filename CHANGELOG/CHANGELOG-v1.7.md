# Changelog v1.7

## Know before update


 - Action is required to update the firmware on virtual machines with a connected USB device (a corresponding migration prompt will appear in the VM status):
    - either disconnect the USB device and migrate the virtual machine;
    - or restart the virtual machine.
    
    Until the action is taken, the virtual machine will continue running, but it cannot be migrated.
    After completing the action, the virtual machine will be updated to the current firmware version and will be ready for migration again.

## Features


 - **[vm]** Reduced USB device downtime during virtual machine migration. [#2098](https://github.com/deckhouse/virtualization/pull/2098)
 - **[vm]** Added a garbage collector for completed and failed virtual machine pods:
    - Pods older than 24 hours are deleted.
    - No more than 2 completed pods are retained. [#2091](https://github.com/deckhouse/virtualization/pull/2091)
 - **[vm]** When scheduling virtual machines on nodes, the system now takes into account whether a USB device uses USB 2.0 (High-Speed) or USB 3.0 (SuperSpeed). [#2045](https://github.com/deckhouse/virtualization/pull/2045)
 - **[vm]** VirtualDisk owner reference is now saved at snapshot time and restored when restoring from snapshot, so restored disks are again owned by the restored VirtualMachine. [#2032](https://github.com/deckhouse/virtualization/pull/2032)
 - **[vm]** Added a mechanism to prevent TCP connection drops during live migration of a virtual machine. [#1939](https://github.com/deckhouse/virtualization/pull/1939)

## Fixes


 - **[core]** Fixed validation for the `AlwaysForced` virtual machine migration policy: `VirtualMachineOperation` resources with the `Evict` or `Migrate` type without explicit `force=true` are now rejected for this policy. [#2120](https://github.com/deckhouse/virtualization/pull/2120)
 - **[core]** Fixed the creation of block devices from VMDK files (especially for VMDKs in the `streamOptimized` format used in exports from VMware). [#2065](https://github.com/deckhouse/virtualization/pull/2065)
 - **[vm]** Action is required to update the firmware on virtual machines with a connected USB device. [#2166](https://github.com/deckhouse/virtualization/pull/2166)
    Action is required to update the firmware on virtual machines with a connected USB device (a corresponding migration prompt will appear in the VM status):
    - either disconnect the USB device and migrate the virtual machine;
    - or restart the virtual machine.
    
    Until the action is taken, the virtual machine will continue running, but it cannot be migrated.
    After completing the action, the virtual machine will be updated to the current firmware version and will be ready for migration again.
 - **[vm]** Block devices can now be attached and detached even if the virtual machine is running on a cordoned node. [#2163](https://github.com/deckhouse/virtualization/pull/2163)
 - **[vm]** Fixed virtual machine eviction during node drain: pods responsible for block device attachments are no longer removed from a cordoned node before virtual machine migration is complete. [#2153](https://github.com/deckhouse/virtualization/pull/2153)
 - **[vm]** Fixed double storage quota consumption during migration of a virtual machine with local storage. [#2148](https://github.com/deckhouse/virtualization/pull/2148)
 - **[vm]** Fixed an issue where a virtual machine could get stuck in the `Maintenance` state during restore from a snapshot. [#2144](https://github.com/deckhouse/virtualization/pull/2144)
 - **[vm]** Stabilized the operation of USB devices for virtualization on Deckhouse version ≥1.76 and Kubernetes version ≥1.33. [#2137](https://github.com/deckhouse/virtualization/pull/2137)
 - **[vm]** Fixed the detection of USB devices on the host: previously, there was a possibility of duplicate USB devices appearing. [#2122](https://github.com/deckhouse/virtualization/pull/2122)
 - **[vm]** The order of additional network interfaces is now deterministic and does not change after virtual machine restarts. [#2001](https://github.com/deckhouse/virtualization/pull/2001)
 - **[vm]** Added storage-side error messages (from the CSI driver) to the virtual machine status for block device attachment failures. [#1766](https://github.com/deckhouse/virtualization/pull/1766)

## Chore


 - **[core]** Fixed vulnerability:
    - CVE-2026-32283
    - CVE-2026-27139
    - CVE-2026-32289
    - CVE-2026-32288
    - CVE-2026-32281
    - CVE-2026-27142
    - CVE-2026-33997
    - CVE-2026-33726
    - CVE-2026-32282
    - CVE-2026-32280
    - CVE-2026-25679
    - CVE-2026-34040
    - CVE-2026-34986
    - CVE-2026-39883
    - CVE-2026-33186 [#2219](https://github.com/deckhouse/virtualization/pull/2219)

