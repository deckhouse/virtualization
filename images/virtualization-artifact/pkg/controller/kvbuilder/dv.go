/*
Copyright 2024 Flant JSC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package kvbuilder

import (
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"

	"github.com/deckhouse/virtualization-controller/pkg/common"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/provisioner"
	"github.com/deckhouse/virtualization-controller/pkg/common/pvc"
	"github.com/deckhouse/virtualization-controller/pkg/common/resource_builder"
)

// DV is a helper to construct DataVolume to import an image from DVCR onto PVC.
type DV struct {
	resource_builder.ResourceBuilder[*cdiv1.DataVolume]
}

func NewDV(name types.NamespacedName) *DV {
	return &DV{
		ResourceBuilder: resource_builder.NewResourceBuilder(
			&cdiv1.DataVolume{
				TypeMeta: metav1.TypeMeta{
					Kind:       "DataVolume",
					APIVersion: cdiv1.SchemeGroupVersion.String(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: name.Namespace,
					Name:      name.Name,
					Annotations: map[string]string{
						"cdi.kubevirt.io/storage.deleteAfterCompletion": "false",
					},
				},
				Spec: cdiv1.DataVolumeSpec{
					Source: &cdiv1.DataVolumeSource{},
				},
			}, resource_builder.ResourceBuilderOptions{},
		),
	}
}

func (b *DV) SetPVC(storageClassName *string,
	size resource.Quantity,
	accessMode corev1.PersistentVolumeAccessMode,
	volumeMode corev1.PersistentVolumeMode,
) {
	b.Resource.Spec.PVC = pvc.CreateSpec(storageClassName,
		size,
		accessMode,
		volumeMode,
	)
}

func (b *DV) SetImmediate() {
	b.AddAnnotation("cdi.kubevirt.io/storage.bind.immediate.requested", "true")
}

func (b *DV) SetDataSource(source *cdiv1.DataVolumeSource) {
	b.Resource.Spec.Source = source
}

func (b *DV) SetNodePlacement(nodePlacement *provisioner.NodePlacement) error {
	if nodePlacement == nil || len(nodePlacement.Tolerations) == 0 {
		return nil
	}

	anno := b.Resource.GetAnnotations()
	if anno == nil {
		anno = make(map[string]string)
	}

	data, err := json.Marshal(nodePlacement.Tolerations)
	if err != nil {
		return fmt.Errorf("failed to marshal tolerations: %w", err)
	}

	anno[annotations.AnnProvisionerTolerations] = string(data)

	err = provisioner.KeepNodePlacementTolerations(nodePlacement, b.Resource)
	if err != nil {
		return fmt.Errorf("failed to keep node placement: %w", err)
	}

	return nil
}

func (b *DV) SetRegistryDataSource(imageName, authSecret, caBundleConfigMap string) {
	url := common.DockerRegistrySchemePrefix + imageName

	dataSource := cdiv1.DataVolumeSourceRegistry{
		URL: &url,
	}

	if authSecret != "" {
		dataSource.SecretRef = &authSecret
	}
	if caBundleConfigMap != "" {
		dataSource.CertConfigMap = &caBundleConfigMap
	}

	b.Resource.Spec.Source.Registry = &dataSource
}

func (b *DV) SetBlankDataSource() {
	b.Resource.Spec.Source.Blank = &cdiv1.DataVolumeBlankImage{}
}
