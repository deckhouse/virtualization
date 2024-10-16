{{- define "kube_api_rewriter.env" -}}
- name: LOG_LEVEL
  value: {{ .Values.virtualization.logLevel }}
{{- if eq .Values.virtualization.logLevel "debug" }}
- name: PPROF_BIND_ADDRESS
  value: ":8129"
{{- end }}
{{- end -}}

{{- define "kube_api_rewriter.vpa_container_policy" -}}
- containerName: proxy
  minAllowed:
    cpu: 10m
    memory: 150Mi
  maxAllowed:
    cpu: 20m
    memory: 250Mi
{{- end -}}

{{- define "kubeproxy_resources" -}}
cpu: 100m
memory: 150Mi
{{- end -}}

{{- define "nowebhook_kubeproxy_patch" -}}
  '{{ include "nowebhook_kubeproxy_patch_tmpl" . | fromYaml | toJson }}'
{{- end }}

{{- define "nowebhook_kubeproxy_patch_tmpl" -}}
  {{- $ctx := index . 0 -}}
  {{- $containerName := index . 1 -}}
  {{- $proxyImage := include "helm_lib_module_image" (list $ctx "kubeApiProxy") }}
spec:
  template:
    metadata:
      annotations:
        kubectl.kubernetes.io/default-container: {{ $containerName }}
    spec:
      volumes:
      - name: kube-api-proxy-kubeconfig
        configMap:
          defaultMode: 0644
          name: kube-api-proxy-kubeconfig
{{- if eq $ctx.Values.virtualization.logLevel "debug" }}
      ports:
      - containerPort: 8129
        name: pprof
        protocol: TCP
{{- end }}
      containers:
      - name: proxy
        image: {{ $proxyImage }}
        imagePullPolicy: IfNotPresent
        env:
        {{- include "kube_api_rewriter.env" $ctx | nindent 8 }}
        resources:
          requests:
          {{- include "helm_lib_module_ephemeral_storage_only_logs" . | nindent 12 }}
          {{- if not ( $ctx.Values.global.enabledModules | has "vertical-pod-autoscaler-crd") }}
          {{- include "kubeproxy_resources" . | nindent 12 }}
          {{- end }}
        securityContext:
          allowPrivilegeEscalation: false
          capabilities:
            drop:
              - ALL
          seccompProfile:
            type: RuntimeDefault
        terminationMessagePath: /dev/termination-log
        terminationMessagePolicy: File
      - name: {{ $containerName }}
        env:
        - name: KUBECONFIG
          value: /kubeconfig.local/proxy.kubeconfig
        volumeMounts:
        - name: kube-api-proxy-kubeconfig
          mountPath: /kubeconfig.local
{{- end -}}
