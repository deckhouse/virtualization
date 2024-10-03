---
title: "Руководство пользователя"
menuTitle: "Руководство пользователя"
weight: 50
---

{{< alert level="info" >}}
С детальным описанием параметров настройки ресурсов приведенных в данном документе вы можете ознакомится в разделе [Custom Resources](cr.html)
{{< /alert >}}

# Образы

Ресурс `VirtualImage` (`vi`) используются для хранения образов виртуальных машин, он доступен только в том неймспейсе или проекте, в котором был создан.

Образы бывают следующих видов:

- ISO-образ — это установочный образ, который обычно используется для установки операционной системы с нуля. Такие образы обычно распространяются производителями операционных систем и применяются для установки ОС на физические и виртуальные серверы.
- Образ диска виртуальной машины с предустановленной системой — это диск с уже установленной и настроенной операционной системой, готовой к использованию сразу после создания виртуальной машины. Некоторые производители предоставляют такие образы, и они могут быть в различных форматах, таких как qcow2, raw, vmdk и других.

Примеры ресурсов, где можно получить образ диска виртуальной машины:

- Ubuntu: https://cloud-images.ubuntu.com
- Alt Linux: https://ftp.altlinux.ru/pub/distributions/ALTLinux/platform/images/cloud/x86_64
- Astra Linux: https://download.astralinux.ru/ui/native/mg-generic/alse/cloudinit

После создания ресурсов тип и размер образа определяется автоматически, данная информация будет отражена в статусе ресурса.

Образы могут быть получены из различных источников, таких как HTTP-серверы, на которых расположены файлы образов, или контейнерные реестры (container registries), где образы сохраняются и становятся доступны для скачивания. Также существует возможность загрузить образы напрямую из командной строки, используя утилиту `curl`.

Проектный образ поддерживаются два варианта хранения:

- `ContainerRegistry` - тип по умолчанию, при котором образ хранится в `DVCR`.
- `Kubernetes` - тип, при котором в качестве хранилища для образа используется `PVC`. Этот вариант предпочтителен, если используется хранилище с поддержкой быстрого клонирования `PVC`, что позволяет быстрее создавать диски из образов.

