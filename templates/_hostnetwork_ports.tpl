{{- /*
Port constants for DaemonSets running with hostNetwork: true.

All three DaemonSets — virt-handler, vm-route-forge, virtualization-dra —
run with hostNetwork, so every bound port is exposed on the node's network
interfaces. Ports below are chosen outside the KubeVirt live-migration range
(4135-4199) and must not overlap with other well-known services on cluster nodes.

Port map:

  virt-handler (kube-api-rewriter runs as its sidecar):
  4135-4199  virt-handler: live-migration tunnels (KubeVirt migration range).
  4100       virt-handler: healthz and Prometheus metrics (--port flag), kube-rbac-proxy implemented natively.
  4101       virt-handler: Console server port (--console-server-port flag).
  4102       kube-api-rewriter sidecar: Prometheus metrics (MONITORING_BIND_ADDRESS), bound to pod IP.
             liveness and readiness probes (/proxy/healthz, /proxy/readyz).
  4103       kube-api-rewriter sidecar: pprof (PPROF_BIND_ADDRESS), bound to pod IP, debug mode only.
  4104       kube-api-rewriter sidecar: Kubernetes API proxy (CLIENT_PROXY_PORT),
             virt-handler connects here instead of the real API server.

  vm-route-forge:
  4105       vm-route-forge: liveness and readiness probes (HEALTH_PROBE_BIND_ADDRESS).
  4106       vm-route-forge: pprof (PPROF_BIND_ADDRESS), debug mode only.

  virtualization-dra:
  4107       virtualization-dra: gRPC liveness and readiness probes.
  4280       virtualization-dra: USB/IP daemon (--usbipd-port flag).
*/ -}}

{{- /* virt-handler */ -}}
{{- define "virt_handler.migration_port_first" -}}4135{{- end -}}
{{- define "virt_handler.migration_port_last" -}}4199{{- end -}}

{{- define "virt_handler.port" -}}4100{{- end -}}
{{- define "virt_handler.console_server_port" -}}4101{{- end -}}
{{- define "virt_handler.rewriter_healthz_port" -}}4102{{- end -}}
{{- define "virt_handler.rewriter_monitoring_port" -}}4102{{- end -}}
{{- define "virt_handler.rewriter_pprof_port" -}}4103{{- end -}}
{{- define "virt_handler.rewriter_proxy_port" -}}4104{{- end -}}

{{- /* vm-route-forge */ -}}
{{- define "vm_route_forge.health_port" -}}4105{{- end -}}
{{- define "vm_route_forge.pprof_port" -}}4106{{- end -}}

{{- /* virtualization-dra */ -}}
{{- define "virtualization_dra.health_port" -}}4107{{- end -}}
{{- define "virtualization_dra.usbipd_port" -}}4280{{- end -}}
