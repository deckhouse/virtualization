---
title: "FAQ"
weight: 70
---

## Как установить ОС в виртуальной машине из ISO-образа?

Рассмотрим пример установки ОС из ISO-образа ОС Windows.
Для этого загрузите и опубликуйте его на каком-либо HTTP-сервисе, доступном из кластера.

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
        - kind: VirtualDisk
          name: win-disk
        - kind: ClusterVirtualImage
          name: win-11-iso
        - kind: ClusterVirtualImage
          name: win-virtio-iso
    ```

1. После создания ресурса запустите ВМ:

   ```bash
   d8 v start win-vm
   ```

1. К ней необходимо подключиться и с помощью графического установщика
выполнить установку ОС и драйверов `virtio`.

   Команда для подключения:

   ```bash
   d8 v vnc -n default win-vm
   ```

1. После окончания установки перезагрузите виртуальную машину.

1. Для продолжения работы с виртуальной машиной также используйте команду:

   ```bash
   d8 v vnc -n default win-vm
   ```

## Как предоставить файл ответов Windows(Sysprep)?

Чтобы выполнить автоматическую установку Windows,
создайте файл ответов (обычно именуются unattend.xml или autounattend.xml).
Для примера возьмем файл, позволяющий:

- Добавить русский язык и раскладку;
- Указать расположение virtio драйверов необходимых для установки
  (поэтому важен порядок дисковых устройств в спецификации ВМ);
- Разметить диски для установки windows на ВМ c EFI;
- Создать в группе администраторов пользователя *cloud* с паролем *cloud*;
- Создать непривилегированного пользователя *user* с паролем *user*.

<details><summary><b>autounattend.xml</b></summary>

```xml
<?xml version="1.0" encoding="utf-8"?>
<unattend xmlns="urn:schemas-microsoft-com:unattend" xmlns:wcm="http://schemas.microsoft.com/WMIConfig/2002/State">
  <settings pass="offlineServicing"></settings>
  <settings pass="windowsPE">
    <component name="Microsoft-Windows-International-Core-WinPE" processorArchitecture="amd64" publicKeyToken="31bf3856ad364e35" language="neutral" versionScope="nonSxS">
      <SetupUILanguage>
        <UILanguage>ru-RU</UILanguage>
      </SetupUILanguage>
      <InputLocale>0409:00000409;0419:00000419</InputLocale>
      <SystemLocale>en-US</SystemLocale>
      <UILanguage>ru-RU</UILanguage>
      <UserLocale>en-US</UserLocale>
    </component>
    <component name="Microsoft-Windows-PnpCustomizationsWinPE" processorArchitecture="amd64" publicKeyToken="31bf3856ad364e35" language="neutral" versionScope="nonSxS">
      <DriverPaths>
        <PathAndCredentials wcm:keyValue="4b29ba63" wcm:action="add">
          <Path>E:\amd64\w11</Path>
        </PathAndCredentials>
        <PathAndCredentials wcm:keyValue="25fe51ea" wcm:action="add">
          <Path>E:\NetKVM\w11\amd64</Path>
        </PathAndCredentials>
      </DriverPaths>
    </component>
    <component name="Microsoft-Windows-Setup" processorArchitecture="amd64" publicKeyToken="31bf3856ad364e35" language="neutral" versionScope="nonSxS">
      <DiskConfiguration>
        <Disk wcm:action="add">
          <DiskID>0</DiskID>
          <WillWipeDisk>true</WillWipeDisk>
          <CreatePartitions>
            <!-- Recovery partition -->
            <CreatePartition wcm:action="add">
              <Order>1</Order>
              <Type>Primary</Type>
              <Size>250</Size>
            </CreatePartition>
            <!-- EFI system partition (ESP) -->
            <CreatePartition wcm:action="add">
              <Order>2</Order>
              <Type>EFI</Type>
              <Size>100</Size>
            </CreatePartition>
            <!-- Microsoft reserved partition (MSR) -->
            <CreatePartition wcm:action="add">
              <Order>3</Order>
              <Type>MSR</Type>
              <Size>128</Size>
            </CreatePartition>
            <!-- Windows partition -->
            <CreatePartition wcm:action="add">
              <Order>4</Order>
              <Type>Primary</Type>
              <Extend>true</Extend>
            </CreatePartition>
          </CreatePartitions>
          <ModifyPartitions>
            <!-- Recovery partition -->
            <ModifyPartition wcm:action="add">
              <Order>1</Order>
              <PartitionID>1</PartitionID>
              <Label>Recovery</Label>
              <Format>NTFS</Format>
              <TypeID>de94bba4-06d1-4d40-a16a-bfd50179d6ac</TypeID>
            </ModifyPartition>
            <!-- EFI system partition (ESP) -->
            <ModifyPartition wcm:action="add">
              <Order>2</Order>
              <PartitionID>2</PartitionID>
              <Label>System</Label>
              <Format>FAT32</Format>
            </ModifyPartition>
            <!-- MSR partition does not need to be modified -->
            <!-- Windows partition -->
            <ModifyPartition wcm:action="add">
              <Order>3</Order>
              <PartitionID>4</PartitionID>
              <Label>Windows</Label>
              <Letter>C</Letter>
              <Format>NTFS</Format>
            </ModifyPartition>
          </ModifyPartitions>
        </Disk>
        <WillShowUI>OnError</WillShowUI>
      </DiskConfiguration>
      <ImageInstall>
        <OSImage>
          <InstallTo>
            <DiskID>0</DiskID>
            <PartitionID>4</PartitionID>
          </InstallTo>
        </OSImage>
      </ImageInstall>
      <UserData>
        <ProductKey>
          <Key>VK7JG-NPHTM-C97JM-9MPGT-3V66T</Key>
          <WillShowUI>OnError</WillShowUI>
        </ProductKey>
        <AcceptEula>true</AcceptEula>
      </UserData>
      <UseConfigurationSet>false</UseConfigurationSet>
    </component>
  </settings>
  <settings pass="generalize"></settings>
  <settings pass="specialize">
    <component name="Microsoft-Windows-Deployment" processorArchitecture="amd64" publicKeyToken="31bf3856ad364e35" language="neutral" versionScope="nonSxS">
      <RunSynchronous>
        <RunSynchronousCommand wcm:action="add">
          <Order>1</Order>
          <Path>powershell.exe -NoProfile -Command "$xml = [xml]::new(); $xml.Load('C:\Windows\Panther\unattend.xml'); $sb = [scriptblock]::Create( $xml.unattend.Extensions.ExtractScript ); Invoke-Command -ScriptBlock $sb -ArgumentList $xml;"</Path>
        </RunSynchronousCommand>
        <RunSynchronousCommand wcm:action="add">
          <Order>2</Order>
          <Path>powershell.exe -NoProfile -Command "Get-Content -LiteralPath 'C:\Windows\Setup\Scripts\Specialize.ps1' -Raw | Invoke-Expression;"</Path>
        </RunSynchronousCommand>
        <RunSynchronousCommand wcm:action="add">
          <Order>3</Order>
          <Path>reg.exe load "HKU\DefaultUser" "C:\Users\Default\NTUSER.DAT"</Path>
        </RunSynchronousCommand>
        <RunSynchronousCommand wcm:action="add">
          <Order>4</Order>
          <Path>powershell.exe -NoProfile -Command "Get-Content -LiteralPath 'C:\Windows\Setup\Scripts\DefaultUser.ps1' -Raw | Invoke-Expression;"</Path>
        </RunSynchronousCommand>
        <RunSynchronousCommand wcm:action="add">
          <Order>5</Order>
          <Path>reg.exe unload "HKU\DefaultUser"</Path>
        </RunSynchronousCommand>
      </RunSynchronous>
    </component>
  </settings>
  <settings pass="auditSystem"></settings>
  <settings pass="auditUser"></settings>
  <settings pass="oobeSystem">
    <component name="Microsoft-Windows-International-Core" processorArchitecture="amd64" publicKeyToken="31bf3856ad364e35" language="neutral" versionScope="nonSxS">
      <InputLocale>0409:00000409;0419:00000419</InputLocale>
      <SystemLocale>en-US</SystemLocale>
      <UILanguage>ru-RU</UILanguage>
      <UserLocale>en-US</UserLocale>
    </component>
    <component name="Microsoft-Windows-Shell-Setup" processorArchitecture="amd64" publicKeyToken="31bf3856ad364e35" language="neutral" versionScope="nonSxS">
      <UserAccounts>
        <LocalAccounts>
          <LocalAccount wcm:action="add">
            <Name>cloud</Name>
            <DisplayName>cloud</DisplayName>
            <Group>Administrators</Group>
            <Password>
              <Value>cloud</Value>
              <PlainText>true</PlainText>
            </Password>
          </LocalAccount>
          <LocalAccount wcm:action="add">
            <Name>User</Name>
            <DisplayName>user</DisplayName>
            <Group>Users</Group>
            <Password>
              <Value>user</Value>
              <PlainText>true</PlainText>
            </Password>
          </LocalAccount>
        </LocalAccounts>
      </UserAccounts>
      <AutoLogon>
        <Username>cloud</Username>
        <Enabled>true</Enabled>
        <LogonCount>1</LogonCount>
        <Password>
          <Value>cloud</Value>
          <PlainText>true</PlainText>
        </Password>
      </AutoLogon>
      <OOBE>
        <ProtectYourPC>3</ProtectYourPC>
        <HideEULAPage>true</HideEULAPage>
        <HideWirelessSetupInOOBE>true</HideWirelessSetupInOOBE>
        <HideOnlineAccountScreens>false</HideOnlineAccountScreens>
      </OOBE>
      <FirstLogonCommands>
        <SynchronousCommand wcm:action="add">
          <Order>1</Order>
          <CommandLine>powershell.exe -NoProfile -Command "Get-Content -LiteralPath 'C:\Windows\Setup\Scripts\FirstLogon.ps1' -Raw | Invoke-Expression;"</CommandLine>
        </SynchronousCommand>
      </FirstLogonCommands>
    </component>
  </settings>