С полным описанием параметров конфигурации образов можно ознакомиться по [ссылке](cr.html#virtualimage)

## Создание образа с HTTP-сервера

Рассмотрим вариант создания образа с вариантом хранения в DVCR. Выполните следующую команду для создания `VirtualImage`:

```yaml
d8 k apply -f - <<EOF
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualImage
metadata:
  name: ubuntu-22.04
spec:
  # Сохраним образ в DVCR
  storage: ContainerRegistry
  # Источник для создания образа.
  dataSource:
    type: HTTP
    http:
      url: "https://cloud-images.ubuntu.com/minimal/releases/jammy/release/ubuntu-22.04-minimal-cloudimg-amd64.img"
EOF
```

Проверить результат создания `VirtualImage`:

```bash
d8 k get virtualimage ubuntu-22.04
# или более короткий вариант
d8 k get vi ubuntu-22.04

# NAME           PHASE   CDROM   PROGRESS   AGE
# ubuntu-22.04   Ready   false   100%       23h
```

После создания ресурс `VirtualImage` может находиться в следующих состояниях (фазах):

- `Pending` - ожидание готовности всех зависимых ресурсов, требующихся для создания образа.
- `WaitForUserUpload` - ожидание загрузки образа пользователем (фаза присутствует только для `type=Upload`).
- `Provisioning` - идет процесс создания образа.
- `Ready` - образ создан и готов для использования.
- `Failed` - произошла ошибка в процессе создания образа.
- `Terminating` - идет процесс удаления Образа. Образа может "зависнуть" в данном состоянии если он еще подключен к виртуальной машине.

До тех пор пока образ не перешёл в фазу `Ready` содержимое всего блока `.spec` допускается изменять. При изменении процесс создании диска запустится заново. После перехода в фазу `Ready` содержимое блока `.spec` менять нельзя!

Отследить процесс создания образа можно путем добавления ключа `-w` к предыдущей команде:

```bash
d8 k get vi ubuntu-22.04 -w

# NAME           PHASE          CDROM   PROGRESS   AGE
# ubuntu-22.04   Provisioning   false              4s
# ubuntu-22.04   Provisioning   false   0.0%       4s
# ubuntu-22.04   Provisioning   false   28.2%      6s
# ubuntu-22.04   Provisioning   false   66.5%      8s
# ubuntu-22.04   Provisioning   false   100.0%     10s
# ubuntu-22.04   Provisioning   false   100.0%     16s
# ubuntu-22.04   Ready          false   100%       18s
```

В описание ресурса `VirtualImage` можно получить дополнительную информацию о скачанном образе:

```bash
d8 k describe vi ubuntu-22.04
```

Теперь рассмотрим пример создания образа с хранением его в PVC:

```yaml
d8 k apply -f - <<EOF
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualImage
metadata:
  name: ubuntu-22.04-pvc
spec:
  # Настройки хранения проектного образа.
  storage: Kubernetes
  # Источник для создания образа.
  dataSource:
    type: HTTP
    http:
      url: "https://cloud-images.ubuntu.com/minimal/releases/jammy/release/ubuntu-22.04-minimal-cloudimg-amd64.img"
EOF
```

Проверить результат создания `VirtualImage`:

```bash
d8 k get vi ubuntu-22.04-pvc

# NAME              PHASE   CDROM   PROGRESS   AGE
# ubuntu-22.04-pvc  Ready   false   100%       23h
```

## Создание образа из Container Registry

Образ, хранящийся в Container Registry имеет определенный формат. Рассмотрим на примере:

Для начала загрузите образ локально:

```bash
curl -L https://cloud-images.ubuntu.com/minimal/releases/jammy/release/ubuntu-22.04-minimal-cloudimg-amd64.img -o ubuntu2204.img
```

Далее создайте `Dockerfile` со следующим содержимым:

```Dockerfile
FROM scratch
COPY ubuntu2204.img /disk/ubuntu2204.img
```

Соберите образ и загрузите его в container registry. В качестве container registry в примере ниже использован docker.io. для выполнения вам необходимо иметь учетную запись сервиса и настроенное окружение.

```bash
docker build -t docker.io/<username>/ubuntu2204:latest
```

где `username` — имя пользователя, указанное при регистрации в docker.io.

Загрузите созданной образ в container registry:

```bash
docker push docker.io/<username>/ubuntu2204:latest
```

Чтобы использовать этот образ, создайте в качестве примера ресурс:

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

## Загрузка образа из командной строки

Чтобы загрузить образ из командной строки, предварительно создайте следующий ресурс, как представлено ниже на примере `VirtualImage`:

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

После создания, ресурс перейдет в фазу `WaitForUserUpload` (`d8 k get vi some-image`), а это значит, что он готов для загрузки образа.

Доступно два варианта загрузки с узла кластера и с произвольного узла за пределами кластера:

```bash
d8 k get vi some-image -o jsonpath="{.status.imageUploadURLs}"  | jq

# {
#   "external":"https://virtualization.example.com/upload/g2OuLgRhdAWqlJsCMyNvcdt4o5ERIwmm",
#   "inCluster":"http://10.222.165.239"
# }
```

В качестве примера загрузите образ Cirros

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
# NAME         PHASE   CDROM   PROGRESS   AGE
# some-image   Ready   false   100%       1m
```

# Диски

Диски в виртуальных машинах необходимы для записи и хранения данных, обеспечивая полноценное функционирование приложений и операционных систем. Под "капотом" этих дисков используется хранилище, предоставляемое платформой.

В зависимости от свойств хранилища поведение дисков при создании и виртуальных машин в процессе эксплуатации может отличаться:

Свойство VolumeBindingMode:

- `Immediate` - Диск создается сразу после создания ресурса (предполагается, что диск будет доступен для подключения к виртуальной машине на любом узле кластера).
- `WaitForFirstConsumer` - Диск создается только после того как будет подключен к виртуальной машине и будет создан на том узле, на котором будет запущена виртуальная машина.

Режим доступа AccessMode:

- `ReadWriteOnce (RWO)` - доступ к диску предоставляется только одному экземпляру виртуальной машины. Живая миграция виртуальных машин с такими дисками невозможна.
- `ReadWriteMany (RWX)` - множественный доступ к диску. Живая миграция виртуальных машин с такими дисками возможна.

При создании диска контроллер самостоятельно определит наиболее оптимальные параметры поддерживаемые хранилищем.

Внимание: Создать диски из iso-образов - нельзя!

Чтобы узнать доступные варианты хранилищ на платформе, выполните следующую команду:

```bash
kubectl get storageclass

# NAME                          PROVISIONER                           RECLAIMPOLICY   VOLUMEBINDINGMODE      ALLOWVOLUMEEXPANSION   AGE
# i-linstor-thin-r1 (default)   replicated.csi.storage.deckhouse.io   Delete          Immediate              true                   48d
# i-linstor-thin-r2             replicated.csi.storage.deckhouse.io   Delete          Immediate              true                   48d
# i-linstor-thin-r3             replicated.csi.storage.deckhouse.io   Delete          Immediate              true                   48d
# linstor-thin-r1               replicated.csi.storage.deckhouse.io   Delete          WaitForFirstConsumer   true                   48d
# linstor-thin-r2               replicated.csi.storage.deckhouse.io   Delete          WaitForFirstConsumer   true                   48d
# linstor-thin-r3               replicated.csi.storage.deckhouse.io   Delete          WaitForFirstConsumer   true                   48d
# nfs-4-1-wffc                  nfs.csi.k8s.io                        Delete          WaitForFirstConsumer   true                   30d
```

С полным описанием параметров конфигурации дисков можно ознакомиться [тут](https://deckhouse.ru/products/kubernetes-platform/modules/virtualization/stable/cr.html#virtualdisk)

## Создание пустого диска

Пустые диски обычно используются для установки на них ОС, либо для хранения каких-либо данных.

Создайте диск:

```yaml
d8 k apply -f - <<EOF
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualDisk
metadata:
  name: vd-blank
spec:
  # Настройки параметров хранения диска.
  persistentVolumeClaim:
    # Подставьте ваше название StorageClass.
    storageClassName: i-linstor-thin-r2
    size: 100Mi
EOF
```

После создания ресурс `VirtualDisk` может находиться в следующих состояниях (фазах):

- `Pending` - ожидание готовности всех зависимых ресурсов, требующихся для создания диска.
- `Provisioning` - идет процесс создания диска.
- `Resizing` - идет процесс изменения размера диска.
- `WaitForFirstConsumer` - диск ожидает создания виртуальной машины, которая будет его использовать.
- `Ready` - диск создан и готов для использования.
- `Failed` - произошла ошибка в процессе создания.
- `Terminating` - идет процесс удаления диска. Диск может "зависнуть" в данном состоянии если он еще подключен к виртуальной машине.

До тех пор пока диск не перешёл в фазу `Ready` содержимое всего блока `.spec` допускается изменять. При изменении процесс создании диска запустится заново.

Проверьте состояние диска после создание командой:

```bash
d8 k get vd vd-blank
# NAME       PHASE   CAPACITY   AGE
# vd-blank   Ready   100Mi      1m2s
```

## Создание диска из образа

Диск также можно создавать и заполнять данными из ранее созданных образов `ClusterVirtualImage` и `VirtualImage`.

При создании диска можно указать его желаемый размер, который должен быть равен или больше размера распакованного образа. Если размер не указан, то будет создан диск с размером, соответствующим исходному образу диска.

На примере ранее созданного кластерного образа `ClusterVirtualImage`, рассмотрим команду позволяющую определить размер распакованного образа:

```bash
d8 k get cvi ubuntu-22.04 -o wide

# NAME           PHASE   CDROM   PROGRESS   STOREDSIZE   UNPACKEDSIZE   REGISTRY URL                                                                       AGE
# ubuntu-22.04   Ready   false   100%       285.9Mi      2.5Gi          dvcr.d8-virtualization.svc/cvi/ubuntu-22.04:eac95605-7e0b-4a32-bb50-cc7284fd89d0   122m
```

Искомый размер указан в колонке **UNPACKEDSIZE** и равен 2.5Gi.

Создадим диск из этого образа:

```yaml
d8 k apply -f - <<EOF
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualDisk
metadata:
  name: ubuntu-root
spec:
  # Настройки параметров хранения диска.
  persistentVolumeClaim:
    # Укажем размер больше чем значение распакованного образа.
    size: 10Gi
    # Подставьте ваше название StorageClass.
    storageClassName: i-linstor-thin-r2
  # Источник из которого создается диск.
  dataSource:
    type: ObjectRef
    objectRef:
      kind: VirtualImage
      name: ubuntu-22.04
EOF
```

А теперь создайте диск, без явного указания размера:

```yaml
d8 k apply -f - <<EOF
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualDisk
metadata:
  name: ubuntu-root-2
spec:
  # Настройки параметров хранения диска.
  persistentVolumeClaim:
    # Подставьте ваше название StorageClass.
    storageClassName: i-linstor-thin-r2
  # Источник из которого создается диск.
  dataSource:
    type: ObjectRef
    objectRef:
      kind: VirtualImage
      name: ubuntu-22.04
EOF
```

Проверьте состояние дисков после создания:

```bash
d8 k get vd

# NAME           PHASE   CAPACITY   AGE
# ubuntu-root    Ready   10Gi       7m52s
# ubuntu-root-2  Ready   2590Mi     7m15s
```

## Изменение размера диска

Размер дисков можно увеличивать, даже если они уже подключены к работающей виртуальной машине. Для этого отредактируйте поле `spec.persistentVolumeClaim.size`:

Проверим размер до изменения:

```bash
d8 k get vd ubuntu-root

# NAME          PHASE   CAPACITY   AGE
# ubuntu-root   Ready   10Gi       10m
```

Применим изменения:

```bash
kubectl patch vd ubuntu-root --type merge -p '{"spec":{"persistentVolumeClaim":{"size":"11Gi"}}}'
```

Проверим размер после изменения:

```bash
d8 k get vd ubuntu-root

# NAME          PHASE   CAPACITY   AGE
# ubuntu-root   Ready   11Gi       12m
```

# Виртуальные машины

Для создания виртуальной машины используется ресурс `VirtualMachine`, его параметры позволяют сконфигурировать:

- ресурсы, требуемые для работы виртуальной машины (процессор, память, диски и образы);
- правила размещения виртуальной машины на узлах кластера;
- настройки загрузчика и оптимальные параметры для гостевой ОС;
- политику запуска виртуальной машины и политику применения изменений;
- сценарии начальной конфигурации (cloud-init).

С полным описанием параметров конфигурации виртуальной машины можно ознакомиться по [этой](https://deckhouse.ru/products/kubernetes-platform/modules/virtualization/stable/cr.html#virtualmachine) ссылке.

## Создание виртуальной машины

Ниже представлен пример простой конфигурации виртуальной машины, запускающей ОС Ubuntu 22.04. В примере используется сценарий первичной инициализации виртуальной машины (cloud-init), который устанавливает пакет **nginx** и создает пользователя `cloud` с паролем `cloud`:

Создайте виртуальную машину с диском созданным [ранее](#создание-диска-из-образа).

Ниже представлен пример простой конфигурации виртуальной машины, запускающей ОС Ubuntu 22.04. В примере используется сценарий первичной инициализации виртуальной машины (cloud-init), который устанавливает пакет **nginx** и создает пользователя `cloud` с паролем `cloud`:

Пароль был сгенерирован с использованием следующей команды и при необходимости вы можете его поменять на свой:

```bash
mkpasswd --method=SHA-512 --rounds=4096 -S saltsalt
```

```yaml
d8 k apply -f - <<"EOF"
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualMachine
metadata:
  name: ubuntu-vm
spec:
  # Название класса ВМ.
  virtualMachineClassName: host
  # Блок скриптов первичной инициализации ВМ.
  provisioning:
    type: UserData
    # Пример cloud-init-сценария для создания пользователя cloud с паролем cloud и установки сервиса nginx.
    userData: |
      #cloud-config
      package_update: true
      packages:
        - nginx
      run_cmd:
        - systemctl daemon-relaod
        - systemctl enable --now nginx
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
      name: ubuntu-root
EOF
```

После создания `VirtualMachine` может находиться в следующих состояниях (фазах):

- `Pending` - ожидание готовности всех зависимых ресурсов, требующихся для запуска виртуальной машины.
- `Starting` - идет процесс запуска виртуальной машины.
- `Running` - виртуальная машина запущена.
- `Stopping` - идет процесс остановки виртуальной машины.
- `Stopped` - виртуальная машина остановлена.
- `Terminating` - виртуальная машина удаляется.
- `Migrating` - виртуальная машина находится в состоянии живой миграции на другой узел.

Проверьте состояние виртуальной машины после создания:

```bash
d8 k get vm ubuntu-vm

# NAME        PHASE     NODE           IPADDRESS     AGE
# ubuntu-vm   Running   virtlab-pt-2   10.66.10.12   11m
```

После создания виртуальная машина автоматически получит IP-адрес из диапазона, указанного в настройках модуля (блок `virtualMachineCIDRs`).

## Подключение к виртуальной машине

Для подключения к виртуальной машине доступны следующие способы:

- протокол удаленного управления (например SSH), который должен быть предварительно настроен на виртуальной машине.
- серийная консоль (serial console)
- протокол VNC

В качестве примера подключитесь к виртуальной машине по серийной консоли

```bash
d8 v console ubuntu-vm

# Successfully connected to ubuntu-vm console. The escape sequence is ^]

ubuntu-vm login: cloud
Password: cloud
```

Нажмите `Ctrl+]` для завершения работы с серийной консолью.

Пример команды для подключения по VNC:

```bash
d8 v vnc ubuntu-vm
```

Пример команды для подключения по SSH.

```bash
d8 v ssh cloud@ubuntu-vm --local-ssh
```

## Управление состоянием виртуальной машины и политика запуска

Управление состоянием виртуальной машины осуществляется следующими способами:

- Путем создания ресурса `VirtualMachineOperation` (`VMOP`)
- С использованием утилиты `d8`

Ресурс `VirtualMachineOperation` декларативно описывает императивную операцию, которую необходимо применить к виртуальной машине. Данная операция применяется к виртуальной машине сразу после её создания в кластере.

Пример операции для выполнения перезагрузки виртуальной машины с именем `ubuntu-vm`:

```yaml
d8 k apply -f - <<EOF
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualMachineOperation
metadata:
  name: restart-linux-vm-$(date +%s)
