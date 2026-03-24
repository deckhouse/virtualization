{{- define "virtualization-dra.isEnabled" -}}
{{- if eq (include "hasValidModuleConfig" .) "true" -}}
{{- if semverCompare ">=1.34" .Values.global.discovery.kubernetesVersion -}}
{{- if has "DRAResourceClaimDeviceStatus" .Values.virtualization.internal.kubeAPIServerFeatureGates -}}
{{- if has "DRADeviceBindingConditions" .Values.virtualization.internal.kubeAPIServerFeatureGates -}}
{{- if has "DRAConsumableCapacity" .Values.virtualization.internal.kubeAPIServerFeatureGates -}}
true
{{- end -}}
{{- end -}}
{{- end -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "virtualization-dra.featureGates" -}}
{{- $base := "--feature-gates=USBGateway=true,USBNodeLocalMultiAllocation=true" -}}
{{- if has "DRAPartitionableDevices" .Values.virtualization.internal.kubeAPIServerFeatureGates -}}
{{- $base = printf "%s,DRAPartitionableDevices=true" $base -}}
{{- end -}}
{{- $base -}}
{{- end -}}
