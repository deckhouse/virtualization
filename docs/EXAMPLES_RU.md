---
title: "Примеры конфигурации"
---

## Быстрый старт

Пример создания виртуальной машины с Ubuntu 22.04.

Создадим namespace, где будем создавать виртуальные машины:

```bash
kubectl create ns vms
```

Создадим диск виртуальной машины из внешнего источника:

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

Посмотрим на текущий статус ресурса:

```bash
kubectl -n vms get virtualmachinedisk -o wide

# NAME            PHASE   CAPACITY    PROGRESS   TARGET PVC                                               AGE
# linux-disk      Ready   10Gi        100%       vmd-vmd-blank-001-10c7616b-ba9c-4531-9874-ebcb3a2d83ad   1m
```

Далее создадим виртуальную машину из следующей спецификации:

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

Проверим, что виртуальная машина создана и запущена:

```bash
kubectl -n default get virtualmachine

# NAME       PHASE     NODENAME   IPADDRESS    AGE
# linux-vm   Running   virtlab-1  10.66.10.1   5m
```

Подключимся к виртуальной машине с использованием консоли (для выхода из консоли необходимо нажать `Ctrl+]`):

```bash
dvp console -n vms linux-vm
```

Подключимся к машине с использованием VNC:

```bash
dvp vnc -n vms linux-vm
```

После выполнения команды запустится VNC-клиент, используемый в системе по умолчанию. Альтернативный способ подключения — с помощью параметра `--proxy-only` пробросить VNC-порт на локальную машину.

## Образы

`VirtualMachineImage` и `ClusterVirtualMachineImage` предназначены для хранения образов дисков виртуальных машин или установочных образов в формате `iso`, чтобы создавать и однотипно воспроизводить диски виртуальных машин. При подключении к виртуальной машине эти образы доступны только для чтения, и установочный образ в формате `iso` будет подключен в виде cdrom-устройства.

Ресурс `VirtualMachineImage` доступен только в том пространстве имен, в котором он был создан, а `ClusterVirtualMachineImage` доступен для всех пространств имен внутри кластера.

В зависимости от конфигурации, ресурс `VirtualMachineImage` может хранить данные в `DVCR` или использовать дисковое хранилище, предоставляемое платформой (PV). С другой стороны, `ClusterVirtualMachineImage` хранит данные только в `DVCR`, обеспечивая единый доступ ко всем образам для всех пространств имен в кластере.

Рассмотрим на примерах создание этих ресурсов.

### Создание и использование образа c HTTP-ресурса

Создадим `VirtualMachineImage` и в качестве хранилища образов будем использовать `DVCR`:

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

Посмотрим, что получилось:

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

Для просмотра списка доступных StorageClass'ов выполните следующую команду:

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

Просмотрим статус `ClusterVirtualMachineImage`:

```bash
kubectl get clustervirtualmachineimage

# NAME         PHASE   CDROM   PROGRESS   AGE
# ubuntu-img   Ready   false   100%       11m
```

Образы могут быть созданы из различных внешних источников, таких как HTTP-сервер, где размещены файлы образов или контейнерный реестр (container registry), где образы хранятся и доступны для загрузки. Кроме того, возможно загрузить образы напрямую из командной строки, используя утилиту curl. Давайте рассмотрим каждый из этих вариантов более подробно.

### Создание и использование образа из container registry

Первое, что необходимо, — это сформировать сам образ для хранения в container registry.

В качестве примера рассмотрим вариант создания docker-образа c диском Ubuntu 22.04.

Загрузим образ локально:

```bash
curl -L https://cloud-images.ubuntu.com/minimal/releases/jammy/release-20230615/ubuntu-22.04-minimal-cloudimg-amd64.img -o ubuntu2204.img
```

Создадим Dockerfile со следующим содержимым:

```Dockerfile
FROM scratch
COPY ubuntu2204.img /disk/ubuntu2204.img
```

Соберем образ и загрузим его в container registry. В качестве container registry будем использовать docker.io, для этого вам необходимо иметь учетную запись сервиса и настроенное окружение.

```bash
docker build -t docker.io/username/ubuntu2204:latest
```

где `username` — ваше имя пользователя, указанное при регистрации в docker.io.

Загрузим созданный образ в container registry:

```bash
docker push docker.io/username/ubuntu2204:latest
```

Чтобы использовать этот образ, создадим в качестве примера ресурс `ClusterVirtualMachineImage`:

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

Чтобы посмотреть ресурс и его статус, выполните команду:

