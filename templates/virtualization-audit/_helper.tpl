{{- /* Helper to detect if the cluster and the module are configured to enable audit.
- audit.enabled should be true in ModuleConfig
- log-shipper and runtime-audit-engine modules should be enabled.

Usage:

    {{- if eq (include "audit.isEnabled" .) "true" }}
    kind: Deployment
    ...
    {{ -end }}

*/}}
{{- define "audit.isEnabled" -}}
{{- if (dig "moduleConfig" "audit" "enabled" false .Values.virtualization.internal) -}}
{{- if (.Values.global.enabledModules | has "log-shipper") -}}
{{- if (.Values.global.enabledModules | has "runtime-audit-engine") -}}
true
{{- end }}
{{- end }}
{{- end }}
{{- end }}
