{{- /* Helpers to create patches for component customizer in Kubevirt and CDI configurations.

- kube_api_rewriter.pod_spec_strategic_patch_json - creates a JSON patch for a pod spec to add kube-api-rewriter sidecar container.
- kube_api_rewriter.service_spec_port_patch_json - creates a JSON patch for a service spec to point it to the kube-api-rewriter webhook proxy.
- kube_api_rewriter.webhook_spec_port_patch_json - creates a JSON patch for a validating or mutating webhook spec to point it to the kube-api-rewriter webhook proxy.

*/ -}}

{{- define "kube_api_rewriter.pod_spec_strategic_patch_json" -}}
  '{{ include "kube_api_rewriter.pod_spec_strategic_patch" . | fromYaml | toJson }}'
{{- end }}

{{- define "kube_api_rewriter.pod_spec_strategic_patch" -}}
  {{- $ctx := index . 0 -}}
  {{- $mainContainerName := index . 1 -}}
  {{- $settings := dict -}}
  {{- if ge (len .) 3 -}}
  {{-   $settings = index . 2 -}}
  {{- end -}}
  {{- $isWebhook := hasKey $settings "WEBHOOK_ADDRESS" -}}
spec:
  template:
    metadata:
      annotations:
        kubectl.kubernetes.io/default-container: {{ $mainContainerName }}
    spec:
      volumes:
      {{- include "kube_api_rewriter.kubeconfig_volume" . | nindent 6 }}
      containers:
      {{- include "kube_api_rewriter.sidecar_container" (tuple $ctx $settings) | nindent 6 }}
      - name: {{ $mainContainerName }}
        env:
        {{- include "kube_api_rewriter.kubeconfig_env" . | nindent 8 }}
        volumeMounts:
        {{- include "kube_api_rewriter.kubeconfig_volume_mount" . | nindent 8 }}
{{- end -}}


{{- define "kube_api_rewriter.service_spec_port_patch_json" -}}
  '{{ include "kube_api_rewriter.service_spec_port_patch" . | fromYaml | toJson }}'
{{- end }}

{{- define "kube_api_rewriter.service_spec_port_patch" -}}
spec:
  ports:
  - name: {{ include "kube_api_rewriter.webhook_port_name" . }}
    port: {{ include "kube_api_rewriter.webhook_port" . }}
    protocol: TCP
    targetPort: {{ include "kube_api_rewriter.webhook_port_name" . }}
{{- end }}


{{- define "kube_api_rewriter.webhook_spec_port_patch_json" -}}
  '{{ include "kube_api_rewriter.webhook_spec_port_patch" . | fromYaml | toJson }}'
{{- end }}

{{- define "kube_api_rewriter.webhook_spec_port_patch" -}}
{{- $webhookNames := list . -}}
{{- if (kindIs "slice" .) -}}
{{-   $webhookNames = . -}}
{{- end -}}
webhooks:
{{- range $webhookNames }}
- name: {{ . }}
  clientConfig:
    service:
      port: {{ include "kube_api_rewriter.webhook_port" . }}
{{- end -}}
{{- end -}}
