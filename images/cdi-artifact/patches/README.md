# Patches

#### `000-bundle-images.patch`

Iternal patch which adds images bundle target with all images to build.

#### `005-override-crds.patch`

Rename group name for all cdi CRDs to override them with deckhouse virtualization CRDs.

Also, remove short names and change categories. Just in case.

#### `006-customizer.patch`

Add `spec.customizeComponents` to the crd cdi to customize resources.

https://github.com/kubevirt/containerized-data-importer/pull/3070

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

Set the storage class name for the scratch pvc from the original pvc that will own the scratch pvc, or set it to an empty value if not available.

#### `012-add-caps-for-deckhouse-provisioners.patch`

Add capabilities for deckhouse provisioners to cdi StorageProfile.

#### `013-converting-images-in-filesystem-to-qcow2.patch`

Converting images in the file system to qcow2.

CDI can currently upload virtual machine images to persistent volumes (PVCs). Regardless of the target, whether it's a block device or a file, CDI converts the image to raw format. We're changing this behavior, but only for file targets. Conversion will now happen to the qcow2 format.

#### `014-delete-service-monitor.patch`

Removed the creation of a service monitor from the cdi-operator.
