package kvbuilder

import (
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"

	"github.com/deckhouse/virtualization-controller/pkg/common"
	"github.com/deckhouse/virtualization-controller/pkg/common/pvc"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
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
						"cdi.kubevirt.io/storage.deleteAfterCompletion":    "false",
						"cdi.kubevirt.io/storage.bind.immediate.requested": "true",
					},
				},
				Spec: cdiv1.DataVolumeSpec{
					Source: &cdiv1.DataVolumeSource{},
				},
			}, helper.ResourceBuilderOptions{},
		),
	}
}

func (b *DV) SetPVC(storageClassName *string, size resource.Quantity) {
	b.Resource.Spec.PVC = pvc.CreateSpecForDataVolume(storageClassName, size)
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
