---
title: "Настройки"
weight: 30
---

Пример конфигурации Deckhouse Virtualization Platform:

```yaml
apiVersion: deckhouse.io/v1alpha1
kind: ModuleConfig
metadata:
  name: virtualization
spec:
  # Включаем модуль.
  enabled: true
  version: 1
  settings:
    # Перечень подсетей для виртуальных машин.
    virtualMachineCIDRs:
      - 10.10.10.0/24
      - 10.20.10.0/24
      - 10.30.10.0/24
      - 11.11.22.33/32
    # Настройки параметров хранилища образов виртуальных машин.
    dvcr:
      storage:
        persistentVolumeClaim:
          size: 50G
        type: PersistentVolumeClaim
```
