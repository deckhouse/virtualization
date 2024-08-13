---
title: "Установка"
weight: 15
---

Для установки Deckhouse Virtualization Platform выполните следующие шаги:

1. Разверните кластер Deckhouse Kubernetes Platform по [инструкции](https://deckhouse.ru/gs/#%D0%B4%D1%80%D1%83%D0%B3%D0%B8%D0%B5-%D0%B2%D0%B0%D1%80%D0%B8%D0%B0%D0%BD%D1%82%D1%8B).

   Требования к процессору на узлах кластера, где планируется запускать виртуальные машины, включают:
   - архитектуру x86_64 и поддержку инструкций Intel-VT или AMD-V;
   - на узлах кластера поддерживается любая [совместимая](https://deckhouse.ru/documentation/v1/supported_versions.html#linux) ОС на базе Linux;
   - ядро Linux на узлах кластера должно быть версии 5.7 или более новой;
   - прочие требования к узлам кластера описаны в документе: [Подготовка к production](https://deckhouse.ru/guides/production.html).

1. Включите необходимые модули.

   Для хранения данных виртуальных машин необходимо включить один из следующих модулей согласно инструкции по их установке:
   - [SDS-Replicated-volume](https://deckhouse.ru/modules/sds-replicated-volume/stable/)
   - [CEPH-CSI](/documentation/v1/modules/031-ceph-csi/)

   Также возможно использовать другие варианты хранилищ, поддерживающие создание блочных устройств с режимом доступа `RWX` (`ReadWriteMany`).

1. Создайте манифест mc.yaml со следующим содержимым:

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
   Примените созданный манифест с использованием команды `d8 k apply -f mc.yaml`.
