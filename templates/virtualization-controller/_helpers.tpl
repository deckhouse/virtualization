{{- define "virtualization-controller.envs" -}}
{{- $registry := include "dvcr.get_registry" (list .) }}
- name: KUBECONFIG
  value: "/kubeconfig.local/proxy.kubeconfig"
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
- name: VIRTUAL_MACHINE_IP_LEASES_RETENTION_DURATION
  value: "10m"
- name: UPLOADER_INGRESS_HOST
  value: {{ include "helm_lib_module_public_domain" (list . "virtualization") }}
- name: UPLOADER_INGRESS_TLS_SECRET
  value: {{ include "helm_lib_module_https_secret_name" (list . "ingress-tls") }}
- name: UPLOADER_INGRESS_CLASS
  value: {{ include "helm_lib_module_ingress_class" . | quote }}
{{- with .Values.virtualization.importerResourceRequirements }}
{{- if hasKey . "limits" }}
- name: IMPORTER_LIMITS
  value: {{ .limits | toJson | quote }}
{{- end }}
{{- if hasKey . "requests" }}
- name: IMPORTER_REQUESTS
  value: {{ .requests | toJson | quote }}
{{- end }}
{{- end }}
{{- if eq .Values.virtualization.logLevel "debug" }}
- name: PPROF_BIND_ADDRESS
  value: ":8081"
{{- end }}
{{- end }}
