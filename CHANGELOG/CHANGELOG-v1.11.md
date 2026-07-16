# Changelog v1.11

## Fixes


 - **[cli]** d8 v ssh and d8 v scp now connect to the cluster selected by global flags like --context and --kubeconfig. [#2678](https://github.com/deckhouse/virtualization/pull/2678)
 - **[cli]** collect-debug-info no longer fails to produce an archive when the VM is stopped or a referenced resource is missing. [#2677](https://github.com/deckhouse/virtualization/pull/2677)
 - **[module]** The module changelog is now shown in the Deckhouse console for each release. [#2674](https://github.com/deckhouse/virtualization/pull/2674)
 - **[vm]** A SecureBoot virtual machine now reports on the VirtualMachine why it cannot start (for example, a missing default StorageClass) instead of staying pending without explanation. [#2614](https://github.com/deckhouse/virtualization/pull/2614)

