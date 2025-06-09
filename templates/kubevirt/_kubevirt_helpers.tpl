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
                - {{ $dvpNestingLevel }}
            - matchExpressions:
              - key: node.deckhouse.io/dvp-nesting-level
                operator: DoesNotExist
{{- end -}}

{{- define "kubevirt.virthandler_nodeaffinity_strategic_patch_json" -}}
  '{{ include "kubevirt.virthandler_nodeaffinity_strategic_patch" . | fromYaml | toJson }}'
{{- end }}