spec:
  virtualMachineName: ubuntu-vm
  # Тип применяемой операции = применяемая операция.
  type: Restart
EOF
```

Аналогичное действие можно выполнить с использованием утилиты `d8`:

```bash
d8 v restart  ubuntu-vm
```

Перечень возможных операций приведен в таблице ниже:

| d8             | VMOP type | Действие                      |
| -------------- | --------- | ----------------------------- |
| `d8 v stop`    | `stop`    | Остановить ВМ                 |
| `d8 v start`   | `start`   | Запустить ВМ                  |
| `d8 v restart` | `restart` | Перезапустить ВМ              |
| `d8 v migrate` | `migrate` | Мигрировать ВМ на другой узел |

Политика запуска предназначена для автоматизированного управления состоянием виртуальной машины. Определяется она в виде параметра `.spec.runPolicy` в спецификации виртуальной машины. Поддерживается следующие политики:

- `AlwaysOnUnlessStoppedManually` - (по умолчанию) после создания ВМ всегда находится в запущенном состоянии. В случае сбоев работа ВМ восстанавливается автоматически. Остановка ВМ возможно только путем вызова команды `d8 v stop` или создания соотвествующей операции.
- `AlwaysOn` - после создания ВМ всегда находится в работающем состоянии, даже в случае ее выключения средствами ОС. В случае сбоев работа ВМ восстанавливается автоматически.
- `Manual` - после создания состоянием ВМ управляет пользователь вручную с использованием команд или операций.
- `AlwaysOff` - после создания ВМ всегда находится в выключенном состоянии. Возможность включения ВМ через команды\операции - отсуствует.

## Изменение конфигурации виртуальной машины

Изменения в конфигурацию виртуальной машины можно вносить в любой момент времени сразу после создания ресурса `VirtualMachine`, но как применятся данные изменения зависит от текущей фазы виртуальной машины и того, какие именно изменения были сделаны.

Изменения в конфигурацию виртуальной машины можно внести с использованием следующей команды вручную. Пример:

```bash
d8 k edit vm ubuntu-vm
```

Если виртуальная машина находится в выключенном (.status.phase: `Stopped`) состоянии, то внесенные изменения применятся сразу после её старта.

В случае если виртуальная машина запущенна (.status.phase: `Running`), то изменения могут быть применены по-разному и все зависит от того что именно мы меняем.

| Блок конфигурации                       | Как меняется            |
| --------------------------------------- | ----------------------- |
| `.metadata.labels`                      | Применяется сразу       |
| `.metadata.annotations`                 | Применяется сразу       |
| `.spec.runPolicy`                       | Применяется сразу       |
| `.spec.disruptions.restartApprovalMode` | Применяется сразу       |
| `.spec.*`                               | Только после рестарт ВМ |

Рассмотрим пример изменения конфигурации виртуальной машины:

Допустим мы хоти изменить количество ядер процессора. Сейчас виртуальня машина запущена и ей доступно одно ядро. Это мы можем проверить подключившись к вм с использованием серийной консоли и выполнив команду `nproc`.

```bash
d8 v ssh cloud@ubuntu-vm --local-ssh --command "nproc"
# 1
```

Примените следующий патч виртуальной машине `ubuntu-vm`, который изменит количество ядер с 1 на 2.

```bash
d8 k patch vm ubuntu-vm --type merge -p '{"spec":{"cpu":{"cores":2}}}'
# virtualmachine.virtualization.deckhouse.io/ubuntu-vm patched
```

Изменения к конфигурации применены, но не еще не применены к виртуальной машине. Проверьте, снова выполнив:

```bash
d8 v ssh cloud@ubuntu-vm --local-ssh --command "nproc"
# 1
```

Выполните команду, она отобразит изменения которые ожидают применения (для которых требуется рестарт):

```bash
d8 k get vm ubuntu-vm -o jsonpath="{.status.restartAwaitingChanges}" | jq .

