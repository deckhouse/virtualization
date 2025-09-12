# Changelog v1.0

## Features


 - **[api]** add metadata preservation for VirtualDisk [#1426](https://github.com/deckhouse/virtualization/pull/1426)
 - **[core]** Add VM restore feature using new type Restore for VirtualMachineOperation [#1307](https://github.com/deckhouse/virtualization/pull/1307)

## Fixes


 - **[api]** Fixed kubebuilder annotations to generate CRDs with correct categories and short names. [#1421](https://github.com/deckhouse/virtualization/pull/1421)
 - **[core]** fix CVE-2025-47907 [#1413](https://github.com/deckhouse/virtualization/pull/1413)
 - **[vd]** Set disk to failed when image pull fails from registry [#1400](https://github.com/deckhouse/virtualization/pull/1400)
 - **[vm]** fix `cores` and `coreFraction` validation in sizing policy [#1420](https://github.com/deckhouse/virtualization/pull/1420)
 - **[vm]** fix incorrect data encoding during snapshot creation and restoration by removing redundant base64 encoding when storing JSON in Kubernetes Secrets. [#1419](https://github.com/deckhouse/virtualization/pull/1419)
 - **[vm]** fix message in NetworkReady condition [#1414](https://github.com/deckhouse/virtualization/pull/1414)
 - **[vm]** Add display of `.status.network` if `.spec.network` is empty [#1412](https://github.com/deckhouse/virtualization/pull/1412)
 - **[vm]** Block network spec changes when SDN feature gate is disabled [#1408](https://github.com/deckhouse/virtualization/pull/1408)

## Chore


 - **[api]** Updated CRD short names to remove plural forms and reorganized resource categories. [#1407](https://github.com/deckhouse/virtualization/pull/1407)
 - **[core]** Reduce kubevirt components restarts. [#1449](https://github.com/deckhouse/virtualization/pull/1449)
 - **[module]** Reduce module restarts during installation. [#1445](https://github.com/deckhouse/virtualization/pull/1445)
 - **[module]** Support "in-cluster" upload when publicDomainTemplate is empty. [#1440](https://github.com/deckhouse/virtualization/pull/1440)
 - **[vm]** Check is first block device bootable. [#1359](https://github.com/deckhouse/virtualization/pull/1359)

