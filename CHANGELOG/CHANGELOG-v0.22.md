# Changelog v0.22

## Features


 - **[api]** The storage classes managed by the `local-path-provisioner` module are now deprecated for VirtualImage and VirtualDisk creation. [#1243](https://github.com/deckhouse/virtualization/pull/1243)

## Fixes


 - **[api]** The allowed name lengths for resources have been adjusted and the corresponding validation has been added:
    - ClusterVirtualImage: 48 characters (instead of 36)
    - VirtualImage: 49 characters (instead of 37) [#1229](https://github.com/deckhouse/virtualization/pull/1229)
 - **[module]** Now in clusters with High Availability mode, the virtualization components on the master nodes use 3 replicas. [#1208](https://github.com/deckhouse/virtualization/pull/1208)
 - **[module]** Fixed the deployment of the virtualization module in HTTP mode (when using `Disabled` or `OnlyInURI` options for the https.mode setting), which could lead to blocking the execution of the deckhouse queue. [#1207](https://github.com/deckhouse/virtualization/pull/1207)
 - **[module]** Fixed the deployment of the module on nodes with CentOS, Rocky Linux, and Alma Linux with SELinux enabled (Enforced). Now the installation completes without errors. [#1203](https://github.com/deckhouse/virtualization/pull/1203)
 - **[module]** Reduced the module size to 50MB (previously 445MB). [#1181](https://github.com/deckhouse/virtualization/pull/1181)
 - **[vm]** Removed unnecessary warnings about virtual machines running in privileged mode — such messages are no longer displayed, as this is standard and expected behavior of the system. [#1202](https://github.com/deckhouse/virtualization/pull/1202)
 - **[vmsnapshot]** Fixed the hotplugging of existing images when restoring a virtual machine from a snapshot. [#1198](https://github.com/deckhouse/virtualization/pull/1198)