# [
#   {
#     "currentValue": 1,
#     "desiredValue": 2,
#     "operation": "replace",
#     "path": "cpu.cores"
#   }
# ]
```

Выполните следующую команду:

```bash
d8 k get vm ubuntu-vm -o wide

# NAME        PHASE     CORES   COREFRACTION   MEMORY   NEED RESTART   AGENT   MIGRATABLE   NODE           IPADDRESS     AGE
# ubuntu-vm   Running   2       100%           1Gi      True           False   True         virtlab-pt-1   10.66.10.13   5m16s
```

В колонке `NEED RESTART` мы видим значение `True`, а это значит что для применения изменений требуется перезагрузка.

Выполним перезагрузку виртуальной машине:

```bash
d8 v restart ubuntu-vm
```

После перезагрузки виртуальной машины изменения будут применены и блок `.status.restartAwaitingChanges` будет пустой.

Выполните команду для проверки:

```bash
d8 v ssh cloud@ubuntu-vm --local-ssh --command "nproc"
# 2
```

Порядок применения изменений виртуальной машины через рестарт является поведением по умолчанию. Если есть необходимость применять внесенные изменения сразу и автоматически, для этого нужно изменит политику применения изменений:

```yaml
spec:
  disruptions:
    restartApprovalMode: Automatic
