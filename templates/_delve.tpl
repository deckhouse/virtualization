{{/*
File created during build in werf.yaml
{{-  if eq .DEBUG_COMPONENT }}
    - |
      cat <<EOF>> /delve.yaml
      debug:
        component: "{{ .DEBUG_COMPONENT }}""
      EOF
{{-  end }}
*/}}
{{ define "delve" }}
{{-   $filePath := "delve.yaml" }}
{{-   if .Files.Get $filePath }}
{{-     .Files.Get $filePath }}
{{-   end }}
{{- end }}

{{- define "delvePorts" -}}
{{- $delve := index . 0 -}}
{{- $image := index . 1 -}}
{{- if eq $image ($delve | dig "debug" "component" "<missing>") -}}
- containerPort: 2345
  name: tcp-dlv-2345
  protocol: TCP
{{-   end -}}
{{- end }}