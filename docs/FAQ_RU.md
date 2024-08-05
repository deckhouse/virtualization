---
title: "FAQ"
---

## Как установить ОС в виртуальной машине из ISO-образа?

**Установка ОС в виртуальной машине из ISO-образа на примере установки ОС Windows**

Для установки ОС необходим ISO-образ ОС Windows. Для этого загрузите и опубликуйте его на каком-либо HTTP-сервисе, доступном из кластера.

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

2. Создайте ресурсы с ISO-образами ОС Windows и драйверами virtio:

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

3. Создайте виртуальную машину:

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

После создания ресурса виртуальная машина будет запущена. К ней необходимо подключиться, и с помощью графического установщика выполнить установку ОС и драйверов `virtio`.

Команда для подключения:

```bash
d8 v vnc -n default win-vm
```

6. После окончания установки завершите работу виртуальной машины.

7. Модифицируйте ресурс `VirtualMachine` и примените изменения:

```yaml
spec:
  # ...
  runPolicy: AlwaysON
  # ...
  blockDeviceRefs:
    # Удалить из блока все ресурсы ClusterVirtualImage с ISO-дисками.
    - kind: VirtualDisk
      name: win-disk
```

8. После внесенных изменений виртуальная машина запустится, для продолжения работы с ней используйте команду:

```bash
d8 v vnc -n default win-vm
```

## Как создать образ виртуальной машины для container registry

Образ диска виртуальной машины, хранящийся в container registry, должен быть сформирован специальным образом.

Пример Dockerfile для создания образа:

```Dockerfile
FROM scratch
COPY image-name.img /disk/image-name.img
```

1. Соберите образ и отправьте его в `container registry`:

```bash
docker build -t docker.io/username/image:latest

docker push docker.io/username/image:latest
```

## Как перенаправить трафик на виртуальную машину

Так как виртуальная машина функционирует в кластере Kubernetes, направление сетевого трафика осуществляется аналогично направлению трафика на поды.

1. Для этого создайте сервис с требуемыми настройками.

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

2. Чтобы направить сетевой трафик на 80-й порт виртуальной машины, создайте сервис:

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

# Как увеличить размер DVCR

Чтобы увеличить размер диска для DVCR, необходимо установить больший размер в конфигурации модуля `virtualization`, чем текущий размер.

1. Проверьте текущий размер dvcr:

```shell
kubectl get mc virtualization -o jsonpath='{.spec.settings.dvcr.storage.persistentVolumeClaim}'
#Output
{"size":"58G","storageClass":"linstor-thick-data-r1"}
```

2. Задайте размер:

```shell
kubectl patch mc virtualization \
  --type merge -p '{"spec": {"settings": {"dvcr": {"storage": {"persistentVolumeClaim": {"size":"59G"}}}}}}'

#Output
moduleconfig.deckhouse.io/virtualization patched
```

3. Проверьте изменение размера:

```shell
kubectl get mc virtualization -o jsonpath='{.spec.settings.dvcr.storage.persistentVolumeClaim}'
#Output
{"size":"59G","storageClass":"linstor-thick-data-r1"}

kubectl get pvc dvcr -n d8-virtualization
#Output
NAME   STATUS   VOLUME                                     CAPACITY     ACCESS MODES   STORAGECLASS            AGE
dvcr   Bound    pvc-6a6cedb8-1292-4440-b789-5cc9d15bbc6b   57617188Ki   RWO            linstor-thick-data-r1   7d
```

## Как предоставить файл ответов windows(Sysprep)

Чтобы предоставить виртуальной машине windows файл ответов необходимо указать provisioning с типом SysprepRef.

Прежде всего необходимо создать секрет:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: sysprep-config
data:
  unattend.xml: XXXx # base64 файла ответов
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

## Использование виртуализации совместно с модулем Admission-policy-engine

Для начала рекомендуется ознакомиться с настройками модуля [Admission-policy-engine](https://deckhouse.ru/documentation/v1/modules/015-admission-policy-engine/).  
При настройке политик безопасности рекомендуется руководствоваться политиками безопасности, которые установленны в вашей компании.

Рассмотрим на примере включенной политики Baseline.  
Так как Baseline не позволяет по умолчанию запускать Pod виртуальной машины из-за повышенных привилегий, необходимых для корректной работы виртуальной машины, то потребуется вручную настроить пространства имён, в которых они будут запускаться.

1. Исключение пространства имён из политики Baseline.  
```yaml
apiVersion: v1
kind: Namespace
metadata:
  labels:
    kubernetes.io/metadata.name: <название пространства имён>
    security.deckhouse.io/pod-policy: privileged
  name: <название пространства имён>
spec:
  finalizers:
  - kubernetes
```
2. Настройка новой политики безопасности.  
Эта политика основана на Baseline и позволяет запускать виртуальные машины в заданном прострастве имён.
```yaml
---
apiVersion: deckhouse.io/v1alpha1
kind: SecurityPolicy
metadata:
  name: virt-launcher-deny
spec:
  enforcementAction: Deny
  match:
    namespaceSelector:
      labelSelector:
        matchLabels:
          kubernetes.io/metadata.name: <название пространства имён>
    labelSelector:
      matchLabels:
        kubevirt.internal.virtualization.deckhouse.io: virt-launcher
  policies:
    allowPrivilegeEscalation: true
    allowedCapabilities:
      - NET_BIND_SERVICE
      - SYS_NICE
    runAsUser:
      ranges:
        - max: 0
          min: 0
      rule: MustRunAs
---
apiVersion: deckhouse.io/v1alpha1
kind: SecurityPolicy
metadata:
  name: other-deny
spec:
  enforcementAction: Deny
  match:
    namespaceSelector:
      labelSelector:
        matchLabels:
          kubernetes.io/metadata.name: <название пространства имён>
    labelSelector:
      matchExpressions:
        - key: kubevirt.internal.virtualization.deckhouse.io
          operator: NotIn
          values:
            - virt-launcher
  policies:
    allowedCapabilities:
      - AUDIT_WRITE
      - CHOWN
      - DAC_OVERRIDE
      - FOWNER
      - FSETID
      - KILL
      - MKNOD
      - NET_BIND_SERVICE
      - SETFCAP
      - SETGID
      - SETPCAP
      - SETUID
      - SYS_CHROOT
    allowedProcMount: Default
    seccompProfiles:
      allowedProfiles:
        - RuntimeDefault
        - Localhost
        - ""
        - undefined
      allowedLocalhostFiles:
        - '*'
    allowedUnsafeSysctls:
      - kernel.shm_rmid_forced
      - net.ipv4.ip_local_port_range
      - net.ipv4.ip_unprivileged_port_start
      - net.ipv4.tcp_syncookies
      - net.ipv4.ping_group_range
    allowHostNetwork: false
    allowPrivileged: false
    allowPrivilegeEscalation: false
    seLinux:
      - type: container_t
      - type: container_init_t
      - type: container_kvm_t
      - level: s0
    runAsUser:
      rule: MustRunAsNonRoot
```
