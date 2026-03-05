{{- define "virtualization-dra.isEnabled" -}}
{{- if eq (include "hasValidModuleConfig" .) "true" -}}
{{- if semverCompare ">=1.34" .Values.global.discovery.kubernetesVersion -}}
{{- if eq "true" .Values.virtualization.internal.hasDraFeatureGates -}}
true
{{- end -}}
{{- end -}}
{{- end -}}
{{- end -}}
