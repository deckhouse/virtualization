# Changelog v0.21

## [MALFORMED]


 - #1133 invalid type "chored"
 - #1176 invalid type "docs"

## Features


 - **[core]** Automatic VM rebalancing based on CPU utilization (80% threshold) and affinity rules. Functionality becomes active only when descheduler module is enabled [#962](https://github.com/deckhouse/virtualization/pull/962)
 - **[images]** To the statuses of `VirtualImage` and `ClusterVirtualImage` resources, the condition `InUse` has been added, indicating whether the image is currently in use (for example, by a running virtual machine or for creating a virtual disk). [#859](https://github.com/deckhouse/virtualization/pull/859)
 - **[module]** Detect nodes with enabled /dev/kvm and CPU features VMX/SVM to schedule virt-handlers only on capable nodes. [#1076](https://github.com/deckhouse/virtualization/pull/1076)
 - **[vm]** support hotplug of PVCs with filesystem volume mode [#1060](https://github.com/deckhouse/virtualization/pull/1060)

## Fixes


 - **[core]** Remove init container with root privileges. [#1148](https://github.com/deckhouse/virtualization/pull/1148)
 - **[module]** Fix spec.strategy patching for Deployment/cdi-deployment in HA mode. [#1173](https://github.com/deckhouse/virtualization/pull/1173)

## Chore


 - **[disks]** Revert [#1172](https://github.com/deckhouse/virtualization/pull/1172)
 - **[disks]** Fix const for PVCLost phase, sync Go sources and CRDs (VD, CVI). [#1165](https://github.com/deckhouse/virtualization/pull/1165)
 - **[module]** bump golang version with base-images [#1174](https://github.com/deckhouse/virtualization/pull/1174)
 - **[vd]** Fix disk deletion error "invalid phase Lost" [#1169](https://github.com/deckhouse/virtualization/pull/1169)

