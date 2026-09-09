---
title: "Installation"
weight: 15
---

{{< alert level="warning" >}}
Module components must be deployed on physical servers (bare-metal).

Installation on virtual machines is allowed for demonstration purposes only, but nested virtualization must be enabled. If the module is deployed on virtual machines, technical support is not provided.
{{< /alert >}}

## Scaling limits

The module is designed for a cluster of this size:

- up to `1000` nodes;
- up to `50000` virtual machines.

The module has no additional restrictions and is compatible with any hardware supported by the operating systems on which it can be installed.

## Hardware and software requirements

Hardware requirements for the virtualization module match the requirements for the [Deckhouse Kubernetes Platform](/products/kubernetes-platform/guides/production.html#resource-requirements), with an additional requirement: CPU virtualization support on the hosts where virtual machines will be launched.

### Additional requirements for virtualization support

On all cluster nodes where virtual machines are planned to be launched, hardware virtualization support must be provided:

- CPU: Support for Intel-VT (VMX) or AMD-V (SVM) instructions.
- BIOS/UEFI: Hardware virtualization support enabled in the BIOS/UEFI settings.

{{< alert level="warning" >}}
Ensuring the stable operation of live migration mechanisms requires using the same Linux kernel version on all cluster nodes.

Differences between kernel versions can lead to incompatible interfaces, system calls, and resource handling, which can disrupt the virtual machine migration process.
{{< /alert >}}

It is recommended to use Linux kernels with up-to-date security updates provided by the maintainers of your chosen distribution. Such updates are not a prerequisite for virtualization to function, but they reduce security risks for the cluster as a whole.

{{< alert level="info" >}}
On Astra Linux nodes, **Astra Linux platform version 1.8.3 or higher** is required for virtualization to work correctly (earlier versions contain a bug that interferes with virtualization).
{{< /alert >}}

## Supported guest operating systems

The virtualization platform supports operating systems running on `x86` and `x86_64` architectures as guest operating systems. For correct operation in paravirtualization mode, `VirtIO` drivers must be installed to ensure efficient interaction between the virtual machine and the hypervisor.

Successful startup of the operating system is determined by the following criteria:

- Correct installation and booting of the OS.
- Uninterrupted operation of key components such as networking and storage.
- No crashes or errors during operation.

For Linux family operating systems, it is recommended to use guest OS images with `cloud-init` support, which allows initializing virtual machines after their creation.

For Windows family operating systems, the platform supports initialization with [autounattend](https://learn.microsoft.com/en-us/windows-hardware/manufacture/desktop/windows-setup-automation-overview) installation.

## Virtual machine configuration limits

- Maximum number of cores supported: `248`.
- Maximum amount of RAM: `1024 GB`.
- The maximum number of block devices to be attached: `16`.

## Supported storage systems

Virtual machine disks are created using PersistentVolume resources. To manage these resources and allocate disk space in the cluster, one or more supported storage systems must be deployed:

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

1. Deploy the Deckhouse Kubernetes Platform cluster following the [instructions](/products/kubernetes-platform/gs/).

1. To store virtual machine data (virtual disks and images), enable one or multiple [supported storages](#supported-storage-systems).

1. Set the default `StorageClass`:

   ```shell
   # Specify the name of your StorageClass object.
   DEFAULT_STORAGE_CLASS=replicated-storage-class
   sudo -i d8 k patch mc global --type='json' -p='[{"op": "replace", "path": "/spec/settings/defaultClusterStorageClass", "value": "'"$DEFAULT_STORAGE_CLASS"'"}]'
   ```

1. Turn on the [`console`](/modules/console/) module, which will allow you to manage virtualization components through the Deckhouse web UI (available only for users of the Enterprise Edition).

1. Enable the `virtualization` module:

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

## Component placement across nodes

The distribution of components across cluster nodes depends on the cluster's configuration. For example, a cluster may consist of:

- Only master nodes, for running the control plane and workload components.
- Only master nodes and worker nodes.
- Master nodes, system nodes, and worker nodes.
- Other combinations (depending on the architecture).

{{< alert level="warning" >}}
In this context, worker nodes are nodes that don't have taints preventing regular workloads from running.
{{< /alert >}}

What each component is responsible for is described in [Virtualization subsystem](/products/kubernetes-platform/documentation/v1/architecture/virtualization/). The table below lists the components of the virtualization control plane and the nodes where they can be placed. Components are scheduled by priority, and if a suitable node type is available in the cluster, the component lands on it.

| Component name                | Node group                                        | Comment                                                                                                        |
|-------------------------------|---------------------------------------------------|----------------------------------------------------------------------------------------------------------------|
| `virt-operator-*`             | system/master                                     |                                                                                                                |
| `virt-api-*`                  | master                                            |                                                                                                                |
| `virt-controller-*`           | system/worker                                     |                                                                                                                |
| `virt-handler-*`              | All cluster nodes                                 |                                                                                                                |
| `virtualization-api-*`        | master                                            |                                                                                                                |
| `virtualization-controller-*` | master                                            |                                                                                                                |
| `dvcr-*`                      | system                                            | Storage must be available on the node. If there are no system nodes, the component is placed on a worker node. |
| `virtualization-audit-*`      | master                                            | Available in the EE edition.                                                                                   |
| `virtualization-dra-*`        | Selected nodes                                    | Available in the EE edition.                                                                                   |
| `vm-route-forge-*`            | All cluster nodes                                 |                                                                                                                |

The `virtualization-dra-*` component runs only on nodes labeled `virtualization.deckhouse.io/usbip`.

Components used to create and import virtual machine images or disks (they run only for the duration of the creation or import operation):

| Component name                                   | Node group    | Comment                                                                            |
|--------------------------------------------------|---------------|--------------------------------------------------------------------------------------|
| `d8v-vi-importer-*`, `d8v-cvi-importer-*`        | system/worker | Pulls an image from an external source or another resource into the image storage. |
| `d8v-vi-uploader-*`, `d8v-cvi-uploader-*`        | system/worker | Receives a file that you upload from the command line or the web interface.        |
| `d8v-vd-pvc-importer-*`, `d8v-vi-pvc-importer-*` | system/worker | Moves an image from the image storage onto a disk volume.                          |
| `d8v-pvc-pvc-source-importer-*`                  | system/worker | Serves the data of the source volume over the network when a disk is cloned.       |
| `d8v-pvc-pvc-target-importer-*`                  | system/worker | Receives the data on the target volume when a disk is cloned.                       |
| `d8v-vi-bounder-*`                               | system/worker | Holds the volume on the right node while the image is being created on it.         |

### Cluster with taints on all nodes

In some clusters, taints are configured on every node. This lets administrators explicitly control which nodes pods and virtual machines can be scheduled onto.

When running the `virtualization` module in this setup, keep the following in mind:

1. When creating a [VirtualDisk](cr.html#virtualdisk), pay attention to the StorageClass `volumeBindingMode`. With `Immediate`, a PersistentVolume is created as soon as the disk is created — before the virtual machine is scheduled. Make sure the provisioner can create volumes on nodes that are allowed for virtual machines through [placement settings](./user_guide.html#placing-vms-on-nodes), including `nodeSelector`, `tolerations`, and settings in the virtual machine `spec` or [VirtualMachineClass](cr.html#virtualmachineclass). Otherwise, the disk may end up on a node where the virtual machine cannot run. With `WaitForFirstConsumer`, the volume is created on the node where the virtual machine is scheduled, and this issue does not occur.

1. [VirtualImage](cr.html#virtualimage) and [ClusterVirtualImage](cr.html#clustervirtualimage) require the temporary components from the table above. They have a toleration for the `dedicated.deckhouse.io=system` taint.

1. The cluster must have a `system` NodeGroup, or the administrator can add the `dedicated.deckhouse.io=system` taint to selected nodes without creating a NodeGroup. Without such nodes, these components will not be scheduled, and images will not reach the `Ready` phase.

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

- Virtualization resource management components (the control plane).
- Virtual machine launch components (the firmware).

Updating control plane components does not affect the operation of already running virtual machines, but may cause a brief interruption of established VNC/serial port connections while the control plane component is restarted.

Updates to virtual machine firmware during a platform upgrade may require virtual machines to be migrated to the new "firmware" version.
The module migrates a machine once, and if the migration fails, the machine owner has to move or reboot it themselves.
{{< /alert >}}

For information on versions available at the update channels, see the [release channels site](https://releases.deckhouse.io/).
