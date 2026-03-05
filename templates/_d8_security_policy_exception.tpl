{{- define "d8.securityPolicyException.isEnabled" -}}
{{- $crdAPIVer := "deckhouse.io/v1alpha1/SecurityPolicyException" -}}
{{- if or (.Values.global.discovery.apiVersions | has $crdAPIVer) (.Capabilities.APIVersions.Has $crdAPIVer) -}}
true
{{- end -}}
{{- end -}}
