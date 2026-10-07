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

{{- define "kubevirt.featureGates" -}}
- HotplugVolumes
- Snapshot
- ExpandDisks
- CPUManager
- Sidecar
- VolumeSnapshotDataSource
{{/*
Even though these feature gates are enabled by default in KubeVirt 1.6.2, they still must not be removed from the config:
- VMLiveUpdateFeatures
- VolumeMigration
- VolumesUpdateStrategy

When rolling from KubeVirt 1.3.1 to KubeVirt 1.6.2, the config is updated first,
and only after some significant time is the virt-controller updated.
That is, the old version of virt-controller runs with the new config for a fairly long time.
It is important for the old version of virt-controller to see these feature gates in the new configuration,
because its business logic does not yet have their default behavior implemented.
*/}}
- VMLiveUpdateFeatures
- VolumeMigration
- VolumesUpdateStrategy
- HostDevicesWithDRA
- HostDevices
- HotplugHostDevicesWithDRA # custom feature gate - added in our KubeVirt fork, not present in upstream
- GPUsWithDRA
- VideoConfig # required for spec.domain.devices.video
{{- if has "HotplugCPUAndMemoryWithInPlaceResize" (.Values.virtualization.internal | dig "moduleConfig" "featureGates" list) }}
- InPlaceResize # custom feature gate - added in our KubeVirt fork, not present in upstream
{{- end }}
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


{{/*
  Note on `capabilities.drop: [ALL]` for the privileged containers below.

  It has no effect at runtime. containerd builds the OCI spec by applying the CRI
  security context first and `oci.WithPrivileged` afterwards; the latter composes
  `WithAllCurrentCapabilities`, which overwrites bounding/effective/permitted sets with
  the full capability set. A privileged container therefore always keeps every
  capability, no matter what `add`/`drop` say.

  It is set only to satisfy the Deckhouse security policy validation, which expects the
  field to be present. Dropping the capabilities for real requires `privileged: false`
  plus an explicit list of the capabilities, devices and mounts virt-handler needs.
*/}}
{{- define "kubevirt.virt_handler_security_contexts_strategic_patch" -}}
spec:
  template:
    spec:
      containers:
      - name: virt-handler
        securityContext:
          privileged: true
          readOnlyRootFilesystem: true
          allowPrivilegeEscalation: true
          runAsUser: 0
          runAsGroup: 0
          # The security policy exception matches a container only when runAsNonRoot is set
          # explicitly: an unset field never equals the allowed value.
          runAsNonRoot: false
          # No-op under `privileged: true`, required by the Deckhouse security policy.
          capabilities:
            drop:
              - ALL
          seLinuxOptions:
            level: s0
          # The value is irrelevant in practice: the runtime never applies a seccomp profile to a
          # privileged container (containerd returns early from generateSeccompSpecOpts when
          # privileged is set), so the container runs unconfined whatever is written here. The field
          # is present only because the Deckhouse security policy rejects a container without an
          # explicit profile, and Unconfined is what actually happens on the node.
          seccompProfile:
            type: Unconfined
      - name: virt-launcher-image-holder
        securityContext:
          readOnlyRootFilesystem: true
          allowPrivilegeEscalation: false
          # The container only holds the virt-launcher image on the node, so it needs no
          # privileges at all. Values are explicit because the security policy rejects an
          # unset runAsUser/runAsNonRoot pair.
          runAsUser: 64535
          runAsGroup: 64535
          runAsNonRoot: true
          capabilities:
            drop:
              - ALL
          seccompProfile:
            type: RuntimeDefault
      initContainers:
      - name: virt-launcher
        securityContext:
          privileged: true
          readOnlyRootFilesystem: true
          allowPrivilegeEscalation: true
          runAsUser: 0
          runAsGroup: 0
          # The security policy exception matches a container only when runAsNonRoot is set
          # explicitly: an unset field never equals the allowed value.
          runAsNonRoot: false
          # No-op under `privileged: true`, required by the Deckhouse security policy.
          capabilities:
            drop:
              - ALL
          # The value is irrelevant in practice: the runtime never applies a seccomp profile to a
          # privileged container (containerd returns early from generateSeccompSpecOpts when
          # privileged is set), so the container runs unconfined whatever is written here. The field
          # is present only because the Deckhouse security policy rejects a container without an
          # explicit profile, and Unconfined is what actually happens on the node.
          seccompProfile:
            type: Unconfined
{{- end -}}


{{- define "kubevirt.virt_handler_security_contexts_strategic_patch_json" -}}
'{{ include "kubevirt.virt_handler_security_contexts_strategic_patch" . | fromYaml | toJson }}'
{{- end }}


