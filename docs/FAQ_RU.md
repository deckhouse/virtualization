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

1. После создания ресурса виртуальная машина будет запущена.
К ней необходимо подключиться, и с помощью графического установщика
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

## Как предоставить файл ответов Windows(Sysprep)

Чтобы выполнять автоматическую установку Windows,
закодируйте файл ответов (обычно именуются unattend.xml или autounattend.xml) в base64 и внесите в секрет.

autounattend.xml

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

sysprep-config.yml

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: sysprep-config
type: "provisioning.virtualization.deckhouse.io/sysprep"
data:
  autounattend.xml: |
    PD94bWwgdmVyc2lvbj0iMS4wIiBlbmNvZGluZz0idXRmLTgiPz4KPHVuYXR0ZW5kIHhtbG
    5zPSJ1cm46c2NoZW1hcy1taWNyb3NvZnQtY29tOnVuYXR0ZW5kIiB4bWxuczp3Y209Imh0
    dHA6Ly9zY2hlbWFzLm1pY3Jvc29mdC5jb20vV01JQ29uZmlnLzIwMDIvU3RhdGUiPgogID
    xzZXR0aW5ncyBwYXNzPSJvZmZsaW5lU2VydmljaW5nIj48L3NldHRpbmdzPgogIDxzZXR0
    aW5ncyBwYXNzPSJ3aW5kb3dzUEUiPgogICAgPGNvbXBvbmVudCBuYW1lPSJNaWNyb3NvZn
    QtV2luZG93cy1JbnRlcm5hdGlvbmFsLUNvcmUtV2luUEUiIHByb2Nlc3NvckFyY2hpdGVj
    dHVyZT0iYW1kNjQiIHB1YmxpY0tleVRva2VuPSIzMWJmMzg1NmFkMzY0ZTM1IiBsYW5ndW
    FnZT0ibmV1dHJhbCIgdmVyc2lvblNjb3BlPSJub25TeFMiPgogICAgICA8U2V0dXBVSUxh
    bmd1YWdlPgogICAgICAgIDxVSUxhbmd1YWdlPnJ1LVJVPC9VSUxhbmd1YWdlPgogICAgIC
    A8L1NldHVwVUlMYW5ndWFnZT4KICAgICAgPElucHV0TG9jYWxlPjA0MDk6MDAwMDA0MDk7
    MDQxOTowMDAwMDQxOTwvSW5wdXRMb2NhbGU+CiAgICAgIDxTeXN0ZW1Mb2NhbGU+ZW4tVV
    M8L1N5c3RlbUxvY2FsZT4KICAgICAgPFVJTGFuZ3VhZ2U+cnUtUlU8L1VJTGFuZ3VhZ2U+
    CiAgICAgIDxVc2VyTG9jYWxlPmVuLVVTPC9Vc2VyTG9jYWxlPgogICAgPC9jb21wb25lbn
    Q+CiAgICA8Y29tcG9uZW50IG5hbWU9Ik1pY3Jvc29mdC1XaW5kb3dzLVBucEN1c3RvbWl6
    YXRpb25zV2luUEUiIHByb2Nlc3NvckFyY2hpdGVjdHVyZT0iYW1kNjQiIHB1YmxpY0tleV
    Rva2VuPSIzMWJmMzg1NmFkMzY0ZTM1IiBsYW5ndWFnZT0ibmV1dHJhbCIgdmVyc2lvblNj
    b3BlPSJub25TeFMiPgogICAgICA8RHJpdmVyUGF0aHM+CiAgICAgICAgPFBhdGhBbmRDcm
    VkZW50aWFscyB3Y206a2V5VmFsdWU9IjRiMjliYTYzIiB3Y206YWN0aW9uPSJhZGQiPgog
    ICAgICAgICAgPFBhdGg+RTpcYW1kNjRcdzExPC9QYXRoPgogICAgICAgIDwvUGF0aEFuZE
    NyZWRlbnRpYWxzPgogICAgICAgIDxQYXRoQW5kQ3JlZGVudGlhbHMgd2NtOmtleVZhbHVl
    PSIyNWZlNTFlYSIgd2NtOmFjdGlvbj0iYWRkIj4KICAgICAgICAgIDxQYXRoPkU6XE5ldE
    tWTVx3MTFcYW1kNjQ8L1BhdGg+CiAgICAgICAgPC9QYXRoQW5kQ3JlZGVudGlhbHM+CiAg
    ICAgIDwvRHJpdmVyUGF0aHM+CiAgICA8L2NvbXBvbmVudD4KICAgIDxjb21wb25lbnQgbm
    FtZT0iTWljcm9zb2Z0LVdpbmRvd3MtU2V0dXAiIHByb2Nlc3NvckFyY2hpdGVjdHVyZT0i
    YW1kNjQiIHB1YmxpY0tleVRva2VuPSIzMWJmMzg1NmFkMzY0ZTM1IiBsYW5ndWFnZT0ibm
    V1dHJhbCIgdmVyc2lvblNjb3BlPSJub25TeFMiPgogICAgICA8RGlza0NvbmZpZ3VyYXRp
    b24+CiAgICAgICAgPERpc2sgd2NtOmFjdGlvbj0iYWRkIj4KICAgICAgICAgIDxEaXNrSU
    Q+MDwvRGlza0lEPiAKICAgICAgICAgIDxXaWxsV2lwZURpc2s+dHJ1ZTwvV2lsbFdpcGVE
    aXNrPiAKICAgICAgICAgIDxDcmVhdGVQYXJ0aXRpb25zPgogICAgICAgICAgICA8IS0tIF
    JlY292ZXJ5IHBhcnRpdGlvbiAtLT4KICAgICAgICAgICAgPENyZWF0ZVBhcnRpdGlvbiB3
    Y206YWN0aW9uPSJhZGQiPgogICAgICAgICAgICAgIDxPcmRlcj4xPC9PcmRlcj4gCiAgIC
    AgICAgICAgICAgPFR5cGU+UHJpbWFyeTwvVHlwZT4gCiAgICAgICAgICAgICAgPFNpemU+
    MjUwPC9TaXplPiAKICAgICAgICAgICAgPC9DcmVhdGVQYXJ0aXRpb24+CiAgICAgICAgIC
    AgIDwhLS0gRUZJIHN5c3RlbSBwYXJ0aXRpb24gKEVTUCkgLS0+CiAgICAgICAgICAgIDxD
    cmVhdGVQYXJ0aXRpb24gd2NtOmFjdGlvbj0iYWRkIj4KICAgICAgICAgICAgICA8T3JkZX
    I+MjwvT3JkZXI+IAogICAgICAgICAgICAgIDxUeXBlPkVGSTwvVHlwZT4gCiAgICAgICAg
    ICAgICAgPFNpemU+MTAwPC9TaXplPiAKICAgICAgICAgICAgPC9DcmVhdGVQYXJ0aXRpb2
    4+CiAgICAgICAgICAgIDwhLS0gTWljcm9zb2Z0IHJlc2VydmVkIHBhcnRpdGlvbiAoTVNS
    KSAtLT4KICAgICAgICAgICAgPENyZWF0ZVBhcnRpdGlvbiB3Y206YWN0aW9uPSJhZGQiPg
    ogICAgICAgICAgICAgIDxPcmRlcj4zPC9PcmRlcj4gCiAgICAgICAgICAgICAgPFR5cGU+
    TVNSPC9UeXBlPiAKICAgICAgICAgICAgICA8U2l6ZT4xMjg8L1NpemU+IAogICAgICAgIC
    AgICA8L0NyZWF0ZVBhcnRpdGlvbj4KICAgICAgICAgICAgPCEtLSBXaW5kb3dzIHBhcnRp
    dGlvbiAtLT4KICAgICAgICAgICAgPENyZWF0ZVBhcnRpdGlvbiB3Y206YWN0aW9uPSJhZG
    QiPgogICAgICAgICAgICAgIDxPcmRlcj40PC9PcmRlcj4gCiAgICAgICAgICAgICAgPFR5
    cGU+UHJpbWFyeTwvVHlwZT4gCiAgICAgICAgICAgICAgPEV4dGVuZD50cnVlPC9FeHRlbm
    Q+IAogICAgICAgICAgICA8L0NyZWF0ZVBhcnRpdGlvbj4KICAgICAgICAgIDwvQ3JlYXRl
    UGFydGl0aW9ucz4KICAgICAgICAgIDxNb2RpZnlQYXJ0aXRpb25zPgogICAgICAgICAgIC
    A8IS0tIFJlY292ZXJ5IHBhcnRpdGlvbiAtLT4KICAgICAgICAgICAgPE1vZGlmeVBhcnRp
    dGlvbiB3Y206YWN0aW9uPSJhZGQiPgogICAgICAgICAgICAgIDxPcmRlcj4xPC9PcmRlcj
    4gCiAgICAgICAgICAgICAgPFBhcnRpdGlvbklEPjE8L1BhcnRpdGlvbklEPiAKICAgICAg
    ICAgICAgICA8TGFiZWw+UmVjb3Zlcnk8L0xhYmVsPiAKICAgICAgICAgICAgICA8Rm9ybW
    F0Pk5URlM8L0Zvcm1hdD4gCiAgICAgICAgICAgICAgPFR5cGVJRD5kZTk0YmJhNC0wNmQx
    LTRkNDAtYTE2YS1iZmQ1MDE3OWQ2YWM8L1R5cGVJRD4gCiAgICAgICAgICAgIDwvTW9kaW
    Z5UGFydGl0aW9uPgogICAgICAgICAgICA8IS0tIEVGSSBzeXN0ZW0gcGFydGl0aW9uIChF
    U1ApIC0tPgogICAgICAgICAgICA8TW9kaWZ5UGFydGl0aW9uIHdjbTphY3Rpb249ImFkZC
    I+CiAgICAgICAgICAgICAgPE9yZGVyPjI8L09yZGVyPgogICAgICAgICAgICAgIDxQYXJ0
    aXRpb25JRD4yPC9QYXJ0aXRpb25JRD4KICAgICAgICAgICAgICA8TGFiZWw+U3lzdGVtPC
    9MYWJlbD4KICAgICAgICAgICAgICA8Rm9ybWF0PkZBVDMyPC9Gb3JtYXQ+CiAgICAgICAg
    ICAgIDwvTW9kaWZ5UGFydGl0aW9uPgogICAgICAgICAgICA8IS0tIE1TUiBwYXJ0aXRpb2
    4gZG9lcyBub3QgbmVlZCB0byBiZSBtb2RpZmllZCAtLT4KICAgICAgICAgICAgPCEtLSBX
    aW5kb3dzIHBhcnRpdGlvbiAtLT4KICAgICAgICAgICAgPE1vZGlmeVBhcnRpdGlvbiB3Y2
    06YWN0aW9uPSJhZGQiPgogICAgICAgICAgICAgIDxPcmRlcj4zPC9PcmRlcj4KICAgICAg
    ICAgICAgICA8UGFydGl0aW9uSUQ+NDwvUGFydGl0aW9uSUQ+CiAgICAgICAgICAgICAgPE
    xhYmVsPldpbmRvd3M8L0xhYmVsPgogICAgICAgICAgICAgIDxMZXR0ZXI+QzwvTGV0dGVy
    PgogICAgICAgICAgICAgIDxGb3JtYXQ+TlRGUzwvRm9ybWF0PgogICAgICAgICAgICA8L0
    1vZGlmeVBhcnRpdGlvbj4KICAgICAgICAgIDwvTW9kaWZ5UGFydGl0aW9ucz4KICAgICAg
    ICA8L0Rpc2s+CiAgICAgICAgPFdpbGxTaG93VUk+T25FcnJvcjwvV2lsbFNob3dVST4KIC
    AgICAgPC9EaXNrQ29uZmlndXJhdGlvbj4KICAgICAgPEltYWdlSW5zdGFsbD4KICAgICAg
    ICA8T1NJbWFnZT4KICAgICAgICAgIDxJbnN0YWxsVG8+CiAgICAgICAgICAgIDxEaXNrSU
    Q+MDwvRGlza0lEPgogICAgICAgICAgICA8UGFydGl0aW9uSUQ+NDwvUGFydGl0aW9uSUQ+
    CiAgICAgICAgICA8L0luc3RhbGxUbz4KICAgICAgICA8L09TSW1hZ2U+CiAgICAgIDwvSW
    1hZ2VJbnN0YWxsPgogICAgICA8VXNlckRhdGE+CiAgICAgICAgPFByb2R1Y3RLZXk+CiAg
    ICAgICAgICA8S2V5PlZLN0pHLU5QSFRNLUM5N0pNLTlNUEdULTNWNjZUPC9LZXk+CiAgIC
    AgICAgICA8V2lsbFNob3dVST5PbkVycm9yPC9XaWxsU2hvd1VJPgogICAgICAgIDwvUHJv
    ZHVjdEtleT4KICAgICAgICA8QWNjZXB0RXVsYT50cnVlPC9BY2NlcHRFdWxhPgogICAgIC
    A8L1VzZXJEYXRhPgogICAgICA8VXNlQ29uZmlndXJhdGlvblNldD5mYWxzZTwvVXNlQ29u
    ZmlndXJhdGlvblNldD4KICAgIDwvY29tcG9uZW50PgogIDwvc2V0dGluZ3M+CiAgPHNldH
    RpbmdzIHBhc3M9ImdlbmVyYWxpemUiPjwvc2V0dGluZ3M+CiAgPHNldHRpbmdzIHBhc3M9
    InNwZWNpYWxpemUiPgogICAgPGNvbXBvbmVudCBuYW1lPSJNaWNyb3NvZnQtV2luZG93cy
    1EZXBsb3ltZW50IiBwcm9jZXNzb3JBcmNoaXRlY3R1cmU9ImFtZDY0IiBwdWJsaWNLZXlU
    b2tlbj0iMzFiZjM4NTZhZDM2NGUzNSIgbGFuZ3VhZ2U9Im5ldXRyYWwiIHZlcnNpb25TY2
    9wZT0ibm9uU3hTIj4KICAgICAgPFJ1blN5bmNocm9ub3VzPgogICAgICAgIDxSdW5TeW5j
    aHJvbm91c0NvbW1hbmQgd2NtOmFjdGlvbj0iYWRkIj4KICAgICAgICAgIDxPcmRlcj4xPC
    9PcmRlcj4KICAgICAgICAgIDxQYXRoPnBvd2Vyc2hlbGwuZXhlIC1Ob1Byb2ZpbGUgLUNv
    bW1hbmQgIiR4bWwgPSBbeG1sXTo6bmV3KCk7ICR4bWwuTG9hZCgnQzpcV2luZG93c1xQYW
    50aGVyXHVuYXR0ZW5kLnhtbCcpOyAkc2IgPSBbc2NyaXB0YmxvY2tdOjpDcmVhdGUoICR4
    bWwudW5hdHRlbmQuRXh0ZW5zaW9ucy5FeHRyYWN0U2NyaXB0ICk7IEludm9rZS1Db21tYW
    5kIC1TY3JpcHRCbG9jayAkc2IgLUFyZ3VtZW50TGlzdCAkeG1sOyI8L1BhdGg+CiAgICAg
    ICAgPC9SdW5TeW5jaHJvbm91c0NvbW1hbmQ+CiAgICAgICAgPFJ1blN5bmNocm9ub3VzQ2
    9tbWFuZCB3Y206YWN0aW9uPSJhZGQiPgogICAgICAgICAgPE9yZGVyPjI8L09yZGVyPgog
    ICAgICAgICAgPFBhdGg+cG93ZXJzaGVsbC5leGUgLU5vUHJvZmlsZSAtQ29tbWFuZCAiR2
    V0LUNvbnRlbnQgLUxpdGVyYWxQYXRoICdDOlxXaW5kb3dzXFNldHVwXFNjcmlwdHNcU3Bl
    Y2lhbGl6ZS5wczEnIC1SYXcgfCBJbnZva2UtRXhwcmVzc2lvbjsiPC9QYXRoPgogICAgIC
    AgIDwvUnVuU3luY2hyb25vdXNDb21tYW5kPgogICAgICAgIDxSdW5TeW5jaHJvbm91c0Nv
    bW1hbmQgd2NtOmFjdGlvbj0iYWRkIj4KICAgICAgICAgIDxPcmRlcj4zPC9PcmRlcj4KIC
    AgICAgICAgIDxQYXRoPnJlZy5leGUgbG9hZCAiSEtVXERlZmF1bHRVc2VyIiAiQzpcVXNl
    cnNcRGVmYXVsdFxOVFVTRVIuREFUIjwvUGF0aD4KICAgICAgICA8L1J1blN5bmNocm9ub3
    VzQ29tbWFuZD4KICAgICAgICA8UnVuU3luY2hyb25vdXNDb21tYW5kIHdjbTphY3Rpb249
    ImFkZCI+CiAgICAgICAgICA8T3JkZXI+NDwvT3JkZXI+CiAgICAgICAgICA8UGF0aD5wb3
    dlcnNoZWxsLmV4ZSAtTm9Qcm9maWxlIC1Db21tYW5kICJHZXQtQ29udGVudCAtTGl0ZXJh
    bFBhdGggJ0M6XFdpbmRvd3NcU2V0dXBcU2NyaXB0c1xEZWZhdWx0VXNlci5wczEnIC1SYX
    cgfCBJbnZva2UtRXhwcmVzc2lvbjsiPC9QYXRoPgogICAgICAgIDwvUnVuU3luY2hyb25v
    dXNDb21tYW5kPgogICAgICAgIDxSdW5TeW5jaHJvbm91c0NvbW1hbmQgd2NtOmFjdGlvbj
    0iYWRkIj4KICAgICAgICAgIDxPcmRlcj41PC9PcmRlcj4KICAgICAgICAgIDxQYXRoPnJl
    Zy5leGUgdW5sb2FkICJIS1VcRGVmYXVsdFVzZXIiPC9QYXRoPgogICAgICAgIDwvUnVuU3
    luY2hyb25vdXNDb21tYW5kPgogICAgICA8L1J1blN5bmNocm9ub3VzPgogICAgPC9jb21w
    b25lbnQ+CiAgPC9zZXR0aW5ncz4KICA8c2V0dGluZ3MgcGFzcz0iYXVkaXRTeXN0ZW0iPj
    wvc2V0dGluZ3M+CiAgPHNldHRpbmdzIHBhc3M9ImF1ZGl0VXNlciI+PC9zZXR0aW5ncz4K
    ICA8c2V0dGluZ3MgcGFzcz0ib29iZVN5c3RlbSI+CiAgICA8Y29tcG9uZW50IG5hbWU9Ik
    1pY3Jvc29mdC1XaW5kb3dzLUludGVybmF0aW9uYWwtQ29yZSIgcHJvY2Vzc29yQXJjaGl0
    ZWN0dXJlPSJhbWQ2NCIgcHVibGljS2V5VG9rZW49IjMxYmYzODU2YWQzNjRlMzUiIGxhbm
    d1YWdlPSJuZXV0cmFsIiB2ZXJzaW9uU2NvcGU9Im5vblN4UyI+CiAgICAgIDxJbnB1dExv
    Y2FsZT4wNDA5OjAwMDAwNDA5OzA0MTk6MDAwMDA0MTk8L0lucHV0TG9jYWxlPgogICAgIC
    A8U3lzdGVtTG9jYWxlPmVuLVVTPC9TeXN0ZW1Mb2NhbGU+CiAgICAgIDxVSUxhbmd1YWdl
    PnJ1LVJVPC9VSUxhbmd1YWdlPgogICAgICA8VXNlckxvY2FsZT5lbi1VUzwvVXNlckxvY2
    FsZT4KICAgIDwvY29tcG9uZW50PgogICAgPGNvbXBvbmVudCBuYW1lPSJNaWNyb3NvZnQt
    V2luZG93cy1TaGVsbC1TZXR1cCIgcHJvY2Vzc29yQXJjaGl0ZWN0dXJlPSJhbWQ2NCIgcH
    VibGljS2V5VG9rZW49IjMxYmYzODU2YWQzNjRlMzUiIGxhbmd1YWdlPSJuZXV0cmFsIiB2
    ZXJzaW9uU2NvcGU9Im5vblN4UyI+CiAgICAgIDxVc2VyQWNjb3VudHM+CiAgICAgICAgPE
    xvY2FsQWNjb3VudHM+CiAgICAgICAgICA8TG9jYWxBY2NvdW50IHdjbTphY3Rpb249ImFk
    ZCI+CiAgICAgICAgICAgIDxOYW1lPmNsb3VkPC9OYW1lPgogICAgICAgICAgICA8RGlzcG
    xheU5hbWU+Y2xvdWQ8L0Rpc3BsYXlOYW1lPgogICAgICAgICAgICA8R3JvdXA+QWRtaW5p
    c3RyYXRvcnM8L0dyb3VwPgogICAgICAgICAgICA8UGFzc3dvcmQ+CiAgICAgICAgICAgIC
    AgPFZhbHVlPmNsb3VkPC9WYWx1ZT4KICAgICAgICAgICAgICA8UGxhaW5UZXh0PnRydWU8
    L1BsYWluVGV4dD4KICAgICAgICAgICAgPC9QYXNzd29yZD4KICAgICAgICAgIDwvTG9jYW
    xBY2NvdW50PgogICAgICAgICAgPExvY2FsQWNjb3VudCB3Y206YWN0aW9uPSJhZGQiPgog
    ICAgICAgICAgICA8TmFtZT5Vc2VyPC9OYW1lPgogICAgICAgICAgICA8RGlzcGxheU5hbW
    U+dXNlcjwvRGlzcGxheU5hbWU+CiAgICAgICAgICAgIDxHcm91cD5Vc2VyczwvR3JvdXA+
    CiAgICAgICAgICAgIDxQYXNzd29yZD4KICAgICAgICAgICAgICA8VmFsdWU+dXNlcjwvVm
    FsdWU+CiAgICAgICAgICAgICAgPFBsYWluVGV4dD50cnVlPC9QbGFpblRleHQ+CiAgICAg
    ICAgICAgIDwvUGFzc3dvcmQ+CiAgICAgICAgICA8L0xvY2FsQWNjb3VudD4KICAgICAgIC
    A8L0xvY2FsQWNjb3VudHM+CiAgICAgIDwvVXNlckFjY291bnRzPgogICAgICA8QXV0b0xv
    Z29uPgogICAgICAgIDxVc2VybmFtZT5jbG91ZDwvVXNlcm5hbWU+CiAgICAgICAgPEVuYW
    JsZWQ+dHJ1ZTwvRW5hYmxlZD4KICAgICAgICA8TG9nb25Db3VudD4xPC9Mb2dvbkNvdW50
    PgogICAgICAgIDxQYXNzd29yZD4KICAgICAgICAgIDxWYWx1ZT5jbG91ZDwvVmFsdWU+Ci
    AgICAgICAgICA8UGxhaW5UZXh0PnRydWU8L1BsYWluVGV4dD4KICAgICAgICA8L1Bhc3N3
    b3JkPgogICAgICA8L0F1dG9Mb2dvbj4KICAgICAgPE9PQkU+CiAgICAgICAgPFByb3RlY3
    RZb3VyUEM+MzwvUHJvdGVjdFlvdXJQQz4KICAgICAgICA8SGlkZUVVTEFQYWdlPnRydWU8
    L0hpZGVFVUxBUGFnZT4KICAgICAgICA8SGlkZVdpcmVsZXNzU2V0dXBJbk9PQkU+dHJ1ZT
    wvSGlkZVdpcmVsZXNzU2V0dXBJbk9PQkU+CiAgICAgICAgPEhpZGVPbmxpbmVBY2NvdW50
    U2NyZWVucz5mYWxzZTwvSGlkZU9ubGluZUFjY291bnRTY3JlZW5zPgogICAgICA8L09PQk
    U+CiAgICAgIDxGaXJzdExvZ29uQ29tbWFuZHM+CiAgICAgICAgPFN5bmNocm9ub3VzQ29t
    bWFuZCB3Y206YWN0aW9uPSJhZGQiPgogICAgICAgICAgPE9yZGVyPjE8L09yZGVyPgogIC
    AgICAgICAgPENvbW1hbmRMaW5lPnBvd2Vyc2hlbGwuZXhlIC1Ob1Byb2ZpbGUgLUNvbW1h
    bmQgIkdldC1Db250ZW50IC1MaXRlcmFsUGF0aCAnQzpcV2luZG93c1xTZXR1cFxTY3JpcH
    RzXEZpcnN0TG9nb24ucHMxJyAtUmF3IHwgSW52b2tlLUV4cHJlc3Npb247IjwvQ29tbWFu
    ZExpbmU+CiAgICAgICAgPC9TeW5jaHJvbm91c0NvbW1hbmQ+CiAgICAgIDwvRmlyc3RMb2
    dvbkNvbW1hbmRzPgogICAgPC9jb21wb25lbnQ+CiAgPC9zZXR0aW5ncz4KPC91bmF0dGVu
    ZD4K
```

Затем можно создать виртуальную машину, которая в процессе установки будет использовать файл ответов.
Чтобы предоставить виртуальной машине Windows файл ответов, необходимо указать provisioning с типом SysprepRef.
Вы также можете указать здесь другие файлы в формате base64 (customize.ps1, id_rsa.pub,...),
необходимые для успешного выполнения скриптов внутри файла ответов.

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
