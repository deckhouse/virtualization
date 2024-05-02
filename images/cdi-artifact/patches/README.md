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