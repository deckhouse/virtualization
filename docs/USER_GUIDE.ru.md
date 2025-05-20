---
title: "Руководство пользователя"
menuTitle: "Руководство пользователя"
weight: 50
---

## Введение

Данное руководство предназначено для пользователей Deckhouse Virtualization Platform и описывает порядок создания и изменения ресурсов, которые доступны для создания в проектах и пространствах имен кластера.

## Быстрый старт по созданию ВМ

Пример создания виртуальной машины с Ubuntu 22.04.

1. Создайте образ виртуальной машины из внешнего источника:

   ```yaml
   d8 k apply -f - <<EOF
   apiVersion: virtualization.deckhouse.io/v1alpha2
   kind: VirtualImage
   metadata:
     name: ubuntu
   spec:
     storage: ContainerRegistry
     dataSource:
       type: HTTP
       http:
         url: https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.img
   EOF
   ```

1. Создайте диск виртуальной машины из образа, созданного на предыдущем шаге (Внимание: перед созданием убедитесь, что в системе присутствует StorageClass по умолчанию):

   ```yaml
   d8 k apply -f - <<EOF
   apiVersion: virtualization.deckhouse.io/v1alpha2
   kind: VirtualDisk
   metadata:
     name: linux-disk
   spec:
     dataSource:
       type: ObjectRef
       objectRef:
         kind: VirtualImage
         name: ubuntu
   EOF
   ```

