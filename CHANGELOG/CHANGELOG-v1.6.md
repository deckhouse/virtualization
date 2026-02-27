# Changelog v1.6

## Features


 - **[api]** Add NodeUSBDevice and USBDevice. [#1913](https://github.com/deckhouse/virtualization/pull/1913)
 - **[module]** add vm pods info to vm dashboard [#2002](https://github.com/deckhouse/virtualization/pull/2002)
 - **[module]** add Virtualization Overview dashboard and VM info metric [#1956](https://github.com/deckhouse/virtualization/pull/1956)
 - **[vm]** Extend the `vlct` command with a `--from-file` flag that allows users to display domain information directly from a local libvirt XML file [#2014](https://github.com/deckhouse/virtualization/pull/2014)
 - **[vm]** add usb over ip [#1840](https://github.com/deckhouse/virtualization/pull/1840)
 - **[vm]** Implementation of dynamic USB device allocation in a cluster using the Kubernetes DRA mechanism. [#1696](https://github.com/deckhouse/virtualization/pull/1696)
 - **[vmsnapshot]** add d8v prefix to snapshot supplemental resources [#1733](https://github.com/deckhouse/virtualization/pull/1733)

## Fixes


 - **[vd]** Ensure tolerations are copied from VM to VD importer Prime for WFFC [#1999](https://github.com/deckhouse/virtualization/pull/1999)
 - **[vm]** correct networks spec validation for main-only case [#2027](https://github.com/deckhouse/virtualization/pull/2027)
 - **[vm]** Labels and annotations now work properly on virtual machines. [#1971](https://github.com/deckhouse/virtualization/pull/1971)
 - **[vm]** Prevent OOMKills for the target Pod during VM migration with hotplugged disks. [#1947](https://github.com/deckhouse/virtualization/pull/1947)

## Chore


 - **[api]** deprecate VirtualMachineRestore and remove related tests [#1959](https://github.com/deckhouse/virtualization/pull/1959)
 - **[module]** add module.yaml to image release-channel-version [#2023](https://github.com/deckhouse/virtualization/pull/2023)
 - **[vmbda]** Add message explaining what to do when vd/vi/cvi stuck in Terminating state. [#2000](https://github.com/deckhouse/virtualization/pull/2000)
 - **[vmop]** Do not clone labels on VM by restore virtual machine operation. [#2009](https://github.com/deckhouse/virtualization/pull/2009)
 - **[vmop]** Remove CloneCompleted and RestoreComplete conditions. Refactoring. [#1925](https://github.com/deckhouse/virtualization/pull/1925)