</unattend>
```

</details>

Создайте секрет из этого xml файла:

```bash
d8 k create secret generic sysprep-config --type="provisioning.virtualization.deckhouse.io/sysprep" --from-file=./autounattend.xml
```

Затем можно создать виртуальную машину, которая в процессе установки будет использовать файл ответов.

Чтобы предоставить виртуальной машине Windows файл ответов, необходимо указать provisioning с типом SysprepRef.
Вы также можете указать здесь другие файлы в формате base64, необходимые для успешного выполнения скриптов внутри файла ответов.

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
    - kind: VirtualDisk
      name: win-disk
    - kind: ClusterVirtualImage
      name: win-11-iso
    - kind: ClusterVirtualImage
      name: win-virtio-iso
```

## Как использовать Ansible для конфигурирования виртуальных машин?

[Ansible](https://docs.ansible.com/ansible/latest/index.html) — это инструмент автоматизации, который позволяет выполнять задачи на удаленных серверах с использованием протокола SSH. В данном примере мы рассмотрим, как использовать Ansible для управления виртуальными машинами расположенных в проекте demo-app.

В рамках примера предполагается, что:

- У вас есть виртуальная машина с именем frontend в проекте demo-app.
- На виртуальной машине создан пользователь cloud для доступа по SSH.
- Приватный SSH-ключ пользователя хранится в файле ./tmp/demo на сервере Ansible.

Пример inventory-файла:

```yaml
---
all:
  vars:
    ansible_ssh_common_args: '-o ProxyCommand="d8 v port-forward --stdio=true %h %p"'
    # Пользователь по умолчанию, для доступа по SSH.
    ansible_user: cloud
    # Путь к приватному ключу.
    ansible_ssh_private_key_file: ./tmp/demo
  hosts:
    # Название узла в формате <название ВМ>.<название проекта>.
    frontend.demo-app:

```

Чтобы проверить значение аптайма виртуальной машины, используйте следующую команду:

```bash
ansible -m shell -a "uptime" -i inventory.yaml all

# frontend.demo-app | CHANGED | rc=0 >>
# 12:01:20 up 2 days,  4:59,  0 users,  load average: 0.00, 0.00, 0.00
```

Если вы не хотите использовать файл inventory, можно передать все параметры прямо в командной строке:

```bash
ansible -m shell -a "uptime" \
  -i "frontend.demo-app," \
  -e "ansible_ssh_common_args='-o ProxyCommand=\"d8 v port-forward --stdio=true %h %p\"'" \
  -e "ansible_user=cloud" \
  -e "ansible_ssh_private_key_file=./tmp/demo" \
  all
```

## Как перенаправить трафик на виртуальную машину?

Виртуальная машина функционирует в кластере Kubernetes, поэтому направление сетевого трафика к ней осуществляется аналогично направлению трафика к подам. Для маршрутизации сетевого трафика на виртуальную машину применяется стандартный механизм Kubernetes — ресурс Service, который выбирает целевые объекты по меткам (label selector).

1. Создайте сервис с требуемыми настройками.

   В качестве примера приведена виртуальная машина с меткой `vm: frontend-0`, HTTP-сервисом, опубликованным на портах 80 и 443, и открытым SSH на порту 22:

    ```yaml
    apiVersion: virtualization.deckhouse.io/v1alpha2
    kind: VirtualMachine
    metadata:
      name: frontend-0
      namespace: dev
      labels:
        vm: frontend-0
    spec: ...
    ```

1. Чтобы направить сетевой трафик на порты виртуальной машины, создайте Service:

   Следующий Service обеспечивает доступ к виртуальной машине: он слушает порты 80 и 443 и перенаправляет трафик на соответствующие порты целевой виртуальной машины. SSH-доступ извне предоставляется по порту 2211:

    ```yaml
    apiVersion: v1
    kind: Service
    metadata:
      name: frontend-0-svc
      namespace: dev
    spec:
      type: LoadBalancer
      ports:
      - name: ssh
        port: 2211
        protocol: TCP
        targetPort: 22
      - name: http
        port: 80
        protocol: TCP
        targetPort: 80
      - name: https
        port: 443
        protocol: TCP
        targetPort: 443
      selector:
        vm: frontend-0
    ```

## Как увеличить размер DVCR?

Чтобы увеличить размер диска для DVCR, необходимо установить больший размер в конфигурации модуля `virtualization`, чем текущий размер.

1. Проверьте текущий размер DVCR:

    ```shell
    d8 k get mc virtualization -o jsonpath='{.spec.settings.dvcr.storage.persistentVolumeClaim}'
    ```

   Пример вывода:

   ```txt
    {"size":"58G","storageClass":"linstor-thick-data-r1"}
    ```

1. Задайте размер:

    ```shell
    d8 k patch mc virtualization \
      --type merge -p '{"spec": {"settings": {"dvcr": {"storage": {"persistentVolumeClaim": {"size":"59G"}}}}}}'
    ```

   Пример вывода:

   ```txt
    moduleconfig.deckhouse.io/virtualization patched
    ```

1. Проверьте изменение размера:

    ```shell
    d8 k get mc virtualization -o jsonpath='{.spec.settings.dvcr.storage.persistentVolumeClaim}'
    ```

   Пример вывода:

    ```txt
    {"size":"59G","storageClass":"linstor-thick-data-r1"}
    ```

1. Проверьте текущее состояние DVCR:

    ```shell
    d8 k get pvc dvcr -n d8-virtualization
    ```

   Пример вывода:

    ```txt
    NAME STATUS VOLUME                                    CAPACITY    ACCESS MODES   STORAGECLASS           AGE
    dvcr Bound  pvc-6a6cedb8-1292-4440-b789-5cc9d15bbc6b  57617188Ki  RWO            linstor-thick-data-r1  7d
    ```

## Как создать golden image для Linux?

Golden image — это предварительно настроенный образ виртуальной машины, который можно использовать для быстрого создания новых ВМ с уже установленным программным обеспечением и настройками.

1. Создайте виртуальную машину, установите на неё необходимое программное обеспечение и выполните все требуемые настройки.

1. Установите и настройте qemu-guest-agent (рекомендуется):

   - Для RHEL/CentOS:

     ```bash
     yum install -y qemu-guest-agent
     ```

   - Для Debian/Ubuntu:

     ```bash
     apt-get update
     apt-get install -y qemu-guest-agent
     ```

1. Включите и запустите сервис:

   ```bash
   systemctl enable qemu-guest-agent
   systemctl start qemu-guest-agent
   ```

1. Установите политику запуска ВМ [runPolicy: AlwaysOnUnlessStoppedManually](/modules/virtualization/stable/cr.html#virtualmachine-v1alpha2-spec-runpolicy) — это потребуется, чтобы ВМ можно было выключить.

1. Подготовьте образ. Очистите неиспользуемые блоки файловой системы:

   ```bash
   fstrim -v /
   fstrim -v /boot
   ```

1. Очистите сетевые настройки:

   - Для RHEL:

     ```bash
     nmcli con delete $(nmcli -t -f NAME,DEVICE con show | grep -v ^lo: | cut -d: -f1)
     rm -f /etc/sysconfig/network-scripts/ifcfg-eth*
     ```

   - Для Debian/Ubuntu:

     ```bash
     rm -f /etc/network/interfaces.d/*
     ```

1. Очистите системные идентификаторы:

   ```bash
   echo -n > /etc/machine-id
   rm -f /var/lib/dbus/machine-id
   ln -s /etc/machine-id /var/lib/dbus/machine-id
   ```

1. Удалите SSH host keys:

   ```bash
   rm -f /etc/ssh/ssh_host_*
   ```

1. Очистите systemd journal:

   ```bash
   journalctl --vacuum-size=100M --vacuum-time=7d
   ```

1. Очистите кэш пакетных менеджеров:

   - Для RHEL:

     ```bash
     yum clean all
     ```

   - Для Debian/Ubuntu:

     ```bash
     apt-get clean
     ```

1. Очистите временные файлы:

   ```bash
   rm -rf /tmp/*
   rm -rf /var/tmp/*
   ```

1. Очистите логи:

   ```bash
   find /var/log -name "*.log" -type f -exec truncate -s 0 {} \;
   ```

1. Очистите историю команд:

   ```bash
   history -c
   ```

   Для RHEL: выполните сброс и восстановление контекстов SELinux (выберите один из вариантов):

   - Вариант 1: Проверка и восстановление контекстов сразу:

     ```bash
     restorecon -R /
     ```

   - Вариант 2: Запланировать relabel при следующей загрузке:

     ```bash
     touch /.autorelabel
     ```

1. Убедитесь, что в `/etc/fstab` используются UUID или LABEL вместо имён устройств (например, `/dev/sdX`). Для проверки выполните:

   ```bash
   blkid
   cat /etc/fstab
   ```

1. Очистите состояние cloud-init, логи и seed (рекомендуемый способ):

   ```bash
   cloud-init clean --logs --seed
   ```

1. Выполните финальную синхронизацию и очистку буферов:

   ```bash
   sync
   echo 3 > /proc/sys/vm/drop_caches
   ```

1. Выключите виртуальную машину:

   ```bash
   poweroff
   ```

1. Создайте ресурс `VirtualImage` из диска подготовленной ВМ:

   ```bash
   d8 k apply -f -<<EOF
   apiVersion: virtualization.deckhouse.io/v1alpha2
   kind: VirtualImage
   metadata:
     name: <image-name>
     namespace: <namespace>
   spec:
     dataSource:
       type: ObjectRef
       objectRef:
         kind: VirtualDisk
         name: <source-disk-name>
   EOF
   ```

   Альтернативно, создайте `ClusterVirtualImage`, чтобы образ был доступен на уровне кластера для всех проектов:

    ```bash
    d8 k apply -f -<<EOF
    apiVersion: virtualization.deckhouse.io/v1alpha2
    kind: ClusterVirtualImage
    metadata:
      name: <image-name>
    spec:
      dataSource:
        type: ObjectRef
        objectRef:
          kind: VirtualDisk
          name: <source-disk-name>
          namespace: <namespace>
    EOF
    ```

1. Создайте диск ВМ из созданного образа:

   ```bash
   d8 k apply -f -<<EOF
   apiVersion: virtualization.deckhouse.io/v1alpha2
   kind: VirtualDisk
   metadata:
     name: <vm-disk-name>
     namespace: <namespace>
   spec:
     dataSource:
       type: ObjectRef
       objectRef:
         kind: VirtualImage
         name: <image-name>
   EOF
   ```

После выполнения этих шагов у вас будет golden image, который можно использовать для быстрого создания новых виртуальных машин с предустановленным программным обеспечением и настройками.
