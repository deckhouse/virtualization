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
