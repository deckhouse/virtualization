# In-cluster profiling

Deploys Grafana Pyroscope and an Alloy DaemonSet that continuously scrapes pprof
endpoints of the module components and labels every profile with `service_name`,
`pod`, `node` and `container`, so any instance can be picked in the Pyroscope UI.

Intended for dev clusters only.

## Usage

1. Enable pprof endpoints: set `logLevel: debug` in the `virtualization` ModuleConfig.
2. `task pprof:install`
3. `task pprof:ui` and open <http://localhost:4040>.
4. `task pprof:uninstall` when done. `task pprof:wipe` drops collected profiles.

## Components

| service_name              | endpoint               | reached via            |
|---------------------------|------------------------|------------------------|
| virtualization-controller | `:8081`                | pod IP                 |
| vm-route-forge            | `:4106`                | node IP (hostNetwork)  |
| virtualization-dra        | `127.0.0.1:4109`       | loopback (hostNetwork) |
| virtualization-dra-pci    | `:8081`                | pod IP                 |

To add a component, declare a container port named `pprof` in its chart template
and add its `app` label to the `keep` rule in `manifests/config.alloy`.
