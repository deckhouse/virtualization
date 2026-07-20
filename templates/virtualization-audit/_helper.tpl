{{- /* Helper to detect if the cluster and the module are configured to enable audit.
- audit.enabled should be true in ModuleConfig
- log-shipper and runtime-audit-engine modules should be enabled.
- the audit TLS certificate must already be generated (the hook runs asynchronously),
  otherwise dependent resources (ClusterLogDestination, cert secret, Deployment) would
  render with empty CA/cert/key.

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
{{- if and (dig "audit" "cert" "ca" "" .Values.virtualization.internal) (dig "audit" "cert" "crt" "" .Values.virtualization.internal) (dig "audit" "cert" "key" "" .Values.virtualization.internal) -}}
true
{{- end }}
{{- end }}
{{- end }}
{{- end }}
{{- end }}
