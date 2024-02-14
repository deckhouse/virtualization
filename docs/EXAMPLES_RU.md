---
title: "Примеры конфигурации"
---

## Быстрый старт

Пример создания виртуальной машины с Ubuntu 22.04.

1. Создайте namespace для виртуальных машин с помощью команды:

```bash
kubectl create ns vms
```

2. Создайте диск виртуальной машины из внешнего источника:

```yaml
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualMachineDisk
metadata:
  name: linux-disk
  namespace: vms
spec:
  persistentVolumeClaim:
    size: 10Gi
    storageClassName: local-path
  dataSource:
    type: HTTP
    http:
      url: "https://cloud-images.ubuntu.com/minimal/releases/jammy/release-20230615/ubuntu-22.04-minimal-cloudimg-amd64.img"
```

После создания `VirtualMachineDiks` в namespace vms, запустится под с именем `importer-*`, который осуществит загрузку заданного образа.

3. Посмотрите текущий статус ресурса с помощью комнады:

```bash
kubectl -n vms get virtualmachinedisk -o wide

# NAME            PHASE   CAPACITY    PROGRESS   TARGET PVC                                               AGE
# linux-disk      Ready   10Gi        100%       vmd-vmd-blank-001-10c7616b-ba9c-4531-9874-ebcb3a2d83ad   1m
```

4. Создайте виртуальную машину из следующей спецификации:

```yaml
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualMachine
metadata:
  name: linux-vm
  namespace: vms
  labels:
    vm: linux
spec:
  runPolicy: AlwaysOn # Виртуальная машина должна быть всегда включена.
  enableParavirtualization: true # Использовать паравиртуализацию (virtio).
  osType: Generic
  bootloader: BIOS
  cpu:
    cores: 1
    coreFraction: 10% # Запросить 10% процессорного времени одного ядра.
  memory:
    size: 1Gi
  provisioning: # Пример cloud-init-сценария для создания пользователя cloud с паролем cloud.
    type: UserData
    userData: |
      #cloud-config
      users:
      - name: cloud
        passwd: $6$rounds=4096$vln/.aPHBOI7BMYR$bBMkqQvuGs5Gyd/1H5DP4m9HjQSy.kgrxpaGEHwkX7KEFV8BS.HZWPitAtZ2Vd8ZqIZRqmlykRCagTgPejt1i.
        shell: /bin/bash
        sudo: ALL=(ALL) NOPASSWD:ALL
        chpasswd: { expire: False }
        lock_passwd: false
        ssh_authorized_keys:
          - ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDTXjTmx3hq2EPDQHWSJN7By1VNFZ8colI5tEeZDBVYAe9Oxq4FZsKCb1aGIskDaiAHTxrbd2efoJTcPQLBSBM79dcELtqfKj9dtjy4S1W0mydvWb2oWLnvOaZX/H6pqjz8jrJAKXwXj2pWCOzXerwk9oSI4fCE7VbqsfT4bBfv27FN4/Vqa6iWiCc71oJopL9DldtuIYDVUgOZOa+t2J4hPCCSqEJK/r+ToHQbOWxbC5/OAufXDw2W1vkVeaZUur5xwwAxIb3wM3WoS3BbwNlDYg9UB2D8+EZgNz1CCCpSy1ELIn7q8RnrTp0+H8V9LoWHSgh3VCWeW8C/MnTW90IR
  blockDevices:
    - type: VirtualMachineDisk
      virtualMachineDisk:
        name: linux-disk
```

5. Проверьте с помощью команды, что виртуальная машина создана и запущена:

```bash
kubectl -n default get virtualmachine

# NAME       PHASE     NODENAME   IPADDRESS    AGE
# linux-vm   Running   virtlab-1  10.66.10.1   5m
```

6. Подключитесь с помощью консоли к виртуальной машине (для выхода из консоли необходимо нажать `Ctrl+]`):

```bash
dvp console -n vms linux-vm
```

7. Подключитесь к машине с использованием VNC:

```bash
dvp vnc -n vms linux-vm
```

После выполнения команды запустится VNC-клиент, используемый в системе по умолчанию. Альтернативный способ подключения — с помощью параметра `--proxy-only` пробросить VNC-порт на локальную машину.

