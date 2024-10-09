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

Why we needed it? 

Previous CDI version uses evanphx-json-patch version 5.6.0+incompatible.
That version ignores non-existent path and it actually a v4 from main branch.

CDI 1.60.3 uses evanphx-json-patch /v5 version, which has options to change
the behaviour for add and remove operations, but there is no option
to change behaviour for the replace operation.

#### `016-scratch-filesystem-overhead-formula.patch`

Manage the filesystem overhead of the scratch PVC using a formula derived from empirical estimates, adjusted for the target PVC size.
