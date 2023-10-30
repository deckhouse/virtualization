{{- define "virtualization-controller.envs" -}}
{{- $registry := include "dvcr.get_registry" (list .) }}
- name: VERBOSITY
  value: "3"
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
- name: IMPORTER_DESTINATION_AUTH_SECRET
  value: dvcr-dockercfg-rw
- name: IMPORTER_DESTINATION_REGISTRY
  value: {{ $registry | quote }}
- name: IMPORTER_DESTINATION_INSECURE_TLS
  value: "true"
- name: UPLOADER_IMAGE
  value: {{ include "helm_lib_module_image" (list . "dvcrUploader") }}
{{- end }}
