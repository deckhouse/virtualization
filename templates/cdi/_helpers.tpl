
{{- define "cdi.kubeproxy_resources" -}}
cpu: 100m
memory: 150Mi
{{- end -}}

{{- define "cdi.nowebhook_kubeproxy_patch" -}}
  '{{ include "cdi.nowebhook_kubeproxy_patch_tmpl" . | fromYaml | toJson }}'
{{- end }}

{{- define "cdi.nowebhook_kubeproxy_patch_tmpl" -}}
  {{- $ctx := index . 0 -}}
  {{- $containerName := index . 1 -}}
  {{- $proxyImage := include "helm_lib_module_image" (list $ctx "kubeApiProxy") }}
  {{- $proxyImage = "dev-registry.deckhouse.io/virt/dev/diafour/kube-api-proxy:latest" }}
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
        imagePullPolicy: Always
        command: ["/proxy"]
        resources:
          requests:
          {{- include "helm_lib_module_ephemeral_storage_only_logs" . | nindent 12 }}
          {{- if not ( $ctx.Values.global.enabledModules | has "vertical-pod-autoscaler-crd") }}
          {{- include "cdi.kubeproxy_resources" . | nindent 12 }}
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

{{- define "cdi.apiserver_kubeproxy_patch" -}}
  '{{ include "cdi.apiserver_kubeproxy_patch_tmpl" . | fromYaml | toJson }}'
{{- end }}

{{- define "cdi.apiserver_kubeproxy_patch_tmpl" -}}
  {{- $proxyImage := include "helm_lib_module_image" (list . "kubeApiProxy") }}
  {{- $proxyImage = "dev-registry.deckhouse.io/virt/dev/diafour/kube-api-proxy:latest" }}
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
        imagePullPolicy: Always
        command: ["/proxy"]
        resources:
          requests:
          {{- include "helm_lib_module_ephemeral_storage_only_logs" . | nindent 12 }}
          {{- if not ( .Values.global.enabledModules | has "vertical-pod-autoscaler-crd") }}
          {{- include "cdi.kubeproxy_resources" . | nindent 12 }}
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
        ports:
          - containerPort: 24192
            name: webhook-proxy
            protocol: TCP
        env:
        - name: WEBHOOK_ADDRESS
          value: "https://127.0.0.1:8443"
        - name: WEBHOOK_CERT_FILE
          value: "/var/run/certs/cdi-apiserver-server-cert/tls.crt"
        - name: WEBHOOK_KEY_FILE
          value: "/var/run/certs/cdi-apiserver-server-cert/tls.key"
        volumeMounts:
        - mountPath: /var/run/certs/cdi-apiserver-server-cert
          name: server-cert
          readOnly: true
      - name: cdi-apiserver
        env:
        - name: KUBECONFIG
          value: /kubeconfig.local/proxy.kubeconfig
        volumeMounts:
        - name: kube-api-proxy-kubeconfig
          mountPath: /kubeconfig.local
{{- end -}}
