{{- define "cdi.strategic_affinity_patch" -}}
  {{- $labelValue := index . 0 -}}
  '{{ include "cdi.tmplAntiAffinity" (list $labelValue) | fromYaml | toJson }}'
{{- end }}

{{- define "cdi.tmplAntiAffinity" -}}
{{- $labelValue := index . 0 -}}
spec:
  template:
    spec:
      affinity:
        podAntiAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
          - labelSelector:
              matchExpressions:
              - key: app
                operator: In
                values:
                - {{ $labelValue }}
            topologyKey: kubernetes.io/hostname
{{- end -}}


{{- define "cdi.strategic_kubeproxy_patch" -}}
  {{- $context := index . 0 -}}
  {{- $containerName := index . 1 -}}
  {{- $webhookProxy := index . 2 -}}
  '{{ include "cdi.tmplKubeProxy" (list $context $containerName $webhookProxy) | fromYaml | toJson }}'
{{- end }}

{{- define "cdi.tmplKubeProxy" -}}
  {{- $ctx := index . 0 -}}
  {{- $containerName := index . 1 -}}
  {{- $webhookProxy := index . 2 -}}
  {{- $proxyImage := include "helm_lib_module_image" (list $ctx "kubeApiProxy") }}
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
          {{- if not ( $ctx.Values.global.enabledModules | has "vertical-pod-autoscaler-crd") }}
            cpu: 10m
            memory: 150Mi
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
        env:
        - name: POD_NAMESPACE
          valueFrom:
            fieldRef:
              fieldPath: metadata.namespace
        - name: WEBHOOK_PROXY
          value: '{{ $webhookProxy }}'
      - name: {{ $containerName }}
        env:
        - name: KUBECONFIG
          value: /kubeconfig.local/proxy.kubeconfig
        volumeMounts:
        - name: kube-api-proxy-kubeconfig
          mountPath: /kubeconfig.local
{{- end -}}