## Образы

`VirtualMachineImage` и `ClusterVirtualMachineImage` используются для хранения образов виртуальных машин.
Образы могут быть следующих видов:
- Образ диска виртуальной машины, который предназначен для тиражирования идентичных дисков виртуальных машин.
- ISO-образ, содержащий файлы для установки ОС. Этот тип образа подключается к виртуальной машине как cdrom.

Ресурс `VirtualMachineImage` доступен только в том пространстве имен, в котором был создан, а `ClusterVirtualMachineImage` доступен для всех пространств имен внутри кластера.

В зависимости от конфигурации, ресурс `VirtualMachineImage` может хранить данные в `DVCR` или использовать дисковое хранилище, предоставляемое платформой (PV). `ClusterVirtualMachineImage` хранит данные только в `DVCR`, обеспечивая единый доступ ко всем образам для всех пространств имен в кластере.

### Создание и использование образа c HTTP-ресурса

1. Создадайте `VirtualMachineImage` и в качестве хранилища образов используйте `DVCR`:

```yaml
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualMachineImage
metadata:
  name: ubuntu-img
  namespace: vms
spec:
  storage: ContainerRegistry
  dataSource:
    type: HTTP
    http:
      url: "https://cloud-images.ubuntu.com/minimal/releases/jammy/release-20230615/ubuntu-22.04-minimal-cloudimg-amd64.img"
```

2. Проверьте результат с помощью команды:

```bash
kubectl -n vms get virtualmachineimage

# NAME         PHASE   CDROM   PROGRESS   AGE
# ubuntu-img   Ready   false   100%       10m
```

Для хранения образа в дисковом хранилище, предоставляемом платформой, настройки `storage` будут выглядеть следующим образом:

```yaml
spec:
  storage: Kubernetes
  persistentVolumeClaim:
    storageClassName: "your-storage-class-name"
```

где `your-storage-class-name` — это название StorageClass, который будет использоваться.

3. Для просмотра списка доступных StorageClass'ов выполните следующую команду:

```bash
kubectl get storageclass

# Пример вывода команды:
# NAME                          PROVISIONER              RECLAIMPOLICY   VOLUMEBINDINGMODE      ALLOWVOLUMEEXPANSION   AGE
# linstor-thin-r1               linstor.csi.linbit.com   Delete          WaitForFirstConsumer   true                   20d
# linstor-thin-r2               linstor.csi.linbit.com   Delete          WaitForFirstConsumer   true                   20d
# linstor-thin-r3               linstor.csi.linbit.com   Delete          WaitForFirstConsumer   true                   20d
```

Ресурс `ClusterVirtualMachineImage` создается по аналогии, но не требует указания настроек `storage`:

```yaml
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: ClusterVirtualMachineImage
metadata:
  name: ubuntu-img
spec:
  dataSource:
    type: HTTP
    http:
      url: "https://cloud-images.ubuntu.com/minimal/releases/jammy/release-20230615/ubuntu-22.04-minimal-cloudimg-amd64.img"
```

4. Проверьте статус `ClusterVirtualMachineImage` с помощью команды:

```bash
kubectl get clustervirtualmachineimage

# NAME         PHASE   CDROM   PROGRESS   AGE
# ubuntu-img   Ready   false   100%       11m
```

Образы могут быть получены из различных источников, таких как HTTP-серверы, на которых расположены файлы образов, или контейнерные реестры (container registries), где образы сохраняются и становятся доступны для скачивания. Также существует возможность загрузить образы напрямую из командной строки, используя утилиту `curl`.

### Создание и использование образа из container registry

1. Cформируйте образ для хранения в `container registry`.

Ниже представлен пример создания docker-образа c диском Ubuntu 22.04.

* Загрузите образ локально:

```bash
curl -L https://cloud-images.ubuntu.com/minimal/releases/jammy/release-20230615/ubuntu-22.04-minimal-cloudimg-amd64.img -o ubuntu2204.img
```

* Создайте Dockerfile со следующим содержимым:

