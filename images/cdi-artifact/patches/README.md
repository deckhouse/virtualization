# Patches

#### `007-content-type-json.patch`
set ContentTypeJson for kubernetes clients.

#### `008-rename-core-resources.patch`
Replace "cdi" with "cdi-internal-virtualziation" in the core resource names.

#### `009-remove-upload-apiservice.patch`

Do not install apiservice v1beta1.upload.cdi.kubevirt.io. This APIService is not used
by DVP, but conflicts with original CDI.

#### `010-stop-managing-datavolume-crd.patch`

Do not manage DataVolume CRD with cdi-operator. Module will install this CRD using Helm.

#### `011-change-storage-class-for-scratch-pvc.patch`

Force setting an empty string to Status.ScratchSpaceStorageClass if config field ScratchSpaceStorageClass is empty
to prevent using the default storage class name for the scratch pvc.

The empty string in Status.ScratchSpaceStorageClass will force the cdi-operator
to set the storage class name for the scratch pvc from the original pvc that will own the scratch pvc, or set it to an empty value if not available.

#### `012-add-caps-for-deckhouse-provisioners.patch`

Add capabilities for deckhouse provisioners to cdi StorageProfile.

#### `013-converting-images-in-filesystem-to-qcow2.patch`

Converting images in the file system to qcow2.

CDI can currently upload virtual machine images to persistent volumes (PVCs). Regardless of the target, whether it's a block device or a file, CDI converts the image to raw format. We're changing this behavior, but only for file targets. Conversion will now happen to the qcow2 format.

#### `014-delete-service-monitor.patch`

Removed the creation of a service monitor from the cdi-operator.

#### `015-fix-replace-op-for-evanphx-json-patch-v5-lib.patch`

Fix JSON patch replace operation behaviour: set value even if path is not exist.

Why we need it? 

Previous CDI version uses evanphx-json-patch version 5.6.0+incompatible.
That version ignores non-existent path and it actually a v4 from main branch.

CDI 1.60.3 uses evanphx-json-patch /v5 version, which has options to change
the behaviour for add and remove operations, but there is no option
to change behaviour for the replace operation.

#### `016-scratch-filesystem-overhead-formula.patch`

Manage the filesystem overhead of the scratch PVC using a formula derived from empirical estimates depending on virtual image size.

Why we need it?

The size of the scratch PVC is calculated based on the size of the virtual image being imported. CDI has configuration
options to set overhead percent globally or for the particular storage class. However, the overhead percent
is not the same and depends on the size the virtual VM image.
Previously we adjusted the whole PVC size to get bigger scratch PVC. This patch adds overhead for the scratch PVC only,
leaving the size of the target PVC intact.

#### `017-add-format-conversion-for-pvc-cloning.patch`

When cloning via DataVolume from PVC to PVC, there was no format conversion. 
The target PVC should have a raw type for volume mode Block and a qcow2 type for Filesystem. 
The patch adds this conversion.

#### `018-cover-cdi-metrics.patch`

Configure cdi's components metrics web servers to listen on localhost. 
This is necessary for ensuring that the metrics can be accessed only by Prometheus via kube-rbac-proxy sidecar.

Currently covered metrics:
- cdi-controller
- cdi-deployment

#### `019-optimize-csi-clone.patch`

Cloning PVC to PVC for provisioner `rbd.csi.ceph.com` now works via csi-clone instead of snapshot.
With csi-clone, it's possible to specify the same or a larger capacity for the target pvc immediately, with no need to postpone resizing.

#### `020-manage-provisioner-tolerations.patch`

Add annotation to manage provisioner tolerations to avoid unschedulable error.

#### `021-fallback-to-host-clone-for-lvm-thick.patch`

When cloning from PVC to PVC, it's necessary to select a cloning strategy. By default, the cloning strategy `snapshot` is selected.
However, `replicated.csi.storage.deckhouse.io` and `local.csi.storage.deckhouse.io` can create snapshots only when using LVM Thin.
To avoid errors, for LVM Thick, it's necessary to use `copy` cloning strategy (`csi-clone` is also unavailable since the CSI itself creates a snapshot when using `csi-clone`).

#### `022-add-datavolume-quouta-not-exceeded-condition.patch`

A new condition, QuotaNotExceeded, has been added to the DataVolume resource to indicate that the project's quotas have not been exceeded.

This patch includes an architectural assumption where the condition of the DataVolume resource is modified by an external controller. In the future, CDI usage is planned to be discontinued, making this assumption non-disruptive.

#### `023-remove-upload-proxy-server-variables.patch`

The CDI uploadproxy and serverproxy functionality is not used. Deployment of these images and deployments has been removed.

#### `024-cdi-controller-change-bash-utils-to-binary.patch`

We want fully reproducible distroless images (without bash). This patch replaces bash usage with static binaries:
- `bash -c "echo 'hello cdi'"` is replaced with "hello" binary.
- `cat /tmp/ready` is replaced with "printFile /tmp/ready"