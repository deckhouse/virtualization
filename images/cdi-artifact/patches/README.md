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

#### `010-rename-apigroups-in-starred-rbac.patch`

Rename apiGroup to internal.virtualization.deckhouse.io for ClusterRole for cdi-deployment to prevent permanent patching:

```
{"level":"debug","ts":"2024-06-28T12:39:26Z","logger":"events","msg":"Successfully updated resource *v1.ClusterRole cdi-internal-virtualization","type":"Normal","object":{"kind":"CDI","name":"config","uid":"2e7b5bf7-2c38-4118-a80d-04a8e67ca08b","apiVersion":"cdi.kubevirt.io/v1beta1","resourceVersion":"420200766"},"reason":"UpdateResourceSuccess"}
```
