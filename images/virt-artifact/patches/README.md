# Patches

#### `001-bundle-extra-images.patch`

Internal patch which adds `libguestfs`, `virt-exportserver` and `virt-exportproxy`
to images bundle target.

#### `005-prevent-permanent-patching-of-services.patch`

Fix patching of Services during each reconcile:

```
{"component":"virt-operator","level":"info","msg":"service kubevirt-prometheus-metrics patched","pos":"core.go:142","timestamp":"2024-07-09T16:03:18.136326Z"}
{"component":"virt-operator","level":"info","msg":"service virt-api patched","pos":"core.go:142","timestamp":"2024-07-09T16:03:18.138751Z"}
{"component":"virt-operator","level":"info","msg":"service kubevirt-operator-webhook patched","pos":"core.go:142","timestamp":"2024-07-09T16:03:18.140853Z"}
{"component":"virt-operator","level":"info","msg":"service virt-exportproxy patched","pos":"core.go:142","timestamp":"2024-07-09T16:03:18.142806Z"}
```

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

#### `016-rename-install-strategy-labels.patch`

Rename kubevirt.io/install-strategy-registry labels to install.internal.virtualization.deckhouse.io/install-strategy-registry.
Rename app.kubernetes.io/managed-b value from virt-operator to virt-operator-internal-virtualization.

Rewrite these labels with patch, because strategy generator Job starts without kube-api-rewriter.

#### `017-fix-vmi-subresource-url.patch`

Use virtualization-api instead subresources.kubevirt.io for vmi operations.

#### `018-rename-devices-kubevirt-io.patch`

Rename additional resources previded with Device Plugin API to not overlap with original Kubevirt.

Rename unix-socket path used for register devices.

#### `019-remove-deprecation-warnings-from-crds.patch`

Virtualization-controller doesn't use deprecated apiGroup versions. Deprecation warnings are distracting in our case.

#### `020-stop-managing-kvvm-kvvmi-crds.patch`

Stop managing VirtualMachine and VirtualMachineInstance CRDs with virt-operator. Module will install this CRDs using Helm.

#### `021-support-qcow2-for-filesystem.patch`

Support format qcow2 for pvc with filesystem mode.

When generating XML for libvirt, we utilize converters that translate the virtual machine instance specification into a Domain. We're making a slight adjustment to this process.
We're changing the raw format for disks to qcow2 for all images created on the file system. These values are hardcoded as we can't determine the disk format used by the virtual machine through qemu-img.
Additionally, kubevirt can create images on an empty PVC. We're changing this behavior as well, altering the format of the created disk to qcow2. This is achieved using qemu-img.

#### `022-cleanup-error-pods.patch`

Cleanup stale Pods owned by the VMI, keep only last 3 in the Failed phase.

Why we need it?

Unsuccessful migrations may leave a lot of Pods. These huge lists reduce performance on virtualization-controller and cdi-deployment restarts.

#### `023-replace-expressions-for-validating-admission-policy.patch`

Replace the expressions for the ValidatingAdmissionPolicy kubevirt-node-restriction-policy.
This is necessary because of the kube-api-rewriter that changes the labels.

#### `024-cover-kubevirt-metrics.patch`

Configure kubevirt's components metrics web servers to listen on localhost. 
This is necessary for ensuring that the metrics can be accessed only by Prometheus via kube-rbac-proxy sidecar.

Currently covered metrics:
- virt-handler
- virt-controller
- virt-api

#### `025-stream-graceful-shutdown.patch`

Graceful termination of websocket connection for serial console and vnc connections.

#### `026-add-healthz-to-virt-operator.patch`

Add separate healthz endpoint to virt-operator.

#### `027-auto-migrate-if-nodeplacement-changed.patch`

Start the migration if the nodeSelector or affinity has changed.
How does it work?
1. When changing the affinity or nodeSelector in the vm, the vm controller updates the vmi specification.
2. When changing the affinity or nodeSelector in vmi, the vmi controller will set the `NodePlacementNotMatched` condition to True in vmi.
3. The workload-updater controller monitors the vmi and starts migration when there is a `NodePlacementNotMatched` conditions on the vmi.
4. When the migration is completed, virt-handler will remove the condition `NodePlacementNotMatched` from the vmi 

#### `028-inject-placement-anynode.patch`

By default, the virtual-operator adds a nodePlacement with the RequireControlPlanePreferNonWorker.
But we set up the placement ourselves, so we replace the policy with AnyNode.

#### `029-use-OFVM_CODE-for-linux.patch`

Kubevirt uses OVFM_CODE.secboot.fd in 2 combinations: OVFM_CODE.secboot.fd + OVFM_VARS.secboot.fd when secboot is enabled and OVFM_CODE.secboot.fd + OVFM_VARS.fd when secboot is disabled.
It works fine with original CentOS based virt-launcher in both secboot modes.
We use ALTLinux based virt-launcher, and it fails to start Linux VM with more than 12 CPUs in secboot disabled mode.

Kubevirt uses flags to detect firmware combinations in converter.
EFIConfiguration, so we can't set needed files directly. 
But there is combination for SEV: OVFM_CODE.cc.fd + OVMF_VARS.fd that works for Linux, because OVFM_CODE.cc.fd is actually a symlink to OVFM_CODE.fd. 
So, we set true for the second flag to force OVFM_CODE.cc.fd + OVMF_VARS.fd for non-Windows virtual machines._

#### `030-hide-target-pod-during-migration-via-cilium-label.patch`

During the VM migration process, two pods with the same address are created and packets are randomly delivered to them. 
To force delivery of packages to only one VM pod, the special label `network.deckhouse.io/hidden-pod` for target pod were added.
When the migration completes, the label is removed and the target pod becomes accessible via network.

d8-cni-cilium ensures that once the label is removed from the target pod, only the target pod remains accessible over the network (while the source pod does not).

#### `031-prevent-adding-node-selector-for-dvp-generic-cpu-model.patch`

- Do not add cpu-model nodeSelector for "kvm64" model. This selector prevents starting VMs as node-labeler ignores to labeling nodes with "kvm64" model.

- Overwrite calculated model on migration, put back "kvm64" for Discovery and Features vmclass types.
