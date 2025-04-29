# Changelog v0.17

## Features


 - **[core]** Remove the host and host-passthrough virtual machine classes from the installation of virtualization. If they already exist in the cluster, they will be retained. [#926](https://github.com/deckhouse/virtualization/pull/926)
 - **[core]** Enhance security by restricting access to the virtqemud socket, allowing only the virt-launcher to connect. [#817](https://github.com/deckhouse/virtualization/pull/817)
 - **[core]** Introduce the vlctl tool as a replacement for virsh, compatible with the restricted libvirt socket. [#817](https://github.com/deckhouse/virtualization/pull/817)
 - **[core]** Enhance security by disabling unnecessary admin and read-only servers in libvirt's QEMU and logging services, reducing potential attack surfaces and preventing the creation of specific sockets. [#809](https://github.com/deckhouse/virtualization/pull/809)
 - **[core]** Enhance security by tracking and verifying synchronized checksums of virtual machine instances, ensuring that spec changes are consistent and reducing the risk of unauthorized alterations by an attacker. [#743](https://github.com/deckhouse/virtualization/pull/743)
 - **[observability]** Add a Grafana dashboard for monitoring virtual machine metrics. [#861](https://github.com/deckhouse/virtualization/pull/861)
 - **[observability]** Add a Prometheus metric indicating the readiness of the virtual machine agent. [#848](https://github.com/deckhouse/virtualization/pull/848)
 - **[vd]** Optimize the creation time for empty (blank) disks. [#786](https://github.com/deckhouse/virtualization/pull/786)
 - **[vd]** Improve the user experience for virtual disks by hiding irrelevant conditions. [#780](https://github.com/deckhouse/virtualization/pull/780)
 - **[vm]** Add new reasons for the `Completed` condition of `VirtualMachineOperation` to communicate the current progress and status of the requested virtual machine migration to the user. [#957](https://github.com/deckhouse/virtualization/pull/957)
 - **[vm]** Implement a controller to evacuate virtual machines whose pods have been requested for evacuation. It creates a `VirtualMachineOperation` to migrate the virtual machine. Information about the required evacuation will be displayed in the status of the virtual machine. [#919](https://github.com/deckhouse/virtualization/pull/919)
 - **[vm]** Introduce hypervisor versions in the status of virtual machines to provide detailed information about the versions of QEMU and libvirt used by the hypervisor. [#907](https://github.com/deckhouse/virtualization/pull/907)
 - **[vm]** Implement a controller to update the firmware version of virtual machines when the virtualization version is updated. This controller initiates a `VirtualMachineOperation` to migrate the virtual machine to the new firmware version. Information about the update process or any user-required actions will be reflected in the virtual machine's condition. [#881](https://github.com/deckhouse/virtualization/pull/881)
 - **[vm]** Implement the ability to cancel the migration of a virtual machine by deleting the corresponding `VirtualMachineOperation` resource. [#857](https://github.com/deckhouse/virtualization/pull/857)
 - **[vm]** Implement an automatic CPU topology configuration mechanism for the virtual machines. The number of cores/sockets depends on the number of cores in `.spec.cpu.cores`. For more details, refer to the documentation. [#747](https://github.com/deckhouse/virtualization/pull/747)
 - **[vm]** Add hot-plugged images to the status of the virtual machine. [#681](https://github.com/deckhouse/virtualization/pull/681)

## Fixes


 - **[api]** Fix the issue of block devices getting stuck in the Terminating phase. [#920](https://github.com/deckhouse/virtualization/pull/920)
 - **[api]** Fix network unavailability to dvcr inside a Project with network policy `Restricted` for block devices with data source type Upload. [#791](https://github.com/deckhouse/virtualization/pull/791)
 - **[core]** Resolve potential compatibility issues related to the truncation of scsi disk serial numbers in QEMU. [#842](https://github.com/deckhouse/virtualization/pull/842)
 - **[module]** Fix the Kubernetes version switch issue during updates from 1.29 to 1.30 in newer Deckhouse versions (1.69+). [#986](https://github.com/deckhouse/virtualization/pull/986)
 - **[vd]** Remove the phase 'Stopped' during startup when launching a virtual machine with the run policies AlwaysOn and AlwaysOnUnlessStopManually. Improve the message in the BlockDeviceReady condition for the virtual machine. [#782](https://github.com/deckhouse/virtualization/pull/782)
 - **[vm]** Resolve EFI bootloader issues with more than 8 cores. [#910](https://github.com/deckhouse/virtualization/pull/910)
 - **[vm]** Fix a bug with the early deletion of resource VirtualMachineBlockDeviceAttachment. Now it is deleted only after detachment is completed. [#841](https://github.com/deckhouse/virtualization/pull/841)
 - **[vm]** Redesign and improve BlockDeviceReady condition messages of virtual machine. [#800](https://github.com/deckhouse/virtualization/pull/800)
 - **[vm]** Rename FilesystemReady condition of virtual machine to FilesystemFrozen. [#714](https://github.com/deckhouse/virtualization/pull/714)
 - **[vm]** Add a new error message that appears when a virtual machine is unable to freeze its filesystem because the agent is not ready to perform this operation. [#713](https://github.com/deckhouse/virtualization/pull/713)

## Chore


 - **[core]** Change vm-router-forge image to distroless. [#790](https://github.com/deckhouse/virtualization/pull/790)
 - **[core]** Change virt-launcher image to distroless. [#773](https://github.com/deckhouse/virtualization/pull/773)
 - **[core]** Change distroless user to deckhouse. [#757](https://github.com/deckhouse/virtualization/pull/757)
 - **[core]** Change virt-handler image to distroless. [#748](https://github.com/deckhouse/virtualization/pull/748)
 - **[core]** Change virtualization-api and virtualization-controller images to distroless. [#745](https://github.com/deckhouse/virtualization/pull/745)
 - **[core]** Change virt-operator image to distroless. [#744](https://github.com/deckhouse/virtualization/pull/744)
 - **[core]** Change dvcr images to distroless. [#715](https://github.com/deckhouse/virtualization/pull/715)
 - **[docs]** Updated the documentation in accordance with version 0.17.0 updates of virtualization. [#897](https://github.com/deckhouse/virtualization/pull/897)
 - **[docs]** Add more cloud image sources to the admin guide in the documentation. [#813](https://github.com/deckhouse/virtualization/pull/813)
 - **[docs]** Add ansible provisioning guide to FAQ in the documentation. [#803](https://github.com/deckhouse/virtualization/pull/803)

