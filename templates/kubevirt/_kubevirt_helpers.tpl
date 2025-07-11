{{- define "kubevirt.virthandler_nodeaffinity_strategic_patch" -}}
  {{- $dvpNestingLevel := . -}}
spec:
  template:
    spec:
      affinity:
        nodeAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
            nodeSelectorTerms:
            - matchExpressions:
              - key: node.deckhouse.io/dvp-nesting-level
                operator: In
                values:
                - "{{ $dvpNestingLevel }}"
            - matchExpressions:
              - key: node.deckhouse.io/dvp-nesting-level
                operator: DoesNotExist
{{- end -}}

{{- define "kubevirt.virthandler_nodeaffinity_strategic_patch_json" -}}
  '{{ include "kubevirt.virthandler_nodeaffinity_strategic_patch" . | fromYaml | toJson }}'
{{- end }}

{{- define "kubevirt.virthandler_nodeseletor_strategic_patch" -}}
  {{- $defaultLabels := dict "kubernetes.io/os" "linux" "virtualization.deckhouse.io/kvm-enabled" "true" -}}
spec:
  template:
    spec:
      nodeSelector:
{{ $defaultLabels | toYaml | nindent 8 }}
{{- end -}}

{{- define "kubevirt.virthandler_nodeseletor_strategic_patch_json" -}}
  '{{ include "kubevirt.virthandler_nodeseletor_strategic_patch" . | fromYaml | toJson }}'
{{- end }}

{{- define "kubevirt.logVerbosity" -}}
  {{- if eq . "error" -}}2
  {{- else if eq . "warning" -}}3
  {{- else if eq . "info" -}}4
  {{- else if eq . "debug" -}}7
  {{- else -}}4
  {{- end -}}
{{- end -}}
