{{- define "kube_rbac_proxy.sidecar_container" -}}
{{- $ctx := index . 0 }}
{{- $settings := index . 1 }}
- name: {{ $settings.containerName | default "kube-rbac-proxy" }}
  {{- include "helm_lib_module_container_security_context_read_only_root_filesystem_capabilities_drop_all_pss_restricted" $ctx | nindent 2 }}
  {{- if eq $settings.runAsUserNobody true }}
    runAsNonRoot: true
    runAsUser: 65534
    runAsGroup: 65534
  {{- end }}
  image: {{ include "helm_lib_module_common_image" (list $ctx "kubeRbacProxy") }}
  imagePullPolicy: IfNotPresent
  terminationMessagePath: /dev/termination-log
  terminationMessagePolicy: File
  args:
  - "--secure-listen-address=$(KUBE_RBAC_PROXY_LISTEN_ADDRESS):{{ $settings.listenPort | default "8082" }}"
  - "--v={{ $settings.logLevel | default "2" }}"
  - "--logtostderr=true"
  - "--stale-cache-interval={{ $settings.staleCacheInterval | default "1h30m" }}"
  {{- if hasKey $settings "ignorePaths" }}
  - "--ignore-paths={{ $settings.ignorePaths }}"
  {{- end }}
  env:
  - name: KUBE_RBAC_PROXY_LISTEN_ADDRESS
    valueFrom:
      fieldRef:
        apiVersion: v1
        fieldPath: status.podIP
  - name: KUBE_RBAC_PROXY_CONFIG
    value: |
      excludePaths:
        - {{ $settings.excludePath | default "/config" }}
      upstreams:
        {{- range $settings.upstreams }}
        - upstream: {{ .upstream }}
          path: {{ .path }}
          authorization:
            resourceAttributes:
              namespace: {{ .namespace | default "d8-virtualization" }}
              apiGroup: {{ .apiGroup | default "apps" }}
              apiVersion: {{ .apiVersion | default "v1" }}
              resource: {{ .resource | default "deployments" }}
              subresource: {{ .subresource | default "prometheus-metrics" }}
              name: {{ .name }}
        {{- end }}
  resources:
    requests:
      {{- include "helm_lib_module_ephemeral_storage_only_logs" $ctx | nindent 6 }}
      {{- if not ( $ctx.Values.global.enabledModules | has "vertical-pod-autoscaler") }}
      {{- include "helm_lib_container_kube_rbac_proxy_resources" $ctx | nindent 6 }}
      {{- end }}
  ports:
  - containerPort: {{ $settings.listenPort | default "8082" }}
    name: {{ $settings.portName | default "https-metrics" }}
    protocol: TCP
  livenessProbe:
    tcpSocket:
      port: {{ $settings.portName | default "https-metrics" }}
    initialDelaySeconds: 10
  readinessProbe:
    tcpSocket:
      port: {{ $settings.portName | default "https-metrics" }}
    initialDelaySeconds: 10
{{- end -}}

{{- define "kube_rbac_proxy.pod_spec_strategic_patch" -}}
{{- $ctx := index . 0 }}
{{- $settings := index . 1 }}
spec:
  template:
    spec:
      containers:
      {{- include "kube_rbac_proxy.sidecar_container" (tuple $ctx $settings) | nindent 6 }}
{{- end }}

{{- define "kube_rbac_proxy.image" -}}
{{- include "helm_lib_module_common_image" (list . "kubeRbacProxy") -}}
{{- end -}}

{{- define "kube_rbac_proxy.vpa_container_policy" -}}
- containerName: {{ $.containerName | default "kube-rbac-proxy" }}
  minAllowed:
    cpu: 10m
    memory: 15Mi
  maxAllowed:
    cpu: 20m
    memory: 30Mi
{{- end -}}

{{- define "kube_rbac_proxy.pod_spec_strategic_patch_json" -}}
  '{{ include "kube_rbac_proxy.pod_spec_strategic_patch" . | fromYaml | toJson }}'
{{- end }}
