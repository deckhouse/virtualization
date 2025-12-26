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
---
kind: Service
apiVersion: v1
metadata:
  name: {{ $name }}
  namespace: {{ $ctx.Values.namespace }}
  labels:
    {{- include "infra.vm-labels" $name | nindent 4 }}
spec:
  clusterIP: None
  selector:
    {{- include "infra.vm-labels" $name | nindent 4 }}
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
{{- range $i, $v := $cfg.additionalDisks }}
  - kind: VirtualDisk
    name: {{ printf "%s-%d" $name $i }}
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
      packages:
        - qemu-guest-agent
        - jq
        - rsync
        - bind9-dnsutils
      users:
        - default
        - name: cloud
          passwd: $6$rounds=4096$vln/.aPHBOI7BMYR$bBMkqQvuGs5Gyd/1H5DP4m9HjQSy.kgrxpaGEHwkX7KEFV8BS.HZWPitAtZ2Vd8ZqIZRqmlykRCagTgPejt1i.
          shell: /bin/bash
          sudo: ALL=(ALL) NOPASSWD:ALL
          chpasswd: {expire: False}
          lock_passwd: false
          ssh_authorized_keys:
            - {{ $ctx.Values.discovered.publicSSHKey }}

      runcmd:
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
