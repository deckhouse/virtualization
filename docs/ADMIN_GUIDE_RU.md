---
title: "Руководство администратора"
weight: 40
---

## Введение

Данное руководство предназначено для [администраторов](./README_RU.md#ролевая-модель) Deckhouse Virtualization Platform и описывает порядок создания и изменения кластерных ресурсов.

Также администратор обладает правами на управление проектными ресурсами, описание которых содержится в документе ["Инструкция пользователя"](./USER_GUIDE_RU.md).

## Образы

Ресурс `ClusterVirtualImage` служит для загрузки образов виртуальных машин во внутрикластерное хранилище, после чего с его помощью можно создавать диски виртуальных машин. Он доступен во всех пространствах имен/проектах кластера.

Процесс создания образа включает следующие шаги:

- Пользователь создаёт ресурс `ClusterVirtualImage`.
- После создания образ автоматически загружается из указанного в спецификации источника в хранилище (DVCR).
- После завершения загрузки ресурс становится доступным для создания дисков.

Существуют различные типы образов:

- ISO-образ — установочный образ, используемый для начальной установки операционной системы. Такие образы выпускаются производителями ОС и используются для установки на физические и виртуальные серверы.
- Образ диска с предустановленной системой — содержит уже установленную и настроенную операционную систему, готовую к использованию после создания виртуальной машины. Эти образы предлагаются несколькими производителями и могут быть представлены в таких форматах, как qcow2, raw, vmdk и другие.

Примеры ресурсов для получения образов виртуальной машины:

- **Ubuntu**: https://cloud-images.ubuntu.com
- **Alt Linux**: https://ftp.altlinux.ru/pub/distributions/ALTLinux/platform/images/cloud/x86_64
- **Astra Linux**: https://download.astralinux.ru/ui/native/mg-generic/alse/cloudinit

После создания ресурса тип и размер образа определяются автоматически, и эта информация отражается в статусе ресурса.

Образы могут быть загружены из различных источников, таких как HTTP-серверы, где расположены файлы образов, или контейнерные реестры. Также доступна возможность загрузки образов напрямую из командной строки с использованием утилиты curl.

Образы могут быть созданы из других образов и дисков виртуальных машин.

С полным описанием параметров конфигурации ресурса ClusterVirtualImage можно ознакомиться по [ссылке](cr.html#clustervirtualimage).

### Создание образа с HTTP-сервера

Рассмотрим вариант создания кластерного образа.

Выполните следующую команду для создания `ClusterVirtualImage`:

```yaml
d8 k apply -f - <<EOF
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: ClusterVirtualImage
metadata:
  name: ubuntu-22.04
spec:
  # Источник для создания образа.
  dataSource:
    type: HTTP
    http:
      url: "https://cloud-images.ubuntu.com/minimal/releases/jammy/release/ubuntu-22.04-minimal-cloudimg-amd64.img"
EOF
```

Проверьте результат создания `ClusterVirtualImage`:

```bash
d8 k get clustervirtualimage ubuntu-22.04
# или более короткий вариант
d8 k get cvi ubuntu-22.04

# NAME           PHASE   CDROM   PROGRESS   AGE
# ubuntu-22.04   Ready   false   100%       23h
```

После создания ресурс `ClusterVirtualImage` может находиться в следующих состояниях (фазах):

- `Pending` - ожидание готовности всех зависимых ресурсов, требующихся для создания образа.
- `WaitForUserUpload` - ожидание загрузки образа пользователем (фаза присутствует только для `type=Upload`).
- `Provisioning` - идет процесс создания образа.
- `Ready` - образ создан и готов для использования.
- `Failed` - произошла ошибка в процессе создания образа.
- `Terminating` - идет процесс удаления Образа. Образа может "зависнуть" в данном состоянии если он еще подключен к виртуальной машине.

До тех пор пока образ не перешёл в фазу `Ready` содержимое всего блока `.spec` допускается изменять. При изменении процесс создании диска запустится заново. После перехода в фазу `Ready` содержимое блока `.spec` менять нельзя!

Отследить процесс создания образа можно путем добавления ключа `-w` к предыдущей команде:

```bash
d8 k get cvi ubuntu-22.04 -w

# NAME           PHASE          CDROM   PROGRESS   AGE
# ubuntu-22.04   Provisioning   false              4s
# ubuntu-22.04   Provisioning   false   0.0%       4s
# ubuntu-22.04   Provisioning   false   28.2%      6s
# ubuntu-22.04   Provisioning   false   66.5%      8s
# ubuntu-22.04   Provisioning   false   100.0%     10s
# ubuntu-22.04   Provisioning   false   100.0%     16s
# ubuntu-22.04   Ready          false   100%       18s
```

В описание ресурса `ClusterVirtualImage` можно получить дополнительную информацию о скачанном образе:

```bash
d8 k describe cvi ubuntu-22.04
```

### Создание образа из Container Registry

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
kind: ClusterVirtualImage
metadata:
  name: ubuntu-2204
spec:
  dataSource:
    type: ContainerImage
    containerImage:
      image: docker.io/<username>/ubuntu2204:latest
EOF
```

### Загрузка образа из командной строки

Чтобы загрузить образ из командной строки, предварительно создайте следующий ресурс, как представлено ниже на примере `ClusterVirtualImage`:

```yaml
d8 k apply -f - <<EOF
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: ClusterVirtualImage
metadata:
  name: some-image
spec:
  # Настройки источника образа.
  dataSource:
    type: Upload
EOF
```

После создания, ресурс перейдет в фазу `WaitForUserUpload`, а это значит, что он готов для загрузки образа.

Доступно два варианта загрузки с узла кластера и с произвольного узла за пределами кластера:

```bash
d8 k get cvi some-image -o jsonpath="{.status.imageUploadURLs}"  | jq

# {
#   "external":"https://virtualization.example.com/upload/g2OuLgRhdAWqlJsCMyNvcdt4o5ERIwmm",
#   "inCluster":"http://10.222.165.239/upload"
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
d8 k get cvi some-image
# NAME         PHASE   CDROM   PROGRESS   AGE
# some-image   Ready   false   100%       1m
```

## Классы виртуальных машин

Ресурс `VirtualMachineClass` предназначен для централизованной конфигурации предпочтительных параметров виртуальных машин. Он позволяет определять инструкции CPU и политики конфигурации ресурсов CPU и памяти для виртуальных машин, а также определять соотношения этих ресурсов. Помимо этого, `VirtualMachineClass` обеспечивает управление размещением виртуальных машин по узлам платформы. Это позволяет администраторам эффективно управлять ресурсами платформы виртуализации и оптимально размещать виртуальные машины на узлах платформы.

Платформа виртуализации предоставляет 3 предустановленных ресурса `VirtualMachineClass`:

```bash
kubectl get virtualmachineclass
NAME               PHASE   AGE
host               Ready   6d1h
host-passthrough   Ready   6d1h
generic            Ready   6d1h
```

- `host` - данный класс использует виртуальный CPU, максимально близкий к CPU узла платформы по набору инструкций. Это обеспечивает высокую производительность и функциональность, а также совместимость с живой миграцией для узлов с похожими типами процессоров. Например, миграция ВМ между узлами с процессорами Intel и AMD не будет работать. Это также справедливо для процессоров разных поколений, так как набор инструкций у них отличается.
- `host-passthrough` - используется физический CPU узла платформы напрямую без каких-либо изменений. При использовании данного класса, гостевая ВМ может быть мигрирована только на целевой узел, у которого CPU точно соответствует CPU исходного узла.
- `generic` - универсальная модель CPU, использующая достаточно старую, но поддерживаемую большинством современных процессоров модель Nehalem. Это позволяет запускать ВМ на любых узлах кластера с возможностью живой миграции.

`VirtualMachineClass` является обязательным для указания в конфигурации виртуальной машины, пример того как указывать класс в спецификации ВМ:

```yaml
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualMachine
metadata:
  name: linux-vm
spec:
  virtualMachineClassName: generic # название ресурса VirtualMachineClass
  ...
```

{{< alert level="warning" >}}
Внимание! Рекомендуется создать как минимум один ресурс `VirtualMachineClass` в кластере с типом Discovery сразу после того как все узлы будут настроены и добавлены в кластер. Это позволит использовать в виртуальных машинах универсальный процессор с максимально возможными характеристиками с учетом ЦП на узлах кластера, что позволит виртуальным машинам использовать максимум возможностей ЦП и при необходимости беспрепятственно осуществлять миграцию между узлами кластера.
{{< /alert >}}

Администраторы платформы могут создавать требуемые классы виртуальных машин по своим потребностям, но рекомендуется создавать необходимый минимум. Рассмотрим на следующем примере:

### Пример конфигурации VirtualMachineClass

![](./images/vmclass-examples.ru.png)

Представим, что у нас есть кластер из четырех узлов. Два из этих узлов с лейблом `group=blue` оснащены процессором "CPU X" с тремя наборами инструкций, а остальные два узла с лейблом `group=green` имеют более новый процессор "CPU Y" с четырьмя наборами инструкций.

Для оптимального использования ресурсов данного кластера, рекомендуется создать три дополнительных класса виртуальных машин (VirtualMachineClass):

- **universal**: Этот класс позволит виртуальным машинам запускаться на всех узлах платформы и мигрировать между ними. При этом будет использоваться набор инструкций для самой младшей модели CPU, что обеспечит наибольшую совместимость.
- **cpuX**: Этот класс будет предназначен для виртуальных машин, которые должны запускаться только на узлах с процессором "CPU X". ВМ смогут мигрировать между этими узлами, используя доступные наборы инструкций "CPU X".
- **cpuY**: Этот класс предназначен для виртуальных машин, которые должны запускаться только на узлах с процессором "CPU Y". ВМ смогут мигрировать между этими узлами, используя доступные наборы инструкций "CPU Y".

> Наборы инструкций для процессора — это список всех команд, которые процессор может выполнять, таких как сложение, вычитание или работа с памятью. Они определяют, какие операции возможны, влияют на совместимость программ и производительность, а также могут меняться от одного поколения процессоров к другому.

Примерные конфигурации ресурсов для данного кластера:

```yaml
---
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualMachineClass
metadata:
  name: universal
spec:
  cpu:
    discovery: {}
    type: Discovery
  sizingPolicies: { ... }
---
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualMachineClass
metadata:
  name: cpuX
spec:
  cpu:
    discovery: {}
    type: Discovery
  nodeSelector:
    matchExpressions:
      - key: group
        operator: In
        values: ["blue"]
  sizingPolicies: { ... }
---
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualMachineClass
metadata:
  name: cpuY
spec:
  cpu:
    discovery:
      nodeSelector:
        matchExpressions:
          - key: group
            operator: In
            values: ["green"]
    type: Discovery
  sizingPolicies: { ... }
```

### Прочие варианты конфигурации

Пример конфигурации ресурса `VirtualMachineClass`:

```yaml
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualMachineClass
metadata:
  name: discovery
spec:
  cpu:
    # сконфигурировать универсальный vCPU для заданного набора узлов
    discovery:
      nodeSelector:
        matchExpressions:
          - key: node-role.kubernetes.io/control-plane
            operator: DoesNotExist
    type: Discovery
  # разрешать запуск ВМ с данным классом только на узлах группы worker
  nodeSelector:
    matchExpressions:
      - key: node.deckhouse.io/group
        operator: In
        values:
          - worker
  # политика конфигурации ресурсов
  sizingPolicies:
    # для диапазона от 1 до 4 ядер возможно использовать от 1 до 8 Гб оперативной памяти с шагом 512Mi
    # т.е 1Гб, 1,5Гб, 2Гб, 2,5Гб итд
    # запрещено использовать выделенные ядра
    # и доступны все варианты параметра corefraction
    - cores:
        min: 1
        max: 4
      memory:
        min: 1Gi
        max: 8Gi
        step: 512Mi
      dedicatedCores: [false]
      coreFractions: [5, 10, 20, 50, 100]
    # для диапазона от 5 до 8 ядер возможно использовать от 5 до 16 Гб оперативной памяти с шагом 1Гб
    # т.е. 5Гб, 6Гб, 7Гб, итд
    # запрещено использовать выделенные ядра
    # и доступны некоторые варианты параметра corefraction
    - cores:
        min: 5
        max: 8
      memory:
        min: 5Gi
        max: 16Gi
        step: 1Gi
      dedicatedCores: [false]
      coreFractions: [20, 50, 100]
    # для диапазона от 9 до 16 ядер возможно использовать от 9 до 32 Гб оперативной памяти с шагом 1Гб
    # можно использовать выделенные ядра (а можно и не использовать)
    # и доступны некоторые варианты параметра corefraction
    - cores:
        min: 9
        max: 16
      memory:
        min: 9Gi
        max: 32Gi
        step: 1Gi
      dedicatedCores: [true, false]
      coreFractions: [50, 100]
    # для диапазона от 17 до 1024 ядер возможно использовать от 1 до 2 Гб оперативной памяти из расчета на одно ядро
    # доступны для использования только выделенные ядра
    # и единственный параметр corefraction = 100%
    - cores:
        min: 17
        max: 1024
      memory:
        perCore:
          min: 1Gi
          max: 2Gi
      dedicatedCores: [true]
      coreFractions: [100]
```

Далее приведены фрагменты конфигураций `VirtualMachineClass` для решения различных задач:

- класс с vCPU с требуемым набором процессорных инструкций, для этого используем `type: Features`, чтобы задать необходимый набор поддерживаемых инструкций для процессора:

  ```yaml
  spec:
    cpu:
      features:
        - vmx
      type: Features
  ```

- класс c универсальным vCPU для заданного набора узлов, для этого используем `type: Discovery`:

  ```yaml
  spec:
    cpu:
      discovery:
        nodeSelector:
          matchExpressions:
            - key: node-role.kubernetes.io/control-plane
              operator: DoesNotExist
      type: Discovery
  ```

- чтобы создать vCPU конкретного процессора с предварительно определенным набором инструкций, используем тип `type: Model`. Предварительно, чтобы получить перечень названий поддерживаемых CPU для узла кластера, выполните команду:

  ```bash
  kubectl get nodes <node-name> -o json | jq '.metadata.labels | to_entries[] | select(.key | test("cpu-model")) | .key | split("/")[1]' -r

  # Примерный вывод:
  #
  # IvyBridge
  # Nehalem
  # Opteron_G1
  # Penryn
  # SandyBridge
  # Westmere
  ```

далее указать в спецификации ресурса `VirtualMachineClass`:

```yaml
spec:
  cpu:
    model: IvyBridge
    type: Model
```

## Механизмы обеспечения надежности

### Миграция / Режим обслуживания

Миграция виртуальных машин является важной функцией в управлении виртуализованной инфраструктурой. Она позволяет перемещать работающие виртуальные машины с одного физического узла на другой без их отключения. Миграция виртуальных машин необходима для ряда задач и сценариев:

- Балансировка нагрузки: Перемещение виртуальных машин между узлами позволяет равномерно распределять нагрузку на серверы, обеспечивая использование ресурсов наилучшим образом.
- Перевод узла в режим обслуживания: Виртуальные машины могут быть перемещены с узлов, которые нужно вывести из эксплуатации для выполнения планового обслуживания или обновления программного обеспечения.
- Обновление "прошивки" виртуальных машин: Миграция позволяет обновить "прошивку" виртуальных машины не прерывая их работу.

#### Запуск миграции произвольной машины

Далее будет рассмотрен пример миграции выбранной виртуальной машины:

Перед запуском миграции посмотрите текущий статус виртуальной машины:

```bash
kubectl get vm
# NAME                                   PHASE     NODE           IPADDRESS     AGE
# linux-vm                              Running   virtlab-pt-1   10.66.10.14   79m
```

Мы видим что на данный момент она запущена на узле `virtlab-pt-1`.

Для осуществления миграции виртуальной машины с одного узла на другой, с учетом требований к размещению виртуальной машины используется ресурс `VirtualMachineOperations` (`vmop`) с типом migrate.

```yaml
d8 k apply -f - <<EOF
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualMachineOperation
metadata:
  name: migrate-linux-vm-$(date +%s)
spec:
  # имя виртуальной машины
  virtualMachineName: linux-vm
  # операция для миграции
  type: Migrate
EOF
```

Сразу после создания ресурса `vmop`, выполните команду:

```bash
kubectl get vm -w
# NAME                                   PHASE       NODE           IPADDRESS     AGE
# linux-vm                              Running     virtlab-pt-1   10.66.10.14   79m
# linux-vm                              Migrating   virtlab-pt-1   10.66.10.14   79m
# linux-vm                              Migrating   virtlab-pt-1   10.66.10.14   79m
# linux-vm                              Running     virtlab-pt-2   10.66.10.14   79m
```

#### Режим обслуживания

При выполнении работ на узлах с запущенными виртуальными машинами существует риск нарушения их работоспособности. Чтобы этого избежать, узел можно перевести в режим обслуживания и мигрировать виртуальные машины на другие свободные узлы.

Для этого необходимо выполнить следующую команду:

```bash
kubectl drain <nodename> --ignore-daemonsets --delete-emptydir-dat
```

где `<nodename>` - узел, на котором предполагается выполнить работы и который должен быть освобожден от всех ресурсов (в том числе и от системных).

Если есть необходимость вытеснить с узла только виртуальные машины, выполните следующую команду:

```bash
kubectl drain <nodename> --pod-selector vm.kubevirt.internal.virtualization.deckhouse.io/name --delete-emptydir-data
```

После выполнения конмад `kubectl drain` - узле перейдет в режим обслуживания и виртуальные машины на нем запускаться не смогут. Чтобы вывести его из режима обслуживания выполните следующую команду:

```bash
kubectl uncordon <nodename>
```

![](./images/drain.ru.png)

### ColdStandby

ColdStandby обеспечивает механизм восстановления работы виртуальной машины после сбоя на узле, на котором она была запущена.

Для работы данного механизма необходимо выполнит следующие требования:

- Политика запуска виртуальной машины (`.spec.runPolicy`) должна быть одна из: `AlwaysOnUnlessStoppedManually`, `AlwaysOn`.
- На узлах, где запущены виртуальные машины, должен быть включен механизм [fencing](https://deckhouse.ru/products/kubernetes-platform/documentation/v1/modules/040-node-manager/cr.html#nodegroup-v1-spec-fencing-mode).

Рассмотрим как это работает на примере:

- Кластер состоит из трех узлов master, workerA и workerB. На worker-узлах включен механизм Fencing.
- Виртуальная машина `linux-vm` запущена на узле workerA.
- На узле workerA возникает проблема (выключилось питание, пропала сеть, итд)
- Контроллер проверяет доступность узлов и обнаруживает, что workerA недоступен.
- Контроллер удаляет узел `workerA` из кластер.
- Виртуальная машина `linux-vm` запускается на другом подходящем узле (workerB).

![](./images/coldstandby.ru.png)

# Настройки Хранилищ

Хранилища применяются платформой для создания дисков `VirtualDisk`. В основе ресурсов `VirtualDisk` лежит ресурс `PersistentVolumeClaim`. При создании диска контроллер автоматически выберет наиболее оптимальные параметры, поддерживаемые хранилищем, на основании известных ему данных.

Приоритеты настройки параметров `PersistentVolumeClaim` при создании диска посредством автоматического определения характеристик хранилища:

- RWX + Block
- RWX + FileSystem
- RWO + Block
- RWO + FileSystem

Если хранилище неизвестно и определить его параметры автоматически - невозможно, используется режим: RWO + FileSystem
