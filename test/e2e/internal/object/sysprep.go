/*
Copyright 2026 Flant JSC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package object

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// windows2022UnattendXML sets up a cloud/Administrator account (password
// Cloud1234, matching the DefaultUser convention used elsewhere in this
// package) and starts sshd. It authorizes whichever key is placed in the
// "authorized_keys" file alongside it on the rendered sysprep CD, copying it
// from whichever of D:-H: exposes it: with the virtio driver and
// iperf3/jq-tools ISOs also attached, the sysprep CD's own drive letter is
// not fixed.
const windows2022UnattendXML = `<?xml version="1.0" encoding="utf-8"?>
<unattend xmlns="urn:schemas-microsoft-com:unattend">

  <settings pass="specialize">
    <component name="Microsoft-Windows-Shell-Setup" processorArchitecture="amd64" publicKeyToken="31bf3856ad364e35" language="neutral" versionScope="nonSxS">
      <ComputerName>*</ComputerName>
      <TimeZone>Russian Standard Time</TimeZone>
    </component>
  </settings>

  <settings pass="oobeSystem">
    <component name="Microsoft-Windows-International-Core" processorArchitecture="amd64" publicKeyToken="31bf3856ad364e35" language="neutral" versionScope="nonSxS">
      <InputLocale>0409:00000409</InputLocale>
      <SystemLocale>en-US</SystemLocale>
      <UILanguage>en-US</UILanguage>
      <UserLocale>en-US</UserLocale>
    </component>

    <component name="Microsoft-Windows-Shell-Setup" processorArchitecture="amd64" publicKeyToken="31bf3856ad364e35" language="neutral" versionScope="nonSxS">
      <TimeZone>Russian Standard Time</TimeZone>

      <OOBE>
        <HideEULAPage>true</HideEULAPage>
      </OOBE>

      <UserAccounts>
        <AdministratorPassword>
          <Value>Cloud1234</Value>
          <PlainText>true</PlainText>
        </AdministratorPassword>
        <LocalAccounts>
          <LocalAccount wcm:action="add" xmlns:wcm="http://schemas.microsoft.com/WMIConfig/2002/State">
            <Name>cloud</Name>
            <DisplayName>cloud</DisplayName>
            <Group>Administrators</Group>
            <Password><Value>Cloud1234</Value><PlainText>true</PlainText></Password>
          </LocalAccount>
        </LocalAccounts>
      </UserAccounts>

      <AutoLogon>
        <Enabled>true</Enabled>
        <LogonCount>1</LogonCount>
        <Username>Administrator</Username>
        <Password><Value>Cloud1234</Value><PlainText>true</PlainText></Password>
      </AutoLogon>

      <FirstLogonCommands>
        <SynchronousCommand wcm:action="add" xmlns:wcm="http://schemas.microsoft.com/WMIConfig/2002/State">
          <Order>1</Order>
          <Description>SSH public key from the provisioning CD</Description>
          <CommandLine>cmd /c md C:\ProgramData\ssh 2&gt;nul &amp; for %i in (D E F G H) do if exist %i:\authorized_keys copy /y %i:\authorized_keys C:\ProgramData\ssh\administrators_authorized_keys</CommandLine>
        </SynchronousCommand>
        <SynchronousCommand wcm:action="add" xmlns:wcm="http://schemas.microsoft.com/WMIConfig/2002/State">
          <Order>2</Order>
          <Description>ACL for the key file</Description>
          <CommandLine>cmd /c icacls C:\ProgramData\ssh\administrators_authorized_keys /inheritance:r /grant "*S-1-5-18:F" /grant "*S-1-5-32-544:F"</CommandLine>
        </SynchronousCommand>
        <SynchronousCommand wcm:action="add" xmlns:wcm="http://schemas.microsoft.com/WMIConfig/2002/State">
          <Order>3</Order>
          <Description>SSH key for cloud user (sshd Match Group does not resolve on a localized OS)</Description>
          <CommandLine>cmd /c md C:\Users\cloud\.ssh 2&gt;nul &amp; for %i in (D E F G H) do if exist %i:\authorized_keys copy /y %i:\authorized_keys C:\Users\cloud\.ssh\authorized_keys</CommandLine>
        </SynchronousCommand>
        <SynchronousCommand wcm:action="add" xmlns:wcm="http://schemas.microsoft.com/WMIConfig/2002/State">
          <Order>4</Order>
          <Description>ACL for the cloud user's key file</Description>
          <CommandLine>cmd /c icacls C:\Users\cloud\.ssh\authorized_keys /inheritance:r /grant "*S-1-5-18:F" /grant "cloud:F"</CommandLine>
        </SynchronousCommand>
        <SynchronousCommand wcm:action="add" xmlns:wcm="http://schemas.microsoft.com/WMIConfig/2002/State">
          <Order>5</Order>
          <Description>start sshd</Description>
          <CommandLine>powershell -NoProfile -Command "Start-Service sshd; Set-Service sshd -StartupType Automatic"</CommandLine>
        </SynchronousCommand>
        <SynchronousCommand wcm:action="add" xmlns:wcm="http://schemas.microsoft.com/WMIConfig/2002/State">
          <Order>6</Order>
          <Description>extend the system volume</Description>
          <CommandLine>powershell -NoProfile -Command "$max = (Get-PartitionSupportedSize -DriveLetter C).SizeMax; if ($max -gt (Get-Partition -DriveLetter C).Size) { Resize-Partition -DriveLetter C -Size $max }"</CommandLine>
        </SynchronousCommand>
      </FirstLogonCommands>
    </component>
  </settings>
</unattend>
`

// NewWindows2022SysprepSecret builds a SysprepRef Secret for a Windows Server
// 2022 guest: the same unattend.xml validated on the manual
// update-virtualization-version cluster, paired with this package's own
// DefaultUser SSH keypair (the same one framework.SSHCommand authenticates
// with by default) so the resulting VM is reachable exactly like the
// Linux scenarios are.
func NewWindows2022SysprepSecret(name, namespace string) *corev1.Secret {
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Type: v1alpha2.SecretTypeSysprep,
		Data: map[string][]byte{
			"authorized_keys": []byte(defaultSSHPublicKey + "\n"),
			"unattend.xml":    []byte(windows2022UnattendXML),
		},
	}
}
