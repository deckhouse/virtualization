{{- /*
Not shipped once the check-sdn-route-forger hook records that the sdn agents route
virtual machines; the record is never reverted.
*/ -}}
{{- define "vm-route-forge.isEnabled" -}}
{{- if eq (include "hasValidModuleConfig" .) "true" -}}
{{- if (.Values.global.enabledModules | has "cni-cilium") -}}
{{- if not (dig "moduleState" "sdnRouteForgerDetection" false .Values.virtualization.internal) -}}
true
{{- end -}}
{{- end -}}
{{- end -}}
{{- end -}}
