---
title: "Installation"
weight: 15
---

{{< alert level="warning" >}}
Module components must be deployed on physical servers (bare-metal).

Installation on virtual machines is allowed for demonstration purposes only, but nested virtualization must be enabled. If the module is deployed on virtual machines, technical support is not provided.
{{< /alert >}}

## Scaling options

The module supports the following configuration:

- Maximum number of nodes: `1000`.
- Maximum number of virtual machines: `50000`.

The module has no additional restrictions and is compatible with any hardware that is supported by operating systems on which it can be installed.

## Hardware and software requirements

Hardware requirements for the virtualization module match the requirements for [Deckhouse Kubernetes Platform](https://deckhouse.io/products/kubernetes-platform/gs/), with the additional requirement for CPU virtualization support on hosts where virtual machines will be launched.

### Additional requirements for virtualization support

On all cluster nodes where virtual machines are planned to be launched, hardware virtualization support must be ensured:

- Processor: support for Intel-VT (VMX) or AMD-V (SVM) instructions;
- BIOS/UEFI: hardware virtualization support enabled in BIOS/UEFI settings.

{{< alert level="warning" >}}
Ensuring stable operation of live migration mechanisms requires the use of an identical version of the Linux kernel on all cluster nodes.

This is because differences in kernel versions can lead to incompatible interfaces, system calls, and resource handling, which can disrupt the virtual machine migration process.
{{< /alert >}}

## Supported guest operating systems

The virtualization platform supports operating systems running on `x86` and `x86_64` architectures as guest operating systems. For correct operation in paravirtualization mode, `VirtIO` drivers must be installed to ensure efficient interaction between the virtual machine and the hypervisor.

Successful startup of the operating system is determined by the following criteria:

- Correct installation and booting of the OS.
- Uninterrupted operation of key components such as networking and storage.
- No crashes or errors during operation.

For Linux family operating systems, it is recommended to use guest OS images with `cloud-init` support, which allows initializing virtual machines after their creation.

For Windows family operating systems, the platform supports initialization with [autounattend](https://learn.microsoft.com/ru-ru/windows-hardware/manufacture/desktop/windows-setup-automation-overview) installation.

## Supported virtual machine configurations

- Maximum number of cores supported: `248`.
- Maximum amount of RAM: `1024 GB`.
- The maximum number of block devices to be attached: `16`.

## Supported storage systems

Virtual machines use `PersistentVolume` resources. To manage these resources and allocate disk space within the cluster, one or more supported storage systems must be installed:

| Storage System            | Disk Location             |
| ------------------------- | ------------------------- |
| sds-local-volume          | Local                     |
| sds-replicated-volume     | Replicas on cluster nodes |
| Ceph Cluster              | External storage          |
| NFS (Network File System) | External storage          |
| TATLIN.UNIFIED (Yadro)    | External storage          |
| Huawei Dorado             | External storage          |
| HPE 3par                  | External storage          |

## Installation

1. Deploy the Deckhouse Kubernetes Platform cluster following the [instructions](https://deckhouse.io/products/kubernetes-platform/gs/).

2. To store virtual machine data (virtual disks and images), enable one or multiple [supported storages](#supported-storage-systems).

3. Set the default `StorageClass`:

   ```shell
   # Specify the name of your StorageClass object.
   DEFAULT_STORAGE_CLASS=replicated-storage-class
   sudo -i d8 k patch mc global --type='json' -p='[{"op": "replace", "path": "/spec/settings/defaultClusterStorageClass", "value": "'"$DEFAULT_STORAGE_CLASS"'"}]'
   ```

4. Turn on the [`console`](https://deckhouse.io/modules/console/stable/) module, which will allow you to manage virtualization components through the Deckhouse web UI (available only for users of the Enterprise Edition).

5. Enable the `virtualization` module:

   {{< alert level="warning" >}}
   Enabling the `virtualization` module involves restarting kubelet/containerd and cilium agents on all nodes where virtual machines are supposed to start. This is necessary to configure the connectivity of containerd and DVCR.
   {{< /alert >}}

   To enable the `virtualization` module, create a `ModuleConfig` resource with the module settings.

   {{< alert level="warning" >}}
   Before enabling the module, carefully review its settings in the [Administrator guide](./admin_guide.html#module-parameters).
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

   To check if the module is ready, use the following command:

   ```bash
   d8 k get modules virtualization
   ```

   Example output:

   ```txt
   NAME             WEIGHT   SOURCE      PHASE   ENABLED   READY
   virtualization   900      deckhouse   Ready   True      True
   ```

   The module phase should be `Ready`.

## Component placement by nodes

The distribution of components across cluster nodes depends on the cluster's configuration. For example, a cluster may consist of:

- only master nodes, for running the control plane and workload components;
- only master nodes and worker nodes;
- master nodes, system nodes, and worker nodes;
- other combinations (depending on the architecture).

{{< alert level="warning" >}}
Worker nodes are understood as nodes that have no restrictions (taints) that prevent running regular workloads (pods, virtual machines).
{{< /alert >}}

The table lists the main virtualization management plane components and the nodes where they can be placed. Components are distributed by priority — if there is a suitable node type in the cluster, the component will be placed on it.

| Component Name                 | Node group for running components | Comment                                      |
| ----------------------------- | --------------------------------- | -------------------------------------------- |
| `cdi-operator-*`              | system/worker                     |                                              |
| `cdi-apiserver-*`             | master                            |                                              |
| `cdi-deployment-*`            | system/worker                     |                                              |
| `virt-api-*`                  | master                            |                                              |
| `virt-controller-*`           | system/worker                     |                                              |
| `virt-operator-*`             | system/worker                     |                                              |
| `virtualization-api-*`        | master                            |                                              |
| `virtualization-controller-*` | master                            |                                              |
| `virtualization-audit-*`      | system/worker                     |                                              |
| `dvcr-*`                      | system/worker                     | Storage must be available on the node        |
| `virt-handler-*`              | All cluster nodes                 |                                              |
| `vm-route-forge-*`            | All cluster nodes                 |                                              |

Components for creating and loading (importing) virtual machine images or disks (they run only during creation or loading):

| Component Name                  | Node group for running components | Comment                                      |
| ------------------------------ | --------------------------------- | -------------------------------------------- |
| `importer-*`                   | system/worker                     |                                              |
| `uploader-*`                   | system/worker                     |                                              |

## Module update

The `virtualization` module uses five update channels designed for use in different environments that have different requirements in terms of reliability:

| Update Channel | Description                                                                                                                                                                                                                                                        |
| -------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Alpha          | The least stable update channel with the most frequent appearance of new versions. It is oriented to development clusters with a small number of developers.                                                                                                       |
| Beta           | Focused on development clusters, like the Alpha update channel. Receives versions that have been pre-tested on the Alpha update channel.                                                                                                                           |
| Early Access   | Recommended update channel if you are unsure. Suitable for clusters where there is a lot of activity going on (new applications being launched, finalized, etc.). Functionality updates will not reach this update channel until one week after they are released. |
| Stable         | Stable update channel for clusters where active work is finished and mostly operational. Functionality updates to this update channel do not reach this update channel until two weeks after they appear in the release.                                           |
| Rock Solid     | The most stable update channel. Suitable for clusters that need a higher level of stability. Feature updates do not reach this channel until one month after they are released.                                                                                    |

The `virtualization` module components can be updated automatically or with manual confirmation, as updates are released in update channels.

{{< alert level="warning" >}}
When considering updates, the module components can be divided into two categories:

- Virtualization resource management components (control plane).
- Virtualization resource management components ("firmware").

Updating control plane components does not affect the operation of already running virtual machines, but may cause a brief interruption of established VNC/serial port connections while the control plane component is restarted.

Updates to virtual machine firmware during a platform upgrade may require virtual machines to be migrated to the new "firmware" version.
Migration during the upgrade is performed once, if the migration was unsuccessful, the virtual machine owner will need to perform it themselves by either evict the virtual machine or reboot it.
{{< /alert >}}

For information on versions available at the update channels, visit https://releases.deckhouse.io/.
