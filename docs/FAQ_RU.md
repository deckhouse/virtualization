---
title: "FAQ"
weight: 70
---

## Как установить ОС в виртуальной машине из ISO-образа?

Рассмотрим пример установки ОС из ISO-образа ОС Windows. Для этого загрузите и опубликуйте его на каком-либо HTTP-сервисе, доступном из кластера.

1. Создайте пустой диск для установки ОС:

    ```yaml
    apiVersion: virtualization.deckhouse.io/v1alpha2
    kind: VirtualDisk
    metadata:
      name: win-disk
      namespace: default
    spec:
      persistentVolumeClaim:
        size: 100Gi
        storageClassName: local-path
    ```

1. Создайте ресурсы с ISO-образами ОС Windows и драйверами virtio:

    ```yaml
    apiVersion: virtualization.deckhouse.io/v1alpha2
    kind: ClusterVirtualImage
    metadata:
      name: win-11-iso
    spec:
      dataSource:
        type: HTTP
        http:
          url: "http://example.com/win11.iso"
    ```

    ```yaml
    apiVersion: virtualization.deckhouse.io/v1alpha2
    kind: ClusterVirtualImage
    metadata:
      name: win-virtio-iso
    spec:
      dataSource:
        type: HTTP
        http:
          url: "https://fedorapeople.org/groups/virt/virtio-win/direct-downloads/stable-virtio/virtio-win.iso"
    ```

1. Создайте виртуальную машину:

    ```yaml
    apiVersion: virtualization.deckhouse.io/v1alpha2
    kind: VirtualMachine
    metadata:
      name: win-vm
      namespace: default
      labels:
        vm: win
    spec:
      virtualMachineClassName: generic
      runPolicy: Manual
      osType: Windows
      bootloader: EFI
      cpu:
        cores: 6
        coreFraction: 50%
      memory:
        size: 8Gi
      enableParavirtualization: true
      blockDeviceRefs:
        - kind: ClusterVirtualImage
          name: win-11-iso
        - kind: ClusterVirtualImage
          name: win-virtio-iso
        - kind: VirtualDisk
          name: win-disk
    ```

1. После создания ресурса виртуальная машина будет запущена. К ней необходимо подключиться, и с помощью графического установщика выполнить установку ОС и драйверов `virtio`.

    Команда для подключения:
    
    ```bash
    d8 v vnc -n default win-vm
    ```

1. После окончания установки завершите работу виртуальной машины.

1. Модифицируйте ресурс `VirtualMachine` и примените изменения:

    ```yaml
    spec:
      # ...
      runPolicy: AlwaysOn
      # ...
      blockDeviceRefs:
        # Удалить из блока все ресурсы ClusterVirtualImage с ISO-дисками.
        - kind: VirtualDisk
          name: win-disk
    ```

1. После внесенных изменений виртуальная машина запустится. Для продолжения работы с ней используйте команду:

   ```bash
   d8 v vnc -n default win-vm
   ```

## Как предоставить файл ответов Windows(Sysprep)

Чтобы предоставить виртуальной машине Windows файл ответов, необходимо указать provisioning с типом SysprepRef.

Прежде всего необходимо создать секрет:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: sysprep-config
data:
  unattend.xml: XXXx # base64 файла ответов
type: "provisioning.virtualization.deckhouse.io/sysprep"
```

Затем можно создать виртуальную машину, которая в процессе установке будет использовать файл ответов.
Внесите файл ответов (обычно именуются unattend.xml или autounattend.xml) в секрет, чтобы выполнять автоматическую установку Windows.
Вы также можете указать здесь другие файлы в формате base64 (customize.ps1, id_rsa.pub,...), необходимые для успешного выполнения скриптов внутри файла ответов.

```yaml
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualMachine
metadata:
  name: win-vm
  namespace: default
  labels:
    vm: win
spec:
  virtualMachineClassName: generic
  provisioning:
    type: SysprepRef
    sysprepRef:
      kind: Secret
      name: sysprep-config
  runPolicy: AlwaysOn
  osType: Windows
  bootloader: EFI
  cpu:
    cores: 6
    coreFraction: 50%
  memory:
    size: 8Gi
  enableParavirtualization: true
  blockDeviceRefs:
    - kind: ClusterVirtualImage
      name: win-11-iso
    - kind: ClusterVirtualImage
      name: win-virtio-iso
    - kind: VirtualDisk
      name: win-disk
```

## Как перенаправить трафик на виртуальную машину

Виртуальная машина функционирует в кластере Kubernetes, поэтому направление сетевого трафика осуществляется аналогично направлению трафика на поды:

1. Создайте сервис с требуемыми настройками.

   В качестве примера приведена виртуальная машина с HTTP-сервисом, опубликованным на порте 80, и следующим набором меток:

    ```yaml
    apiVersion: virtualization.deckhouse.io/v1alpha2
    kind: VirtualMachine
    metadata:
      name: web
      labels:
        vm: web
    spec: ...
    ```

1. Чтобы направить сетевой трафик на 80-й порт виртуальной машины, создайте сервис:

    ```yaml
    apiVersion: v1
    kind: Service
    metadata:
      name: svc-1
    spec:
      ports:
        - name: http
          port: 8080
          protocol: TCP
          targetPort: 80
      selector:
        app: old
    ```

   Можно изменять метки виртуальной машины без необходимости перезапуска, что позволяет настраивать перенаправление сетевого трафика между различными сервисами в реальном времени.
   Предположим, что был создан новый сервис и требуется перенаправить трафик на виртуальную машину от этого сервиса:

    ```yaml
    apiVersion: v1
    kind: Service
    metadata:
      name: svc-2
    spec:
      ports:
        - name: http
          port: 8080
          protocol: TCP
          targetPort: 80
      selector:
        app: new
    ```

   При изменении метки на виртуальной машине, трафик с сервиса `svc-2` будет перенаправлен на виртуальную машину:

    ```yaml
    metadata:
      labels:
        app: old
    ```

## Как увеличить размер DVCR

Чтобы увеличить размер диска для DVCR, необходимо установить больший размер в конфигурации модуля `virtualization`, чем текущий размер.

1. Проверьте текущий размер dvcr:

    ```shell
    d8 k get mc virtualization -o jsonpath='{.spec.settings.dvcr.storage.persistentVolumeClaim}'
    #Output
    {"size":"58G","storageClass":"linstor-thick-data-r1"}
    ```

1. Задайте размер:

    ```shell
    d8 k patch mc virtualization \
      --type merge -p '{"spec": {"settings": {"dvcr": {"storage": {"persistentVolumeClaim": {"size":"59G"}}}}}}'
    
    #Output
    moduleconfig.deckhouse.io/virtualization patched
    ```

1. Проверьте изменение размера:

    ```shell
    d8 k get mc virtualization -o jsonpath='{.spec.settings.dvcr.storage.persistentVolumeClaim}'
    #Output
    {"size":"59G","storageClass":"linstor-thick-data-r1"}
    
    d8 k get pvc dvcr -n d8-virtualization
    #Output
    NAME STATUS VOLUME                                    CAPACITY    ACCESS MODES   STORAGECLASS           AGE
    dvcr Bound  pvc-6a6cedb8-1292-4440-b789-5cc9d15bbc6b  57617188Ki  RWO            linstor-thick-data-r1  7d
    ```
