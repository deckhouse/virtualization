{{- define "virtualization-dra.isEnabled" -}}
{{- if eq (include "hasValidModuleConfig" .) "true" -}}
{{- if semverCompare ">=1.34" .Values.global.discovery.kubernetesVersion -}}
false
{{- end -}}
{{- end -}}
{{- end -}}
