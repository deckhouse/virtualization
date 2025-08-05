{{- /* Helpers to add kube-api-rewriter sidecar container to a pod.

To connect to kube-api-rewriter main controller should has KUBECONFIG env,
volumeMount with kubeconfig, and Pod should has volume with kubeconfig ConfigMap.

These settings are provided by helpers:

- kube_api_rewriter.kubeconfig_env defines KUBECONFIG env with file from the
  mounted ConfigMap.
- kube_api_rewriter.kubeconfig_volume_mount defines volumeMount for kubeconfig ConfigMap.
- kube_api_rewriter.kubeconfig_volume defines volume with kubeconfig ConfigMap.

Kube-api-rewriter sidecar should be the first container in the Pod, to
main controller not fail on start.

Kube-api-rewriter sidecar works in 2 modes: without webhook or with webhook rewriting.

Sidecar without webhook is the simplest one:

spec:
  template:
    spec:
      containers:
        {{ include "kube_api_rewriter.sidecar_container" . | nindent 8 }}
        - name: main-controller
          ...
          env:
            {{- include "kube_api_rewriter.kubeconfig_env" . | nindent 12 }}
            ...
          volumeMounts:
            {{- include "kube_api_rewriter.kubeconfig_volume_mount" . | nindent 12 }}
            ...
      volumes:
        {{- include "kube_api_rewriter.kubeconfig_volume" | nindent 8 }}
        ...


Webhook mode requires additional settings:

- WEBHOOK_ADDRESS - address of the webhook in the main controller
- WEBHOOK_CERT_FILE - path to the webhook certificate file.
- WEBHOOK_KEY_FILE - path to the webhook key file.
- webhookCertsVolumeName - name of the Pod volume with webhook certificates.
- webhookCertsMountPath - path to mount the webhook certificates.

The assumption here is that main controller has a webhook server and
certificates are already mounted in the Pod, so kube-api-rewriter
can use certificates from that volume to impersonate the webhook server.

Example of adding kube-api-rewriter to the Deployment:

spec:
  template:
    spec:
      containers:
      {{- $rewriterSettings := dict }}
      {{- $_ := set $rewriterSettings "WEBHOOK_ADDRESS" "https://127.0.0.1:6443" }}
      {{- $_ := set $rewriterSettings "WEBHOOK_CERT_FILE" "/etc/webhook-certificates/tls.crt" }}
      {{- $_ := set $rewriterSettings "WEBHOOK_KEY_FILE" "/etc/webhook-certificates/tls.key" }}
      {{- $_ := set $rewriterSettings "webhookCertsVolumeName" "webhook-certs" }}
      {{- $_ := set $rewriterSettings "webhookCertsMountPath" "/etc/webhook-certificates" }}
      {{- include "kube_api_rewriter.sidecar_container" (tuple . $rewriterSettings) | nindent 6 }}
        - name: main-controller
          ...
          env:
            {{- include "kube_api_rewriter.kubeconfig_env" . | nindent 12 }}
            ...
          ports:
            - containerPort: 6443  # Goes to the WEBHOOK_ADDRESS
              name: webhooks
              protocol: TCP
          volumeMounts:
            {{- include "kube_api_rewriter.kubeconfig_volume_mount" . | nindent 12 }}
            - name: webhook-certs
              mountPath: /etc/webhook-certificates  # Goes to the webhookCertsMountPath
              readOnly: true
            ...
      volumes:
        {{- include "kube_api_rewriter.kubeconfig_volume" | nindent 8 }}
        - name: webhook-certs  # Name of the existing volume goes to the webhookCertsVolumeName.
          secret:
            optional: true
            secretName: webhook-certs
        ...

 */ -}}

{{- define "kube_api_rewriter.image" -}}
{{- include "helm_lib_module_image" (list . "kubeApiRewriter") | toJson -}}
{{- end -}}


{{- define "kube_api_rewriter.kubeconfig_env" -}}
- name: KUBECONFIG
  value: /kubeconfig.local/kube-api-rewriter.kubeconfig
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

{{- /* Container port for the pprof server */ -}}
{{- define "kube_api_rewriter.pprof_container_port" -}}
- containerPort: {{ include "kube_api_rewriter.pprof_port" . }}
  name: pprof
  protocol: TCP
{{- end }}

{{- /* Sidecar container spec with kube-api-rewriter */ -}}
{{- /* Usage without the webhook proxy: {{ include kube_api_rewriter.sidecar_container . }} */ -}}
{{- /* Usage with the webhook: {{ include kube_api_rewriter.sidecar_container (tuple . $webhookSettings) }} */ -}}
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
    - name: MONITORING_BIND_ADDRESS
      value: "127.0.0.1:9090"
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
      path: /healthz
      port: 9090
      scheme: HTTP
    initialDelaySeconds: 10
  readinessProbe:
    httpGet:
      path: /readyz
      port: 9090
      scheme: HTTP
    initialDelaySeconds: 10
  terminationMessagePath: /dev/termination-log
  terminationMessagePolicy: File
  {{- if $isWebhook }}
  volumeMounts:
    {{- include "kube_api_rewriter.webhook_volume_mount" (tuple $settings.webhookCertsVolumeName $settings.webhookCertsMountPath) | nindent 4 }}
  {{- end }}
  ports:
  {{- if eq $ctx.Values.virtualization.logLevel "debug" }}
  {{-   include "kube_api_rewriter.pprof_container_port" . | nindent 4 }}
  {{- end }}
  {{- if $isWebhook -}}
  {{-   include "kube_api_rewriter.webhook_container_port" .| nindent 4 }}
  {{- end -}}
{{- end -}}
