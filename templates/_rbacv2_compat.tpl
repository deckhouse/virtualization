{{- /*
  Tells the RBACv2 templates which role model the cluster speaks: the capability/scope model of
  DKP 1.78 and later, or the legacy manage/use one. Remove it once the module no longer supports
  clusters below 1.78.
*/ -}}
{{- define "virtualization.rbacv2_new_scheme" -}}
  {{- $raw := (.Values.global).deckhouseVersion | default "dev" | toString -}}
  {{- $mm := regexFind "^v?[0-9]+[.][0-9]+" $raw -}}
  {{- if $mm -}}
    {{- semverCompare ">= 1.78" (printf "%s.0" $mm) -}}
  {{- else -}}
    {{- /* "dev" or "unknown": a build off any branch says the same, so answer with the model
           whose mistake only loses access. A dev stand below 1.78 flips this to false. */ -}}
    true
  {{- end -}}
{{- end -}}
