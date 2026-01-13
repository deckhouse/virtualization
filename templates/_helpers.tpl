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

{{- /* Returns node selector for workloads, only system or control-plane */ -}}
{{- define "virt_helper_system_master_node_selector" }}
{{-   $context := index . 0 }} {{- /* Template context with .Values, .Chart, etc */ -}}
{{-   if gt (index $context.Values.global.discovery.d8SpecificNodeCountByRole "system" | int) 0 }}
nodeSelector:
  node-role.deckhouse.io/system: ""
{{-   else }}
nodeSelector:
  node-role.kubernetes.io/control-plane: ""
{{-   end }}
{{- end }}

{{- /* Return logLevel as a string. */}}
{{- define "moduleLogLevel" -}}
{{- dig "logLevel" "" .Values.virtualization -}}
{{- end }}

{{- /* Return logFormat as a string. */}}
{{- define "moduleLogFormat" -}}
{{- dig "logFormat" "" .Values.virtualization -}}
{{- end }}

{{- define "hasValidModuleConfig" -}}
{{- if (hasKey .Values.virtualization.internal "moduleConfig" ) -}}
true
{{- end }}
{{- end }}

{{- define "vpa.policyUpdateMode" -}}
{{-   $kubeVersion := .Values.global.discovery.kubernetesVersion -}}
{{-   $updateMode := "" -}}
{{-   if semverCompare ">=1.33.0" $kubeVersion -}}
{{-     $updateMode = "InPlaceOrRecreate" -}}
{{-   else -}}
{{-     $updateMode = "Recreate" -}}
{{-   end }}
{{- $updateMode }}
{{- end }}