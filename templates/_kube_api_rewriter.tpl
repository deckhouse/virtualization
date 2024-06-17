{{- define "kube_api_rewriter.env" -}}
- name: LOG_LEVEL
  value: Debug
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
metadata:
  annotations:
    kubectl.kubernetes.io/default-container: {{ $containerName }}
spec:
  template:
    spec:
      volumes:
      - name: kube-api-proxy-kubeconfig
        configMap:
          name: kube-api-proxy-kubeconfig
      containers:
      - name: proxy
        image: {{ $proxyImage }}
        imagePullPolicy: IfNotPresent
        env:
        {{- include "kube_api_rewriter.env" . | nindent 8 }}
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
