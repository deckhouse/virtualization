# Changelog v1.5

## Features


 - **[core]** Add qouta exclude labels for virt-launcher pods. [#1872](https://github.com/deckhouse/virtualization/pull/1872)

## Fixes


 - **[vm]** Prevent a false "Restart Required" condition when  restarting the VM. [#1910](https://github.com/deckhouse/virtualization/pull/1910)
 - **[vm]** Get shutdown reason from correct pod. [#1890](https://github.com/deckhouse/virtualization/pull/1890)
 - **[vm]** Correct message in AwaitingRestartToApplyConfiguration condition. [#1869](https://github.com/deckhouse/virtualization/pull/1869)
 - **[vmop]** Correct message for 'Completed' condition for VirtualMachineOperation in phase 'Pending' [#1873](https://github.com/deckhouse/virtualization/pull/1873)
 - **[vmsnapshot]** Fix "unsupported empty phase" errors for VMSnapshot. [#1922](https://github.com/deckhouse/virtualization/pull/1922)

## Chore


 - **[api]** Add heritage=deckhouse label for Pods created in non-system namespaces [#1880](https://github.com/deckhouse/virtualization/pull/1880)
 - **[module]** Add bounder image to svace report. [#1921](https://github.com/deckhouse/virtualization/pull/1921)
 - **[module]** Compile swtpm and libraries with x86-64-v2 compatibility. [#1857](https://github.com/deckhouse/virtualization/pull/1857)

