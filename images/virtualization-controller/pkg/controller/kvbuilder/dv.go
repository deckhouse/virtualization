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

type DV struct {
	helper.ResourceBuilder[*cdiv1.DataVolume]
}

func NewDV(name types.NamespacedName) *DV {
	return &DV{
		ResourceBuilder: helper.NewResourceBuilder(
			&cdiv1.DataVolume{
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

func (b *DV) SetPVC(storageClassName string, size resource.Quantity) {
	b.Resource.Spec.PVC = pvc.CreateSpecForDataVolume(storageClassName, size)
}

func (b *DV) SetRegistryDataSource(imageName string) {
	url := common.DockerRegistrySchemePrefix + imageName

	b.Resource.Spec.Source.Registry = &cdiv1.DataVolumeSourceRegistry{
		URL: &url,
	}
}

func (b *DV) SetBlankDataSource() {
	b.Resource.Spec.Source.Blank = &cdiv1.DataVolumeBlankImage{}
}
