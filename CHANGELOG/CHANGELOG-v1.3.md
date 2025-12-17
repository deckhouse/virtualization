# Changelog v1.3

## Features


 - **[vmclass]** Added the `.spec.sizingPolicies.defaultCoreFraction` field to the `VirtualMachineClass` resource, allowing you to set the default `coreFraction` for virtual machines that use this class. [#1783](https://github.com/deckhouse/virtualization/pull/1783)
 - **[vmop]** Add type for d8_virtualization_virtualmachineoperation_status_phase VMOP metric. [#1803](https://github.com/deckhouse/virtualization/pull/1803)

## Fixes


 - **[images]** Added the ability to use system nodes to create project and cluster images. [#1747](https://github.com/deckhouse/virtualization/pull/1747)
 - **[observability]** Fixed the display of virtual machine charts in clusters running in HA mode. [#1801](https://github.com/deckhouse/virtualization/pull/1801)
 - **[vd]** Fixed an issue with restoring labels and annotations on a disk created from a snapshot. [#1776](https://github.com/deckhouse/virtualization/pull/1776)
 - **[vd]** Accelerated disk attachment in `WaitForFirstConsumer` mode for virtual machines. [#1516](https://github.com/deckhouse/virtualization/pull/1516)

