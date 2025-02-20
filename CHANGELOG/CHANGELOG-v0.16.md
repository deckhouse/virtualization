# Changelog v0.16

## Features


 - **[api]** create a network policy for the importer to avoid restrictions during import to dvcr [#675](https://github.com/deckhouse/virtualization/pull/675)
 - **[vi]** add the ability to create a VirtualImage from a VirtualDiskSnapshot [#617](https://github.com/deckhouse/virtualization/pull/617)
 - **[vm]** move error about exceeding the allowed number of block devices from DiskAttachmentCapacityAvailable into BlockDevicesReady condition [#633](https://github.com/deckhouse/virtualization/pull/633)
 - **[vmclass]** add events about available nodes and sizing policies changed [#606](https://github.com/deckhouse/virtualization/pull/606)
 - **[vmip]** add new events [#645](https://github.com/deckhouse/virtualization/pull/645)

## Fixes


 - **[api]** reduce max len of block device names to avoid errors during vm creation [#737](https://github.com/deckhouse/virtualization/pull/737)
 - **[core]** add validatingadmissionpolicies and validatingadmissionpolicybindings to rbac for virt-operator (required for kubernetes >=v1.30) [#749](https://github.com/deckhouse/virtualization/pull/749)
 - **[core]** manage pods network priority during a migration using the cilium label [#642](https://github.com/deckhouse/virtualization/pull/642)
 - **[images]** images can be successfully created from images on immediate storage class [#712](https://github.com/deckhouse/virtualization/pull/712)
 - **[vd]** add WaitingForFirstConsumer phase for virtual disks created from snapshots [#704](https://github.com/deckhouse/virtualization/pull/704)
 - **[vi]** fix the possible hanging on the Terminating state for the virtual image that was attached to a virtual machine [#755](https://github.com/deckhouse/virtualization/pull/755)
 - **[vi]** fixed hang in Pending when creating an image from a snapshot, improved messages in DatasourceReady condition [#721](https://github.com/deckhouse/virtualization/pull/721)
 - **[vm]** move the vm to a pending phase instead of starting it if it has invalid specs during start/restart to prevent the use of outdated specifications [#678](https://github.com/deckhouse/virtualization/pull/678)
 - **[vm]** disk serials are now generated using the MD5 hash of the disk uid instead of the disk name itself; this prevents errors caused by recent QEMU changes enforcing a strict 36-character limit on serial numbers [#710](https://github.com/deckhouse/virtualization/pull/710)
 - **[vmop]** fixed resource hang in InProgress during Evict/Migrate operations [#758](https://github.com/deckhouse/virtualization/pull/758)

## Chore


 - **[core]** fix build firmware (edk2) using with ovmf 4MB instead of 2MB [#707](https://github.com/deckhouse/virtualization/pull/707)

