# Changelog v1.9

## Features


 - **[core]** CLI lifecycle commands (start, stop, restart, evict, migrate) now support multiple VM targets, --all flag, --label-selector flag for label-based targeting, and --yes flag for non-interactive confirmation. [#2412](https://github.com/deckhouse/virtualization/pull/2412)
 - **[core]** Compatible VirtualMachineOperation resources can now supersede another active operation on the same VM. [#2338](https://github.com/deckhouse/virtualization/pull/2338)
 - **[core]** Added the `Uptime` column to `VirtualMachine` resources, showing the time since the VM started. [#2279](https://github.com/deckhouse/virtualization/pull/2279)
 - **[vm]** The VM status now includes a `No bootable device` message when the VM cannot find a bootable disk to start. [#2404](https://github.com/deckhouse/virtualization/pull/2404)
 - **[vm]** Add domain jobs and block-jobs info subcommands to vlctl. [#2280](https://github.com/deckhouse/virtualization/pull/2280)
 - **[vm]** Added the ability to change `coreFraction` on a running VM without a restart. The new value is applied via live migration. [#2210](https://github.com/deckhouse/virtualization/pull/2210)
 - **[vm]** Added the ability to attach additional network interfaces without a restart via the virtual machine's `.spec.networks`. [#2187](https://github.com/deckhouse/virtualization/pull/2187)
 - **[vm]** System virtual machine resources (pods with `d8v-hp-` and `d8v-vm-` prefixes) now run as the `deckhouse` user, without root privileges. [#2097](https://github.com/deckhouse/virtualization/pull/2097)
 - **[vm]** A restart is no longer required to attach and detach virtual disks and images via the virtual machine's `.spec.blockDeviceRefs`:
    - Works for new virtual machines starting from v1.9.0.
    - For previously created virtual machines, a restart is required to enable this behavior. [#2033](https://github.com/deckhouse/virtualization/pull/2033)

## Fixes


 - **[core]** Better handling Windows guests: start and migration should work in clusters with frequent CPU frequencies drifts [#2345](https://github.com/deckhouse/virtualization/pull/2345)
 - **[module]** Fixed an issue where invalid `virtualization` module ModuleConfig settings could block the Deckhouse queue. [#2246](https://github.com/deckhouse/virtualization/pull/2246)
 - **[module]** Fixed duplicate series on the `Virtualization / Overview` dashboard. [#2189](https://github.com/deckhouse/virtualization/pull/2189)
 - **[vd]** Time spent in the `WaitForFirstConsumer` phase is no longer included in `.status.stats.creationDuration.totalProvisioning` of virtual disks. [#2379](https://github.com/deckhouse/virtualization/pull/2379)
 - **[vd]** Allow ingress from virtualization namespace to importer pods [#2356](https://github.com/deckhouse/virtualization/pull/2356)
 - **[vm]** Fixed disk update for stopped VMs with WaitForFirstConsumer storage class. Previously, when a VM was stopped and disks were changed, the KVVM was not updated, causing the VM to get stuck in starting phase. [#2407](https://github.com/deckhouse/virtualization/pull/2407)
 - **[vm]** Fix VM migration failure caused by incorrect target disk size for filesystem-backed hotplug volumes. [#2402](https://github.com/deckhouse/virtualization/pull/2402)
 - **[vm]** Fix possible scheduling problems after changing vmclass from Discovery type to Model type. [#2352](https://github.com/deckhouse/virtualization/pull/2352)
 - **[vm]** Fixed an issue with VM migration cancellation that prevented new migrations from starting. [#2282](https://github.com/deckhouse/virtualization/pull/2282)

## Chore


 - **[api]** Removed the deprecated `VirtualMachineRestore` resource. Use `VirtualMachineOperation` with the `Clone` or `Restore` type, or `VirtualMachineSnapshotOperation` instead. [#2368](https://github.com/deckhouse/virtualization/pull/2368)