```bash
kubectl get clustervirtalmachineimage
```

### Загрузка образа из командной строки

Чтобы загрузить образ из командной строки, нам предварительно нужно создать следующий ресурс, рассмотрим на примере `ClusterVirtualMachineImage`:

```yaml
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: ClusterVirtualMachineImage
metadata:
  name: some-image
spec:
  dataSource:
    type: Upload
```

После того как ресурс будет создан, посмотрим его статус:

```bash
kubectl get clustervirtualmachineimages some-image -o json | jq .status.uploadCommand -r

> uploadCommand: curl https://virtualization.example.com/upload/dSJSQW0fSOerjH5ziJo4PEWbnZ4q6ffc
    -T example.iso
```

> Стоит отметить, что CVMI с типом Upload ожидает начала загрузки образа 15 минут после создания. По истечении данного таймаута ресурс перейдет в состояние Failed.

Загрузим для примера образ Cirros и загрузим его:

```bash
curl -L http://download.cirros-cloud.net/0.5.1/cirros-0.5.1-x86_64-disk.img -o cirros.img
https://virtualization.example.com/upload/dSJSQW0fSOerjH5ziJo4PEWbnZ4q6ffc -T cirros.img
```

После завершения работы команды `curl` образ должен быть создан.

Проверить, что все прошло успешно, можно, проверив статус созданного образа:

```bash
kubectl get clustervirtualmachineimages

# NAME         PHASE   CDROM   PROGRESS   AGE
# some-image   Ready   false   100%       10m
```

## Диски

Диски используются в виртуальных машинах для записи и хранения данных. Для хранения дисков используется хранилище, предоставляемое платформой.

Чтобы посмотреть доступные варианты, выполните команду:

```bash
kubectl get storageclass
```

Рассмотрим варианты, какие диски мы можем создавать:

### Создание пустого диска

Первое, что стоит отметить, — это то, что диски мы можем создавать пустыми!

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

После создания диска мы его можем использовать для подключения к виртуальной машине.

Посмотреть состояние созданного ресурса можно командой:

```bash
kubectl get virtualmachinedisk

# NAME        PHASE  CAPACITY   AGE
# vmd-blank   Ready  100Mi      1m
```

### Создание диска из образа

Мы можем создавать диски, используя уже имеющиеся образы дисков, а также внешние источники, подобно образам.

При создании ресурса диска мы можем указать желаемый размер. Если размер не указан, то будет создан диск с размером, соответствующим исходному образу диска, который хранится в ресурсе `VirtualMachineImage` или `ClusterVirtualMachineImage`. Если необходимо создать диск большего размера, необходимо явно указать это.

В качестве примера используем ранее созданный `ClusterVirtualMachineImage` с именем `ubuntu-2204`:

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

Размер дисков можно менять (пока только в сторону увеличения), даже если они подключены к виртуальной машине, для этого необходимо отредактировать поле `spec.persistentVolumeClame.size`:

```yaml
kubectl patch ubuntu-root --type merge -p '{"spec":{"persistentVolumeClaim":{"size":"11Gi"}}}'
```

### Подключение дисков к запущенным виртуальным машинам

Диски можно подключать «на живую», к уже запущенной виртуальной машине, для этого используется ресурс `VirtualMachineBlockDeviceAttachment`, например:

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

Если изменить имя машины в этом ресурсе на имя другой машины, диск будет переподключен от одной виртуальной машины к другой.
При удалении ресурса `VirtualMachineBlockDeviceAttachment` диск от виртуальной машины будет отключен.

Чтобы посмотреть список подключенных «на живую» дисков, выполните команду:

```bash
kubectl get virtualmachineblockdeviceattachments
```

## Виртуальные машины

Итак, теперь у нас есть диски и образы, перейдем к самому главному — созданию виртуальной машины.

Для создания виртуальной машины используется ресурс `VirtualMachine`, его параметры позволяют сконфигурировать:

- ресурсы, требуемые для работы виртуальной машины (процессор, память, диски и образы);
- правила размещения виртуальной машины на узлах кластера;
- настройки загрузчика и оптимальные параметры для гостевой ОС;
- политику запуска виртуальной машины и политику применения изменений;
- сценарии начальной конфигурации (cloud-init).

Cоздадим виртуальную машину и настроим ее по шагам:

### 0. Создание диска для виртуальной машины

Первое, что нужно, прежде чем создавать ресурс виртуальной машины, — это создать диск с установленной ОС.

Создадим диск для виртуальной машины:

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

### 1. Создание виртуальной машины

