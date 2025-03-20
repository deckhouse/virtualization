{{- define "virtualization-controller.envs" -}}
{{- $registry := include "dvcr.get_registry" (list .) }}
- name: LOG_LEVEL
  value: {{ .Values.virtualization.logLevel }}
{{- if eq .Values.virtualization.logLevel "debug" }}
- name: LOG_DEBUG_VERBOSITY
  value: "10"
{{- end }}
- name: LOG_FORMAT
  value: {{ .Values.virtualization.logFormat }}
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
- name: VIRTUAL_MACHINE_CIDRS
  value: {{ join "," .Values.virtualization.virtualMachineCIDRs | quote }}
{{- if (hasKey .Values.virtualization "virtualImages") }}
- name: VIRTUAL_IMAGE_STORAGE_CLASS
  value: {{ .Values.virtualization.virtualImages.storageClassName }}
- name: VIRTUAL_IMAGE_DEFAULT_STORAGE_CLASS
  value: {{ .Values.virtualization.virtualImages.defaultStorageClassName }}
{{- if (hasKey .Values.virtualization.virtualImages "allowedStorageClassSelector") }}
- name: VIRTUAL_IMAGE_ALLOWED_STORAGE_CLASSES
  value: {{ join "," .Values.virtualization.virtualImages.allowedStorageClassSelector.matchNames | quote }}
{{- end }}
{{- end }}
{{- if (hasKey .Values.virtualization "virtualDisks") }}
- name: VIRTUAL_DISK_DEFAULT_STORAGE_CLASS
  value: {{ .Values.virtualization.virtualDisks.defaultStorageClassName }}
{{- if (hasKey .Values.virtualization.virtualDisks "allowedStorageClassSelector") }}
- name: VIRTUAL_DISK_ALLOWED_STORAGE_CLASSES
  value: {{ join "," .Values.virtualization.virtualDisks.allowedStorageClassSelector.matchNames | quote }}
{{- end }}
{{- end }}
- name: VIRTUAL_MACHINE_IP_LEASES_RETENTION_DURATION
  value: "10m"
- name: UPLOADER_INGRESS_HOST
  value: {{ include "helm_lib_module_public_domain" (list . "virtualization") }}
- name: UPLOADER_INGRESS_TLS_SECRET
  value: {{ include "helm_lib_module_https_secret_name" (list . "ingress-tls") }}
- name: UPLOADER_INGRESS_CLASS
  value: {{ include "helm_lib_module_ingress_class" . | quote }}
- name: PROVISIONING_POD_LIMITS
  value: '{"cpu":"750m","memory":"600M"}'
- name: PROVISIONING_POD_REQUESTS
  value: '{"cpu":"100m","memory":"60M"}'
- name: GC_VMOP_TTL
  value: "24h"
- name: GC_VMOP_SCHEDULE
  value: "0 * * * *"
- name: GC_VMI_MIGRATION_TTL
  value: "24h"
- name: GC_VMI_MIGRATION_SCHEDULE
  value: "0 * * * *"
- name: METRICS_BIND_ADDRESS
  value: "127.0.0.1:8080"
{{- if eq .Values.virtualization.logLevel "debug" }}
- name: PPROF_BIND_ADDRESS
  value: ":8081"
{{- end }}
- name: FIRMWARE_IMAGE
  value: {{ include "helm_lib_module_image" (list . "virtLauncher") }}
{{- end }}
