{{- define "strategic_affinity_patch" -}}
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
