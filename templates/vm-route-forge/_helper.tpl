{{- define "vm-route-forge.isEnabled" -}}
{{- if eq (include "hasValidModuleConfig" .) "true" -}}
{{- if (.Values.global.enabledModules | has "cni-cilium") -}}
true
{{- end -}}
{{- end -}}
{{- end -}}
