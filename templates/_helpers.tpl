{{- define "priorityClassName" -}}
system-cluster-critical
{{- end }}

{{- define "spec_template_spec_antiaffinity_patch" -}}
  {{- $key := index . 0 -}}
  {{- $labelValue := index . 1 -}}
  '{{ include "tmplAntiAffinity" (list $key $labelValue) | fromYaml | toJson }}'
{{- end }}

{{- define "tmplAntiAffinity" -}}
  {{- $key := index . 0 -}}
  {{- $labelValue := index . 1 -}}
spec:
  template:
    spec:
      affinity:
        podAntiAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
          - labelSelector:
              matchExpressions:
              - key: {{ $key }}
                operator: In
                values:
                - {{ $labelValue }}
            topologyKey: kubernetes.io/hostname
{{- end -}}

{{- define "spec_strategy_rolling_update_patch" -}}
  '{{ include "tmplSpecStrategyRollingUpdate" . | fromYaml | toJson }}'
{{- end }}

{{- define "tmplSpecStrategyRollingUpdate" -}}
spec:
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxSurge: 0
      maxUnavailable: 1
{{- end -}}

{{- /* Returns node selector for workloads, only system or control-plane */ -}}
{{- define "virt_helper_system_master_node_selector" }}
{{-   $context := index . 0 }} {{- /* Template context with .Values, .Chart, etc */ -}}
{{-   if gt (index $context.Values.global.discovery.d8SpecificNodeCountByRole "system" | int) 0 }}
nodeSelector:
  node-role.deckhouse.io/system: ""
{{-   else }}
nodeSelector:
  node-role.kubernetes.io/control-plane: ""
{{-   end }}
{{- end }}

{{- /* Return logLevel as a string. */}}
{{- define "moduleLogLevel" -}}
{{- dig "logLevel" "" .Values.virtualization -}}
{{- end }}

{{- /* Return logFormat as a string. */}}
{{- define "moduleLogFormat" -}}
{{- dig "logFormat" "" .Values.virtualization -}}
{{- end }}

{{- define "hasValidModuleConfig" -}}
{{- if (hasKey .Values.virtualization.internal "moduleConfig") -}}
true
{{- end }}
{{- end }}

{{/* https://werf.io/docs/v2/usage/deploy/tracking.html#disabling-state-tracking-and-ignoring-resource-errors-werf-only */}}
{{- define "werf.annotations.disabling_state_tracking_and_ignoring_resource_errors" }}
annotations:
  werf.io/fail-mode: IgnoreAndContinueDeployProcess
  werf.io/track-termination-mode: NonBlocking
{{- end }}

{{- define "vpa.policyUpdateMode" -}}
{{-   $kubeVersion := .Values.global.discovery.kubernetesVersion -}}
{{-   $updateMode := "" -}}
{{-   if semverCompare ">=1.33.0" $kubeVersion -}}
{{-     $updateMode = "InPlaceOrRecreate" -}}
{{-   else -}}
{{-     $updateMode = "Recreate" -}}
{{-   end }}
{{- $updateMode }}
{{- end }}

{{- /*
  virtualization.uploadViaAPIGatewayEnabled returns "true" (non-empty) when the
  uploader should be exposed via the Gateway API instead of Ingress: the alb
  module is enabled, a Gateway to attach to is known and a publicDomainTemplate
  is set (needed for the upload host and its TLS certificate). Empty string
  means disabled.
*/ -}}
{{- define "virtualization.uploadViaAPIGatewayEnabled" -}}
{{- $gateway := dict -}}
{{- include "helm_lib_module_gateway" (list . $gateway) -}}
{{- if and (.Values.global.enabledModules | has "alb") $gateway.name (ne "<missing>" (dig "modules" "publicDomainTemplate" "<missing>" .Values.global)) (include "helm_lib_module_https_route_tls_enabled" .) -}}
true
{{- end -}}
{{- end -}}

{{- /*
  virtualization.uploaderListenerSetName returns the name of the ListenerSet that
  publishes the upload host on the Gateway. The virtualization-controller attaches
  every per-upload HTTPRoute to it, so the name is part of the contract between
  this chart and the controller.
*/ -}}
{{- define "virtualization.uploaderListenerSetName" -}}
virtualization
{{- end -}}

{{- /*
  virtualization.uploaderListenerName returns the name of the ListenerSet listener
  the HTTPRoutes reference via sectionName.
*/ -}}
{{- define "virtualization.uploaderListenerName" -}}
virtualization
{{- end -}}

{{- /*
  virtualization.uploaderGatewayHost returns the host image upload is published on
  through the Gateway API: the module public domain, the same host the Ingress
  exposure uses. Only one of the two exposures is active at a time, so they do not
  compete for the host.
*/ -}}
{{- define "virtualization.uploaderGatewayHost" -}}
{{- include "helm_lib_module_public_domain" (list . "virtualization") -}}
{{- end -}}

{{- /*
  virtualization.uploaderGatewayTLSSecretName returns the Secret the ListenerSet
  terminates TLS with. With cert-manager the Gateway API exposure orders its own
  certificate, so the listener does not depend on the Ingress one being issued. A
  custom certificate is a single cluster-wide one, so it is reused as is.
*/ -}}
{{- define "virtualization.uploaderGatewayTLSSecretName" -}}
{{- if eq (include "helm_lib_module_https_mode" .) "CertManager" -}}
httproute-tls
{{- else -}}
{{- include "helm_lib_module_https_secret_name" (list . "ingress-tls") -}}
{{- end -}}
{{- end -}}

