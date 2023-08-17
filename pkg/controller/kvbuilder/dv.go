package kvbuilder

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"

	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/helper"
)

const dockerRegistrySchemePrefix = "docker://"

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
	b.Resource.Spec.PVC = &corev1.PersistentVolumeClaimSpec{
		StorageClassName: &storageClassName,
		AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}, // TODO: ensure this mode is appropriate
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceStorage: size,
			},
		},
	}
}

func (b *DV) SetRegistryDataSource(imageName string) {
	url := dockerRegistrySchemePrefix + imageName

	b.Resource.Spec.Source.Registry = &cdiv1.DataVolumeSourceRegistry{
		URL: &url,
	}
}

func (b *DV) SetBlankDataSource() {
	b.Resource.Spec.Source.Blank = &cdiv1.DataVolumeBlankImage{}
}