```

## Сценарии начальной инициализации

Сценарии начальной инициализации предназначены для первичной конфигурации виртуальной машины при её запуске.

Поддерживается [CloudInit](https://cloudinit.readthedocs.io) для конфигурирования виртуальных машин под управлением ОС \*nix и [Sysprep](https://learn.microsoft.com/ru-ru/windows-hardware/manufacture/desktop/sysprep--system-preparation--overview?view=windows-11) для конфигурирования виртуальных машин под управлением ОС Windows.

Сценарий CloudInit можно встраивать непосредственно в спецификацию виртуальной машины, но он ограничен максимальной длиной в 2048 байт:

```yaml
spec:
  provisioning:
    type: UserData
    userData: |
      #cloud-config
      package_update: true
      ...
```

При более длинных сценариях и\или наличия приватных данных, сценарий начальной инициализации виртуальной машины может быть создан в Secret'е. Пример Secret'а со сценарием CloudInit приведен ниже:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: cloud-init-example
data:
  userData: <base64 data>
type: provisioning.virtualization.deckhouse.io/cloud-init
```

фрагмент конфигурации виртуальной машины с при использовании скрипта начальной инициализации CloudInit хранящегося в Secret'е:

```yaml
spec:
  provisioning:
    type: UserDataRef
    userDataRef:
      kind: Secret
      name: cloud-init-example
```