{{/* Calculate the cluster limit of live migrations transferring memory.
 This template returns:
  - Nothing if the limit is disabled.
  - The value of the max-active-migrations-per-cluster annotation if it is a number.
  - Count of nodes with virt-handler if kubevirt config is in 'Deployed' phase.
  - Current activeMigrationsPerCluster if config is not in 'Deployed' phase.
  - Default migrations count (2) if there is no kubevirt config.
 This behaviour prevents unnecessary helm installs during installation.
 */}}
{{- define "kubevirt.active_migrations_per_cluster" -}}
{{- $default := 2 -}}
{{- $limit := .Values.virtualization.internal | dig "virtConfig" "maxActiveMigrationsPerCluster" "" | toString -}}
{{- if eq $limit "disabled" -}}
{{- else if $limit -}}
{{-   $limit -}}
{{- else -}}
{{- $phase := .Values.virtualization.internal | dig "virtConfig" "phase" "<missing>" -}}
{{- if eq $phase "<missing>" -}}
{{-   $default -}}
{{- else -}}
{{-   $current := .Values.virtualization.internal | dig "virtConfig" "activeMigrationsPerCluster" 0 | int -}}
{{-   if or (eq $phase "Deployed") (le $current 0) -}}
{{-     max $default ( .Values.virtualization.internal |  dig "virtHandler" "nodeCount" 0 ) -}}
{{-   else -}}
{{-     $current -}}
{{-   end -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{/* The per-node limits of live migrations transferring memory. A shared budget for both
 directions replaces the separate ones; a limit of 0 is disabled and left out. */}}
{{- define "kubevirt.active_migrations_per_node" -}}
{{- $shared := int (.Values.virtualization.internal | dig "virtConfig" "maxActiveMigrationsPerNode" 0) -}}
{{- $outbound := int (.Values.virtualization.internal | dig "virtConfig" "maxActiveOutboundMigrationsPerNode" 1) -}}
{{- $inbound := int (.Values.virtualization.internal | dig "virtConfig" "maxActiveInboundMigrationsPerNode" 1) -}}
{{- if gt $shared 0 }}
activeMigrationsPerNode: {{ $shared }}
{{- else }}
{{- if gt $outbound 0 }}
activeOutboundMigrationsPerNode: {{ $outbound }}
{{- end }}
{{- if gt $inbound 0 }}
activeInboundMigrationsPerNode: {{ $inbound }}
{{- end }}
{{- end }}
{{- end -}}

{{- define "kubevirt.bandwidth_per_migration" -}}
{{- .Values.virtualization.internal | dig "virtConfig" "bandwidthPerMigration" "640Mi" -}}
{{- end -}}

{{- define "kubevirt.completion_timeout_per_gib" -}}
{{- .Values.virtualization.internal | dig "virtConfig" "completionTimeoutPerGiB" 800 -}}
{{- end -}}

{{- define "kubevirt.parallel_outbound_migrations_per_node" -}}
{{- $window := int (.Values.virtualization.internal | dig "virtConfig" "parallelOutboundMigrationsPerNode" 2) -}}
{{- if eq $window 0 -}}
{{- /* 0 means the outgoing migrations are not limited. KubeVirt cannot switch the cap off,
 so 1000000 (far above any realistic migration count) stands in for "unlimited". */ -}}
{{- 1000000 -}}
{{- else -}}
{{- $window -}}
{{- end -}}
{{- end -}}

{{- define "kubevirt.progress_timeout" -}}
{{- .Values.virtualization.internal | dig "virtConfig" "progressTimeout" 150 -}}
{{- end -}}

{{- define "kubevirt.disable_tls" -}}
{{- .Values.virtualization.internal | dig "virtConfig" "disableTLS" false -}}
{{- end -}}

{{- define "kubevirt.migrations" -}}
bandwidthPerMigration: {{ include "kubevirt.bandwidth_per_migration" . | quote }}
completionTimeoutPerGiB: {{ include "kubevirt.completion_timeout_per_gib" . }}
disableTLS: {{ include "kubevirt.disable_tls" . }}
{{- /* KubeVirt counts migrations waiting with a prepared target against parallelMigrationsPerCluster,
 so the cluster limit is set on the migrations transferring memory instead. KubeVirt cannot switch
 the cap off, so 1000000 (far above any realistic migration count) stands in for "unlimited". */}}
parallelMigrationsPerCluster: 1000000
parallelOutboundMigrationsPerNode: {{ include "kubevirt.parallel_outbound_migrations_per_node" . }}
progressTimeout: {{ include "kubevirt.progress_timeout" . }}
{{- include "kubevirt.active_migrations_per_node" . }}
{{- with include "kubevirt.active_migrations_per_cluster" . }}
activeMigrationsPerCluster: {{ . }}
{{- end }}
{{- end -}}
