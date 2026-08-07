{{- if eq (include "audit.isEnabled" .) "true" }}
- name: kubernetes.virtualization.audit_state
  rules:
    - alert: D8VirtualizationSecurityEventsNotRecorded
      # The absent() branch covers a deleted Deployment: without it the series is gone
      # and `== 0` has nothing to compare. It is safe here only because these rules are
      # rendered when audit is enabled at all. The guard keeps the alert silent when the
      # metrics of every Deployment in the namespace are missing — that is a failure of
      # kube-state-metrics, not of the audit component.
      # The absent() branch only counts when the object existed at some point during the
      # last week: during the very first installation the components created by
      # virt-operator do not exist yet, and a slow rollout must not page anybody.
      expr: |
        (
          max by (namespace, deployment) (
            kube_deployment_status_replicas_available{namespace="d8-virtualization", deployment="virtualization-audit"}
          ) == 0
          or
          (
            absent(kube_deployment_status_replicas_available{namespace="d8-virtualization", deployment="virtualization-audit"})
            and on()
            count(max_over_time(kube_deployment_status_replicas_available{namespace="d8-virtualization", deployment="virtualization-audit"}[7d])) > 0
          )
        )
        unless on()
        absent(kube_deployment_status_replicas_available{namespace="d8-virtualization"})
      labels:
        severity_level: "5"
        tier: cluster
      for: 10m
      annotations:
        plk_protocol_version: "1"
        plk_markup_format: "markdown"
        plk_create_group_if_not_exists__d8_virtualization_health: "D8VirtualizationHealth,tier=~tier,prometheus=deckhouse,kubernetes=~kubernetes"
        plk_grouped_by__d8_virtualization_health: "D8VirtualizationHealth,tier=~tier,prometheus=deckhouse,kubernetes=~kubernetes"
        summary: Security events of the virtualization module are not recorded.
        description: |
          Audit is enabled in the module settings, but the virtualization-audit component has no available replicas, so no security events are produced.

          Impact:
          - Access to virtual machines through the console, VNC and port forwarding, changes to virtual machines, and forbidden operations are no longer recorded.
          - The events that happen while the component is down are lost for good: they are not collected retroactively.
          - Nothing else is affected: virtual machines, their disks, networking and the module API keep working.

          The recommended course of action:
          1. Retrieve details of the Deployment: `d8 k -n d8-virtualization describe deploy virtualization-audit`
          2. View the logs: `d8 k -n d8-virtualization logs deploy/virtualization-audit --tail=100`
          3. Check that the audit TLS certificate exists: `d8 k -n d8-virtualization get secret virtualization-audit-tls`
{{- end }}
