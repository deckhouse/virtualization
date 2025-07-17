{{- /* Returns node selector for workloads depend on strategy. */ -}}
{{- define "virt_helper_node_selector" }}
{{-   $context := index . 0 }} {{- /* Template context with .Values, .Chart, etc */ -}}
{{-   $strategy := index . 1 | include "helm_lib_internal_check_node_selector_strategy" }} {{- /* strategy, one of "system" "master" */ -}}
{{-   $module_values := dict }}
{{-   if lt (len .) 3 }}
{{-     $module_values = (index $context.Values (include "helm_lib_module_camelcase_name" $context)) }}
{{-   else }}
{{-     $module_values = index . 2 }}
{{-   end }}

{{-   if eq $strategy "system" }}
{{-     if gt (index $context.Values.global.discovery.d8SpecificNodeCountByRole "system" | int) 0 }}
nodeSelector:
  node-role.deckhouse.io/system: ""
{{-     else if gt (index $context.Values.global.discovery.d8SpecificNodeCountByRole "master" | int) 0 }}
nodeSelector:
  node-role.deckhouse.io/control-plane: ""
{{-     end }}
{{-   end }}
{{- end }}