{{- define "virtualization-dra.isEnabled" -}}
{{- if eq (include "hasValidModuleConfig" .) "true" -}}
{{- if semverCompare ">=1.34" .Values.global.discovery.kubernetesVersion -}}
true
{{- end -}}
{{- end -}}
{{- end -}}
