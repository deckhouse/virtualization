---
title: "FAQ"
---

## Как установить ОС в виртуальной машине из ISO-образа?

Рассмотрим установку ОС в виртуальной машине из ISO-образа на примере установки ОС Windows.

Для установки ОС нам потребуется ISO-образ ОС Windows. Необходимо его загрузить и опубликовать на каком-либо HTTP-сервисе, доступном из кластера.

Создадим пустой диск для установки ОС:

```yaml
apiVersion: virtualization.deckhouse.io/v2alpha1
kind: VirtualMachineDisk
metadata:
  name: win-disk
  namespace: default
spec:
  persistentVolumeClaim:
    size: 100Gi
    storageClassName: local-path
```

Создадим ресурсы с ISO-образами ОС Windows и драйверами virtio:

```yaml
apiVersion: virtualization.deckhouse.io/v2alpha1
kind: ClusterVirtualMachineImage
metadata:
  name: win-11-iso
spec:
  dataSource:
    type: HTTP
    http:
      url: "http://example.com/win11.iso"
```

```yaml
apiVersion: virtualization.deckhouse.io/v2alpha1
kind: ClusterVirtualMachineImage
metadata:
  name: win-virtio-iso
spec:
  dataSource:
    type: HTTP
    http:
      url: "https://fedorapeople.org/groups/virt/virtio-win/direct-downloads/stable-virtio/virtio-win.iso"
```

Создадим виртуальную машину:

```yaml
apiVersion: virtualization.deckhouse.io/v2alpha1
kind: VirtualMachine
metadata:
  name: win-vm
  namespace: default
  labels:
    vm: win
spec:
  runPolicy: Manual
  osType: Windows
  bootloader: EFI
  cpu:
    cores: 6
    coreFraction: 50%
  memory:
    size: 8Gi
  enableParavirtualization: true
  blockDevices:
    - type: ClusterVirtualMachineImage
      clusterVirtualMachineImage:
        name: win-iso
    - type: ClusterVirtualMachineImage
      clusterVirtualMachineImage:
        name: win-virtio-iso
    - type: VirtualMachineDisk
      virtualMachineDisk:
        name: win-disk
```

После создания ресурса виртуальная машина будет запущена. К ней необходимо подключиться, с помощью графического установщика добавить драйверы `virtio` и выполнить установку ОС.

```bash
virtctl -n default vnc win-vm
```

После окончания установки завершить работу виртуальной машины.

Далее необходимо модифицировать ресурс `VirtualMachine` и применить изменения:

```yaml
spec:
  # ...
  runPolicy: AlwaysON
  # ...
  blockDevices:
    # Удалить из блока все ресурсы ClusterVirtualMachineImage с ISO-дисками.
    - type: VirtualMachineDisk
      virtualMachineDisk:
        name: win-disk
```

## Как создать образ виртуальной машины для container registry

Образ диска виртуальной машины, хранящийся в container registry, должен быть сформирован специальным образом.

Пример Dockerfile для создания образа:

```Dockerfile
FROM scratch
COPY image-name.img /disk/image-name.img
```

Далее необходимо собрать образ и запушить его в container registry:

```bash
docker build -t docker.io/username/image:latest

docker push docker.io/username/image:latest
```

## Как перенаправить трафик на виртуальную машину

Так как виртуальная машина работает в кластере Kubernetes, перенаправление на нее сетевого трафика осуществляется по аналогии с перенаправлением трафика через поды.

Для этого нужно всего лишь создать сервис с требуемыми настройками.

Допустим, у нас есть виртуальная машина с HTTP-сервисом, опубликованным на порте 80, и следующим набором меток:

```yaml
apiVersion: virtualization.deckhouse.io/v2alpha1
kind: VirtualMachine
metadata:
  name: web
  labels:
    vm: web
spec: ...
```

Чтобы направить сетевой трафик на 80-й порт виртуальной машины, создадим сервис:

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

Настройки меток виртуальной машины мы можем менять «на лету», то есть изменение меток не требует рестарта виртуальной машины, а это значит, что мы можем конфигурировать перенаправление сетевого трафика с разных сервисов динамически.

Представим, что мы создали новый сервис и хотим перенаправить трафик на нашу виртуальную машину с него:

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

Изменив метки на виртуальной машине, мы перенаправим на нее сетевой трафик с сервиса `svc-2`:

```yaml
metadata:
  labels:
    app: old
```
