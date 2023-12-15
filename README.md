# Deckhouse virtualization module

<p align="center">
  <img src="docs/images/d8-virtualization-logo.png" width="400px" />
</p>

## Description

This module is designed to run and manage virtual machines and their resources on [the Deckhouse platform](https://deckhouse.io).

It offers the following features:

- A simple and intuitive interface for declarative creation and management of virtual machines and their resources.
- The ability to run legacy applications that for some reason cannot or are difficult to run in a container.
- Ability to run virtual machines and containerized applications in the same environment.
- Integration with the existing Deckhouse ecosystem to leverage its capabilities for virtual machines.

## Requirements

The following conditions are required to run the module:

- A processor with x86_64 architecture and support for Intel-VT or AMD-V instructions.
- The Linux kernel on the cluster nodes must be version 5.7 or newer.
- The [CNI Cilium](https://deckhouse.ru/documentation/v1/modules/021-cni-cilium/) module to provide network connectivity for virtual machines.
- Modules SDS-DRBD or [Ceph](https://deckhouse.ru/documentation/v1/modules/031-ceph-csi/) for storing virtual machine data. It is also possible to use other storage options that support the creation of block devices with `RWX` (`ReadWriteMany`) access mode.

To connect to a virtual machine via Serial Console or VNC protocol, install the virtctl client.

## Architecture

The module includes the following components:

- The module core, based on the KubeVirt project and uses QEMU/KVM + libvirtd to run virtual machines.
- Deckhouse Virtualization Container Registry (DVCR) - repository for storing and caching virtual machine images.
- Virtualization-controller - API for creating and managing virtual machine resources.

The API provides capabilities for creating and managing the following resources:

- Images
- Virtual machine disks
- Virtual machines

## How to enable module

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
    vmCIDRs:
    - 10.66.10.0/24
    - 10.66.20.0/24
    - 10.66.30.0/24
  version: 1
```
