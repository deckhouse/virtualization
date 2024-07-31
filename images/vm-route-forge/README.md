## vm-route-forge

This controller watches for VirtualMachines in virtualization.deckhouse.io group and updates routes in table 1490 to route traffic between VMs via Cilium agents.

It should be run as a DaemonSet with the `hostNetwork: true` to be able to modify route tables on cluster Nodes.

### Configuration

#### Log verbosity

Set VERBOSITY environment variable or -v flag.

#### Route table ID

Hardcoded as integer `1490`.

#### CIDRs

Use --cidr flags to specify CIDRs to limit managed IPs. Controller will update routes for VMs which IPs belong to specified CIDRs.

Example:

```
vm-route-forge --cidr 10.2.0.0/24 --cidr 10.2.1.0/24 --cidr 10.2.2.0/24 
```

Controller will update route for VM with IP 10.2.1.32, but will ignore VM with IP 10.2.4.5.

#### Dry run mode

Use --dry-run flag to enable non destructive mode. The controller will not actually delete or replace rules and routes, only log these actions.

#### Healthz addresses

Controller can't predict used ports when starting in host network mode. So, be default, healthz are started on random free ports. Use flags to specify these addresses:

`--health-probe-bind-address` - set port for /healthz endpoint, e.g. `--health-probe-bind-address=:9321`

