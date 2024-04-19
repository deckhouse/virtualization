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

#### `007-tolerations-for-strategy-dumper-job.patch`

There is a problem when all nodes in cluster have taints, KubeVirt can't run virt-operator-strategy-dumper job.
The provided fix will always run the job in same place where virt-operator runs

- https://github.com/kubevirt/kubevirt/pull/9360

#### `010-override-crds.patch`

Rename group name for all kubevirt CRDs to override them with deckhouse virtualization CRDs.

Also, remove short names and change categories. Just in case.

#### `011-virt-api-authentication.patch`
Added the ability for virt-api to authenticate clients with certificates signed by our rootCA located in the config-map virtualization-ca.

#### `012-support-kubeconfig-env.patch`
Support `KUBECONFIG` environment variable. 
