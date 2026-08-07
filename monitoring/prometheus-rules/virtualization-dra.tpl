{{- if eq (include "virtualization-dra.isEnabled" .) "true" }}
- name: kubernetes.virtualization.dra_state
  rules:
    - alert: D8VirtualizationUsbPassthroughUnavailable
      expr: |
        max by (namespace, daemonset) (
          kube_daemonset_status_number_available{namespace="d8-virtualization", daemonset="virtualization-dra"}
        ) == 0
      for: 15m
      labels:
        severity_level: "4"
        tier: cluster
      annotations:
        plk_protocol_version: "1"
        plk_markup_format: "markdown"
        plk_create_group_if_not_exists__d8_virtualization_health: "D8VirtualizationHealth,tier=~tier,prometheus=deckhouse,kubernetes=~kubernetes"
        plk_grouped_by__d8_virtualization_health: "D8VirtualizationHealth,tier=~tier,prometheus=deckhouse,kubernetes=~kubernetes"
        summary: USB passthrough is enabled but works nowhere.
        description: |
          The virtualization-dra DaemonSet is enabled, but not a single one of its Pods is available in the cluster.

          Impact:
          - USB passthrough does not work anywhere: virtual machines requesting a USB device will not start on any node.
          - Everything else in the module is unaffected.

          Note that Kubernetes reports the DaemonSet as healthy in this state: it only schedules Pods onto nodes that match the required labels, so when no node matches, the desired count is zero and nothing looks wrong.

          The recommended course of action:
          1. virtualization-dra requires two node labels. Check which nodes carry them:
             `d8 k get nodes -l virtualization.deckhouse.io/usbip=true`
             `d8 k get nodes -l virtualization.deckhouse.io/containerd-version=v2`
          2. If the `usbip` label is missing, the `usbip_core`, `usbip_host` and `vhci_hcd` kernel modules could not be loaded on the nodes — check the kernel version and the module packages.
          3. If the `containerd-version` label is not `v2`, the node runs containerd v1, which cannot run the required NRI hooks.
          4. Inspect the Pods if they exist but fail: `d8 k -n d8-virtualization describe pod -l app=virtualization-dra`

    - alert: D8VirtualizationUsbPassthroughDegraded
      # Both branches are aggregated without labels on purpose: count() drops them anyway,
      # and matching label sets keep the two branches from raising two separate incidents.
      # `or vector(0)` is what makes the worst case work: with no node carrying all three
      # labels the second count() returns an empty vector and the subtraction would yield
      # nothing at all. The first count() needs no such fallback — the nodes ready for USB
      # passthrough are a subset of the nodes running virtual machines, so it can only be
      # empty when the second one is too, and then there is nothing to report anyway.
      expr: |
        max (
          kube_daemonset_status_desired_number_scheduled{namespace="d8-virtualization", daemonset="virtualization-dra"}
          - kube_daemonset_status_number_available{namespace="d8-virtualization", daemonset="virtualization-dra"}
        ) > 0
        or
        (
          count(kube_node_labels{label_virtualization_deckhouse_io_kvm_enabled="true"})
          -
          (
            count(
              kube_node_labels{
                label_virtualization_deckhouse_io_kvm_enabled="true",
                label_virtualization_deckhouse_io_usbip="true",
                label_virtualization_deckhouse_io_containerd_version="v2"
              }
            )
            or vector(0)
          )
        ) > 0
      # A DaemonSet rolls out one Pod at a time, so on a large cluster a routine update
      # takes longer than the usual 15 minutes. Nothing here has to be handled at night.
      for: 1h
      labels:
        severity_level: "6"
        tier: cluster
      annotations:
        plk_protocol_version: "1"
        plk_markup_format: "markdown"
        plk_create_group_if_not_exists__d8_virtualization_health: "D8VirtualizationHealth,tier=~tier,prometheus=deckhouse,kubernetes=~kubernetes"
        plk_grouped_by__d8_virtualization_health: "D8VirtualizationHealth,tier=~tier,prometheus=deckhouse,kubernetes=~kubernetes"
        summary: USB passthrough is unavailable on some nodes.
        description: |
          USB passthrough does not work on every node that runs virtual machines, either because the component failed there or because the node does not meet its requirements.

          Impact:
          - A virtual machine requesting a USB device will not start on the affected nodes, and may fail to schedule at all if the remaining nodes cannot host it.
          - Virtual machines without USB devices are unaffected.

          The recommended course of action:

          1. Compare the nodes that run virtual machines with those ready for USB passthrough:
             ```bash
             d8 k get nodes -l virtualization.deckhouse.io/kvm-enabled=true
             d8 k get nodes -l virtualization.deckhouse.io/usbip=true,virtualization.deckhouse.io/containerd-version=v2
             ```
          2. A node missing from the second list does not meet the requirements:
             - no `usbip` label — the `usbip_core`, `usbip_host` and `vhci_hcd` kernel modules could not be loaded there;
             - `containerd-version` is not `v2` — the node runs containerd v1, which cannot run the required hooks.
          3. If a node has both labels but the component is still not running there, inspect it:
             `d8 k -n d8-virtualization get pod -l app=virtualization-dra --field-selector=status.phase!=Running -o wide`
{{- end }}
