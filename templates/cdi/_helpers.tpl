{{- define "cdi.apiserver_kubeproxy_patch" -}}
  '{{ include "cdi.apiserver_kubeproxy_patch_tmpl" . | fromYaml | toJson }}'
{{- end }}

{{- define "cdi.apiserver_kubeproxy_patch_tmpl" -}}
  {{- $proxyImage := include "helm_lib_module_image" (list . "kubeApiProxy") }}
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
        resources:
          requests:
          {{- include "helm_lib_module_ephemeral_storage_only_logs" . | nindent 12 }}
          {{- if not ( .Values.global.enabledModules | has "vertical-pod-autoscaler-crd") }}
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
