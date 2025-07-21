{{- define "priorityClassName" -}}
system-cluster-critical
{{- end }}

{{- define "spec_template_spec_antiaffinity_patch" -}}
  {{- $key := index . 0 -}}
  {{- $labelValue := index . 1 -}}
  '{{ include "tmplAntiAffinity" (list $key $labelValue) | fromYaml | toJson }}'
{{- end }}

{{- define "tmplAntiAffinity" -}}
  {{- $key := index . 0 -}}
  {{- $labelValue := index . 1 -}}
spec:
  template:
    spec:
      affinity:
        podAntiAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
          - labelSelector:
              matchExpressions:
              - key: {{ $key }}
                operator: In
                values:
                - {{ $labelValue }}
            topologyKey: kubernetes.io/hostname
{{- end -}}

{{- define "spec_strategy_rolling_update_patch" -}}
  '{{ include "tmplSpecStrategyRollingUpdate" . | fromYaml | toJson }}'
{{- end }}

{{- define "tmplSpecStrategyRollingUpdate" -}}
spec:
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxSurge: 0
      maxUnavailable: 1
{{- end -}}

{{- /* Returns node selector for workloads depend on strategy. (Returns only system or control-plane node selector) */ -}}
{{- define "virt_helper_node_selector" }}
{{-   $context := index . 0 }} {{- /* Template context with .Values, .Chart, etc */ -}}
{{-   $strategy := index . 1 | include "helm_lib_internal_check_node_selector_strategy" }} {{- /* check strategy, one of "frontend" "monitoring" "system" "master" "any-node" "wildcard" */ -}}
{{-   if eq $strategy "system" }}
{{-     if gt (index $context.Values.global.discovery.d8SpecificNodeCountByRole "system" | int) 0 }}
nodeSelector:
  node-role.deckhouse.io/system: ""
{{-     else }}
nodeSelector:
  node-role.kubernetes.io/control-plane: ""
{{-     end }}
{{-   end }}
{{- end }}