1. Создайте виртуальную машину:

   В примере используется cloud-init-сценарий для создания пользователя cloud с паролем cloud, сгенерированный следующим образом:

   ```bash
   mkpasswd --method=SHA-512 --rounds=4096
   ```

   Изменить имя пользователя и пароль можно в этой секции:

   ```yaml
   users:
     - name: cloud
       passwd: $6$rounds=4096$G5VKZ1CVH5Ltj4wo$g.O5RgxYz64ScD5Ach5jeHS.Nm/SRys1JayngA269wjs/LrEJJAZXCIkc1010PZqhuOaQlANDVpIoeabvKK4j1
   ```

   Создайте виртуальную машину из следующей спецификации:

   ```yaml
   d8 k apply -f - <<"EOF"
   apiVersion: virtualization.deckhouse.io/v1alpha2
   kind: VirtualMachine
   metadata:
     name: linux-vm
   spec:
     virtualMachineClassName: host
     cpu:
       cores: 1
     memory:
       size: 1Gi
     provisioning:
       type: UserData
       userData: |
         #cloud-config
         ssh_pwauth: True
         users:
         - name: cloud
           passwd: '$6$rounds=4096$saltsalt$fPmUsbjAuA7mnQNTajQM6ClhesyG0.yyQhvahas02ejfMAq1ykBo1RquzS0R6GgdIDlvS.kbUwDablGZKZcTP/'
           shell: /bin/bash
           sudo: ALL=(ALL) NOPASSWD:ALL
           lock_passwd: False
     blockDeviceRefs:
       - kind: VirtualDisk
         name: linux-disk
   EOF
   ```

   Полезные ссылки:

   - [Документация по cloud-init](https://cloudinit.readthedocs.io/).
   - [Параметры ресурсов](cr.html).

1. Проверьте с помощью команды, что образ и диск созданы, а виртуальная машина - запущена. Ресурсы создаются не мгновенно, поэтому прежде чем они придут в готовое состояние потребуется подождать какое-то время.

   ```bash
   d8 k  get vi,vd,vm
   ```

   Пример вывода:

   ```txt
   NAME                                                 PHASE   CDROM   PROGRESS   AGE
   virtualimage.virtualization.deckhouse.io/ubuntu      Ready   false   100%
   #
   NAME                                                 PHASE   CAPACITY   AGE
   virtualdisk.virtualization.deckhouse.io/linux-disk   Ready   300Mi      7h40m
   #
   NAME                                                 PHASE     NODE           IPADDRESS     AGE
   virtualmachine.virtualization.deckhouse.io/linux-vm  Running   virtlab-pt-2   10.66.10.2    7h46m
   ```

1. Подключитесь с помощью консоли к виртуальной машине (для выхода из консоли необходимо нажать `Ctrl+]`):

   ```bash
   d8 v console linux-vm
   ```

   Пример вывода:

   ```txt
   Successfully connected to linux-vm console. The escape sequence is ^]
   #
   linux-vm login: cloud
   Password: cloud
   ...
   cloud@linux-vm:~$
   ```

1. Для удаления созданных ранее ресурсов используйте следующие команды:

   ```bash
   d8 k delete vm linux-vm
   d8 k delete vd linux-disk
   d8 k delete vi ubuntu
   ```

## Образы

Ресурс `VirtualImage` предназначен для загрузки образов виртуальных машин и их последующего использования для создания дисков виртуальных машин. Данный ресурс доступен только в неймспейсе или проекте в котором он был создан.

При подключении к виртуальной машине доступ к образу предоставляется в режиме «только чтение».

Процесс создания образа включает следующие шаги:

- Пользователь создаёт ресурс `VirtualImage`.
- После создания образ автоматически загружается из указанного в спецификации источника в хранилище (DVCR).
- После завершения загрузки, ресурс становится доступным для создания дисков.

Существуют различные типы образов:

- **ISO-образ** — установочный образ, используемый для начальной установки операционной системы. Такие образы выпускаются производителями ОС и используются для установки на физические и виртуальные серверы.
- **Образ диска с предустановленной системой** — содержит уже установленную и настроенную операционную систему, готовую к использованию после создания виртуальной машины. Готовые образы можно получить на ресурсах разработчиков дистрибутива, либо создать самостоятельно.

Примеры ресурсов для получения образов виртуальной машины:

- Ubuntu
  - [24.04 LTS (Noble Numbat)](https://cloud-images.ubuntu.com/noble/current/)
  - [22.04 LTS (Jammy Jellyfish)](https://cloud-images.ubuntu.com/jammy/current/)
  - [20.04 LTS (Focal Fossa)](https://cloud-images.ubuntu.com/focal/current/)
  - [Minimal images](https://cloud-images.ubuntu.com/minimal/releases/)
- Debian
  - [12 bookworm](https://cdimage.debian.org/images/cloud/bookworm/latest/)
  - [11 bullseye](https://cdimage.debian.org/images/cloud/bullseye/latest/)
- AlmaLinux
  - [9](https://repo.almalinux.org/almalinux/9/cloud/x86_64/images/)
  - [8](https://repo.almalinux.org/almalinux/8/cloud/x86_64/images/)
- RockyLinux
  - [9.5](https://download.rockylinux.org/pub/rocky/9.5/images/x86_64/)
  - [8.10](https://download.rockylinux.org/pub/rocky/8.10/images/x86_64/)
- CentOS
  - [10 Stream](https://cloud.centos.org/centos/10-stream/x86_64/images/)
  - [9 Stream](https://cloud.centos.org/centos/9-stream/x86_64/images/)
  - [8 Stream](https://cloud.centos.org/centos/8-stream/x86_64/)
  - [8](https://cloud.centos.org/centos/8/x86_64/images/)
- Alt Linux
  - [p10](https://ftp.altlinux.ru/pub/distributions/ALTLinux/p10/images/cloud/x86_64/)
  - [p9](https://ftp.altlinux.ru/pub/distributions/ALTLinux/p9/images/cloud/x86_64/)
- [Astra Linux](https://download.astralinux.ru/ui/native/mg-generic/alse/cloudinit)

Поддерживаются следующие форматы образов с предустановленной системой:

- qcow2
- raw
- vmdk
- vdi

Также файлы образов могут быть сжаты одним из следующих алгоритмов сжатия: gz, xz.

После создания ресурса, тип и размер образа определяются автоматически и эта информация отражается в статусе ресурса.

Образы могут быть загружены из различных источников, таких как HTTP-серверы, где расположены файлы образов, или контейнерные реестры. Также доступна возможность загрузки образов напрямую из командной строки с использованием утилиты curl.

Образы могут быть созданы из других образов и дисков виртуальных машин.

Проектный образ поддерживает два варианта хранения:

- `ContainerRegistry` - тип по умолчанию, при котором образ хранится в `DVCR`.
- `PersistentVolumeClaim` - тип, при котором в качестве хранилища для образа используется `PVC`. Этот вариант предпочтителен, если используется хранилище с поддержкой быстрого клонирования `PVC`, что позволяет быстрее создавать диски из образов.

С полным описанием параметров конфигурации ресурса `VirtualImage` можно ознакомиться [в документации к ресурсу](cr.html#virtualimage).

### Создание образа с HTTP-сервера

Рассмотрим вариант создания образа с вариантом хранения в DVCR.

1. Выполните следующую команду для создания `VirtualImage`:

   ```yaml
   d8 k apply -f - <<EOF
   apiVersion: virtualization.deckhouse.io/v1alpha2
   kind: VirtualImage
   metadata:
     name: ubuntu-22-04
   spec:
     # Сохраним образ в DVCR.
     storage: ContainerRegistry
     # Источник для создания образа.
     dataSource:
       type: HTTP
       http:
         url: https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.img
   EOF
   ```

1. Проверьте результат создания `VirtualImage`:

   ```bash
   d8 k get virtualimage ubuntu-22-04
   # или более короткий вариант
   d8 k get vi ubuntu-22-04
   ```

   Пример вывода:

   ```txt
   NAME           PHASE   CDROM   PROGRESS   AGE
   ubuntu-22-04   Ready   false   100%       23h
   ```

После создания ресурс `VirtualImage` может находиться в следующих состояниях (фазах):

- `Pending` - ожидание готовности всех зависимых ресурсов, требующихся для создания образа.
- `WaitForUserUpload` - ожидание загрузки образа пользователем (фаза присутствует только для `type=Upload`).
- `Provisioning` - идет процесс создания образа.
- `Ready` - образ создан и готов для использования.
- `Failed` - произошла ошибка в процессе создания образа.
- `Terminating` - идет процесс удаления Образа. Образ может «зависнуть» в данном состоянии, если он еще подключен к виртуальной машине.

До тех пор пока образ не перешёл в фазу `Ready`, содержимое всего блока `.spec` допускается изменять. При изменении процесс создании диска запустится заново. После перехода в фазу `Ready` содержимое блока `.spec` менять нельзя!

Диагностика проблем с ресурсом осуществляется путем анализа информации в блоке `.status.conditions`.

Отследить процесс создания образа можно путем добавления ключа `-w` к предыдущей команде:

```bash
d8 k get vi ubuntu-22-04 -w
```

Пример вывода:

```txt
NAME           PHASE          CDROM   PROGRESS   AGE
ubuntu-22-04   Provisioning   false              4s
ubuntu-22-04   Provisioning   false   0.0%       4s
ubuntu-22-04   Provisioning   false   28.2%      6s
ubuntu-22-04   Provisioning   false   66.5%      8s
ubuntu-22-04   Provisioning   false   100.0%     10s
ubuntu-22-04   Provisioning   false   100.0%     16s
ubuntu-22-04   Ready          false   100%       18s
```

В описание ресурса `VirtualImage` можно получить дополнительную информацию о скачанном образе:

```bash
d8 k describe vi ubuntu-22-04
```

Теперь рассмотрим пример создания образа с хранением его в PVC:

```yaml
d8 k apply -f - <<EOF
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualImage
metadata:
  name: ubuntu-22-04-pvc
spec:
  # Настройки хранения проектного образа.
  storage: PersistentVolumeClaim
  persistentVolumeClaim:
    # Подставьте ваше название StorageClass.
    storageClassName: i-sds-replicated-thin-r2
  # Источник для создания образа.
  dataSource:
    type: HTTP
    http:
      url: https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.img
EOF
```

Проверьте результат создания `VirtualImage`:

```bash
d8 k get vi ubuntu-22-04-pvc
```

Пример вывода:

```txt
NAME              PHASE   CDROM   PROGRESS   AGE
ubuntu-22-04-pvc  Ready   false   100%       23h
```

Если параметр `.spec.persistentVolumeClaim.storageClassName` не указан, то будет использован `StorageClass` по умолчанию на уровне кластера, либо для образов, если он указан в [настройках модуля](./admin_guide.html#настройки-классов-хранения-для-образов).

### Создание образа из Container Registry

Образ, хранящийся в Container Registry, имеет определенный формат. Рассмотрим на примере:

1. Загрузите образ локально:

   ```bash
   curl -L https://cloud-images.ubuntu.com/minimal/releases/jammy/release/ubuntu-22.04-minimal-cloudimg-amd64.img -o ubuntu2204.img
   ```

1. Создайте `Dockerfile` со следующим содержимым:

   ```Dockerfile
   FROM scratch
   COPY ubuntu2204.img /disk/ubuntu2204.img
   ```

1. Соберите образ и загрузите его в container registry. В качестве container registry в примере ниже использован docker.io. Для выполнения необходимо иметь учетную запись сервиса и настроенное окружение.

   ```bash
   docker build -t docker.io/<username>/ubuntu2204:latest
   ```

   где `username` — имя пользователя, указанное при регистрации в docker.io.

1. Загрузите созданный образ в container registry:

   ```bash
   docker push docker.io/<username>/ubuntu2204:latest
   ```

1. Чтобы использовать этот образ, создайте в качестве примера ресурс:

   ```yaml
   d8 k apply -f - <<EOF
   apiVersion: virtualization.deckhouse.io/v1alpha2
   kind: VirtualImage
   metadata:
     name: ubuntu-2204
   spec:
     storage: ContainerRegistry
     dataSource:
       type: ContainerImage
       containerImage:
         image: docker.io/<username>/ubuntu2204:latest
   EOF
   ```

### Загрузка образа из командной строки

Чтобы загрузить образ из командной строки, предварительно создайте ресурс, как представлено ниже на примере `VirtualImage`:

```yaml
d8 k apply -f - <<EOF
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualImage
metadata:
  name: some-image
spec:
  # Настройки хранения проектного образа.
  storage: ContainerRegistry
  # Настройки источника образа.
  dataSource:
    type: Upload
EOF
```

После создания, ресурс перейдет в фазу `WaitForUserUpload`, а это значит, что он готов для загрузки образа.

Доступно два варианта загрузки с узла кластера и с произвольного узла за пределами кластера:

```bash
d8 k get vi some-image -o jsonpath="{.status.imageUploadURLs}"  | jq
```

Пример вывода:

```json
{
  "external":"https://virtualization.example.com/upload/g2OuLgRhdAWqlJsCMyNvcdt4o5ERIwmm",
  "inCluster":"http://10.222.165.239/upload"
}
```

В качестве примера загрузите образ Cirros:

```bash
curl -L http://download.cirros-cloud.net/0.5.1/cirros-0.5.1-x86_64-disk.img -o cirros.img
```

Выполните загрузку образа с использование следующей команды

```bash
curl https://virtualization.example.com/upload/g2OuLgRhdAWqlJsCMyNvcdt4o5ERIwmm --progress-bar -T cirros.img | cat
```

После завершения загрузки образ должен быть создан и перейти в фазу `Ready`

```bash
d8 k get vi some-image
```

Пример вывода:

```txt
NAME         PHASE   CDROM   PROGRESS   AGE
some-image   Ready   false   100%       1m
```

### Создание образа из диска

Существует возможность создать образ из [диска](#диски). Для этого необходимо выполнить одно из следующих условий:

- Диск не подключен ни к одной из виртуальных машин.
- Виртуальная машина, к которой подключен диск, находится в выключенном состоянии.

Пример создания образа из диска:

```yaml
d8 k apply -f - <<EOF
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualImage
metadata:
  name: linux-vm-root
spec:
  storage: ContainerRegistry
  dataSource:
    type: ObjectRef
    objectRef:
      kind: VirtualDisk
      name: linux-vm-root
EOF
```

### Создание образа из снимка диска

Можно создать образ из [снимка](#снимки). Для этого необходимо чтобы снимок диска находился в фазе готовности.

Пример создания образа из моментального снимка диска:

```yaml
d8 k apply -f - <<EOF
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualImage
metadata:
  name: linux-vm-root
spec:
  storage: ContainerRegistry
  dataSource:
    type: ObjectRef
    objectRef:
      kind: VirtualDiskSnapshot
      name: linux-vm-root-snapshot
EOF
```

## Диски

Диски в виртуальных машинах необходимы для записи и хранения данных, они обеспечивают полноценное функционирование приложений и операционных систем. Хранилище для этих дисков предоставляет платформа.

В зависимости от свойств хранилища, поведение дисков при создании и виртуальных машин в процессе эксплуатации может отличаться:

Свойство VolumeBindingMode:

`Immediate` - Диск создается сразу после создания ресурса (предполагается, что диск будет доступен для подключения к виртуальной машине на любом узле кластера).

![](images/vd-immediate.ru.png)

`WaitForFirstConsumer` - Диск создается только после того как будет подключен к виртуальной машине и будет создан на том узле, на котором будет запущена виртуальная машина.

![](images/vd-wffc.ru.png)

Режим доступа AccessMode:

- `ReadWriteOnce (RWO)` - доступ к диску предоставляется только одному экземпляру виртуальной машины. Живая миграция виртуальных машин с такими дисками невозможна.
- `ReadWriteMany (RWX)` - множественный доступ к диску. Живая миграция виртуальных машин с такими дисками возможна.

При создании диска контроллер самостоятельно определит наиболее оптимальные параметры поддерживаемые хранилищем.

> Внимание: Создать диски из iso-образов - нельзя!

Чтобы узнать доступные варианты хранилищ на платформе, выполните следующую команду:

```bash
d8 k  get storageclass
```

Пример вывода:

```txt
NAME                                 PROVISIONER                           RECLAIMPOLICY   VOLUMEBINDINGMODE      ALLOWVOLUMEEXPANSION   AGE
i-sds-replicated-thin-r1 (default)   replicated.csi.storage.deckhouse.io   Delete          Immediate              true                   48d
i-sds-replicated-thin-r2             replicated.csi.storage.deckhouse.io   Delete          Immediate              true                   48d
i-sds-replicated-thin-r3             replicated.csi.storage.deckhouse.io   Delete          Immediate              true                   48d
sds-replicated-thin-r1               replicated.csi.storage.deckhouse.io   Delete          WaitForFirstConsumer   true                   48d
sds-replicated-thin-r2               replicated.csi.storage.deckhouse.io   Delete          WaitForFirstConsumer   true                   48d
sds-replicated-thin-r3               replicated.csi.storage.deckhouse.io   Delete          WaitForFirstConsumer   true                   48d
nfs-4-1-wffc                         nfs.csi.k8s.io                        Delete          WaitForFirstConsumer   true                   30d
```

С полным описанием параметров конфигурации дисков можно ознакомиться [в документации ресурса](cr.html#virtualdisk).

### Создание пустого диска

Пустые диски обычно используются для установки на них ОС, либо для хранения каких-либо данных.

Создайте диск:

```yaml
d8 k apply -f - <<EOF
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualDisk
metadata:
  name: blank-disk
spec:
  # Настройки параметров хранения диска.
  persistentVolumeClaim:
    # Подставьте ваше название StorageClass.
    storageClassName: i-sds-replicated-thin-r2
    size: 100Mi
EOF
```

После создания ресурс `VirtualDisk` может находиться в следующих состояниях (фазах):

- `Pending` - ожидание готовности всех зависимых ресурсов, требующихся для создания диска.
- `Provisioning` - идет процесс создания диска.
- `Resizing` - идет процесс изменения размера диска.
- `WaitForFirstConsumer` - диск ожидает создания виртуальной машины, которая будет его использовать.
- `WaitForUserUpload` - диск ожидает от пользователя загрузки образа (type: Upload).
- `Ready` - диск создан и готов для использования.
- `Failed` - произошла ошибка в процессе создания.
- `PVCLost` - системная ошибка, PVC с данными утерян.
- `Terminating` - идет процесс удаления диска. Диск может «зависнуть» в данном состоянии, если он еще подключен к виртуальной машине.

До тех пор пока диск не перешёл в фазу `Ready` содержимое всего блока `.spec` допускается изменять. При изменении процесс создании диска запустится заново.

Диагностика проблем с ресурсом осуществляется путем анализа информации в блоке `.status.conditions`.

Если параметр `.spec.persistentVolumeClaim.storageClassName` не указан, то будет использован `StorageClass` по умолчанию на уровне кластера, либо для образов, если он указан в [настройках модуля](./admin_guide.html#настройки-классов-хранения-для-дисков).

Проверьте состояние диска после создания командой:

```bash
d8 k get vd blank-disk
```

Пример вывода:

```txt
NAME       PHASE   CAPACITY   AGE
blank-disk   Ready   100Mi      1m2s
```

### Создание диска из образа

Диск также можно создавать и заполнять данными из ранее созданных образов `ClusterVirtualImage` и `VirtualImage`.

При создании диска можно указать его желаемый размер, который должен быть равен или больше размера распакованного образа. Если размер не указан, то будет создан диск с размером, соответствующим исходному образу диска.

На примере ранее созданного проектного образа `VirtualImage`, рассмотрим команду позволяющую определить размер распакованного образа:

```bash
d8 k get cvi ubuntu-22-04 -o wide
```

Пример вывода:

```txt
NAME           PHASE   CDROM   PROGRESS   STOREDSIZE   UNPACKEDSIZE   REGISTRY URL                                                                       AGE
ubuntu-22-04   Ready   false   100%       285.9Mi      2.5Gi          dvcr.d8-virtualization.svc/cvi/ubuntu-22-04:eac95605-7e0b-4a32-bb50-cc7284fd89d0   122m
```

Искомый размер указан в колонке **UNPACKEDSIZE** и равен 2.5Gi.

Создадим диск из этого образа:

```yaml
d8 k apply -f - <<EOF
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualDisk
metadata:
  name: linux-vm-root
spec:
  # Настройки параметров хранения диска.
  persistentVolumeClaim:
    # Укажем размер больше чем значение распакованного образа.
    size: 10Gi
    # Подставьте ваше название StorageClass.
    storageClassName: i-sds-replicated-thin-r2
  # Источник из которого создается диск.
  dataSource:
    type: ObjectRef
    objectRef:
      kind: VirtualImage
      name: ubuntu-22-04
EOF
```

А теперь создайте диск, без явного указания размера:

```yaml
d8 k apply -f - <<EOF
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualDisk
metadata:
  name: linux-vm-root-2
spec:
  # Настройки параметров хранения диска.
  persistentVolumeClaim:
    # Подставьте ваше название StorageClass.
    storageClassName: i-sds-replicated-thin-r2
  # Источник из которого создается диск.
  dataSource:
    type: ObjectRef
    objectRef:
      kind: VirtualImage
      name: ubuntu-22-04
EOF
```

Проверьте состояние дисков после создания:

```bash
d8 k get vd
```

Пример вывода:

```txt
NAME           PHASE   CAPACITY   AGE
linux-vm-root    Ready   10Gi       7m52s
linux-vm-root-2  Ready   2590Mi     7m15s
```

### Изменение размера диска

Размер дисков можно увеличивать, даже если они уже подключены к работающей виртуальной машине. Для этого отредактируйте поле `spec.persistentVolumeClaim.size`:

Проверьте размер до изменения:

```bash
d8 k get vd linux-vm-root
```

Пример вывода:

```txt
NAME          PHASE   CAPACITY   AGE
linux-vm-root   Ready   10Gi       10m
```

Примените изменения:

```bash
d8 k patch vd linux-vm-root --type merge -p '{"spec":{"persistentVolumeClaim":{"size":"11Gi"}}}'
```

Проверьте размер после изменения:

```bash
d8 k get vd linux-vm-root
```

Пример вывода:

```txt
NAME          PHASE   CAPACITY   AGE
linux-vm-root   Ready   11Gi       12m
```

## Виртуальные машины

Для создания виртуальной машины используется ресурс `VirtualMachine`. Его параметры позволяют сконфигурировать:

- [класс виртуальной машины](admin_guide.html#классы-виртуальных-машин)
- ресурсы, требуемые для работы виртуальной машины (процессор, память, диски и образы);
- правила размещения виртуальной машины на узлах кластера;
- настройки загрузчика и оптимальные параметры для гостевой ОС;
- политику запуска виртуальной машины и политику применения изменений;
- сценарии начальной конфигурации (cloud-init);
- перечень блочных устройств.

С полным описанием параметров конфигурации виртуальных машин можно ознакомиться по [в документации конфигурации](cr.html#virtualmachine).

### Создание виртуальной машины

Ниже представлен пример конфигурации виртуальной машины, запускающей ОС Ubuntu 22.04. В примере используется сценарий первичной инициализации виртуальной машины (cloud-init), который устанавливает гостевого агента `qemu-guest-agent` и сервис `nginx`, а также создает пользователя `cloud` с паролем `cloud`:

Пароль в примере был сгенерирован с использованием команды `mkpasswd --method=SHA-512 --rounds=4096 -S saltsalt` и при необходимости вы можете его поменять на свой:

Создайте виртуальную машину с диском созданным [ранее](#создание-диска-из-образа):

```yaml
d8 k apply -f - <<"EOF"
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualMachine
metadata:
  name: linux-vm
spec:
  # Название класса ВМ.
  virtualMachineClassName: host
  # Блок скриптов первичной инициализации ВМ.
  provisioning:
    type: UserData
    # Пример cloud-init-сценария для создания пользователя cloud с паролем cloud и установки сервиса агента qemu-guest-agent и сервиса nginx.
    userData: |
      #cloud-config
      package_update: true
      packages:
        - nginx
        - qemu-guest-agent
      run_cmd:
        - systemctl daemon-reload
        - systemctl enable --now nginx.service
        - systemctl enable --now qemu-guest-agent.service
      ssh_pwauth: True
      users:
      - name: cloud
        passwd: '$6$rounds=4096$saltsalt$fPmUsbjAuA7mnQNTajQM6ClhesyG0.yyQhvahas02ejfMAq1ykBo1RquzS0R6GgdIDlvS.kbUwDablGZKZcTP/'
        shell: /bin/bash
        sudo: ALL=(ALL) NOPASSWD:ALL
        lock_passwd: False
      final_message: "The system is finally up, after $UPTIME seconds"
  # Настройки ресурсов ВМ.
  cpu:
    # Количество ядер ЦП.
    cores: 1
    # Запросить 10% процессорного времени одного физического ядра.
    coreFraction: 10%
  memory:
    # Объем оперативной памяти.
    size: 1Gi
  # Список дисков и образов, используемых в ВМ.
  blockDeviceRefs:
    # Порядок дисков и образов в данном блоке определяет приоритет загрузки.
    - kind: VirtualDisk
      name: linux-vm-root
EOF
```

Проверьте состояние виртуальной машины после создания:

```bash
d8 k get vm linux-vm
```

Пример вывода:

```txt
NAME        PHASE     NODE           IPADDRESS     AGE
linux-vm   Running   virtlab-pt-2   10.66.10.12   11m
```

После создания виртуальная машина автоматически получит IP-адрес из диапазона, указанного в настройках модуля (блок `virtualMachineCIDRs`).

### Жизненный цикл виртуальной машины

Виртуальная машина (ВМ) проходит через несколько этапов своего существования — от создания до удаления. Эти этапы называются фазами и отражают текущее состояние ВМ. Чтобы понять, что происходит с ВМ, нужно проверить её статус (поле `.status.phase`), а для более детальной информации — блок `.status.conditions`. Ниже описаны все основные фазы жизненного цикла ВМ, их значение и особенности.

![](./images/vm-lifecycle.ru.png)

- `Pending` - ожидание готовности ресурсов

    ВМ только что создана, перезапущена или запущена после остановки и ожидает готовности необходимых ресурсов (дисков, образов, ip-адресов и т.д.).
    - Возможные проблемы:
      - не готовы зависимые ресурсы: диски, образы, классы ВМ, секрет со сценарием начальной конфигурации и пр.
    - Диагностика: В `.status.conditions` стоит обратить внимание на условия `*Ready`. По ним можно определить, что блокирует переход к следующей фазе, например, ожидание готовности дисков (BlockDevicesReady) или класса ВМ (VirtualMachineClassReady).

      ``` bash
      d8 k get vm <vm-name> -o json | jq '.status.conditions[] | select(.type | test(".*Ready"))'
      ```

- `Starting` - запуск виртуальной машины

    Все зависимые ресурсы ВМ - готовы, и система пытается запустить ВМ на одном из узлов кластера.
    - Возможные проблемы:
      - Нет подходящего узла для запуска.
      - На подходящих узлах недостаточно CPU или памяти.
      - Превышены квоты неймспейса или проекта.
    - Диагностика:
      - Если запуск затягивается, проверьте `.status.conditions`, условие `type: Running`

      ``` bash
      d8 k get vm <vm-name> -o json | jq '.status.conditions[] | select(.type=="Running")'
      ```

- `Running` - виртуальная машина запущена

    ВМ успешно запущена и работает.
    - Особенности:
      - При установленном в гостевой системе qemu-guest-agent, условие `AgentReady` будет истинно,а в `.status.guestOSInfo` будет отображена информация о запущенной гостевой ОС.
      - Условие `type: FirmwareUpToDate, status: False` информирует о том, что прошивку ВМ требуется обновить.
      - Условие `type: ConfigurationApplied, status: False` информирует о том, что конфигурация ВМ не применена для запущенной ВМ.
      - Условие `type: SizingPolicyMatched, status: False` информирует о том, что конфигурация ресурсов ВМ не соответствует требованиям политики сайзинга для используемого VirtualMachineClass и требует привести эти настройки в соответствие иначе сохранить новые изменения в конфигурации ВМ будет невозможно.
      - Условие `type: AwaitingRestartToApplyConfiguration, status: True` отображает информацию о необходимости выполнить вручную перезагрузку ВМ, т.к. некоторые изменения конфигурации невозможно применить без перезагрузки ВМ.
    - Возможные проблемы:
      - Внутренний сбой в работе ВМ или гипервизора.
    - Диагностика:
      - Проверьте `.status.conditions`, условие `type: Running`

      ``` bash
      d8 k get vm <vm-name> -o json | jq '.status.conditions[] | select(.type=="Running")'
      ```

- `Stopping` - ВМ останавливается или перезагружается

- `Stopped` - ВМ остановлена и не потребляет вычислительные ресурсы

- `Terminating` - ВМ удаляется.

    Данная фаза необратима. Все связанные с ВМ ресурсы освобождаются, но не удаляются автоматически.

- `Migrating` - живая миграция ВМ

    ВМ переносится на другой узел кластера (живая миграция).
    - Особенности:
      - Миграция ВМ поддерживается только для нелокальных дисков, условие `type: Migratable` отображает информацию о том может ли ВМ мигрировать или нет.
    - Возможные проблемы:
      - Несовместимость процессорных инструкций (при использовании типов процессоров host или host-passthrough).
      - Различие версиях ядер на узлах гипервизоров.
      - На подходящих узлах недостаточно CPU или памяти.
      - Превышены квоты неймспейса или проекта.
    - Диагностика:
      - Проверьте `.status.conditions` условие `type: Migrating`, а также блок `.status.migrationState`

    ```bash
    d8 k get vm <vm-name> -o json | jq '.status | {condition: .conditions[] | select(.type=="Migrating"), migrationState}'
    ```

Условие `type: SizingPolicyMatched, status: False` отображает несоответствие конфигурации ресурсов политике сайзинга используемого VirtualMachineClass. При нарушении политики сохранить параметры ВМ без приведения ресурсов в соответствие политике - невозможно.

Условия отображают информацию о состоянии ВМ, а также на возникающие проблемы. Понять, что не так с ВМ можно путем их анализа:

```bash
d8 k get vm fedora -o json | jq '.status.conditions[] | select(.message != "")'
```

### Настройка CPU и coreFraction

При создании виртуальной машины вы можете настроить, сколько процессорных ресурсов она будет использовать, с помощью параметров `cores` и `coreFraction`. Эти параметры определяют, сколько виртуальных ядер "видит" ВМ и какую минимальную долю их мощности она гарантированно получит.

{{< alert level="warning">}}
Доступные значения `coreFraction` могут быть определены в ресурсе VirtualMachineClass для заданного диапазона ядер (`cores`), в этом случае допускается использовать только эти значения.
{{< /alert >}}

Параметр `cores` задает количество виртуальных процессорных ядер, доступных ВМ. Например, если указать `cores: 2`, ВМ будет работать так, словно у нее два ядра. Параметр `coreFraction` указывает минимальную гарантированную долю мощности каждого ядра в процентах. Например, при `coreFraction: 10%` ВМ всегда получит не менее 10% производительности каждого ядра, даже если узел (физический сервер) сильно загружен. Если же на узле есть свободные ресурсы, ВМ может использовать до 100% мощности каждого ядра, что позволяет достичь максимальной производительности.

Рассмотрим пример конфигурации:

```yaml
spec:
  cpu:
    cores: 2
    coreFraction: 10%
```

В этом случае ВМ "видит" два виртуальных ядра и гарантированно получает мощность, эквивалентную 20% одного физического ядра (0.2 CPU). Если на узле есть неиспользуемые ресурсы, ВМ может задействовать до 100% мощности двух ядер (2 CPU).

{{< alert level="info">}}
Этот подход напоминает переподписку процессорных ресурсов, где ВМ может использовать больше мощности, чем зарезервировано, если ресурсы доступны. Однако в отличие от традиционной переподписки, где нет гарантий минимальной производительности, coreFraction обеспечивает ВМ предсказуемый минимум ресурсов. Это делает работу ВМ стабильной даже при высокой нагрузке на кластер.
{{< /alert >}}

Параметры `cores` и `coreFraction` учитываются при планировании размещения ВМ на узлах. Гарантированная мощность (минимальная доля каждого ядра) учитывается при выборе узла, чтобы он мог обеспечить необходимую производительность для всех ВМ. Если узел не располагает достаточными ресурсами для выполнения гарантий, ВМ не будет запущена на этом узле.

![](./images/vm-corefraction.ru.png)

### Настройка ресурсов виртуальной машины и политика сайзинга

Политика сайзинга в VirtualMachineClass, заданная в разделе `.spec.sizingPolicies`, определяет правила настройки ресурсов виртуальной машины, включая количество ядер, объём памяти и долю использования ядер (`coreFraction`). Эта политика не является обязательной. Если она отсутствует, можно указывать произвольные значения для ресурсов без строгих требований. Однако если политика сайзинга присутствует, конфигурация виртуальной машины должна строго ей соответствовать. В противном случае сохранение конфигурации будет невозможно.

Политика делит количество ядер (`cores`) на диапазоны, например, 1–4 ядра или 5–8 ядер. Для каждого диапазона указывается, сколько памяти можно выделить (`memory`) и какие значения `coreFraction` разрешены.

Если конфигурация ВМ (ядра, память или coreFraction) не соответствует политике, в статусе появится условие `type: SizingPolicyMatched, status: False`.

Если политику в VirtualMachineClass изменят со временем, существующим ВМ придётся подстраиваться под новые правила, иначе их настройки нельзя будет сохранить.

Например:

```yaml
spec:
  sizingPolicies:
    - cores:
        min: 1
        max: 4
      memory:
        min: 1Gi
        max: 8Gi
      coreFractions: [5, 10, 20, 50, 100]
    - cores:
        min: 5
        max: 8
      memory:
        min: 5Gi
        max: 16Gi
      coreFractions: [20, 50, 100]
```

Если ВМ использует 2 ядра, она попадает в диапазон 1–4 ядра. Тогда память можно выбрать от 1 ГБ до 8 ГБ, а coreFraction — только 5%, 10%, 20%, 50% или 100%. Для 6 ядер — диапазон 5–8 ядер, где память от 5 ГБ до 16 ГБ, а coreFraction — 20%, 50% или 100%.

### Автоматическая конфигурация топологии CPU

Топология CPU виртуальной машины (ВМ) определяет, как ядра процессора распределяются по сокетам. Это важно для обеспечения оптимальной производительности и совместимости с приложениями, которые могут зависеть от конфигурации процессора. В конфигурации ВМ вы задаете только общее количество ядер процессора, а топология (количество сокетов и ядер в каждом сокете) рассчитывается автоматически на основе этого значения.

Количество ядер процессора указывается в конфигурации ВМ следующим образом:

```yaml
spec:
  cpu:
    cores: 1
```

Далее система автоматически определяет топологию в зависимости от заданного числа ядер. Правила расчета зависят от диапазона количества ядер и описаны ниже.

- Если количество ядер от 1 до 16 (1 ≤ `.spec.cpu.cores` ≤ 16):
  - Используется 1 сокет.
  - Количество ядер в сокете равно заданному значению.
  - Шаг изменения: 1 (можно увеличивать или уменьшать количество ядер по одному).
  - Допустимые значения: любое целое число от 1 до 16 включительно.
  - Пример: Если задано `.spec.cpu.cores` = 8, то топология: 1 сокет с 8 ядрами.
- Если количество ядер от 17 до 32 (16 < `.spec.cpu.cores` ≤ 32):
  - Используется 2 сокета.
  - Ядра равномерно распределяются между сокетами (количество ядер в каждом сокете одинаковое).
  - Шаг изменения: 2 (общее количество ядер должно быть четным).
  - Допустимые значения: 18, 20, 22, 24, 26, 28, 30, 32.
  - Ограничения: минимум 9 ядер в сокете, максимум 16 ядер в сокете.
  - Пример: Если задано `.spec.cpu.cores` = 20, то топология: 2 сокета по 10 ядер каждый.
- Если количество ядер от 33 до 64 (32 < `.spec.cpu.cores` ≤ 64):
  - Используется 4 сокета.
  - Ядра равномерно распределяются между сокетами.
  - Шаг изменения: 4 (общее количество ядер должно быть кратно 4).
  - Допустимые значения: 36, 40, 44, 48, 52, 56, 60, 64.
  - Ограничения: минимум 9 ядер в сокете, максимум 16 ядер в сокете.
  - Пример: Если задано `.spec.cpu.cores` = 40, то топология: 4 сокета по 10 ядер каждый.
- Если количество ядер больше 64 (`.spec.cpu.cores` > 64):
  - Используется 8 сокетов.
  - Ядра равномерно распределяются между сокетами.
  - Шаг изменения: 8 (общее количество ядер должно быть кратно 8).
  - Допустимые значения: 72, 80, 88, 96 и так далее до 248
  - Ограничения: минимум 9 ядер в сокете.
  - Пример: Если задано `.spec.cpu.cores` = 80, то топология: 8 сокетов по 10 ядер каждый.

Шаг изменения указывает, на сколько можно увеличивать или уменьшать общее количество ядер, чтобы они равномерно распределялись по сокетам.

Максимально возможное количество ядер - 248.

Текущая топология ВМ (количество сокетов и ядер в каждом сокете) отображается в статусе ВМ в следующем формате:

```yaml
status:
  resources:
    cpu:
      coreFraction: 10%
      cores: 1
      requestedCores: "1"
      runtimeOverhead: "0"
      topology:
        sockets: 1
        coresPerSocket: 1
```

### Агент гостевой ОС

Для повышения эффективности управления ВМ рекомендуется установить QEMU Guest Agent — инструмент, который обеспечивает взаимодействие между гипервизором и операционной системой внутри ВМ.

Чем поможет агент?

- Обеспечит создание консистентных снимков дисков и ВМ.
- Позволит получать информацию о работающей ОС, которая будет отражена в статусе ВМ.
  Пример:

  ```yaml
  status:
    guestOSInfo:
      id: fedora
      kernelRelease: 6.11.4-301.fc41.x86_64
      kernelVersion: '#1 SMP PREEMPT_DYNAMIC Sun Oct 20 15:02:33 UTC 2024'
      machine: x86_64
      name: Fedora Linux
      prettyName: Fedora Linux 41 (Cloud Edition)
      version: 41 (Cloud Edition)
      versionId: "41"
  ```

- Позволит отслеживать, что ОС действительно загрузилась:

  ```bash
  d8 k get vm -o wide
  ```

  Пример вывода (колонка `AGENT`):
  ```console
  NAME     PHASE     CORES   COREFRACTION   MEMORY   NEED RESTART   AGENT   MIGRATABLE   NODE           IPADDRESS    AGE
  fedora   Running   6       5%             8000Mi   False          True    True         virtlab-pt-1   10.66.10.1   5d21h
  ```

Как установить QEMU Guest Agent:

Для Debian-based ОС:

```bash
sudo apt install qemu-guest-agent
```

Для Centos-based ОС:

```bash
sudo yum install qemu-guest-agent
```

Запуск службы агента:

```bash
sudo systemctl enable --now qemu-guest-agent
```

### Подключение к виртуальной машине

Для подключения к виртуальной машине доступны следующие способы:

- протокол удаленного управления (например SSH), который должен быть предварительно настроен на виртуальной машине.
- серийная консоль (serial console).
- протокол VNC.

Пример подключения к виртуальной машине с использованием серийной консоли:

```bash
d8 v console linux-vm
```

Пример вывода:

```txt
Successfully connected to linux-vm console. The escape sequence is ^]
linux-vm login: cloud
Password: cloud
```

Нажмите `Ctrl+]` для завершения работы с серийной консолью.

Пример команды для подключения по VNC:

```bash
d8 v vnc linux-vm
```

Пример команды для подключения по SSH:

```bash
d8 v ssh cloud@linux-vm --local-ssh
```

### Политика запуска и управление состоянием виртуальной машины

Политика запуска виртуальной машины предназначена для автоматизированного управления состоянием виртуальной машины. Определяется она в виде параметра `.spec.runPolicy` в спецификации виртуальной машины. Поддерживаются следующие политики:

- `AlwaysOnUnlessStoppedManually` - (по умолчанию) после создания ВМ всегда находится в запущенном состоянии. В случае сбоев работа ВМ восстанавливается автоматически. Остановка ВМ возможно только путем вызова команды `d8 v stop` или создания соответствующей операции.
- `AlwaysOn` - после создания ВМ всегда находится в работающем состоянии, даже в случае ее выключения средствами ОС. В случае сбоев работа ВМ восстанавливается автоматически.
- `Manual` - после создания состоянием ВМ управляет пользователь вручную с использованием команд или операций.
- `AlwaysOff` - после создания ВМ всегда находится в выключенном состоянии. Возможность включения ВМ через команды\операции - отсутствует.

Состоянием виртуальной машины можно управлять с помощью следующих методов:

- Создание ресурса `VirtualMachineOperation` (`vmop`).
- Использование утилиты `d8` с соответствующей подкомандой.

Ресурс `VirtualMachineOperation` декларативно определяет императивное действие, которое должно быть выполнено на виртуальной машине. Это действие применяется к виртуальной машине сразу после создания соответствующего `vmop`. Действие применяется к виртуальной машине один раз.

Пример операции для выполнения перезагрузки виртуальной машины с именем `linux-vm`:

```yaml
d8 k create -f - <<EOF
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualMachineOperation
metadata:
  generateName: restart-linux-vm-
spec:
  virtualMachineName: linux-vm
  # Тип применяемой операции = применяемая операция.
  type: Restart
EOF
```

Посмотреть результат действия можно с использованием команды:

```bash
d8 k get virtualmachineoperation
# или
d8 k get vmop
```

Аналогичное действие можно выполнить с использованием утилиты `d8`:

```bash
d8 v restart linux-vm
```

Перечень возможных операций приведен в таблице ниже:

| d8             | vmop type | Действие                      |
| -------------- | --------- | ----------------------------- |
| `d8 v stop`    | `Stop`    | Остановить ВМ                 |
| `d8 v start`   | `Start`   | Запустить ВМ                  |
| `d8 v restart` | `Restart` | Перезапустить ВМ              |
| `d8 v  evict`  | `Evict`   | Мигрировать ВМ на другой узел |

### Изменение конфигурации виртуальной машины

Конфигурацию виртуальной машины можно изменять в любое время после создания ресурса `VirtualMachine`. Однако то, как эти изменения будут применены, зависит от текущей фазы виртуальной машины и характера внесённых изменений.

Изменения в конфигурацию виртуальной машины можно внести с использованием следующей команды:

```bash
d8 k edit vm linux-vm
```

Если виртуальная машина находится в выключенном состоянии (`.status.phase: Stopped`), внесённые изменения вступят в силу сразу после её запуска.

Если виртуальная машина работает (`.status.phase: Running`), то способ применения изменений зависит от их типа:

| Блок конфигурации                       | Как применяется                              |
| --------------------------------------- | ---------------------------------------------|
| `.metadata.labels`                      | Сразу                                        |
| `.metadata.annotations`                 | Сразу                                        |
| `.spec.liveMigrationPolicy`             | Сразу                                        |
| `.spec.runPolicy`                       | Сразу                                        |
| `.spec.disruptions.restartApprovalMode` | Сразу                                        |
| `.spec.affinity`                        | EE, SE+ : Сразу, CE: Требуется перезапуск ВМ |
| `.spec.nodeSelector`                    | EE, SE+ : Сразу, CE: Требуется перезапуск ВМ |
| `.spec.*`                               | Требуется перезапуск ВМ                      |

Рассмотрим пример изменения конфигурации виртуальной машины:

Предположим, мы хотим изменить количество ядер процессора. В данный момент виртуальная машина запущена и использует одно ядро, что можно подтвердить, подключившись к ней через серийную консоль и выполнив команду `nproc`.

```bash
d8 v ssh cloud@linux-vm --local-ssh --command "nproc"
```

Пример вывода:

```txt
1
```

Примените следующий патч к виртуальной машине, чтобы изменить количество ядер с 1 на 2.

```bash
d8 k patch vm linux-vm --type merge -p '{"spec":{"cpu":{"cores":2}}}'
```

Пример вывода:

```txt
# virtualmachine.virtualization.deckhouse.io/linux-vm patched
```

Изменения в конфигурации внесены, но ещё не применены к виртуальной машине. Проверьте это, повторно выполнив:

```bash
d8 v ssh cloud@linux-vm --local-ssh --command "nproc"
```

Пример вывода:

```txt
1
```

Для применения этого изменения необходим перезапуск виртуальной машины. Выполните следующую команду, чтобы увидеть изменения, ожидающие применения (требующие перезапуска):

```bash
d8 k get vm linux-vm -o jsonpath="{.status.restartAwaitingChanges}" | jq .
```

Пример вывода:

```json
[
  {
    "currentValue": 1,
    "desiredValue": 2,
    "operation": "replace",
    "path": "cpu.cores"
  }
]
```

Выполните команду:

```bash
d8 k get vm linux-vm -o wide
```

Пример вывода:

```txt
NAME        PHASE     CORES   COREFRACTION   MEMORY   NEED RESTART   AGENT   MIGRATABLE   NODE           IPADDRESS     AGE
linux-vm    Running   2       100%           1Gi      True           True    True         virtlab-pt-1   10.66.10.13   5m16s
```

В колонке `NEED RESTART` мы видим значение `True`, а это значит что для применения изменений требуется перезагрузка.

Выполните перезагрузку виртуальной машины:

```bash
d8 v restart linux-vm
```

После перезагрузки изменения будут применены и блок `.status.restartAwaitingChanges` будет пустой.

Выполните команду для проверки:

```bash
d8 v ssh cloud@linux-vm --local-ssh --command "nproc"
```

Пример вывода:

```txt
2
```

Порядок применения изменений виртуальной машины через «ручной» рестарт является поведением по умолчанию. Если есть необходимость применять внесенные изменения сразу и автоматически, для этого нужно изменить политику применения изменений:

```yaml
spec:
  disruptions:
    restartApprovalMode: Automatic
```

### Сценарии начальной инициализации

Сценарии начальной инициализации предназначены для первичной конфигурации виртуальной машины при её запуске.

В качестве сценариев начальной инициализации поддерживаются:

- [CloudInit](https://cloudinit.readthedocs.io).
- [Sysprep](https://learn.microsoft.com/ru-ru/windows-hardware/manufacture/desktop/sysprep--system-preparation--overview).

Сценарий CloudInit можно встраивать непосредственно в спецификацию виртуальной машины, но этот сценарий ограничен максимальной длиной в 2048 байт:

```yaml
spec:
  provisioning:
    type: UserData
    userData: |
      #cloud-config
      package_update: true
      ...
```

При более длинных сценариях и/или наличия приватных данных, сценарий начальной инициализации виртуальной машины может быть создан в ресурсе Secret. Пример ресурса Secret со сценарием CloudInit приведен ниже:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: cloud-init-example
data:
  userData: <base64 data>
type: provisioning.virtualization.deckhouse.io/cloud-init
```

фрагмент конфигурации виртуальной машины с при использовании скрипта начальной инициализации CloudInit хранящегося в ресурсе Secret:

```yaml
spec:
  provisioning:
    type: UserDataRef
    userDataRef:
      kind: Secret
      name: cloud-init-example
```

Примечание: Значение поля `.data.userData` должно быть закодировано в формате Base64.

Для конфигурирования виртуальных машин под управлением ОС Windows с использованием Sysprep, поддерживается только вариант с ресурсом Secret.

Пример ресурса Secret с сценарием Sysprep приведен ниже:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: sysprep-example
data:
  unattend.xml: <base64 data>
type: provisioning.virtualization.deckhouse.io/sysprep
```

Примечание: Значение поля `.data.unattend.xml` должно быть закодировано в формате Base64.

фрагмент конфигурации виртуальной машины с использованием скрипта начальной инициализации Sysprep в ресурсе Secret:

```yaml
spec:
  provisioning:
    type: SysprepRef
    sysprepRef:
      kind: Secret
      name: sysprep-example
```

### Размещение ВМ по узлам

Для управления размещением виртуальных машин по узлам можно использовать следующие подходы:

- Простое связывание по меткам (`nodeSelector`).
- Предпочтительное связывание (`Affinity`).
- Избежание совместного размещения (`AntiAffinity`).

{{< alert level="info" >}}
Параметры размещения виртуальных машин можно изменить в реальном времени (доступно только в Enterprise-редакции). Однако, если новые параметры размещения не совпадают с текущими, виртуальная машина будет перемещена на узлы, соответствующие новым требованиям.
{{< /alert >}}

#### Простое связывание по меткам (nodeSelector)

`nodeSelector` — это простейший способ контролировать размещение виртуальных машин, используя набор меток. Он позволяет задать, на каких узлах могут запускаться виртуальные машины, выбирая узлы с необходимыми метками.

```yaml
spec:
  nodeSelector:
    disktype: ssd
```

![](images/placement-nodeselector.ru.png)

В этом примере виртуальная машина будет размещена только на узлах, которые имеют метку `disktype` со значением `ssd`.

#### Предпочтительное связывание (Affinity)

`Affinity` предоставляет более гибкие и мощные инструменты по сравнению с `nodeSelector`. Он позволяет задавать «предпочтения» и «обязательности» для размещения виртуальных машин. `Affinity` поддерживает два вида: `nodeAffinity` и `virtualMachineAndPodAffinity`.

`nodeAffinity` позволяет определять на каких узлах может быть запущена виртуальная машина с помощью выражений меток и может быть мягким (preferred) или жестким (required).

Пример использования nodeAffinity:

```yaml
spec:
  affinity:
    nodeAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
        nodeSelectorTerms:
          - matchExpressions:
              - key: disktype
                operator: In
                values:
                  - ssd
```

![](images/placement-node-affinity.ru.png)

В этом примере виртуальная машина будет размещена только на узлах, которые имеют метку `disktype` со значением `ssd`.

`virtualMachineAndPodAffinity` управляет размещением виртуальных машин относительно других виртуальных машин. Он позволяет задавать предпочтение размещения виртуальных машин на тех же узлах, где уже запущены определенные виртуальные машины.

Пример:

```yaml
spec:
  affinity:
    virtualMachineAndPodAffinity:
      preferredDuringSchedulingIgnoredDuringExecution:
        - weight: 1
          podAffinityTerm:
            labelSelector:
              matchLabels:
                server: database
            topologyKey: "kubernetes.io/hostname"
```

![](images/placement-vm-affinity.ru.png)

В этом примере виртуальная машина будет размещена, если будет такая возможность (тк используется preferred) только на узлах на которых присутствует виртуальная машина с меткой server и значением database.

#### Избежание совместного размещения (AntiAffinity)

`AntiAffinity` — это противоположность `Affinity`, которая позволяет задавать требования для избегания размещения виртуальных машин на одних и тех же узлах. Это полезно для распределения нагрузки или обеспечения отказоустойчивости.

Термины `Affinity` и `AntiAffinity` применимы только к отношению между виртуальными машинами. Для узлов используемые привязки называются `nodeAffinity`. В `nodeAffinity` нет отдельного антитеза, как в случае с `virtualMachineAndPodAffinity`, но можно создать противоположные условия, задав отрицательные операторы в выражениях меток: чтобы акцентировать внимание на исключении определенных узлов, можно воспользоваться `nodeAffinity` с оператором, таким как `NotIn`.

Пример использования `virtualMachineAndPodAntiAffinity`:

```yaml
spec:
  affinity:
    virtualMachineAndPodAntiAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
        - labelSelector:
            matchLabels:
              server: database
          topologyKey: "kubernetes.io/hostname"
```

![](images/placement-vm-antiaffinity.ru.png)

В данном примере создаваемая виртуальная машина не будет размещена на одном узле с виртуальной машиной с меткой server: database.

### Статические и динамические блочные устройства

Блочные устройства можно разделить на два типа по способу их подключения: статические и динамические (hotplug).

Блочные устройства и их особенности представлены в таблице:

| Тип блочного устройства | Комментарий                                                     |
| ----------------------- | --------------------------------------------------------------- |
| `VirtualImage`          | подключается в режиме для чтения, или как cdrom для iso-образов |
| `ClusterVirtualImage`   | подключается в режиме для чтения, или как cdrom для iso-образов |
| `VirtualDisk`           | подключается в режиме для чтения и записи                       |

#### Статические блочные устройства

Статические блочные устройства указываются в спецификации виртуальной машины в блоке `.spec.blockDeviceRefs` в виде списка. Порядок устройств в этом списке определяет последовательность их загрузки. Таким образом, если диск или образ указан первым, загрузчик сначала попробует загрузиться с него. Если это не удастся, система перейдет к следующему устройству в списке и попытается загрузиться с него. И так далее до момента обнаружения первого загрузчика.

Изменение состава и порядка устройств в блоке `.spec.blockDeviceRefs` возможно только с перезагрузкой виртуальной машины.

Фрагмент конфигурации VirtualMachine со статически подключенными диском и проектным образом:

```yaml
spec:
  blockDeviceRefs:
    - kind: VirtualDisk
      name: <virtual-disk-name>
    - kind: VirtualImage
      name: <virtual-image-name>
```

#### Динамические блочные устройства

Динамические блочные устройства можно подключать и отключать от виртуальной машины, находящейся в запущенном состоянии, без необходимости её перезагрузки.

Для подключения динамических блочных устройств используется ресурс `VirtualMachineBlockDeviceAttachment` (`vmbda`).

Создайте ресурс, который подключит пустой диск blank-disk к виртуальной машине linux-vm:

```yaml
d8 k apply -f - <<EOF
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualMachineBlockDeviceAttachment
metadata:
  name: attach-blank-disk
spec:
  blockDeviceRef:
    kind: VirtualDisk
    name: blank-disk
  virtualMachineName: linux-vm
EOF
```

После создания `VirtualMachineBlockDeviceAttachment` может находиться в следующих состояниях (фазах):

- `Pending` - ожидание готовности всех зависимых ресурсов.
- `InProgress` - идет процесс подключения устройства.
- `Attached` - устройство подключено.

Диагностика проблем с ресурсом осуществляется путем анализа информации в блоке `.status.conditions`.

Проверьте состояние вашего ресурса:

```bash
d8 k get vmbda attach-blank-disk
```

Пример вывода:

```txt
NAME                PHASE      VIRTUAL MACHINE NAME   AGE
attach-blank-disk   Attached   linux-vm              3m7s
```

Подключитесь к виртуальной машине и удостоверитесь, что диск подключен:

```bash
d8 v ssh cloud@linux-vm --local-ssh --command "lsblk"
```

Пример вывода:

```txt
NAME    MAJ:MIN RM  SIZE RO TYPE MOUNTPOINTS
sda       8:0    0   10G  0 disk <--- статично подключенный диск linux-vm-root
|-sda1    8:1    0  9.9G  0 part /
|-sda14   8:14   0    4M  0 part
`-sda15   8:15   0  106M  0 part /boot/efi
sdb       8:16   0    1M  0 disk <--- cloudinit
sdc       8:32   0 95.9M  0 disk <--- динамически подключенный диск blank-disk
```

Для отключения диска от виртуальной машины удалите ранее созданный ресурс:

```bash
d8 k delete vmbda attach-blank-disk
```

Подключение образов, осуществляется по аналогии. Для этого в качестве `kind` указать VirtualImage или ClusterVirtualImage и имя образа:

```yaml
d8 k apply -f - <<EOF
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualMachineBlockDeviceAttachment
metadata:
  name: attach-ubuntu-iso
spec:
  blockDeviceRef:
    kind: VirtualImage # или ClusterVirtualImage
    name: ubuntu-iso
  virtualMachineName: linux-vm
EOF
```

### Публикация виртуальных машин с использованием сервисов

Достаточно часто возникает необходимость сделать так, чтобы доступ к этим виртуальным машинам был возможен извне, например, для публикации каких-либо сервисов или удалённого администрирования. Для этих целей мы можем использовать сервисы, которые обеспечивают маршрутизацию трафика из внешней сети к внутренним ресурсам кластера. Рассмотрим несколько вариантов.

Предварительно, проставьте на ранее созданной вм следующие лейблы:

```bash
d8 k label vm linux-vm app=nginx
```

Пример вывода:

```txt
virtualmachine.virtualization.deckhouse.io/linux-vm labeled
```

#### Публикация сервисов виртуальной машины с использованием сервиса с типом NodePort

Сервис `NodePort` открывает определённый порт на всех узлах кластера, перенаправляя трафик на заданный внутренний порт сервиса.

Создайте следующий сервис:

```yaml
d8 k apply -f - <<EOF
apiVersion: v1
kind: Service
metadata:
  name: linux-vm-nginx-nodeport
spec:
  type: NodePort
  selector:
    # Лейбл по которому сервис определяет на какую виртуальную машину направлять трафик.
    app: nginx
  ports:
    - protocol: TCP
      port: 80
      targetPort: 80
      nodePort: 31880
EOF
```

![](images/lb-nodeport.ru.png)

В данном примере будет создан сервис с типом `NodePort`, который открывает внешний порт 31880 на всех узлах вашего кластера. Этот порт будет направлять входящий трафик на внутренний порт 80 виртуальной машины, где запущено приложение Nginx.

#### Публикация сервисов виртуальной машины с использованием сервиса с типом LoadBalancer

При использовании типа сервиса `LoadBalancer` кластер создаёт внешний балансировщик нагрузки, который распределит входящий трафик по всем экземплярам вашей виртуальной машины.

```yaml
d8 k apply -f - <<EOF
apiVersion: v1
kind: Service
metadata:
  name: linux-vm-nginx-lb
spec:
  type: LoadBalancer
  selector:
    # Лейбл по которому сервис определяет на какую виртуальную машину направлять трафик
    app: nginx
  ports:
    - protocol: TCP
      port: 80
      targetPort: 80
EOF
```

![](images/lb-loadbalancer.ru.png)

#### Публикация сервисов виртуальной машины с использованием Ingress

`Ingress` позволяет управлять входящими HTTP/HTTPS запросами и маршрутизировать их к различным серверам в рамках вашего кластера. Это наиболее подходящий метод, если вы хотите использовать доменные имена и SSL-терминацию для доступа к вашим виртуальным машинам.

Для публикации сервиса виртуальной машины через `Ingress` необходимо создать следующие ресурсы:

Внутренний сервис для связки с `Ingress`. Пример:

```yaml
d8 k apply -f - <<EOF
apiVersion: v1
kind: Service
metadata:
  name: linux-vm-nginx
spec:
  selector:
    # лейбл по которому сервис определяет на какую виртуальную машину направлять трафик
    app: nginx
  ports:
    - protocol: TCP
      port: 80
      targetPort: 80
EOF
```

И ресурс `Ingress` для публикации. Пример:

```yaml
d8 k apply -f - <<EOF
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: linux-vm
spec:
  rules:
    - host: linux-vm.example.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: linux-vm-nginx
                port:
                  number: 80
EOF
```

![](images/lb-ingress.ru.png)

### Живая миграция виртуальной машины

Живая миграция виртуальных машин (ВМ) — это процесс перемещения работающей ВМ с одного физического узла на другой без её отключения. Эта функция играет ключевую роль в управлении виртуализованной инфраструктурой, обеспечивая непрерывность работы приложений во время технического обслуживания, балансировки нагрузки или обновлений.

#### Как работает живая миграция

Процесс живой миграции включает несколько этапов:

1. **Создание нового экземпляра ВМ**

   На целевом узле создаётся новая ВМ в приостановленном состоянии. Её конфигурация (процессор, диски, сеть) копируется с исходного узла.

2. **Первичная передача памяти**

   Вся оперативная память ВМ копируется на целевой узел по сети. Это называется первичной передачей.

3. **Отслеживание изменений (Dirty Pages)**

    Пока память передаётся, ВМ продолжает работать на исходном узле и может изменять некоторые страницы памяти. Такие страницы называются «грязными» (dirty pages), и гипервизор их помечает.

4. **Итеративная синхронизация**

   После первичной передачи начинается повторная отправка только изменённых страниц. Этот процесс повторяется в несколько циклов:
   - Чем выше нагрузка на ВМ, тем больше «грязных» страниц появляется, и тем дольше длится миграция.
   - При хорошей пропускной способности сети объём несинхронизированных данных постепенно уменьшается.

5. **Финальная синхронизация и переключение**

    Когда количество «грязных» страниц становится минимальным, ВМ на исходном узле приостанавливается (обычно на 100 миллисекунд):
    - Оставшиеся изменения памяти передаются на целевой узел.
    - Состояние процессора, устройств и открытых соединений синхронизируется.
    - ВМ запускается на новом узле, а исходная копия удаляется.

![](./images/migration.ru.png)

{{< alert level="warning" >}}
Cкорость сети играет важную роль. Если пропускная способность низкая, итераций становится больше, а время простоя ВМ может увеличиться. В худшем случае миграция может вообще не завершиться.
{{< /alert >}}

#### Механизм AutoConverge

Если сеть не справляется с передачей данных, а количество «грязных» страниц продолжает расти, будет полезен механизм AutoConverge. Он помогает завершить миграцию даже при низкой пропускной способности сети.

Принципы работы механизма AutoConverge:

1. **Замедление процессора ВМ**

    Гипервизор постепенно снижает частоту процессора исходной ВМ. Это уменьшает скорость появления новых «грязных» страниц. Чем выше нагрузка на ВМ, тем сильнее замедление.

2. **Ускорение синхронизации**

    Как только скорость передачи данных превышает скорость изменения памяти, запускается финальная синхронизация, и ВМ переключается на новый узел.

3. **Автоматическое завершение**

    Финальная синхронизация запускается, когда скорость передачи данных превышает скорость изменения памяти.

AutoConverge — это своего рода «страховка», которая гарантирует, что миграция завершится, даже если сеть не справляется с передачей данных. Однако замедление процессора может повлиять на производительность приложений, работающих на ВМ, поэтому его использование нужно контролировать.

#### Настройка политики миграции

Для настройки поведения миграции используйте параметр `.spec.liveMigrationPolicy` в конфигурации ВМ. Допустимые значения параметра:

- `AlwaysSafe` — Миграция всегда выполняется без замедления процессора (AutoConverge не используется). Подходит для случаев, когда важна максимальная производительность ВМ, но требует высокой пропускной способности сети.
- `PreferSafe` — (используется в качестве политики по умолчанию) Миграция выполняется без замедления процессора (AutoConverge не используется). Однако можно запустить миграцию с замедлением процессора, используя ресурс VirtualMachineOperation с параметрами `type=Evict` и `force=true`.
- `AlwaysForced` — Миграция всегда использует AutoConverge, то есть процессор замедляется при необходимости. Это гарантирует завершение миграции даже при плохой сети, но может снизить производительность ВМ.
- `PreferForced` — Миграция использует AutoConverge, то есть процессор замедляется при необходимости.  Однако можно запустить миграцию без замедления процессора, используя ресурс VirtualMachineOperation с параметрами `type=Evict` и `force=false`.

#### Виды миграции

Миграция может осуществляться пользователем вручную, либо автоматически при следующих системных событиях:

- Обновлении «прошивки» виртуальной машины.
- Перераспределение нагрузки в кластере.
- Перевод узла в режим технического обслуживания (Drain узла)
- При изменении [параметров размещения ВМ](#размещение-вм-по-узлам) (не доступно в Community-редакции).

Триггером к живой миграции является появление ресурса `VirtualMachineOperations` с типом `Evict`.

В таблице приведены префиксы названия ресурса `VirtualMachineOperations` с типом `Evict`, создаваемые для живых миграций вызванных системными событиями:

| Вид системного события           | Префикс имени ресурса  |
|----------------------------------|------------------------|
| Обновлении «прошивки»            | firmware-update-*      |
| Перераспределение нагрузки       | evacuation-*           |
| Drain узла                       | evacuation-*           |
| Изменение параметров размещения  | nodeplacement-update-* |

Данный ресурс может находится в следующих состояниях:

- `Pending` - ожидается выполнение операции.
- `InProgress` - живая миграция выполняется.
- `Completed` - живая миграция виртуальной машины завершилась успешно.
- `Failed` - живая миграция виртуальной машины завершилась неуспешно.

Посмотреть активные операции можно с использованием команды:

```bash
d8 k get vmop
```

Пример вывода:

```txt
NAME                    PHASE       TYPE    VIRTUALMACHINE      AGE
firmware-update-fnbk2   Completed   Evict   linux-vm            1m
```

Прервать любую живую миграцию пока она находится в фазе `Pending`, `InProgress` можно удалив соответствующий ресурс `VirtualMachineOperations`.

#### Как выполнить живую миграцию виртуальной машины с использованием `VirtualMachineOperations`.

Рассмотрим пример. Перед запуском миграции посмотрите текущий статус виртуальной машины:

```bash
d8 k get vm
```

Пример вывода:

```txt
NAME                                   PHASE     NODE           IPADDRESS     AGE
linux-vm                               Running   virtlab-pt-1   10.66.10.14   79m
```

Мы видим что на данный момент она запущена на узле `virtlab-pt-1`.

Для осуществления миграции виртуальной машины с одного узла на другой, с учетом требований к размещению виртуальной машины используется команда:

```bash
d8 v evict -n <namespace> <vm-name>
```

Выполнение данной команды приводит к созданию ресурса `VirtualMachineOperations`.

Запустить миграцию можно также создав ресурс `VirtualMachineOperations` (`vmop`) с типом `Evict` вручную:

```yaml
d8 k create -f - <<EOF
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualMachineOperation
metadata:
  generateName: evict-linux-vm-
spec:
  # Имя виртуальной машины.
  virtualMachineName: linux-vm
  # Операция для миграции.
  type: Evict
  # Разрешить замедление процессора механизмом AutoConverge, для гарантии, что миграция выполнится.
  force: true
EOF
```

Для отслеживания миграции виртуальной машины сразу после создания ресурса `vmop`, выполните команду:

```bash
d8 k get vm -w
```

Пример вывода:

```txt
NAME                                  PHASE       NODE           IPADDRESS     AGE
linux-vm                              Running     virtlab-pt-1   10.66.10.14   79m
linux-vm                              Migrating   virtlab-pt-1   10.66.10.14   79m
linux-vm                              Migrating   virtlab-pt-1   10.66.10.14   79m
linux-vm                              Running     virtlab-pt-2   10.66.10.14   79m
```

#### Живая миграция виртуальной машины при изменении параметров размещения (недоступно в CE редакции)

Рассмотрим механизм миграции на примере кластера с двумя группами узлов (`NodeGroups`): green и blue . Допустим, виртуальная машина (ВМ) изначально запущена на узле группы green , а её конфигурация не содержит ограничений на размещение.

Шаг 1. Добавление параметра размещения
Укажем в спецификации ВМ требование к размещению в группе green :

```yaml
spec:
  nodeSelector:
    node.deckhouse.io/group: green
```

После сохранения изменений ВМ продолжит работать на текущем узле, так как условие `nodeSelector` уже выполняется.

Шаг 2. Изменение группы размещения
Изменим требование на размещение в группе blue :

```yaml
spec:
  nodeSelector:
    node.deckhouse.io/group: blue
```

Теперь текущий узел (группы green) не соответствует новым условиям. Система автоматически создаст объект `VirtualMachineOperations` типа Evict, что инициирует живую миграцию ВМ на доступный узел группы blue.

Пример вывода ресурса

```txt
NAME                         PHASE       TYPE    VIRTUALMACHINE      AGE
nodeplacement-update-dabk4   Completed   Evict   linux-vm            1m
```

## IP-адреса виртуальных машин

Блок `.spec.settings.virtualMachineCIDRs` в конфигурации модуля virtualization задает список подсетей для назначения ip-адресов виртуальным машинам (общий пул ip-адресов). Все адреса в этих подсетях доступны для использования, за исключением первого (адрес сети) и последнего (широковещательный адрес).

Ресурс `VirtualMachineIPAddressLease` (`vmipl`): кластерный ресурс, который управляет арендой IP-адресов из общего пула, указанного в `virtualMachineCIDRs`.

Чтобы посмотреть список аренд IP-адресов (`vmipl`), используйте команду:

```bash
d8 k get vmipl
```

Пример вывода:

```txt
NAME             VIRTUALMACHINEIPADDRESS                             STATUS   AGE
ip-10-66-10-14   {"name":"linux-vm-7prpx","namespace":"default"}     Bound    12h
```

Ресурс `VirtualMachineIPAddress` (`vmip`): проектный/неймспейсный ресурс, который отвечает за резервирование арендованных IP-адресов и их привязку к виртуальным машинам. IP-адреса могут выделяться автоматически или по явному запросу.

По умолчанию IP-адрес виртуальной машине назначается автоматически из подсетей, определенных в модуле и закрепляется за ней до её удаления. Проверить назначенный IP-адрес можно с помощью команды:

```bash
d8 k get vmip
```

Пример вывода:

```txt
NAME             ADDRESS       STATUS     VM         AGE
linux-vm-7prpx   10.66.10.14   Attached   linux-vm   12h
```

Алгоритм автоматического присвоения IP-адреса виртуальной машине выглядит следующим образом:

- Пользователь создает виртуальную машину с именем `<vmname>`.
- Контроллер модуля автоматически создает ресурс `vmip` с именем `<vmname>-<hash>`, чтобы запросить IP-адрес и связать его с виртуальной машиной.
- Для этого `vmip` создается ресурс аренды `vmipl`, который выбирает случайный IP-адрес из общего пула.
- Как только ресурс `vmip` создан, виртуальная машина получает назначенный IP-адрес.

IP-адрес виртуальной машине назначается автоматически из подсетей, определенных в модуле, и остается закрепленным за машиной до её удаления. После удаления виртуальной машины ресурс `vmip` также удаляется, но IP-адрес временно остается закрепленным за проектом/неймспейсом и может быть повторно запрошен явно.

С полным описанием параметров конфигурации ресурсов `vmip` и `vmipl` машин можно ознакомиться по ссылкам:

- [`VirtualMachineIPAddress`](cr.html#virtualmachineipaddress).
- [`VirtualMachineIPAddressLease`](cr.html#virtualmachineipaddresslease).

### Как запросить требуемый ip-адрес?

1. Создайте ресурс `vmip`:

   ```yaml
   d8 k apply -f - <<EOF
   apiVersion: virtualization.deckhouse.io/v1alpha2
   kind: VirtualMachineIPAddress
   metadata:
     name: linux-vm-custom-ip
   spec:
     staticIP: 10.66.20.77
     type: Static
   EOF
   ```

1. Создайте новую или измените существующую виртуальную машину и в спецификации укажите требуемый ресурс `vmip` явно:

   ```yaml
   spec:
     virtualMachineIPAddressName: linux-vm-custom-ip
   ```

### Как сохранить присвоенный виртуальной машине ip-адрес?

Чтобы автоматически выданный ip-адрес виртуальной машины не удалился вместе с самой виртуальной машиной выполните следующие действия.

Получите название ресурса `vmip` для заданной виртуальной машины:

```bash
d8 k get vm linux-vm -o jsonpath="{.status.virtualMachineIPAddressName}"
```

Пример вывода:

```txt
linux-vm-7prpx
```

Удалите блоки `.metadata.ownerReferences` из найденного ресурса:

```bash
d8 k patch vmip linux-vm-7prpx --type=merge --patch '{"metadata":{"ownerReferences":null}}'
```

После удаления виртуальной машины, ресурс `vmip` сохранится и его можно будет переиспользовать снова во вновь созданной виртуальной машине:

```yaml
spec:
  virtualMachineIPAddressName: linux-vm-7prpx
```

Даже если ресурс `vmip` будет удален. Он остаётся арендованным для текущего проекта/неймспейса еще 10 минут. Поэтому существует возможность вновь его занять по запросу:

```yaml
d8 k apply -f - <<EOF
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualMachineIPAddress
metadata:
  name: linux-vm-custom-ip
spec:
  staticIP: 10.66.20.77
  type: Static
EOF
```

## Снимки

Снимки предназначены для сохранения состояния ресурса в конкретный момент времени. На данный момент времени поддерживаются снимки дисков и снимки виртуальных машин.

### Создание снимков дисков

Для создания снимков виртуальных дисков используется ресурс `VirtualDiskSnapshot` . Эти снимки могут служить источником данных при создании новых дисков, например, для клонирования или восстановления информации.

Чтобы гарантировать целостность данных, снимок диска можно создать в следующих случаях:

- Диск не подключен ни к одной виртуальной машине.
- ВМ выключена.
- ВМ запущена, но yстановлен qemu-guest-agent в гостевой ОС.
Файловая система успешно "заморожена" (операция fsfreeze).

Если консистентность данных не требуется (например, для тестовых сценариев), снимок можно создать:

- На работающей ВМ без "заморозки" файловой системы.
- Даже если диск подключен к активной ВМ.

Для этого в манифесте VirtualDiskSnapshot укажите:

```yaml
spec:
  requiredConsistency: false
```

При создании снимка требуется указать названия класса снимка томов `VolumeSnapshotClasses`, который будет использоваться для создания снимка.

Для получения списка поддерживаемых ресурсов `VolumeSnapshotClasses` выполните команду:

```bash
d8 k get volumesnapshotclasses
```

Пример вывода:

```txt
NAME                     DRIVER                                DELETIONPOLICY   AGE
csi-nfs-snapshot-class   nfs.csi.k8s.io                        Delete           34d
sds-replicated-volume    replicated.csi.storage.deckhouse.io   Delete           39d
```

Пример манифеста для создания снимка диска:

```yaml
d8 k apply -f - <<EOF
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualDiskSnapshot
metadata:
  name: linux-vm-root-snapshot
spec:
  requiredConsistency: true
  virtualDiskName: linux-vm-root
  volumeSnapshotClassName: sds-replicated-volume
EOF
```

Для просмотра списка снимков дисков, выполните следующую команду:

```bash
d k get vdsnapshot
```

Пример вывода:

```txt
NAME                   PHASE     CONSISTENT   AGE
linux-vm-root-snapshot Ready     true         3m2s
```

После создания `VirtualDiskSnapshot` может находиться в следующих состояниях (фазах):

- `Pending` - ожидание готовности всех зависимых ресурсов, требующихся для создания снимка.
- `InProgress` — идет процесс создания снимка виртуального диска.
- `Ready` — создание снимка успешно завершено, и снимок виртуального диска доступен для использования.
- `Failed` — произошла ошибка во время процесса создания снимка виртуального диска.
- `Terminating` — ресурс находится в процессе удаления.

Диагностика проблем с ресурсом осуществляется путем анализа информации в блоке `.status.conditions`.

С полным описанием параметров конфигурации ресурса `VirtualDiskSnapshot` машин можно ознакомиться [в документации ресурса](cr.html#virtualdisksnapshot).

### Восстановление дисков из снимков

Для того чтобы восстановить диск из ранее созданного снимка диска, необходимо в качестве `dataSource` указать соответствующий объект:

```yaml
d8 k apply -f - <<EOF
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualDisk
metadata:
  name: linux-vm-root
spec:
  # Настройки параметров хранения диска.
  persistentVolumeClaim:
    # Укажем размер больше чем значение .
    size: 10Gi
    # Подставьте ваше название StorageClass.
    storageClassName: i-sds-replicated-thin-r2
  # Источник из которого создается диск.
  dataSource:
    type: ObjectRef
    objectRef:
      kind: VirtualDiskSnapshot
      name: linux-vm-root-snapshot
EOF
```

### Создание снимков виртуальных машин

Для создания снимков виртуальных машин используется ресурс `VirtualMachineSnapshot`.

Снимки можно использовать для реализации следующих сценариев:
- [Восстановление ВМ на момент создания снимка](#восстановление-виртуальной-машины)
- [Создание клона ВМ / Использование снимка как шаблона для создания ВМ](#создание-клона-вм--использование-снимка-как-шаблона-для-создания-вм)

![](./images/vm-restore-clone.ru.png)

Если снимок планируется использовать как шаблон, перед его созданием выполните в гостевой ОС:

- Удаление персональных данных (файлы, пароли, история команд).
- Установку критических обновлений ОС.
- Очистку системных журналов.
- Сброс сетевых настроек.
- Удаление уникальных идентификаторов (например, через `sysprep` для Windows).
- Оптимизацию дискового пространства.
- Сброс конфигураций инициализации (`cloud-init clean`).

{{< alert level="info" >}}
Снимок содержит конфигурацию виртуальной машины и снимки всех её дисков.

Восстановление снимка предполагает полное восстановление виртуальной машины на моммент создания её снимка.
{{< /alert >}}

Снимок будет создан успешно, если:

- ВМ выключена
- Установлен `qemu-guest-agent` и файловая система успешно "заморожена".

Если целостность данных не критична, снимок можно создать на работающей ВМ без заморозки ФС. Для этого укажите в спецификации:

```yaml
spec:
  requiredConsistency: false
```

При создании снимка необходимо указать названия классов снимков томов `VolumeSnapshotClass`, которые будут использованы для создания снимков дисков, подключенных к виртуальной машине.

Чтобы получить список поддерживаемых ресурсов `VolumeSnapshotClasses`, выполните команду:

```bash
d8 k get volumesnapshotclasses
```

Пример вывода:

```txt
NAME                     DRIVER                                DELETIONPOLICY   AGE
csi-nfs-snapshot-class   nfs.csi.k8s.io                        Delete           34d
sds-replicated-volume    replicated.csi.storage.deckhouse.io   Delete           39d
```

Создание снимка виртуальной машины будет неудачным, если выполнится хотя бы одно из следующих условий:

- не все зависимые устройства виртуальной машины готовы;
- есть изменения, ожидающие перезапуска виртуальной машины;
- среди зависимых устройств есть диск, находящийся в процессе изменения размера.

При создании снимка динамический IP-адрес ВМ автоматически преобразуется в статический и сохраняется для восстановления.

Если не требуется преобразование и использование старого IP-адреса виртуальной машины, можно установить соответствующую политику в значение `Never`. В этом случае будет использован тип адреса без преобразования (`Auto` или `Static`).

```yaml
spec:
  keepIPAddress: Never
```

Пример манифеста для создания снимка виртуальной машины:

```yaml
d8 k apply -f - <<EOF
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualMachineSnapshot
metadata:
  name: linux-vm-snapshot
spec:
  virtualMachineName: linux-vm
  volumeSnapshotClasses:
    - storageClassName: i-sds-replicated-thin-r2 # Подставьте ваше название StorageClass.
      volumeSnapshotClassName: sds-replicated-volume # Подставьте ваше название VolumeSnapshotClass.
  requiredConsistency: true
  keepIPAddress: Never
EOF
```

### Восстановление из снимков

Для восстановления виртуальной машины из снимка используется ресурс `VirtualMachineRestore` . В процессе восстановления в кластере автоматически создаются следующие объекты:

- VirtualMachine — основной ресурс ВМ с конфигурацией из снимка.
- VirtualDisk — диски, подключенные к ВМ на момент создания снимка.
- VirtualBlockDeviceAttachment — связи дисков с ВМ (если они существовали в исходной конфигурации).
- Secret — секреты с настройками cloud-init или sysprep (если они были задействованы в оригинальной ВМ).

Важно: ресурсы создаются только в том случае , если они присутствовали в конфигурации ВМ на момент создания снимка. Это гарантирует восстановление точной копии среды, включая все зависимости и настройки.

#### Восстановление виртуальной машины

{{< alert level="warning" >}}
Чтобы восстановить виртуальную машину, необходимо удалить её текущую конфигурацию и все связанные диски. Это связано с тем, что процесс восстановления возвращает виртуальную машину и её диски к состоянию, зафиксированному в момент создания резервного снимка.
{{< /alert >}}

Пример манифеста для восстановления виртуальной машины из снимка:

```yaml
d8 k apply -f - <<EOF
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualMachineRestore
metadata:
  name: <restore name>
spec:
  virtualMachineSnapshotName: <virtual machine snapshot name>
EOF
```

#### Создание клона ВМ / Использование снимка как шаблона для создания ВМ

Снимок виртуальной машины может использоваться как для создания её точной копии (клона), так и в качестве шаблона для развёртывания новых ВМ с аналогичной конфигурацией.

Для этого требуется создать ресурс `VirtualMachineRestore` и задать параметры переименования в блоке `.spec.nameReplacements`, чтобы избежать конфликтов имён.

Пример манифеста для восстановления ВМ из снимка:

```yaml
d8 k apply -f - <<EOF
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualMachineRestore
metadata:
  name: <name>
spec:
  virtualMachineSnapshotName: <virtual machine snapshot name>
  nameReplacements:
    - from:
        kind: VirtualMachine
        name: <old vm name>
      to: <new vm name>
    - from:
        kind: VirtualDisk
        name: <old disk name>
      to: <new disk name>
    - from:
        kind: VirtualDisk
        name: <old secondary disk name>
      to: <new secondary disk name>
    - from:
        kind: VirtualMachineBlockDeviceAttachment
        name: <old attachment name>
      to: <new attachment name>
EOF
```

При восстановлении виртуальной машины из снимка важно учитывать следующие условия:

1. Если ресурс `VirtualMachineIPAddress` уже существует в кластере, он не должен быть назначен другой ВМ .
2. Для статических IP-адресов (`type: Static`) значение должно полностью совпадать с тем, что было зафиксировано в снимке.
3. Секреты, связанные с автоматизацией (например, конфигурация cloud-init или sysprep), должны точно соответствовать восстанавливаемой конфигурации.

Несоблюдение этих требований приведёт к ошибке восстановления . Это связано с тем, что система проверяет целостность конфигурации и уникальность ресурсов для предотвращения конфликтов в кластере.
