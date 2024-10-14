---
title: "Installation"
weight: 15
---

## DVP Requirements

### Resource requirements:

The following minimum resources are recommended for infrastructure nodes, depending on their role in the cluster:

- Master node-4 CPUs, 8 GB of RAM, 60 GB of disk space on a fast disk (400+ IOPS);
- Worker node-the requirements are similar to those for the master node, but largely depend on the nature of the load running on the node (nodes).

> If you plan to use the virtualization module in a production environment, it is recommended to deploy it on physical servers. Deploying the module on virtual machines is also possible, but in this case you need to enable nested virtualization.

### Requirements for platform nodes:

- Linux-based OS:
  - CentOS 7, 8, 9
  - Debian 10, 11, 12
  - Rocky Linux 8, 9
  - Ubuntu 18.04, 20.04, 22.04, 24.04
- Linux kernel version >= 5.7
- CPU with x86_64 c architecture with support for Intel-VT (vmx) or AMD-V (svm) instructions

## Installation

1. Deploy the Deckhouse Kubernetes Platform cluster by [instruction](https://deckhouse.io/products/kubernetes-platform/gs/).

2. Enable the necessary modules.

   To store virtual machine data (virtual disks and images), you must enable one or more of the following modules according to the installation instructions:

   - [SDS-Replicated-volume](https://deckhouse.io/modules/sds-replicated-volume/stable/)
   - [SDS-Local-volume](https://deckhouse.io/modules/sds-local-volume/stable/)
   - [CSI-nfs](https://deckhouse.io/modules/csi-nfs/stable/)
   - [CEPH-CSI](/documentation/v1/modules/031-ceph-csi/)

3. [Set](https://kubernetes.io/docs/tasks/administer-cluster/change-default-storage-class/) default `StorageClass`.
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

The `.spec.settings.dvcr` block describes the settings for the repository for storing virtual machine images, this block specifies the size of the storage provided for storing images `.spec.settings.dvcr.storage.persistentVolumeClaim.size`. The `.spec.settings.virtualMachineCIDRs` block specifies the list of subnets. Virtual machine addresses will be allocated automatically or on request from the specified subnet ranges in order.

You can track the readiness of the module using the following command:

```bash
d8 k get modules virtualization
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

Deckhouse Virtualization Platform components can be updated automatically, or with manual confirmation as updates are released in the update channels.

For information on the versions available on the update channels, visit this site at https://releases.deckhouse.io/.
