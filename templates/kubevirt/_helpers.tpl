
{{- define "kubevirt.apiserver_kubeproxy_patch" -}}
  '{{ include "kubevirt.apiserver_kubeproxy_patch_tmpl" . | fromYaml | toJson }}'
{{- end }}


{{- define "kubevirt.apiserver_kubeproxy_patch_tmpl" -}}
  {{- $proxyImage := include "helm_lib_module_image" (list . "kubeApiProxy") }}
metadata:
  annotations:
    kubectl.kubernetes.io/default-container: virt-api
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
              value: https://127.0.0.1:8443
            - name: WEBHOOK_CERT_FILE
              value: /etc/virt-api/certificates/tls.crt
            - name: WEBHOOK_KEY_FILE
              value: /etc/virt-api/certificates/tls.key
          volumeMounts:
            - name: kubevirt-virt-api-certs
              mountPath: /etc/virt-api/certificates
              readOnly: true
        - name: virt-api
          command:
            - virt-api
            - --kubeconfig=/kubeconfig.local/proxy.kubeconfig
          volumeMounts:
            - name: kube-api-proxy-kubeconfig
              mountPath: /kubeconfig.local
{{- end -}}


