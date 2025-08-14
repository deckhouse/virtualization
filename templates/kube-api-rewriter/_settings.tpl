{{- define "kube_api_rewriter.sidecar_name" -}}proxy{{- end -}}

{{- define "kube_api_rewriter.webhook_port" -}}24192{{- end -}}

{{- /* Port name length must be no more than 15 characters. */ -}}
{{- define "kube_api_rewriter.webhook_port_name" -}}webhook-proxy{{- end -}}

{{- define "kube_api_rewriter.pprof_port" -}}8129{{- end -}}

{{- define "kube_api_rewriter.env" -}}
- name: LOG_LEVEL
  value: {{ include "moduleLogLevel" . }}
{{- if eq (include "moduleLogLevel" .) "debug" }}
- name: PPROF_BIND_ADDRESS
  value: ":{{ include "kube_api_rewriter.pprof_port" . }}"
{{- end }}
{{- end -}}

{{- define "kube_api_rewriter.resources" -}}
cpu: 100m
memory: 30Mi
{{- end -}}

{{- define "kube_api_rewriter.vpa_container_policy" -}}
- containerName: proxy
  minAllowed:
    cpu: 10m
    memory: 30Mi
  maxAllowed:
    cpu: 20m
    memory: 60Mi
{{- end -}}
