# Changelog v1.3

## [MALFORMED]


 - #1715 missing section, missing summary, missing type, unknown section ""

## Features


 - **[core]** Add VirtualMachineSnapshotOperations. [#1728](https://github.com/deckhouse/virtualization/pull/1728)

## Fixes


 - **[vd]** VirtualDisk no longer stuck in WaitForFirstConsumer phase after VM attachment. [#1516](https://github.com/deckhouse/virtualization/pull/1516)
 - **[vm]** Fix migrating if node selector in vmclass was changed. [#1773](https://github.com/deckhouse/virtualization/pull/1773)
 - **[vmop]** Fix VirtualDisk annotations and labels restoring by VMOP Restore. [#1753](https://github.com/deckhouse/virtualization/pull/1753)

## Chore


 - **[api]** Force the failed state of a snapshot that cannot be taken right now. [#1744](https://github.com/deckhouse/virtualization/pull/1744)
 - **[images]** Increase memory limit value to workaround OOMKill during importing huge images ~2.9GiB on linux kernels 6.12+. [#1781](https://github.com/deckhouse/virtualization/pull/1781)
 - **[module]** Cancel dvcr garbage collection if wait for provisioners for more than 2 hours. [#1740](https://github.com/deckhouse/virtualization/pull/1740)