Примечание: Значение поля `.data.userData` должно быть закодировано в формате Base64.

Для конфигурирования виртуальных машин под управлением ОС Windows с использованием Sysprep, поддерживается только вариант с Secret.

Пример Secret с сценарием Sysprep приведен ниже:

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

фрагмент конфигурации виртуальной машины с использованием скрипта начальной инициализации Sysprep в Secret'е:

```yaml
spec:
  provisioning:
    type: SysprepRef
    sysprepRef:
      kind: Secret
      name: sysprep-example
```

## Размещение ВМ по узлам

Модуль предоставляет возможности управления размещением виртуальных машин по узлам.

### nodeSelector

`nodeSelector` — это простейший способ контролировать размещение виртуальных машин, используя набор меток. Он позволяет задать, на каких узлах могут запускаться виртуальные машины, выбирая узлы с необходимыми метками.

```yaml
spec:
  nodeSelector:
    disktype: ssd
```

В этом примере виртуальная машина будет размещена только на узлах, которые имеют метку disktype со значением ssd.

### Affinity

`Affinity` предоставляет более гибкие и мощные инструменты по сравнению с `nodeSelector`. Он позволяет задавать "предпочтения" и "обязательности" для размещения виртуальных машин. `Affinity` поддерживает два вида: `nodeAffinity` и `virtualMachineAndPodAffinity`.

`nodeAffinity` позволяет определять, на каких узлах может быть запущена виртуальная машина, с помощью выражений меток, и может быть мягким (preferred) или жестким (required).

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

В этом примере виртуальная машина будет размещена только на узлах, которые имеют метку disktype со значением ssd.

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

В этом примере виртуальная машина будет размещена, если будет такая возможность (тк используется preffered) только на узлах на которых присутствует виртуальная машина с меткой server и значением database.

### AntiAffinity

`AntiAffinity` — это противоположность `Affinity`, которая позволяет задавать требования для избегания размещения виртуальных машин на одних и тех же узлах. Это полезно для распределения нагрузки или обеспечения отказоустойчивости.

Термины `Affinity` и `AntiAffinity` применимы только к отношению между виртуальныеми машинами. Для узлов используемые привязки называются `nodeAffinity`. В `nodeAffinity` нет отдельного антитеза, как в случае с `virtualMachineAndPodAffinity`, но можно создать противоположные условия, задав отрицательные операторы в выражениях меток: чтобы акцентировать внимание на исключении определенных узлов, можно воспользоваться `nodeAffinity` с оператором, таким как `NotIn`.

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

В данном примере виртуальные машины с меткой server: databased не будут размещены на одном и том же узле.

## Статически и динамически блочные устройства

Блочные устройства можно разделить на два типа по способу их подключения: статические и динамические (hotplug).

TODO: надо сказать что VD может быть подключен к ВМ только 1 раз.

### Статические блочные устройства

Статические блочные устройства указываются в спецификации виртуальной машины в блоке `.spec.blockDeviceRefs`. Этот блок представляет собой список, в который могут быть включены следующие блочные устройства:

- `VirtualImage`
- `ClusterVirtualImage`
- `VirtualDisk`

Порядок устройств в этом списке определяет последовательность их загрузки. Таким образом, если диск или образ указан первым, загрузчик сначала попробует загрузиться с него. Если это не удастся, система перейдет к следующему устройству в списке и попытается загрузиться с него. И так далее до момента обнаружения первого загрузчика.

Изменение состава и порядка устройств в блоке `.spec.blockDeviceRefs` возможно только с перезагрузкой виртуальной машины.

### Динамические блочные устройства