```Dockerfile
FROM scratch
COPY ubuntu2204.img /disk/ubuntu2204.img
```

* Соберите образ и загрузите его в `container registry`. В качестве `container registry` в примере ниже использован docker.io. для выполнения вам необходимо иметь учетную запись сервиса и настроенное окружение.

```bash
docker build -t docker.io/username/ubuntu2204:latest
```

где `username` — имя пользователя, указанное при регистрации в docker.io.

* Загрузите созданный образ в `container registry` с помощью команды:

```bash
docker push docker.io/username/ubuntu2204:latest
```

* Чтобы использовать этот образ, создайте в качестве примера ресурс `ClusterVirtualMachineImage`:

```yaml
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: ClusterVirtualMachineImage
metadata:
  name: ubuntu-2204
spec:
  dataSource:
    type: ContainerImage
    containerImage:
      image: docker.io/username/ubuntu2204:latest
```

* Чтобы посмотреть ресурс и его статус, выполните команду:

```bash
kubectl get clustervirtalmachineimage
```

### Загрузка образа из командной строки

1. Чтобы загрузить образ из командной строки, предварительно создайте следующий ресурс, как представлено ниже на примере `ClusterVirtualMachineImage`:

```yaml
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: ClusterVirtualMachineImage
metadata:
  name: some-image
spec:
  dataSource:
    type: Upload
```

2. После того как ресурс будет создан, проверьте его статус с помощью команды:

```bash
kubectl get clustervirtualmachineimages some-image -o json | jq .status.uploadCommand -r

> uploadCommand: curl https://virtualization.example.com/upload/dSJSQW0fSOerjH5ziJo4PEWbnZ4q6ffc
    -T example.iso
```

> CVMI с типом **Upload** ожидает начала загрузки образа 15 минут после создания. По истечении этого срока ресурс перейдет в состояние **Failed**.

3. Загрузите образ Cirros (представлено в качестве примера):

```bash
curl -L http://download.cirros-cloud.net/0.5.1/cirros-0.5.1-x86_64-disk.img -o cirros.img
https://virtualization.example.com/upload/dSJSQW0fSOerjH5ziJo4PEWbnZ4q6ffc -T cirros.img
```

После завершения работы команды `curl` образ должен быть создан.

4. Проверьте, что статус созданного образа `Ready`:

```bash
kubectl get clustervirtualmachineimages

# NAME         PHASE   CDROM   PROGRESS   AGE
# some-image   Ready   false   100%       10m
```

## Диски

Диски используются в виртуальных машинах для записи и хранения данных. Для хранения дисков используется хранилище, предоставляемое платформой.

1. Чтобы посмотреть доступные варианты, выполните команду:

```bash
kubectl get storageclass
```

### Создание пустого диска

> Существует возможность создания пустых дисков.

1. Создайте диск:

```yaml
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualMachineDisk
metadata:
  name: vmd-blank
spec:
  persistentVolumeClaim:
    storageClassName: "your-storage-class-name"
    size: 100M
```

Созданный диск можно использовать для подключения к виртуальной машине.

2. Проверьте состояние созданного ресурса с помощью команды:

```bash
kubectl get virtualmachinedisk

# NAME        PHASE  CAPACITY   AGE
# vmd-blank   Ready  100Mi      1m
```

### Создание диска из образа

> Можно создать диски из существующих дисковых образов, а также из внешних ресурсов, таких как образы.

При создании ресурса диска можно указать желаемый размер. Если размер не указан, то будет создан диск с размером, соответствующим исходному образу диска, который хранится в ресурсе `VirtualMachineImage` или `ClusterVirtualMachineImage`. Если необходимо создать диск большего размера, укажите необходимый размер.

В качестве примера рассмотрен ранее созданный `ClusterVirtualMachineImage` с именем `ubuntu-2204`:

```yaml
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualMachineDisk
metadata:
  name: ubuntu-root
spec:
  persistentVolumeClaim:
    size: 10Gi
    storageClassName: "your-storage-class-name"
  dataSource:
    type: ClusterVirtualMachineImage
    clusterVirtualMachineImage:
      name: ubuntu-img
```

### Изменение размера диска

