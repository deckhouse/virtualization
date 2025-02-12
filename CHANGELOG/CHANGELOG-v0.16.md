# Changelog v0.16

## Features


 - **[api]** create a network policy for the importer to avoid restrictions during import to dvcr [#675](https://github.com/deckhouse/virtualization/pull/675)
 - **[vi]** add the ability to create a VirtualImage from a VirtualDiskSnapshot [#617](https://github.com/deckhouse/virtualization/pull/617)
 - **[vm]** move error about exceeding the allowed number of block devices from DiskAttachmentCapacityAvailable into BlockDevicesReady condition [#633](https://github.com/deckhouse/virtualization/pull/633)
 - **[vmclass]** add events about available nodes and sizing policies changed [#606](https://github.com/deckhouse/virtualization/pull/606)
 - **[vmip]** add new events [#645](https://github.com/deckhouse/virtualization/pull/645)

## Fixes


 - **[api]** reduce max len of block device names to avoid errors during vm creation [#737](https://github.com/deckhouse/virtualization/pull/737)
 - **[core]** manage pods network priority during a migration using the cilium label [#642](https://github.com/deckhouse/virtualization/pull/642)
 - **[images]** images can be successfully created from images on immediate storage class [#712](https://github.com/deckhouse/virtualization/pull/712)
 - **[vd]** add WaitingForFirstConsumer phase for virtual disks created from snapshots [#704](https://github.com/deckhouse/virtualization/pull/704)
 - **[vm]** disk serials are now generated using the MD5 hash of the disk uid instead of the disk name itself; this prevents errors caused by recent QEMU changes enforcing a strict 36-character limit on serial numbers [#710](https://github.com/deckhouse/virtualization/pull/710)

## Chore


 - **[core]** fix build firmware (edk2) using with ovmf 4MB instead of 2MB [#707](https://github.com/deckhouse/virtualization/pull/707)

