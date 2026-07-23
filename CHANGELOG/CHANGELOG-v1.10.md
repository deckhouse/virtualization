# Changelog v1.10

## v1.10.0

### ci

- **fix** (low): Changelog entries no longer require impact_level, scheduled changelog pipelines run only their own job, and changelog MRs carry a readable changelog body. ([!35](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/35))
- **chore** (low): Align license headers of CDI-derived pvc-artifact files to pass license checks. ([!54](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/54))

### disks

- **fix** (default): A VirtualMachine with paravirtualization disabled no longer gets stuck in Pending when it boots from a newly created image-backed disk on a WaitForFirstConsumer storage class. ([!75](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/75))

### module

- **feature** (low): virtualMachineCIDRs in the ModuleConfig/virtualization is now optional; VM IP address management (IPAM) is simply unavailable until it is configured, instead of blocking the module from starting. ([!1](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/1))
- **fix** (default): VirtualMachinePool RBAC is granted only in editions where the feature is available, so the resource is not exposed in CE. ([!49](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/49))

### test

- **chore** (low): Blockdevice e2e suites run on the custom e2e-br image with observer-based waits and source-derived disk sizes. ([!19](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/19))

### vm

- **fix** (default): Virtual machines with several local disks migrate reliably: a migration no longer reports success while leaving a disk unmigrated, no longer hangs on an inconsistent volume set, no longer disrupts hotplugged disk attachments, and if it cannot proceed, it fails within minutes and the virtual machine recovers automatically. ([!2](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/2))
- **fix** (default): Virtual machines with several local disks migrate reliably: a migration no longer reports success while leaving a disk unmigrated, no longer hangs on an inconsistent volume set, no longer disrupts hotplugged disk attachments, and if it cannot proceed, it fails within minutes and the virtual machine recovers automatically. ([!10](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/10))
- **fix** (low): A virtual machine restored from a snapshot in the stopped state no longer gets stuck in Pending and can be started again. ([!31](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/31))
- **fix** (default): A VirtualMachine no longer stays stuck as migrating after a finished migration, which previously blocked all further migrations and evictions of that VM. ([!46](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/46))
- **fix** (default): Live migration of a virtual machine with additional networks no longer gets stuck waiting for the network on the target node. ([!50](https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/50))
