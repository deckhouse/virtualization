# Changelog v0.23

## Features


 - **[core]** All containers have been switched to read-only mode, which is part of efforts to enhance security and ensure integrity control of the virtualization components. [#1244](https://github.com/deckhouse/virtualization/pull/1244)
 - **[vmrestore]** Added the ability to forcefully restore a virtual machine from a snapshot using the restoreMode parameter. This parameter has two possible values:
    - Safe - a safe recovery option when there are no conflicts with the virtual machine's resources;
    - Forced - a forced recovery option that can be applied to a running virtual machine but may lead to destructive consequences if conflicts arise during the recovery process.
    
    If the forcibly restored virtual disks are used by another virtual machine or the restored IP address is reserved by another virtual machine, the recovery process will fail, and this will be reported in the VirtualMachineRestore resource status. [#1115](https://github.com/deckhouse/virtualization/pull/1115)

## Fixes


 - **[api]** A virtual machine with the `AlwaysOn` run policy can be restored with the `forced` mode. [#1294](https://github.com/deckhouse/virtualization/pull/1294)
 - **[core]** Fixed the placement of the virtualization management component on system nodes when they are present in the cluster. If there are no system nodes in the cluster, it will be placed on master nodes. [#1260](https://github.com/deckhouse/virtualization/pull/1260)
 - **[vd]** For a virtual disk in `Filesystem` mode, fixed the ability to dynamically attach (hotplug) to a virtual machine. [#1241](https://github.com/deckhouse/virtualization/pull/1241)
 - **[vd]** Fixed the creation of virtual disks using NFS storage with the `no_root_squash` option. [#1210](https://github.com/deckhouse/virtualization/pull/1210)