Размер дисков можно изменить только в сторону увеличения, даже если они подключены к виртуальной машине. Для этого отредактируйте поле `spec.persistentVolumeClame.size`:

```yaml
kubectl patch ubuntu-root --type merge -p '{"spec":{"persistentVolumeClaim":{"size":"11Gi"}}}'
```

### Подключение дисков к запущенным виртуальным машинам

Диски могут быть подключены в работающей виртуальной машине с использованием `VirtualMachineBlockDeviceAttachment` ресурса:

```yaml
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualMachineBlockDeviceAttachment
metadata:
  name: vmd-blank-attachment
spec:
  virtualMachineName: linux-vm # Имя виртуальной машины, к которой будет подключен диск.
  blockDevice:
    type: VirtualMachineDisk
    virtualMachineDisk:
      name: vmd-blank # Имя подключаемого диска.
```

Если в указанном ресурсе изменить имя виртуальной машины на имя другой виртуальной машины, диск будет перенаправлен от одной виртуальной машины к другой.
При удалении ресурса `VirtualMachineBlockDeviceAttachment` диск от виртуальной машины будет отключен.

Чтобы посмотреть список подключенных дисков в работающей виртуальной машине, выполните команду:

```bash
kubectl get virtualmachineblockdeviceattachments
```

## Виртуальные машины

Для создания виртуальной машины используется ресурс `VirtualMachine`, его параметры позволяют сконфигурировать:

- ресурсы, требуемые для работы виртуальной машины (процессор, память, диски и образы);
- правила размещения виртуальной машины на узлах кластера;
- настройки загрузчика и оптимальные параметры для гостевой ОС;
- политику запуска виртуальной машины и политику применения изменений;
- сценарии начальной конфигурации (cloud-init).

### Создание диска для виртуальной машины

Создайте диск с установыленной ОС для виртуальной машины:

```yaml
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualMachineDisk
metadata:
  name: ubuntu-2204-root
spec:
  persistentVolumeClaim:
    size: 10Gi
  dataSource:
    type: HTTP
    http:
      url: "https://cloud-images.ubuntu.com/minimal/releases/jammy/release-20230615/ubuntu-22.04-minimal-cloudimg-amd64.img"
```

### Создание виртуальной машины

Ниже представлен пример простой конфигурации виртуальной машины, запускающей ОС Ubuntu 22.04. В примере используется сценарий первичной инициализации виртуальной машины (cloud-init), который устанавливает пакет **nginx** и создает пользователя `cloud` с паролем `cloud`:

```yaml
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualMachine
metadata:
  name: linux-vm
  namespace: default
  labels:
    vm: linux
spec:
  runPolicy: AlwaysOn
  provisioning:
    type: UserData
    userData: |
      #cloud-config
      package_update: true
      packages:
        - nginx
      run_cmd:
        - systemctl daemon-relaod
        - systemctl enable --now nginx
      users:
      - name: cloud
        # password: cloud
        passwd: $6$rounds=4096$vln/.aPHBOI7BMYR$bBMkqQvuGs5Gyd/1H5DP4m9HjQSy.kgrxpaGEHwkX7KEFV8BS.HZWPitAtZ2Vd8ZqIZRqmlykRCagTgPejt1i.
        shell: /bin/bash
        sudo: ALL=(ALL) NOPASSWD:ALL
        chpasswd: { expire: False }
        lock_passwd: false
  cpu:
    cores: 1
  memory:
    size: 2Gi
  blockDevices:
    # Порядок дисков и образов в данном блоке определяет приоритет загрузки.
    - type: VirtualMachineDisk
      virtualMachineDisk:
        name: ubuntu-2204-root
```

При наличии приватных данных, сценарий начальной инициализации виртуальной машины может быть создан в Secret'е. Пример Secret'а приведен ниже:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: linux-vm-cloud-init
  namespace: default
data:
  userData: # Тут cloud-init-конфиг в Base64.
type: Opaque
```

Спецификация виртуальной машины будет выглядеть следующим образом:

```yaml
spec:
  provisioning:
    type: UserDataSecret
    userDataSercertRef:
      name: linux-vm-cloud-init
