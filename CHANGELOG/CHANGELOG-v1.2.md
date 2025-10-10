# Changelog v1.2

## Features


 - **[core]** Promote vd/vi import error if url is broken. [#1534](https://github.com/deckhouse/virtualization/pull/1534)
 - **[observability]** Added Prometheus metrics for virtual machine and virtual disk snapshots (d8_virtualization_virtualmachinesnapshot_info and d8_virtualization_virtualdisksnapshot_info), allowing users to monitor the presence of snapshots through their observability dashboards. [#1555](https://github.com/deckhouse/virtualization/pull/1555)

## Fixes


 - **[core]** kill workers if main fuzz process don't kill they. [#1560](https://github.com/deckhouse/virtualization/pull/1560)
 - **[core]** fix path placement for filesystem pvc. [#1548](https://github.com/deckhouse/virtualization/pull/1548)
 - **[module]** fix CVE-2025-58058 and CVE-2025-54410 [#1556](https://github.com/deckhouse/virtualization/pull/1556)
 - **[module]** Current PR fix panics, which causes when during e2e tests one of pods of virtualization-controller is completed. [#1535](https://github.com/deckhouse/virtualization/pull/1535)
 - **[vm]** prohibit duplicating networks in the VirtualMachine `.spec` [#1545](https://github.com/deckhouse/virtualization/pull/1545)
 - **[vmip]** Added validation to ensure that an IP address is not already in use when updating the `VirtualMachineIP` address. [#1530](https://github.com/deckhouse/virtualization/pull/1530)
 - **[vmop]** This PR enhances the `VirtualMachineOperation` CRD by adding validation rules to ensure safe and valid resource naming during clone operations. [#1522](https://github.com/deckhouse/virtualization/pull/1522)

