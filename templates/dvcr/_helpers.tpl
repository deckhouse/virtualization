{{- define "dvcr.isEnabled" -}}
{{- if eq (include "hasValidModuleConfig" . ) "true" -}}
true
{{- end }}
{{- end }}

{{- define "dvcr.isGarbageCollection" -}}
{{- .Values.virtualization.internal | dig "dvcr" "garbageCollectionModeEnabled" "false" | default "false" -}}
{{- end }}

{{- define "dvcr.envs" -}}
- name: REGISTRY_HTTP_TLS_CERTIFICATE
  value: /etc/ssl/docker/tls.crt
- name: REGISTRY_HTTP_TLS_KEY
  value: /etc/ssl/docker/tls.key

- name: REGISTRY_AUTH
  value: "htpasswd"
- name: REGISTRY_AUTH_HTPASSWD_REALM
  value: "Registry Realm"
- name: REGISTRY_AUTH_HTPASSWD_PATH
  value: "/auth/htpasswd"

- name: REGISTRY_HTTP_SECRET
  valueFrom:
    secretKeyRef:
      name: dvcr-secrets
      key: salt

{{- if eq (.Values.virtualization.internal.moduleConfig | dig "dvcr" "storage" "type" "") "PersistentVolumeClaim" }}
- name: REGISTRY_STORAGE_FILESYSTEM_ROOTDIRECTORY
  value: "/var/lib/registry"
{{- else if eq (.Values.virtualization.internal.moduleConfig | dig "dvcr" "storage" "type" "") "ObjectStorage" }}
  {{- if eq .Values.virtualization.internal.moduleConfig.dvcr.storage.objectStorage.type "S3" }}
- name: REGISTRY_STORAGE_S3_REGION
  value: "{{ .Values.virtualization.internal.moduleConfig.dvcr.storage.objectStorage.s3.region }}"
- name: REGISTRY_STORAGE_S3_BUCKET
  value: "{{ .Values.virtualization.internal.moduleConfig.dvcr.storage.objectStorage.s3.bucket }}"
- name: REGISTRY_STORAGE_S3_ACCESSKEY
  valueFrom:
    secretKeyRef:
      name: dvcr-object-storage-credentials
      key: s3AccessKey
- name: REGISTRY_STORAGE_S3_SECRETKEY
  valueFrom:
    secretKeyRef:
      name: dvcr-object-storage-credentials
      key: s3SecretKey
- name: REGISTRY_STORAGE_S3_REGIONENDPOINT
  value: "{{ .Values.virtualization.internal.moduleConfig.dvcr.storage.objectStorage.s3.regionEndpoint }}"
  {{- end }}
{{- end }}
{{- end }}

{{- define "dvcr.envs.garbageCollection" -}}
{{- if eq (.Values.virtualization.internal.moduleConfig | dig "dvcr" "storage" "type" "") "PersistentVolumeClaim" }}
- name: REGISTRY_STORAGE_FILESYSTEM_ROOTDIRECTORY
  value: "/var/lib/registry"
{{- end }}
{{- end }}


{{- define "dvcr.volumeMounts" -}}
- name: "dvcr-config"
  mountPath: "/etc/docker/registry"

{{- if eq (.Values.virtualization.internal.moduleConfig | dig "dvcr" "storage" "type" "") "PersistentVolumeClaim" }}
- name: data
  mountPath: /var/lib/registry/
{{- end }}

- mountPath: /etc/ssl/docker
  name: dvcr-tls
  readOnly: true

- name: auth
  mountPath: /auth
  readOnly: true

{{- end -}}

{{- define "dvcr.volumeMounts.garbageCollection" -}}
- name: "dvcr-config"
  mountPath: "/etc/docker/registry"
{{- if eq (.Values.virtualization.internal.moduleConfig | dig "dvcr" "storage" "type" "") "PersistentVolumeClaim" }}
- name: data
  mountPath: /var/lib/registry/
{{- end }}
{{- end -}}


{{- define "dvcr.volumes" -}}
- name: dvcr-config
  configMap:
    name: dvcr-config

{{- if eq (.Values.virtualization.internal.moduleConfig | dig "dvcr" "storage" "type" "") "PersistentVolumeClaim" }}
- name: data
  persistentVolumeClaim:
    claimName: dvcr
{{- end }}

- name: dvcr-tls
  secret:
    secretName: dvcr-tls

- name: auth
  secret:
    secretName: dvcr-secrets
    items:
    - key: htpasswd
      path: htpasswd
{{- end -}}


{{- define "dvcr.helm_lib_deployment_strategy_and_replicas_for_ha" -}}
{{- if eq (include "dvcr.isGarbageCollection" . ) "true" }}
replicas: 1
strategy:
  type: Recreate
{{- else if and (include "helm_lib_ha_enabled" .) (eq (.Values.virtualization.internal.moduleConfig | dig "dvcr" "storage" "type" "") "ObjectStorage") }}
replicas: 2
strategy:
  type: RollingUpdate
  rollingUpdate:
    maxSurge: 0
    maxUnavailable: 1
{{- else if eq (.Values.virtualization.internal.moduleConfig | dig "dvcr" "storage" "type" "") "ObjectStorage" }}
replicas: 1
strategy:
  type: RollingUpdate
{{- else if eq (.Values.virtualization.internal.moduleConfig | dig "dvcr" "storage" "type" "") "PersistentVolumeClaim" }}
replicas: 1
strategy:
  type: Recreate
{{- end }}
{{- end -}}

{{- define "dvcr.helm_lib_is_ha_to_value" -}}
  {{- $context := index . 0 -}}
  {{- $yes := index . 1 -}}
  {{- $no  := index . 2 -}}
  {{- if and (include "helm_lib_ha_enabled" $context) (eq ($context.Values.virtualization.internal.moduleConfig | dig "dvcr" "storage" "type" "") "ObjectStorage") }}
    {{- $yes -}}
  {{- else }}
    {{- $no -}}
  {{- end }}
{{- end -}}

{{- define "dvcr.generate_dockercfg" -}}
  {{- $registry := index . 1 -}}
  {{- $user := index . 2 -}}
  {{- $password := index . 3 | b64dec -}}
  .dockerconfigjson:  {{ printf "{\"auths\": {\"%s\": {\"auth\": \"%s\"}}}" $registry (printf "%s:%s" $user $password | b64enc) | b64enc }}
{{- end -}}


{{- define "dvcr.get_registry" -}}
  {{- $context := index . 0 -}}
{{- printf "dvcr.d8-%s.svc" $context.Chart.Name }}
{{- end -}}