Ниже представлен пример простейшей конфигурации виртуальной машины, запускающей ОС Ubuntu 22.04. В примере используется сценарий первичной инициализации виртуальной машины (cloud-init), который устанавливает пакет nginx и создает пользователя `cloud` с паролем `cloud`:

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

При наличии каких-то приватных данных сценарий начальной инициализации виртуальной машины может быть создан в Secret'е.

Пример Secret'а:

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

Как это будет выглядеть в спецификации виртуальной машины:

```yaml
spec:
  provisioning:
    type: UserDataSecret
    userDataSercertRef:
      name: linux-vm-cloud-init
```

Создадим виртуальную машину из манифеста выше.

После запуска виртуальня машина должна быть в статусе `Ready`.

```bash
kubectl get virtualmachine

# NAME       PHASE     NODENAME      IPADDRESS     AGE
# linux-vm   Running   node-name-x   10.66.10.1    5m
```

После создания виртуальная машина автоматически получит IP-адрес из диапазона, указанного в настройках модуля (блок `virtualMachineCIDRs`).

Если мы хотим зафиксировать конкретный IP-адрес для машины перед ее запуском, необходимо выполнить следующие шаги:

1. Создать ресурс `VirtualMachineIPAddressClaim`, в котором зафиксировать желаемый IP-адрес виртуальной машины:

```yaml
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualMachineIPAddressClaim
metadata:
  name: <claim-name>
  namespace: <namespace>
spec:
  address: "W.X.Y.Z"
```

2. Соответствующим образом зафиксировать изменения в спецификации виртуальной машины:

```yaml
spec:
  virtualMachineIPAddressClaimName: <claim-name>
```

### 2. Настройка правил размещения виртуальной машины

Допустим, нам необходимо, чтобы виртуальная машина запускалась на заданном наборе узлов, например на группе узлов `system`. В этом нам поможет следующий фрагмент конфигурации:

```yaml
spec:
  tolerations:
    - key: "node-role.kubernetes.io/system"
      operator: Exists
      effect: NoSchedule
  nodeSelector:
    node-role.kubernetes.io/system: ""
```

Внесите изменения в ранее созданную спецификацию виртуальной машины.

### 3. Настройка порядка применения изменений

После внесения изменений в конфигурацию машины ничего не произойдет, так как по умолчанию применяется политика применения изменений `Manual`, а это значит, что изменения нужно подтвердить.

Как мы можем это понять?

Посмотрим на статус VM:

```bash
kubectl get linux-vm -o jsonpath='{.status}'
```

В поле `.status.pendingChanges` мы увидим изменения, которые требуют применения. В поле `.status.message` сообщение, о том что для применения требуемых изменений необходим рестарт виртуальной машины.

Создадим и применим следующий ресурс, от отвечает за декларативный способ управления состоянием виртуальной машины:

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

Посмотрим за состоянием созданного ресурса:

```bash
kubectl get vmops restart

# NAME       PHASE       VMNAME     AGE
# restart    Completed   linux-vm   1m
```

Как только он перейдет в состояние `Completed` - перезагрузка виртуальной машины завершилась, и новые параметры конфигурации виртуальной машины - применены.

Что делать, если мы хотим, чтобы изменения требуемые перезагрузки виртуальной машины применялись автоматически? Для этого необходимо сконфигурировать политику применения изменений следующим образом:

```yaml
spec:
  disruptions:
    approvalMode: Automatic
```

### 4. Политика запуска виртуальной машины

Подключимся к виртуальной машине с использованием серийной консоли:

```bash
dvp console -n default linux-vm
```

Завершим работу виртуальной машины:

```bash
cloud@linux-vm$ sudo poweroff
```

Далее посмотрим на статус виртуальной машины:

```bash
kubectl get virtualmachine

# NAME       PHASE     NODENAME       IPADDRESS   AGE
# linux-vm   Running   node-name-x    10.66.10.1  5m
```

Виртуальная машина снова запущена! Но почему так произошло?

Во отличие от классических систем виртуализации, для определения состояния виртуальной машины мы используем политику запуска, которая определяет желаемое состояние виртуальной машины в любой момент времени.

При создании виртуальной машины мы указали параметр `runPolicy: AlwaysOn`, а это значит, что виртуальная машина должна быть запущена, даже если по какой-то причине произошло ее выключение, рестарт или сбой, повлекший завершение ее работы.

Чтобы выключить машину, поменяем значение политики на `AlwaysOff`, при этом произойдет корректное завершение работы виртуальной машины.
