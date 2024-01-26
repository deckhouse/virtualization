---
title: "Configuration"
---

To configure the module, you must specify one or more desired subnets from which IP addresses for virtual machines will be allocated. For example, you can specify the following configuration in YAML format:

```yaml
virtualMachineCIDRs:
  - 10.10.10.0/24
  - 10.20.10.0/24
  - 10.30.10.0/24
  - 11.11.22.11/32
```

However, it is important to remember that the subnet for virtual machines should not be the same as the Pod subnet and Services subnet. Address conflicts can lead to unpredictable behavior and networking problems.

In addition, you will also need to set parameters for the image store. For example, you can use the following configuration for a storage of type PersistentVolumeClaim, with a size of 50G:

```yaml
settings:
  dvcr:
    storage:
      type: PersistentVolumeClaim
      persistentVolumeClaim:
        size: 50G
```

These settings will help you determine the available subnets for the virtual machines and configure the storage for the images accordingly.
