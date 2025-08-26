{{- define "virtualization-api.isEnabled" -}}
{{- if eq (include "hasValidModuleConfig" .) "true" -}}
true
{{- end -}}
{{- end -}}
