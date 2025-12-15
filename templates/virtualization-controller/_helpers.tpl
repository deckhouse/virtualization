{{- define "virtualization-controller.isEnabled" -}}
{{- if eq (include "hasValidModuleConfig" .) "true" -}}
true
{{- end -}}
{{- end -}}

{{- define "virtualization-controller.envs" -}}
{{- $registry := include "dvcr.get_registry" (list .) }}
- name: LOG_LEVEL
  value: {{ include "moduleLogLevel" . }}
{{- if eq (include "moduleLogLevel" .) "debug" }}
- name: LOG_DEBUG_VERBOSITY
  value: "10"
{{- end }}
- name: LOG_FORMAT
  value: {{ include "moduleLogFormat" . }}
- name: FORCE_BRIDGE_NETWORK_BINDING
  value: "1"
- name: DISABLE_HYPERV_SYNIC
  value: "1"
- name: POD_NAMESPACE
  valueFrom:
    fieldRef:
      fieldPath: metadata.namespace
- name: IMPORTER_IMAGE
  value: {{ include "helm_lib_module_image" (list . "dvcrImporter") }}
- name: UPLOADER_IMAGE
  value: {{ include "helm_lib_module_image" (list . "dvcrUploader") }}
- name: BOUNDER_IMAGE
  value: {{ include "helm_lib_module_image" (list . "bounder") }}
- name: DVCR_AUTH_SECRET
  value: dvcr-dockercfg-rw
- name: DVCR_CERTS_SECRET
  value: dvcr-tls
- name: DVCR_REGISTRY_URL
  value: {{ $registry | quote }}
- name: DVCR_INSECURE_TLS
  value: "true"
- name: DVCR_IMAGE_MONITOR_SCHEDULE
  value: {{ .Values.virtualization.internal.moduleConfig.dvcr.imageMonitorSchedule | quote }}
- name: DVCR_GC_SCHEDULE
  value: "{{ .Values.virtualization.internal.moduleConfig | dig "dvcr" "gc" "schedule" "" }}"
- name: VIRTUAL_MACHINE_CIDRS
  value: {{ join "," .Values.virtualization.internal.moduleConfig.virtualMachineCIDRs | quote }}
{{- if (hasKey .Values.virtualization.internal.moduleConfig "virtualImages") }}
- name: VIRTUAL_IMAGE_STORAGE_CLASS
  value: {{ .Values.virtualization.internal.moduleConfig.virtualImages.storageClassName }}
- name: VIRTUAL_IMAGE_DEFAULT_STORAGE_CLASS
  value: {{ .Values.virtualization.internal.moduleConfig.virtualImages.defaultStorageClassName }}
{{- if (hasKey .Values.virtualization.internal.moduleConfig.virtualImages "allowedStorageClassSelector") }}
- name: VIRTUAL_IMAGE_ALLOWED_STORAGE_CLASSES
  value: {{ join "," .Values.virtualization.internal.moduleConfig.virtualImages.allowedStorageClassSelector.matchNames | quote }}
{{- end }}
{{- end }}
{{- if (hasKey .Values.virtualization.internal.moduleConfig "virtualDisks") }}
- name: VIRTUAL_DISK_DEFAULT_STORAGE_CLASS
  value: {{ .Values.virtualization.internal.moduleConfig.virtualDisks.defaultStorageClassName }}
{{- if (hasKey .Values.virtualization.internal.moduleConfig.virtualDisks "allowedStorageClassSelector") }}
- name: VIRTUAL_DISK_ALLOWED_STORAGE_CLASSES
  value: {{ join "," .Values.virtualization.internal.moduleConfig.virtualDisks.allowedStorageClassSelector.matchNames | quote }}
{{- end }}
{{- end }}
- name: VIRTUAL_MACHINE_IP_LEASES_RETENTION_DURATION
  value: "10m"
{{- if ne "<missing>" (dig "modules" "publicDomainTemplate" "<missing>" .Values.global) }}
- name: UPLOADER_INGRESS_HOST
  value: {{ include "helm_lib_module_public_domain" (list . "virtualization") }}
{{- end }}
{{- if (include "helm_lib_module_https_ingress_tls_enabled" .) }}
- name: UPLOADER_INGRESS_TLS_SECRET
  value: {{ include "helm_lib_module_https_secret_name" (list . "ingress-tls") }}
{{- end }}
- name: UPLOADER_INGRESS_CLASS
  value: {{ include "helm_lib_module_ingress_class" . | quote }}
- name: PROVISIONING_POD_LIMITS
  value: '{"cpu":"750m","memory":"600M"}'
- name: PROVISIONING_POD_REQUESTS
  value: '{"cpu":"100m","memory":"60M"}'
- name: GC_VMOP_TTL
  value: "24h"
- name: GC_VMOP_SCHEDULE
  value: "0 0 * * *"
- name: GC_VMI_MIGRATION_TTL
  value: "24h"
- name: GC_VMI_MIGRATION_SCHEDULE
  value: "0 0 * * *"
{{- if (hasKey .Values.virtualization.internal.moduleConfig "liveMigration") }}
- name: LIVE_MIGRATION_BANDWIDTH_PER_NODE
  value: {{ .Values.virtualization.internal.moduleConfig.liveMigration.bandwidthPerNode | quote }}
- name: LIVE_MIGRATION_MAX_MIGRATIONS_PER_NODE
  value: {{ .Values.virtualization.internal.moduleConfig.liveMigration.maxMigrationsPerNode | quote }}
- name: LIVE_MIGRATION_NETWORK
  value: {{ .Values.virtualization.internal.moduleConfig.liveMigration.network | quote }}
{{- if (hasKey .Values.virtualization.internal.moduleConfig.liveMigration "dedicated") }}
- name: LIVE_MIGRATION_DEDICATED_INTERFACE_NAME
  value: {{ .Values.virtualization.internal.moduleConfig.liveMigration.dedicated.interfaceName | quote }}
{{- end }}
{{- end }}
- name: METRICS_BIND_ADDRESS
  value: "127.0.0.1:8080"
{{- if eq (include "moduleLogLevel" .) "debug" }}
- name: PPROF_BIND_ADDRESS
  value: ":8081"
{{- end }}
- name: FIRMWARE_IMAGE
  value: {{ include "helm_lib_module_image" (list . "virtLauncher") }}
- name: CLUSTER_UUID
  value: {{ .Values.global.discovery.clusterUUID }}
- name: CLUSTER_POD_SUBNET_CIDR
  value: {{ .Values.global.clusterConfiguration.podSubnetCIDR }}
- name: CLUSTER_SERVICE_SUBNET_CIDR
  value: {{ .Values.global.clusterConfiguration.serviceSubnetCIDR }}
{{- end }}
