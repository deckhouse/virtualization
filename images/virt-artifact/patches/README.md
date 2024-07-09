# Patches

#### `001-bundle-extra-images.patch`

Internal patch which adds `libguestfs`, `virt-exportserver` and `virt-exportproxy`
to images bundle target.

#### `002-fix-vcpu-count-issue.patch`

Fixes an bug where a VM was created with one socket even though more sockets were specified in the domain spec.

- https://github.com/kubevirt/kubevirt/pull/10473


#### `003-macvtap-binding.patch`

This PR adds macvtap networking mode for binding podNetwork.

- https://github.com/kubevirt/community/pull/186
- https://github.com/kubevirt/kubevirt/pull/7648

#### `004-backport-10001-from-upstream.patch`

Backport fix for VMI metric kubevirt_vmi_phase_count.

- https://github.com/kubevirt/kubevirt/pull/10001

Fix ton of errors in virt-controller logs:
{"component":"virt-controller","level":"error","msg":"Failed to create metric for VMIs phase","pos":"collector.go:256","reason":"inconsistent label cardinality: expected 7 label values but got 6
in []string{\"virtlab-rs-1\", \"running\", \"<none>\", \"<none>\", \"<none>\", \"<none>\"}",...


#### `007-tolerations-for-strategy-dumper-job.patch`

There is a problem when all nodes in cluster have taints, KubeVirt can't run virt-operator-strategy-dumper job.
The provided fix will always run the job in same place where virt-operator runs

- https://github.com/kubevirt/kubevirt/pull/9360

#### `011-virt-api-authentication.patch`
Added the ability for virt-api to authenticate clients with certificates signed by our rootCA located in the config-map virtualization-ca.

#### `012-support-kubeconfig-env.patch`
Support `KUBECONFIG` environment variable. 

#### `013-virt-api-rate-limiter.patch`
A patch has been added to enable the configuration of the rate limiter via the environment variables VIRT_API_RATE_LIMITER_QPS and VIRT_API_RATE_LIMITER_BURST.

#### `014-delete-apiserver.patch`
Do not create Kubevirt APIService.

#### `015-rename-core-resources.patch`
Replace "kubevirt" with "kubevirt-internal-virtualziation" in the core resource names.

#### `016-rename-apigroups-in-starred-rbac.patch`

Rename apiGroup to internal.virtualization.deckhouse.io for ClusterRole for virt-controller to prevent permanent patching:

```
{"component":"virt-operator","level":"info","msg":"clusterrole kubevirt-internal-virtualization-controller patched","pos":"core.go:142","timestamp":"2024-07-09T16:03:18.138751Z"}
```
