---
title: "Configuration"
---

Virtualization module configuration example:

```yaml
apiVersion: deckhouse.io/v1alpha1
kind: ModuleConfig
metadata:
  name: virtualization
spec:
  # Enable the module.
  enabled: true
  version: 1
  settings:
    # List of subnets for virtual machines.
    virtualMachineCIDRs:
      - 10.10.10.0/24
      - 10.20.10.0/24
      - 10.30.10.0/24
      - 11.11.22.33/32
    # Virtual machine image storage settings.
    dvcr:
      storage:
        persistentVolumeClaim:
          size: 50G
        type: PersistentVolumeClaim
```
