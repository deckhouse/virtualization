{{- define "dvcr.isEnabled" -}}
{{- if eq (include "hasValidModuleConfig" . ) "true" -}}
true
{{- end }}
{{- end }}

{{- /* Safe accessor for dvcr storage type to avoid nil pointer during linting */ -}}
{{- define "dvcr.storageType" -}}
{{- $mc := .Values.virtualization.internal.moduleConfig | default dict -}}
{{- dig "dvcr" "storage" "type" "" $mc -}}
{{- end -}}

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

{{- if eq (include "dvcr.storageType" .) "PersistentVolumeClaim" }}
- name: REGISTRY_STORAGE_FILESYSTEM_ROOTDIRECTORY
  value: "/var/lib/registry"
{{- else if eq (include "dvcr.storageType" .) "ObjectStorage" }}
  {{- if eq (dig "dvcr" "storage" "objectStorage" "type" "" .Values.virtualization.internal.moduleConfig) "S3" }}
- name: REGISTRY_STORAGE_S3_REGION
  value: "{{ dig "dvcr" "storage" "objectStorage" "s3" "region" "" .Values.virtualization.internal.moduleConfig }}"
- name: REGISTRY_STORAGE_S3_BUCKET
  value: "{{ dig "dvcr" "storage" "objectStorage" "s3" "bucket" "" .Values.virtualization.internal.moduleConfig }}"
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
  value: "{{ dig "dvcr" "storage" "objectStorage" "s3" "regionEndpoint" "" .Values.virtualization.internal.moduleConfig }}"
  {{- end }}
{{- end }}
{{- end }}


{{- define "dvcr.volumeMounts" -}}
- name: "dvcr-config"
  mountPath: "/etc/docker/registry"

{{- if eq (include "dvcr.storageType" .) "PersistentVolumeClaim" }}
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


{{- define "dvcr.volumes" -}}
- name: dvcr-config
  configMap:
    name: dvcr-config

{{- if eq (include "dvcr.storageType" .) "PersistentVolumeClaim" }}
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
{{- if and (include "helm_lib_ha_enabled" .) (eq (include "dvcr.storageType" .) "ObjectStorage") }}
replicas: 2
strategy:
  type: RollingUpdate
  rollingUpdate:
    maxSurge: 0
    maxUnavailable: 1
{{- else if eq (include "dvcr.storageType" .) "ObjectStorage" }}
replicas: 1
strategy:
  type: RollingUpdate
{{- else if eq (include "dvcr.storageType" .) "PersistentVolumeClaim" }}
replicas: 1
strategy:
  type: Recreate
{{- end }}
{{- end -}}

{{- define "dvcr.helm_lib_is_ha_to_value" -}}
  {{- $context := index . 0 -}}
  {{- $yes := index . 1 -}}
  {{- $no  := index . 2 -}}
  {{- if and (include "helm_lib_ha_enabled" $context) (eq (include "dvcr.storageType" $context) "ObjectStorage") }}
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
