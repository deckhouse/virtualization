{{- define "kube_rbac_proxy.pod_spec_strategic_patch" -}}
{{- $ctx := index . 0 }}
{{- $settings := index . 1 }}

spec:
  template:
    spec:
      containers:
      - name: {{ $settings.containerName | default "kube-rbac-proxy" }}
        {{- include "helm_lib_module_container_security_context_read_only_root_filesystem" $ctx | nindent 8 }}
        image: {{ include "helm_lib_module_common_image" (list $ctx "kubeRbacProxy") }}
        args:
        - "--secure-listen-address=$(KUBE_RBAC_PROXY_LISTEN_ADDRESS):{{ $settings.listenPort | default "8082" }}"
        - "--client-ca-file={{ $settings.clientCAFile | default "/etc/kube-rbac-proxy/ca.crt" }}"
        - "--v={{ $settings.logLevel | default "2" }}"
        - "--logtostderr=true"
        - "--stale-cache-interval={{ $settings.staleCacheInterval | default "1h30m" }}"
        - "--livez-path={{ $settings.livezPath | default "/livez" }}"
        env:
        - name: KUBE_RBAC_PROXY_LISTEN_ADDRESS
          valueFrom:
            fieldRef:
              fieldPath: status.podIP
        - name: KUBE_RBAC_PROXY_CONFIG
          value: |
            excludePaths:
              - {{ $settings.excludePath | default "/config" }}
            upstreams:
              - upstream: {{ $settings.upstream | default "http://127.0.0.1:8080/metrics" }}
                path: {{ $settings.path | default "/metrics" }}
                authorization:
                  resourceAttributes:
                    namespace: {{ $settings.namespace }}
                    apiGroup: {{ $settings.apiGroup }}
                    apiVersion: {{ $settings.apiVersion }}
                    resource: {{ $settings.resource }}
                    subresource: {{ $settings.subresource }}
                    name: {{ $settings.name }}
        resources:
          requests:
            {{- include "helm_lib_module_ephemeral_storage_only_logs" $ctx | nindent 12 }}
            {{- if not ( $ctx.Values.global.enabledModules | has "vertical-pod-autoscaler") }}
            {{- include "helm_lib_container_kube_rbac_proxy_resources" $ctx | nindent 12 }}
            {{- end }}
        volumeMounts:
        - name: kube-rbac-proxy-ca
          mountPath: /etc/kube-rbac-proxy
        ports:
        - containerPort: {{ $settings.listenPort | default "8082" }}
          name: {{ $settings.portName | default "https-metrics" }}
          protocol: TCP
      volumes:
      - name: kube-rbac-proxy-ca
        configMap:
          defaultMode: 420
          name: kube-rbac-proxy-ca.crt
{{- end }}

{{- define "kube_rbac_proxy.image" -}}
{{- include "helm_lib_module_common_image" (list . "kubeRbacProxy") -}}
{{- end -}}

{{- define "kube_rbac_proxy.pod_spec_strategic_patch_json" -}}
  '{{ include "kube_rbac_proxy.pod_spec_strategic_patch" . | fromYaml | toJson }}'
{{- end }}
