{{- define "virtualization-dra-usb-gateway.isEnabled" -}}
{{- if eq (include "hasValidModuleConfig" .) "true" -}}
true
{{- end -}}
{{- end -}}
