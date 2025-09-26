# Changelog v1.1

## [MALFORMED]


 - #1471 missing section, missing summary, missing type, unknown section ""

## Features


 - **[core]** add virtual machine name and namespace flag completions to cli [#1404](https://github.com/deckhouse/virtualization/pull/1404)
 - **[module]** Adds the `D8VirtualizationDVCRInsufficientCapacityRisk` alert to monitor DVCR storage. Triggers when free space drops below 5GB or 20% of capacity. [#1461](https://github.com/deckhouse/virtualization/pull/1461)
 - **[module]** add alert KubeNodeAwaitingVirtualMachinesEvictionBeforeShutdown [#1268](https://github.com/deckhouse/virtualization/pull/1268)
 - **[observability]** Repeat "Network details" graphs for all networks specified in the VM spec. [#1475](https://github.com/deckhouse/virtualization/pull/1475)
 - **[vm]** add volume migration in EE [#1360](https://github.com/deckhouse/virtualization/pull/1360)
 - **[vmop]** add VM clone operation feature. [#1418](https://github.com/deckhouse/virtualization/pull/1418)

## Fixes


 - **[module]** fix install packages via dnf and yum [#1464](https://github.com/deckhouse/virtualization/pull/1464)
 - **[observability]** Combine legends and unify line colors for different migrations on live migration memory graph. [#1474](https://github.com/deckhouse/virtualization/pull/1474)
 - **[vd]** allow delete pending resource [#1448](https://github.com/deckhouse/virtualization/pull/1448)
 - **[vd]** Prevent empty strings in allowedStorageClassSelector configuration for VirtualDisks and VirtualImage [#1422](https://github.com/deckhouse/virtualization/pull/1422)
 - **[vi]** Prevent empty strings in allowedStorageClassSelector configuration for VirtualDisks and VirtualImage [#1422](https://github.com/deckhouse/virtualization/pull/1422)
 - **[vm]** fix creating snapshot from vm with awaiting restart changes [#1477](https://github.com/deckhouse/virtualization/pull/1477)
 - **[vmop]** fix the validation of VirtualDisk names during the cloning process of a VirtualMachine. [#1496](https://github.com/deckhouse/virtualization/pull/1496)
 - **[vmop]** Fix the problem where a disk that in the "Terminating" phase  was wrongly added to kvvm's volumes during a restore operation in Strict mode. [#1493](https://github.com/deckhouse/virtualization/pull/1493)

## Chore


 - **[core]** Enable build glib2 in closed environment. [#1478](https://github.com/deckhouse/virtualization/pull/1478)
 - **[module]** Support containerd integrity checks for containers with system images running in non-system namespaces. [#1432](https://github.com/deckhouse/virtualization/pull/1432)

