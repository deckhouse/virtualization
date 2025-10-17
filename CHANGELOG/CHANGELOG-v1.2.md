# Changelog v1.2

## [MALFORMED]


 - #1588 invalid type "refactor"

## Fixes


 - **[vm]** Fixed an issue where the Virtualization Controller could panic on unexpected block device deletion. [#1585](https://github.com/deckhouse/virtualization/pull/1585)
 - **[vmbda]** Fix missing Serial for Attached images and disks in intvirtvm. [#1580](https://github.com/deckhouse/virtualization/pull/1580)
 - **[vmbda]** VMBDA now reports a clear error if the device is not available on the VM's node. [#1561](https://github.com/deckhouse/virtualization/pull/1561)

## Chore


 - **[core]** More renames for containers to work with containerd v2. [#1579](https://github.com/deckhouse/virtualization/pull/1579)
 - **[module]** Fix build for p11-kit, fix mount of /var/log/libvirt in virt-launcher image. [#1576](https://github.com/deckhouse/virtualization/pull/1576)
 - **[module]** Use at least golang 1.24 for all components. [#1575](https://github.com/deckhouse/virtualization/pull/1575)

