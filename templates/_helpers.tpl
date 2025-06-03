{{- define "priorityClassName" -}}
system-cluster-critical
{{- end }}

{{- define "spec_template_spec_antiaffinity_patch" -}}
  {{- $key := index . 0 -}}
  {{- $labelValue := index . 1 -}}
  '{{ include "tmplAntiAffinity" (list $key $labelValue) | fromYaml | toJson }}'
{{- end }}

{{- define "tmplAntiAffinity" -}}
  {{- $key := index . 0 -}}
  {{- $labelValue := index . 1 -}}
spec:
  template:
    spec:
      affinity:
        podAntiAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
          - labelSelector:
              matchExpressions:
              - key: {{ $key }}
                operator: In
                values:
                - {{ $labelValue }}
            topologyKey: kubernetes.io/hostname
{{- end -}}

{{- define "spec_strategy_rolling_update_patch" -}}
  '{{ include "tmplSpecStrategyRollingUpdate" . | fromYaml | toJson }}'
{{- end }}

{{- define "tmplSpecStrategyRollingUpdate" -}}
spec:
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxSurge: 0
      maxUnavailable: 1
{{- end -}}
