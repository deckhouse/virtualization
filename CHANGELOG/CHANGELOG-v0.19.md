# Changelog v0.19

## Features


 - **[vd]** The `vd-protection` finalizer will be removed from the virtual disk if the virtual machine is stopped. [#1014](https://github.com/deckhouse/virtualization/pull/1014)
 - **[vm]** Add Tini as the init process in virt-launcher container to properly handle orphaned/zombie processes and improve container stability. [#1058](https://github.com/deckhouse/virtualization/pull/1058)

## Fixes


 - **[vm]** Set max embedded cloud-init block size to 2048 bytes [#1083](https://github.com/deckhouse/virtualization/pull/1083)
 - **[vm]** Improve Status Reporting [#1023](https://github.com/deckhouse/virtualization/pull/1023)
 - **[vmip]** Fixed issues with hanging and duplicating VirtualMachineIPAddressLease resources, as well as issues with attaching VirtualMachineIPAddress to a virtual machine. [#1081](https://github.com/deckhouse/virtualization/pull/1081)
 - **[vmip]** Fix creating many vmipleases from one vmip. [#1012](https://github.com/deckhouse/virtualization/pull/1012)
 - **[vmsnapshot]** The `VirtualMachineSnapshot` controller properly handles the agent status of the `VirtualMachine`. [#1065](https://github.com/deckhouse/virtualization/pull/1065)

## Chore


 - **[core]** Move virtualization subcommand to virtualization repository from deckhouse/deckhouse-cli. [#1045](https://github.com/deckhouse/virtualization/pull/1045)
 - **[core]** remove unnecessary binaries [#1042](https://github.com/deckhouse/virtualization/pull/1042)
 - **[core]** move to 3p-kubevirt [#956](https://github.com/deckhouse/virtualization/pull/956)
 - **[module]** update deckhouse base-images to 0.5.2 [#1044](https://github.com/deckhouse/virtualization/pull/1044)
 - **[module]** Update golang image, and go modules to mitigate CVEs. [#1039](https://github.com/deckhouse/virtualization/pull/1039)
 - **[vm]** Add node shutdown inhibit label to VM Pod. [#1061](https://github.com/deckhouse/virtualization/pull/1061)

