{{- define "virtualization-controller.envs" -}}
- name: DISABLE_HYPERV_SYNIC
  value: "1"
{{- end }}
