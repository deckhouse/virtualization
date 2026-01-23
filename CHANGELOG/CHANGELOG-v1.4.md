# Changelog v1.4

## Features


 - **[core]** Exclude hoplug pod from qouta calculations. [#1866](https://github.com/deckhouse/virtualization/pull/1866)
 - **[core]** Exclude from qouta system resources for CVI/VI/VD. [#1848](https://github.com/deckhouse/virtualization/pull/1848)
 - **[images]** Added monitoring of DVCR image presence for VirtualImage and ClusterVirtualImage [#1622](https://github.com/deckhouse/virtualization/pull/1622)
 - **[vd]** Enable storage class migration for hotplugged disks [#1785](https://github.com/deckhouse/virtualization/pull/1785)
 - **[vm]** Support network configuration without a 'Main' network type in spec.networks [#1818](https://github.com/deckhouse/virtualization/pull/1818)
 - **[vmop]** Add ability to create VMOP Clone with VM in Running state. [#1816](https://github.com/deckhouse/virtualization/pull/1816)
 - **[vmop]** Enable migration for VMs with hotplugged local disks [#1779](https://github.com/deckhouse/virtualization/pull/1779)

## Fixes


 - **[module]** Correct KubeVirt virtualization metric unit from milliseconds to seconds. [#1752](https://github.com/deckhouse/virtualization/pull/1752)
 - **[vm]** Prevent false RestartRequired during upgrade when firmware.uuid is not set on KVVM [#1875](https://github.com/deckhouse/virtualization/pull/1875)
 - **[vm]** Use hostNetwork for hotplug pods to avoid IP consumption [#1823](https://github.com/deckhouse/virtualization/pull/1823)
 - **[vmip]** Fixed attaching the VirtualMachineIPAddress resource to a virtual machine when the address was created in advance. [#1879](https://github.com/deckhouse/virtualization/pull/1879)
 - **[vmop]** Correctly set annotations on resources being deleted while restoring. [#1878](https://github.com/deckhouse/virtualization/pull/1878)

## Chore


 - **[core]** Add rewrite rule for "machine-type.node.kubevirt.io" labels on Nodes. [#1854](https://github.com/deckhouse/virtualization/pull/1854)

