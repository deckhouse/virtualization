{{- define "virtualization-dra.isEnabled" -}}
{{- if eq (include "hasValidModuleConfig" .) "true" -}}
false
{{- end -}}
{{- end -}}
