---
title: "Installation"
weight: 15
---

> **WARNING.** Module components must be deployed on physical servers (bare-metal).
>
> Installation on virtual machines is allowed for demonstration purposes only, but nested virtualization must be enabled. If the module is deployed on virtual machines, technical support is not provided.

## Scaling options

The platform supports the following configuration:

- Maximum number of nodes: `1000`.
- Maximum number of virtual machines: `50000`.

The module has no additional restrictions and is compatible with any hardware that is supported by [operating systems](#supported-os-for-platform-nodes) on which it can be installed.

## Hardware Requirements

1. A dedicated **machine for installation**.

   This machine will run the Deckhouse installer. For example, it can be an administrator's laptop or any other computer that is not intended to be added to the cluster. Requirements for this machine:

   - OS: Windows 10+, macOS 10.15+, Linux (Ubuntu 18.04+, Fedora 35+);
   - Installed Docker Engine or Docker Desktop (instructions for [Ubuntu](https://docs.docker.com/engine/install/ubuntu/), [macOS](https://docs.docker.com/desktop/mac/install/), [Windows](https://docs.docker.com/desktop/windows/install/));
   - HTTPS access to the container image registry at `registry.deckhouse.io`;
   - SSH key-based access to the node that will serve as the **master node** of the future cluster;
   - SSH key-based access to the node that will serve as the **worker node** of the future cluster (if the cluster will consist of more than one master node).

1. **Server for the master node**

   There can be multiple servers running the cluster’s control plane components, but only one server is required at installation time. The others can be added later via node management mechanisms.

   Requirements for a physical bare-metal server:

   - Resources:
     - CPU:
       - x86_64 architecture;
       - Support for Intel-VT (VMX) or AMD-V (SVM) instructions;
       - At least 4 cores.
     - RAM: At least 8 GB.
     - Disk space:
       - At least 60 GB;
       - High-speed disk (400+ IOPS).
   - OS [from the list of supported ones](#supported-os-for-platform-nodes):
     - Linux kernel version `5.7` or newer.
   - **Unique hostname** across all servers in the future cluster;
   - Network access:
     - HTTPS access to the container image registry at `registry.deckhouse.io`;
     - Access to the package repositories of the chosen OS;
     - SSH key-based access from the **installation machine** (see p.1);
     - Network access from the **installation machine** (see p.1) on port `22322/TCP`.
   - Required software:
     - The `cloud-utils` and `cloud-init` packages must be installed (package names may vary depending on the chosen OS).
   > **Warning.** The container runtime will be installed automatically, so do not pre-install any `containerd` or `docker` packages.

1. **Servers for worker nodes**

   These nodes will run virtual machines, so the servers must have enough resources to handle the planned number of VMs. Additional disks may be required if you deploy a software-defined storage solution.

   Requirements for a physical bare-metal server:

   - Resources:
     - CPU:
       - x86_64 architecture;
       - Support for Intel-VT (VMX) or AMD-V (SVM) instructions;
       - At least 4 cores;
     - RAM: At least 8 GB;
     - Disk space:
       - At least 60 GB;
       - High-speed disk (400+ IOPS);
       - Additional disks for software-defined storage;
   - OS [from the list of supported ones](#supported-os-for-platform-nodes);
     - Linux kernel version `5.7` or newer;
   - **Unique hostname** across all servers in the future cluster;
   - Network access:
     - HTTPS access to the container image registry at `registry.deckhouse.io`;
     - Access to the package repositories of the chosen OS;
     - SSH key-based access from the **installation machine** (see p.1);
   - Required software:
     - The `cloud-utils` and `cloud-init` packages must be installed (package names may vary depending on the chosen OS).
   > **Important.** The container runtime will be installed automatically, so do not pre-install any `containerd` or `docker` packages.

1. **Storage hardware**

   Depending on the chosen storage solution, additional resources may be required. For details, refer to the section [Storage Management](/products/virtualization-platform/documentation/admin/platform-management/storage/sds/lvm-local.html).

## Supported OS for platform nodes

| Linux distribution          | Supported versions              |
| --------------------------- | ------------------------------- |
| CentOS                      | 7, 8, 9                         |
| Debian                      | 10, 11, 12                      |
| Ubuntu                      | 20.04, 22.04, 24.04      |

{{< alert level=“warning”>}}
Ensuring stable operation of live migration mechanisms requires the use of an identical version of the Linux kernel on all cluster nodes.

This is because differences in kernel versions can lead to incompatible interfaces, system calls, and resource handling, which can disrupt the virtual machine migration process.
{{{< /alert >}}

## Supported guest operating systems

The virtualization platform supports operating systems running on `x86` and `x86_64` architectures as guest operating systems. For correct operation in paravirtualization mode, `VirtIO` drivers must be installed to ensure efficient interaction between the virtual machine and the hypervisor.

Successful startup of the operating system is determined by the following criteria:

  * correct installation and booting of the OS;
  * uninterrupted operation of key components such as networking and storage;
  * no crashes or errors during operation.

For Linux family operating systems it is recommended to use guest OS images with `cloud-init` support, which allows initializing virtual machines after their creation.

For Windows operating systems, the platform supports initialization using the built-in sysprep utility.

## Supported virtual machine configurations

Maximum number of cores supported: `254`
Maximum amount of RAM: `1024 GB`

## Supported Storage Systems

Virtual machines use `PersistentVolume` resources. To manage these resources and allocate disk space within the cluster, one or more supported storage systems must be installed:

| Storage System                              | Disk Location              |
|---------------------------------------------|----------------------------|
| sds-local-volume                            | Local                     |
| sds-replicated-volume                       | Replicas on cluster nodes |
| Ceph Cluster                                | External storage          |
| NFS (Network File System)                   | External storage          |
| TATLIN.UNIFIED (Yadro)                      | External storage          |

## Installation

1. Deploy the Deckhouse Kubernetes Platform cluster by [instruction](https://deckhouse.io/products/kubernetes-platform/gs/).

2. To store virtual machine data (virtual disks and images), you must enable one or more supported [storage](#supported-storage-systems).

<<<<<<< HEAD
3. Set default `StorageClass`.

   ```shell
   # Specify the name of your StorageClass object.
   DEFAULT_STORAGE_CLASS=replicated-storage-class
   sudo -i d8 k patch mc global --type='json' -p='[{"op": "replace", "path": "/spec/settings/defaultClusterStorageClass", "value": "'"$DEFAULT_STORAGE_CLASS"'"}]'
   ```
=======
3. In the `global` module, set `StorageClass` as the default.
>>>>>>> 771cd5e4 (docs(module): update install)

4. Turn on the [console](https://deckhouse.io/modules/console/stable/) module, which will allow you to manage virtualization components through via UI (This feature is available only to users of the EE edition).

5. Enable the `virtualization` module:

{{< alert level="warning" >}}
Attention! Enabling the `virtualization` module involves restarting kubelet/containerd on all nodes where virtual machines are supposed to start. This is necessary to configure the connectivity of containerd and DVCR.
{{< /alert >}}

To enable the `virtualization` module, you need to create a `ModuleConfig` resource containing the module settings.

{{< alert level="info" >}}
For a complete list of configuration options, see ["Settings"](./configuration.html)
{{< /alert >}}

Example of module configuration:

```yaml
d8 k apply -f - <<EOF
apiVersion: deckhouse.io/v1alpha1
kind: ModuleConfig
metadata:
  name: virtualization
spec:
  enabled: true
  settings:
    dvcr:
      storage:
        persistentVolumeClaim:
          size: 50G
        type: PersistentVolumeClaim
    virtualMachineCIDRs:
      - 10.66.10.0/24
  version: 1
EOF
```

The `.spec.settings.dvcr` block describes the settings for the repository for storing virtual machine images, it specifies the size of the storage provided for storing images `.spec.settings.dvcr.storage.persistentVolumeClaim.size` and the storage class `.spec.settings.dvcr.storage.persistentVolumeClaim.storageClassName`.

The `.spec.settings.virtualMachineCIDRs` block specifies the list of subnets. Virtual machine addresses will be allocated automatically or on request from the specified subnet ranges in order.

You can track the readiness of the module using the following command:

```bash
d8 k get modules virtualization
```

Example output:

```txt
# NAME             WEIGHT   STATE     SOURCE     STAGE   STATUS
# virtualization   900      Enabled   Embedded           Ready
```

The module status should be `Ready`.

## Platform Update

Deckhouse Virtualization Platform uses five update channels designed for use in different environments that have different requirements in terms of reliability:

| Update Channel | Description                                                                                                                                                                                                                                                        |
| -------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Alpha          | The least stable update channel with the most frequent appearance of new versions. It is oriented to development clusters with a small number of developers.                                                                                                       |
| Beta           | Focused on development clusters, like the Alpha update channel. Receives versions that have been pre-tested on the Alpha update channel.                                                                                                                           |
| Early Access   | Recommended update channel if you are unsure. Suitable for clusters where there is a lot of activity going on (new applications being launched, finalized, etc.). Functionality updates will not reach this update channel until one week after they are released. |
| Stable         | Stable update channel for clusters where active work is finished and mostly operational. Functionality updates to this update channel do not reach this update channel until two weeks after they appear in the release.                                           |
| Rock Solid     | The most stable update channel. Suitable for clusters that need a higher level of stability. Feature updates do not reach this channel until one month after they are released.                                                                                    |

{{< alert level="warning" >}}
In platform upgrades, the components can be divided into two categories:

- Virtualization resource management components (control plane)
- Virtualization resource management components ("firmware").

Updating the control plane components does not affect the operation of virtual machines that are already running. However, changes to the "firmware" during a platform upgrade may require virtual machines to be migrated to the new "firmware" version.
{{< /alert >}}

Deckhouse Virtualization Platform components can be updated automatically, or with manual confirmation as updates are released in the update channels.

For information on the versions available on the update channels, visit the site at https://releases.deckhouse.io/.
