# Changelog v0.17

## [MALFORMED]


 - #709 invalid type "feat", unknown section "e2e"
 - #713 invalid type "feat"
 - #750 invalid type "test"
 - #774 invalid type "feat"
 - #784 unknown section "doc"
 - #787 invalid type "test"
 - #792 invalid type "refactor"
 - #794 unknown section "test"

## Features


 - **[vm]** VirtualMachine.Status.BlockDeviceRefs contains hotplugged images [#681](https://github.com/deckhouse/virtualization/pull/681)

## Fixes


 - **[vd]** fix resizing handler and cover it with unit tests [#685](https://github.com/deckhouse/virtualization/pull/685)
 - **[vi]** bug fixes related to VirtualImage and VDSnapshot ObjectRef [#781](https://github.com/deckhouse/virtualization/pull/781)
 - **[vm]** rename filesystemReady condition to filesystemFrozen [#714](https://github.com/deckhouse/virtualization/pull/714)

## Chore


 - **[core]** change distroless user to deckhouse [#757](https://github.com/deckhouse/virtualization/pull/757)
 - **[core]** virt-handler image to distroless [#748](https://github.com/deckhouse/virtualization/pull/748)
 - **[core]** virtualization api,controller to distroless [#745](https://github.com/deckhouse/virtualization/pull/745)
 - **[core]** virt operator,exportproxy,exportserver to distroless [#744](https://github.com/deckhouse/virtualization/pull/744)
 - **[core]** apply new cilium network priority api [#733](https://github.com/deckhouse/virtualization/pull/733)
 - **[core]** change dvcr images to distroless [#715](https://github.com/deckhouse/virtualization/pull/715)
 - **[module]** moving tls certificates generations to go hooks sdk. [#701](https://github.com/deckhouse/virtualization/pull/701)