Динамические блочные устройства можно подключать и отключать от виртуальной машины, находящейся в запущенном состоянии, без необходимости перезагрузки.

Для подключения динамических блочных устройств используется ресурс `VirtualMachineBlockDeviceAttachment` (`vmbda`). На данный момент для подключения в качестве динамического блочного устройства поддерживается только `VirtualDisk`.

Создайте следующий ресурс, который подключит пустой диск `vd-blank` к виртуальной машине `ubuntu-vm`:

```yaml
d8 k apply -f - <<EOF
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualMachineBlockDeviceAttachment
metadata:
  name: attach-vd-blank
spec:
  blockDeviceRef:
    kind: VirtualDisk
    name: vd-blank
  virtualMachineName: ubuntu-vm
EOF
```

После создания `VirtualMachineBlockDeviceAttachment` может находиться в следующих состояниях (фазах):

- `Pending` - ожидание готовности всех зависимых ресурсов.
- `InProgress` - идет процесс подключения устройства.
- `Attached` - устройство подключено.

Проверьте состояние вашего ресурса:

```bash
d8 k get vmbda attach-vd-blank

# NAME              PHASE      VIRTUAL MACHINE NAME   AGE
# attach-vd-blank   Attached   ubuntu-vm              3m7s
```

Подключитесь к виртуальной машине и удостоверитесь, что диск подключен:

```bash
d8 v ssh cloud@ubuntu-vm --local-ssh --command "lsblk"

# NAME    MAJ:MIN RM  SIZE RO TYPE MOUNTPOINTS
# sda       8:0    0   10G  0 disk <--- статично подключенный диск ubuntu-root
# |-sda1    8:1    0  9.9G  0 part /
# |-sda14   8:14   0    4M  0 part
# `-sda15   8:15   0  106M  0 part /boot/efi
# sdb       8:16   0    1M  0 disk <--- cloudinit
# sdc       8:32   0 95.9M  0 disk <--- динамически подключенный диск vd-blank
```

Для отключения диска от виртуальной машины удалите ранее созданный ресурс:

```bash
d8 k delete vmbda attach-vd-blank
```

## Публикация виртуальных машин с использованием сервисов

Достаточно часто возникает необходимость сделать так, чтобы доступ к этим виртуальным машинам был возможен извне, например, для публикации каких-либо сервисов или удалённого администрирования. Для этих целей мы можем использовать сервисы, которые обеспечивают маршрутизацию трафика из внешней сети к внутренним ресурсам кластера. Рассмотрим несколько вариантов.

Предварительно, проставьте на ранее созданной вм следующие лейблы:

```bash
d8 k label vm ubuntu-vm app=nginx
# virtualmachine.virtualization.deckhouse.io/ubuntu-vm labeled
```

### Публикация сервисов виртуальной машины с использованием сервиса с типом NodePort

Сервис NodePort открывает определённый порт на всех узлах кластера, перенаправляя трафик на заданный внутренний порт сервиса.

Создайте следующий сервис:

```yaml
d8 k apply -f - <<EOF
apiVersion: v1
kind: Service
metadata:
  name: ubuntu-vm-nginx-nodeport
spec:
  type: NodePort
  selector:
    # лейбл по которому сервис определяет на какую виртуальную машину направлять трафик
    app: nginx
  ports:
    - protocol: TCP
      port: 80
      targetPort: 80
      nodePort: 31880
EOF
```

В данном примере будет создан сервис с типом NodePort, который открывает внешний порт 31880 на всех узлах вашего кластера. Этот порт будет направлять входящий трафик на внутренний порт 80 виртуальной машины, где запущено приложение Nginx.

### Публикация сервисов виртуальной машины с использованием сервиса с типом LoadBalancer

При использовании типа сервиса LoadBalancer кластер создаёт внешний балансировщик нагрузки, который распределит входящий трафик по всем экземплярам вашей виртуальной машины.

```yaml
d8 k apply -f - <<EOF
apiVersion: v1
kind: Service
metadata:
  name: ubuntu-vm-nginx-lb
spec:
  type: LoadBalancer
  selector:
    # лейбл по которому сервис определяет на какую виртуальную машину направлять трафик
    app: nginx
  ports:
    - protocol: TCP
      port: 80
      targetPort: 80
EOF
```

### Публикация сервисов виртуальной машины с использованием Ingress

Ingress позволяет управлять входящими HTTP/HTTPS запросами и маршрутизировать их к различным серверам в рамках вашего кластера. Это наиболее подходящий метод, если вы хотите использовать доменные имена и SSL-терминацию для доступа к вашим виртуальным машинам.

Для публикации сервиса виртуальной машины через Ingress необходимо создать следующие ресурсы:

Внутренний сервис для связки с Ingress. Пример:

```yaml
d8 k apply -f - <<EOF
apiVersion: v1
kind: Service
metadata:
  name: ubuntu-vm-nginx
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

И ресурс Ingress для публикации. Пример:

```yaml
d8 k apply -f - <<EOF
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: ubuntu-vm
spec:
  rules:
    - host: ubuntu-vm.example.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: ubuntu-vm-nginx
                port:
                  number: 80
EOF
```

## IP-адреса виртуальных машин

Блок `.spec.settings.virtualMachineCIDRs` в конфигурации модуля virtualization задает список подсетей для назначения ip-адресов виртуальным машинам (общий пул ip-адресов). Все адреса в этих подсетях доступны для использования, за исключением первого (адрес сети) и последнего (широковещательный адрес).

Ресурс `VirtualMachineIPAddressLease` (`vmipl`): Кластерный ресурс, который управляет арендой IP-адресов из общего пула, указанного в `virtualMachineCIDRs`.

Чтобы посмотреть список аренд IP-адресов (`vmipl`), используйте команду:

```bash
d8 k get vmipl
# NAME             VIRTUALMACHINEIPADDRESS                              STATUS   AGE
# ip-10-66-10-14   {"name":"ubuntu-vm-7prpx","namespace":"default"}     Bound    12h
```

Ресурс `VirtualMachineIPAddress` (`vmip`): Проектный/неймспейсный ресурс, который отвечает за резервирование арендованных IP-адресов и их привязку к виртуальным машинам. IP-адреса могут выделяться автоматически или по явному запросу.

Чтобы посмотреть список `vmip`, используйте команду:

```bash
d8 k get vmipl
# NAME             VIRTUALMACHINEIPADDRESS                              STATUS   AGE
# ip-10-66-10-14   {"name":"ubuntu-vm-7prpx","namespace":"default"}     Bound    12h
```

По умолчанию ip-адрес виртуальной машине назначается автоматически из подсетей, определенных в модуле и закрепляется за ней до её удаления. Проверить назначенный ip-адрес можно с помощью команды:

```bash
k get vmip
# NAME              ADDRESS       STATUS     VM          AGE
# ubuntu-vm-7prpx   10.66.10.14   Attached   ubuntu-vm   12h
```

Алгоритм автоматического присвоения ip-адреса виртуальной машине выглядит следующим образом:

- Пользователь создает виртуальную машину с именем `<vmname>`.
- Контроллер модуля автоматически создает ресурс `vmip` с именем `<vmname>-<hash>`, чтобы запросить IP-адрес и связать его с виртуальной машиной.
- Для этого `vmip` создается ресурс аренды `vmipl`, который выбирает случайный IP-адрес из общего пула.
- Как только ресурс `vmip` создан, виртуальная машина получает назначенный IP-адрес.

IP-адрес виртуальной машине назначается автоматически из подсетей, определенных в модуле, и остается закрепленным за машиной до её удаления. После удаления виртуальной машины ресурс `vmip` также удаляется, но IP-адрес временно остается закрепленным за проектом/неймспейсом и может быть повторно запрошен явно.

### Как запросить требуемый ip-адрес?

Задача: запросить конкретный ip-адрес из подсетей `virtualMachineCIDRs`.

Создайте ресурс `vmip`:

```yaml
d8 k apply -f - <<EOF
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualMachineIPAddress
metadata:
  name: ubuntu-vm-custom-ip
spec:
  staticIP: 10.66.20.77
  type: Static
EOF
```

Создайте новую или измените существующую виртуальную машину и в спецификации укажите требуемый ресурс `vmip` явно:

```yaml
spec:
  virtualMachineIPAdressName: ubuntu-vm-custom-ip
```

### Как сохранить присвоенный виртуальной машине ip-адрес?

Задача: сохранить выданный виртуальной машине ip-адрес для его повторного использования после удаления виртуальной машины.

Чтобы автоматически выданный ip-адрес виртуальной машины не удалился вместе с самой виртуальной машиной выполните следующие действия.

Получите название ресурса `vmip` для заданной виртуальной машины:

```bash
d8 k get vm ubuntu-vm -o jsonpath="{.status.virtualMachineIPAddressName}"
# ubuntu-vm-7prpx
```

Удалите блоки `.metadata.ownerReferences` из найденного ресурса:

```bash
d8 k patch vmip ubuntu-vm-7prpx --type=merge --patch '{"metadata":{"ownerReferences":null}}'
```

После удаления виртуальной машины, ресурс `vmip` сохранится и его можно будет переиспользовать снова во вновь созданной виртуальной машине:

```
spec:
  virtualMachineIPAdressName: ubuntu-vm-7prpx
```

Даже если ресурс `vmip` будет удален. Он остаётся арендованным для текущего проекта/неймспейса еще 10 минут. Поэтому существует возможность вновь его занять по запросу:

```bash
d8 k apply -f - <<EOF
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualMachineIPAddress
metadata:
  name: ubuntu-vm-custom-ip
spec:
  staticIP: 10.66.20.77
  type: Static
EOF
```
