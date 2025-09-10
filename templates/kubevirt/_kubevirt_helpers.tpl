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

{{- define "kubevirt.delve_strategic_patch" -}}
{{- $image := index . 0 }}
spec:
  template:
    spec:
      containers:
      - name: {{ printf "%s" ( split "/" $image)._1 }}
        command: null
        livenessProbe: null
        readinessProbe: null
        ports:
        - containerPort: 2345
          name: tcp-dlv-2345
          protocol: TCP
{{- end -}}

{{- define "kubevirt.delve_strategic_patch_json" -}}
'{{ include "kubevirt.delve_strategic_patch" . | fromYaml | toJson }}'
{{- end }}

{{/* Calculate parallel migrations per cluster.
 This template returns:
  - Count of nodes with virt-handler if kubevirt config is in 'Deployed' phase.
  - Current parallelMigrationsPerCluster if config is not in 'Deployed' phase.
  - Default migrations count (2) if there is no kubevirt config.
 This behaviour prevents unnecessary helm installs during installation.

 Values from
 */}}
{{- define "kubevirt.parallel_migrations_per_cluster" -}}
{{- $default := 2 -}}
{{- $phase := .Values.virtualization.internal | dig "virtConfig" "phase" "<missing>" -}}
{{- if eq $phase "<missing>" -}}
{{-   $default -}}
{{- else -}}
{{-   if eq $phase "Deployed" -}}
{{-     max $default ( .Values.virtualization.internal |  dig "virtHandler" "nodeCount" 0 ) -}}
{{-   else -}}
{{-     .Values.virtualization.internal | dig "virtConfig" "parallelMigrationsPerCluster" $default -}}
{{-   end -}}
{{- end -}}
{{- end -}}
