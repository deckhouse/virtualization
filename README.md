# Deckhouse virtualization module

<p align="center">
    <img alt="Virtualization" src="docs/images/DVP_light_mode.svg#gh-light-mode-only" alt="Virtualization" width="950" />
    <img alt="Virtualization" src="docs/images/DVP_dark_mode.svg#gh-dark-mode-only" alt="Virtualization" width="950" />
</p>

## Description

This module is designed to run and manage virtual machines and their resources on [the Deckhouse platform](https://deckhouse.io).

It offers the following features:

- A simple and intuitive interface for declarative creation and management of virtual machines and their resources.
- The ability to run legacy applications that for some reason cannot or are difficult to run in a container.
- Ability to run applications that require non-Linux operating systems.
- Ability to run virtual machines and containerized applications in the same environment.
- Integration with the existing Deckhouse ecosystem to leverage its capabilities for virtual machines.

### Resource requirements:

The following minimum resources are recommended for infrastructure nodes, depending on their role in the cluster:

- Master node-4 CPUs, 8 GB of RAM, 60 GB of disk space on a fast disk (400+ IOPS);
- Worker node-the requirements are similar to those for the master node, but largely depend on the nature of the load running on the node (nodes).

> If you plan to use the virtualization module in a production environment, it is recommended to deploy it on physical servers. Deploying the module on virtual machines is also possible, but in this case you need to enable nested virtualization.

### Requirements for platform nodes:

- [Supported Linux-based OS](https://deckhouse.io/products/kubernetes-platform/documentation/v1/supported_versions.html#linux)
- Linux kernel version >= 5.7
- CPU with x86_64 c architecture with support for Intel-VT (vmx) or AMD-V (svm) instructions

## What do I need to enable the module?

1. Deploy the Deckhouse Kubernetes Platform cluster by [instruction](https://deckhouse.io/products/kubernetes-platform/gs/).

2. Enable the necessary modules.

   To store virtual machine data (virtual disks and images), you must enable one or more of the following modules according to the installation instructions:

   - [SDS-Replicated-volume](https:/deckhouse.io/modules/sds-replicated-volume/stable/)
   - [SDS-Local-volume](https://deckhouse.io/modules/sds-local-volume/stable/)
   - [CSI-nfs](https://deckhouse.io/modules/csi-nfs/stable/)
   - [CSI-CEPH](https://deckhouse.io/modules/csi-ceph/stable/)
   ...

3. [Set](https://deckhouse.io/products/kubernetes-platform/documentation/v1/storage/admin/supported-storage.html#how-to-set-the-default-storageclass) default `StorageClass`.

4. Turn on the [console](https://deckhouse.ru/modules/console/stable/) module, which will allow you to manage virtualization components through via UI (This feature is available only to users of the EE edition).

5. Enable the `virtualization` module:

Attention! Enabling the `virtualization` module involves restarting kubelet/containerd on all nodes where virtual machines are supposed to start. This is necessary to configure the connectivity of containerd and DVCR.

Example of `ModuleConfig` to enable the virtualization module

```yaml
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
          size: 50G # size of DVCR storage
        type: PersistentVolumeClaim
    virtualMachineCIDRs:
      - 10.66.10.0/24
      - 10.66.20.0/24
      - 10.66.30.0/24
  version: 1
```

[More information](https://deckhouse.io/modules/virtualization/stable/)