```

1. Создайте виртуальную машину из манифеста представленного выше.

После запуска виртуальная машина должна иметь статус `Ready`.

```bash
kubectl get virtualmachine

# NAME       PHASE     NODENAME      IPADDRESS     AGE
# linux-vm   Running   node-name-x   10.66.10.1    5m
```

После создания виртуальная машина автоматически получит IP-адрес из диапазона, указанного в настройках модуля (блок `virtualMachineCIDRs`).

2. Чтобы зафиксировать IP-адрес виртуальной машины перед ее запуском, выполните следующие шаги:

* Создайте ресурс `VirtualMachineIPAddressClaim`, в котором зафиксирован желаемый IP-адрес виртуальной машины:

```yaml
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualMachineIPAddressClaim
metadata:
  name: <claim-name>
  namespace: <namespace>
spec:
  address: "W.X.Y.Z"
```

* Зафиксируйте изменения в спецификации виртуальной машины:

```yaml
spec:
  virtualMachineIPAddressClaimName: <claim-name>
```

### 2. Настройка правил размещения виртуальной машины

1. Для того, чтобы виртуальная машина запускалась на заданном наборе узлов, например, на группе узлов `system`, используйте следующий фрагмент конфигурации:

```yaml
spec:
  tolerations:
    - key: "node-role.kubernetes.io/system"
      operator: Exists
      effect: NoSchedule
  nodeSelector:
    node-role.kubernetes.io/system: ""
```

2. Внесите изменения в ранее созданную спецификацию виртуальной машины.

### 3. Настройка порядка применения изменений

Внесенные изменения в конфигурацию виртуальной машины не отобразятся, так как по умолчанию применяется политика изменений `Manual`. Для применения изменений виртуальную машину требуется перезагрузить.

1. Чтобы проверить статус виртуальной машины, введите командую:

```bash
kubectl get linux-vm -o jsonpath='{.status}'
```

В поле `.status.pendingChanges` отобразятся изменения, которые требуют подтверждения.

В поле `.status.message` появится сообщение: для применения изменений, необходимо перезапустить виртуальную машину.

2. Создайте и примените ресурс, который отвечает за декларативный способ управления состоянием виртуальной машины, как представлено на примере ниже:

```bash
cat <<EOF | kubectl apply -f -
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualMachineOperation
metadata:
  name: restart
spec:
  virtualMachineName: linux-vm
  type: Restart
EOF
```

3. Проверьте состояние созданного ресурса:

```bash
kubectl get vmops restart

# NAME       PHASE       VMNAME     AGE
# restart    Completed   linux-vm   1m
```

Если созданный ресурс находится в состоянии `Completed` - перезагрузка виртуальной машины завершилась и новые параметры конфигурации виртуальной машины применены.

Чтобы изменения в конфигурации виртуальной машины применялись автоматически при ее перезапуске, настройте политику применения изменений следующим образом (пример ниже):

```yaml
spec:
  disruptions:
    approvalMode: Automatic
```

### 4. Политика запуска виртуальной машины

1. Подключитесь к виртуальной машине с использованием серийной консоли с помощью комнды:

```bash
dvp console -n default linux-vm
```

2. Завершите работу виртуальной машины с помощью команды:

```bash
cloud@linux-vm$ sudo poweroff
```

Далее посмотрим на статус виртуальной машины с помощью команды:

```bash
kubectl get virtualmachine

# NAME       PHASE     NODENAME       IPADDRESS   AGE
# linux-vm   Running   node-name-x    10.66.10.1  5m
```

Виртуальная машина была перезапущена. Причина перезапуска:

> В отличие от традиционных систем виртуализации, мы используем политику запуска для определения состояния виртуальной машины, которая определяет требуемое состояние виртуальной машины в любое время.

> При создании виртуальной машины используется параметр `runPolicy: AlwaysOn`. Это означает, что виртуальная машина будет запущена, даже если по каким-либо причинам произошло ее отключение, перезапуск или сбой, вызвавший прекращение ее работы.

Для выключения виртуальной машины, поменяйте значение политики на `AlwaysOff`. После чего произойдет корректное завершение работы виртуальной машины.
