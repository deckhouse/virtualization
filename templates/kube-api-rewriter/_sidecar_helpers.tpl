{{- define "kube_api_rewriter.image" -}}
{{- include "helm_lib_module_image" (list . "kubeApiRewriter") | toJson -}}
{{- end -}}

{{- define "kube_api_rewriter.kubeconfig_env" -}}
{{- $settings := dict -}}
{{- if (kindIs "slice" .) -}}
{{-   if ge (len .) 2 -}}
{{-     $settings = index . 1 -}}
{{-   end -}}
{{- end -}}
{{- $kubeconfigFilename := $settings.kubeconfigFilename | default "kube-api-rewriter.kubeconfig" -}}
- name: KUBECONFIG
  value: /kubeconfig.local/{{ $kubeconfigFilename }}
{{- end }}

{{- define "kube_api_rewriter.kubeconfig_volume" -}}
- name: kube-api-rewriter-kubeconfig
  configMap:
    defaultMode: 0644
    name: kube-api-rewriter-kubeconfig
{{- end }}

{{- define "kube_api_rewriter.kubeconfig_volume_mount" -}}
- name: kube-api-rewriter-kubeconfig
  mountPath: /kubeconfig.local
{{- end }}

{{- define "kube_api_rewriter.webhook_volume_mount" -}}
{{- $volumeName := index . 0 -}}
{{- $mountPath := index . 1 -}}
- mountPath: {{ $mountPath }}
  name: {{ $volumeName }}
  readOnly: true
{{- end }}

{{- define "kube_api_rewriter.webhook_container_port" -}}
- containerPort: {{ include "kube_api_rewriter.webhook_port" . }}
  name: {{ include "kube_api_rewriter.webhook_port_name" . }}
  protocol: TCP
{{- end }}

{{- define "kube_api_rewriter.pprof_container_port" -}}
- containerPort: {{ include "kube_api_rewriter.pprof_port" . }}
  name: pprof
  protocol: TCP
{{- end }}

{{- define "kube_api_rewriter.sidecar_container" -}}
  {{- $ctx := . -}}
  {{- $settings := dict -}}
  {{- if (kindIs "slice" .) -}}
  {{-   $ctx = index . 0 -}}
  {{-   if ge (len .) 2 -}}
  {{-     $settings = index . 1 -}}
  {{-   end -}}
  {{- end -}}
  {{- $isWebhook := hasKey $settings "WEBHOOK_ADDRESS" -}}
  {{- $injectPodIP := $settings.injectPodIP | default false -}}
  {{- $healthzPort := $settings.healthzPort | default 8082 -}}
  {{- $healthzPath := $settings.healthzPath | default "/proxy/healthz" -}}
  {{- $readyzPath := $settings.readyzPath | default "/proxy/readyz" -}}
  {{- $clientProxyPort := $settings.clientProxyPort | default (include "kube_api_rewriter.client_proxy_port" $ctx | int) -}}
  {{- $monitoringBindAddress := $settings.monitoringBindAddress | default "127.0.0.1:9090" -}}
  {{- $pprofBindAddress := $settings.pprofBindAddress | default (printf ":%s" (include "kube_api_rewriter.pprof_port" $ctx)) -}}
  {{- $pprofPort := last (splitList ":" $pprofBindAddress) | int -}}
  {{- $probeScheme := $settings.probeScheme | default "HTTPS" -}}
- name: {{ include "kube_api_rewriter.sidecar_name" $ctx }}
  image: {{ include "kube_api_rewriter.image" $ctx }}
  imagePullPolicy: IfNotPresent
  env:
    {{- if $isWebhook }}
    - name: WEBHOOK_ADDRESS
      value: "{{ $settings.WEBHOOK_ADDRESS }}"
    - name: WEBHOOK_CERT_FILE
      value: "{{ $settings.WEBHOOK_CERT_FILE }}"
    - name: WEBHOOK_KEY_FILE
      value: "{{ $settings.WEBHOOK_KEY_FILE }}"
    {{- end }}
    {{- if $injectPodIP }}
    - name: POD_IP
      valueFrom:
        fieldRef:
          fieldPath: status.podIP
    {{- end }}
    - name: CLIENT_PROXY_PORT
      value: "{{ $clientProxyPort }}"
    - name: MONITORING_BIND_ADDRESS
      value: "{{ $monitoringBindAddress }}"
    {{- if $settings.monitoringAuth }}
    - name: MONITORING_AUTH
      value: {{ $settings.monitoringAuth | toJson | quote }}
    {{- end }}
    {{- if eq (include "moduleLogLevel" $ctx) "debug" }}
    - name: PPROF_BIND_ADDRESS
      value: "{{ $pprofBindAddress }}"
    {{- end }}
    {{- include "kube_api_rewriter.env" $ctx | nindent 4 }}
  resources:
    requests:
      {{- include "helm_lib_module_ephemeral_storage_only_logs" . | nindent 6 }}
      {{- if not ( $ctx.Values.global.enabledModules | has "vertical-pod-autoscaler-crd") }}
      {{- include "kube_api_rewriter.resources" . | nindent 6 }}
      {{- end }}
  securityContext:
    allowPrivilegeEscalation: false
    readOnlyRootFilesystem: true
    capabilities:
      drop:
        - ALL
    seccompProfile:
      type: RuntimeDefault
  livenessProbe:
    httpGet:
      path: {{ $healthzPath }}
      port: {{ $healthzPort }}
      scheme: {{ $probeScheme }}
    initialDelaySeconds: 10
  readinessProbe:
    httpGet:
      path: {{ $readyzPath }}
      port: {{ $healthzPort }}
      scheme: {{ $probeScheme }}
    initialDelaySeconds: 10
  terminationMessagePath: /dev/termination-log
  terminationMessagePolicy: File
  {{- if $isWebhook }}
  volumeMounts:
    {{- include "kube_api_rewriter.webhook_volume_mount" (tuple $settings.webhookCertsVolumeName $settings.webhookCertsMountPath) | nindent 4 }}
  {{- end }}
  ports:
  {{- if eq (include "moduleLogLevel" $ctx) "debug" }}
  - containerPort: {{ $pprofPort }}
    name: pprof
    protocol: TCP
  {{- end }}
  {{- if $isWebhook }}
  - containerPort: {{ include "kube_api_rewriter.webhook_port" $ctx }}
    name: {{ include "kube_api_rewriter.webhook_port_name" $ctx }}
    protocol: TCP
  {{- end -}}
{{- end -}}
