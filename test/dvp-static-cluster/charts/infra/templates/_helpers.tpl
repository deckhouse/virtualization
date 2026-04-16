{{- define "infra.vm-labels" -}}
{{- $prefix := regexReplaceAll "-\\d+$" . "" -}}
vm: {{ . }}
group: {{ $prefix }}
{{- end }}

{{- define "infra.vmclass-name" -}}
{{ .Values.namespace }}-cpu
{{- end }}

{{- define "infra.vd-root-name" -}}
{{ . }}-root
{{- end }}

{{- define "infra.vm" -}}
{{- $ctx := index . 0 -}}
{{- $name := index . 1 -}}
{{- $cfg := index . 2 -}}
{{- $networkConfig := get $ctx.Values.instances "networkConfig" | default dict -}}
{{- $clusterNetworkName := get $networkConfig "clusterNetworkName" | default "" -}}

---
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualMachine
metadata:
  name: {{ $name }}
  namespace: {{ $ctx.Values.namespace }}
  labels:
    {{- include "infra.vm-labels" $name | nindent 4 }}
spec:
  blockDeviceRefs:
  - kind: VirtualDisk
    name: {{ include "infra.vd-root-name" $name }}
{{- if ne $ctx.Values.storageType "nfs" }}
{{- range $i, $v := $cfg.additionalDisks }}
  - kind: VirtualDisk
    name: {{ printf "%s-%d" $name $i }}
{{- end }}
  networks:
    - type: Main
{{- if $clusterNetworkName }}
    - type: ClusterNetwork
      name: {{ $clusterNetworkName }}
{{- end }}
  bootloader: {{ $ctx.Values.image.bootloader }}
  liveMigrationPolicy: PreferForced
  cpu:
    coreFraction: {{ $cfg.cpu.coreFraction }}
    cores: {{ $cfg.cpu.cores }}
  disruptions:
    restartApprovalMode: Automatic
  enableParavirtualization: true
  memory:
    size: {{ $cfg.memory.size }}
  osType: Generic
  provisioning:
    type: UserData
    userData: |
      #cloud-config
      ssh_pwauth: true
      package_update: true
      write_files:
        - path: /etc/netplan/99-eno2.yaml
          content: |
            network:
              version: 2
              ethernets:
                eno2:
                  dhcp4: false
                  dhcp6: false
                  addresses: []
                  link-local: []
                  optional: true
      packages:
        - qemu-guest-agent
        - jq
        - rsync
        - bind9-dnsutils
      users:
        - default
        - name: cloud
          passwd: {{ $ctx.Values.discovered.userPasswd }}
          shell: /bin/bash
          sudo: ALL=(ALL) NOPASSWD:ALL
          chpasswd: {expire: False}
          lock_passwd: false
          ssh_authorized_keys:
            - {{ $ctx.Values.discovered.publicSSHKey }}

      runcmd:
        - netplan apply
        - ip link set eno2 up
        - systemctl enable --now qemu-guest-agent.service
      final_message: "\U0001F525\U0001F525\U0001F525 The system is finally up, after $UPTIME seconds \U0001F525\U0001F525\U0001F525"
  runPolicy: AlwaysOn
  virtualMachineClassName: {{ include "infra.vmclass-name" $ctx }}
---
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualDisk
metadata:
  name: {{ include "infra.vd-root-name" $name }}
  namespace: {{ $ctx.Values.namespace }}
  labels:
    {{- include "infra.vm-labels" $name | nindent 4 }}
spec:
  dataSource:
    objectRef:
      kind: VirtualImage
      name: base-image
    type: ObjectRef
  persistentVolumeClaim:
    size: {{ $cfg.rootDiskSize | default "50Gi" }}
    {{- if $ctx.Values.storageClass }}
    storageClassName: {{ $ctx.Values.storageClass }}
    {{- end }}

{{ if ne $ctx.Values.storageType "nfs" }}
  {{range $i, $v := $cfg.additionalDisks }}
---
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualDisk
metadata:
  name: {{ printf "%s-%d" $name $i }}
  namespace: {{ $ctx.Values.namespace }}
spec:
  persistentVolumeClaim:
    size: {{ $v.size }}
    {{- if $ctx.Values.storageClass }}
    storageClassName: {{ $ctx.Values.storageClass }}
    {{- end }}
  {{- end }}
{{- end }}
{{- end }}
{{- end }}