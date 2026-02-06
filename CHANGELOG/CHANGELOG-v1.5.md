# Changelog v1.5

## Features


 - **[core]** Add qouta exclude labels for virt-launcher pods. [#1872](https://github.com/deckhouse/virtualization/pull/1872)
 - **[module]** add VM operation timestamp metrics and operations table to dashboard [#1935](https://github.com/deckhouse/virtualization/pull/1935)
 - **[vd]** add VM name in wide output [#1889](https://github.com/deckhouse/virtualization/pull/1889)
 - **[vm]** A virtual machine can now be migrated to a particular node using the NodeSelector option in the virtual machine operation. [#1874](https://github.com/deckhouse/virtualization/pull/1874)

## Fixes


 - **[api]** rename conditions [#1948](https://github.com/deckhouse/virtualization/pull/1948)
 - **[core]** fix VMs with `EFIWithSecureBoot` bootloader failing to start when configured with more than 12 vCPUs. [#1916](https://github.com/deckhouse/virtualization/pull/1916)
 - **[module]** fix losing first 4 characters after VM reboot during console session [#1915](https://github.com/deckhouse/virtualization/pull/1915)
 - **[vm]** fix volume migration for disks in Filesystem mode [#1940](https://github.com/deckhouse/virtualization/pull/1940)
 - **[vm]** Prevent a false "Restart Required" condition when  restarting the VM. [#1910](https://github.com/deckhouse/virtualization/pull/1910)
 - **[vm]** Remove migrating condition for failed VM Migration. [#1891](https://github.com/deckhouse/virtualization/pull/1891)
 - **[vm]** Get shutdown reason from correct pod. [#1890](https://github.com/deckhouse/virtualization/pull/1890)
 - **[vm]** Correct message in AwaitingRestartToApplyConfiguration condition. [#1869](https://github.com/deckhouse/virtualization/pull/1869)
 - **[vmop]** Make Clone operation works for WaitForFirstConsumer disks. [#1926](https://github.com/deckhouse/virtualization/pull/1926)
 - **[vmop]** Correct message for 'Completed' condition for VirtualMachineOperation in phase 'Pending' [#1873](https://github.com/deckhouse/virtualization/pull/1873)
 - **[vmsnapshot]** Fix "unsupported empty phase" errors for VMSnapshot. [#1922](https://github.com/deckhouse/virtualization/pull/1922)

## Chore


 - **[api]** Add heritage=deckhouse label for Pods created in non-system namespaces [#1880](https://github.com/deckhouse/virtualization/pull/1880)
 - **[core]** Add missing fields to `VirtualMachine`'s OpenAPI spec. [#1951](https://github.com/deckhouse/virtualization/pull/1951)
 - **[module]** Add bounder image to svace report. [#1921](https://github.com/deckhouse/virtualization/pull/1921)
 - **[module]** Compile swtpm and libraries with x86-64-v2 compatibility. [#1857](https://github.com/deckhouse/virtualization/pull/1857)

