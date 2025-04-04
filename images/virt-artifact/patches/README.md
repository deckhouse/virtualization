# Patches

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

#### `030-prevent-adding-node-selector-for-dvp-generic-cpu-model.patch`

- Do not add cpu-model nodeSelector for "kvm64" model. This selector prevents starting VMs as node-labeler ignores to labeling nodes with "kvm64" model.

- Overwrite calculated model on migration, put back "kvm64" for Discovery and Features vmclass types.

#### `031-hotplug-container-disk.patch`

Add Hotplug container-disk volumes.
How `container-disk` and HotPlug Work
The `container-disk` is a program written in C used within KubeVirt to facilitate the mounting of container-based disk images into virtual machines. Its core function is to start up and create a UNIX socket within a specific directory. The program terminates when the socket is removed or upon receiving a `SIGTERM` signal.

##### Key Workflow: `container-disk`

##### Initialization
- A sidecar container, running the `container-disk` image, is created alongside the `virt-launcher` pod.
- An init-container in the `virt-launcher` pod copies the `container-disk` program to a shared `emptyDir` volume. This setup allows the sidecar to execute the program.

##### Socket Creation

- The `container-disk` program creates a socket in the `emptyDir` volume.
- This shared volume allows the `virt-handler` to locate the socket on the host machine at:
  `/var/lib/kubelet/pods/.../volumes/kubernetes.io~empty-dir/`.

##### Socket Detection and Mounting

- Upon detecting the socket, `virt-handler` identifies it as a `container-disk` volume and retrieves its parent mount point.
- For a container runtime like `containerd`, the mount point resolves to the root filesystem of the pulled image, typically at:
  `/run/containerd/io.containerd.runtime.v2.task/k8s.io/<uid>/rootfs/`.
- The disk image must be located at `disk/disk.img` within this filesystem and is mounted into the VM.

## HotPlug in KubeVirt
The HotPlug mechanism allows dynamic attachment of PVCs and `container-disk` volumes to a running VM by leveraging a separate `hotplug` pod.

### HotPlug Pod Setup
- A `hotplug` pod is created with the target PVCs mounted into an `emptyDir` volume under the `/hp` directory.
- The `container-disk` program runs in the `hotplug` pod to create the necessary sockets for these volumes.

### Volume Detection and Mounting
- The `virt-handler` locates the sockets on the host system at:
  `/var/lib/kubelet/pods/<uid-hotplug-pod>/volumes/empty-dir/hp-disks/...`.
- For block devices, `virt-handler` creates a block device on the VM using `mknodat`.
- For file systems, the volume is mounted as a file.

### Unmounting
- The unmount process is identical to that of `hotplug PVCs`.
- The `emptyDir` resources are retained and cleaned up later by Kubernetes.

#### `032-fix-virt-controller-tests.patch`

Fix unit tests for virt-controller.

```shell
# Use to run tests:
ginkgo -succinct /home/dmitrii/Base/Flant/kubevirt/pkg/virt-controller/...
```

#### `033-manage-pods-network-priotity-during-migration-using-cilium-label.patch`

**Problem:**
During the VM migration process, two pods with the same address are created and packets are randomly delivered to them.

**Solution**:
To force delivery of packages to only one VM pod, the special label `network.deckhouse.io/pod-common-ip-priority` were added.
The label allows setting the priority of pod for cilium relative to other pods with the same IP address.
Network traffic will be directed to the pod with the higher priority.
Absence of the label means the lowest priority (pod with a network priority label is more prioritized than a pod without a label).
The lower the numerical value, the higher the priority.

**How does it work?**
1. When migration starts, the source pod receives a decreased network priority ("1").
2. The target pod is immediately created with the lowest network priority ("2").
3. When the virtual machine is suspended for offline migration, the target pod receives the highest network priority ("0"),
   while the source pod retains its decreased priority ("1").

Thus, packets are delivered as expected: initially only to the source pod during migration, and after migration completes, only to the target pod.

#### `034-allow-update-kvvmi-for-virtualization-sas.patch`

By default, the KVVMI spec can update only KubeVirt service accounts. This patch adds our virtualization accounts to the allowed list.
(`virtualization-controller`, `virtualization-api`)

#### `035-allow-change-serial-on-kvvmi.patch`

By default, the disk specification is immutable, but for backward compatibility, we need to allow modifying the serial.

#### `036-enhance-SCSI-disk-serial-validation.patch`

