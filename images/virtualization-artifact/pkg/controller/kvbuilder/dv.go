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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"

	"github.com/deckhouse/virtualization-controller/pkg/common"
	"github.com/deckhouse/virtualization-controller/pkg/common/pvc"
	cc "github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
)

const (
	unpackFormatQcow2 = "qcow2"
	unpackFormatRaw   = "raw"
)

// DV is a helper to construct DataVolume to import an image from DVCR onto PVC.
type DV struct {
	helper.ResourceBuilder[*cdiv1.DataVolume]
}

func NewDV(name types.NamespacedName) *DV {
	return &DV{
		ResourceBuilder: helper.NewResourceBuilder(
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
						cc.AnnUnpackFormat: unpackFormatQcow2,
					},
				},
				Spec: cdiv1.DataVolumeSpec{
					Source: &cdiv1.DataVolumeSource{},
				},
			}, helper.ResourceBuilderOptions{},
		),
	}
}

func (b *DV) SetPVC(storageClassName *string,
	size resource.Quantity,
	accessMode corev1.PersistentVolumeAccessMode,
	volumeMode corev1.PersistentVolumeMode,
) {
	b.Resource.Spec.PVC = pvc.CreateSpecForDataVolume(storageClassName,
		size,
		accessMode,
		volumeMode,
	)
}

func (b *DV) SetDataSource(source *cdiv1.DataVolumeSource) {
	b.Resource.Spec.Source = source
}

func (b *DV) SetUnpackFormat(mode corev1.PersistentVolumeMode) {
	format := unpackFormatRaw
	switch mode {
	case corev1.PersistentVolumeBlock:
		format = unpackFormatRaw
	case corev1.PersistentVolumeFilesystem:
		format = unpackFormatQcow2
	}
	annos := b.Resource.ObjectMeta.GetAnnotations()
	if annos == nil {
		annos = map[string]string{}
	}
	annos[cc.AnnUnpackFormat] = format
	b.Resource.ObjectMeta.SetAnnotations(annos)
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
