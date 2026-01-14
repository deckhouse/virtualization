{{- define "cluster-config.full-svc-address" -}}
{{- $ctx := index . 0 -}}
{{- $name := index . 1 -}}
{{ $name }}.{{ $ctx.Values.namespace }}.svc.{{ $ctx.Values.discovered.clusterDomain }}
{{- end }}
