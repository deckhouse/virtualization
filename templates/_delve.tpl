{{/*
File created during build in werf.yaml
{{-  if eq .DEBUG_COMPONENT }}
    - |
      cat <<EOF>> /delve.yaml
      debug:
        component: {{ printf "%s" (split "/" .DEBUG_COMPONENT)._1 }}
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
{{-   if and ($delve) (eq $delve.debug.component $image) -}}
- containerPort: 2345
  name: tcp-dlv-2345
  protocol: TCP
{{-   end -}}
{{- end }}