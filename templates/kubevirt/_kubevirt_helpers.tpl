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

{{- define "kubevirt.virt_handler_ports_json_patch" -}}
'[
  {
    "op":"replace",
    "path":"/spec/template/spec/containers/0/ports",
    "value":[
      {
        "containerPort":{{ include "virt_handler.port" . | int }},
        "name":"metrics",
        "protocol":"TCP"
      },
      {
        "containerPort":{{ include "virt_handler.console_server_port" . | int }},
        "name":"console",
        "protocol":"TCP"
      }
    ]
  }
]'
{{- end -}}

{{- define "kubevirt.virt_api_args_strategic_patch" -}}
spec:
  template:
    spec:
      containers:
      - name: virt-api
        args:
        - --port
        - "8443"
        - --console-server-port
        - {{ include "virt_handler.console_server_port" . | quote }}
        - --subresources-only
        - -v
        - "2"
{{- end -}}

{{- define "kubevirt.virt_api_args_strategic_patch_json" -}}
'{{ include "kubevirt.virt_api_args_strategic_patch" . | fromYaml | toJson }}'
{{- end }}

{{- define "kubevirt.virt_handler_args_strategic_patch" -}}
spec:
  template:
    spec:
      containers:
      - name: virt-handler
        args:
        - --port
        - {{ include "virt_handler.port" . | quote }}
        - --hostname-override
        - $(NODE_NAME)
        - --pod-ip-address
        - $(MY_POD_IP)
        - --max-metric-requests
        - "3"
        - --console-server-port
        - {{ include "virt_handler.console_server_port" . | quote }}
        - --migration-port-range-enabled
        - "true"
        - --migration-port-range-first
        - {{ include "virt_handler.migration_port_first" . | quote }}
        - --migration-port-range-last
        - {{ include "virt_handler.migration_port_last" . | quote }}
        - --graceful-shutdown-seconds
        - "315"
        - -v
        - "2"
{{- end -}}

{{- define "kubevirt.virt_handler_args_strategic_patch_json" -}}
'{{ include "kubevirt.virt_handler_args_strategic_patch" . | fromYaml | toJson }}'
{{- end }}

{{- define "kubevirt.virt_handler_probes_strategic_patch" -}}
spec:
  template:
    spec:
      containers:
      - name: virt-handler
        livenessProbe:
          httpGet:
            path: /healthz
            port: {{ include "virt_handler.port" . | int }}
            scheme: HTTPS
          failureThreshold: 3
          initialDelaySeconds: 15
          periodSeconds: 45
          successThreshold: 1
          timeoutSeconds: 10
        readinessProbe:
          httpGet:
            path: /healthz
            port: {{ include "virt_handler.port" . | int }}
            scheme: HTTPS
          failureThreshold: 3
          initialDelaySeconds: 15
          periodSeconds: 20
          successThreshold: 1
          timeoutSeconds: 10
{{- end -}}

{{- define "kubevirt.virt_handler_probes_strategic_patch_json" -}}
'{{ include "kubevirt.virt_handler_probes_strategic_patch" . | fromYaml | toJson }}'
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

{{- define "kubevirt.bandwidth_per_migration" -}}
{{- .Values.virtualization.internal | dig "virtConfig" "bandwidthPerMigration" "640Mi" -}}
{{- end -}}

{{- define "kubevirt.completion_timeout_per_gib" -}}
{{- .Values.virtualization.internal | dig "virtConfig" "completionTimeoutPerGiB" 800 -}}
{{- end -}}

{{- define "kubevirt.parallel_outbound_migrations_per_node" -}}
{{- .Values.virtualization.internal | dig "virtConfig" "parallelOutboundMigrationsPerNode" 2 -}}
{{- end -}}

{{- define "kubevirt.progress_timeout" -}}
{{- .Values.virtualization.internal | dig "virtConfig" "progressTimeout" 150 -}}
{{- end -}}

{{- define "kubevirt.disable_tls" -}}
{{- .Values.virtualization.internal | dig "virtConfig" "disableTLS" false -}}
{{- end -}}

{{- define "kubevirt.migrations" -}}
bandwidthPerMigration: {{ include "kubevirt.bandwidth_per_migration" . }}
completionTimeoutPerGiB: {{ include "kubevirt.completion_timeout_per_gib" . }}
disableTLS: {{ include "kubevirt.disable_tls" . }}
parallelMigrationsPerCluster: {{ include "kubevirt.parallel_migrations_per_cluster" . }}
parallelOutboundMigrationsPerNode: {{ include "kubevirt.parallel_outbound_migrations_per_node" . }}
progressTimeout: {{ include "kubevirt.progress_timeout" . }}
{{- end -}}