**Related Issue:** [#13858](https://github.com/kubevirt/kubevirt/issues/13858)
**Pull Request:** [#13859](https://github.com/kubevirt/kubevirt/pull/13859)

##### What this PR does
- **Before:** A virtual machine (VM) launched by QEMU could fail if a disk's serial number exceeded 36 characters, as QEMU enforces this limit. KubeVirt did not validate this beforehand, leading to runtime errors.
- **After:**
  - The API now validates disk serial numbers, preventing users from setting values longer than 36 characters and avoiding VM startup failures in QEMU.
  - For existing VMs, serial numbers exceeding this limit will be automatically truncated to maintain backward compatibility.

##### Why this change?
This update ensures compatibility with recent QEMU changes and prevents runtime errors by enforcing validation at the API level while preserving support for existing VMs through automatic serial number truncation.

#### `037-set-ReadOnlyRootFilesystem-to-virt-launcher.patch`
To enhance security, this patch enables ReadOnlyRootFilesystem for the virt-launcher compute pod.
Since libvirt and QEMU require writable directories, five emptyDir volumes are added and mounted to:
- /var/run
- /var/log
- /etc/libvirt
- /var/lib/libvirt/
- /var/cache/libvirt
- /var/lib/swtpm-localca
- /tmp

This ensures compatibility while maintaining a read-only root filesystem for improved isolation and security.

#### `038-disable-unnecessary-libvirt-sockets.patch`

This patch disables unnecessary libvirt sockets by running `virtqemud` and `virtlogd` with additional flags to prevent their creation. Specifically, we disable the admin and read-only servers:

- `--no-admin-srv` and `--no-ro-srv` flags for `virtqemud`.
- `--no-admin-srv` flag for `virtlogd`.

By using these flags, the following sockets are not created in the first place:

- `/var/run/libvirt/virtlogd-admin-sock`
- `/var/run/libvirt/virtqemud-admin-sock`
- `/var/run/libvirt/virtqemud-sock-ro`

This ensures a cleaner runtime environment, reducing unnecessary components and preventing unintended interactions, without affecting libvirt's core functionality.

##### Dependency

This patch depends on the [001-disable-ro-and-admin-servers.patch](../../libvirt/patches/001-disable-ro-and-admin-servers.patch) for proper flag application to disable the relevant servers and prevent the creation of their associated sockets.

#### `039-get-applied-checksum.patch`

This patch introduces the GetAppliedChecksum() method in virt-launcher.

virt-handler now tracks synchronized VMIs and computes their checksums. Periodically, it queries virt-launcher for the applied checksum. This ensures that we have two checksums:

The last one sent for synchronization.
The one actually applied.
Storing these checksums in VMI annotations helps verify that spec changes were pushed by virt-handler and not by an attacker.

#### `040-set-reboot-policy.patch`

This patch modifies the behavior of domain reboot actions in virt-launcher by overriding the default reboot policy.

### Changes Introduced:
- Registers a QEMU monitor shutdown event callback to handle shutdown events.
- Sends a QEMU agent command to override the reboot action, changing it from `reboot` to `shutdown`.
- Logs shutdown events and writes them to `/dev/termination-log`.
- Ensures that domain shutdown events are captured and processed correctly.

#### `041-rename-node-labeller-virt-launcher-init.patch`

This patch modifies init container args in virt-launcher images from node-labeller.sh to node-labeller.
This bash script has been rewritten to golang.

#### `042-restrict-libvirt-socket-to-qemu.patch`

This patch enhances security by ensuring that `virtqemud` only accepts connections from its corresponding process. It achieves this by using the `LIBVIRT_UNIX_SOCKET_AUTH_PID` environment variable, which restricts access to the process ID (PID) of the `virt-launcher` process that started `virtqemud`.

#### `043-add-qemu-and-libvirt-versions`

This path adds annotations to the VMI with the versions of libvirt and qemu used to create the VMI.

##### Changes
- Configures `virtqemud` to use the `LIBVIRT_UNIX_SOCKET_AUTH_PID` environment variable, restricting access to the `virt-launcher` process.
- Ensures that `virtqemud` only accepts connections from the process that initiated it.
- Prevents unauthorized processes from accessing the libvirt socket, reducing security risks and the potential for privilege escalation.
- Updates the migration mechanism: since virt-handler directly connects to the `virtqemud` socket during migration, the libvirt patch does not authorize it. To address this issue, an additional `migration-proxy` has been introduced in `virt-launcher`. This proxy receives traffic from `virt-handler` and forwards it to `virtqemud`.
- A new gRPC call, MigrationProxy, has been added to start this migration proxy.



##### Dependency
This patch depends on the [002-auth-pid-restriction.patch](../../libvirt/patches/002-auth-pid-restriction.patch) in libvirt, which introduces the `LIBVIRT_UNIX_SOCKET_AUTH_PID` environment variable to restrict socket access based on PID.
